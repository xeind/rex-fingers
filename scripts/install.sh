#!/bin/sh
# Install rex-fingers from a GitHub release, on a Mac:
#
#   curl -fsSL https://raw.githubusercontent.com/xeind/rex-fingers/main/scripts/install.sh | sh
#
# It fetches the release tarball for this Mac with curl, checks it against
# the release's SHA256SUMS, and puts the plugin and the program in
# ~/.config/rex/plugins/rex-fingers. A file curl fetches carries no
# quarantine attribute, so macOS runs the program; docs/HISTORY.md, "The
# prebuilt tarball", says why that matters. An install made by this script
# is replaced as a whole on the next run.
#
# It never edits the Rex config and never reloads it: it prints the lines
# to add and the commands to run.
#
# Environment:
#   REX_FINGERS_VERSION  a tag such as v0.5.0; default: the latest release
#   REX_FINGERS_DIR      the install folder; default:
#                        ~/.config/rex/plugins/rex-fingers
#   REX_FINGERS_URL      where the releases are; default:
#                        https://github.com/xeind/rex-fingers. A test
#                        sets a file:// folder with releases/download/TAG/
#
# Everything runs inside main, so a download cut short runs nothing.

main() {
  REPO_URL=${REX_FINGERS_URL:-https://github.com/xeind/rex-fingers}
  DIR=${REX_FINGERS_DIR:-$HOME/.config/rex/plugins/rex-fingers}
  VERSION=${REX_FINGERS_VERSION:-}

  if [ "$(uname -s)" != Darwin ]; then
    fail "the release has a program for macOS only. On $(uname -s), build it with Go 1.26:
  git clone --depth 1 $REPO_URL $DIR
  go build -C $DIR -o bin/rex-fingers ."
  fi
  case $(uname -m) in
    arm64) ARCH=arm64 ;;
    x86_64) ARCH=amd64 ;;
    *) fail "no release program for the processor $(uname -m)" ;;
  esac
  for COMMAND in curl tar shasum; do
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

  if [ -d "$DIR/.git" ]; then
    fail "$DIR is a git clone. Update it with git, as README.md says under Update,
or move it away and run this script again."
  fi
  if [ -e "$DIR" ] && [ ! -x "$DIR/bin/rex-fingers" ]; then
    fail "$DIR exists and holds no rex-fingers install; move it away and run this script again"
  fi

  # The new install is unpacked beside the old one, so the swap at the end
  # is two renames on one file system.
  PARENT=$(dirname "$DIR")
  mkdir -p "$PARENT" || fail "could not create $PARENT"
  WORK=$(mktemp -d "$PARENT/.rex-fingers-install.XXXXXX") || fail "could not create a folder in $PARENT"
  trap 'rm -rf "$WORK"' EXIT
  trap 'exit 1' INT TERM

  TARBALL=rex-fingers-$VERSION-darwin-$ARCH.tar.gz
  BASE=$REPO_URL/releases/download/$VERSION
  echo "rex-fingers: fetching $TARBALL"
  curl -fsSL -o "$WORK/$TARBALL" "$BASE/$TARBALL" || fail "could not fetch $BASE/$TARBALL"
  curl -fsSL -o "$WORK/SHA256SUMS" "$BASE/SHA256SUMS" || fail "could not fetch $BASE/SHA256SUMS"

  WANT=$(awk -v f="$TARBALL" '$2 == f { print $1 }' "$WORK/SHA256SUMS")
  GOT=$(shasum -a 256 "$WORK/$TARBALL" | awk '{ print $1 }')
  [ -n "$WANT" ] || fail "SHA256SUMS has no line for $TARBALL"
  [ "$WANT" = "$GOT" ] || fail "the checksum of $TARBALL does not match SHA256SUMS"

  tar -xzf "$WORK/$TARBALL" -C "$WORK" || fail "could not unpack $TARBALL"
  NEW=$WORK/rex-fingers
  REPORTED=$("$NEW/bin/rex-fingers" version 2>&1) || fail "the program does not run: $REPORTED"
  [ "$REPORTED" = "$VERSION" ] || fail "the program reports $REPORTED, not $VERSION"

  if [ -e "$DIR" ]; then
    mv "$DIR" "$WORK/previous" || fail "could not move the old install aside"
  fi
  mv "$NEW" "$DIR" || {
    [ -e "$WORK/previous" ] && mv "$WORK/previous" "$DIR"
    fail "could not move the new install into $DIR"
  }

  # The default folder is printed as README.md writes it.
  if [ "$DIR" = "$HOME/.config/rex/plugins/rex-fingers" ]; then
    DIR_LUA='os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers"'
  else
    DIR_LUA="\"$DIR\""
  fi

  cat <<EOF

rex-fingers $VERSION is in $DIR

If your Rex config does not load it yet, add to ~/.config/rex/init.lua:

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

fail() {
  echo "rex-fingers install: $1" >&2
  exit 1
}

main "$@"
