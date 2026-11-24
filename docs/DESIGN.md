# Design

How rex-fingers works now. What was tried and dropped is in
[HISTORY.md](HISTORY.md); what Rex does and does not offer is in
[REX-FACTS.md](REX-FACTS.md).

Rex has no API that draws into an existing terminal. So the plugin has two
overlays: a floating pane over the pane, or the pane's own tty.

## The float: `overlay = "float"`

The Lua action `rex_fingers`:

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
and the program calls `rex focus` on the original pane. The float is two
rows shorter than the pane, and `show` keeps the bottom rows.

The key toggles. The action first looks for a floating block labeled
`fingers` in the window (`open_floats`). If it finds one, it writes Escape
into it through the block's `write` method and returns. The program takes
that as a cancel, so the float closes and focus returns as above. Without
this, each press captured the focused float and stacked another over it.

## The pane: `overlay = "pane"`

There is no floating pane. The action:

1. Reads the pane's `modes`, `process`, `pwd`, and `size`, and captures
   the screen as above.
2. Runs `rex-fingers draw --begin --pid P --vt-file F --rows N --cols M`
   through `rex.exec`. The program finds the foreground process's tty
   and writes the dimmed screen with hints straight into it.
3. Stores the run in `state[block_id]`.

The in-pane subcommands print one JSON object each. `draw` returns the
tty, the hint count, and `scrolled`, the number of panes on the alternate
screen; `press` returns `done`, `typed`, and `paste`.

### The keys

There is no key mode. Rex cannot bind a key while an action runs, so
`setup` binds every key when the config loads, as a Lua function through
`rex.bind(key, fn)`. It binds them only when `overlay = "pane"` or
`window_key` is set; a float with no window key binds none.

- Each hint letter is bound plain and with each modifier whose action is
  set. With a run on the focused pane, `press` runs `rex-fingers press
  --tty T --typed ... --key K` and returns `true`; the program repaints,
  or on a full hint restores the pane and runs the action.
- `Escape`, `q`, and `Ctrl+C` run `finish` with restore.
- `Enter` returns `true` and does nothing, so it does not run what is at
  the prompt.
- `Space` closes the run as `Escape` does. Typed during a run, it went
  to the program unseen under a scrolled page and left the hints up. A
  key handler cannot queue the app's `pane.send_key`, which would type
  it and scroll the pane back: Rex lets only an action queue app
  actions. Bound, Space does not return a scrolled pane when idle.
- Nothing else is bound. During a run, a letter that is not a hint or a
  capital with no action types into the pane, and the hints stay up.
- With no run, every one of these functions returns nothing and Rex sends
  the key to the pane.
- `ctrl+b`, `key`, and `window_key` stay unbound, as they start
  sequences.

The action `rex_fingers_cancel` restores the pane from the palette or the
CLI. The trigger key restores a run that is up.

A key bound this way has a cost outside a run. The app sends it to the
server and never hands it to the pane itself, so a declined key is typed
but does not return a scrolled pane to its last row. REX-FACTS.md has the
measurement. That is why only the keys a run needs are bound.

### A run left without restore

A config reload or an app restart drops the Lua record of the run. The
hints stay painted, and the keys pass through. So `draw` writes
`$TMPDIR/rex-fingers-<tty>.run.json`: the capture paths, the pane's
modes, the letters typed, and the text it painted (`ui.Painted`). `press`
rewrites it on a repaint; `restoreAll` deletes it.

The next `draw --begin`, or `restore --recover` from the key or cancel
action with no run on record, reads it (`paneSpec.recover`). When the
pane's text equals the painted text, the old capture moves under the new
paths, the modes come from the file, and `\x1b8` precedes `Begin`.
Otherwise it deletes the old files. `press` and plain `restore` take the
modes from the file too, since the Lua flags carry what was read from the
painted frame.

### A pane that writes during the run

Every paint ends with `\x1b8`, so the program's output lands where its
cursor was. Lua captures each pane as VT (`currents`) before every `press`
and `restore` and passes it as `--current-file`, or `current=` in a
`--pane` spec. It must be VT, not text; see REX-FACTS.md.

