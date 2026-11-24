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
# Needs a Mac. Every install runs the tarball of this Mac's processor; on
# Apple silicon with Rosetta the amd64 tarball runs too. The lookup of the
# latest release runs against a local web server that redirects as GitHub
# does; it needs python3, and is skipped without it. Nothing outside
# $TMPDIR is written, and the server answers on 127.0.0.1 only: no tag in this repo,
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
SERVER=
cleanup() {
  [ -n "$SERVER" ] && kill "$SERVER" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT
trap 'exit 1' INT TERM
REPO=$WORK/repo
REL=$WORK/rel/releases/download/$TAG
mkdir -p "$REPO" "$REL"

echo "== the release build of the tree, as $TAG"
git ls-files -z -co --exclude-standard | xargs -0 tar -cf - | tar -xf - -C "$REPO" || exit 1
# The throwaway repo turns off signing and hooks, so a maintainer's git
# setup neither prompts nor runs here; build_release.sh's worktree reads
# the same config.
(
  cd "$REPO" && git init -q &&
    git config user.name test && git config user.email test@localhost &&
    git config commit.gpgsign false && git config tag.gpgsign false &&
    git config core.hooksPath /dev/null &&
    git add -A && git commit -qm "test: the tree" && git tag -a "$TAG" -m "$TAG"
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

# This Mac's processor, as install.sh names its tarball and as file(1)
# names the program; OTHER is the tarball that runs here only under
# Rosetta, or not at all.
case $(uname -m) in
  arm64) NATIVE=arm64; NATIVE_MACHO=arm64; OTHER=amd64; OTHER_MACHO=x86_64 ;;
  *) NATIVE=amd64; NATIVE_MACHO=x86_64; OTHER=arm64; OTHER_MACHO=arm64 ;;
esac

# install LOG MODE DIR [VAR=VALUE...]: runs install.sh piped into sh, as
# the curl line does. MODE native runs this Mac's shell; MODE other runs
# an x86_64 shell under Rosetta. VAR=VALUE pairs override the defaults.
install() {
  LOG=$1; MODE=$2; DIR=$3; shift 3
  set -- env REX_FINGERS_URL="file://$WORK/rel" REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR="$DIR" "$@"
  if [ "$MODE" = other ]; then set -- "$@" arch -x86_64 /bin/sh; else set -- "$@" /bin/sh; fi
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
refused() { # refused LOG DIR [VAR=VALUE...]: install.sh exits non-zero
  R_LOG=$1; R_DIR=$2; shift 2
  ! install "$R_LOG" native "$R_DIR" "$@"
}

echo "== install, $NATIVE, this Mac's processor"
D=$WORK/native/rex-fingers
check "fresh install" install fresh native "$D"
check "fetched the $NATIVE tarball" grep -q "darwin-$NATIVE.tar.gz" "$WORK/fresh.log"
check "program runs, no quarantine" installed "$D" "$NATIVE_MACHO"
check "prints the setup lines" grep -q 'fingers.setup{' "$WORK/fresh.log"
check "prints a folder of its own as a literal path" grep -qF "local fingers_dir = \"$D\"" "$WORK/fresh.log"
check "prints no reload of its own" sh -c "! grep -q 'reloaded' '$WORK/fresh.log'"
check "writes a manifest of its files" grep -qx './lua/rex-fingers/init.lua' "$D/.rex-fingers-manifest"
check "install over an install" install again native "$D"
check "program runs after the second install" installed "$D" "$NATIVE_MACHO"
check "no work folder left" leftovers "$D"
check "no lock left" test ! -e "$D.install-lock"
touch "$D/.DS_Store" "$D/lua/.DS_Store"
check "install over an install Finder has shown" install finder native "$D"

echo "== the documented line, which fetches the script first"
# The README's line with the script's URL pointed at this tree's copy.
# shellcheck disable=SC2016 # the inner sh expands it
LINE='script=$(curl -fsSL "$SCRIPT_URL") && sh -c "$script"'
D=$WORK/line/rex-fingers
check "installs" env SCRIPT_URL="file://$REPO/scripts/install.sh" REX_FINGERS_URL="file://$WORK/rel" \
  REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR="$D" sh -c "$LINE"
check "program runs" installed "$D" "$NATIVE_MACHO"
D=$WORK/line-missing/rex-fingers
check "a failed fetch of the script fails the line" sh -c "! env SCRIPT_URL=file://$WORK/no-such-script.sh REX_FINGERS_DIR='$D' sh -c '$LINE'"
check "and installs nothing" test ! -e "$D"

echo "== install into the default folder, with HOME in \$TMPDIR"
H=$WORK/home
check "install with no REX_FINGERS_DIR" sh -c "env -u REX_FINGERS_DIR HOME='$H' REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=$TAG sh '$REPO/scripts/install.sh' >'$WORK/default.log' 2>&1"
check "lands in HOME/.config/rex/plugins/rex-fingers" installed "$H/.config/rex/plugins/rex-fingers" "$NATIVE_MACHO"
check "prints the folder as README.md does" grep -qF 'local fingers_dir = os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers"' "$WORK/default.log"
check "leaves the Rex config alone" test ! -e "$H/.config/rex/init.lua"

echo "== install of the latest release, through a local web server"
# GitHub answers /releases/latest with a redirect to /releases/tag/TAG;
# install.sh takes the tag from the end of that URL. The server does the
# same and serves the downloads from the release folder.
cat >"$WORK/serve.py" <<'EOF'
import functools, http.server, sys
root, tag = sys.argv[1], sys.argv[2]
class Handler(http.server.SimpleHTTPRequestHandler):
    def redirected(self):
        if self.path.rstrip("/").endswith("/releases/latest"):
            self.send_response(302)
            self.send_header("Location", "/releases/tag/" + tag)
            self.end_headers()
            return True
        if "/releases/tag/" in self.path:
            self.send_response(200)
            self.send_header("Content-Length", "0")
            self.end_headers()
            return True
        return False
    def do_HEAD(self):
        if not self.redirected(): super().do_HEAD()
    def do_GET(self):
        if not self.redirected(): super().do_GET()
    def log_message(self, *args): pass
server = http.server.HTTPServer(("127.0.0.1", 0), functools.partial(Handler, directory=root))
print(server.server_address[1], flush=True)
server.serve_forever()
EOF
# /usr/bin/python3 without the Command Line Tools opens an install dialog
# instead of running, so it counts only with them.
if command -v python3 >/dev/null 2>&1 &&
  { [ "$(command -v python3)" != /usr/bin/python3 ] || xcode-select -p >/dev/null 2>&1; }; then
  python3 "$WORK/serve.py" "$WORK/rel" "$TAG" >"$WORK/port" 2>"$WORK/serve.log" &
  SERVER=$!
  for _ in 1 2 3 4 5 6 7 8 9 10; do [ -s "$WORK/port" ] && break; sleep 0.3; done
  PORT=$(cat "$WORK/port")
  case $PORT in
    '' | *[!0-9]*)
      bad "the web server started (port '$PORT')"
      cat "$WORK/serve.log"
      ;;
    *)
      D=$WORK/latest/rex-fingers
      check "install with no REX_FINGERS_VERSION" install latest native "$D" \
        REX_FINGERS_URL="http://127.0.0.1:$PORT" REX_FINGERS_VERSION= NO_PROXY=127.0.0.1 no_proxy=127.0.0.1
      check "found $TAG as the latest" grep -q "rex-fingers-$TAG-darwin-$NATIVE.tar.gz" "$WORK/latest.log"
      check "program runs" installed "$D" "$NATIVE_MACHO"
      ;;
  esac
  kill "$SERVER" 2>/dev/null; SERVER=
