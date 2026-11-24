# History

Why rex-fingers is the way it is, where the reason is a thing that was
tried and dropped. `git log` holds the rest. The current design is in
[DESIGN.md](DESIGN.md).

## Pane mode became the default on 2026-11-18

The defaults since 2026-11-18 are `overlay = "pane"`, `window_key =
"alt+shift+f"`, and `follow_scroll = false`. Pane mode was opt-in before:
it used a key mode, Rex for Mac showed a popup listing the mode's
bindings, and the user turned it down 2026-11-07. It now binds function
keys and no mode, and the user confirmed on build 2018 that no popup
shows. It is the default because it hides no rows and is the path the
live suite covers. `follow_scroll` stays off: it takes the clipboard for
half a second, loses a copied image, and needs the server on the app's
machine.

## The key mode

The plugin once used a key mode, `fingers`, with one `rex_fingers_key`
binding per letter and modifier. The app listed every binding of the
active mode in a panel no setting hides. [REX-FACTS.md](REX-FACTS.md)
holds what was tested.

Until 2026-11-18 the action queued `client.mode.enter`, which the app
refused from the CLI. Nothing enters a key mode now, so
`rex do rex_fingers scope=window` needs no client.

## A pick on a pane that moved

`press` once refused every pick on a pane whose program had written
during the run. In window scope, one pane with a spinner, such as Claude
Code, blocked every copy. It now picks, since a letter on screen names
the same match in the capture.

## The key sent back into Lua

The first version of `follow_scroll` had the app send a key, `f19`, back
to a Lua key function after each copy. A key sent to a pane moves a
scrolled pane to its last row, so the pane flickered to the bottom before
the frame came up and stayed at the bottom after the run. Nothing calls
back into Lua now; the program waits on the clipboard.

## Several matches for a scrolled page

Until 2026-11-18, when the rows a scrolled pane shows stood in several
places of the capture, the live screen was hinted, and the pane looked as
if it had jumped to the bottom. `findOffset` now takes the page whose rows
line up with the copied lines, then the nearest.

## The prebuilt tarball

On 2026-11-19, `v0.3.0` went out as a GitHub release with two macOS
tarballs, each holding the built program and the Lua plugin, and the
install became one `curl` line. It was withdrawn the same day, and the
release was deleted; the tag stays.

The program was signed ad hoc and not notarized. A copy fetched with
`curl` ran. A copy downloaded with a browser carried
`com.apple.quarantine`, and macOS killed it with exit 137 and showed the
dialog "rex-fingers Not Opened". Only `xattr -dr com.apple.quarantine` on
the folder got past it. Notarizing needs a paid Apple Developer account.

The install went back to `go install` and `git clone` at one tag. A
program built on the user's machine is never quarantined. The cost is
that the user needs Go 1.26 and git. `scripts/build_release.sh` built the
tarballs; `git show 2a27f56:scripts/build_release.sh` prints it.

On 2026-11-22, `go install` gave way to `go build -o bin/rex-fingers`
inside the clone. `go install` writes to `GOBIN`, which mise points at
its own folder, so the documented `~/go/bin/rex-fingers` did not exist
there. The build into the clone ignores `GOBIN` and `GOPATH`, and it
builds the program from the same checkout as the plugin.

## The floating pane that read the keys

On 2026-11-19 a branch added `key_input = "focus"`: a one-row floating
pane took focus during a run, and a `keys` subcommand read the hint keys
from its tty. Only the two triggers stayed bound, so typing in a
scrolled pane returned it to its last row. The costs: a float on screen
for each run, the pane out of focus meanwhile, about 0.1 s before the
first key was read, and no `follow_scroll`, since the app copies from
the pane that has focus. The owner tried it and turned it down the same
day, and it was never committed. The plugin binds fewer keys instead.
[REX-FACTS.md](REX-FACTS.md) keeps the floating-layer facts it relied on.

## The script pushed to a host

On 2026-11-24 a run on a Rex host took 2.3 s to the hints, since every
call from this machine's action is a round trip. The first plan was to
push a Lua script to the host with `client.set_script`: it loads into
the server's memory for one client, binds keys there, and calls the Rex
API at no network cost. It worked from a `rex do` script. From the
config action, on a host's tab in the app, Rex refused it: "a key
handler cannot replace the script of the client whose key it is
handling". A script the CLI pushes belongs to the CLI's client, and the
app's keys do not reach it. So the keys stayed with a floating reader,
and the host's own `rex` runs the rest: the agent of
[DESIGN.md](DESIGN.md). A pushed script may work once Rex runs actions
from client scripts; [REX-FACTS.md](REX-FACTS.md) has what was tested.

Installing the plugin and the program on the host was ruled out by the
owner the same day: the plugin on the client machine must work on any
host and session it connects to.

## The key mode for a host's panes

On 2026-11-24 a key mode came back for host panes only, for some hours,
and was never committed. The reader had two faults: letters reached the
shell when the app's focus did not follow, and the app froze when the
user left the tab during a run, since the run's action stays alive. With
a mode there is no reader and no live action: the trigger drew the hints
through the agent, queued `client.mode.enter` for a `fingers` mode, and
returned; each letter was bound in the mode to an action,
`rex_fingers_host_key`, which told the agent the pick by a label on its
block.

It did not work. In a host's tab the app never turned the mode on: no
panel showed, `s` was typed into the shell, and the app logged `Skipped
client.mode.exit, which rex_fingers asked for: it isn't available here`
for each of six runs, and nothing about the enter. The plugin's side was
sound: the mode and its 138 bindings were in `rex keymap`, and no action
failed. The host's keymap is empty, and the likely cause is that the app
looks for the mode there; that was not proven. [REX-FACTS.md](REX-FACTS.md)
has the evidence. The reader came back the same day.

## The align option

`align` chose which two rows a float hides, top or bottom. It was cut on
2026-11-18, before the first release. The float keeps the bottom rows,
where the prompt is.
