#!/bin/sh
# In-pane lifecycle test, headless. Each case opens a fresh Rex session,
# drives rex-fingers through the CLI the way the Lua plugin does, and
# checks that the pane is back as it was: same text, autowrap and cursor
# modes set, no capture or run file left. Most cases call press and restore
# as the Lua keys would. The "keys" case sends real key events through
# client.key, so the plugin must be loaded in the running config.
#
#   sh scripts/test_in_pane.sh            # all cases
#   sh scripts/test_in_pane.sh output     # cases whose name contains "output"
#
# Needs bin/rex-fingers (go build -o bin/rex-fingers .), jq, pbcopy.
cd "$(dirname "$0")/.." || exit 1
BIN=./bin/rex-fingers
FILTER=${1:-}
PASS=0; FAIL=0

# --- session helpers --------------------------------------------------
open_session() {
  S=$(rex new fingers-test --json | jq -r .session_id); sleep 1.5
  B=$(rex api call -s "$S" session.view '{}' | jq -r '.windows[0].layers[0].blocks[0].block_id')
  P=$(rex block call -s "$S" -b "$B" process | jq -r .foreground.pid)
  T=$(ps -o tty= -p "$P" | tr -d ' ')
  SZ=$(rex block call -s "$S" -b "$B" size)
  R=$(echo "$SZ" | jq -r .rows); C=$(echo "$SZ" | jq -r .columns)
  RUN="${TMPDIR:-/tmp}/rex-fingers-$T.run.json"
  common="--rows $R --cols $C"
  # A killed session's tty number is reused; a run file left by a failed
  # case must not leak into the next.
  rm -f "$RUN"
}
N=0
close_session() { rex kill "$S" >/dev/null 2>&1; }
# type runs a shell line in the pane and waits for it.
type_line() { rex send -s "$S" -b "$B" "$1" && rex send-key -s "$S" -b "$B" enter; sleep "${2:-0.8}"; }
# capture writes a fresh VT and unwrapped-text pair and sets VT and TXT.
capture() {
  N=$((N + 1))
  VT=/tmp/fingers-test-$$-$N.vt; TXT=/tmp/fingers-test-$$-$N.txt
  rex block call -s "$S" -b "$B" format '{"format":"vt"}' | jq -j .content > "$VT"
  rex block call -s "$S" -b "$B" format '{"format":"text","unwrap":true,"trim":true}' | jq -j .content > "$TXT"
}
# text prints the pane without the right prompt's clock, which nushell
# redraws whenever it repaints the prompt.
text() { rex capture -s "$S" -b "$B" --format=text --trim | sed -E 's/ +[0-9]{2}:[0-9]{2}:[0-9]{2}$//; s/ *$//'; }
# send_key KEY MODS TEXT sends one key press through client.key and prints
# whether a Lua key function took it. KEY is a W3C name in snake_case.
send_key() {
  rex api call -s "$S" client.key "{\"session_id\":\"$S\",\"block_id\":\"$B\",\"key\":{\"key\":\"$1\",\"mods\":\"$2\",\"consumed_mods\":\"\",\"text\":\"$3\",\"unshifted_codepoint\":0,\"composing\":false,\"action\":\"press\"}}" | jq -r .handled
}
# current writes the pane's VT as it is now, what the Lua side passes to
# press and restore as --current-file, and sets CUR.
current() {
  N=$((N + 1)); CUR=/tmp/fingers-test-$$-$N.cur
  rex block call -s "$S" -b "$B" format '{"format":"vt"}' | jq -j .content > "$CUR"
}
modes_ok() {
  rex block call -s "$S" -b "$B" modes | jq -e '[.modes[] | select(.number==7 or .number==25) | .set] == [true,true]' >/dev/null
}
# hint_for SUFFIX prints the hint drawn over a match whose text ends in
# SUFFIX: the letters just before it on screen. Pass the part of the match
# the hint cannot cover, "mp/d" for a two-letter hint over "/tmp/d".
hint_for() { text | grep -o "[a-z]*$1" | head -1 | sed "s#$1\$##"; }


