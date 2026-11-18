# Resource and multi-pane tests: specification

You are adding a test script to rex-fingers. This sheet is the whole
brief. Read it to the end before you run anything.

## Goal

Write `scripts/test_resources.sh`. It proves that rex-fingers leaves
nothing behind and holds up with many panes and long scrollback:

- no temp file, run file, wait file, or process is left after any run
- memory use of the program stays bounded as the scrollback grows
- the Rex server does not grow across many runs
- runs across 2, 4, 8, and 12 panes draw, pick, and restore correctly
- a pane that closes or resizes during a run does not strand the others

Find bugs and report them. Do not fix them.

## Read first

| File | Why |
| --- | --- |
| `AGENTS.md` | how the plugin works, the facts it depends on, the headless test loop |
| `scripts/test_in_pane.sh` | the harness to copy: `open_session`, `capture`, `check`, `begin_case`, `end_case` |
| `pane.go`, `await.go` | the `draw`, `press`, `restore` subcommands and the background wait |
| `lua/rex-fingers/init.lua` | the `rex_fingers` and `rex_fingers_cancel` actions |

## Rules

Never:

- Run a git command that changes anything. No add, commit, stash, reset,
  or checkout. `git status` and `git diff` are fine.
- Change `~/.config/rex/` or any file outside this repo and `/tmp`.
- Run `rex config reload`.
- Call `client.perform`, `rex -C`, `rex focus`, or `rex block focus`. They
  move focus in the user's app while the user works.
- Touch a session you did not create. Name every session you create
  `fingers-test`, and kill it when its case ends.
- Change plugin code: `*.go` outside tests, `lua/`. A bug you find goes in
  the report.

Always:

- Save the clipboard to a file before the script's first case and restore
  it when the script exits, with a `trap`. Some cases write the clipboard.
- Use headless sessions: `rex new fingers-test --json`.
- Build first: `go build -o bin/rex-fingers .`, and run it once before any
  timing, since macOS checks a new binary on first run.
- Run one simple command per shell call. Edit files with the edit tool,
  not `sed` or a heredoc.
- Follow the writing rules in `~/.claude/CLAUDE.md` for comments and the
  report.

## The script

- POSIX `sh`, like `scripts/test_in_pane.sh`. Copy its helpers; do not
  source it, since it runs its cases when sourced.
- `sh scripts/test_resources.sh` runs every case.
  `sh scripts/test_resources.sh NAME` runs the cases whose name contains
  NAME.
- It prints `== case name`, then `ok` or one `FAIL: reason` line per
  failed check, and ends with `passed N, failed M`. The exit status is 0
  only when M is 0.
- Each case ends with the checks `end_case` makes in
  `scripts/test_in_pane.sh`: pane text unchanged, modes 7 and 25 set, no
  capture or run file left. Add: no wait file left, mode 1049 not set.

Define "left behind" once, in a helper, and use it in every case:

- files in `$TMPDIR` (or `/tmp` when unset) named `rex-fingers-*`
- capture files the case created
- processes that match `rex-fingers draw`, checked up to 4 seconds after
  the case, since the background wait can run 3 seconds

## Cases

### Leaks

1. **200 draw and restore cycles on one pane.** Count the files and
   processes before and after. The counts must be equal.
2. **50 runs with `--await-views 1`, in three kinds.** In one third the
   rows arrive: write a page to the clipboard with `pbcopy`, as the case
   "await views" in `scripts/test_in_pane.sh` does. In one third a
   `restore` calls the run off during the wait. In one third nothing
   arrives and the wait times out. After all 50: no process, no wait file,
   no `rex-fingers-clipboard-*` file, and the clipboard holds what it held
   before the first run.
3. **100 trigger and cancel cycles through Lua.** `rex do rex_fingers -s S
   -b B`, then `rex do rex_fingers_cancel -s S -b B`. After them, a key
   must pass through: `client.key` with `escape` returns `handled: false`.
   AGENTS.md shows the call.
4. **A killed session during a run.** Draw on a pane, kill the session,
   then draw on a new session. The run file of the dead tty must not
   break the new run. Tty numbers are reused.

### Memory

5. **Peak memory of `draw` against scrollback size.** Use
   `/usr/bin/time -l` and read "maximum resident set size". Measure at
   1,000, 10,000, and 100,000 lines of scrollback, with and without
   `--view-file`. Build the capture file by hand for the large sizes; a
   pane's scrollback has a limit. Print the three numbers. Then set the
   pass limit at twice what you measured for 100,000 lines and write the
   measured value in a comment beside it.
6. **Peak memory of `press` and `restore`** at the same three sizes. They
   read the same capture on every key.
7. **Rex server memory across 200 Lua trigger and cancel cycles.** Read
   the server's resident size with `ps -o rss= -p PID` before and after.
   Find the PID from `rex server` subcommands or `pgrep`. Print both
   numbers. Fail if it grows by more than 20 MB. If you cannot find the
   server process, say so in the report and skip the case with a note.

### Many panes

8. **2, 4, 8, and 12 panes in one window, by the program.** Make the panes
   with `rex split`. Print a different path in each. One `draw --begin`
   with a `--pane` spec per pane. Check: every pane shows a hint, no hint
   letters repeat across panes, `press` on a hint from the last pane
   reports that pane's index and copies its text, and every pane is
   restored. Print the time of the `draw` call for each count.
9. **8 panes through Lua.** `rex do rex_fingers -s S -b B scope=window`,
   then cancel. Every pane restored.
10. **Mixed panes.** One window with a pane at a shell prompt, a pane in
    the alternate screen (`less` on a file), a pane with long scrollback,
    and a pane 10 columns wide. Draw, then restore. The `less` pane must
    still be in `less` and show the same rows.
11. **A pane closes during a run.** Start a run over 4 panes, kill one
    pane's shell (`rex send` of `exit`), then cancel. The other 3 are
    restored, and the cancel does not fail.
12. **A pane is resized during a run.** Resize one pane with
    `rex resize` while the hints are up, then cancel. Report what the pane
    shows afterwards; mark it a bug if hint letters remain.

### Scale

13. **A screen where every row is a match**, with a two-letter alphabet
    (`--alphabet as`). At most 4 hints exist. The run must draw, report 4
    hints or fewer, pick one, and restore.
14. **Row width cases at 12 panes.** `cat testdata/width-cases.txt` in
    each pane, draw, restore, text unchanged.

## How to verify your own work

- Run the whole script twice in a row. The second run must pass too; a
  first run that leaves something behind shows up there.
- Prove each leak check can fail. Plant a file named
  `rex-fingers-ttys999.run.json` in the temp directory, run the case, see
  it fail, remove the file. Do the same with a `sleep 30` process renamed
  to match. Say in the report that you did this.
- Run `sh scripts/test_in_pane.sh` at the end. It must still report
  `passed 27, failed 0`.

## Report

When you finish, print a report with these parts and nothing else:

1. **Result:** passed and failed counts, and the run time.
2. **Numbers:** a table of the memory and timing measurements.
3. **Bugs found:** one entry each, with the command that reproduces it,
   what you expected, and what happened. Write "none" if none.
4. **Not tested:** each case you skipped or weakened, and why.
5. **Files changed:** should be `scripts/test_resources.sh` only.

Do not commit. The user reviews first.
