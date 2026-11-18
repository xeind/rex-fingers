#!/bin/sh
# Smoke test without a Rex server. It checks formatting, static analysis,
# the build, every Go test under the race detector, and a short fuzz run of
# the screen parser and the pattern matcher. The live-Rex cases are in
# scripts/test_in_pane.sh.
#
#   sh scripts/smoke_test.sh              # fuzz 10 s per target
#   FUZZTIME=60s sh scripts/smoke_test.sh # fuzz longer
#   FUZZTIME=0 sh scripts/smoke_test.sh   # no fuzz; CI runs it this way
#
# CI skips the fuzz because a timed fuzz run can fail with "context
# deadline exceeded" when an input is still running as the time ends. That
# is no finding, and it turned a docs-only commit red. The fuzz targets
# still run as plain tests over their seed inputs in the go test step.
cd "$(dirname "$0")/.." || exit 1
FUZZTIME=${FUZZTIME:-10s}
FAIL=0

step() { # step NAME COMMAND...
  NAME=$1; shift
  printf '== %s\n' "$NAME"
  if "$@"; then echo "   ok"; else echo "   FAIL: $NAME"; FAIL=$((FAIL + 1)); fi
}

formatted() { [ -z "$(gofmt -l .)" ] || { gofmt -l .; return 1; }; }

step "gofmt"                 formatted
step "go vet"                go vet ./...
step "go build"              go build -o /dev/null .
step "go test -race"         go test -race -count=1 ./...
if [ "$FUZZTIME" = 0 ]; then
  echo "== fuzz: skipped, FUZZTIME is 0"
else
  step "fuzz screen.Parse"     go test -run '^$' -fuzz '^FuzzParse$' -fuzztime "$FUZZTIME" ./internal/screen
  step "fuzz patterns.Find"    go test -run '^$' -fuzz '^FuzzFind$' -fuzztime "$FUZZTIME" ./internal/patterns
fi

echo
if [ "$FAIL" = 0 ]; then echo "smoke test passed"; else echo "smoke test failed: $FAIL step(s)"; fi
[ "$FAIL" = 0 ]