# --- assertions -------------------------------------------------------
check() { # check NAME CONDITION...
  NAME=$1; shift
  if "$@"; then return 0; fi
  echo "    FAIL: $NAME"; CASE_OK=0; return 1
}
same_text() { [ "$(text)" = "$BEFORE" ]; }
no_files() { [ ! -e "$RUN" ] && ! ls /tmp/fingers-test-$$-* >/dev/null 2>&1; }
show_diff() { echo "    expected:"; echo "$BEFORE" | sed 's/^/      |/' | head -6; echo "    got:"; text | sed 's/^/      |/' | head -6; }
begin_case() {
  CASE=$1
  case "$CASE" in *"$FILTER"*) ;; *) return 1 ;; esac
  echo "== $CASE"; CASE_OK=1; open_session; return 0
}
end_case() {
  check "pane text unchanged" same_text || show_diff
  check "autowrap and cursor modes set" modes_ok
  check "no capture or run file left" no_files
  rm -f /tmp/fingers-test-$$-*
  close_session
  if [ "$CASE_OK" = 1 ]; then PASS=$((PASS + 1)); echo "   ok"; else FAIL=$((FAIL + 1)); fi
}

# --- cases ------------------------------------------------------------
if begin_case "draw then restore"; then
  type_line 'clear; echo /tmp/a https://example.com; echo /tmp/b'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  check "hints drawn" [ "$(text)" != "$BEFORE" ]
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  end_case
fi

if begin_case "press a full hint copies and restores"; then
  type_line 'clear; echo /tmp/a; echo /tmp/b'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  H=$(hint_for tmp/b); pbcopy </dev/null
  current
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --key "$H" --current-file "$CUR")
  check "press reports done" [ "$(echo "$OUT" | jq -r .done)" = true ]
  check "an unchanged pane is not reported as changed" [ "$(echo "$OUT" | jq -r .changed)" = null ]
  sleep 0.3; check "clipboard holds /tmp/b" [ "$(pbpaste)" = /tmp/b ]
  end_case
fi

if begin_case "two-letter hint: partial press repaints, full press restores"; then
  type_line 'clear; echo /tmp/a /tmp/b /tmp/c /tmp/d'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as >/dev/null
  H=$(hint_for mp/d); check "hint has two letters" [ "${#H}" = 2 ]
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --key "$(echo "$H" | cut -c1)")
  check "first letter: not done, typed recorded" [ "$(echo "$OUT" | jq -r .typed)" = "$(echo "$H" | cut -c1)" ]
  check "run file records typed" [ "$(jq -r .typed "$RUN")" = "$(echo "$H" | cut -c1)" ]
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --typed "$(echo "$H" | cut -c1)" --key "$(echo "$H" | cut -c2)")
  check "second letter: done" [ "$(echo "$OUT" | jq -r .done)" = true ]
  end_case
fi

if begin_case "letter outside the alphabet changes nothing"; then
  type_line 'clear; echo /tmp/a'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as >/dev/null
  PAINTED=$(text)
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --key z)
  check "not done, nothing typed" [ "$(echo "$OUT" | jq -c '[.done,.typed]')" = '[false,""]' ]
  check "frame unchanged" [ "$(text)" = "$PAINTED" ]
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  end_case
fi

if begin_case "no hints: nothing drawn, nothing left"; then
  type_line 'clear; echo nothing to see here'; BEFORE=$(text)
  capture; OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns url)
  check "zero hints reported" [ "$(echo "$OUT" | jq -r .hints)" = 0 ]
  rm -f "$VT" "$TXT" # the Lua side deletes the capture when hints == 0
  end_case
fi

