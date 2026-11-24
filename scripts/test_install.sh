#!/bin/sh
# Test every release download and scripts/install.sh, with no GitHub. It
# copies the tree as it is now, tracked and untracked files but not the
# ignored ones, into a throwaway git repo under $TMPDIR, tags v0.99.0 there,
# and runs scripts/build_release.sh. Then it checks each tarball and
# installs each one through file:// URLs into folders under $TMPDIR.
#
#   sh scripts/test_install.sh
#
# Takes about a minute, most of it the smoke test of the release build.
# Needs a Mac. The amd64 install runs under Rosetta; without Rosetta that
# case is skipped. Nothing outside $TMPDIR is written: no tag in this repo,
# no change to ~/.config/rex.
cd "$(dirname "$0")/.." || exit 1
# Go stamps a tag of v2 or above only when the module path ends in /vN.
TAG=v0.99.0
PASS=0
FAIL=0

ok() { PASS=$((PASS + 1)); echo "   ok   $1"; }
bad() { FAIL=$((FAIL + 1)); echo "   FAIL $1"; }
check() { # check NAME COMMAND...
  NAME=$1; shift
  if "$@" >/dev/null 2>&1; then ok "$NAME"; else bad "$NAME"; fi
}

[ "$(uname -s)" = Darwin ] || { echo "test_install: needs a Mac" >&2; exit 1; }

WORK=$(mktemp -d "${TMPDIR:-/tmp}/rex-fingers-test-install.XXXXXX") || exit 1
trap 'rm -rf "$WORK"' EXIT
trap 'exit 1' INT TERM
REPO=$WORK/repo
REL=$WORK/rel/releases/download/$TAG
mkdir -p "$REPO" "$REL"

echo "== the release build of the tree, as $TAG"
git ls-files -z -co --exclude-standard | xargs -0 tar -cf - | tar -xf - -C "$REPO" || exit 1
(
  cd "$REPO" && git init -q && git add -A &&
    git -c user.name=test -c user.email=test@localhost commit -qm "test: the tree" &&
    git -c user.name=test -c user.email=test@localhost tag -a "$TAG" -m "$TAG"
) || { echo "test_install: the throwaway repo failed" >&2; exit 1; }
if sh "$REPO/scripts/build_release.sh" "$TAG" >"$WORK/build.log" 2>&1; then
  ok "build_release.sh $TAG"
else
  bad "build_release.sh $TAG"
  tail -20 "$WORK/build.log"
  exit 1
