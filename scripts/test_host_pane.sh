#!/bin/sh
# Hints on a pane of a Rex host, without a host: a second Rex server on
# this machine, with its own state directory, has its own host ID, so its
# panes count as a host's. Its config delays every Rex call, as a host
# 280 ms away does. Nothing reaches the user's server, panes, or
# clipboard: the copy goes to a file.
#
# Needs rex, jq, perl, and bin/rex-fingers. About 3 minutes.
# Usage: sh scripts/test_host_pane.sh [filter]
set -u
root=$(cd "$(dirname "$0")/.." && pwd)
[ -x "$root/bin/rex-fingers" ] || { echo "build first: go build -o bin/rex-fingers ."; exit 1; }
filter=${1:-}
dir=$(mktemp -d /tmp/rex-fingers-host.XXXXXX)
sock="unix://$dir/s.sock"
unset REX_SESSION REX_BLOCK REX_SERVER REX_CLIENT

cat > "$dir/init.lua" <<EOF
-- The delay in seconds before every Rex call is the content of the file
-- latency; rex.exec is the one wait a config action has.
local function delay()
  local f = io.open("$dir/latency")
  if not f then return end
  local s = f:read("*l")
  f:close()
  if tonumber(s) and tonumber(s) > 0 then rex.exec({ "sleep", s }) end
end
local function slow(fn)
  return function(...)
    delay()
    return fn(...)
  end
end
rex.call = slow(rex.call)
rex.block.call = slow(rex.block.call)
for _, name in ipairs({ "view", "new_layer", "close_layer", "focus_block" }) do
  rex.session[name] = slow(rex.session[name])
end
local fingers = dofile("$root/lua/rex-fingers/init.lua")
fingers.setup{
  binary = "$root/bin/rex-fingers",
  follow_scroll = true,
  flash = true,
  ctrl_action = ":reveal:",
  copy_command = "cat > $dir/copied",
}
EOF

T() { rex -S "$sock" --autostart=false "$@"; }
REX_CONFIG="$dir/init.lua" rex server run --listen "$sock" --state-dir "$dir/state" --restore=false > "$dir/server.log" 2>&1 &
server=$!
trap 'T server stop >/dev/null 2>&1; kill $server 2>/dev/null; rm -rf "$dir"' EXIT
i=0
until T server status >/dev/null 2>&1; do
  i=$((i + 1)); [ $i -gt 50 ] && { echo "the test server did not start"; cat "$dir/server.log"; exit 1; }
  sleep 0.1
done

passed=0 failed=0
ok() { passed=$((passed + 1)); echo "  ok   $1"; }
fail() { failed=$((failed + 1)); echo "  FAIL $1"; }
expect() { if [ "$2" = "$3" ]; then ok "$1"; else fail "$1: got '$2', want '$3'"; fi; }
now() { perl -MTime::HiRes=time -e 'printf "%d", time'; }

blocks() { T api call -s "$S" session.list_blocks '{}' | jq -r '.blocks[] | "\(.label // "-") \(.block_id)"'; }
reader() { T api call -s "$S" session.view '{}' | jq -r '[.windows[0].layers[] | select(.kind!="tiled") | .blocks[] | select(.label=="fingers-reader") | .block_id][0] // empty'; }
focused() { T api call -s "$S" session.view '{}' | jq -r '.windows[0].focused_block_id'; }
screen() { T capture -s "$S" -b "${1:-$B}" --format=text --trim; }
vt() { T block call -s "$S" -b "$B" format '{"format":"vt"}' | jq -j .content; }
# key BYTES types into the reader, as the app does while it has the focus.
key() { T block call -s "$S" -b "$(reader)" write "$(jq -cn --arg d "$1" '{data:$d}')" >/dev/null; }
trigger() { T --timeout 60s do rex_fingers -s "$S" -b "$@" > "$dir/trigger.out" 2>&1; }
result() { tr -d '\n' < "$dir/trigger.out" | cut -c1-90; }
wait_reader() { i=0; until [ -n "$(reader)" ]; do i=$((i + 1)); [ $i -gt 80 ] && return 1; sleep 0.1; done; }
wait_hints() { i=0; until screen | grep -q 'atmp/alpha'; do i=$((i + 1)); [ $i -gt 120 ] && return 1; sleep 0.1; done; }