if begin_case "cancel action with no run on record recovers"; then
  type_line 'clear; echo /tmp/a; echo /tmp/b'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1; sleep 1
  end_case
fi

if begin_case "trigger action with hints left recovers, cancel cleans"; then
  type_line 'clear; echo /tmp/a; echo /tmp/b'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  # scope=window takes the in-pane path whatever overlay says. No run is on
  # record for the left hints, so draw --begin recovers them first.
  rex do rex_fingers -s "$S" -b "$B" scope=window >/dev/null 2>&1; sleep 1.5
  check "run file present after the trigger" [ -e "$RUN" ]
  rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1; sleep 1
  end_case
fi

if begin_case "second draw recovers a lost run, restore cleans"; then
  type_line 'clear; echo /tmp/a; echo /tmp/b'; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --cursor-visible=false --wraparound=false >/dev/null
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --cursor-visible=false --wraparound=false >/dev/null
  end_case
fi

if begin_case "pane output during the mode, no scroll"; then
  type_line 'clear; echo /tmp/a; echo /tmp/b'
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  # Something writes to the pane while the hints are up: here the shell
  # echoes a typed command and its output.
  type_line 'echo /tmp/c'
  BEFORE=$(text | sed -E -e 's#(^|[^[:alnum:]/])[a-z](tmp/)#\1/\2#g' -e 's#^[a-z](/dev/)#~\1#')  # what the pane should show: new output, no hints
  current
  OUT=$($BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --current-file "$CUR")
  check "restore reports the change" [ "$(echo "$OUT" | jq -r .changed)" = true ]
  end_case
fi

if begin_case "pane output during the mode, scrolls"; then
  type_line 'clear; seq 1 '"$((R - 3))"'; echo /tmp/a'
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  type_line 'echo /tmp/c; echo /tmp/d; echo /tmp/e'
  BEFORE=$(text | sed -E -e 's#(^|[^[:alnum:]/])[a-z](tmp/)#\1/\2#g' -e 's#^[a-z](/dev/)#~\1#')
  current
  H=$(hint_for tmp/a); pbcopy </dev/null
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --key "${H:-a}" --current-file "$CUR")
  check "press on a changed pane still picks" [ "$(echo "$OUT" | jq -c '[.done,.changed,.pane]')" = '[true,true,0]' ]
  sleep 0.3; check "clipboard holds /tmp/a" [ "$(pbpaste)" = /tmp/a ]
  end_case
fi

if begin_case "a status line rewritten in place, two-letter hint picks"; then
  # A spinner or a timer, as Claude Code shows, changes the pane on every
  # press. Two letters: the first must not repaint over the new output.
  # Under sh: nushell shows only the last statement's echo, as a table.
  W='echo /tmp/a /tmp/b /tmp/c /tmp/d; i=0; while [ $i -lt 40 ]; do printf "\rworking %d" $i; i=$((i+1)); sleep 0.2; done'
  type_line "clear; sh -c '$W'" 0.6
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as >/dev/null
  H=$(hint_for mp/d); check "hint has two letters" [ "${#H}" = 2 ]
  sleep 0.5; current; pbcopy </dev/null
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --key "$(echo "$H" | cut -c1)" --current-file "$CUR")
  check "first letter: not done" [ "$(echo "$OUT" | jq -c '[.done,.typed]')" = "[false,\"$(echo "$H" | cut -c1)\"]" ]
  sleep 0.5; current
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --typed "$(echo "$H" | cut -c1)" --key "$(echo "$H" | cut -c2)" --current-file "$CUR")
  check "second letter: picked" [ "$(echo "$OUT" | jq -c '[.done,.pane]')" = '[true,0]' ]
  sleep 0.3; check "clipboard holds /tmp/d" [ "$(pbpaste)" = /tmp/d ]
  sleep 8 # the loop ends, so the pane's text settles
  BEFORE=$(text | sed -E -e 's#(^|[^[:alnum:]/])[a-z]{1,2}(tmp/)#\1/\2#g' -e 's#^[a-z](/dev/)#~\1#')
  end_case
