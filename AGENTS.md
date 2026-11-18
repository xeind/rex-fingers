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
   fingers.setup{
     binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
   }
   ```
   `binary` must be an absolute path. Set `window_key = false` if the
   user wants one key only.
4. `rex config check`, then `rex config reload`.
5. Confirm with `rex keymap | grep rex_fingers`: `alt+f` for the focused
   pane and `alt+shift+f` for every pane of the window.

The defaults since 2026-11-18 are `overlay = "pane"`, `window_key =
"alt+shift+f"`, and `follow_scroll = false`. Pane mode was opt-in before:
it used a key mode, Rex for Mac showed a popup listing the mode's
bindings, and the user turned it down 2026-11-07. It now binds function
keys and no mode, and the user confirmed on build 2018 that no popup
shows. It is the default because it hides no rows and is the path the
live suite covers. `follow_scroll` stays off: it takes the clipboard for
half a second, loses a copied image, and needs the server on the app's
machine.

Every option and its default is in `README.md` under Configuration. A
walkthrough with a check per step is `docs/CONFIGURING.md`. Unknown option
names make `setup` error, so read the list before adding one.

## Change the keys or customize it for a user

Ask what the user wants, then change only the `fingers.setup{ ... }` call
in their Rex config. For each change:

1. Before you pick a key, run `rex keymap` and check that nothing else
   uses it.
2. Edit the option. Show the user the diff.
3. Run `rex config check`. Fix any error before you go on.
4. Ask the user before `rex config reload`.
5. Run `rex keymap | grep rex_fingers` and tell the user which keys are
   bound.

| The user wants | Set in `setup` |
| --- | --- |
| A different key | `key = "alt+g"`. Any Rex key or sequence works, such as tmux-fingers' `"ctrl+b>shift+f"`. |
| Another key for every pane, or none | `window_key = "alt+g"`, or `false` |
| No key from the plugin | `key = false`, then `rex.bind("alt+f", "rex_fingers")` in their own bindings |
| Hints in a floating pane that never writes to their pane | `overlay = "float"`. It hides two rows. |
| Hints on the rows the focused pane is scrolled to | `follow_scroll = true`, with `overlay = "pane"`. Tell them the cost: copy mode flashes, the clipboard is saved and restored as text, and the server must run on the app's machine. |
| Other hint letters | `keyboard_layout = "colemak-homerow"`, from `rex-fingers layouts`; or `alphabet = "asdfghjkl"`, which wins over the layout |
| Hint on the other end of a match | `hint_position = "right"` or `"gutter"` |
| Other colors | `hint_style` and `highlight_style`, SGR with palette slots such as `"1;38;5;0;48;5;13"`. Use slots, never hex, so the theme applies. |
| No dimming, or a flash on a pick | `dim = false`; `flash = true` |
| Other actions per modifier | `main_action`, `shift_action`, `ctrl_action`, `alt_action`: `:copy:`, `:paste:`, `:open:`, `""`, or a shell command that reads the match on stdin |
| Fewer or more patterns | `patterns = { "path", "url" }`; `extra = { [[\bISSUE-\d+\b]] }`; names from `rex-fingers patterns` |
| One key for one job | `rex.bind("alt+u", "rex_fingers", { patterns = "url,markdown-url" })`, with `key = false` for the plain one if wanted |
| A remote Rex server | `copy_command = "ssh my-laptop pbcopy"` |

Layouts leave out `c`, `i`, `m`, `q`, and `n`, as tmux-fingers does. A
tmux-fingers user finds every option mapped in
`docs/TMUX-FINGERS.md`.

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
3. Stores the run in `state[block_id]`. There is no key mode. `setup`
   binds each hint letter, plain and with each modifier whose action is
   set, as a Lua function through `rex.bind(key, fn)`; Rex cannot bind a
   key while an action runs (see the facts below). With a run on the
   focused pane, `press` runs `rex-fingers press --tty T --typed ... --key
   K` and returns `true`; the program repaints, or on a full hint
   restores the pane and runs the action. `Escape`, `q`, and `Ctrl+C` run
   `finish` with restore. `Enter`, `Space`, and the letters that are not
   hints return `true` and do nothing, so they do not type into the pane.
   With no run, every one of these functions returns nothing and Rex
   sends the key to the pane. `ctrl+b`, `key`, and `window_key` stay
   unbound, as they start sequences. The action `rex_fingers_cancel`
   restores the pane from the palette or the CLI. The trigger key
   restores a run that is up.
4. A run can end without restore: a config reload or an app restart
   drops the Lua record of the run, and the hints stay painted, and the
   keys pass through. So `draw` writes
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
   and the cursor is left where the program put it. `press` still picks:
   a letter on screen names the same match in the capture. It skips the
   repaint after a first letter, which would cover the new output, and
   the flash, and reports `changed = true`. Before, it refused every pick
   on a moved pane, and in window scope one pane with a spinner, such as
   Claude Code, blocked every copy. `scripts/test_in_pane.sh` has four
   cases for it. Rows that scrolled into the scrollback keep the hints:
   no tty write reaches the scrollback, and the terminal block has no
   method that edits it. Painting on the alternate screen would keep the
   scrollback clean but discard what the program prints during the run,
   so the limit stays and the README states it. Reproduce with a pane
   that prints a line every 300 ms: draw once scrollback exists, wait
   3 s, cancel, and `rex capture --format=text` shows hint letters on the
   rows that scrolled off.
6. A pane scrolled up shows its scrollback, and the steps above hint its
   live screen, out of view. With `follow_scroll = true` the app copies
   the rows each pane shows and the program takes them from the clipboard,
   since Rex reports no scroll position (`follow_scroll` in `init.lua`,
   `await.go`):
   - `start_panes` with `await = 1` runs `draw --begin --await-views 1`
     with one `--pane` spec per pane, the focused pane first. That call
     saves the clipboard to a file, puts a marker text on it, writes
     `$TMPDIR/rex-fingers-<tty>.wait` per pane, starts a copy of itself in
     its own session, and returns the ttys at once. Lua stores the run
     then.
   - Lua queues `pane.copy_mode.enter`, `move screen_top`, `select line`,
     `move screen_bottom`, and `yank` with `rex.client.queue`, then
     `enter`, `select char`, and `exit`. The app runs them after the
     action returns. The last three drop the selection the yank leaves on
     screen, a grey box over the whole pane; `enter` and `exit` alone do
     not.
   - The background copy polls the clipboard. A text other than the marker
     is the rows of the next awaited pane, in the order of the specs; it
     writes them to that pane's view file. An empty clipboard counts only
     when it stays empty, since a writer clears before it fills. After the
     awaited panes, or 3 s with no copy, it puts the saved clipboard back
     and draws.
   - Only the focused pane is awaited. Copy mode acts on the focused pane
     alone, and in window scope the other panes are hinted on their live
     screens. Reading every pane with `pane.focus_next` between the copies
     worked through `rex -C APP do`, where the CLI runs the queue one
     action per call: 1.2 s for four panes. On a key press the app runs
     the whole queue at once, the focus has not moved when the next copy
     runs, and every copy was of the first pane. `--await-views N` still
     takes a count, for a caller that can pace the copies.
   - Until the frame is up, `press` finds the wait file and no run file
     and ignores the key. `restore` deletes the wait file, which calls the
     run off: the background copy then draws nothing and deletes the
     captures. A pane with no run file is not repainted.
   The first version had the app send a key, `f19`, back to a Lua key
   function after each copy. A key sent to a pane moves a scrolled pane
   to its last row, so the pane flickered to the bottom before the frame
   came up and stayed at the bottom after the run. Nothing calls back
   into Lua now.
   `draw --begin` looks the rows up in the capture (`paneSpec.locate`,
   `findOffset`), compared without spaces or line breaks, since the copy
   joins wrapped rows and drops blank rows at the foot. A match above the
   live screen sets `offset` and `alt`; none means the live screen. The
   same rows can stand in several places, as after a command run twice;
   such pages read the same, so the one whose rows line up with the copied
   lines is taken, then the nearest. Until 2026-11-18 several matches meant
   the live screen, and a scrolled pane then looked as if it had jumped to
   the bottom. An `alt` pane is painted on the alternate screen
   (`inpane.BeginAlt`), which shows wherever the pane is scrolled, and
   `inpane.RestoreAlt` leaves it with no repaint. `runState` carries
   `offset` and `alt` to `press` and `restore`. A scrolled pane loses what
   its program prints during the run; that is the cost item 5 names, paid
   only by panes that are scrolled. A call with origin `api` or `cli`,
   such as `rex do` from a shell, has no app and hints the live screen;
   `follow_scroll=true` on the call overrides that, for `rex -C APP do`.

The in-pane subcommands print one JSON object each. `draw` returns the
tty, the hint count, and `scrolled`, the number of panes on the alternate
screen; `press` returns `done`, `typed`, and `paste`.

The `scope = "window"` argument, bound by `window_key`, always takes the
in-pane path, whatever `overlay` says: a float covers one pane, and only
the in-pane keys let one key reach every pane. So the keys are bound for
both overlays. With it, the action
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
| `pane.go` | CLI: `draw`, `press`, `restore` for `overlay = "pane"`, one pane or several `--pane` specs; each prints one JSON object; `runState` and `recover` for a run left without restore; `locate` and `findOffset` for the page a scrolled pane shows |
| `internal/inpane` | `Begin` and `Restore` sequences, `BeginAlt` and `RestoreAlt` for a scrolled pane, `TTY(pid)` to find the pane's tty |
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
go test ./...                       # unit tests: screen, patterns, hints, CLI, docs
sh scripts/smoke_test.sh            # gofmt, vet, build, -race, 10 s fuzz; no Rex needed
sh scripts/test_in_pane.sh          # live cases; opens Rex sessions, overwrites the clipboard
go vet ./... && gofmt -l .
go build -o bin/rex-fingers .       # bin/ is gitignored
rex config check && rex config reload
rex do rex_fingers -b "$REX_BLOCK"  # open the overlay on this pane
rex server logs | grep -i lua       # Lua errors the check misses
```