# begin NAME LATENCY: a session with one pane that shows two paths.
begin() {
  name=$1
  case "$name" in *"$filter"*) ;; *) return 1 ;; esac
  echo "== $name"
  echo "$2" > "$dir/latency"
  rm -f "$dir/copied"
  S=$(T new "fingers-host-test" --json | jq -r .session_id)
  B=$(T api call -s "$S" session.view '{}' | jq -r '.windows[0].layers[0].blocks[0].block_id')
  sleep 1.5
  T send -s "$S" -b "$B" "clear; printf '/tmp/%s\n' alpha beta"
  T send-key -s "$S" -b "$B" enter
  sleep 1
  vt > "$dir/vt.before"
}
# clean: what every run must leave. The pane's own files are the frame
# file and the program's run file.
clean() {
  expect "no reader or writer left" "$(blocks | grep -c 'fingers-')" 0
  expect "focus on the pane" "$(focused)" "$B"
  id=$(printf %s "$B" | tr -cd '[:alnum:]')
  expect "no file of the run left" "$(find "${TMPDIR:-/tmp}" -maxdepth 1 -name "rex-fingers-*$id*" 2>/dev/null | wc -l | tr -d ' ')" 0
}
unchanged() { vt > "$dir/vt.after"; if cmp -s "$dir/vt.before" "$dir/vt.after"; then ok "pane as before"; else fail "pane differs from before"; fi; }
finish() { T kill "$S" >/dev/null; }

if begin "pick" 0.28; then
  trigger "$B" &
  run=$!
  wait_hints && wait_reader
  expect "the reader has the focus" "$(focused)" "$(reader)"
  key a; wait $run
  expect "copied" "$(cat "$dir/copied" 2>/dev/null)" /tmp/alpha
  clean; unchanged; finish
fi

if begin "early key" 0.28; then
  # Typed as soon as the reader is up, before the hints are drawn.
  trigger "$B" &
  run=$!
  wait_reader; key s; wait $run
  expect "copied" "$(cat "$dir/copied" 2>/dev/null)" /tmp/beta
  clean; unchanged; finish
fi

if begin "shift pastes" 0.28; then
  trigger "$B" &
  run=$!
  wait_hints && wait_reader; key S; wait $run; sleep 0.5
  case "$(screen | tail -1)" in *"/tmp/beta"*) ok "pasted at the prompt" ;; *) fail "not pasted: $(screen | tail -1)" ;; esac
  clean; finish
fi

if begin "ctrl copies a path" 0.28; then
  # A host's path is not opened here; a URL would open the browser.
  trigger "$B" &
  run=$!
  wait_hints && wait_reader; key "$(printf '\001')"; wait $run
  expect "copied" "$(cat "$dir/copied" 2>/dev/null)" /tmp/alpha
  clean; unchanged; finish
fi

if begin "escape" 0.28; then
  trigger "$B" &
  run=$!
  wait_hints && wait_reader; key "$(printf '\033')"; wait $run
  expect "nothing copied" "$(cat "$dir/copied" 2>/dev/null)" ""
  clean; unchanged; finish
fi

