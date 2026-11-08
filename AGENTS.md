# rex-fingers

Letter hints over paths, URLs, and hashes on a Rex pane, like tmux-fingers.
Go program plus a Lua plugin for Rex, the terminal multiplexer by
Superlogical. This file is the map for a coding agent asked to install,
configure, extend, or debug it.

## Install it for a user

1. `go install github.com/xeind/rex-fingers@latest`. The binary lands in
   `$(go env GOPATH)/bin/rex-fingers`.
2. Clone this repo to a permanent path, usually
   `~/.config/rex/plugins/rex-fingers`.
3. In the user's Rex config, `~/.config/rex/init.lua` or a file it
   requires, add:
   ```lua
   local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
   fingers.setup{ binary = os.getenv("HOME") .. "/go/bin/rex-fingers" }
   ```
   `binary` must be an absolute path.
4. `rex config check`, then `rex config reload`.
5. Confirm with `rex keymap | grep rex_fingers`: `alt+f` by default, and
   `overlay = "float"`. Pane mode is opt-in because Rex for Mac shows a
   popup listing the key mode's bindings; the user turned it down
   2026-11-07.

Every option and its default is in `README.md` under Configuration. A
walkthrough with a check per step is `docs/CONFIGURING.md`. Unknown option
names make `setup` error, so read the list before adding one.

## How it works

Rex has no API that draws into an existing terminal. The Lua action
`rex_fingers`:

1. Finds the focused pane's rectangle in `session.view`.
2. Captures the pane's screen twice with the terminal block's `format`
   method, as VT sequences and as text with `unwrap = true`, each to a
   temp file. The VT redraws the screen row by row; the unwrapped text
   says which rows the terminal wrapped (`screen.Join`), so a match can
   span them and copy whole.
3. Opens a floating layer with that rectangle, running
   `rex-fingers show --session S --block B --vt-file F --rows N ...` with
   `shell = "none"`.

The Go program parses the VT into a cell grid, runs the patterns on each
row, draws the dimmed screen with hints over the matches, reads one hint
in raw mode, runs the action, and exits. The floating pane closes on exit
and the program calls `rex focus` on the original pane.

The key toggles. The action first looks for a floating block labeled
`fingers` in the window (`open_floats`). If it finds one, it writes Escape
into it through the block's `write` method and returns. The program takes
that as a cancel, so the float closes and focus returns as above. Without
this, each press captured the focused float and stacked another over it.

That is `overlay = "float"`. With `overlay = "pane"` there is no floating
pane. The action:

1. Reads the pane's `modes`, `process`, `pwd`, and `size`, and captures
   the screen as above.
2. Runs `rex-fingers draw --begin --pid P --vt-file F --rows N --cols M`
   through `rex.exec`. The program finds the foreground process's tty
   and writes the dimmed screen with hints straight into it.
3. Enters the exclusive key mode `fingers`. Each hint letter is bound
   there to `rex_fingers_key`, plain and with each modifier whose action
   is set. The action runs `rex-fingers press --tty T --typed ... --key K`
   for each key; the program repaints, or on a full hint restores the pane
   and runs the action. `Escape`, `q`, and `Ctrl+C` run
   `rex_fingers_cancel`, which runs `rex-fingers restore`.
4. A run can end without restore: a config reload or an app restart
   drops the mode, and the hints stay painted. So `draw` writes
   `$TMPDIR/rex-fingers-<tty>.run.json`: the capture paths, the pane's
   modes, the letters typed, and the text it painted (`ui.Painted`).
   `press` rewrites it on a repaint; `restoreAll` deletes it. The next
   `draw --begin`, or `restore --recover` from the key or cancel action
   with no run on record, reads it (`paneSpec.recover`). When the pane's
   text equals the painted text, the old capture moves under the new
   paths, the modes come from the file, and `\x1b8` precedes `Begin`.
   Otherwise it deletes the old files. `press` and plain `restore` take the
   modes from the file too, since the Lua flags carry what was read from
   the painted frame.
