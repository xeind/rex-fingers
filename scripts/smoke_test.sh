#!/bin/sh
# Smoke test without a Rex server. It checks formatting, static analysis,
# the build, every Go test under the race detector, and a short fuzz run of
# the screen parser and the pattern matcher. The live-Rex cases are in
# scripts/test_in_pane.sh.
#
#   sh scripts/smoke_test.sh              # fuzz 10 s per target
#   FUZZTIME=60s sh scripts/smoke_test.sh # fuzz longer
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
step "fuzz screen.Parse"     go test -run '^$' -fuzz '^FuzzParse$' -fuzztime "$FUZZTIME" ./internal/screen
step "fuzz patterns.Find"    go test -run '^$' -fuzz '^FuzzFind$' -fuzztime "$FUZZTIME" ./internal/patterns

echo
if [ "$FAIL" = 0 ]; then echo "smoke test passed"; else echo "smoke test failed: $FAIL step(s)"; fi
[ "$FAIL" = 0 ]
