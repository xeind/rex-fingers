# Configuring rex-fingers

A walkthrough from a fresh Rex config to a tuned setup. Each step ends with
a check, so you know it worked before moving on. The whole thing takes
about ten minutes.

## Quick reference

| You want | Set in `setup` |
| --- | --- |
| A different key | `key = "alt+g"`. Any Rex key or sequence works, such as tmux-fingers' `"ctrl+b>shift+f"`. |
| Another key for every pane, or none | `window_key = "alt+g"`, or `false` |
| No key from the plugin | `key = false`, then `rex.bind("alt+f", "rex_fingers")` in your own bindings |
| Hints in a floating pane that never writes to your pane | `overlay = "float"`. It hides two rows. |
| Hints on the rows the focused pane is scrolled to | `follow_scroll = true`, with `overlay = "pane"`. The cost: copy mode flashes, the clipboard is saved and restored as text, and the server must run on the app's machine. |
| Other hint letters | `keyboard_layout = "colemak-homerow"`, from `rex-fingers layouts`; or `alphabet = "asdfghjkl"`, which wins over the layout |
| Hint on the other end of a match | `hint_position = "right"` or `"gutter"` |
| Other colors | `hint_style` and `highlight_style`, SGR with palette slots such as `"1;38;5;0;48;5;13"`. Use slots, never hex, so the theme applies. |
| No dimming, or a flash on a pick | `dim = false`; `flash = true` |
| Other actions per modifier | `main_action`, `shift_action`, `ctrl_action`, `alt_action`: `:copy:`, `:paste:`, `:open:`, `""`, or a shell command that reads the match on stdin |
| Fewer or more patterns | `patterns = { "path", "url" }`; `extra = { [[\bISSUE-\d+\b]] }`; names from `rex-fingers patterns` |
| One key for one job | `rex.bind("alt+u", "rex_fingers", { patterns = "url,markdown-url" })`, with `key = false` for the plain one if wanted |
| A remote Rex server | `copy_command = "ssh my-laptop pbcopy"` |

Layouts leave out `c`, `i`, `m`, `q`, and `n`, as tmux-fingers does. A
tmux-fingers user finds every option mapped in
[TMUX-FINGERS.md](TMUX-FINGERS.md).

## 1. Get the program and the plugin

Build the program with Go 1.26 or newer and clone the plugin next to your
config, both at the same tag so they match:

```sh
V=v0.3.0
go install github.com/xeind/rex-fingers@$V
git clone --branch $V --depth 1 https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
"$(go env GOPATH)/bin/rex-fingers" version
```

Check: the last command prints `v0.3.0`. Note the full path it ran from;
the next step needs it.

## 2. Load the plugin

Add to `~/.config/rex/init.lua`, with the path from step 1 as `binary`:

```lua
local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
fingers.setup{
  binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
}
```

```sh
rex config check
rex config reload
rex keymap | grep rex_fingers
```

Check: the last command prints two lines: `alt+f` and `alt+shift+f`, each
with the file and line the binding came from.

## 3. Try it

Print something with a path and a URL, then press <kbd>Alt-f</kbd>:

```sh
echo "/usr/local/bin/rex https://example.com"
```

Check: the pane dims, `a` appears over the path and `s` over the URL. Press
`a`, then paste somewhere. You get the path.

