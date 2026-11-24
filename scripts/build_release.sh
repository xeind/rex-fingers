#!/bin/sh
# Build the release tarballs for a tag. Each tarball holds the program and
# the Lua plugin, so the two cannot drift apart. It publishes nothing: the
# tag stays local and no file leaves the machine.
#
#   git tag -a v0.5.0 -m "rex-fingers v0.5.0"
#   sh scripts/build_release.sh v0.5.0
#
# Writes dist/rex-fingers-TAG-OS-ARCH.tar.gz per platform, dist/SHA256SUMS,
# and dist/NOTES.md. scripts/install.sh fetches these names; keep them in
# step. The notes are docs/releases/TAG.md when that file exists; write it
# before you tag. Without it they are the commit subjects since the
# previous tag, to edit before publishing. GitHub shows each line break of
# the notes, so keep a paragraph on one line.
#
# The build runs in a temporary git worktree of the tag, so a change or an
# untracked file in this tree does not reach the tarball. Go stamps the
# version from git, and an untracked file alone stamps TAG+dirty.
cd "$(dirname "$0")/.." || exit 1
REPO=$(pwd)
TAG=${1:-}
PLATFORMS="darwin/arm64 darwin/amd64"

fail() { echo "build_release: $1" >&2; exit 1; }

echo "$TAG" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || fail "usage: sh scripts/build_release.sh vX.Y.Z"
git rev-parse -q --verify "refs/tags/$TAG" >/dev/null || fail "no tag $TAG; run git tag first"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/rex-fingers-release.XXXXXX") || fail "mktemp failed"
SOURCE="$WORK/source"
STAGE="$WORK/stage"
cleanup() {
  git -C "$REPO" worktree remove --force "$SOURCE" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT
trap 'exit 1' INT TERM

git worktree add --detach "$SOURCE" "$TAG" >/dev/null 2>&1 || fail "git worktree add of $TAG failed"

FUZZTIME=0 sh "$SOURCE/scripts/smoke_test.sh" || fail "the smoke test failed"

# Everything is made in $OUT first; dist/ is replaced only when every
# step has worked, so a failed build leaves the last dist/ as it was.
OUT="$WORK/dist"
mkdir -p "$OUT"
for PLATFORM in $PLATFORMS; do
  OS=${PLATFORM%/*}; ARCH=${PLATFORM#*/}
  rm -rf "$STAGE"
  mkdir -p "$STAGE/rex-fingers/bin"
  (cd "$SOURCE" && CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -trimpath -ldflags "-s -w" \
    -o "$STAGE/rex-fingers/bin/rex-fingers" .) || fail "the build for $PLATFORM failed"
  STAMPED=$(go version -m "$STAGE/rex-fingers/bin/rex-fingers" | awk '$1 == "mod" { print $3 }')
  [ "$STAMPED" = "$TAG" ] || fail "the $PLATFORM binary reports $STAMPED, not $TAG"
  cp -R "$SOURCE/lua" "$SOURCE/README.md" "$SOURCE/LICENSE" "$STAGE/rex-fingers/" ||
    fail "the copy of the plugin for $PLATFORM failed"
  [ -f "$STAGE/rex-fingers/lua/rex-fingers/init.lua" ] ||
    fail "the $PLATFORM tarball would hold no lua/rex-fingers/init.lua"
  # COPYFILE_DISABLE keeps macOS tar from adding ._ files for extended
  # attributes. The owner flags keep the builder's user name out of the
  # tarball.
  COPYFILE_DISABLE=1 tar --uid 0 --gid 0 --uname '' --gname '' \
    -czf "$OUT/rex-fingers-$TAG-$OS-$ARCH.tar.gz" -C "$STAGE" rex-fingers \
    || fail "tar for $PLATFORM failed"
done

(cd "$OUT" && shasum -a 256 rex-fingers-*.tar.gz > SHA256SUMS) || fail "the checksums failed"
[ "$(wc -l < "$OUT/SHA256SUMS" | tr -d ' ')" = "$(echo "$PLATFORMS" | wc -w | tr -d ' ')" ] ||
  fail "SHA256SUMS does not have one line per tarball"

if [ -f "$SOURCE/docs/releases/$TAG.md" ]; then
  cp "$SOURCE/docs/releases/$TAG.md" "$OUT/NOTES.md" || fail "the copy of the notes failed"
else
  PREVIOUS=$(git describe --tags --abbrev=0 "$TAG^" 2>/dev/null)
  git log --format='- %s' ${PREVIOUS:+"$PREVIOUS.."}"$TAG" > "$OUT/NOTES.md" || fail "the notes failed"
fi
[ -s "$OUT/NOTES.md" ] || fail "the notes are empty"

{ rm -rf "$REPO/dist" && mv "$OUT" "$REPO/dist"; } || fail "could not move the release into dist/"

echo
ls -l "$REPO/dist"
echo "built $TAG; nothing is published"
