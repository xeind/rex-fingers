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

## The align option

`align` chose which two rows a float hides, top or bottom. It was cut on
2026-11-18, before the first release. The float keeps the bottom rows,
where the prompt is.