else
  echo "   skip: no python3 to serve the redirect"
fi

echo "== install, $OTHER"
if [ "$NATIVE" = arm64 ] && arch -x86_64 /usr/bin/true 2>/dev/null; then
  D=$WORK/other/rex-fingers
  check "fresh install under Rosetta" install other other "$D"
  check "fetched the $OTHER tarball" grep -q "darwin-$OTHER.tar.gz" "$WORK/other.log"
  check "program runs, no quarantine" installed "$D" "$OTHER_MACHO"
elif [ "$NATIVE" = arm64 ]; then
  echo "   skip $OTHER: Rosetta is not installed"
else
  echo "   skip $OTHER: an Intel Mac cannot run it"
fi

echo "== refusals, each leaves the folder as it was"
# Each fixture has a built program, so only the check named refuses it.
fixture() { # fixture DIR: a folder with an executable bin/rex-fingers
  mkdir -p "$1/bin" && cp "$WORK/native/rex-fingers/bin/rex-fingers" "$1/bin/"
}
D=$WORK/clone/rex-fingers; fixture "$D"; mkdir "$D/.git"
check "refuses a git clone" refused clone "$D"
check "said it is a git clone" grep -q 'git clone or worktree' "$WORK/clone.log"
check "git clone untouched" test -d "$D/.git"
D=$WORK/worktree/rex-fingers; fixture "$D"; echo 'gitdir: /elsewhere' >"$D/.git"
check "refuses a worktree, whose .git is a file" refused worktree "$D"
check "said it is a git clone" grep -q 'git clone or worktree' "$WORK/worktree.log"
check "worktree untouched" grep -q gitdir "$D/.git"
D=$WORK/foreign/rex-fingers; fixture "$D"; echo keep >"$D/notes"
check "refuses a folder with a program and a file of its own" refused foreign "$D"
check "said a file is not the install's" grep -q 'holds files an install did not put there' "$WORK/foreign.log"
check "foreign folder untouched" grep -qx keep "$D/notes"
D=$WORK/added/rex-fingers; mkdir -p "$WORK/added"; cp -R "$WORK/native/rex-fingers" "$D"; echo keep >"$D/bin/notes"
check "refuses an install with a file added since" refused added "$D"
check "said a file is not the install's" grep -q 'holds files an install did not put there' "$WORK/added.log"
check "added file untouched" grep -qx keep "$D/bin/notes"
mkdir -p "$WORK/link"; ln -s "$WORK/native/rex-fingers" "$WORK/link/rex-fingers"
check "refuses a symbolic link" refused link "$WORK/link/rex-fingers"
check "said it is a link" grep -q 'symbolic link' "$WORK/link.log"
check "link untouched" test -L "$WORK/link/rex-fingers"
D=$WORK/locked/rex-fingers; mkdir -p "$WORK/locked"; cp -R "$WORK/native/rex-fingers" "$D"; mkdir "$D.install-lock"
check "refuses while another install holds the lock" refused locked "$D"
check "said another install runs" grep -q 'another install' "$WORK/locked.log"
check "left the other install's lock" test -d "$D.install-lock"
mkdir -p "$WORK/cwd"
check "refuses a relative REX_FINGERS_DIR" sh -c "cd '$WORK/cwd' && ! env REX_FINGERS_URL=file://$WORK/rel REX_FINGERS_VERSION=$TAG REX_FINGERS_DIR=plugins/rex-fingers sh '$REPO/scripts/install.sh'"
check "made nothing in the current folder" test ! -e "$WORK/cwd/plugins"
check "refuses a quote in REX_FINGERS_DIR" refused quote "$WORK/a\"b/rex-fingers"
check "refuses a backslash in REX_FINGERS_DIR" refused backslash "$WORK/a\\b/rex-fingers"
check "refuses a carriage return in REX_FINGERS_DIR" refused cr "$WORK/a$(printf '\r')b/rex-fingers"
check "refuses a line break in REX_FINGERS_DIR" refused lf "$WORK/a
b/rex-fingers"
check "made no folder for any of them" sh -c "! ls -d '$WORK'/a*b >/dev/null 2>&1"