5. The pane's program may write while the hints are up. Every paint ends
   with `\x1b8`, so that output lands where the program's cursor was.
   Lua captures each pane as VT (`currents`) before every `press` and
   `restore` and passes it as `--current-file`, or `current=` in a
   `--pane` spec; VT, not text, see the facts below. When the screen's
   text differs from the painted text the pane moved on
   (`paneSpec.checkMoved`): `alignRows` finds the scroll offset, only the
   rows that still show the frame are repainted (`inpane.RestoreRows`),
   the cursor is left where the program put it, and `press` ends the run
   with `changed = true` and no pick. `scripts/test_in_pane.sh` has three
   cases for it. Rows that scrolled into the scrollback keep the hints:
   no tty write reaches the scrollback, and the terminal block has no
   method that edits it. Painting on the alternate screen would keep the
   scrollback clean but discard what the program prints during the run,
   so the limit stays and the README states it. Reproduce with a pane
   that prints a line every 300 ms: draw once scrollback exists, wait
   3 s, cancel, and `rex capture --format=text` shows hint letters on the
   rows that scrolled off.

The in-pane subcommands print one JSON object each. `draw` returns the
tty and the hint count; `press` returns `done`, `typed`, and `paste`.

The `scope = "window"` argument, bound by `window_key`, always takes the
in-pane path, whatever `overlay` says: a float covers one pane, and only
the key mode lets one key reach every pane. So the mode and its actions
are registered for both overlays. With it, the action
does the same for every terminal block of the window at once. It reads
them from `layer.blocks[]` with `creator_name == "com.superlogical.terminal"`,
puts the focused block first, and passes one `--pane pid=,vt=,rows=,cols=,cursor=,wrap=`
spec per pane. `ui.NewSet` gives the panes one hint list and `ui.PressSet`
finds a pick across them; `press` returns `pane`, the index of the pane it
came from. A paste goes to the first pane, the one the key was pressed
in, and `--cwd` is its directory. The run is stored under every member
block, and `finish` clears and restores them all.

## Layout

| Path | Holds |
| --- | --- |
| `main.go` | CLI: `show`, `patterns`, `layouts`, `version`; action dispatch |
| `pane.go` | CLI: `draw`, `press`, `restore` for `overlay = "pane"`, one pane or several `--pane` specs; each prints one JSON object; `runState` and `recover` for a run left without restore |
| `internal/inpane` | `Begin` and `Restore` sequences, `TTY(pid)` to find the pane's tty |
| `internal/screen` | VT dump to lines of cells; `Parse`, `Line.Column`, `Line.CellAt` |
| `internal/patterns` | `Builtin` map, `Default` (all but `digit`), `Weight` per pattern for hint order, `Compile`, `Set.Find` with leftmost-longest overlap rule |
| `internal/hints` | `Generate` prefix-free hints; `Layouts` and `Alphabet` |
| `internal/ui` | `Overlay`: raw mode, `draw`, key handling, modifiers |
| `internal/rexcli` | `Pane.Size`, `Capture`, `Write`, `Pwd`, `Focus` through the `rex` CLI |
| `lua/rex-fingers/init.lua` | `setup(opts)`, the action, rect lookup, capture |
| `docs/CONFIGURING.md` | user walkthrough |
| `assets/` | `rex-fingers.icon`, the Icon Composer document: `claw.svg` as a matte Liquid Glass layer over `field.svg`, Rex's radial gradient sampled from the app icon, and `claw-dark.svg`, the claw filled with that gradient, shown only in Dark; `logo-claw-glass-rex.png` and `logo-claw-glass-rex-dark.png`, its exports at 512 px, the README logo (regenerate with File > Export, Appearance All, 2x); `logo-claw-black.svg` and `logo-claw-white.svg`, the bare mark; `social-preview.png`, the dark render padded to 1280x640 for GitHub |

## Commands

