# Configuring rex-fingers

A walkthrough from a fresh Rex config to a tuned setup. Each step ends with
a check, so you know it worked before moving on. The whole thing takes
about ten minutes.

## 1. Build the program

```sh
go install github.com/xeind/rex-fingers@latest
"$(go env GOPATH)/bin/rex-fingers" version
```

Check: the second command prints a version number. Note the full path it
ran from; the next step needs it.

## 2. Load the plugin

Clone it next to your config:

```sh
git clone https://github.com/xeind/rex-fingers ~/.config/rex/plugins/rex-fingers
```

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

Check: the last command prints one line with `ctrl+b shift+f` and the file
and line the binding came from.

## 3. Try it

Print something with a path and a URL, then press <kbd>Ctrl-b</kbd>
<kbd>F</kbd>:

```sh
echo "/usr/local/bin/rex https://example.com"
```

Check: the pane dims, `a` appears over the path and `s` over the URL. Press
`a`, then paste somewhere. You get the path.

If nothing appears, see [Troubleshooting](../README.md#troubleshooting).

## 4. Pick your key

The default is <kbd>Ctrl-b</kbd> then <kbd>Shift-f</kbd>, the tmux-fingers
default on the tmux prefix. Any Rex key works:

```lua
fingers.setup{ binary = "...", key = "alt+f" }
```

Or keep your bindings in one place: set `key = false` and bind it yourself.

```lua
fingers.setup{ binary = "...", key = false }
rex.bind("ctrl+b>shift+f", "rex_fingers")
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

All nineteen built-in patterns are on by default. To keep only some:

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
rex.bind("ctrl+b>u", "rex_fingers", { patterns = "url,markdown-url" })
rex.bind("ctrl+b>g", "rex_fingers", { patterns = "git-status,diff,sha" })
rex.bind("ctrl+b>o", "rex_fingers", { patterns = "file-line,path", main_action = "xargs -I{} rex run -- nvim {}" })
```

The arguments are `patterns`, `main_action`, `shift_action`, `ctrl_action`,
`alt_action`, and `copy_command`.

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
rex.bind("ctrl+b>shift+f", "rex_fingers")
rex.bind("ctrl+b>u", "rex_fingers", { patterns = "url,markdown-url" })
```