fi

if begin_case "pane output during the mode, a background writer"; then
  # The pane's shell may be nushell, so the writer is started through sh.
  type_line "clear; echo /tmp/a; sh -c 'for i in 1 2 3 4 5; do sleep 0.3; echo /tmp/w\$i; done &'" 0.3
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  sleep 2
  BEFORE=$(text | sed -E -e 's#(^|[^[:alnum:]/])[a-z](tmp/)#\1/\2#g' -e 's#^[a-z](/dev/)#~\1#')
  current
  OUT=$($BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --current-file "$CUR")
  check "restore reports the change" [ "$(echo "$OUT" | jq -r .changed)" = true ]
  # Counted as occurrences: nushell holds the tty in raw mode at its
  # prompt, so background output gets no carriage returns and staircases.
  check "the writer's five lines are on screen" [ "$(text | grep -o 'tmp/w' | wc -l | tr -d ' ')" = 5 ]
  end_case
fi

if begin_case "scrollback above a half-empty screen, restore with current"; then
  # clear in nushell also wipes the scrollback, so the screen is erased
  # with ED 2 alone: the rows above stay in the scrollback.
  type_line "sh -c 'seq 1 40; printf \"\\033[H\\033[2J\"; echo /tmp/a'"; BEFORE=$(text)
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  check "hints drawn" [ "$(text)" != "$BEFORE" ]
  current
  OUT=$($BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --current-file "$CUR")
  check "an unchanged pane is not reported as changed" [ "$(echo "$OUT" | jq -r .changed)" = false ]
  end_case
fi

if begin_case "window scope: two panes drawn, pressed, restored"; then
  type_line 'clear; echo /tmp/one'; BEFORE=$(text)
  B2=$(rex split -s "$S" -b "$B" --split=below --focus=false --json | jq -r '.block_ids[0]'); sleep 1.5
  rex send -s "$S" -b "$B2" 'clear; echo /tmp/two' && rex send-key -s "$S" -b "$B2" enter; sleep 0.8
  P2=$(rex block call -s "$S" -b "$B2" process | jq -r .foreground.pid)
  SZ2=$(rex block call -s "$S" -b "$B2" size); R2=$(echo "$SZ2" | jq -r .rows); C2=$(echo "$SZ2" | jq -r .columns)
  BEFORE2=$(rex capture -s "$S" -b "$B2" --format=text --trim | sed 's/ *$//')
  SZ=$(rex block call -s "$S" -b "$B" size); R=$(echo "$SZ" | jq -r .rows); C=$(echo "$SZ" | jq -r .columns)
  capture; V1=$VT; X1=$TXT
  N=$((N + 1)); V2=/tmp/fingers-test-$$-$N.vt; X2=/tmp/fingers-test-$$-$N.txt
  rex block call -s "$S" -b "$B2" format '{"format":"vt"}' | jq -j .content > "$V2"
  rex block call -s "$S" -b "$B2" format '{"format":"text","unwrap":true,"trim":true}' | jq -j .content > "$X2"
  S1="vt=$V1,text=$X1,rows=$R,cols=$C"; S2="vt=$V2,text=$X2,rows=$R2,cols=$C2"
  OUT=$($BIN draw --begin --pane "pid=$P,$S1" --pane "pid=$P2,$S2" --patterns path)
  T2=$(echo "$OUT" | jq -r '.panes[1].tty')
  H=$(rex capture -s "$S" -b "$B2" --format=text --trim | grep -o '[a-z]*tmp/two' | head -1 | sed 's#tmp/two$##')
  check "second pane has a hint" [ -n "$H" ]
  pbcopy </dev/null
  OUT=$($BIN press --pane "tty=/dev/$T,$S1" --pane "tty=$T2,$S2" --patterns path --key "$H")
  check "press names pane 1" [ "$(echo "$OUT" | jq -r .pane)" = 1 ]
  sleep 0.3; check "clipboard holds /tmp/two" [ "$(pbpaste)" = /tmp/two ]
  check "second pane text unchanged" [ "$(rex capture -s "$S" -b "$B2" --format=text --trim | sed 's/ *$//')" = "$BEFORE2" ]
  end_case