When the screen's text differs from the painted text, the pane moved on
(`paneSpec.checkMoved`): `alignRows` finds the scroll offset, only the
rows that still show the frame are repainted (`inpane.RestoreRows`), and
the cursor is left where the program put it. `press` still picks: a
letter on screen names the same match in the capture. It skips the
repaint after a first letter, which would cover the new output, and the
flash, and reports `changed = true`. `scripts/test_in_pane.sh` has six
cases for it.

A row the program wrote into, such as a prompt row with a typed
character, no longer equals the painted row, so it is not repainted
whole. `writtenRows` takes it when it still shows at least half of the
painted row's non-blank cells and at least one hint cell. `Line.Diff`
finds the columns that changed, and `Line.Slice` cuts the other columns
out of the first capture with their styles. The restore writes those
slices and nothing else on that row: it does not clear the row, so the
row loses its hints and keeps what the program added. A row with less in
common is new output, and a row with no hint cell left was redrawn by its
program; both are left alone. A zsh prompt that shows a path is the usual
case: zsh adds a typed character to the row, where nushell redraws the
whole prompt.

Rows that scrolled into the scrollback keep the hints: no tty write
reaches the scrollback, and the terminal block has no method that edits
it. Painting on the alternate screen would keep the scrollback clean but
discard what the program prints during the run, so the limit stays and
the README states it. Reproduce with a pane that prints a line every
300 ms: draw once scrollback exists, wait 3 s, cancel, and `rex capture
--format=text` shows hint letters on the rows that scrolled off.

## Window scope

The `scope = "window"` argument, bound by `window_key`, always takes the
in-pane path, whatever `overlay` says: a float covers one pane, and only
the in-pane keys let one key reach every pane. With a float and no
`window_key` those keys are not bound, and the action raises an error
that names the two settings.

The action does the same for every terminal block of the window at once.
It reads them from `layer.blocks[]` with `creator_name ==
"com.superlogical.terminal"`, puts the focused block first, and passes
one `--pane pid=,vt=,rows=,cols=,cursor=,wrap=` spec per pane.
`ui.NewSet` gives the panes one hint list and `ui.PressSet` finds a pick
across them; `press` returns `pane`, the index of the pane it came from.
A paste goes to the first pane, the one the key was pressed in, and
`--cwd` is its directory. The run is stored under every member block, and
`finish` clears and restores them all.

## A pane on a Rex host

