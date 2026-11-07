# Coming from tmux-fingers

Every tmux-fingers option has a place in rex-fingers. The defaults are the
same.

| tmux-fingers | rex-fingers | Note |
| --- | --- | --- |
| `@fingers-key` | `key` | A Rex key or sequence, such as `alt+f` or `ctrl+b>shift+f` |
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
| `@fingers-hint-position` | `hint_position` | `left`, `right`, or `gutter` |
| `@fingers-keyboard-layout` | `keyboard_layout` | Same twenty names |
| `@fingers-show-copied-notification` | `flash` | A reverse-video flash of the pick instead of a message |
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