fi

if begin_case "keys: a run takes hint, Enter, and Escape; an idle pane keeps its keys"; then
  type_line 'clear; echo /tmp/a; echo /tmp/b'; BEFORE=$(text)
  check "idle: Escape is not taken" [ "$(send_key escape '' '')" = false ]
  rex do rex_fingers -s "$S" -b "$B" scope=window >/dev/null 2>&1; sleep 1.5
  H=$(hint_for tmp/b); check "a hint is drawn" [ -n "$H" ]
  check "run: Enter is taken" [ "$(send_key enter '' '')" = true ]
  check "run: a non-hint letter is taken" [ "$(send_key key_n '' n)" = true ]
  pbcopy </dev/null
  check "run: the hint is taken" [ "$(send_key "key_$H" '' "$H")" = true ]
  sleep 0.5; check "clipboard holds /tmp/b" [ "$(pbpaste)" = /tmp/b ]
  check "after the pick: Escape is not taken" [ "$(send_key escape '' '')" = false ]
  rex do rex_fingers -s "$S" -b "$B" scope=window >/dev/null 2>&1; sleep 1.5
  check "Escape is taken" [ "$(send_key escape '' '')" = true ]
  sleep 0.5
  end_case
fi

# --- option and action cases --------------------------------------------
# count_hints draws with the given flags on a fresh capture, prints the
# number of hints, and puts the pane back. It runs in a subshell, so it
# leaves no capture behind.
count_hints() {
  capture
  OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common "$@")
  n=$(echo "$OUT" | jq -r .hints)
  if [ "$n" = 0 ]; then rm -f "$VT" "$TXT"
  else $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null; fi
  echo "$n"
}
float_layers() { rex api call -s "$S" session.view '{}' | jq '[.windows[0].layers[] | select(.kind!="tiled")] | length'; }

if begin_case "config: check passes, key bound, no fingers key mode"; then
  BEFORE=$(text)
  check "rex config check passes" rex config check
  check "rex_fingers is bound" sh -c 'rex keymap | grep -q rex_fingers'
  check "no fingers/ binding" [ "$(rex keymap --json | jq '[.bindings[] | select(.keys[0] | startswith("fingers/"))] | length')" = 0 ]
  check "no fingers mode" [ "$(rex keymap --json | jq '.modes | tostring | test("\"fingers\"")')" = false ]
  end_case
fi

if begin_case "float: trigger opens a floating pane, trigger again closes it"; then
  type_line 'clear; echo /tmp/a'; BEFORE=$(text)
  rex do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1; sleep 2
  F=$(float_layers)
  if [ "$F" = 0 ]; then
    echo "   note: the loaded config draws in the pane, so the float path was not tested"
    rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1; sleep 1
  else
    check "one floating layer is open" [ "$F" = 1 ]
    rex do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1; sleep 2
    check "the second trigger closes it" [ "$(float_layers)" = 0 ]
  fi
  end_case
fi

if begin_case "patterns: named, default without digit, extra, and a bad name"; then
  type_line 'clear; echo /tmp/a https://example.com 8080 ISSUE-42'; BEFORE=$(text)
  N_ALL=$(count_hints); N_DIGIT=$(count_hints --patterns digit)
  N_URL=$(count_hints --patterns url); N_EXTRA=$(count_hints --patterns url --extra 'ISSUE-[0-9]+')
  check "default finds matches" [ "$N_ALL" -gt 0 ]
  check "digit is off by default but works when named" [ "$N_DIGIT" -gt 0 ]
  check "url alone finds fewer than the default" [ "$N_URL" -lt "$N_ALL" ]
  check "an extra pattern adds matches" [ "$N_EXTRA" -gt "$N_URL" ]
  capture
  check "an unknown pattern name fails" sh -c "! $BIN draw --begin --pid $P --vt-file $VT --text-file $TXT $common --patterns nonsense >/dev/null 2>&1"
  rm -f "$VT" "$TXT"
  end_case
