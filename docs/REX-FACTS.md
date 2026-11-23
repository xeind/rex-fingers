# Facts the code depends on

Tested against Rex Beta for Mac 0.1.0 (2027), CLI `0.1.0-preview.20`:
both live test scripts ran on it 2026-11-18. The facts below that name
build 2018 were recorded as such in earlier sessions and not rechecked;
the app reported 2027 on that day. Where an entry says "not recorded",
nobody wrote down the build or the date.

Rex is in beta. When a build changes behavior, recheck the entry, and
change the date, the build, and the code it names.

## Test runs

2026-11-19, the commit after `67c628e`, CLI and server
`0.1.0-preview.20`, macOS on arm64. The runs are headless, so no app
build takes part; the app bundle on disk reported build 2033.

| Run | Result |
| --- | --- |
| `sh scripts/test_in_pane.sh`, nushell 0.116.1 | passed 29, failed 0 |
| `TEST_SHELL=bash sh scripts/test_in_pane.sh`, bash 3.2.57 | passed 29, failed 0 |
| `TEST_SHELL=zsh sh scripts/test_in_pane.sh`, zsh 5.9 | passed 28, failed 1 |
| `sh scripts/test_resources.sh` | passed 17, failed 0 |
| The float loop of [TESTING.md](TESTING.md), on a server started with `overlay = "float"`, 2026-11-18 | the float opened, `a` copied `/tmp/a`, the float closed |
| `scripts/smoke_test.sh` in CI, Ubuntu and macOS, 2026-11-18 | passed |

Cases that fail now and then, each on "pane text unchanged" unless said:

- "keys" under zsh, every run. The prompt ends with `sudo` and the last
  command. The case presses Escape twice on an idle pane, and that zsh
  setup acts on a double Escape. A fault of the case, not of the plugin.
- "keys" under nushell, in some runs. The mark of the right prompt is
  missing after the run. Cause not found.
- "actions" under nushell, once in three runs: no hint was read from the
  pane. Cause not found.
- "leaks: 50 runs with --await-views" of the resource script: the
  clipboard did not hold the saved text after 1 or 2 of 50 runs, in 2 of
  10 runs of the case. Cause not found; another program may have written
  to the clipboard.

On 2026-11-18, at `08caedf`, zsh failed three more cases, all "pane
output during the mode": a hint letter stayed on the prompt row, where
zsh had added the typed command. `writtenRows` fixed that.

The float in the app, and any run on Linux beyond the smoke test, are not
tested.

## The float

**A floating pane is two rows shorter than the pane under it.** `show`
drops the top two rows. The pane reports 80x24 until the app sizes it, so
`ui.waitForSize` polls up to 1.5 s and `draw` reruns on SIGWINCH.
Tested: Rex for Mac; build and date not recorded.

**A pane whose program exits within a moment stays open.** So the program
lives at least 400 ms, by a sleep at the end of `show` in
`internal/cli/cli.go`.
Tested: not recorded.

**`session.view` nests `split.before` and `split.after`**, each with
`rect` in window fractions; a floating layer's layout is relative to its
bounds. Used by the rectangle lookup in `init.lua`. Tested: not recorded.

## Rex hosts

Tested 2026-11-22: CLI and both servers `0.1.0-preview.22` (`a086db17`),
Rex for Mac snapshot 2083, a Mac on arm64 and an Ubuntu 26.04 arm64 host
added as `ssh://cordovan`, reached over Tailscale.

**A key bound to an action works in a host's tab; a key bound to a
function does not.** With the plugin only in the Mac's config, alt+f in a
session on the host ran `rex_fingers`: its `rex.exec` ran here, and
`rex.block.call` read the host's pane. Before the check it failed with
`ps -p 926039: exit status 1`, the pid of the host's shell. The hint
letters, bound with `rex.bind(key, fn)`, reached the shell: hints drawn
through a hidden block on the host, then `s` typed `s`. Client scripts
run on the server they are loaded on (`client.set_script`), and a key
reaches a Lua function only if one is bound to it there.