if begin "window scope" 0.28; then
  B2=$(T split -s "$S" -b "$B" --split=below --focus=false --json | jq -r '.block_id // .block_ids[0]')
  sleep 1.5
  T send -s "$S" -b "$B2" "clear; printf '/tmp/%s\n' gamma"; T send-key -s "$S" -b "$B2" enter; sleep 1
  vt > "$dir/vt.before"
  trigger "$B" scope=window &
  run=$!
  wait_hints && wait_reader
  i=0; L=
  until [ -n "$L" ] || [ $i -gt 80 ]; do L=$(screen "$B2" | grep -o '[a-z]tmp/gamma' | head -1 | cut -c1); i=$((i + 1)); sleep 0.1; done
  key "$L"; wait $run
  expect "the reader is not hinted" "$(jq -r .panes "$dir/trigger.out")" 2
  expect "copied from the second pane" "$(cat "$dir/copied" 2>/dev/null)" /tmp/gamma
  expect "second pane restored" "$(screen "$B2" | grep -c '^/tmp/gamma')" 1
  clean; unchanged; finish
fi

if begin "run limit" 0.28; then
  t=$(now); trigger "$B"; took=$(($(now) - t))
  if [ $took -ge 14 ] && [ $took -le 20 ]; then ok "ended by itself after $took s"; else fail "took $took s, want about 16"; fi
  clean; unchanged; finish
fi

if begin "slow host" 0.9; then
  # Three times the delay: the run must still end before Rex's 30 s.
  t=$(now); trigger "$B"; took=$(($(now) - t))
  if [ $took -le 25 ]; then ok "ended after $took s"; else fail "took $took s"; fi
  case "$(result)" in *Error*) fail "trigger: $(result)" ;; *) ok "no error" ;; esac
  clean; unchanged; finish
fi

if begin "queued triggers" 0.28; then
  # The app queues a trigger pressed during a run and sends it after the
  # run, with the pane at first and then the reader as its block. Here
  # ten arrive at once; nine wait and must start nothing.
  trigger "$B" &
  run=$!
  others=
  for n in 1 2 3 4 5 6 7 8 9; do
    T --timeout 60s do rex_fingers -s "$S" -b "$B" > "$dir/queued.$n" 2>&1 &
    others="$others $!"
  done
  wait_hints && wait_reader; key "$(printf '\033')"; wait $run $others
  sleep 1
  expect "runs started by ten triggers" "$(cat "$dir"/queued.* "$dir/trigger.out" | grep -c hints)" 1
  expect "errors" "$(cat "$dir"/queued.* "$dir/trigger.out" | grep -c Error)" 0
  clean; unchanged; finish
fi

if begin "action killed" 0.28; then
  # The caller goes away after 5 s, as when the app restarts: Rex stops
  # the action and runs no cleanup. The focus is on the reader, so the
  # next trigger arrives there; it must run on the pane.
  T --timeout 5s do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1
  sleep 1
  R=$(reader)
  expect "the dead run left its reader with the focus" "$(focused)" "$R"
  trigger "$R" &
  run=$!
  sleep 3; wait_reader
  if [ "$(reader)" != "$R" ]; then ok "a new reader"; else fail "the old reader is still up"; fi
  key s; wait $run
  expect "copied" "$(cat "$dir/copied" 2>/dev/null)" /tmp/beta
  clean; unchanged; finish
fi

if begin "reader ends itself" 0.28; then
  # Nobody presses the key again after the action died.
  T --timeout 5s do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1
  sleep 22
  expect "reader gone after 20 s" "$(reader)" ""
  expect "focus back on the pane" "$(focused)" "$B"
  finish
fi

if begin "reload during a run" 0.28; then
  # The Lua record is lost. The trigger on the old reader only closes;
  # the one on the pane starts again from the program's run file.
  trigger "$B" &
  run=$!
  wait_hints && wait_reader; R=$(reader)
  T config reload >/dev/null 2>&1; wait $run
  trigger "$R"
  expect "trigger on the old reader" "$(result)" '{"closed":1}'
  expect "nothing left" "$(blocks | grep -c 'fingers-')" 0
  expect "focus on the pane" "$(focused)" "$B"
  sleep 1
  trigger "$B" &
  run=$!
  sleep 3; wait_reader; key "$(printf '\033')"; wait $run
  clean; unchanged; finish
fi

echo
echo "passed $passed, failed $failed"
[ "$failed" = 0 ]