fi
cp "$REPO"/dist/*.tar.gz "$REPO/dist/SHA256SUMS" "$REL/"

echo "== each download"
for ARCH in arm64 amd64; do
  TARBALL=rex-fingers-$TAG-darwin-$ARCH.tar.gz
  [ -f "$REL/$TARBALL" ] || { bad "$TARBALL exists"; continue; }
  tar -tzf "$REL/$TARBALL" | sort >"$WORK/list"
  check "$TARBALL: has the program" grep -qx 'rex-fingers/bin/rex-fingers' "$WORK/list"
  check "$TARBALL: has the plugin" grep -qx 'rex-fingers/lua/rex-fingers/init.lua' "$WORK/list"
  check "$TARBALL: has README.md and LICENSE" sh -c "grep -qx rex-fingers/README.md '$WORK/list' && grep -qx rex-fingers/LICENSE '$WORK/list'"
  check "$TARBALL: everything under rex-fingers/" sh -c "! grep -qv '^rex-fingers/' '$WORK/list'"
  check "$TARBALL: no ._ files" sh -c "! grep -q '/\._' '$WORK/list'"
  check "$TARBALL: no builder name" sh -c "! tar -tvzf '$REL/$TARBALL' | grep -q '$(id -un)'"
  check "$TARBALL: in SHA256SUMS" grep -q "  $TARBALL\$" "$REL/SHA256SUMS"
  mkdir -p "$WORK/unpack-$ARCH"
  tar -xzf "$REL/$TARBALL" -C "$WORK/unpack-$ARCH"
  case $ARCH in amd64) MACHO=x86_64 ;; *) MACHO=arm64 ;; esac
  check "$TARBALL: program is $MACHO" sh -c "file '$WORK/unpack-$ARCH/rex-fingers/bin/rex-fingers' | grep -q 'executable $MACHO'"
  STAMPED=$(go version -m "$WORK/unpack-$ARCH/rex-fingers/bin/rex-fingers" | awk '$1 == "mod" { print $3 }')
  check "$TARBALL: program stamped $TAG" test "$STAMPED" = "$TAG"
done
check "SHA256SUMS: matches both tarballs" sh -c "cd '$REL' && shasum -a 256 -c SHA256SUMS"
check "SHA256SUMS: two lines" test "$(wc -l <"$REL/SHA256SUMS" | tr -d ' ')" = 2

# install LOG ARCH DIR [VAR=VALUE...]: runs install.sh piped into sh, as
# the curl line does.
install() {
  LOG=$1; MODE=$2; DIR=$3; shift 3
  set -- env REX_FINGERS_URL="file://$WORK/rel" REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR="$DIR" "$@"
  if [ "$MODE" = amd64 ]; then set -- "$@" arch -x86_64 /bin/sh; else set -- "$@" /bin/sh; fi
  "$@" <"$REPO/scripts/install.sh" >"$WORK/$LOG.log" 2>&1
}
installed() { # installed DIR MACHO: the program runs and is the right one
  [ "$("$1/bin/rex-fingers" version)" = "$TAG" ] &&
    file "$1/bin/rex-fingers" | grep -q "executable $2" &&
    [ -f "$1/lua/rex-fingers/init.lua" ] &&
    ! xattr "$1/bin/rex-fingers" | grep -q com.apple.quarantine
}
leftovers() { # no .rex-fingers-install.* folder beside DIR
  for F in "$(dirname "$1")"/.rex-fingers-install.*; do [ -e "$F" ] && return 1; done
  return 0
}

echo "== install, arm64"
D=$WORK/arm64/rex-fingers
check "fresh install" install fresh-arm64 arm64 "$D"
check "program runs, no quarantine" installed "$D" arm64
check "prints the setup lines" grep -q 'fingers.setup{' "$WORK/fresh-arm64.log"
check "prints a folder of its own as a literal path" grep -qF "local fingers_dir = \"$D\"" "$WORK/fresh-arm64.log"
check "prints no reload of its own" sh -c "! grep -q 'reloaded' '$WORK/fresh-arm64.log'"
check "install over an install" install again-arm64 arm64 "$D"
check "program runs after the second install" installed "$D" arm64
check "no work folder left" leftovers "$D"

echo "== install into the default folder, with HOME in \$TMPDIR"
H=$WORK/home
check "install with no REX_FINGERS_DIR" sh -c "env -u REX_FINGERS_DIR HOME='$H' REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=$TAG sh '$REPO/scripts/install.sh' >'$WORK/default.log' 2>&1"
check "lands in HOME/.config/rex/plugins/rex-fingers" installed "$H/.config/rex/plugins/rex-fingers" arm64
check "prints the folder as README.md does" grep -qF 'local fingers_dir = os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers"' "$WORK/default.log"
check "leaves the Rex config alone" test ! -e "$H/.config/rex/init.lua"

echo "== install, amd64"
if arch -x86_64 /usr/bin/true 2>/dev/null; then
  D=$WORK/amd64/rex-fingers
  check "fresh install under Rosetta" install fresh-amd64 amd64 "$D"
  check "fetched the amd64 tarball" grep -q "darwin-amd64.tar.gz" "$WORK/fresh-amd64.log"
  check "program runs, no quarantine" installed "$D" x86_64
else
  echo "   skip amd64: Rosetta is not installed"
fi

echo "== refusals, each leaves the folder as it was"
D=$WORK/clone/rex-fingers; mkdir -p "$D/.git"
check "refuses a git clone" sh -c "! env REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR=$D sh $REPO/scripts/install.sh"
check "git clone untouched" test -d "$D/.git"
D=$WORK/foreign/rex-fingers; mkdir -p "$D"; echo keep >"$D/notes"
check "refuses a foreign folder" sh -c "! env REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR=$D sh $REPO/scripts/install.sh"
check "foreign folder untouched" grep -qx keep "$D/notes"

D=$WORK/arm64/rex-fingers
mkdir -p "$WORK/badrel/releases/download/$TAG"
cp "$REL"/* "$WORK/badrel/releases/download/$TAG/"
echo junk >>"$WORK/badrel/releases/download/$TAG/rex-fingers-$TAG-darwin-arm64.tar.gz"
check "refuses a bad checksum" sh -c "! env REX_FINGERS_URL=file://$WORK/badrel REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR=$D sh $REPO/scripts/install.sh"
check "old install still runs" installed "$D" arm64
check "no work folder left" leftovers "$D"
check "refuses a missing release" sh -c "! env REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=v0.0.1 REX_FINGERS_DIR=$WORK/missing/rex-fingers sh $REPO/scripts/install.sh"
check "refuses a malformed version" sh -c "! env REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=latest REX_FINGERS_DIR=$WORK/missing/rex-fingers sh $REPO/scripts/install.sh"
check "missing release left no folder" test ! -e "$WORK/missing/rex-fingers"

echo
echo "passed $PASS, failed $FAIL"
[ "$FAIL" = 0 ]