**`server.status` gives a `host_id` per server**, the same ID Rex keeps in
`host-id` under `~/Library/Application Support/rex` on a Mac and
`~/.local/state/rex` on Linux. A server started with another
`--state-dir` has its own. Used by `remote_host` in `init.lua`.

**A config action has no `rex.sleep`; a `rex do` script has.** In an
action, `rex.sleep` is nil and `rex.call`, `rex.exec`, `os.clock`, and
`os.time` exist (a probe action on a throwaway server).

**`rex.server(LABEL)` offers `block`, `call`, and `session`, no `exec`.**
A script cannot start a program on a host directly; `session.new_block`
with `options = { shell = "none", command = argv }` starts a hidden one.

## The clipboard

**OSC 52.** Rex for Mac build 1825 did not forward OSC 52 to the
clipboard; build 1859 does, from a pane's tty, within a second. Copy
still uses `pbcopy` or a Linux equivalent and writes OSC 52 too, into the
pane's tty in pane mode, so the app's own copy feedback fires when it has
one. Used by `copyText` in `internal/cli/cli.go`. Tested: builds 1825 and
1859; date not recorded.

## Captures

**`unwrap = true` joins soft-wrapped rows.** The VT format with it
returns fewer rows than the screen has. `inpane.Begin` turns auto-wrap
off, so painting must stay row by row from the wrapped VT; the unwrapped
text is used only to mark `Line.Wrap`. A row joins the next when it is
exactly the pane width and the unwrapped text contains both rows run
together. A row wrapped before a wide character ends one cell short and
does not join. Tested: not recorded.

**The `format` method returns the scrollback and the screen.** Its VT
keeps every screen row; its text drops blank rows at the foot of the
screen, so the last `rows` lines of the text reach into the scrollback. A
current capture compared against the painted frame must be VT: from text,
`alignRows` found no match on a half-empty screen with scrollback, and
restore repainted nothing. Tested: not recorded.

## The terminal

**Autowrap off.** The cursor stays on the last column after a row that
fills the width, so erase-to-end-of-line there takes that cell. `ui.draw`
skips `\x1b[K` on a full row. Tested: not recorded.

**Character widths.** Rex's emulator is Ghostty. With grapheme clustering
(DEC mode 2027) off, it sizes each code point on its own: a regional
indicator, half of a flag, is 2 columns, so a flag is 4. `go-runewidth`
says 1, and `cellsOf` overrides it. The terminal block's `modes` method
reports mode 2027 as `grapheme_cluster`; it was off in every pane
measured. With it on, Ghostty would draw a flag pair or a ZWJ sequence as
one 2-column cell, and the parser does not handle that yet. Tested: not
recorded.

## Keys

**Function key bindings.** `rex.bind(key, fn)` at config load runs
`fn(ctx, ev)` with `ctx.block_id` and `ev.keybinding` (`z`, `shift+z`,
`ctrl+y`). Returning `true` takes the key; returning nothing sends it to
the pane. Binding from inside an action or a key function reports success
but binds nothing, so every key is bound at load. Only bound keys reach
Lua; the cost is about 2 ms on the server. A key function that errors or
runs over 5 seconds drops the key. `rex.client.queue` is for actions
only. Used by the binding loop at the end of `setup` in `init.lua`.
Tested: 2026-11-18, build 2018, through `client.key`. In the app on that
build, with `overlay = "pane"`, the user saw no popup.

**A key mode shows a panel no setting hides.** The plugin once used a key
mode, `fingers`, with one `rex_fingers_key` binding per letter and
modifier. A mode binding must name an action, and the action's `ctx` does
not say which key ran it, so each key needed its own binding with `{ key,
mod }` args. The app listed every binding of the active mode as a row of
title plus args, 52 rows and "26 more". `Workspace.showsKeySequenceHints`
(Settings > Terminal > Keyboard > Show key sequence hints) hides only the
panel for a pending prefix such as Ctrl+B; with it off, the mode list
still showed. `rex.mode` takes only `exclusive`, and no `rex.action`
field hides a binding. Tested: builds 1825, 1932, and 2018, the last on
2026-11-18.

