# Testing

## The scripts

| Script | Needs | Opens or overwrites |
| --- | --- | --- |
| `go test ./...` | Go | nothing |
| `sh scripts/smoke_test.sh` | Go; no Rex | nothing. Runs gofmt, vet, build, `go test -race`, and two 10 s fuzz runs. CI runs it on Ubuntu and macOS with `FUZZTIME=0`, which skips the fuzz runs. |
| `sh scripts/test_in_pane.sh` | a Rex server, `bin/rex-fingers`, `jq`, `pbcopy`; the plugin loaded in the running config for the `keys` case | opens sessions named `fingers-test`; **overwrites the clipboard** and does not put it back |
| `sh scripts/test_resources.sh` | the same, and `perl`; about 3 minutes | opens `fingers-test` sessions; saves the clipboard and puts it back as text, so a copied image is lost. It fails if hints are up in the app, and deletes `rex-fingers-*` temp files after a failed case. |

| `sh scripts/test_host_pane.sh` | `rex`, `bin/rex-fingers`, `jq`, `perl`; about 3 minutes | starts a second Rex server under `/tmp` and stops it; opens sessions only there; the copy goes to a file, so the clipboard is not touched |

Both live scripts take a filter: `sh scripts/test_in_pane.sh output` runs
the cases whose name contains "output". Build first: `go build -o
bin/rex-fingers .`.

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

## A pane on a Rex host

`scripts/test_host_pane.sh` needs no host. A second server on this
machine has its own host ID, so the plugin takes its panes for a host's,
and the script's config adds 280 ms to every Rex call. It types into the
reader with the terminal's `write` method, as the app's keys arrive, and
checks after every case that no reader or writer is left, the focus is
on the pane, and the pane's VT is as before. It takes a filter: `sh
scripts/test_host_pane.sh killed`.

It cannot show what the app does: whether the reader is visible, where
the app's own focus is, and the real round trip. For those, press the
key in the app on a host's tab. To see what a run holds up meanwhile,
send a bound key to a local pane once a second with `client.key` (see
"Press a key") and time each call.

## Draw in a pane by hand

`scripts/testdata/width-cases.txt` holds one line per character-width case:

```sh
S=$(rex new flag-probe --json | jq -r .session_id); sleep 1.5
B=$(rex api call -s $S session.view '{}' | jq -r '.windows[0].layers[0].blocks[0].block_id')
rex send -s $S -b $B 'clear; cat scripts/testdata/width-cases.txt' && rex send-key -s $S -b $B enter; sleep 1
rex block call -s $S -b $B format '{"format":"vt"}' | jq -j .content > /tmp/flag.vt
P=$(rex block call -s $S -b $B process | jq -r .foreground.pid)
go build -o bin/rex-fingers . && ./bin/rex-fingers draw --begin --pid $P --vt-file /tmp/flag.vt --rows 24 --cols 80
rex capture -s $S -b $B --format=text --trim | head -9   # every row: text, space, hint over the slash
./bin/rex-fingers restore --pid $P --vt-file /tmp/flag.vt --rows 24
rex kill $S
```

Take `--rows` and `--cols` from the block's `size` method; a headless
pane is not always 80x24.

## Press a key

A headless session has no app, but `client.key` sends a key press to the
server's key functions, so it can press a hint. The `keys` case in
`scripts/test_in_pane.sh` does that.

`rex api call -s $S client.key
'{"session_id":"'$S'","block_id":"'$B'","key":{"key":"escape","mods":"","consumed_mods":"","text":"","unshifted_codepoint":27,"composing":false,"action":"press"}}'`
prints `handled`. It is `true` when a key function took the key, and
`false` when the key went on to the pane, where it is typed. Names are W3C
codes in snake_case: `key_a`, `enter`, `escape`.

## Checks

**No key mode.** `rex keymap --json | jq '[.bindings[] |
select(.keys[0] | startswith("fingers/"))] | length'` must print 0, and
`.modes` must not list `fingers`. Do not use `grep -c fingers/`: it also
matches the source column, `rex-fingers/lua/...`.

**Stale hints.** `draw --begin` as above, then `rex do rex_fingers_cancel
-s $S -b $B`. The pane must show no hint letters and `modes` must report
7 and 25 set. Only the restore tells, since a second draw paints the same
frame whether or not it recovered the first run. Capture to a new file
each time. A capture written over the first run's path is the painted
frame, and `recover` refuses it.

**Window scope.** `rex split -s $S -b $B --split=below --focus=false
--json` gives a second block, each gets its own capture and `--pane`
spec, and one `draw` call takes both. A `press --key` with the same specs
needs no client. Check that the hint letters differ between the panes,
that `press` reports the right `pane` index and clipboard text, and that
both panes are restored. Through Lua, `rex do rex_fingers -s $S -b $B
scope=window` draws the hints and stores the run. The hints stay until a
key press, `rex do rex_fingers_cancel`, or the trigger key restores the
pane.
