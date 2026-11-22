<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-claw-glass-rex-dark.png">
    <img src="assets/logo-claw-glass-rex.png" width="144" alt="rex-fingers: a dinosaur arm shaped like the letter F, with a claw on each finger, in frosted glass on a tile in Rex's colours">
  </picture>
</p>

# rex-fingers

Short arms, long reach.

Letter hints over the paths, URLs, and hashes in a
[Rex](https://www.superlogical.com/rex) pane. Type a hint and the match
goes to the clipboard. A port of
[tmux-fingers](https://github.com/Morantron/tmux-fingers).

<p align="center">
  <img src="assets/demo-window-hints.gif" width="960" alt="Alt-Shift-f in a Rex window with nvim, Claude Code, and a shell: every path, URL, and commit hash in the three panes gets a red letter hint">
</p>

## Install

You need Go 1.26 or newer, git, Rex, and a clipboard command: `pbcopy`
on a Mac, or `wl-copy`, `xclip`, or `xsel` on Linux.

1. Clone the plugin and build the program into its `bin/` folder:

   ```sh
   git clone --branch v0.4.1 --depth 1 https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
   go build -C ~/.config/rex/plugins/rex-fingers -o bin/rex-fingers .
   ~/.config/rex/plugins/rex-fingers/bin/rex-fingers version   # prints v0.4.1
   ```

   The program lands there whatever `GOBIN` or `GOPATH` say.

2. Add to `~/.config/rex/init.lua`:

   ```lua
   local fingers_dir = os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers"
   local fingers = dofile(fingers_dir .. "/lua/rex-fingers/init.lua")
   fingers.setup{
     binary = fingers_dir .. "/bin/rex-fingers",
   }
   ```

3. Run `rex config check`, then `rex config reload`.
   `rex keymap | grep rex_fingers` should list `alt+f` and `alt+shift+f`.

Tested with Rex for Mac 0.1.0 (2044) in nushell, bash, and zsh. On Linux
only the unit tests have run. rex-fingers makes no network call after the
install.

## Update

Move the clone to the new tag and build again:

```sh
git -C ~/.config/rex/plugins/rex-fingers fetch --depth 1 origin tag v0.4.1
git -C ~/.config/rex/plugins/rex-fingers checkout v0.4.1
go build -C ~/.config/rex/plugins/rex-fingers -o bin/rex-fingers .
```

Then run `rex config reload`.

An older install used `go install`, and its `binary` names
`~/go/bin/rex-fingers` or another Go folder. After the update, set
`binary` as in install step 2, then delete the old program. It is in `go env GOBIN`, or in
`$(go env GOPATH)/bin` when `GOBIN` is empty. A build with `GOPATH` set to
the plugin folder left a read-only `pkg/` folder there. Delete it:

```sh
chmod -R u+w ~/.config/rex/plugins/rex-fingers/pkg
rm -r ~/.config/rex/plugins/rex-fingers/pkg
```

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
| hint | copy the match |
| <kbd>Shift</kbd> + hint | paste it into the pane |
| <kbd>Ctrl</kbd> + hint | open it; with `ctrl_action = ":reveal:"`, show a file in Finder |
| <kbd>Esc</kbd>, <kbd>q</kbd>, <kbd>Space</kbd>, <kbd>Ctrl-c</kbd>, or the trigger again | leave |

On a Mac, <kbd>Alt</kbd> is <kbd>Option</kbd>. `rex-fingers patterns`
lists what gets a hint.

## Configuration

Every option, with its default. An unknown name fails `rex config check`.

```lua
fingers.setup{
  binary = "",                    -- absolute path of rex-fingers; set it: the in-pane runs search the Rex server's PATH
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
  nothing, or a shell command that reads the match on standard input,
  with `MODIFIER`, `HINT`, `REX_SESSION`, and `REX_BLOCK` set. Every pick is copied first. `:open:`
  and `:reveal:` take a relative path, `~`, and `file.go:12`, from the
  pane's directory.
- **Styles:** SGR numbers. Use palette slots such as `38;5;12`, not hex,
  so the theme applies.

[docs/CONFIGURING.md](docs/CONFIGURING.md) maps each wish to its option.
[docs/TMUX-FINGERS.md](docs/TMUX-FINGERS.md) maps every tmux-fingers
option.

## Limits

- Hints cover the live screen. `follow_scroll = true` hints the rows a
  scrolled pane shows instead, at a cost: copy mode flashes, the hints
  come half a second later, a copied image on the clipboard is lost, and
  the Rex server must run on the app's machine.
- In a scrolled pane, a key the plugin binds does not jump back to the
  bottom. Scroll down, or type a key that is not a hint.
- Output a program prints while hints show stays on screen, and rows that
  scroll off keep their hint letters. `overlay = "float"` never writes to
  the pane, but hides its top two rows.
- No multi-select and no jump mode: Rex cannot move a pane's cursor from
  outside.
- On a remote Rex server, set `copy_command` to reach your clipboard,
  such as `ssh laptop pbcopy`.

## Troubleshooting

| Symptom | Do |
| --- | --- |
| `go: command not found` | `brew install go`, then run the `go build` line of install step 1. |
| Nothing happens on the key. | `rex config check`, then `rex keymap \| grep rex_fingers`. |
| Every run is slow. | Set `binary`. The first run after an install is slow once while macOS checks the program. |
| An action or `copy_command` says `command not found`. | Actions get the Rex server's `PATH`, `/usr/bin:/bin:/usr/sbin:/sbin` on a Mac. Name the program by its absolute path. |
| Hint letters stayed in the pane. | Press the key again, then <kbd>Esc</kbd>, or run `rex do rex_fingers_cancel`. A config reload while hints were up causes this. |
| `flag provided but not defined` | The program is older than the plugin. Run the `go build` line again, and check that `binary` names the clone's `bin/rex-fingers`. |
| A Lua error the config check misses. | `rex server logs` shows it. |

## License

MIT. [AGENTS.md](AGENTS.md) maps the code for contributors and agents.
