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
| `sh scripts/test_host_pane.sh`, nushell 0.116.1, 2026-11-24, at `e3d4898` | passed 65, failed 0 |
| `sh scripts/test_host_pane.sh`, with the agent, nushell 0.116.1, 2026-11-24 | passed 82, failed 0 |

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
`rex.block.call` read the host's pane. Before the host check it failed
with `ps -p 926039: exit status 1`, the pid of the host's shell. The hint
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
A program that fails at once leaves its block open, so the plugin closes
the writer itself.

**The terminal's `write` method reaches a program in raw mode
unchanged.** A writer running `stty raw -echo; cat > TTY` copied ten
frames in order, and a 200 KB frame in one call. A process argument over
128 KiB fails on Linux with `argument list too long`. Used by `forward`
in `init.lua`. Tested 2026-11-22.

**A pane's `child` on a Mac is `/usr/bin/login`**, whose tty is the
pane's; on Linux it is the shell. The writer waits for its own marker,
not for a process name.

The facts below were tested 2026-11-23: CLI and servers
`0.1.0-preview.22`, the plugin in a second server's config with 280 ms
added to every Rex call, and from the app on a pane of the Ubuntu host;
the app's build was not recorded. `scripts/test_host_pane.sh` repeats
the server part.

**Rex stops an action at 30 seconds and runs no more of its Lua.** The
caller gets `context deadline exceeded`. The same happens on a config
reload (`context canceled`) and when the caller goes away, as with `rex
do --timeout 5s`. A `pcall` around the work does not run. The reader,
the writer, the painted hints, and the Lua record of the run stay. Used
by the guards of `host_run`.

**Rex runs one action of a config at a time.** Ten `rex do rex_fingers`
at once, each on its own connection, ran one after another. In the app,
with a run stopped at 30 s, three more `client.action` calls for
`rex_fingers` failed one by one in the two seconds after it, each with
the block of the stopped one, by then closed (server log, 2026-11-23
21:01).

**While an action is alive, a key bound to a function waits, and is lost
after 5 seconds.** `client.key` for a bound key, to a pane of another
session, returned `dispatching client key to the configuration file:
context deadline exceeded` when the run ended 12 s later, and the key
was not typed. An unbound key and `rex send` were not delayed. In the
app, with a host run alive for 15 s, a bound key sent to a local pane
failed after 15.7 s; during runs of 4.5 s it arrived 4.2 to 4.7 s late.
The Rex docs say the server keeps handling keys while an action waits
for `rex.exec`; it did not.

**A float must lie inside the window with a positive size.** Bounds
outside it or of size zero are refused, and a minimized float loses the
focus. One of 0.001 by 0.001 keeps the focus and reads keys. A block
with `exit = { on_completion = true }` closes its float when its program
ends, and the focus returns to the pane under it. Used by `open_reader`.

**Rex gives the focus only to a block placed in a window.**
`session.focus_block` on a detached block failed with "not found in any
window" (a throwaway server, 2026-11-24, CLI and server
`0.1.0-preview.22`). With the two limits above, the key reader cannot be
hidden: it must be a float on the screen, and the app shows it as a pane
named after its program, `hexdump`.

**Rex takes the foreground program at the time a title arrives as the
one that set it.** On the Ubuntu host, `sh -c` that wrote OSC 2 and then
did `exec hexdump` gave `title` an `owner` named `dash`, with `hexdump`
in the foreground under the same pid. With the title written 0.3 s
later by a background subshell, the `owner` was `hexdump`. The API notes
say to compare the owner's name as well as its pid, so a title whose
owner has another name may count as stale. Used by
`inpane.ReaderCommand`. Whether the app then shows the title in place of
`hexdump` was not seen. Tested 2026-11-24, CLI and servers
`0.1.0-preview.22`.

**A run on the host, at 280 ms a call:** the reader was up 0.8 s after
the trigger, the hints showed at 3.0 s, and the pane was back 1.3 s
after the key. An idle run ended at 16.4 s. That run made 12 calls
before the hints. With 10, tested 2026-11-24 from a `rex do` script, the
second run in one Lua state: reader up at 0.6 s, hints at 2.3 s, the
pane back and the reader closed 1.0 s after the key, the action over
after 3.8 s. The two calls cut are `pwd`, which a host run does not
need, and `session.list_blocks`, the search for a leftover writer, made
only on a session's first run and after a run that died.

The facts below were tested 2026-11-24: CLI and servers
`0.1.0-preview.22`, Rex for Mac snapshot 2104, a throwaway server on
this machine, and the Ubuntu host.

