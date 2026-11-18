<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-claw-glass-rex-dark.png">
    <img src="assets/logo-claw-glass-rex.png" width="144" alt="rex-fingers: a dinosaur arm shaped like the letter F, with a claw on each finger, in frosted glass on a tile in Rex's colours">
  </picture>
</p>

# rex-fingers

Short arms, long reach.

rex-fingers puts letter hints over the paths, URLs, hashes, and other
strings on a [Rex](https://www.superlogical.com/rex) pane. Type a hint and
the match goes to your clipboard. It ports
[tmux-fingers](https://github.com/Morantron/tmux-fingers) to Rex.

You need Go 1.26 or newer, the `rex` command, and a clipboard command
(`pbcopy`, `wl-copy`, `xclip`, or `xsel`). Rex is in beta; this was tested
with Rex for Mac 0.1.0 (2027). An older Go downloads a newer toolchain
before it builds.

## Install

1. Install the program and clone the plugin:

   ```sh
   go install github.com/xeind/rex-fingers@latest
   git clone https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
   ```

2. Add this to `~/.config/rex/init.lua`. `binary` is an absolute path to
   the program; `go install` puts it in `~/go/bin` unless you set `GOPATH`.

   ```lua
   local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
   fingers.setup{
     binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
   }
   ```

3. Run `rex config check`, then `rex config reload`.

## Copy to agent

Paste this into Claude Code, Codex, or OpenCode, and it installs rex-fingers
for you.

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

On a Mac, <kbd>Alt</kbd> is <kbd>Option</kbd>. Everything that is not a match
dims, and each match gets a one or two letter hint. Type the hint:

| Keys | Does |
| --- | --- |
| <kbd>a</kbd>-<kbd>z</kbd> | copy the match |
| <kbd>Shift</kbd> + hint | paste it into the pane |
| <kbd>Ctrl</kbd> + hint | open it with `open` or `xdg-open` |
| <kbd>Alt</kbd> + hint | nothing until you set `alt_action` |
| <kbd>q</kbd>, <kbd>Esc</kbd>, <kbd>Ctrl-c</kbd> | leave |
| the same key again | leave |

`rex-fingers patterns` lists what gets a hint. A paste from every-pane
mode goes into the pane where you pressed the key.

## Configuration

Every option, with its default. An unknown name fails `rex config check`.
[docs/CONFIGURING.md](docs/CONFIGURING.md) walks through it, and
[docs/TMUX-FINGERS.md](docs/TMUX-FINGERS.md) maps tmux-fingers options.

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
  align = "bottom",               -- float only: the pane edge to keep, since the float is two rows shorter
  follow_scroll = false,          -- with "pane": hint the rows the focused pane is scrolled to
}
```

**Actions** are `:copy:`, `:paste:`, `:open:`, `""` for nothing, or a shell
command run with `sh -c`. The command reads the match on standard input and
runs in the pane's directory, with `MODIFIER` and `HINT` in its
environment. Every pick is copied first.

**Styles** are the numbers inside `ESC [ ... m`. The defaults name palette
slots, so they follow your theme.

**Overlay.** `pane` draws into your pane, so every row stays visible.
`float` opens a floating pane over yours, two rows shorter, and never
writes to your pane. Every-pane mode always draws in the panes.

**Scrolled panes.** `follow_scroll = true` hints the rows the focused pane
is scrolled to. It needs `overlay = "pane"` or every-pane mode. Rex reports
no scroll position, so each trigger has the app copy the visible rows in
copy mode. Copy mode flashes, and a helper waits about half a second for
the rows, then puts your clipboard back as text. In every-pane mode only
the focused pane follows its scroll; the others show their live screens.

## Limits

- Hints cover the live screen. A scrolled pane gets hints on the rows it
  shows only when it has focus and `follow_scroll = true`. Without that,
  scroll to the bottom first.
- With `follow_scroll`, a copied image is lost from the clipboard, the Rex
  server must run on the machine the app runs on, and a scrolled pane
  loses what its program prints while the hints are up.
- With `overlay = "float"`, the float hides two rows of the pane; `align`
  picks which.
- No multi-select or jump mode. Rex cannot move a pane's cursor from outside.
- On a remote Rex server, set `copy_command` to a command that reaches your
  clipboard, such as `ssh laptop pbcopy`.
- With `overlay = "pane"` or every-pane mode, output a program prints while
  hints show stays on screen, and the hints stop narrowing as you type.
  Rows that scroll into the scrollback keep their hint letters. A float
  never writes to the pane, so it leaves nothing behind.

## Troubleshooting

**Nothing happens on the key.** Run `rex config check`, then
`rex keymap | grep rex_fingers`. It prints one line for each bound key.

**The first run is slow.** macOS checks a new binary once. If every run is
slow, `binary` is unset.

**Hint letters stayed in the pane.** A config reload or an app restart
while the hints were up left them there. Press the key again, then
<kbd>Esc</kbd>, or run `rex do rex_fingers_cancel`.

**Hint letters stayed in the scrollback.** The pane printed while the
hints were up, and the painted rows scrolled off the screen. No restore
reaches them. On a pane that prints, use `overlay = "float"`.

**`flag provided but not defined`.** The program is older than the Lua
file. Install both from the same version. To see the error, run
`rex do rex_fingers -b "$REX_BLOCK"` in the pane.

**Lua errors.** `rex server logs` shows errors the config check misses.

## License

MIT. [AGENTS.md](AGENTS.md) maps the code for contributors.
