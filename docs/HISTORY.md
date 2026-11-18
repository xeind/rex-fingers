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

The install is `go install` and `git clone` at one tag again. A program
built on the user's machine is never quarantined. The cost is that the
user needs Go 1.26 and git. `scripts/build_release.sh` built the
tarballs; `git show 2a27f56:scripts/build_release.sh` prints it.

## The align option

`align` chose which two rows a float hides, top or bottom. It was cut on
2026-11-18, before the first release. The float keeps the bottom rows,
where the prompt is.