**Every block has `REX_BIN_DIR`, the directory of its server's `rex`.**
On the Ubuntu host it was `~/.local/bin`; on a Mac it is inside
`Rex Server.app` and holds spaces. A hidden block there ran
`"$REX_BIN_DIR/rex" do -e SCRIPT`: the script was on the screen 110 ms
after `session.new_block`, and has `io`, `os`, `rex.exec`, `rex.sleep`,
and the session and block calls. One `rex block call` from a block took
85 ms on the host; a call inside a running script is local. Used by
`inpane.AgentCommand`.

**`REX_SERVER` is empty in a block of a server started with
`--listen`**, and the `rex` in that block then reached the default
server of the machine. `server.status` lists the server's `listeners`;
the primary one with transport `unix` has the `endpoint` a program on
the host uses. Used by `remote_host`, which keeps it in
`host_endpoints`.

**A script under `rex do` reads its block's terminal with `io.read`.**
With the terminal raw, a 260 KB message sent with the terminal's
`write`, escapes and wide characters in it, arrived whole after a line
that gave its length. 64,000 characters the script printed as one word
came back whole from `format` with `unwrap`. A float the script opened
with `session.new_layer` took the focus, and the focus returned to the
pane when the script closed it.

**`rex do` does not end on SIGTERM while its script waits in
`io.read`.** The block stayed until SIGKILL. A block with `exit = {
on_completion = true }` closes when its program ends or is killed, a
hidden one too.

**A config action cannot push a script to the host.**
`client.set_script` loads Lua into a server for the calling client, with
no file there. From a `rex do` script it worked: the pushed script has
`rex.call`, `rex.block.call`, `rex.session`, and `rex.bind`, no `io`,
`os`, or `rex.exec`; it took a bound key, and survived
`server.config_reload`. `rex.action` in it gave the warning "Actions in
a client script run from a later version of Rex". From a config action
in the app, on a host's tab, the call failed in 10 ms: "the
configuration file cannot call client.set_script: a key handler cannot
replace the script of the client whose key it is handling".
`server.config_reload` failed too: "a key handler cannot reload the file
it is running from". A script the CLI pushes belongs to the CLI's
client, so the app's keys do not reach it.

**`blob_read` returns 48 KiB unless `max_bytes` asks for more.** A 5.4 MB
VT dump took 125 calls, and 8 with `max_bytes = 983040`. `blob_read`
takes any `offset`, so the end of a dump is one call: a small `format`
for the `id` and `total`, then `offset = total - N`. Each row of the VT
starts with `ESC[0m` and its own colours. One `format` call without
`max_bytes` fails above 1 MiB with `too_large`. Used by `read_format` in
`init.lua` and `tail` in `agent.lua`.

**A run on the Ubuntu host through the agent**, measured from two `rex
do` scripts on this machine, one with the plugin and one that watched
the pane and typed the hint. A call took 170 to 230 ms. Three runs each:

| | Agent | Reader and writer |
| --- | --- | --- |
| Trigger until the hints were seen | 0.77 to 1.03 s | 2.19 to 2.69 s |
| Key sent until the pane was seen back | 0.44 to 0.46 s | 0.89 to 0.91 s |
| Key sent until the action was over | 0.58 to 0.63 s | 1.43 to 1.56 s |
| Whole action | 1.39 to 1.60 s | 3.72 to 4.19 s |

The watcher sees a change one call late, and its key is a call of 220 ms
itself, so 0.44 s is the least it can report. On the throwaway server,
with 280 ms added to every call of the plugin and none to the watcher,
the pane was back 73 to 82 ms after the key. Not measured from the app.

**The app logs to the macOS log under `com.superlogical.rex`.** `log show
--predicate 'subsystem == "com.superlogical.rex"'` gives its lines by
category: `Actions` names each queued client action it skipped and the
config action that asked for it; `Network`, `RexSync`, and `Sizing` name
blocks it lost. The server's own log records a `client.action` only when
it fails. Found 2026-11-24, app snapshot 2104.

**A key mode did not turn on in a host's tab.** With `fingers` declared
in this machine's config and 138 `fingers/` bindings in `rex keymap`, a
config action run from a key in a host's tab queued `client.mode.enter`.
No panel showed, and `s` reached the shell. On each of six runs the app
logged `Skipped client.mode.exit, which rex_fingers asked for: it isn't
available here`, and nothing about the enter. The host has no config:
its `rex keymap` is empty, and the app fetched no keymap from it during
the runs, the last being half an hour before. Whether the app looks for
the mode in the host's keymap was not proven; a mode entered by its own
key in a host's tab was not tried. Tested 2026-11-24, app snapshot 2104.

