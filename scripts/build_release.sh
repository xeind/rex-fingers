#!/bin/sh
# Build the release tarballs for a tag. Each tarball holds the program and
# the Lua plugin, so the two cannot drift apart. It publishes nothing: the
# tag stays local and no file leaves the machine.
#
#   git tag -a v0.3.0 -m "rex-fingers v0.3.0"
#   sh scripts/build_release.sh v0.3.0
#
# Writes dist/rex-fingers-TAG-OS-ARCH.tar.gz per platform, dist/SHA256SUMS,
# and dist/NOTES.md. The notes are docs/releases/TAG.md when that file
# exists; write it before you tag. Without it they are the commit subjects
# since the previous tag, to edit before publishing. GitHub shows each line
# break of the notes, so keep a paragraph on one line.
#
# Go stamps the version from git, so HEAD must carry the tag and the tree
# must be clean; an untracked file is enough for Go to stamp TAG+dirty.
cd "$(dirname "$0")/.." || exit 1
TAG=${1:-}
PLATFORMS="darwin/arm64 darwin/amd64"

fail() { echo "build_release: $1" >&2; exit 1; }

echo "$TAG" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' || fail "usage: sh scripts/build_release.sh vX.Y.Z"
[ -z "$(git status --porcelain)" ] || fail "the tree has changes or untracked files; commit or move them first"
git tag --points-at HEAD | grep -qx "$TAG" || fail "HEAD does not carry the tag $TAG; run git tag first"

sh scripts/smoke_test.sh || fail "the smoke test failed"

rm -rf dist stage
mkdir -p dist
for PLATFORM in $PLATFORMS; do
  OS=${PLATFORM%/*}; ARCH=${PLATFORM#*/}
  rm -rf stage
  mkdir -p stage/rex-fingers/bin
  CGO_ENABLED=0 GOOS=$OS GOARCH=$ARCH go build -trimpath -ldflags "-s -w" \
    -o stage/rex-fingers/bin/rex-fingers . || fail "the build for $PLATFORM failed"
  STAMPED=$(go version -m stage/rex-fingers/bin/rex-fingers | awk '$1 == "mod" { print $3 }')
  [ "$STAMPED" = "$TAG" ] || fail "the $PLATFORM binary reports $STAMPED, not $TAG"
  cp -R lua README.md LICENSE stage/rex-fingers/
  # COPYFILE_DISABLE keeps macOS tar from adding ._ files for extended
  # attributes. The owner flags keep the builder's user name out of the
  # tarball.
  COPYFILE_DISABLE=1 tar --uid 0 --gid 0 --uname '' --gname '' \
    -czf "dist/rex-fingers-$TAG-$OS-$ARCH.tar.gz" -C stage rex-fingers \
    || fail "tar for $PLATFORM failed"
done
rm -rf stage

(cd dist && shasum -a 256 ./*.tar.gz | sed 's|  \./|  |' > SHA256SUMS) || fail "the checksums failed"

if [ -f "docs/releases/$TAG.md" ]; then
  cp "docs/releases/$TAG.md" dist/NOTES.md
else
  PREVIOUS=$(git describe --tags --abbrev=0 "$TAG^" 2>/dev/null)
  git log --format='- %s' ${PREVIOUS:+"$PREVIOUS.."}"$TAG" > dist/NOTES.md
fi

echo
ls -l dist
echo "built $TAG; nothing is published"