fi

if begin_case "hint position, layout, and alphabet"; then
  type_line 'clear; echo /tmp/1'; BEFORE=$(text)
  # The digit after the last slash is never a hint letter, so a hint over
  # it shows as a letter there. With the hint on the left, it does not.
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path --hint-position right >/dev/null
  check "right: a hint letter replaces the last character" [ "$(text | grep -c '/tmp/[a-z]')" -ge 1 ]
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  check "left: the last character stays" [ "$(text | grep -c '/tmp/[a-z]')" = 0 ]
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  for L in $($BIN layouts | awk '{print $1}'); do
    ALPHA=$($BIN layouts | awk -v l="$L" '$1 == l {print $2}')
    capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path --keyboard-layout "$L" >/dev/null
    H=$(text | grep -o '[a-z;,.]*tmp/1' | head -1 | sed 's#tmp/1$##')
    check "layout $L: the hint '$H' is in its alphabet" sh -c "[ -n '$H' ] && case '$ALPHA' in *'$H'*) true ;; *) false ;; esac"
    $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  done
  end_case
fi

if begin_case "actions: shell command with stdin and MODIFIER, and copy_command"; then
  # One echo: nushell shows only the last statement's output.
  type_line 'clear; echo "/tmp/a /tmp/b"'; BEFORE=$(text)
  ACT=/tmp/fingers-act-$$; rm -f "$ACT" "$ACT.copy"
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  H=$(hint_for tmp/b)
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --key "$H" --modifier ctrl --cwd /tmp \
    --ctrl-action "cat > $ACT; printf '\\n%s\\n' \"\$MODIFIER\" >> $ACT")
  check "press is done" [ "$(echo "$OUT" | jq -r .done)" = true ]
  sleep 0.5
  check "the command read the match on stdin" [ "$(head -1 "$ACT" 2>/dev/null)" = /tmp/b ]
  check "MODIFIER is ctrl" [ "$(tail -1 "$ACT" 2>/dev/null)" = ctrl ]
  capture; $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path >/dev/null
  H=$(hint_for tmp/a)
  $BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --key "$H" --copy-command "cat > $ACT.copy" >/dev/null
  sleep 0.5
  check "copy_command received the match" [ "$(cat "$ACT.copy" 2>/dev/null)" = /tmp/a ]
  rm -f "$ACT" "$ACT.copy"
  end_case
fi

if begin_case "character widths: hints over every width case, pane restored"; then
  type_line 'clear; cat testdata/width-cases.txt'; BEFORE=$(text)
  capture; OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path)
  check "hints drawn over the width cases" [ "$(echo "$OUT" | jq -r .hints)" -gt 0 ]
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  end_case
fi

if begin_case "stress: a full screen of matches, ten draw and restore cycles"; then
  W='i=1; while [ $i -le '"$((R - 4))"' ]; do echo /tmp/f$i; i=$((i+1)); done'
  type_line "clear; sh -c '$W'"; BEFORE=$(text)
  i=0
  while [ $i -lt 10 ]; do
    capture; OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path)
    check "cycle $i: hints drawn" [ "$(echo "$OUT" | jq -r .hints)" -gt 0 ]
    $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
    check "cycle $i: pane restored" same_text || show_diff
    rm -f "$VT" "$TXT"
    i=$((i + 1))
  done
  end_case
fi

echo
echo "passed $PASS, failed $FAIL"
[ "$FAIL" = 0 ]
