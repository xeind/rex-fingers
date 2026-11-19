<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-claw-glass-rex-dark.png">
    <img src="assets/logo-claw-glass-rex.png" width="144" alt="rex-fingers: a dinosaur arm shaped like the letter F, with a claw on each finger, in frosted glass on a tile in Rex's colours">
  </picture>
</p>

# rex-fingers

Short arms, long reach.

rex-fingers puts letter hints over the paths, URLs, and hashes on a
[Rex](https://www.superlogical.com/rex) pane. Type a hint and the match
goes to the clipboard. It ports
[tmux-fingers](https://github.com/Morantron/tmux-fingers) to Rex.

It has two parts, installed at the same version:

- `rex-fingers`, a Go program. It finds the matches, draws the hints, and
  copies the pick.
- `lua/rex-fingers/init.lua`, a Lua plugin. Rex loads it; it binds the
  keys and runs the program.

## Requirements

| Need | Check | Get it |
| --- | --- | --- |
| Go 1.26 or newer | `go version` | `brew install go`, or https://go.dev/dl. An older Go downloads the 1.26 toolchain by itself. |
| git | `git --version` | `xcode-select --install` on a Mac |
| Rex | `rex config check` | https://www.superlogical.com/rex |
| A clipboard command | `command -v pbcopy` | `pbcopy` is part of macOS. On Linux: `wl-copy`, `xclip`, or `xsel`. |

Tested with Rex for Mac 0.1.0 (2027), in nushell, bash, and zsh. On Linux
only the unit tests have run. There is no prebuilt download.

## Install

1. Build the program and clone the plugin at the same tag:

   ```sh
   V=v0.3.1
   go install github.com/xeind/rex-fingers@$V
   git clone --branch $V --depth 1 https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
   "$(go env GOPATH)/bin/rex-fingers" version
   ```

   Check: the last command prints `v0.3.1`. The program is at
   `$(go env GOPATH)/bin/rex-fingers`, which is `~/go/bin/rex-fingers`
   unless `GOPATH` is set.

2. Add this to `~/.config/rex/init.lua`. `binary` must be the absolute
   path from step 1:

   ```lua
   local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
   fingers.setup{
     binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
   }
   ```

3. Load it:

   ```sh
   rex config check
   rex config reload
   rex keymap | grep rex_fingers
   ```

   Check: the last command prints two lines, `alt+f` and `alt+shift+f`.

rex-fingers makes no network call after the install.

## Copy to agent

Paste this into a coding agent to have it do the install:

```text
Install rex-fingers for me in Rex.
Read the section "Install it for a user" in
https://raw.githubusercontent.com/xeind/rex-fingers/main/AGENTS.md
and follow it.
In setup, give binary as an absolute path.
Run rex config check. Then ask me before you reload my config.
Report which keys you bound: alt+f and alt+shift+f.
```

## Use

| Key | Does |
| --- | --- |
| <kbd>Alt-f</kbd> | hint the focused pane |
| <kbd>Alt-Shift-f</kbd> | hint every pane in the window |

On a Mac, <kbd>Alt</kbd> is <kbd>Option</kbd>. Each match gets a hint of
one or two letters, and the rest of the pane dims. Then:

| Keys | Does |
| --- | --- |
| hint | copy the match |
| <kbd>Shift</kbd> + hint | paste it into the pane |
| <kbd>Ctrl</kbd> + hint | open it with `open` or `xdg-open`; `ctrl_action = ":reveal:"` shows a file in Finder instead |
| <kbd>Alt</kbd> + hint | nothing until `alt_action` is set |
| <kbd>q</kbd>, <kbd>Esc</kbd>, <kbd>Ctrl-c</kbd>, or the trigger key again | leave |

In every-pane mode a paste goes into the pane where the key was pressed.
`rex-fingers patterns` lists what gets a hint.

## Configuration

Every option, with its default. An unknown name fails `rex config check`.

```lua
fingers.setup{
  binary = "",                    -- absolute path of rex-fingers; set it, or a login shell starts first
  key = "alt+f",                  -- any Rex key or sequence; false binds nothing
  window_key = "alt+shift+f",     -- a key that hints every pane of the window; false binds nothing
  overlay = "pane",               -- "pane": draw into the pane; "float": a floating pane over yours
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
  hint_style = "",                -- SGR for the hint letters; empty means 1;38;5;0;48;5;12
  highlight_style = "",           -- SGR for the rest of the match; empty means 1;38;5;12
  dim = true,                     -- dim everything that is not a match
  flash = false,                  -- show the pick in reverse video for 150 ms
  follow_scroll = false,          -- experimental: hint the rows the focused pane is scrolled to
}
```

- **Actions:** `:copy:`, `:paste:`, `:open:`, `:reveal:`, `""` for
  nothing, or a shell command run with `sh -c`. `:open:` opens a URL, a
  folder, or a file with its default app. `:reveal:` opens a URL or a
  folder the same way, selects a file in Finder, and opens the nearest
  folder of a path that is gone. Both take a relative path, `~`, and
  `file.go:12` from the pane's directory. The command reads the match on standard input
  and runs in the pane's directory, with `MODIFIER` and `HINT` in its
  environment. Every pick is copied first.
- **Styles:** the numbers inside `ESC [ ... m`. Use palette slots such as
  `38;5;12`, not hex colors, so the theme applies.
- **Overlay:** `pane` draws into the pane, so every row stays visible.
  `float` opens a floating pane two rows shorter and never writes to the
  pane. Every-pane mode always draws in the panes.

More: [docs/CONFIGURING.md](docs/CONFIGURING.md) has a table from each
wish to its option and a walkthrough with a check per step.
[docs/TMUX-FINGERS.md](docs/TMUX-FINGERS.md) maps every tmux-fingers
option.

## Limits

- Hints cover the live screen. Scroll a pane to the bottom first, or see
  `follow_scroll` below.
- With `overlay = "float"`, the float hides the top two rows of the pane.
- There is no multi-select and no jump mode. Rex cannot move a pane's
  cursor from outside.
- On a remote Rex server, set `copy_command` to a command that reaches
  your clipboard, such as `ssh laptop pbcopy`.
- With `overlay = "pane"` or every-pane mode, output that a program prints
  while hints show stays on screen, and the hints stop narrowing as you
  type.
- Rows that scroll into the scrollback while hints show keep their hint
  letters. `overlay = "float"` never writes to the pane and leaves none.

## Experimental: `follow_scroll`

`follow_scroll = true` hints the rows the focused pane is scrolled to. It
is off by default and needs `overlay = "pane"` or every-pane mode. Rex
reports no scroll position, so on each trigger the app copies the visible
rows in copy mode, and a helper takes them from the clipboard. The costs:

- Copy mode flashes, and the hints come up about half a second later.
- The helper puts the clipboard back as text, so a copied image is lost.
- The Rex server must run on the machine the app runs on.
- A scrolled pane loses what its program prints while the hints are up.
- In every-pane mode only the focused pane follows its scroll; the others
  show their live screens.

The option goes when Rex reports a scroll position.

## Troubleshooting

| Symptom | Cause | Do |
| --- | --- | --- |
| `go: command not found` | Go is not installed. | `brew install go`, then repeat step 1. |
| Nothing happens on the key. | The binding did not load. | `rex config check`, then `rex keymap \| grep rex_fingers`. It prints one line per bound key. |
| The first run is slow. | macOS checks a new binary once. | Nothing. If every run is slow, `binary` is unset. |
| Hint letters stayed in the pane. | A config reload or an app restart happened while hints were up. | Press the key again, then <kbd>Esc</kbd>, or run `rex do rex_fingers_cancel`. |
| Hint letters stayed in the scrollback. | The pane printed while hints were up, and the painted rows scrolled off the screen. No restore reaches them. | On a pane that prints, use `overlay = "float"`. |
| `flag provided but not defined` | The program is older than the Lua file. | Install both at the same tag. `rex do rex_fingers -b "$REX_BLOCK"` in the pane prints the error. |
| A Lua error the config check misses. | | `rex server logs` shows it. |

## License

MIT. [AGENTS.md](AGENTS.md) maps the code for contributors and agents.