```sh
go test ./...                       # unit tests: screen, patterns, hints
go vet ./... && gofmt -l .
go build -o bin/rex-fingers .       # bin/ is gitignored
rex config check && rex config reload
rex do rex_fingers -b "$REX_BLOCK"  # open the overlay on this pane
rex server logs | grep -i lua       # Lua errors the check misses
```

## Test loop without a screen

An agent cannot see the overlay. Drive it through the Rex CLI:

```sh
S=$(rex new fingers-test --json | jq -r .session_id)
B=$(rex api call -s $S session.view '{}' | jq -r '.windows[0].layers[0].blocks[0].block_id')
sleep 1.5
rex send -s $S -b $B 'echo /tmp/a https://example.com' && rex send-key -s $S -b $B enter
sleep 1
rex do rex_fingers -s $S -b $B
sleep 2
OB=$(rex api call -s $S session.view '{}' | jq -r '.windows[0].layers[] | select(.kind!="tiled") | .blocks[0].block_id')
rex capture -s $S -b $OB --format=vt --trim | tail -3   # hints are visible as SGR sequences
rex send -s $S -b $OB a; sleep 1.5; pbpaste            # the first match
rex kill $S
```

In a session no app renders, the overlay stays 80x24 and the capture wraps
at column 80. That is a test artifact. Only a real key press in the app
sizes the overlay, so layout questions need the user's eyes or a
screenshot.

The in-pane path needs a client for the key mode, so a headless session
cannot press a hint. It can draw and restore. `testdata/width-cases.txt`
holds one line per character-width case:

```sh
S=$(rex new flag-probe --json | jq -r .session_id); sleep 1.5
B=$(rex api call -s $S session.view '{}' | jq -r '.windows[0].layers[0].blocks[0].block_id')
rex send -s $S -b $B 'clear; cat testdata/width-cases.txt' && rex send-key -s $S -b $B enter; sleep 1
rex block call -s $S -b $B format '{"format":"vt"}' | jq -j .content > /tmp/flag.vt
P=$(rex block call -s $S -b $B process | jq -r .foreground.pid)
go build -o bin/rex-fingers . && ./bin/rex-fingers draw --begin --pid $P --vt-file /tmp/flag.vt --rows 24 --cols 80
rex capture -s $S -b $B --format=text --trim | head -9   # every row: text, space, hint over the slash
./bin/rex-fingers restore --pid $P --vt-file /tmp/flag.vt --rows 24
rex kill $S
```

`rex keymap | grep -c fingers/` counts the mode's bindings; the app lists
every one while the mode is active.

Stale hints: `draw --begin` as above, then `rex do rex_fingers_cancel -s
$S -b $B`. The pane must show no hint letters and `modes` must report 7
and 25 set. Only the restore tells, since a second draw paints the same
frame whether or not it recovered the first run. Take `--rows` and
`--cols` from the block's `size` method; a headless pane is not always
80x24. Capture to a new file each time. A capture written over the first
run's path is the painted frame, and `recover` refuses it.

Window scope is testable the same way with two panes: `rex split -s $S
-b $B --split=below --focus=false --json` gives a second block, each gets
its own capture and `--pane` spec, and one `draw` call takes both. A
`press --key` with the same specs needs no client. Check that the hint
letters differ between the panes, that `press` reports the right `pane`
index and clipboard text, and that both panes are restored. Through Lua,
`rex do rex_fingers -s $S -b $B scope=window` runs up to the key mode and
then restores, since a headless session has no client to enter it.

## Facts the code depends on

- A floating pane in Rex for Mac is two rows shorter than the pane under
  it; `--align` picks which rows to drop. The pane reports 80x24 until the
  app sizes it, so `ui.waitForSize` polls up to 1.5 s and `draw` reruns on
  SIGWINCH.
- Rex for Mac build 1825 did not forward OSC 52 to the clipboard; build
  1859 does, from a pane's tty, within a second. Copy still uses `pbcopy`
  or a Linux equivalent and writes OSC 52 too, into the pane's tty in
  pane mode, so the app's own copy feedback fires when it has one.