A session on a Rex host runs on the host's server, but the app runs the
actions of this machine's config: `rex.block.call` reads the host's pane
while `rex.exec` runs `rex-fingers` here. The pane's process and tty are
the host's. `ps -p` here finds nothing, or a local process with the same
number, whose terminal the program would paint (issue #1). REX-FACTS.md
has the measurements.

So the action tells such a pane apart first, before it looks up a
process: `remote_host` compares `rex.call("server.status").host_id` with
this machine's, read from the `host-id` file Rex keeps
(`~/Library/Application Support/rex` on a Mac, `$XDG_STATE_HOME/rex` on
Linux). Without that file, or when the call fails, the pane counts as
local, as before. The answer is kept per session. A second server on the
same machine, with its own state directory, has its own ID and counts as
a host; `scripts/test_host_pane.sh` uses that. `overlay = "float"`
raises an error on a host's pane, since the float would run this
machine's binary path on the host.

Nothing is installed on the host. A run there is `host_run` in
`init.lua`. It tries the agent first.

### The agent

Every Rex call from this machine's action is a round trip to the host,
and the calls run one after another. So the run moves to the host, where
they are local. Rex gives every block `REX_BIN_DIR`, the directory of the
host's own `rex`, and `rex do -e` runs a Lua script with the full API,
`io`, and `rex.sleep`. The agent is such a script, `agent.lua` in
`internal/inpane`, embedded in the program. `rex-fingers agent` prints its
argv (`inpane.AgentCommand`), and the plugin starts it in a hidden block
on the host (`session.new_block`, label `fingers-agent`) with the address
of the host's server, the session, and its arguments as JSON. The script
travels in the argv.

The plugin and the agent talk through the agent's block. The agent prints
on its screen; the plugin reads that with `format`. The plugin writes to
the agent's terminal, which is raw; the agent reads that with `io.read`.

1. The agent closes what a dead run left, finds the panes (the window's,
   for window scope), opens the reader with the focus, and reads each
   pane: its tty, size, cursor and wrap modes, and the screen rows as VT
   and as unwrapped text. It prints them as `RXF1<...>`.
2. The plugin writes them to files and runs `draw --begin --agent-file`.
   The program writes one JSON document: each pane's frame and the bytes
   that restore it, every hint with its text, and the action of each
   modifier. The plugin writes it to the agent as `RUN <bytes>` and the
   document.
3. The agent writes each frame into its pane's tty, and reads the
   reader's screen every 20 ms. On the last letter of a hint it writes
   the restores, pastes with the terminal's `write` when the action is
   `:paste:`, closes the reader, focuses the pane, and prints
   `RXF2<...>`: the hint, the modifier, and how the run ended.
4. The plugin, which has polled for that, runs `press` with the hint and
   the modifier, so the copy, the opener, or the action command runs
   here. Then it closes the agent's block.

That is three round trips before the hints show: start the agent, read
the panes, write the frames. A pick costs none before the pane is back.

A message of the agent is one word: each byte but letters and digits is
`%XX`, so the terminal's wrapping and trimming cannot change it. A pane
that printed during the run cannot take the restore the program made at
the start. The agent takes each pane's screen 100 ms after the frame and
again at the end. If they differ, it restores nothing and sends the
screens with `RXF2`; `press` or `restore` works out the rows from them as
on a local pane, and the plugin sends the bytes back as `W <pane>
<bytes>`, then `BYE`. The plugin does not close the agent's block then:
a close could come before the agent has written, so the agent ends
itself and its block closes with it.

A plugin that is gone sends nothing: Rex stopped its action, or the app
no longer passes its calls. So the agent waits 6 seconds for a word, and
then writes the restores made at the start. They draw over what the pane
printed since, which is wrong, and less wrong than hint letters left in
a pane. The wait is a limit on the read: the shell that starts the agent
gives it the path of its own terminal, and `stty min 0 time 60` on that
makes `io.read` return nil after 6 seconds. A body is read with no
limit, since a read that gives up cuts it short. When the plugin got no
answer at all, it leaves the block to the agent too.

The agent ends the run by itself at `host_limit`, and its shell kills it
after ten more seconds, so a run whose action Rex stopped still puts the
pane and the focus back. The next agent closes any reader, writer, or
agent it finds. A trigger that arrives on a dead run's reader runs on the
pane that has the focus once that reader is closed.

Two things a local pane has are missing: the first letter of a
two-letter hint changes nothing on screen, and there is no flash.

The hint letters reached the pane's program, with the hints up, when
the app's own key focus had not followed the session's to the reader.
It was seen on the first run in a new pane. Having the agent hand the
focus to the pane and back to the reader did not cure it. So the plugin
focuses the reader once more after the frames are sent, with a call
that goes through the app, as the reader's own did when the plugin
opened it. The owner then saw the letters pick, 2026-11-24.

One fault remains in the app, with or without the agent; REX-FACTS.md
has what was seen.

- The app has stopped answering, and has exited, during a run, each time
  in the second it loaded the panes of another session. That a change of
  tab or session is the trigger is not proven. The run's action is alive
  until the pick,
  and each of its calls to the host goes through the app. A cure needs
  the action to end once the hints are up, and then another way for the
  keys and the pick to reach this machine. A key mode was tried for that
  and did not turn on in a host's tab; HISTORY.md has it.

A host whose blocks have no `rex` prints `RXF0`, and one whose `rex`
cannot run the script prints an error. The plugin then marks the host
and runs without the agent, below. The action argument `agent=false`
does the same for one run; `TEST_AGENT=false` passes it in
`scripts/test_host_pane.sh`.

### Without the agent

The run has three parts.

**Drawing.** The program runs here with `--remote`: `--tty` names a local
file, and every write is appended to it. `draw` replies with the argv of
a writer, `inpane.WriterCommand`: `sh` finds the tty of the pane's
shell, sets its own terminal raw, prints a marker, and runs `cat` into
that tty. The plugin starts it as a hidden block on the host
(`session.new_block`, label `fingers-writer`), waits for the marker, and
after every program call sends the file's bytes to the writer with the
terminal's `write` method (`forward`). The frame goes through `write`
and not argv, which Linux limits to 128 KiB. A remote pane gets no flash:
the flash and the restore would arrive in one write.

**Keys.** The hint letters are function bindings on this machine's
server, and the app sends a host tab's keys to the host's server. So a
floating pane on the host, the reader (label `fingers-reader`), takes
the focus and runs `stty raw -echo; hexdump`, which prints each key as
two hex digits. It sets the title `fingers` 0.3 s after its start, so
that the app does not name the pane `hexdump`. Its argv is `inpane.ReaderCommand`, which the plugin
reads once from `rex-fingers reader`. The action stays alive: it reads
the reader's screen,
and presses each new byte as the key functions do: a letter, a capital
as Shift, a control byte as Ctrl. Escape, `q`, Space, and Ctrl+C end the
run. The reader is 0.001 of the window each way, at the pane's top
right corner; Rex refuses a float of no size or outside the window, and
a minimized one loses the focus. It opens before the hints are drawn, so
a key typed early waits in it and does not reach the pane's program.
Window scope leaves the reader out of the panes it hints.

**Guards.** Rex stops an action after 30 seconds, on a config reload,
and when the caller goes away, and runs no more of its Lua, so no
cleanup of the action's own can be counted on:

- A run ends by itself 15 seconds after the trigger (`host_limit`), and
  the wait for the writer stops 3 seconds before that. The end of a run
  restores the pane, closes the reader, focuses the pane, and then
  closes the writers, each step under `pcall`, also after an error. That
  order is what the user waits for.
- The reader ends itself after 20 seconds (`reader_life`), by a `kill`
  in its own command, and the focus returns to the pane.
- Every trigger first closes the readers it finds in the session, and
  the writers too when one can be left: on a session's first run since
  the config loaded, after a run that died, and when a reader was found.
  Writers are hidden, so the search for them is a round trip of its own.
  A trigger whose block is a reader or a writer draws nothing there.
  When the record of the dead run survived, it runs on that run's pane;
  after a reload it only closes. The hints a dead run painted stay until
  the next trigger, where `draw --begin` starts from the run file.
- Rex runs one action of a config at a time, so a trigger pressed during
  a run arrives when the run has ended, in the app with the closed
  reader as its block. It starts nothing: `closed_readers` knows the
  block, and a trigger in the same second as the end of a run is
  dropped. So the key cannot cancel a run on a host as it does locally.

Three costs remain. Every Rex call is a round trip, about 10 before the
hints show and 5 for a pick; the agent cuts that to 3 and none. While
the action is alive, with or without the agent, the config's Lua is
held: a key bound to a function waits, in every pane, and is lost when
the wait passes 5 seconds. And a call to the host that hangs has no
timeout; only Rex's 30 seconds end it. REX-FACTS.md has the
measurements.

A capture reads only the end of the pane's output, with or without the
agent. `format` returns the whole scrollback, megabytes on a pane in
use. `read_format` asks for a first reply as large as the screen needs;
a longer output is read from `total` minus that with `blob_read`. Each
row of the VT starts with its own SGR reset, so the rows draw the same
without those above.

`follow_scroll` on a host's pane takes another way than on a local
one, where the app's copies are queued and run once the action has
returned: on a host the action lives until the pick. Before the agent
starts, the plugin runs `rex-fingers view --client ID`. The program
saves the clipboard, asks the app for each copy-mode step with `rex -C
ID do`, on this machine's server, waits for the rows, and puts the
clipboard back. The rows go to the agent in its arguments as `view`.
`depth` in `agent.lua` looks for the page in the end of the pane's
output, up to 8 MiB back, and the agent sends the output from that page
down. The pane's spec for `draw` gets `view=FILE`, and the program
finds the page and paints it on the alternate screen, as for a local
pane. A page not found, or more than 300 KB of VT above the last row,
is left: the live screen is hinted. Only the focused pane is followed;
copy mode acts on that one. Without the agent there is no
`follow_scroll` on a host. `:open:` and `:reveal:` copy a path, as
below.

## Paths on another machine

A pane whose foreground process is `ssh`, `mosh-client`, or `et` shows
the other machine's paths, though its tty is here and hints draw as
usual. Opening one would open this machine's file of that name, or its
nearest folder, and `pwd` is the local shell's, not the remote one's. So
`press` turns `:open:` and `:reveal:` on a path into a copy, which every
pick does first anyway, and replies `remote_path`; the plugin logs it. A
URL opens as usual. The mark is `--remote-paths`, or `remote_paths=true`
in a `--pane` spec, sent only to a binary of protocol 3 or later.
`--remote` implies it. tmux,
`docker exec`, and other programs are not matched by name.

## Plugin and program versions

`rex-fingers protocol` prints the number of the flags and replies the
plugin relies on; `init.lua` carries the number it needs. `setup` warns
in the server log when the binary's is lower, or when it has no
`protocol` command at all, and records it in `program_protocol`. A flag
newer than the binary is then not sent. Without this, a binary built
before the plugin was updated fails on the first unknown flag, at the
first key.

## A scrolled pane: `follow_scroll = true`

A pane scrolled up shows its scrollback, and the steps above hint its
live screen, out of view. Rex reports no scroll position. So the app
copies the rows the pane shows, and the program takes them from the
clipboard (`follow_scroll` in `init.lua`, `internal/cli/await.go`):

1. `start_panes` with `await = 1` runs `draw --begin --await-views 1`
   with one `--pane` spec per pane, the focused pane first. That call
   saves the clipboard to a file, puts a marker text on it, writes
   `$TMPDIR/rex-fingers-<tty>.wait` per pane, starts a copy of itself in
   its own session, and returns the ttys at once. Lua stores the run
   then.
2. Lua queues `pane.copy_mode.enter`, `move screen_top`, `select line`,
   `move screen_bottom`, and `yank` with `rex.client.queue`, then
   `enter`, `select char`, and `exit`. The app runs them after the action
   returns. The last three drop the selection the yank leaves on screen,
   a grey box over the whole pane; `enter` and `exit` alone do not.
3. The background copy polls the clipboard. A text other than the marker
   is the rows of the next awaited pane, in the order of the specs; it
   writes them to that pane's view file. An empty clipboard counts only
   when it stays empty, since a writer clears before it fills. After the
   awaited panes, or 3 s with no copy, it puts the saved clipboard back
   and draws.

Only the focused pane is awaited. Copy mode acts on the focused pane
alone, and in window scope the other panes are hinted on their live
screens. Reading every pane with `pane.focus_next` between the copies
worked through `rex -C APP do`, where the CLI runs the queue one action
per call: 1.2 s for four panes. On a key press the app runs the whole
queue at once, the focus has not moved when the next copy runs, and every
copy was of the first pane. `--await-views N` still takes a count, for a
caller that can pace the copies.

Until the frame is up, `press` finds the wait file and no run file and
ignores the key. `restore` deletes the wait file, which calls the run
off: the background copy then draws nothing and deletes the captures. A
pane with no run file is not repainted.

`draw --begin` looks the rows up in the capture (`paneSpec.locate`,
`findOffset`), compared without spaces or line breaks, since the copy
joins wrapped rows and drops blank rows at the foot. A match above the
live screen sets `offset` and `alt`; none means the live screen. The same
rows can stand in several places, as after a command run twice; such
pages read the same, so the one whose rows line up with the copied lines
is taken, then the nearest.

An `alt` pane is painted on the alternate screen (`inpane.BeginAlt`),
which shows wherever the pane is scrolled, and `inpane.RestoreAlt` leaves
it with no repaint. `runState` carries `offset` and `alt` to `press` and
`restore`. A scrolled pane loses what its program prints during the run.

A call with origin `api` or `cli`, such as `rex do` from a shell, has no
app and hints the live screen; `follow_scroll=true` on the call overrides
that, for `rex -C APP do`.
