# rex-fingers

Letter hints over paths, URLs, and hashes on a Rex pane, like tmux-fingers.
Go program plus a Lua plugin for Rex, the terminal multiplexer by
Superlogical. This file is the map for a coding agent asked to install,
configure, extend, or debug it.

## Install it for a user

1. `go install github.com/xeind/rex-fingers@latest`. The binary lands in
   `$(go env GOPATH)/bin/rex-fingers`.
2. Clone this repo to a permanent path, usually
   `~/.config/rex/plugins/rex-fingers`.
3. In the user's Rex config, `~/.config/rex/init.lua` or a file it
   requires, add:
   ```lua
   local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
   fingers.setup{ binary = os.getenv("HOME") .. "/go/bin/rex-fingers" }
   ```
   `binary` must be an absolute path.
4. `rex config check`, then `rex config reload`.
5. Confirm with `rex keymap | grep rex_fingers`.

Every option and its default is in `README.md` under Configuration. A
walkthrough with a check per step is `docs/CONFIGURING.md`. Unknown option
names make `setup` error, so read the list before adding one.

## How it works

Rex has no API that draws into an existing terminal. The Lua action
`rex_fingers`:

1. Finds the focused pane's rectangle in `session.view`.
2. Captures the pane's screen as VT sequences with the terminal block's
   `format` method and writes it to a temp file.
3. Opens a floating layer with that rectangle, running
   `rex-fingers show --session S --block B --vt-file F --rows N ...` with
   `shell = "none"`.

The Go program parses the VT into a cell grid, runs the patterns on each
row, draws the dimmed screen with hints over the matches, reads one hint
in raw mode, runs the action, and exits. The floating pane closes on exit
and the program calls `rex focus` on the original pane.

## Layout

| Path | Holds |
| --- | --- |
| `main.go` | CLI: `show`, `patterns`, `layouts`, `version`; action dispatch |
| `internal/screen` | VT dump to lines of cells; `Parse`, `Line.Column`, `Line.CellAt` |
| `internal/patterns` | `Builtin` map, `Compile`, `Set.Find` with leftmost-longest overlap rule |
| `internal/hints` | `Generate` prefix-free hints; `Layouts` and `Alphabet` |
| `internal/ui` | `Overlay`: raw mode, `draw`, key handling, modifiers |
| `internal/rexcli` | `Pane.Size`, `Capture`, `Write`, `Pwd`, `Focus` through the `rex` CLI |
| `lua/rex-fingers/init.lua` | `setup(opts)`, the action, rect lookup, capture |
| `docs/CONFIGURING.md` | user walkthrough |
| `assets/` | logo SVGs: tile, transparent mark, black, white, adaptive; 512 px PNGs rendered with `qlmanage -t -s 512` |

## Commands

```sh
go test ./...                       # unit tests: screen, patterns, hints
go vet ./... && gofmt -l .
go build -o bin/rex-fingers .       # bin/ is gitignored
rex config check && rex config reload
rex do rex_fingers -b "$REX_BLOCK"  # open the overlay on this pane
rex server logs | grep -i lua       # Lua errors the check misses
```

## Test loop without a screen

An agent cannot see the overlay. Drive it through the Rex CLI:

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

## Facts the code depends on

- A floating pane in Rex for Mac is two rows shorter than the pane under
  it; `--align` picks which rows to drop. The pane reports 80x24 until the
  app sizes it, so `ui.waitForSize` polls up to 1.5 s and `draw` reruns on
  SIGWINCH.
- Rex for Mac did not forward OSC 52 to the clipboard in testing. Copy
  uses `pbcopy` or a Linux equivalent; OSC 52 is written too.
- A pane whose program exits within a moment stays open in Rex. The
  program lives at least 400 ms.
- macOS checks a freshly built binary on first run, about 360 ms. Run it
  once after building before judging latency.
- `session.view` nests `split.before` and `split.after`, each with `rect`
  in window fractions; a floating layer's layout is relative to its bounds.
- Go regexp accepts `(?<match>...)`; the tmux-fingers patterns port as is.

## Rules

- Every pattern change needs a case in `internal/patterns/patterns_test.go`.
- Styles name palette slots (`38;5;N`, `48;5;N`), never hex colors, so
  they follow the theme.
- No shell in Lua strings; the Lua side only builds an argv.
- Options are declared once in `defaults` in `init.lua` and documented in
  `README.md` under Configuration, with the same names and defaults.
- Keep tmux-fingers' names and defaults where Rex allows it.
- Tested against Rex for Mac 0.1.0 (1825), CLI `0.0.1793741575-g1b6654`.
  Rex is in beta; recheck the facts above when a build changes behavior.
