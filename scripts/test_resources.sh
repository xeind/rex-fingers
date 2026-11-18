#!/bin/sh
# Resource and multi-pane test, headless. It checks that rex-fingers leaves
# nothing behind (files, processes, clipboard), that the program's memory
# does not run away with the scrollback, that the Rex server does not grow
# across many runs, and that runs over 2 to 12 panes draw, pick, and
# restore. Each case opens a fresh Rex session named fingers-test and kills
# it at the end. The clipboard is saved first and put back on exit; only
# its text survives, an image does not.
#
#   sh scripts/test_resources.sh            # all cases
#   sh scripts/test_resources.sh memory     # cases whose name contains "memory"
#   COUNT_DIV=10 sh scripts/test_resources.sh leaks   # a tenth of the cycles
#
# Needs bin/rex-fingers (go build -o bin/rex-fingers .), jq, pbcopy, perl,
# and the plugin loaded in the running Rex config.
cd "$(dirname "$0")/.." || exit 1
BIN=./bin/rex-fingers
FILTER=${1:-}
PASS=0; FAIL=0; SKIPPED=""
TMP=${TMPDIR:-/tmp}; TMP=${TMP%/}
WORK=/tmp/fingers-res-work-$$   # notes, snapshots, and the large captures
CAPS=/tmp/fingers-res-caps-$$   # the captures a case hands to the program
DIV=${COUNT_DIV:-1}
START=$(date +%s)
S=
mkdir -p "$WORK" "$CAPS"
pbpaste > "$WORK/clipboard"
cleanup() {
  [ -n "$S" ] && rex kill "$S" >/dev/null 2>&1
  pbcopy < "$WORK/clipboard"
  rm -rf "$WORK" "$CAPS"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# Pass limits for the memory cases, in KB: twice the peak measured with
# 100,000 lines of scrollback, the larger of the two modes (2026-11-18,
# build 2027, arm64). Measured: draw 45264 KB (with --view-file; 23392 KB
# without), press 23408 KB, restore 23136 KB. draw with --view-file took
# 185824 KB while the page search kept every row's cells.
LIMIT_DRAW_KB=90528
LIMIT_PRESS_KB=46816
LIMIT_RESTORE_KB=46272
# The server may grow this much over 200 trigger and cancel cycles.
LIMIT_SERVER_KB=20480
# Hint letters that never appear in "/tmp/pane-NN", so a hint read off the
# screen is unambiguous: one letter replaces "/", two replace "/t".
ALPHA=bdfghjklorsuvwxyz
PAT="--patterns path"

scaled() { n=$(($1 / DIV)); [ "$n" -ge 1 ] || n=1; echo "$n"; }
now_ms() { perl -MTime::HiRes=time -e 'printf "%d\n", time * 1000'; }

# --- what a run leaves behind -------------------------------------------
# One definition, used by every case. Left behind means:
#   - files named rex-fingers-* in the temp directory: run files, wait files,
#     captures, and saved clipboards
#   - files in $CAPS, where this script puts the captures it passes on
#   - processes that run "rex-fingers draw", the background wait included
# The process test reads the command, so a shell whose command line only
# mentions the words does not count.
left_behind() {
  ls -d "$TMP"/rex-fingers-* 2>/dev/null
  ls -d "$CAPS"/* 2>/dev/null
  ps -axo pid=,command= | grep -E '^ *[0-9]+ [^ ]*rex-fingers draw( |$)'
}
count_left() { left_behind | wc -l | tr -d ' '; }
# no_leftovers waits up to 4 seconds, since the background wait can run 3,
# and sets LEFT to what remains.
no_leftovers() {
  w=0
  while :; do
    LEFT=$(left_behind)
    [ -z "$LEFT" ] && return 0
    [ "$w" -ge 16 ] && return 1
    sleep 0.25; w=$((w + 1))
  done
}
# clean_left removes the files a failed case left, so the next case starts
# clean. It leaves processes alone.
clean_left() { rm -f "$TMP"/rex-fingers-* "$CAPS"/*; }

# --- session helpers ----------------------------------------------------
# pane_info BLOCK sets PID_, TTY_, ROWS_, and COLS_.
pane_info() {
  PID_=$(rex block call -s "$S" -b "$1" process | jq -r .foreground.pid)
  TTY_=$(ps -o tty= -p "$PID_" | tr -d ' ')
  SZ_=$(rex block call -s "$S" -b "$1" size)
  ROWS_=$(echo "$SZ_" | jq -r .rows); COLS_=$(echo "$SZ_" | jq -r .columns)
}
open_session() {
  S=$(rex new fingers-test --json | jq -r .session_id); sleep 1.5
  B=$(rex api call -s "$S" session.view '{}' | jq -r '.windows[0].layers[0].blocks[0].block_id')
  BLOCKS=$B
  pane_info "$B"
  P=$PID_; T=$TTY_; R=$ROWS_; C=$COLS_
  RUN="$TMP/rex-fingers-$T.run.json"
  common="--rows $R --cols $C"
}
close_session() { rex kill "$S" >/dev/null 2>&1; S=; }
# open_panes N splits the first pane until the window holds N panes and
# lists them in BLOCKS. The splits alternate below and right.
open_panes() {
  last=$B; k=1
  while [ "$k" -lt "$1" ]; do
    if [ $((k % 2)) = 1 ]; then d=below; else d=right; fi
    last=$(rex split -s "$S" -b "$last" --split=$d --focus=false --json | jq -r '.block_ids[0]')
    BLOCKS="$BLOCKS $last"; k=$((k + 1))
  done
  sleep 2
}
nth() { echo "$1" | awk -v n="$2" '{print $n}'; }
type_to() { rex send -s "$S" -b "$1" "$2" && rex send-key -s "$S" -b "$1" enter; }
type_line() { type_to "$B" "$1"; sleep "${2:-0.8}"; }
# ptext prints a pane without the right prompt's clock.
ptext() { rex capture -s "$S" -b "$1" --format=text --trim | sed -E 's/ +[0-9]{2}:[0-9]{2}:[0-9]{2}$//; s/ *$//'; }
# snapshot_all records every pane's text, to compare at the end of the case.
snapshot_all() { k=0; for b in $BLOCKS; do k=$((k + 1)); ptext "$b" > "$WORK/before-$k"; done; }
N=0
# capture_block BLOCK writes a fresh VT and unwrapped-text pair in $CAPS and
# sets VT and TXT.
capture_block() {
  N=$((N + 1)); VT=$CAPS/cap-$N.vt; TXT=$CAPS/cap-$N.txt
  rex block call -s "$S" -b "$1" format '{"format":"vt"}' | jq -j .content > "$VT"
  rex block call -s "$S" -b "$1" format '{"format":"text","unwrap":true,"trim":true}' | jq -j .content > "$TXT"
}
send_key() { # send_key KEY MODS TEXT: prints whether a Lua key function took it
  rex api call -s "$S" client.key "{\"session_id\":\"$S\",\"block_id\":\"$B\",\"key\":{\"key\":\"$1\",\"mods\":\"$2\",\"consumed_mods\":\"\",\"text\":\"$3\",\"unshifted_codepoint\":0,\"composing\":false,\"action\":\"press\"}}" | jq -r .handled
}
alt_set() { rex block call -s "$S" -b "$1" modes | jq -r 'any(.modes[]; .number==1049 and .set)'; }
# pane_hint BLOCK ALPHABET NAME prints the hint over the match "/tmp/NAME",
# read off the screen. ALPHABET holds the hint letters. A one-letter hint
# replaces the slash, a two-letter hint the slash and the t.
pane_hint() {
  txt=$(ptext "$1")
  h=$(echo "$txt" | grep -o "[$2][$2]mp/$3" | head -1 | cut -c1-2)
  [ -n "$h" ] || h=$(echo "$txt" | grep -o "[$2]tmp/$3" | head -1 | cut -c1)
  echo "$h"
}

# --- many-pane helpers ---------------------------------------------------
# build_specs captures every pane of BLOCKS into $WORK/specs, one line each:
# block, pid, capture, text, rows, columns.
build_specs() {
  : > "$WORK/specs"
  for b in $BLOCKS; do
    pane_info "$b"; capture_block "$b"
    echo "$b $PID_ $VT $TXT $ROWS_ $COLS_" >> "$WORK/specs"
  done
}
draw_args() { awk '{printf "--pane pid=%s,vt=%s,text=%s,rows=%s,cols=%s ", $2,$3,$4,$5,$6}' "$WORK/specs"; }
tty_args() { paste -d' ' "$WORK/specs" "$WORK/ttys" | awk '{printf "--pane tty=%s,vt=%s,text=%s,rows=%s,cols=%s ", $7,$3,$4,$5,$6}'; }
# draw_panes FLAGS... draws over every pane, sets OUT and DRAW_MS, and
# records the ttys draw found.
draw_panes() {
  t0=$(now_ms)
  OUT=$($BIN draw --begin $(draw_args) "$@")
  DRAW_MS=$(($(now_ms) - t0))
  echo "$OUT" | jq -r '.panes[].tty' > "$WORK/ttys"
}
restore_panes() { $BIN restore $(tty_args) "$@" >/dev/null; }
# press_hint HINT FLAGS... presses a hint one letter at a time over every
# pane and sets OUT to the last answer.
press_hint() {
  h=$1; shift; typed=
  while [ -n "$h" ]; do
    k=$(echo "$h" | cut -c1); h=${h#?}
    OUT=$($BIN press $(tty_args) --key "$k" --typed "$typed" "$@")
    [ "$(echo "$OUT" | jq -r .done)" = true ] && return 0
    typed=$(echo "$OUT" | jq -r '.typed // ""')
  done
}

# --- assertions ----------------------------------------------------------
check() { # check NAME CONDITION...
  NAME=$1; shift
  if "$@"; then return 0; fi
  echo "    FAIL: $NAME"; CASE_OK=0; return 1
}
pane_same() { [ "$(ptext "$1")" = "$(cat "$WORK/before-$2")" ]; }
pane_modes_ok() {
  rex block call -s "$S" -b "$1" modes | jq -e '[.modes[] | select(.number==7 or .number==25) | .set] == [true,true]' >/dev/null
}
# pane_alt_ok BLOCK: mode 1049 is set only in the panes of EXPECT_ALT.
pane_alt_ok() {
  want=false
  case " $EXPECT_ALT " in *" $1 "*) want=true ;; esac
  [ "$(alt_set "$1")" = "$want" ]
}
begin_case() {
  CASE=$1
  case "$CASE" in *"$FILTER"*) ;; *) return 1 ;; esac
  echo "== $CASE"; CASE_OK=1; CASE_SKIP=0; EXPECT_ALT=; open_session; return 0
}
# skip_case REASON ends the case without a verdict.
skip_case() { CASE_SKIP=1; echo "   skipped: $1"; SKIPPED="$SKIPPED
  $CASE: $1"; }
# end_case checks every pane and what the case left behind, then closes the
# session. A leftover is reported and removed, so the next case starts clean.
end_case() {
  if [ "$CASE_SKIP" = 1 ]; then close_session; return; fi
  k=0
  for b in $BLOCKS; do
    k=$((k + 1))
    check "pane $k: text unchanged" pane_same "$b" "$k" || {
      echo "    expected:"; sed 's/^/      |/' "$WORK/before-$k" | head -6
      echo "    got:"; ptext "$b" | sed 's/^/      |/' | head -6; }
    check "pane $k: autowrap and cursor modes set" pane_modes_ok "$b"
    check "pane $k: alternate screen state" pane_alt_ok "$b"
  done
  check "no temp, capture, run, or wait file and no draw process left" no_leftovers || {
    echo "$LEFT" | sed 's/^/      /'; clean_left; }
  close_session
  if [ "$CASE_OK" = 1 ]; then PASS=$((PASS + 1)); echo "   ok"; else FAIL=$((FAIL + 1)); fi
}

# --- leaks ---------------------------------------------------------------
if begin_case "leaks: 200 draw and restore cycles on one pane"; then
  type_line 'clear; echo /tmp/a https://example.com; echo /tmp/b'; snapshot_all
  before=$(count_left); n=$(scaled 200); i=0; bad=0
  while [ "$i" -lt "$n" ]; do
    capture_block "$B"
    OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common)
    [ "$(echo "$OUT" | jq -r .hints)" -gt 0 ] 2>/dev/null || bad=$((bad + 1))
    $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
    i=$((i + 1))
  done
  check "every cycle drew hints ($bad of $n did not)" [ "$bad" = 0 ]
  no_leftovers; after=$(count_left)
  check "file and process count equal before and after ($before, $after)" [ "$before" = "$after" ]
  end_case
fi

if begin_case "leaks: 50 runs with --await-views, rows arrive, call off, time out"; then
  W='i=1; while [ $i -le 80 ]; do echo /tmp/file-$i; i=$((i+1)); done'
  type_line "sh -c '$W'" 1.2; snapshot_all
  # The page that ends 30 rows above the last row, as the app would copy it.
  rex capture -s "$S" -b "$B" --format=text > "$WORK/all"
  first=$(($(wc -l < "$WORK/all" | tr -d ' ') - R - 30 + 1))
  sed -n "${first},$((first + R - 1))p" "$WORK/all" > "$WORK/page"
  n=$(scaled 50); i=0; lost=0; noframe=0; noalt=0
  printf 'kept' | pbcopy
  while [ "$i" -lt "$n" ]; do
    capture_block "$B"
    spec="vt=$VT,text=$TXT,rows=$R,cols=$C"
    $BIN draw --begin --await-views 1 --pane "pid=$P,$spec" $PAT >/dev/null
    case $((i % 3)) in
      0) pbcopy < "$WORK/page" ;;
      1) $BIN restore --pane "tty=/dev/$T,$spec" >/dev/null; sleep 0.5 ;;
    esac
    if [ $((i % 3)) != 1 ]; then
      w=0; while [ ! -e "$RUN" ] && [ "$w" -lt 60 ]; do sleep 0.1; w=$((w + 1)); done
      if [ -e "$RUN" ]; then
        [ $((i % 3)) = 0 ] && [ "$(alt_set "$B")" != true ] && noalt=$((noalt + 1))
        $BIN restore --pane "tty=/dev/$T,$spec" >/dev/null
      else
        noframe=$((noframe + 1)); $BIN restore --pane "tty=/dev/$T,$spec" >/dev/null
      fi
    fi
    [ "$(pbpaste)" = kept ] || { lost=$((lost + 1)); printf 'kept' | pbcopy; }
    i=$((i + 1))
  done
  check "a frame came up in every run that was not called off ($noframe did not)" [ "$noframe" = 0 ]
  check "the rows that arrived were drawn on the alternate screen ($noalt were not)" [ "$noalt" = 0 ]
  check "the clipboard held 'kept' after every run ($lost did not)" [ "$lost" = 0 ]
  check "no rex-fingers-clipboard-* file left" [ -z "$(ls "$TMP"/rex-fingers-clipboard-* 2>/dev/null)" ]
  end_case
fi

if begin_case "leaks: 100 trigger and cancel cycles through Lua"; then
  type_line 'clear; echo /tmp/a https://example.com'; snapshot_all
  n=$(scaled 100); i=0; tfail=0; cfail=0
  while [ "$i" -lt "$n" ]; do
    rex do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1 || tfail=$((tfail + 1))
    if [ "$i" = 0 ]; then check "the first trigger draws hints" [ "$(ptext "$B")" != "$(cat "$WORK/before-1")" ]; fi
    rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1 || cfail=$((cfail + 1))
    i=$((i + 1))
  done
  check "no trigger failed ($tfail of $n)" [ "$tfail" = 0 ]
  check "no cancel failed ($cfail of $n)" [ "$cfail" = 0 ]
  check "Escape passes through to the pane" [ "$(send_key escape '' '')" = false ]
  end_case
fi

if begin_case "leaks: a killed session during a run"; then
  type_line 'clear; echo /tmp/dead-a https://example.com'
  capture_block "$B"
  $BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  DEAD_T=$T; DEAD_RUN=$RUN
  check "the dead session's run file exists" [ -e "$DEAD_RUN" ]
  close_session; sleep 0.5
  open_session
  type_line 'clear; echo /tmp/live-b'; snapshot_all
  if [ "$T" = "$DEAD_T" ]; then echo "   note: the new pane reuses $T"
  else echo "   note: the new pane has $T, not $DEAD_T; the dead run's files cannot be reached"; fi
  capture_block "$B"
  OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common $PAT)
  check "the new run drew hints" [ "$(echo "$OUT" | jq -r .hints)" -gt 0 ]
  check "the new pane does not show the dead pane's text" [ -z "$(ptext "$B" | grep dead-a)" ]
  $BIN restore --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common >/dev/null
  if [ "$T" = "$DEAD_T" ]; then
    check "the dead run's file is gone once the tty is reused" [ ! -e "$DEAD_RUN" ]
  else
    # A killed session cannot clean up, and the program never sees its tty
    # again. The files stay in the temp directory.
    [ -e "$DEAD_RUN" ] && echo "   note: $DEAD_RUN and its two captures stay behind"
    clean_left
  fi
  end_case
fi

# --- memory --------------------------------------------------------------
# measure CMD... runs the command under /usr/bin/time -l and sets RSS_KB to
# its maximum resident set size.
measure() {
  /usr/bin/time -l "$@" > "$WORK/out" 2> "$WORK/time"
  RSS_KB=$(awk '/maximum resident set size/ {printf "%d", $1 / 1024}' "$WORK/time")
}
# build_big SIZE writes SIZE lines of scrollback, then the pane's own VT, so
# the last rows are the pane's screen and a restore puts it back as it was.
build_big() {
  awk -v n="$1" 'BEGIN { for (i = 1; i <= n; i++) printf "scroll %d /tmp/synthetic/file-%d https://example.com/%d\r\n", i, i, i }' > "$WORK/big-$1.vt"
  cat "$WORK/pane.vt" >> "$WORK/big-$1.vt"
}
# view_text prints the first R scrollback lines: a page at the far end of the
# capture, the worst case for finding the rows a scrolled pane shows.
view_text() { awk -v r="$R" 'BEGIN { for (i = 1; i <= r; i++) printf "scroll %d /tmp/synthetic/file-%d https://example.com/%d\n", i, i, i }'; }
mem_prepare() {
  type_line 'clear; echo /tmp/a /tmp/b /tmp/c /tmp/d'; snapshot_all
  rex block call -s "$S" -b "$B" format '{"format":"vt"}' | jq -j .content > "$WORK/pane.vt"
  flags="--alphabet as $PAT $common"
}
# mem_draw SIZE VIEW draws over the big capture and puts the pane back.
# DRAW_KB is the draw's peak. VIEW is 1 to pass --view-file.
mem_start() { # mem_start SIZE VIEW [measure]: draws; the big capture is in place
  cp "$WORK/big-$1.vt" "$CAPS/big.vt"; vf=
  if [ "$2" = 1 ]; then view_text > "$CAPS/big.view"; vf="--view-file $CAPS/big.view"; fi
  if [ "$3" = measure ]; then
    measure $BIN draw --begin --pid "$P" --vt-file "$CAPS/big.vt" $flags $vf
    DRAW_KB=$RSS_KB; OUT=$(cat "$WORK/out")
  else
    OUT=$($BIN draw --begin --pid "$P" --vt-file "$CAPS/big.vt" $flags $vf)
  fi
}
mem_limit_check() { # mem_limit_check WHAT KB LIMIT
  check "$1 peak $(($2 / 1024)) MB is under $(($3 / 1024)) MB" [ "$2" -le "$3" ]
}

if begin_case "memory: draw against scrollback size, with and without --view-file"; then
  mem_prepare
  for size in 1000 10000 100000; do
    build_big "$size"
    for view in 0 1; do
      mem_start "$size" "$view" measure
      check "$size lines, view $view: draw found hints" [ "$(echo "$OUT" | jq -r .hints)" -gt 0 ]
      echo "   mem: draw, $size lines, view-file $view: $((DRAW_KB / 1024)) MB ($DRAW_KB KB)"
      mem_limit_check "draw at $size lines, view $view" "$DRAW_KB" "$LIMIT_DRAW_KB"
      $BIN restore --tty /dev/"$T" --vt-file "$CAPS/big.vt" $common >/dev/null
    done
    rm -f "$WORK/big-$size.vt"
  done
  end_case
fi

if begin_case "memory: press and restore against scrollback size"; then
  mem_prepare
  for size in 1000 10000 100000; do
    build_big "$size"
    for view in 0 1; do
      mem_start "$size" "$view"
      measure $BIN press --tty /dev/"$T" --vt-file "$CAPS/big.vt" $flags --key a
      PRESS_KB=$RSS_KB
      check "$size lines, view $view: the first letter is not a pick" [ "$(jq -r .done "$WORK/out")" = false ]
      measure $BIN restore --tty /dev/"$T" --vt-file "$CAPS/big.vt" $common
      REST_KB=$RSS_KB
      echo "   mem: press, $size lines, view-file $view: $((PRESS_KB / 1024)) MB ($PRESS_KB KB)"
      echo "   mem: restore, $size lines, view-file $view: $((REST_KB / 1024)) MB ($REST_KB KB)"
      mem_limit_check "press at $size lines, view $view" "$PRESS_KB" "$LIMIT_PRESS_KB"
      mem_limit_check "restore at $size lines, view $view" "$REST_KB" "$LIMIT_RESTORE_KB"
    done
    rm -f "$WORK/big-$size.vt"
  done
  end_case
fi

if begin_case "memory: Rex server across 200 trigger and cancel cycles"; then
  SPID=$(rex server status | awk '/^pid:/ {print $2}')
  if [ -z "$SPID" ] || ! ps -p "$SPID" >/dev/null 2>&1; then
    skip_case "no server process found: rex server status gave no pid"
  else
    type_line 'clear; echo /tmp/a https://example.com'; snapshot_all
    # Ten cycles first, so one-time allocations do not count as growth.
    i=0; while [ "$i" -lt 10 ]; do
      rex do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1; rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1
      i=$((i + 1)); done
    before=$(ps -o rss= -p "$SPID" | tr -d ' ')
    n=$(scaled 200); i=0
    while [ "$i" -lt "$n" ]; do
      rex do rex_fingers -s "$S" -b "$B" >/dev/null 2>&1; rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1
      i=$((i + 1))
    done
    sleep 1; after=$(ps -o rss= -p "$SPID" | tr -d ' ')
    echo "   mem: server pid $SPID, before $((before / 1024)) MB ($before KB), after $((after / 1024)) MB ($after KB), growth $(((after - before) / 1024)) MB"
    check "the server grew by $(((after - before) / 1024)) MB, at most $((LIMIT_SERVER_KB / 1024)) MB" [ $((after - before)) -le "$LIMIT_SERVER_KB" ]
  fi
  end_case
fi

# --- many panes ----------------------------------------------------------
# case_many N: N panes in one window, drawn by the program in one call.
case_many() {
  if begin_case "many panes: $1 panes drawn by the program, a pick from the last pane"; then
    open_panes "$1"
    k=0; for b in $BLOCKS; do k=$((k + 1)); type_to "$b" "$(printf 'clear; echo /tmp/pane-%02d' "$k")"; done
    sleep 1.5; snapshot_all; build_specs
    draw_panes $PAT --alphabet "$ALPHA"
    echo "   time: draw over $1 panes: $DRAW_MS ms"
    check "draw reports $1 panes" [ "$(echo "$OUT" | jq -r '.panes | length')" = "$1" ]
    k=0; : > "$WORK/hints"; missing=0
    for b in $BLOCKS; do
      k=$((k + 1)); h=$(pane_hint "$b" "$ALPHA" "$(printf 'pane-%02d' "$k")")
      [ -n "$h" ] && echo "$h" >> "$WORK/hints" || missing=$((missing + 1))
    done
    check "every pane shows a hint over its path ($missing do not)" [ "$missing" = 0 ]
    check "no hint repeats across panes" [ -z "$(sort "$WORK/hints" | uniq -d)" ]
    last=$(tail -1 "$WORK/hints")
    pbcopy < /dev/null
    press_hint "$last" $PAT --alphabet "$ALPHA"
    check "press reports done" [ "$(echo "$OUT" | jq -r .done)" = true ]
    check "press names pane $(($1 - 1))" [ "$(echo "$OUT" | jq -r .pane)" = "$(($1 - 1))" ]
    sleep 0.3
    check "the clipboard holds the last pane's path" [ "$(pbpaste)" = "$(printf '/tmp/pane-%02d' "$1")" ]
    end_case
  fi
}
case_many 2
case_many 4
case_many 8
case_many 12

if begin_case "many panes: 8 panes through Lua, scope=window, then cancel"; then
  open_panes 8
  k=0; for b in $BLOCKS; do k=$((k + 1)); type_to "$b" "$(printf 'clear; echo /tmp/pane-%02d' "$k")"; done
  sleep 1.5; snapshot_all
  rex do rex_fingers -s "$S" -b "$B" scope=window >/dev/null 2>&1; sleep 1.5
  k=0; plain=0
  for b in $BLOCKS; do k=$((k + 1)); [ "$(ptext "$b")" = "$(cat "$WORK/before-$k")" ] && plain=$((plain + 1)); done
  check "every pane shows hints ($plain still plain)" [ "$plain" = 0 ]
  rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1; sleep 1
  end_case
fi

if begin_case "many panes: a shell, less, a long scrollback, and a narrow pane"; then
  open_panes 4
  B2=$(nth "$BLOCKS" 2); B3=$(nth "$BLOCKS" 3); B4=$(nth "$BLOCKS" 4)
  awk 'BEGIN { for (i = 1; i <= 100; i++) printf "/tmp/less-line-%d https://example.com/%d\n", i, i }' > "$WORK/less.txt"
  type_to "$B" 'clear; echo /tmp/shell-pane'
  type_to "$B2" "less $WORK/less.txt"
  type_to "$B3" "sh -c 'i=1; while [ \$i -le 300 ]; do echo /tmp/scroll-\$i; i=\$((i+1)); done'"
  # A pane 10 columns wide: the block's resize call is tried first.
  rex block call -s "$S" -b "$B4" resize '{"columns":10,"rows":24}' >/dev/null 2>&1; sleep 0.5
  w=$(rex block call -s "$S" -b "$B4" size | jq -r .columns)
  if [ "$w" != 10 ]; then
    echo "   note: the pane stays $w columns wide, so the narrow pane holds text folded at 10 columns"
    type_to "$B4" "sh -c 'echo /tmp/narrow-pane-with-a-long-name | fold -w 10'"
  else
    type_to "$B4" 'clear; echo /tmp/narrow-pane-with-a-long-name'
  fi
  sleep 2; EXPECT_ALT=$B2; snapshot_all; build_specs
  check "less is in the alternate screen before the run" [ "$(alt_set "$B2")" = true ]
  draw_panes $PAT
  check "draw reports 4 panes" [ "$(echo "$OUT" | jq -r '.panes | length')" = 4 ]
  restore_panes
  check "the less pane still runs less" [ "$(rex block call -s "$S" -b "$B2" process | jq -r '.foreground.name')" = less ]
  end_case
fi

if begin_case "many panes: a pane closes during a run, then cancel"; then
  open_panes 4
  B3=$(nth "$BLOCKS" 3)
  k=0; for b in $BLOCKS; do k=$((k + 1)); type_to "$b" "$(printf 'clear; echo /tmp/pane-%02d' "$k")"; done
  sleep 1.5
  # Pane 3 is the one that closes; the other three are checked at the end.
  BLOCKS=$(echo "$BLOCKS" | tr ' ' '\n' | grep -v "^$B3\$" | tr '\n' ' ' | sed 's/ $//')
  snapshot_all
  rex do rex_fingers -s "$S" -b "$B" scope=window >/dev/null 2>&1; sleep 1.5
  check "the run started: a run file exists for the origin pane" [ -e "$RUN" ]
  type_to "$B3" exit; sleep 1.5
  rex do rex_fingers_cancel -s "$S" -b "$B" >"$WORK/cancel.out" 2>&1; rc=$?
  check "the cancel does not fail (exit $rc: $(head -c 200 "$WORK/cancel.out"))" [ "$rc" = 0 ]
  sleep 1
  end_case
fi

if begin_case "many panes: a pane is resized during a run, then cancel"; then
  open_panes 4
  B2=$(nth "$BLOCKS" 2)
  k=0; for b in $BLOCKS; do k=$((k + 1)); type_to "$b" "$(printf 'clear; echo /tmp/pane-%02d' "$k")"; done
  sleep 1.5; snapshot_all
  rex do rex_fingers -s "$S" -b "$B" scope=window >/dev/null 2>&1; sleep 1.5
  # rex resize is not implemented; the block's resize method is the way.
  rex block call -s "$S" -b "$B2" resize '{"columns":40,"rows":12}' >/dev/null 2>&1; sleep 1
  w=$(rex block call -s "$S" -b "$B2" size | jq -r .columns)
  if [ "$w" != 40 ]; then
    rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1; sleep 1
    skip_case "a headless pane stays $w columns wide: its size is owned by a client that is gone, so resize does not apply"
  else
    echo "   after the resize, before the cancel, the pane shows:"; ptext "$B2" | sed 's/^/      |/' | head -12
    rex do rex_fingers_cancel -s "$S" -b "$B" >/dev/null 2>&1; sleep 1
    echo "   after the cancel, the resized pane shows:"; ptext "$B2" | sed 's/^/      |/' | head -12
    # A hint puts a letter where the slash was: "xtmp/pane-" or "xymp/pane-".
    check "no hint letters remain in the resized pane" [ -z "$(ptext "$B2" | grep -E '[a-z]tmp/pane-|[a-z][a-z]mp/pane-')" ]
    # The reflow changes the pane's text, so only the other panes are compared.
    BLOCKS=$(echo "$BLOCKS" | tr ' ' '\n' | grep -v "^$B2\$" | tr '\n' ' ' | sed 's/ $//')
    snapshot_all
  fi
  end_case
fi

# --- scale ---------------------------------------------------------------
if begin_case "scale: every row a match, two hint letters, at most 4 hints"; then
  W='i=1; while [ $i -le '"$((R - 4))"' ]; do echo /tmp/f$i; i=$((i+1)); done'
  type_line "clear; sh -c '$W'"; snapshot_all
  capture_block "$B"
  OUT=$($BIN draw --begin --pid "$P" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as)
  n=$(echo "$OUT" | jq -r .hints)
  check "the run drew hints and reports 4 or fewer ($n)" [ "$n" -ge 1 -a "$n" -le 4 ]
  h=$(ptext "$B" | grep -o '[as][as]mp/f[0-9]*' | head -1 | cut -c1-2)
  check "a two-letter hint is on screen" [ "${#h}" = 2 ]
  pbcopy < /dev/null
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --key "$(echo "$h" | cut -c1)")
  OUT=$($BIN press --tty /dev/"$T" --vt-file "$VT" --text-file "$TXT" $common --patterns path --alphabet as --typed "$(echo "$h" | cut -c1)" --key "$(echo "$h" | cut -c2)")
  check "the second letter picks" [ "$(echo "$OUT" | jq -r .done)" = true ]
  sleep 0.3
  check "the clipboard holds one of the paths" [ -n "$(pbpaste | grep -E '^/tmp/f[0-9]+$')" ]
  end_case
fi

if begin_case "scale: width cases in 12 panes, draw and restore"; then
  open_panes 12
  for b in $BLOCKS; do type_to "$b" "clear; cat $PWD/scripts/testdata/width-cases.txt"; done
  sleep 2; snapshot_all; build_specs
  draw_panes $PAT
  echo "   time: draw over 12 panes of width cases: $DRAW_MS ms"
  check "hints drawn over the width cases" [ "$(echo "$OUT" | jq -r .hints)" -gt 0 ]
  restore_panes
  end_case
fi

END=$(date +%s)
echo
[ -n "$SKIPPED" ] && echo "skipped:$SKIPPED"
echo "elapsed $((END - START)) s"
echo "passed $PASS, failed $FAIL"
[ "$FAIL" = 0 ]
