#!/bin/sh
# Install rex-fingers from a GitHub release, on a Mac:
#
#   sh -c 'script=$(curl -fsSL https://raw.githubusercontent.com/xeind/rex-fingers/main/scripts/install.sh) && sh -c "$script"'
#
# The outer sh fetches the whole script before it runs any of it, and a
# failed fetch ends the line with curl's error and status; with
# "curl ... | sh", sh would run an empty script and report success.
#
# It fetches the release tarball for this Mac with curl, checks it against
# the release's SHA256SUMS, and puts the plugin and the program in
# ~/.config/rex/plugins/rex-fingers. A file curl fetches carries no
# quarantine attribute, so macOS runs the program; docs/HISTORY.md, "The
# prebuilt tarball", says why that matters.
#
# It replaces a folder only when the folder holds exactly what an earlier
# install put there: the files listed in its .rex-fingers-manifest, or,
# for an install from before the manifest, the release layout below. A
# git clone or worktree, a symbolic link, or a file someone added makes it
# stop and change nothing.
#
# It never edits the Rex config and never reloads it: it prints the lines
# to add and the commands to run.
#
# Environment:
#   REX_FINGERS_VERSION  a tag such as v0.5.0; default: the latest release
#   REX_FINGERS_DIR      the install folder, an absolute path; default:
#                        ~/.config/rex/plugins/rex-fingers
#   REX_FINGERS_URL      where the releases are; default:
#                        https://github.com/xeind/rex-fingers. A test
#                        sets a file:// folder with releases/download/TAG/

# The files of a release tarball up to v0.5.0, which wrote no manifest.
RELEASE_LAYOUT='./LICENSE
./README.md
./bin
./bin/rex-fingers
./lua
./lua/rex-fingers
./lua/rex-fingers/init.lua'
MANIFEST=.rex-fingers-manifest

# State the cleanup reads. BACKUP is the old install while it is moved
# aside; SWAPPED is set once the new install is in place and checked.
WORK=
LOCK=
BACKUP=
SWAPPED=
KEEP=

main() {
  REPO_URL=${REX_FINGERS_URL:-https://github.com/xeind/rex-fingers}
  DIR=${REX_FINGERS_DIR:-$HOME/.config/rex/plugins/rex-fingers}
  VERSION=${REX_FINGERS_VERSION:-}

  # The folder goes into the printed Lua as a quoted string, and the
  # config needs an absolute path.
  case $DIR in
    /*) ;;
    *) fail "REX_FINGERS_DIR must be an absolute path, not $DIR" ;;
  esac
  CR=$(printf '\r')
  case $DIR in
    *'"'* | *\\* | *"$CR"* | *"
"*) fail "REX_FINGERS_DIR must not hold a quote, a backslash, or a line break" ;;
  esac

  if [ "$(uname -s)" != Darwin ]; then
    fail "the release has a program for macOS only. On $(uname -s), build it from source:
  $REPO_URL#build-from-source"
  fi
  case $(uname -m) in
    arm64) ARCH=arm64 ;;
    x86_64) ARCH=amd64 ;;
    *) fail "no release program for the processor $(uname -m)" ;;
  esac
  for COMMAND in curl tar shasum find sort; do
    command -v "$COMMAND" >/dev/null 2>&1 || fail "$COMMAND is missing"
  done

  if [ -z "$VERSION" ]; then
    # The latest release redirects to its tag page; the tag ends the URL.
    LATEST=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$REPO_URL/releases/latest") ||
      fail "could not reach $REPO_URL/releases/latest"
    VERSION=${LATEST##*/}
  fi
  echo "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' ||
    fail "no release found (got '$VERSION'); set REX_FINGERS_VERSION to a tag such as v0.5.0"

  check_destination

  # The new install is unpacked beside the old one, so the swap at the end
  # is two renames on one file system. The lock keeps two runs of this
  # script from swapping the same folder at once.
  PARENT=$(dirname "$DIR")
  mkdir -p "$PARENT" || fail "could not create $PARENT"
  trap cleanup EXIT
  trap 'exit 1' HUP INT TERM
  mkdir "$DIR.install-lock" 2>/dev/null ||
    fail "another install into $DIR is running. If none is, remove $DIR.install-lock"
  LOCK=$DIR.install-lock
  WORK=$(mktemp -d "$PARENT/.rex-fingers-install.XXXXXX") || fail "could not create a folder in $PARENT"

  TARBALL=rex-fingers-$VERSION-darwin-$ARCH.tar.gz
  BASE=$REPO_URL/releases/download/$VERSION
  echo "rex-fingers: fetching $TARBALL"
  curl -fsSL -o "$WORK/$TARBALL" "$BASE/$TARBALL" || fail "could not fetch $BASE/$TARBALL"
  curl -fsSL -o "$WORK/SHA256SUMS" "$BASE/SHA256SUMS" || fail "could not fetch $BASE/SHA256SUMS"

  WANT=$(awk -v f="$TARBALL" '$2 == f { print $1 }' "$WORK/SHA256SUMS")
  GOT=$(shasum -a 256 "$WORK/$TARBALL" | awk '{ print $1 }')
  [ -n "$WANT" ] || fail "SHA256SUMS has no line for $TARBALL"
  [ "$WANT" = "$GOT" ] || fail "the checksum of $TARBALL does not match SHA256SUMS"

  { mkdir "$WORK/unpack" && tar -xzf "$WORK/$TARBALL" -C "$WORK/unpack"; } || fail "could not unpack $TARBALL"
  NEW=$WORK/unpack/rex-fingers
  [ -f "$NEW/lua/rex-fingers/init.lua" ] && [ -r "$NEW/lua/rex-fingers/init.lua" ] ||
    fail "$TARBALL holds no lua/rex-fingers/init.lua"
  REPORTED=$("$NEW/bin/rex-fingers" version 2>&1) || fail "the program does not run: $REPORTED"
  [ "$REPORTED" = "$VERSION" ] || fail "the program reports $REPORTED, not $VERSION"
  { listing "$NEW" >"$WORK/$MANIFEST" && mv "$WORK/$MANIFEST" "$NEW/$MANIFEST"; } ||
    fail "could not write the manifest"

  # The folder may have changed during the download.
  check_destination
  if [ -e "$DIR" ]; then
    mv "$DIR" "$WORK/previous" || fail "could not move the old install aside"
    BACKUP=$WORK/previous
  fi
  [ ! -e "$DIR" ] || fail "something created $DIR during the install"
  mv "$NEW" "$DIR" || fail "could not move the new install into $DIR"
  # A folder made at $DIR just before the rename would have taken the new
  # install inside it; the check catches that.
  [ "$("$DIR/bin/rex-fingers" version 2>/dev/null)" = "$VERSION" ] && [ -f "$DIR/$MANIFEST" ] ||
    fail "$DIR does not hold the new install after the rename"
  SWAPPED=1

  # The default folder is printed as README.md writes it.
  if [ "$DIR" = "$HOME/.config/rex/plugins/rex-fingers" ]; then
    DIR_LUA='os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers"'
  else
    DIR_LUA="\"$DIR\""
  fi

  cat <<EOF

rex-fingers $VERSION is in $DIR

If your Rex config does not load it yet, add these lines to it. It is the
file 'rex config check' prints after path:, ~/.config/rex/init.lua unless
you set XDG_CONFIG_HOME or REX_CONFIG.

  local fingers_dir = $DIR_LUA
  local fingers = dofile(fingers_dir .. "/lua/rex-fingers/init.lua")
  fingers.setup{
    binary = fingers_dir .. "/bin/rex-fingers",
  }

Then run:

  rex config check
  rex config reload
  rex keymap | grep rex_fingers   # alt+f and alt+shift+f
EOF
}