- A pane whose program exits within a moment stays open in Rex. The
  program lives at least 400 ms.
- `rex do rex_fingers` from the CLI: `rex.client.queue` succeeds and the
  app refuses the queued `client.mode.enter` after the action returns, so
  no restore runs and the hints stay. A key press in the app works. The
  run file above covers the leftover.
- With autowrap off, the cursor stays on the last column after a row that
  fills the width, so erase-to-end-of-line there takes that cell. `ui.draw`
  skips `\x1b[K` on a full row.
- The user's shell is nushell. Its line editor holds the tty in raw mode
  at the prompt, so output from a background process gets no carriage
  returns and staircases across the screen. Not a fingers effect; count
  occurrences, not rows, when a test checks such output. Nushell also
  renders `echo a b c` as a table.
- macOS checks a freshly built binary on first run, about 360 ms. Run it
  once after building before judging latency.
- `session.view` nests `split.before` and `split.after`, each with `rect`
  in window fractions; a floating layer's layout is relative to its bounds.
- Go regexp accepts `(?<match>...)`; the tmux-fingers patterns port as is.
  Tests a regexp cannot state live in `patterns.checks` by name: `sha`
  and `ipv6` must stand alone and contain a digit, so `feedback` and
  `Pane::Annotate` are not matches.
- The `format` method's `unwrap = true` joins soft-wrapped rows; the VT
  format with it returns fewer rows than the screen has. `inpane.Begin`
  turns auto-wrap off, so painting must stay row by row from the wrapped
  VT; the unwrapped text is used only to mark `Line.Wrap`. A row joins the
  next when it is exactly the pane width and the unwrapped text contains
  both rows run together. A row wrapped before a wide character ends one
  cell short and does not join.
- The `format` method returns the scrollback and the screen. Its VT keeps
  every screen row; its text drops blank rows at the foot of the screen,
  so the last `rows` lines of the text reach into the scrollback. A
  current capture compared against the painted frame must be VT: from
  text, `alignRows` found no match on a half-empty screen with
  scrollback, and restore repainted nothing.
- Rex's emulator is Ghostty. With grapheme clustering (DEC mode 2027) off,
  it sizes each code point on its own: a regional indicator, half of a
  flag, is 2 columns, so a flag is 4. `go-runewidth` says 1, and `cellsOf`
  overrides it. The terminal block's `modes` method reports mode 2027 as
  `grapheme_cluster`; it was off in every pane measured. With it on,
  Ghostty would draw a flag pair or a ZWJ sequence as one 2-column cell,
  and the parser does not handle that yet.
- A key mode belongs to one client, and a mode binding must name an
  action; a Lua function binding is not allowed in a mode. The action's
  `ctx` does not say which key ran it, so each hint key needs its own
  binding with `{ key, mod }` args. The app lists every binding of the
  active mode as a row of title plus args, so the mode binds only the
  letters and modifiers that can be pressed. No setting hides that
  panel. The user default `Workspace.showsKeySequenceHints` in
  `com.superlogical.rex`, Settings > Terminal > Keyboard > Show key
  sequence hints, hides only the panel for a pending prefix such as
  Ctrl+B, and the app reads it at launch only. Tested 2026-11-07: with
  it off and the app relaunched, the mode list still shows.

## Rules

- Every pattern change needs a case in `internal/patterns/patterns_test.go`.
- Styles name palette slots (`38;5;N`, `48;5;N`), never hex colors, so
  they follow the theme.
- No shell in Lua strings; the Lua side only builds an argv.
- Options are declared once in `defaults` in `init.lua` and documented in
  `README.md` under Configuration, with the same names and defaults.
- Keep tmux-fingers' names and defaults where Rex allows it.
- Tested against Rex for Mac 0.1.0 (1825), CLI `0.0.1793741575-g1b6654`.
  Rex is in beta; recheck the facts above when a build changes behavior.