**The app stopped answering three times during a run on the host.** On
2026-11-24 at 10:48:17, 10:50:05, and 10:57:26 it sent this server no
`client.reply` from one second on, 9 to 14 s into a run. In that same
second each time it sent `data.connect` and `block.method` for the two
panes of a local session, as it does when the user goes to that session.
The first time it answered again after 21 s, and the action had failed
at 29.7 s with two calls past their 10 s. The other two times the
process ended 5 and 7 s later, with exit status 0 and no crash report,
and a new one started 0.4 s after. Three runs left alone to their 15 s
did not stop it. The API notes of `client.action` say an app's actions
and its focus and layout changes run one at a time on its connection,
and the run's action waits on the app for every call to the host; that
the two wait on each other is a reading, not a finding. The owner does
not recall whether they changed tabs. App snapshot 2104; it was not
tried again on purpose.

**Once, the first key after the app reconnected was typed into the
shell**, with the hints up: the run lasted its 15 s. Seven seconds
before, the server log has `websocket control ended unexpectedly`. The
next four runs took 4 to 7 s, and the user saw them pick. Cause not
found.

**OSC 52 written into a host pane's tty** reached the host's server
(`clipboard_read` returned it), but not the Mac's clipboard, with the
pane not focused. Copy runs `pbcopy` here. Tested 2026-11-22. On
2026-11-24, app snapshot 2104, it did reach the Mac's clipboard, twice:
written by another process into the tty of a pane of a session the app
did not show, and again with the app on that session. Nothing uses it
yet; an agent could finish a copy with it and need no answer from this
machine.

The facts below were tested 2026-11-24 in the afternoon: CLI and servers
`0.1.0-preview.22`, app snapshot 2104, the Ubuntu host.

**An action's Rex calls, run from a key in a host's tab, go to the
host; `ctx.client_id` is the app's ID on this machine's server.**
`client.perform` with that ID, called from the action, failed on the
host with 404, three times (host log, 15:46). The app is a client of
both servers, with a different ID on each. A program the action starts
with `rex.exec` runs here, and its `rex -C ID do` reaches the app
through this machine's server. Used by `host_view` in `init.lua`.

**The app performs copy mode on a host's pane, and during a live
action.** `rex -S cordovan -C colter do pane.copy_mode.*`, with the
app on a session of the host and the pane two pages up, put rows 110 to
173 of 300 on the Mac's clipboard. From a run started by a key, with
the action alive, `rex-fingers view` got the rows and the hints showed
on the scrolled page (the owner, 15:53). The eight steps took 1.38 s
from a shell. The app must allow remote control.

