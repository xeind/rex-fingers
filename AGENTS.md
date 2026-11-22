# rex-fingers

Letter hints over paths, URLs, and hashes on a Rex pane, like tmux-fingers.
Go program plus a Lua plugin for Rex, the terminal multiplexer by
Superlogical. This file is the map for a coding agent asked to install,
configure, extend, or debug it.

## Rules

Never:

- Hex colors in a style. Styles name palette slots (`38;5;N`, `48;5;N`),
  so they follow the theme.
- Shell in a Lua string. The Lua side only builds an argv.

Ask first:

- `rex config reload`, and any edit to the user's Rex config.

Always:

- Add a case to `internal/patterns/patterns_test.go` with every pattern
  change.
- Declare an option once in `defaults` in `init.lua`, and document it in
  `README.md` under Configuration with the same name and default.
  `internal/cli/docs_test.go` checks both.
- Keep tmux-fingers' names and defaults where Rex allows it.
- Recheck `docs/REX-FACTS.md` when a Rex build changes behavior.

## Install it for a user

1. Run `go version`. If Go is missing, ask the user, then install it:
   `brew install go` on a Mac. An older Go fetches 1.26 by itself.
   Clone the plugin at one tag, `V`, the newest of
   `git ls-remote --tags --sort=-v:refname https://github.com/xeind/rex-fingers`,
   and build the program into its `bin/`:
   ```sh
   git clone --branch $V --depth 1 https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
   go build -C ~/.config/rex/plugins/rex-fingers -o bin/rex-fingers .
   ~/.config/rex/plugins/rex-fingers/bin/rex-fingers version   # prints $V
   ```
   `go build -o` ignores `GOBIN` and `GOPATH`, so a mise or custom Go
   setup lands the program in the same place. Git ignores `bin/`.
   There is no prebuilt download; `docs/HISTORY.md` says why.
   When `~/.config/rex/plugins/rex-fingers` already exists, update it
   instead with the steps in `README.md` under Update. In step 2, edit
   the user's existing `fingers.setup` call; leave one call only.
2. In the user's Rex config, `~/.config/rex/init.lua` or a file it
   requires, add:
   ```lua
   local fingers_dir = os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers"
   local fingers = dofile(fingers_dir .. "/lua/rex-fingers/init.lua")
   fingers.setup{
     binary = fingers_dir .. "/bin/rex-fingers",
   }
   ```
   `binary` must be an absolute path. Set `window_key = false` if the
   user wants one key only.
3. `rex config check`, then `rex config reload`.
4. Confirm with `rex keymap | grep rex_fingers`: `alt+f` for the focused
   pane and `alt+shift+f` for every pane of the window.

Every option and its default is in `README.md` under Configuration.
Unknown option names make `setup` error, so read the list before adding
one.

## Change the keys or customize it for a user

Ask what the user wants, then change only the `fingers.setup{ ... }` call
in their Rex config. The table at the top of `docs/CONFIGURING.md` maps
each wish to its option. For each change:

1. Before you pick a key, run `rex keymap` and check that nothing else
   uses it.
2. Edit the option. Show the user the diff.
3. Run `rex config check`. Fix any error before you go on.
4. Ask the user before `rex config reload`.
5. Run `rex keymap | grep rex_fingers` and tell the user which keys are
   bound.

## Layout

| Path | Holds |
| --- | --- |
| `main.go` | package `main`: one call to `cli.Main`. It stays at the root so that `go install` of the module path builds the program |
| `internal/cli/cli.go` | CLI: `show`, `patterns`, `layouts`, `version`; action dispatch |
| `internal/cli/pane.go` | CLI: `draw`, `press`, `restore` for `overlay = "pane"`, one pane or several `--pane` specs; each prints one JSON object; `runState` and `recover` for a run left without restore; `locate` and `findOffset` for the page a scrolled pane shows |
| `internal/cli/target.go` | `resolveTarget`: a match to a URL, a folder, a file, or the nearest folder of a missing path; `openTarget` for `:open:` and `:reveal:`; `runOpener`, which a test replaces |
| `internal/cli/await.go` | the background wait of `draw --await-views`, for `follow_scroll` |
| `internal/inpane` | `Begin` and `Restore` sequences, `BeginAlt` and `RestoreAlt` for a scrolled pane, `TTY(pid)` to find the pane's tty |
| `internal/screen` | VT dump to lines of cells; `Parse`, `Line.Column`, `Line.CellAt` |
| `internal/patterns` | `Builtin` map, `Default` (all but `digit`), `Weight` per pattern for hint order, `Compile`, `Set.Find` with leftmost-longest overlap rule |
| `internal/hints` | `Generate` prefix-free hints; `Layouts` and `Alphabet` |
| `internal/ui` | `Overlay`: raw mode, `draw`, key handling, modifiers |
| `internal/rexcli` | `Pane.Size`, `Capture`, `Write`, `Pwd`, `Focus` through the `rex` CLI |
| `lua/rex-fingers/init.lua` | `setup(opts)`, the action, rect lookup, capture |
| `assets/` | the icon and the logos; `assets/README.md` lists them |

## Commands

```sh
go test ./...                       # unit tests: screen, patterns, hints, CLI, docs
sh scripts/smoke_test.sh            # gofmt, vet, build, -race, 10 s fuzz; no Rex needed
sh scripts/test_in_pane.sh          # live cases; opens Rex sessions, overwrites the clipboard
sh scripts/test_resources.sh        # leaks, memory, 2 to 12 panes; about 3 minutes; fails if hints are up in the app
go vet ./... && gofmt -l .
go build -o bin/rex-fingers .       # bin/ is gitignored
rex config check && rex config reload
rex do rex_fingers -b "$REX_BLOCK"  # open the overlay on this pane
rex server logs | grep -i lua       # Lua errors the check misses
```

## Docs

Read the one the task needs:

- `docs/DESIGN.md`: before you change how hints are drawn, picked, or
  restored. The float, the pane overlay and its keys, the run file, a
  pane that writes during a run, window scope, `follow_scroll`.
- `docs/REX-FACTS.md`: before you rely on a Rex behavior, and when a
  result surprises you. What Rex does and lacks, with the build and date
  of each test.
- `docs/TESTING.md`: before you run a live script or test without a
  screen. What each script needs and overwrites, and the headless loops.
- `docs/HISTORY.md`: before you bring back a key mode, a key sent into
  Lua, or an option that was cut.
- `docs/CONFIGURING.md`: the user walkthrough, one check per step.
- `docs/TMUX-FINGERS.md`: every tmux-fingers option and its match here.