If nothing appears, see [Troubleshooting](../README.md#troubleshooting).

## 4. Pick your key

The default is <kbd>Alt-f</kbd>. Any Rex key or sequence works, including
tmux-fingers' key on a Ctrl-b prefix:

```lua
fingers.setup{ binary = "...", key = "ctrl+b>shift+f" }
```

Or keep your bindings in one place: set `key = false` and bind it yourself.

```lua
fingers.setup{ binary = "...", key = false }
rex.bind("alt+f", "rex_fingers")
```

Check: `rex config reload`, then `rex keymap | grep rex_fingers` shows the
new key.

## 5. Pick your letters

Hints come from the keyboard layout, `qwerty` by default. If you type
Colemak or Dvorak, or want only the home row:

```lua
keyboard_layout = "colemak-homerow",
```

`rex-fingers layouts` prints every layout with its letters. For a custom
set, `alphabet = "..."` wins over the layout.

Check: after reload, the hints use the new letters.

## 6. Pick your colors

The defaults are dark text on the theme's bright blue, with everything else
dimmed. The numbers are SGR parameters that name theme palette slots, so
they follow whatever theme Rex is using.

| Want | `hint_style` | `highlight_style` |
| --- | --- | --- |
| bright blue, the default | `"1;38;5;0;48;5;12"` | `"1;38;5;12"` |
| bright magenta | `"1;38;5;0;48;5;13"` | `"1;38;5;13"` |
| bright green | `"1;38;5;0;48;5;10"` | `"1;38;5;10"` |
| tmux-fingers' green on yellow | `"1;38;5;2"` | `"38;5;3"` |
| no color, reverse video | `"1;7"` | `"1;4"` |

Slots 1 to 6 are red, green, yellow, blue, magenta, cyan; 9 to 14 are the
bright versions. `1` is bold, `4` underline, `7` reverse. Set `dim = false`
to keep the pane at full brightness.

Check: after reload, press the key and look.

## 7. Decide what the keys do

Four actions, one per modifier. The defaults are tmux-fingers':

```lua
main_action = ":copy:",    -- hint
shift_action = ":paste:",  -- Shift+hint
ctrl_action = ":open:",    -- Ctrl+hint
alt_action = "",           -- Alt+hint
```

Any of them can be a shell command instead. It gets the match on standard
input, `MODIFIER` and `HINT` in the environment, and runs in the pane's
directory. Every pick is copied to the clipboard before the action runs.

```lua
alt_action = "xargs -I{} rex run -- nvim {}",
```

Check: Alt+hint on a path opens it in a new tab.

## 8. Limit or extend the patterns

Every built-in pattern except `digit` is on by default. To keep only some:

```lua
patterns = { "path", "file-line", "url" },
```

To add your own, in Go regular expression syntax, with `(?<match>...)`
around the part to copy:

```lua
extra = { [[\bISSUE-\d+\b]], [[Error: (?<match>.+)]] },
```

Check: `rex config check` fails with the pattern's position if an
expression is invalid. Otherwise print a matching line and press the key.

## 9. One key per job

Bindings can pass arguments that override the setup for one run:

```lua
rex.bind("alt+u", "rex_fingers", { patterns = "url,markdown-url" })
rex.bind("alt+g", "rex_fingers", { patterns = "git-status,diff,sha" })
rex.bind("alt+o", "rex_fingers", { patterns = "file-line,path", main_action = "xargs -I{} rex run -- nvim {}" })
```

The arguments are `patterns`, `main_action`, `shift_action`, `ctrl_action`,
`alt_action`, `copy_command`, `follow_scroll = "false"` to hint the live
screen on that key when the option is on, and `scope = "window"`. Window scope hints
every pane of the window. It draws in the panes and reads the hint keys
through key functions, with no Rex key mode.

Check: `rex actions | grep rex_fingers` lists them, and each key shows only
its own matches.

## 10. On a remote server

The clipboard command runs on the machine that runs the Rex server. When
that is not the machine you sit at, point `copy_command` back home:

```lua
copy_command = "ssh my-laptop pbcopy",
```

Check: a pick lands on your local clipboard.

## A complete example

```lua
local fingers = dofile(os.getenv("HOME") .. "/.config/rex/plugins/rex-fingers/lua/rex-fingers/init.lua")
fingers.setup{
  binary = os.getenv("HOME") .. "/go/bin/rex-fingers",
  key = false,
  keyboard_layout = "qwerty-homerow",
  hint_style = "1;38;5;0;48;5;13",
  highlight_style = "1;38;5;13",
  alt_action = "xargs -I{} rex run -- nvim {}",
  extra = { [[\bISSUE-\d+\b]] },
}
rex.bind("alt+f", "rex_fingers")
rex.bind("alt+u", "rex_fingers", { patterns = "url,markdown-url" })
```