**`pane.send_key` with a key bound to a config action starts no action
in a host's tab.** `rex -S cordovan -C colter do pane.send_key
key=alt+f` typed the key into the pane. So only a real key press tests
a run in the app.

**The app's own `title` for a float is the terminal's title when its
owner is the foreground program.** With the reader's title written
0.3 s after its start, the app showed the box as `fingers` (the owner,
15:53).

**A key mode turns on in a host's tab when the app is asked through
this machine's server, and not through the host's.** With the app on a
session of the host: `rex -C colter do client.mode.enter name=resize`
succeeded and `client.mode.exit` then worked once, as on a local tab.
`rex -S cordovan -C colter do client.mode.enter name=resize` also
returned without an error, but `client.mode.exit` after it failed with
"isn't available here right now": no mode was on. An action run from a
key in a host's tab sends its queued client actions by way of the host,
which explains the mode that did not turn on in the morning. The docs
say a device follows its home server for shortcuts. Whether a key bound
in the mode then runs its action in a host's tab was not tried; it
needs a real key press. Nothing uses this yet: with it, a run on a host
needs no reader and no live action.

**The app turns on a key mode only after its own reload.** A mode
added to the config and loaded with `rex config reload` was in `rex
keymap`, with its bindings, and the app's new key bindings worked; yet
`client.mode.enter` with its name turned nothing on, as with a name
that does not exist. Seven modes were tried: bound to app actions or to
config actions, exclusive or not, with and without a key bound to
`client.mode.enter`. Only `resize`, in the config since the app
started, turned on. After `rex -C colter do client.config.reload`, the
app's own action, every one of them did. So after a change to the
modes, reload from the app. This, and not only the route through the
host, kept the morning's `fingers` mode off. Tested 2026-11-24, app
snapshot 2104.

**The app shows a panel for an active key mode, one row for each of
its bindings, and no setting hides it.** With `rex_fingers` on, 69
bindings, it covered a third of the window (the owner's screenshot,
17:41), with "Show key sequence hints" off (`defaults read
com.superlogical.rex`: `Workspace.showsKeySequenceHints = 0`). In that
mode, entered through this machine's server, the keys ran their config
action in a host's tab: Escape and the trigger closed a run. The owner
turned the mode down for the panel; it is `host_keys = "mode"`.

**Copy mode shows what it copies, with the copy animation off too.**
`pane.copy_mode.copy_to_line_end` at the top, the middle, and the last
row of a pane, 12 steps in 0.6 s through `client.perform`, gave the
three rows with no selection made. The owner saw the three rows
highlighted, and again with `client.setting.terminal.selection.
play_animation_on_copy` set to false. So no copy of the visible rows is
unseen, and Rex reports the scroll position in no other way.

**A program the server starts for an action from a key has no `rex` on
its `PATH`.** `rex-fingers mode` failed with `exec: "rex": executable
file not found in $PATH` on every run from the app (17:32), and worked
from a shell. Used by `binary` in `internal/rexcli`, which looks in
`REX_BIN_DIR` and the places Rex installs it.

**`client.set_script` is refused from an action the palette runs too.**
With `ctx.origin` `palette`, on a host's tab, the call failed twice
with the same words as from a key: "a key handler cannot replace the
script of the client whose key it is handling" (17:49). So the app's
script on a host cannot be set from the config at all, and the hint
letters cannot be function bindings there.

**`rex.ui.input` and `rex.ui.pick` wait without holding up keys or
other actions, and the wait does not count toward the action's 30
seconds**, by the Rex docs (`customize/actions`). The command palette
shows the question, and the answer needs Return. Read 2026-11-24, not
tested.

**The app stopped answering twice more, each time in the second it
loaded this machine's session.** At 15:46:43 and 15:53:14, 2 and 11 s
into a run on the host, it sent `data.connect` and `block.method` for a
local session, then no `client.reply`. It logged "Application
termination requested" 6 and 11 s later and a new process started
within a second. At 15:53:24 the run failed first, with "the run on the
host ended without an answer". No crash report. That makes five times
with the same order of events.

The facts below were tested 2026-11-24 in the evening: CLI and servers
`0.1.0-preview.22`, app snapshot 2104, a throwaway server on this
machine, and the Ubuntu host. `host_keys = "substitute"` is built on
them; the tests in the app were made with a script that showed a
stand-in block, before that option existed.

**`client.set_script` is refused by its name, on every route.** A
config action got the same words from `rex.call` and from
`rex.server(NAME).call`, with `server` set on the `client.action`
request and without, in 29 ms and with no call sent to the other
server.

**A block can take a pane's place, and the pane can leave the window
and come back.** `session.swap_blocks` needs both blocks in a window. A
new block in a small float, swapped with a tiled pane, took the pane's
place and kept the focus it had; the pane went to the float. No split
changed. `session.detach_block` on the pane then closed the float, and
the pane ran on, detached: it took typed text and printed. To come
back: a second small float with a dummy block, `session.move_block` of
the pane beside the dummy, `swap_blocks` with the stand-in, and
`session.close_layer`. The pane's VT was the same before and after. A
key written to the focused block reached the stand-in and not the pane.

**A block's size follows its owner.** A `rex do` script that called
`session.own_block` with `take` for the pane and the stand-in, and then
the terminal's `resize`, kept both at the pane's 94 by 64 while the app
asked 1 by 1 for the one in the small float. After
`session.disown_block` the app owned the pane again, at 94 by 64. The
terminal's `size` lists each client's last request under `desired`,
with a time, and only while that client is connected: a request made
with `rex block call` was gone from the list at once. A block with no
owner took that request's size. Used by `asked` and `substitute_turn`
in `agent.lua`.

**The app loads a new block of the host in about 1.3 seconds.** That is
the time from `session.new_layer`, called on the host, to the app's
first size request for the block: 1286 and 1305 ms for a new block, 1358
and 1315 ms for a detached pane placed again. A call from this machine
to that host took 170 to 230 ms. A stand-in swapped into the pane's
place 30 ms after it was made showed an empty pane first, and then its
frame (the owner). Held in its small float until the app had asked a
size, and 150 ms more, it showed no empty pane; the float was on screen
for 1.5 s at each end, and the owner called that acceptable.

**A real key press in the app reached the stand-in**, 2.3 s after the
swap, and did not reach the detached pane. The script on the host had
set the focus; no call went through the app.

**A stopped program still takes the first key from its tty.** On this
Mac, with every process on a pane's tty stopped by `SIGSTOP` and the
tty set raw, a `dd` on the tty read `bc` of `abc` and `e` of `de`: zsh,
stopped in its read at the prompt, got `a` and `d` when it continued.
With `cat` in the foreground and the tty cooked before the stop, `dd`
read nothing. With `sleep` in the foreground it read every key. So the
pane's own tty cannot be the key reader. Not tried on Linux.

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