**`rex do rex_fingers scope=window` from the CLI draws hints and stores
the run.** No client is needed, since nothing enters a key mode. Tested:
2026-11-18; build not recorded.

## Scroll position

Tested 2026-11-18 on build 2027. Used by `follow_scroll` in `init.lua`
and by `internal/cli/await.go`.

- No block method, `session.view` field, or terminal event reports the
  scroll position, and `rex.on` handlers in `init.lua` do nothing in this
  build.
- The app's copy mode reads the viewport: a line selection from
  `screen_top` to `screen_bottom`, yanked, puts the visible rows on the
  clipboard.
- App actions act on the focused pane and ignore a `block_id`.
- Queued app actions run after the action returns, and the app cannot run
  a config action from the queue ("does not offer"). `pane.send_key` with
  a key bound to a function is the one way back into Lua, and that
  function's `ctx.block_id` is the focused pane.
- `pane.send_key` moves a scrolled pane to the bottom, which is why the
  plugin does not use it. So does a key the app hands to the pane. A key
  bound to a Lua function does not, though the function declines it and
  the key is typed: the app sends it to the server with
  `client.key.forward`, and the server writes it to the pty. Tested
  2026-11-19 on build 2044 with a pane scrolled up two pages, the first
  visible row read through copy mode. `client.key` with `digit_1`
  (unbound), `key_a`, `space`, and `enter` (bound, declined) left the
  view where it was; `pane.send_key` with `2` and with `a` moved it to
  the bottom. A physical key press was not measured. This holds for
  every key the plugin binds: the hint letters with their modifiers,
  Enter, Space, Escape, `q`, and `Ctrl-c`.
- A key handler bound with `rex.bind(key, fn)` cannot queue an app
  action. `rex.client.queue("pane.send_key", ...)` from one fails with
  "a key handler cannot queue client actions: client actions are
  returned by an invocation of an action". Tested 2026-11-19, app build
  2044, CLI 0.1.0-preview.21.
- A floating layer takes any bounds above zero inside the window. On
  build 2044 the app gave 1 row by 2 columns to a layer 2% wide and 3%
  high, about 145 ms after `session.new_layer`. When its program exits,
  focus returns to the pane that had it. A pane with focus reporting on
  (mode 1004) hears the float take focus and give it back. Nothing uses
  this; `docs/HISTORY.md` says why.
- Entering and leaving the alternate screen alone keeps the scroll
  position: a pane scrolled to a row was at that row after a draw and a
  restore with no key sent.
- `ctx.client_id` is set for `rex do` from a shell too, with `origin =
  "api"`; a key press has `origin = "key"`, so the origin tells the two
  apart.
- The CLI needs Remote Control on in the app to run app actions; a key
  press ran the queued copies with the plugin as it was on 2026-11-18.
- `pane.focus_next` wraps and is back at the first block after one step
  per block.

Timings: with the key sent back into Lua, hints were up about 1.2 s after
the trigger for one pane and 4.8 s for four; with the program waiting on
the clipboard, 580 ms for one pane and 1.2 s for four. The time is the
app's: each copy-mode action took 120 to 200 ms and `pane.focus_next` 400
to 1000 ms through `client.perform`.

## Not Rex

**The user's shell is nushell.** Its line editor holds the tty in raw
mode at the prompt, so output from a background process gets no carriage
returns and staircases across the screen. Not a fingers effect; count
occurrences, not rows, when a test checks such output. Nushell also
renders `echo a b c` as a table.

**macOS checks a freshly built binary on first run**, about 360 ms. Run
it once after building before judging latency.

**Go regexp accepts `(?<match>...)`**; the tmux-fingers patterns port as
is. Tests a regexp cannot state live in `patterns.checks` by name: `sha`
and `ipv6` must stand alone and contain a digit, so `feedback` and
`Pane::Annotate` are not matches.
