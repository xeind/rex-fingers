<p align="center">
  <img src="assets/logo.svg" width="160" alt="rex-fingers: a dinosaur arm shaped like the letter F">
</p>

# rex-fingers

Short arms, long reach.

Letter hints over the paths, URLs, hashes, and other strings on a
[Rex](https://www.superlogical.com/rex) pane. Press a hint and the match is on
your clipboard. A port of [tmux-fingers](https://github.com/Morantron/tmux-fingers)
to Rex, with the same patterns and keys.

<!-- demo: assets/demo.gif -->

## Contents

- [Usage](#usage)
- [What gets a hint](#what-gets-a-hint)
- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Coming from tmux-fingers](#coming-from-tmux-fingers)
- [Recipes](#recipes)
- [How it works](#how-it-works)
- [Limits](#limits)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Acknowledgements](#acknowledgements)

## Usage

Press <kbd>Ctrl-b</kbd> <kbd>F</kbd> in any pane. Everything that is not a
match dims, and each match gets a one or two letter hint drawn over its first
characters. Type the hint.

| Keys | Default action |
| --- | --- |
| <kbd>a</kbd>-<kbd>z</kbd> | `:copy:` the match to the clipboard |
| <kbd>Shift</kbd> + hint | `:paste:` it, typing the match into the pane |
| <kbd>Ctrl</kbd> + hint | `:open:` it with `open` on macOS or `xdg-open` on Linux |
| <kbd>Alt</kbd> + hint | nothing until you set `alt_action` |
| <kbd>Backspace</kbd> | Drop the last typed letter |
| <kbd>q</kbd>, <kbd>Esc</kbd>, <kbd>Ctrl-c</kbd> | Leave without copying |

Every pick is copied to the clipboard first, whatever the action. These are
the tmux-fingers defaults, and each one can be changed; see
[Configuration](#configuration) and [docs/CONFIGURING.md](docs/CONFIGURING.md)
for a walkthrough.

The same text gets the same hint wherever it appears, so a path printed
ten times costs one key. With more matches than letters, some hints become
two letters; the first letter narrows the set and the second picks.

The command palette lists it as **Fingers: Copy by Hint**.

## What gets a hint

| Pattern | Example | Copies |
| --- | --- | --- |
| `path` | `/usr/local/bin/rex`, `~/.config/rex/init.lua` | the path |
| `quoted-path` | `'/tmp/a b/c'`, `"My Documents/notes.txt"` | the path without quotes |
| `file-line` | `src/app.ts:10:5`, `main.go:42` | path and position |
| `url` | `https://example.com/x?y=1`, `git@github.com:a/b.git` | the URL |
| `markdown-url` | `[docs](https://example.com/docs)` | the URL only |
| `email` | `someone@example.com` | the address |
| `sha` | `deadbeefcafe1234` | the hash |
| `uuid` | `3b241101-e2bb-4255-8caf-4136c566a962` | the UUID |
| `hex` | `0x7ffee3` | the number |
| `color` | `#c6b18e` | the color |
| `ip` | `10.0.0.42` | the address |
| `ipv6` | `fe80::1c3a:ff:fe12:3456%en0`, `::1` | the address |
| `semver` | `v2.1.318-beta.1` | the version |
| `digit` | `8080` | numbers of four or more digits |
| `kubernetes` | `pod/nginx-7xb2r`, `deployment.apps/web` | the resource |
| `kubernetes-pod` | `nginx-deployment-66b6c48dd5-7xb2r` | the pod name |
| `git-status` | `modified:   src/main.go` | the path only |
| `git-status-branch` | `Your branch is up to date with 'origin/main'.` | the branch |
| `diff` | `--- a/internal/ui/ui.go` | the path only |

When two patterns cover the same text, the leftmost and then the longest
match wins, so `init.lua:12` is one `file-line` hint rather than a path
plus a number. `rex-fingers patterns` prints every expression.

## Requirements

- Rex, with the `rex` command on the server's PATH. Tested with Rex for Mac
  0.1.0 (1825) and CLI `0.0.1793741575-g1b6654`. Rex is in beta and its API
  moves between builds.
- Go 1.24 or newer, to build the program.
- A clipboard command. `pbcopy` on macOS; `wl-copy`, `xclip`, or `xsel` on
  Linux. The first one found is used.

## Installation

Build the program:

```sh
go install github.com/xeind/rex-fingers@latest
```

That puts `rex-fingers` in `$(go env GOPATH)/bin`, usually `~/go/bin`.

### With a plain `init.lua`

Clone the plugin somewhere permanent and load it from your Rex config:

```sh
git clone https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
```

```lua
-- ~/.config/rex/init.lua
local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
fingers.setup{
  binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
}
```

Check and reload:

```sh
rex config check
rex config reload
```

### With Amber

[Amber](https://github.com/rawnly/rex-package-manager) installs and locks
Rex plugins from GitHub:

```lua
amber.setup({
  plugins = {
    { "xeind/rex-fingers", opts = { binary = os.getenv("HOME") .. "/go/bin/rex-fingers" } },
  },
})
```

Then `rex do amber_install` and `rex config reload`.

## Configuration

Every option is a field of the table passed to `setup`. Unknown fields are
an error, so a typo is caught at `rex config check`.

```lua
fingers.setup{
  binary = "/usr/local/bin/rex-fingers",
  key = "ctrl+b>shift+f",
  patterns = {},
  extra = {},
  main_action = ":copy:",
  shift_action = ":paste:",
  ctrl_action = ":open:",
  alt_action = "",
  copy_command = "",
  keyboard_layout = "qwerty",
  alphabet = "",
  hint_position = "left",
  hint_style = "1;38;5;0;48;5;12",
  highlight_style = "1;38;5;12",
  dim = true,
  align = "bottom",
}
```

Step-by-step setup, with a check after each step, is in
[docs/CONFIGURING.md](docs/CONFIGURING.md).

### `binary`

`default: ""`

Absolute path of the `rex-fingers` program. Set it. Without it the floating
pane starts your login shell to look the program up on PATH, which adds a
visible delay before the hints appear.

### `key`

`default: "ctrl+b>shift+f"`

The key or key sequence to bind, in Rex's `rex.bind` syntax. Set it to
`false` to bind nothing and add your own binding:

```lua
fingers.setup{ binary = "...", key = false }
rex.bind("alt+f", "rex_fingers")
```

### `patterns`

`default: {}`

Names of the built-in patterns to enable. Empty enables all of them.

```lua
patterns = { "path", "url", "file-line" },
```

### `extra`

`default: {}`

Extra regular expressions in Go syntax. Each becomes one more pattern. A
named group `(?<match>...)` limits what is highlighted and copied to that
part of the match. Use Lua long strings so backslashes need no escaping.

```lua
extra = {
  [[git rebase --(abort|continue)]],
  [[Error: (?<match>.+)]],
  [[\bISSUE-\d+\b]],
},
```

A bad expression fails at `rex config check` with the pattern's position.

### `main_action`, `shift_action`, `ctrl_action`, `alt_action`

`default: ":copy:"`, `":paste:"`, `":open:"`, `""`

What happens when a hint is pressed plain, with Shift, with Ctrl, or with
Alt. Each takes one of three special values or a shell command:

| Value | Does |
| --- | --- |
| `:copy:` | Copies the match with `copy_command` |
| `:paste:` | Types the match into the pane through Rex's terminal write method |
| `:open:` | Opens the match with `open` or `xdg-open` |
| any other string | Runs it with `sh -c`, the match on standard input |
| `""` | Nothing |

Every pick is copied first, as in tmux-fingers, so an action can rely on the
clipboard. A shell command runs in the pane's working directory with two
environment variables: `MODIFIER`, one of `main`, `shift`, `ctrl`, `alt`,
and `HINT`, the letters that were typed.

```lua
alt_action = "xargs -I{} rex run -- nvim {}",   -- Alt+hint opens the file in a new tab
```

### `copy_command`

`default: ""`

What `:copy:` runs, with the match on standard input. Empty picks `pbcopy`,
`wl-copy`, `xclip -selection clipboard`, or `xsel --clipboard --input`,
whichever exists.

```lua
copy_command = "ssh laptop pbcopy",
```

### `keyboard_layout`

`default: "qwerty"`

Picks the hint letters by where they sit on the keyboard. The names and
letters are tmux-fingers': `qwerty`, `azerty`, `qwertz`, `dvorak`, and
`colemak`, each also as `-homerow`, `-left-hand`, and `-right-hand`.
`rex-fingers layouts` prints every name with its letters. `c`, `i`, `m`,
`q`, and `n` are never hints.

```lua
keyboard_layout = "colemak-homerow",
```

### `alphabet`

`default: ""`

The hint letters themselves, overriding `keyboard_layout`. Order is
preference: the first letters are used first.

```lua
alphabet = "arstneiowfpluy",
```

Letters outside the alphabet do nothing while the hints are shown, except
`q` to leave.

### `hint_position`

`default: "left"`

Where the hint sits on the match: over its first characters with `left`, or
its last with `right`.

### `align`

`default: "bottom"`

The floating pane in Rex for Mac is two rows shorter than the pane it
covers, because of its header. `bottom` keeps the bottom of the pane and
hides the top two rows. `top` keeps the top and hides the bottom two.

### `hint_style` and `highlight_style`

`default: "1;38;5;0;48;5;12"` and `"1;38;5;12"`

SGR parameters, the numbers that go inside `ESC [ ... m`. `hint_style`
draws the hint letters and `highlight_style` draws the rest of the match.

The defaults name palette slots, not fixed colors. Slot 12 is the theme's
bright blue and slot 0 its black, so hints are dark text on blue and the
rest of a match is blue, in whatever shade the active theme uses. Switch
theme and the hints follow.

| Slots | Colors |
| --- | --- |
| 0 to 7 | black, red, green, yellow, blue, magenta, cyan, white |
| 8 to 15 | the bright versions, in the same order |

`38;5;N` sets the text to slot N and `48;5;N` the background. `1` is bold,
`4` underline, `7` reverse video. For bright magenta hints:

```lua
hint_style = "1;38;5;0;48;5;13",
highlight_style = "1;38;5;13",
```

### `dim`

`default: true`

Dim everything that is not a match, so the hints stand out. Set it to
`false` to keep the pane at full brightness under the hints.

## Coming from tmux-fingers

Every tmux-fingers option has a place here. The defaults are the same.

| tmux-fingers | rex-fingers | Note |
| --- | --- | --- |
| `@fingers-key` | `key` | A Rex key or sequence, such as `ctrl+b>shift+f` |
| `@fingers-jump-key` | none | A pane's cursor cannot be moved from outside in Rex |
| `@fingers-pattern-N` | `extra` | A list instead of numbered options |
| `@fingers-main-action` | `main_action` | Same values: `:copy:`, `:paste:`, `:open:`, or a command |
| `@fingers-ctrl-action` | `ctrl_action` | |
| `@fingers-shift-action` | `shift_action` | |
| `@fingers-alt-action` | `alt_action` | |
| `@fingers-hint-style` | `hint_style` | SGR numbers instead of tmux style words |
| `@fingers-highlight-style` | `highlight_style` | |
| `@fingers-backdrop-style` | `dim` | On or off; the dim attribute is the backdrop |
| `@fingers-selected-*-style` | none | No multi-select mode |
| `@fingers-hint-position` | `hint_position` | `left` or `right` |
| `@fingers-keyboard-layout` | `keyboard_layout` | Same twenty names |
| `@fingers-show-copied-notification` | none | |
| `@fingers-enabled-builtin-patterns` | `patterns` | A list of names; empty means all |
| `@fingers-use-system-clipboard` | `copy_command` | Always on; set the command |
| `@fingers-enable-bindings` | `key = false` | |
| `#{@fingers-cli} start ... --patterns x` | `rex.bind(key, "rex_fingers", { patterns = "x" })` | Per-key arguments |

Style words map to SGR numbers like this: `bold` is `1`, `underscore` is
`4`, `reverse` is `7`, `fg=green` is `38;5;2`, `bg=yellow` is `48;5;3`,
and the bright colors are slots 9 to 14. tmux-fingers' own default,
`fg=green,bold` hints over `fg=yellow` text, is:

```lua
hint_style = "1;38;5;2",
highlight_style = "38;5;3",
```

## Recipes

### A key per pattern set

The action takes a `patterns` argument that overrides the setup option for
one run, so one binding can show only URLs and another only git paths:

```lua
rex.bind("ctrl+b>u", "rex_fingers", { patterns = "url,markdown-url" })
rex.bind("ctrl+b>g", "rex_fingers", { patterns = "git-status,diff,sha" })
rex.bind("ctrl+b>e", "rex_fingers", { patterns = "file-line,path" })
```

### Send the match to a command

The action arguments work the same way. This key opens the picked file in
a new Neovim tab instead of copying it:

```lua
rex.bind("ctrl+b>o", "rex_fingers", {
  patterns = "file-line,path",
  main_action = "xargs -I{} rex run -- nvim {}",
})
```

The match arrives on the command's standard input, `MODIFIER` and `HINT` in
its environment, and the pane's directory as the working directory.

### Start without the prefix

```lua
fingers.setup{ binary = "...", key = "alt+f" }
```

Any Rex key works, including sequences such as `ctrl+space>f`.

### Paste a file:line into an editor

With Neovim in the pane, <kbd>Shift</kbd> + hint on `src/app.ts:10:5` types
the text at the cursor. Type `:e ` first and it opens the file at that
line.

## How it works

Rex has no API that paints into an existing terminal. A plugin can read a
pane, type into it, and open panes of its own, and that is all. So the
action opens a floating pane with exactly the focused pane's rectangle,
taken from `session.view`, and runs `rex-fingers` in it. The program
receives the pane's screen as VT sequences, captured by the Lua side
before the pane opens, redraws it dimmed, and paints the hints over the
matches. When you press a hint the program copies the text, exits, the
floating pane closes, and focus returns to your pane.

tmux-fingers works the same way: it swaps in a pane of its own with a
rendered copy of yours and swaps it back out.

## Limits

- The floating pane in Rex for Mac has a header, so the overlay is two rows
  shorter than the pane. `align` picks which two rows are hidden.
- Matches are found on the visible screen only, not in the scrollback.
- Multi-select and jump mode from tmux-fingers are not ported. Jump has no
  meaning here, because a pane's cursor cannot be moved from outside.
- Rex for Mac did not put an OSC 52 clipboard write on the system clipboard
  in testing, so copying runs a local command. On a remote Rex server, set
  `copy_command` to something that reaches your clipboard, such as an SSH
  command back to your machine.
- A pane whose program exits within a moment of starting stays open in Rex.
  The program lives at least 400 ms so a fast hint press does not trigger
  that.

## Troubleshooting

**Nothing happens on the key.** Run `rex config check`, then `rex keymap |
grep rex_fingers` to see whether the binding loaded and from which file.

**A blank pane shows for a moment, then the hints.** The first run after
building the program is slow while macOS checks the new binary, about a
third of a second. The second run starts in under a tenth. If every run is
slow, `binary` is probably unset and a login shell is starting first.

**The hints land on the wrong characters.** The overlay drew at a different
width than the pane. Open an issue with `rex block call -b PANE size` for
both the pane and the overlay.

**Lua errors.** `rex server logs` shows errors that the config check does
not catch, including a failed capture.

**Run it from a script.** `rex do rex_fingers -b BLOCK` opens the overlay on
that pane. Add `patterns=url` to limit it. Opened this way the app does not
size the overlay; use a real key press to see the layout.

## Development

```sh
go test ./...
go build -o bin/rex-fingers .
rex config reload
rex do rex_fingers -b "$REX_BLOCK"
```

`REX_FINGERS_TIMING=/tmp/t rex-fingers show ...` appends the time to the
first frame to that file. The package layout:

| Path | Holds |
| --- | --- |
| `main.go` | the `show`, `patterns`, and `layouts` commands, and the actions |
| `internal/screen` | VT dump to a grid of cells, with wide characters |
| `internal/patterns` | the pattern table and overlap resolution |
| `internal/hints` | prefix-free hint generation |
| `internal/ui` | raw mode, drawing, keys |
| `internal/rexcli` | the few `rex` CLI calls the program makes |
| `lua/rex-fingers/init.lua` | the Rex action and `setup` |
| `docs/CONFIGURING.md` | the setup walkthrough |
| `assets/` | the logo: `logo.svg` on its tile, `logo-mark.svg` transparent, `logo-mono-black.svg` and `logo-mono-white.svg` one color, `logo-adaptive.svg` following the page's color scheme, plus 512 px PNGs |
| `AGENTS.md` | the map of this repo for coding agents |

## Acknowledgements

- [tmux-fingers](https://github.com/Morantron/tmux-fingers) by Morantron,
  for the idea, the keys, the patterns, and the alphabet.
- [tmux-thumbs](https://github.com/fcsonline/tmux-thumbs) and
  [WezTerm quick select](https://wezfurlong.org/wezterm/quickselect.html),
  for the extra patterns.
- [rex-opener](https://github.com/shaheislam/rex-opener), for how a Rex
  plugin should present itself.

## License

MIT
