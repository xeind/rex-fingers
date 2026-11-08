<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-claw-glass-rex-dark.png">
    <img src="assets/logo-claw-glass-rex.png" width="144" alt="rex-fingers: a dinosaur arm shaped like the letter F, with a claw on each finger, in frosted glass on a tile in Rex's colours">
  </picture>
</p>

# rex-fingers

Short arms, long reach.

Letter hints over the paths, URLs, hashes, and other strings on a
[Rex](https://www.superlogical.com/rex) pane. Press a hint and the match is on
your clipboard. A port of [tmux-fingers](https://github.com/Morantron/tmux-fingers)
to Rex.

## Install

```sh
go install github.com/xeind/rex-fingers@latest
git clone https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
```

```lua
-- ~/.config/rex/init.lua
local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
fingers.setup{
  binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
}
```

`binary` is where `go install` put the program, `$(go env GOPATH)/bin`,
which is `~/go/bin` unless you changed it. Then `rex config check` and
`rex config reload`. The key is <kbd>Alt-f</kbd>, <kbd>Option-f</kbd> on a
Mac; `key` takes any Rex key or sequence, such as tmux-fingers'
`ctrl+b>shift+f`.

With [Amber](https://github.com/rawnly/rex-package-manager):

```lua
{ "xeind/rex-fingers", opts = { binary = os.getenv("HOME") .. "/go/bin/rex-fingers" } },
```

Needs Go 1.24 or newer, the `rex` command on the server's PATH, and a
clipboard command: `pbcopy`, `wl-copy`, `xclip`, or `xsel`. Tested with Rex
for Mac 0.1.0 (1825). Rex is in beta and its API moves between builds.

## Use

Press <kbd>Alt-f</kbd>. Everything that is not a match dims and each match
gets a one or two letter hint. Type the hint.

| Keys | Does |
| --- | --- |
| <kbd>a</kbd>-<kbd>z</kbd> | copy the match |
| <kbd>Shift</kbd> + hint | paste it into the pane |
| <kbd>Ctrl</kbd> + hint | open it with `open` or `xdg-open` |
| <kbd>Alt</kbd> + hint | nothing until you set `alt_action` |
| <kbd>q</kbd>, <kbd>Esc</kbd>, <kbd>Ctrl-c</kbd> | leave |
| <kbd>Alt-f</kbd> again | leave, with `overlay = "float"` |

The same text gets the same hint wherever it appears. With more matches than
letters, some hints are two letters.

## What gets a hint

| Pattern | Example | Copies |
| --- | --- | --- |
| `path` | `/usr/local/bin/rex`, `~/.config/rex/init.lua` | the path |
| `quoted-path` | `'/tmp/a b/c'` | the path without quotes |
| `file-line` | `src/app.ts:10:5` | path and position |
| `url` | `https://example.com/x?y=1`, `git@github.com:a/b.git` | the URL |
| `markdown-url` | `[docs](https://example.com/docs)` | the URL only |
| `email` | `someone@example.com` | the address |
| `sha` | `deadbeefcafe1234` | the hash; needs a digit and must stand alone |
| `uuid` | `3b241101-e2bb-4255-8caf-4136c566a962` | the UUID |
| `hex` | `0x7ffee3` | the number |
| `color` | `#c6b18e` | the color |
| `ip`, `ipv6` | `10.0.0.42`, `fe80::1c3a:ff:fe12:3456%en0` | the address |
| `semver` | `v2.1.318-beta.1` | the version |
| `digit` | `8080` | numbers of four or more digits; off unless named in `patterns` |
| `kubernetes`, `kubernetes-pod` | `pod/nginx-7xb2r`, `nginx-deployment-66b6c48dd5-7xb2r` | the resource or pod |
| `git-status`, `git-status-branch` | `modified:   src/main.go` | the path or branch |
| `diff` | `--- a/internal/ui/ui.go` | the path only |

Where two patterns overlap, the leftmost and then the longest match wins.
File:line matches, paths, and URLs get the first and shortest hints, then
hashes and git paths, then the rest. `rex-fingers patterns` prints every
expression.

## Configuration

Every option, with its default. Unknown names fail at `rex config check`.
A step-by-step walkthrough is [docs/CONFIGURING.md](docs/CONFIGURING.md).

```lua
fingers.setup{
  binary = "",                    -- absolute path of rex-fingers; set it, or a login shell starts first
  key = "alt+f",                  -- any Rex key or sequence; false binds nothing
  window_key = false,             -- a key that hints every pane of the window; shows Rex's key list popup
  overlay = "float",              -- "float": a floating pane over yours; "pane": draw into the pane, with the popup
  patterns = {},                  -- built-in names to enable; empty is all but digit
  extra = {},                     -- more patterns, Go regexps; (?<match>...) picks the copied part
  main_action = ":copy:",         -- hint
  shift_action = ":paste:",       -- Shift + hint
  ctrl_action = ":open:",         -- Ctrl + hint
  alt_action = "",                -- Alt + hint
  copy_command = "",              -- what :copy: runs; empty finds pbcopy, wl-copy, xclip, or xsel
  keyboard_layout = "qwerty",     -- which letters are hints; rex-fingers layouts lists the names
  alphabet = "",                  -- the hint letters themselves, overriding the layout
  hint_position = "left",         -- "left" or "right" end of the match, or the blank "gutter" before it, else its end
  hint_style = "1;38;5;0;48;5;12",-- SGR for the hint letters
  highlight_style = "1;38;5;12",  -- SGR for the rest of the match
  dim = true,                     -- dim everything that is not a match
  flash = false,                  -- show the pick in reverse video for 150 ms
  align = "bottom",               -- float only: the pane edge to keep, since the float is two rows shorter
}
```

**Actions** are `:copy:`, `:paste:`, `:open:`, `""` for nothing, or a shell
command run with `sh -c`, the match on standard input, `MODIFIER` and
`HINT` in the environment, in the pane's directory. Every pick is copied
first.

**Styles** are the numbers inside `ESC [ ... m`. The defaults name palette
slots, so they follow your theme: slot 0 is black, 1 to 7 red through
white, 8 to 15 the bright versions. `38;5;N` is text, `48;5;N` background,
`1` bold, `7` reverse.

**Overlay.** `float`, the default, opens a floating pane over yours, two
rows shorter, and reads the keys itself. `pane` draws into your pane, so
every row stays visible and output printed while the hints are up is kept.
It puts Rex in a key mode named `fingers`, and Rex for Mac shows a popup
listing that mode's keys; no setting hides it. Fewer letters in
`alphabet`, or empty `shift_action` and `ctrl_action`, make the list
shorter.

> **Note.** I want `pane` to be the default: hints drawn in your own pane,
> no rows hidden. It still has bugs, and the popup gets in the way. Once
> Rex adds an option to hide a key mode's popup, `pane` becomes the
> default.

**Window scope.** `window_key`, or `scope = "window"` on a binding, hints
every pane of the window at once and pastes into the pane you pressed the
key in. A float covers one pane, so window scope always draws in the panes
and uses the key mode, popup included, whatever `overlay` says.

**Per-key arguments.** `patterns`, the four actions, and `copy_command` can
also be passed on a binding, for that key only, plus `scope = "window"`:

```lua
fingers.setup{ binary = "...", key = false }
rex.bind("alt+f", "rex_fingers")
rex.bind("alt+u", "rex_fingers", { patterns = "url,markdown-url" })
rex.bind("alt+o", "rex_fingers", { patterns = "file-line,path", main_action = "xargs -I{} rex run -- nvim {}" })
```

Coming from tmux-fingers: [docs/TMUX-FINGERS.md](docs/TMUX-FINGERS.md)
maps every option.

## Limits

- Matches are found on the visible screen, not in the scrollback.
- With `overlay = "float"`, the floating pane in Rex for Mac is two rows
  shorter than the pane; `align` picks which rows are hidden.
- A wrapped path or URL is one match and copies whole, except when the
  wrap falls right before a wide character such as an emoji.
- No multi-select or jump mode. A pane's cursor cannot be moved from
  outside in Rex.
- On a remote Rex server, set `copy_command` to something that reaches
  your clipboard, such as `ssh laptop pbcopy`.
- With `overlay = "pane"` or window scope, a program that writes to the
  pane while the hints are up ends the run on the next key: nothing is
  picked, rows that still show hints are put back, and the new output
  stays. Hint letters on a row the program overwrote stay until something
  redraws that row. Rows that scrolled into the scrollback keep their hint
  letters for good, since nothing written to a terminal reaches its
  scrollback. A float never writes to the pane, so it leaves nothing
  behind.

## Troubleshooting

**Nothing happens on the key.** `rex config check`, then `rex keymap | grep
rex_fingers` shows whether the binding loaded.

**A blank pane shows for a moment.** The first run after a build is slow
while macOS checks the binary. If every run is slow, `binary` is unset.

**A popup lists dozens of keys.** That is Rex's list for the key mode,
used by `overlay = "pane"` and window scope. Use `overlay = "float"` and
leave `window_key` unset to avoid it.

**Hint letters stayed in the pane.** With `overlay = "pane"` or window
scope, a config reload or an app restart while the hints were up ended
the key mode without putting the pane back.
Press the key again, or `Escape`. The program restores the pane from the
capture it took, as long as the pane still shows what it drew.

**Hint letters stayed in the scrollback.** The pane printed while the
hints were up, such as a running build or Claude Code, and the painted
rows scrolled off the screen before `Escape`. No restore can reach them;
clearing the pane's scrollback removes them. On a pane that is printing,
use the float key, not window scope.

**Lua errors.** `rex server logs` shows errors the config check misses.

## Development

`go test ./...`, then `go build -o bin/rex-fingers .`. `AGENTS.md` maps the
code and the headless test loops; `scripts/test_in_pane.sh` runs the
in-pane lifecycle suite.

## Acknowledgements

[tmux-fingers](https://github.com/Morantron/tmux-fingers) by Morantron gave
the idea, the keys, the patterns, and the alphabet. The extra patterns come
from [tmux-thumbs](https://github.com/fcsonline/tmux-thumbs) and
[WezTerm quick select](https://wezfurlong.org/wezterm/quickselect.html).
[rex-opener](https://github.com/shaheislam/rex-opener) showed how a Rex
plugin should present itself.

## License

MIT