## Test loop without a screen

An agent cannot see the overlay. Drive it through the Rex CLI. This loop
reads the floating pane, so the loaded config needs `overlay = "float"`;
`scripts/test_in_pane.sh` covers the default, `overlay = "pane"`:

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

A headless session has no app, but `client.key` sends a key press to the
server's key functions, so it can press a hint. The `keys` case in
`scripts/test_in_pane.sh` does that. `testdata/width-cases.txt` holds one
line per character-width case:

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

There is no `fingers` key mode. `rex keymap --json | jq '[.bindings[] |
select(.keys[0] | startswith("fingers/"))] | length'` must print 0, and
`.modes` must not list `fingers`. Do not use `grep -c fingers/`: it also
matches the source column, `rex-fingers/lua/...`.

To press a key headlessly, call `client.key`; `rex api call -s $S client.key
'{"session_id":"'$S'","block_id":"'$B'","key":{"key":"escape","mods":"","consumed_mods":"","text":"","unshifted_codepoint":27,"composing":false,"action":"press"}}'`
prints `handled`. It is `true` when a key function took the key, and
`false` when the key went on to the pane, where it is typed. Names are W3C
codes in snake_case: `key_a`, `enter`, `escape`.

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
`rex do rex_fingers -s $S -b $B scope=window` draws the hints and stores the
run. The hints stay until a key press, `rex do rex_fingers_cancel`, or the
trigger key restores the pane.

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
- `rex do rex_fingers scope=window` from the CLI draws hints and stores the
  run; no client is needed, since nothing enters a key mode. Until 2026-11-18
  the action queued `client.mode.enter`, which the app refused from the CLI.
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
- The plugin once used a key mode, `fingers`, with one `rex_fingers_key`
  binding per letter and modifier. A mode binding must name an action, and
  the action's `ctx` does not say which key ran it, so each key needed its
  own binding with `{ key, mod }` args. The app listed every binding of the
  active mode as a row of title plus args, 52 rows and "26 more", and no
  setting hid that panel. `Workspace.showsKeySequenceHints` (Settings >
  Terminal > Keyboard > Show key sequence hints) hides only the panel for a
  pending prefix such as Ctrl+B. Tested with it off on builds 1825, 1932,
  and 2018 (2026-11-18): the mode list still showed. `rex.mode` takes only
  `exclusive`, and no `rex.action` field hides a binding.