echo "== an install from before the manifest"
D=$WORK/legacy/rex-fingers; mkdir -p "$WORK/legacy"
cp -R "$WORK/unpack-$NATIVE/rex-fingers" "$D"
check "has the release layout and no manifest" test ! -e "$D/.rex-fingers-manifest"
check "is replaced" install legacy native "$D"
check "now has a manifest" test -f "$D/.rex-fingers-manifest"

echo "== the old install comes back when the swap stops halfway"
# A stand-in for mv, first on the PATH, stops the second rename: it fails,
# or it sends TERM to the installer as a person pressing Ctrl-C would.
mkdir -p "$WORK/shim"
cat >"$WORK/shim/mv" <<'EOF'
#!/bin/sh
case $1 in
  */unpack/rex-fingers)
    [ "$SHIM" = term ] && kill -TERM "$PPID"
    exit 1
    ;;
esac
exec /bin/mv "$@"
EOF
chmod +x "$WORK/shim/mv"
for HOW in fail term; do
  D=$WORK/halfway-$HOW/rex-fingers; mkdir -p "$(dirname "$D")"
  cp -R "$WORK/native/rex-fingers" "$D"
  listing_before=$(cd "$D" && find . | LC_ALL=C sort)
  check "$HOW: the install fails" refused "halfway-$HOW" "$D" PATH="$WORK/shim:$PATH" SHIM=$HOW
  check "$HOW: the old install is back" installed "$D" "$NATIVE_MACHO"
  check "$HOW: with the same files" test "$(cd "$D" && find . | LC_ALL=C sort)" = "$listing_before"
  check "$HOW: no work folder left" leftovers "$D"
  check "$HOW: no lock left" test ! -e "$D.install-lock"
done

D=$WORK/native/rex-fingers
mkdir -p "$WORK/badrel/releases/download/$TAG"
cp "$REL"/* "$WORK/badrel/releases/download/$TAG/"
echo junk >>"$WORK/badrel/releases/download/$TAG/rex-fingers-$TAG-darwin-$NATIVE.tar.gz"
check "refuses a bad checksum on the $NATIVE tarball" refused badsum "$D" REX_FINGERS_URL="file://$WORK/badrel"
check "said why" grep -q "checksum of rex-fingers-$TAG-darwin-$NATIVE.tar.gz does not match" "$WORK/badsum.log"
check "old install still runs" installed "$D" "$NATIVE_MACHO"
check "no work folder left" leftovers "$D"
check "refuses a missing release" refused missing "$WORK/missing/rex-fingers" REX_FINGERS_VERSION=v0.0.1
check "refuses a malformed version" refused malformed "$WORK/missing/rex-fingers" REX_FINGERS_VERSION=latest
check "missing release left no folder" test ! -e "$WORK/missing/rex-fingers"

echo
echo "passed $PASS, failed $FAIL"
[ "$FAIL" = 0 ]
