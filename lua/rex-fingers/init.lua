-- rex-fingers: letter hints over the paths, URLs, hashes, and other strings
-- on a pane. Press the hint to copy the match, Shift+hint to paste it into
-- the pane, Ctrl+hint to open it.
--
-- The action opens a floating pane exactly over the focused pane and runs
-- the rex-fingers program in it. That program captures the pane, redraws
-- it with hints, and closes itself once a hint is pressed.
local M = {}

local terminal = "com.superlogical.terminal"

-- Every option, with its default. The names and defaults follow
-- tmux-fingers where Rex allows it.
local defaults = {
  -- Absolute path of the rex-fingers binary. Empty looks it up on PATH
  -- through the login shell, which is slower to start.
  binary = "",
  -- Key to bind, or false to bind nothing.
  key = "ctrl+b>shift+f",
  -- Built-in pattern names to enable. Empty enables all of them.
  patterns = {},
  -- Extra regular expressions, Go syntax.
  extra = {},
  -- What a hint does: ":copy:", ":paste:", ":open:", or a shell command
  -- that reads the match on stdin. Every pick is copied first.
  main_action = ":copy:",
  shift_action = ":paste:",
  ctrl_action = ":open:",
  alt_action = "",
  -- Command that :copy: runs with the match on stdin. Empty picks pbcopy,
  -- wl-copy, xclip, or xsel.
  copy_command = "",
  -- Keyboard layout that picks the hint letters: qwerty, qwerty-homerow,
  -- colemak, dvorak, ... `rex-fingers layouts` lists them.
  keyboard_layout = "qwerty",
  -- Hint letters, overriding the layout.
  alphabet = "",
  -- Where the hint sits on the match: "left" or "right".
  hint_position = "left",
  -- SGR parameters for the hint letters and the rest of the match. The
  -- defaults name theme palette slots, so they follow the theme.
  hint_style = "",
  highlight_style = "",
  -- Dim everything that is not a match, so the hints stand out.
  dim = true,
  -- Which edge of the pane to keep when the floating pane is shorter.
  align = "bottom",
}

-- rect_of walks a window's layout tree to the block and returns its
-- rectangle as [x, y, w, h] fractions of the window.
local function rect_of(node, block_id)
  if node == nil then return nil end
  if node.block_id == block_id then return node.rect end
  if node.split then
    return rect_of(node.split.before, block_id) or rect_of(node.split.after, block_id)
  end
  return nil
end

-- locate returns the window ID and rectangle of a block in the session.
local function locate(session_id, block_id)
  local view, err = rex.session.view{ session_id = session_id }
  if not view then error(err, 0) end
  for _, w in ipairs(view.windows or {}) do
    for _, layer in ipairs(w.layers or {}) do
      local r = rect_of(layer.layout, block_id)
      if r then
        -- A floating layer's layout is relative to the layer's bounds.
        if layer.kind ~= "tiled" and layer.bounds then
          local b = layer.bounds
          r = { b[1] + r[1] * b[3], b[2] + r[2] * b[4], r[3] * b[3], r[4] * b[4] }
        end
        return w.window_id, r
      end
    end
  end
  error("rex-fingers: pane not found in the session", 0)
end

-- capture writes the pane's screen as VT to a temporary file before the
-- floating pane opens, so the program draws as soon as it starts.
local function capture(ctx)
  local target = { session_id = ctx.session_id, block_id = ctx.block_id }
  local size, err = rex.block.call(terminal, "size", target)
  if not size then error(err, 0) end
  local args = { format = "vt", max_bytes = 983040 }
  for k, v in pairs(target) do args[k] = v end
  local r
  r, err = rex.block.call(terminal, "format", args)
  if not r then error(err, 0) end
  local parts, offset = { r.content }, #r.content
  while r.id and not r.eof do
    r, err = rex.block.call(terminal, "blob_read", { session_id = ctx.session_id, block_id = ctx.block_id, id = r.id, offset = offset })
    if not r then error(err, 0) end
    parts[#parts + 1] = r.data
    offset = offset + #r.data
  end
  local path = os.tmpname()
  local f = assert(io.open(path, "w"))
  f:write(table.concat(parts))
  f:close()
  return path, size.rows
end

-- command builds the program's argument list. args are the action's own
-- arguments, which override the setup options for one run.
local function command(opts, ctx, args)
  args = args or {}
  local function pick(name)
    local v = args[name]
    if v == nil or v == "" then v = opts[name] end
    return v
  end
  local bin = opts.binary ~= "" and opts.binary or "rex-fingers"
  local path, rows = capture(ctx)
  local argv = { bin, "show", "--session", ctx.session_id, "--block", ctx.block_id,
                 "--vt-file", path, "--rows", tostring(rows), "--align", opts.align,
                 "--hint-position", opts.hint_position }
  if not opts.dim then argv[#argv + 1] = "--no-dim" end
  local patterns = args.patterns or (#opts.patterns > 0 and table.concat(opts.patterns, ",")) or ""
  local flags = {
    ["--patterns"] = patterns,
    ["--main-action"] = pick("main_action"),
    ["--shift-action"] = pick("shift_action"),
    ["--ctrl-action"] = pick("ctrl_action"),
    ["--alt-action"] = pick("alt_action"),
    ["--copy-command"] = pick("copy_command"),
    ["--keyboard-layout"] = opts.keyboard_layout,
    ["--alphabet"] = opts.alphabet,
    ["--hint-style"] = opts.hint_style,
    ["--highlight-style"] = opts.highlight_style,
  }
  for flag, value in pairs(flags) do
    if value ~= nil and value ~= "" then
      argv[#argv + 1] = flag
      argv[#argv + 1] = value
    end
  end
  for _, re in ipairs(opts.extra) do
    argv[#argv + 1] = "--extra"
    argv[#argv + 1] = re
  end
  return argv
end

function M.setup(user)
  local opts = {}
  for k, v in pairs(defaults) do opts[k] = v end
  for k, v in pairs(user or {}) do
    if defaults[k] == nil then error("rex-fingers: unknown option " .. tostring(k), 0) end
    opts[k] = v
  end

  rex.action{
    name = "rex_fingers",
    title = "Fingers: Copy by Hint",
    keywords = "copy hint path url sha tmux-fingers quick select",
    args = {
      patterns = "string?",
      main_action = "string?",
      shift_action = "string?",
      ctrl_action = "string?",
      alt_action = "string?",
      copy_command = "string?",
    },
    run = function(ctx, args)
      if not ctx.block_id then error("Fingers: no focused pane", 0) end
      local window_id, rect = locate(ctx.session_id, ctx.block_id)
      local layer, err = rex.session.new_layer{
        session_id = ctx.session_id,
        window_id = window_id,
        bounds = rect,
        focus = true,
        layout = rex.layout.block{
          flavor = terminal .. ".shell",
          label = "fingers",
          options = {
            command = command(opts, ctx, args),
            shell = opts.binary ~= "" and "none" or nil,
            exit = { on_completion = true },
          },
        },
      }
      if not layer then error(err, 0) end
      return { layer_id = layer.layer_id }
    end,
  }

  if opts.key then
    rex.bind(opts.key, "rex_fingers")
  end
end

return M