- Function key bindings, tested 2026-11-18 on build 2018 through
  `client.key`: `rex.bind(key, fn)` at config load runs `fn(ctx, ev)` with
  `ctx.block_id` and `ev.keybinding` (`z`, `shift+z`, `ctrl+y`). Returning
  `true` takes the key; returning nothing sends it to the pane. Binding
  from inside an action or a key function reports success but binds
  nothing, so every key is bound at load. Only bound keys reach Lua; the
  cost is about 2 ms on the server. A key function that errors or runs
  over 5 seconds drops the key. `rex.client.queue` is for actions only.
- Scroll position, tested 2026-11-18 on build 2018. No block method,
  `session.view` field, or terminal event reports it, and `rex.on` handlers
  in `init.lua` do nothing in this build. The app's copy mode reads the
  viewport: a line selection from `screen_top` to `screen_bottom`, yanked,
  puts the visible rows on the clipboard. App actions act on the focused
  pane and ignore a `block_id`. Queued app actions run after the action
  returns, and the app cannot run a config action from the queue
  ("does not offer"); `pane.send_key` with a key bound to a function is
  the one way back into Lua, and that function's `ctx.block_id` is the
  focused pane. `pane.send_key` moves a scrolled pane to the bottom, as
  any key does, which is why the plugin no longer uses it. Entering and
  leaving the alternate screen alone keeps the scroll position: a pane
  scrolled to a row was at that row after a draw and a restore with no
  key sent. `ctx.client_id` is set for `rex do` from a shell too, with
  `origin = "api"`; a key press has `origin = "key"`, so the origin tells
  the two apart. The CLI needs Remote Control on in the app to run app
  actions; a key press ran the queued copies with the plugin as it was on
  2026-11-18. With the key sent back, hints were up about 1.2 s after the
  trigger for one pane and 4.8 s for four; with the program waiting on the
  clipboard, 580 ms for one pane and 1.2 s for four. The time is the
  app's: each copy-mode action took 120 to 200 ms and `pane.focus_next`
  400 to 1000 ms through `client.perform`. `pane.focus_next` wraps and is
  back at the first block after one step per block.
  In the app with `overlay = "pane"` (build 2018, 2026-11-18), the user saw
  no popup.

## Rules

- Every pattern change needs a case in `internal/patterns/patterns_test.go`.
- Styles name palette slots (`38;5;N`, `48;5;N`), never hex colors, so
  they follow the theme.
- No shell in Lua strings; the Lua side only builds an argv.
- Options are declared once in `defaults` in `init.lua` and documented in
  `README.md` under Configuration, with the same names and defaults.
- Keep tmux-fingers' names and defaults where Rex allows it.
- Tested against Rex Beta for Mac 0.1.0 (2018), CLI `0.1.0-preview.15`.
  Rex is in beta; recheck the facts above when a build changes behavior.
