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

Faults 2 and 3 need the action to end once the hints are up. `host_keys
= "mode"` does that: built and tested 2026-11-24, and in the app the
trigger and Escape closed a run. It is off by default, since the app
shows a panel of the mode's 69 keys over the pane and the owner turned
that down. With it, leaving the tab during a run was not tried. A
script pushed to the host failed from a key and from the palette; that
is in HISTORY.md and REX-FACTS.md. A draft of what to ask the Rex developers
is in `docs/REX-REPORT-DRAFT.md`, a file kept out of git; it was not
sent.

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

1. When a Rex build lets a key mode hide its panel, or lets a config
   load a client script on a host, make `host_keys = "mode"` the
   default or bind the keys there: either ends faults 2 and 3 with
   nothing on screen. Recheck both in REX-FACTS.md on a new build.
   A run that types the letter into the shell reopens fault 1.
   A scrolled page on a host, `host_follow_scroll`, shows the app's
   selection; copying one row at a time showed a highlight too.
   A third way is built since 2026-11-24, off unless set: `host_keys
   = "substitute"`. **It failed its first tries in the app.** At 20:00
   and 20:02, on app snapshot 2107, which the app had updated to at
   19:59, the owner pressed the trigger and then a hint letter on a
   pane of the host: the hints showed, the owner saw no FINGERS box,
   and the app logged "Application termination requested" and started
   again, with no crash report, as in fault 2. At 20:02:11, three
   seconds before, it loaded the panes of a local session. The run had
   been up for ten seconds and its agent had seen no key. With
   `host_keys = "reader"` on the same build, minutes later, a pick
   worked. The demo script of the afternoon still worked on 2107 at 20:29:
   the swap, a key into the stand-in, the pane back. So the build
   failed, not the swap. One cause was found at 20:36 and fixed: the
   `tty` of Ubuntu 26.04 prints the path with no newline, so the agent
   never found the first line of the key file, read no key, and never
   moved the substitute. A trace of two runs showed that: no step and
   no key in 10 and 14 seconds (`touch /tmp/rex-fingers-trace` on the
   host turns the trace on). A second cause holds for the reader too, on app snapshot 2107.
   In six runs with `host_keys = "reader"` between 20:39:19 and
   20:39:51 the pick was made, and `/tmp/alpha` was on the clipboard;
   then the host's log has `client.key` for the reader's block with
   404, a second after the frames were sent, and the app's connection
   to the host ended. The app sends one more key event after the key
   that picks, the reader was closed by then, and this build does not
   survive the 404. The agent now gives the focus back at once and
   closes the reader 0.8 s after its report (`linger_reader`), and the
   plugin answers the report with `BYE`. The plugin's own reader, for a
   host without the agent, still closes at the pick. **No try in the
   app was made after these two fixes.** A block the agent makes takes the focused pane's
   place with `session.swap_blocks`, shows the frame, and reads the
   keys; the pane waits detached and comes back at the end. No key
   panel shows, and a small float shows only while the app loads a
   block, about 1.3 s on the Ubuntu host at each end. The owner saw a
   script do this in the app, pressed a key into the block, and called
   it "a lot better". DESIGN.md has the steps and REX-FACTS.md the
   calls and the times. `scripts/test_host_pane.sh` passes its cases
   named `substitute:`. **A run of the plugin itself, by a key press in
   the app, is not yet made.** Not tested either: a pane in a floating
   layer, and `host_follow_scroll` with it. DESIGN.md lists what it
   costs and does not guard. It does not
   end the run's action early, so faults 2 and 3 stay until the agent
   finishes a pick by itself.
2. To see what the app did, read its log: `log show --predicate
   'subsystem == "com.superlogical.rex"'` for the minute of the run. The
   server's own log records an action only when it fails.
3. To time or test a host without the app, see
   [TESTING.md](TESTING.md), "A pane on a Rex host".
4. `sh scripts/test_in_pane.sh` was not run on this tree; it overwrites
   the clipboard. Run it before a release.