# listing DIR: every path under DIR, sorted, without .DS_Store files and
# the manifest. Finder adds .DS_Store to a folder it shows.
listing() {
  (cd "$1" && find . -mindepth 1 ! -name .DS_Store ! -name "$MANIFEST" | LC_ALL=C sort)
}

# check_destination: return when $DIR is absent or holds exactly an
# earlier install; stop otherwise.
check_destination() {
  if [ -e "$DIR/.git" ] || [ -L "$DIR/.git" ]; then
    fail "$DIR is a git clone or worktree. Update it with git, as README.md says under
Build from source, or move it away and run this script again."
  fi
  if [ -L "$DIR" ]; then
    fail "$DIR is a symbolic link; this script replaces only a folder it made"
  fi
  [ -e "$DIR" ] || return 0
  [ -d "$DIR" ] || fail "$DIR is not a folder; move it away and run this script again"
  if [ -f "$DIR/$MANIFEST" ]; then
    EXPECTED=$(cat "$DIR/$MANIFEST") || fail "could not read $DIR/$MANIFEST"
  else
    EXPECTED=$RELEASE_LAYOUT
  fi
  FOUND=$(listing "$DIR") || fail "could not list $DIR"
  [ "$FOUND" = "$EXPECTED" ] ||
    fail "$DIR holds files an install did not put there; move it away and run this script again"
}

# cleanup runs on every exit. Before the swap is done it puts the old
# install back; when it cannot, it keeps the work folder and says where.
cleanup() {
  if [ -n "$BACKUP" ] && [ -z "$SWAPPED" ] && [ -e "$BACKUP" ]; then
    if [ -e "$DIR" ] || ! mv "$BACKUP" "$DIR"; then
      KEEP=1
      echo "rex-fingers install: the old install is kept at $BACKUP" >&2
    fi
  fi
  if [ -n "$WORK" ] && [ -z "$KEEP" ]; then rm -rf "$WORK"; fi
  if [ -n "$LOCK" ]; then rmdir "$LOCK" 2>/dev/null; fi
}

fail() {
  echo "rex-fingers install: $1" >&2
  exit 1
}

main "$@"
