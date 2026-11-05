# rex-fingers

Short arms, long reach. Letter hints over the paths, URLs, hashes, and other
strings on a [Rex](https://www.superlogical.com/rex) pane. Press the hint and
the match is on your clipboard. A port of
[tmux-fingers](https://github.com/Morantron/tmux-fingers) to Rex.

Press the key, and every match on the focused pane gets a one or two letter
hint drawn over it, with the pane's own colors underneath. Type the hint:

| Keys | Result |
| --- | --- |
| `a`-`z` | Copy the match to the clipboard |
| `Shift` + hint | Type the match into the pane |
| `Ctrl` + hint | Open the match with `open` or `xdg-open` |
| `Backspace` | Drop the last typed letter |
| `q`, `Esc`, `Ctrl-C` | Leave without copying |

Built-in patterns: file paths, quoted paths with spaces, files with a line
number such as `main.go:42`, URLs, Markdown links, email addresses, git SHAs,
UUIDs, hex numbers, `#rrggbb` colors, IPv4 and IPv6 addresses, semantic
versions, numbers of four or more digits, Kubernetes resources and pod names,
and the paths in `git status` and `diff` output. The first eleven come from
tmux-fingers; the rest were added from tmux-thumbs and WezTerm's quick
select. `rex-fingers patterns` prints them all.

## How it works

Rex has no API that paints into an existing terminal, so the plugin opens a
floating pane with exactly the focused pane's bounds and runs `rex-fingers`
in it. The program captures the pane with colors, redraws it, and draws the
hints on top. When you press a hint the program exits, the floating pane
closes, and focus returns to the pane. tmux-fingers works the same way.

## Requirements

- Rex with the `rex` CLI on the server's PATH. Tested with Rex for Mac
  0.1.0 (1825) and CLI `0.0.1793741575-g1b6654`; Rex is in beta and its API
  moves between builds
- Go 1.24 or newer to build
- A clipboard command: `pbcopy` on macOS, `wl-copy`, `xclip`, or `xsel` on Linux

## Install

Build the binary:

```sh
go install github.com/xeind/rex-fingers@latest
```

Clone the plugin next to your Rex config and load it from `init.lua`:

```sh
git clone https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
```

```lua
local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
fingers.setup{
  binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
  key = "ctrl+b>shift+f",
}
```

Then:

```sh
rex config check
rex config reload
```

With [Amber](https://github.com/rawnly/rex-package-manager):

```lua
{ "xeind/rex-fingers", opts = { binary = os.getenv("HOME") .. "/go/bin/rex-fingers" } }
```

Set `binary` to an absolute path. Without it the floating pane starts your
login shell to find `rex-fingers` on PATH, which adds a visible delay.

## Options

Every option has a default. Pass only the ones you change.

| Option | Default | Meaning |
| --- | --- | --- |
| `binary` | `""` | Absolute path of `rex-fingers` |
| `key` | `"ctrl+b>shift+f"` | Key to bind, or `false` for none |
| `patterns` | `{}` | Built-in pattern names to enable; empty means all |
| `extra` | `{}` | Extra regular expressions, Go syntax. A `(?<match>...)` group limits what is copied |
| `copy_command` | `""` | Shell command that reads the text on stdin |
| `alphabet` | `"asdfwerzxvjkluopghtyb"` | Letters used for hints |
| `align` | `"bottom"` | Edge of the pane to keep if the floating pane is shorter |
| `hint_style` | `"1;38;5;0;48;5;12"` | SGR parameters for the hint letters |
| `highlight_style` | `"1;38;5;12"` | SGR parameters for the rest of a match |
| `dim` | `true` | Dim everything that is not a match |

Example:

```lua
fingers.setup{
  binary = "/usr/local/bin/rex-fingers",
  patterns = { "path", "url", "sha" },
  extra = { [[git rebase --(abort|continue)]], [[Error: (?<match>.+)]] },
  hint_style = "1;38;5;0;48;5;4",
}
```

## Colors follow the theme

The defaults name palette slots, not fixed colors. Hints are palette color
0 on palette color 12, and the rest of a match is palette color 12, the
theme's bright blue, which reads as a link. Everything else is dimmed, as
tmux-fingers does. Palette
indexes `38;5;N` for text and `48;5;N` for background, with `N` from 0 to
15, map to the theme's sixteen ANSI colors, so any choice follows the theme.
Set `dim = false` to keep the screen at full brightness.

The action is `rex_fingers`, shown as "Fingers: Copy by Hint" in the
command palette. Run it from a script with `rex do rex_fingers -b BLOCK`.

## Limits

- A floating pane in Rex for Mac has a header, so the overlay is two rows
  shorter than the pane. With `align = "bottom"` the top two rows of the
  pane are not shown. Set `align = "top"` to keep the top instead.
- Matches are found on the visible screen only, not in the scrollback.
- Multi-select and jump mode from tmux-fingers are not ported. Jump has no
  meaning here, because the pane's cursor cannot be moved from outside.
- Rex for Mac does not put OSC 52 writes on the clipboard, so copying uses a
  local clipboard command. On a remote server, set `copy_command` to
  something that reaches your clipboard.

## Development

```sh
go test ./...
go build -o bin/rex-fingers .
rex config reload
rex do rex_fingers -b "$REX_BLOCK"
```

## License

MIT
