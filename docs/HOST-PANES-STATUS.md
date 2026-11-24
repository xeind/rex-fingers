# Hints on a host's panes: where the work stands

Written 2026-11-24. This is a record of work in progress, kept in the
repo so that it outlives a session. Update it when a fault below is
closed, and delete it when none is left. How the code works is in
[DESIGN.md](DESIGN.md), "A pane on a Rex host". What Rex was seen to do,
with builds and dates, is in [REX-FACTS.md](REX-FACTS.md), "Rex hosts".
Why two other designs were dropped is in [HISTORY.md](HISTORY.md).

## The goal

`alt+f` and `alt+shift+f` on the panes of any Rex host, with the plugin
and the program on the client machine only. Nothing is installed on a
host; the owner ruled that out.

## What is built

All of it was uncommitted on `main` at `e3d4898` when this was written.

- A host pane's capture reads only the end of the scrollback.
- The agent: a Lua script the host's own `rex do` runs in a hidden
  block, sent in the block's argv. It reads the panes, opens the reader,
  draws, reads the keys, and puts the panes back. The plugin makes three
  calls to the host before the hints show.
- The older run, with the plugin's own reader and writer, for a host
  whose blocks have no `rex`.

## What was measured

| Check | Result |
| --- | --- |
| `sh scripts/test_host_pane.sh`, with the agent | passed, 2026-11-24 |
| `TEST_AGENT=false sh scripts/test_host_pane.sh` | passed, 2026-11-24 |
| A real host, 170 to 230 ms a call, from two `rex do` scripts | hints after 0.8 to 1.0 s; 2.2 to 2.7 s without the agent |
| A real host, the plugin killed with the hints up and the pane printing | hints gone, reader and agent gone, focus back, when looked at 24 s after the kill |
| A real key press in the app, in a host's tab | picked, 2026-11-24; the owner tried new panes and called fault 1 cleared |

## Open faults

Each was seen in the app on 2026-11-24, snapshot 2104. None comes from
the host's speed or from the owner's `setup` options.

1. **Closed 2026-11-24.** Hint letters reached the shell, with the hints up. Seen on the
   first run in a newly opened pane. The session's focus on the host is
   on the reader; the app's own key focus is not. Tried and failed, in
   the app at 14:50: the agent handing the focus to the pane and back to
   the reader. In the code now: the plugin focuses the reader once more
   after the frames are sent, with a call the app itself passes on.
   With that, a run in the app picked at 14:58, and the owner then
   tried new panes and reported the fault cleared. The app shows the
   reader as a small box on the pane, named `hexdump` after its
   program; it cannot be hidden (REX-FACTS.md, "Rex gives the focus
   only to a block placed in a window"). The reader now sets the title
   `fingers`, and the app shows it (the owner, 15:53).
2. **The app stops answering during a run, and has exited.** Five
   times by 2026-11-24 15:53, the last two with `follow_scroll` on a
   host in the tree. The first three
   times, each in the second it loaded the panes of a local session. The
   run's action is alive until the pick, and its calls to the host go
   through the app. That a change of tab is the trigger is not proven.
   No cure is in the code. Until there is one: stay on the tab while the
   hints are up.
3. **The trigger does not close the hints on a host.** Rex runs one
   config action at a time, and the run's action lives for the whole
   run. Escape closes. By design, for now.

Faults 2 and 3 need the action to end once the hints are up. Two ways to
do that failed on this build: a script pushed to the host, and a key
mode. Both are in HISTORY.md. A draft of what to ask the Rex developers
is in [REX-REPORT-DRAFT.md](REX-REPORT-DRAFT.md); it was not sent.

## Review findings, from reading the code

A second reader went through `agent_run`, `host_run`, and `agent.lua` on
2026-11-24. None of these was seen to cause a fault above.

| Finding | State |
| --- | --- |
| A pane that printed during a run kept its hints when the plugin was gone | Fixed: the agent waits 6 s for the plugin and then restores |
| The plugin closed the agent's block before the agent wrote the last repaint | Fixed: the plugin sends `BYE` and the agent ends itself |
| A failed write to a pane's tty is ignored, on draw and on restore | Open |
| One slow start of the agent marks the host as without one until the config loads again | Open |
| The agent's reply is read in one `format` call, which fails above 1 MiB | Open; a pane of 105 by 64 is far below it |
| A trigger in the clock second a run ended is dropped | Kept on purpose: it drops the triggers Rex queued during the run |
| A new run closes every reader and agent in the session, another app's too | Kept on purpose, to clear a dead run; two apps in one session at once is untested |

Also open, from before the review: Ctrl+`j` and Alt+hint do not pick on
a host; the first letter of a two-letter hint changes nothing on screen;
`rex-fingers-*.run.json` files of dead runs stay in `$TMPDIR`.

## How to carry on

1. End the action once the hints are up; that is what faults 2 and 3
   need. A way not yet built: the agent finishes a `:copy:` pick by
   itself, with OSC 52 into the pane's tty, which reached the Mac's
   clipboard on 2026-11-24 (REX-FACTS.md). Picks that need this machine,
   such as a URL to open or a `copy_command`, still need an answer here.
   A run that types the letter into the shell reopens fault 1.
2. To see what the app did, read its log: `log show --predicate
   'subsystem == "com.superlogical.rex"'` for the minute of the run. The
   server's own log records an action only when it fails.
3. To time or test a host without the app, see
   [TESTING.md](TESTING.md), "A pane on a Rex host".
4. `sh scripts/test_in_pane.sh` was not run on this tree; it overwrites
   the clipboard. Run it before a release.
