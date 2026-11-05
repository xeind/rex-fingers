-- rex-fingers: letter hints over the paths, URLs, hashes, and other strings
-- on a pane. Press the hint to copy the match, Shift+hint to paste it into
-- the pane, Ctrl+hint to open it.
--
-- The action opens a floating pane exactly over the focused pane and runs
-- the rex-fingers program in it. That program captures the pane, redraws
-- it with hints, and closes itself once a hint is pressed.
local M = {}

local terminal = "com.superlogical.terminal"

local defaults = {
  -- Path of the rex-fingers binary. Empty means look it up on PATH through
  -- the login shell, which is slower to start.
  binary = "",
  -- Key to bind, or false to bind nothing.
  key = "ctrl+b>shift+f",
  -- Built-in pattern names to enable. Empty enables all of them.
  patterns = {},
  -- Extra regular expressions, Go syntax.
  extra = {},
  -- Command that reads the text to copy on stdin. Empty picks pbcopy,
  -- wl-copy, or xclip.
  copy_command = "",
  -- Letters used for hints.
  alphabet = "",
  -- Which edge of the pane to keep when the floating pane is shorter.
  align = "bottom",
  -- SGR parameters for the hint letters and the rest of the match.
  hint_style = "",
  highlight_style = "",
  -- Dim everything that is not a match, so the hints stand out.
  dim = true,
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

local function command(opts, ctx)
  local bin = opts.binary ~= "" and opts.binary or "rex-fingers"
  local path, rows = capture(ctx)
  local argv = { bin, "show", "--session", ctx.session_id, "--block", ctx.block_id, "--align", opts.align,
                 "--vt-file", path, "--rows", tostring(rows) }
  if not opts.dim then argv[#argv + 1] = "--no-dim" end
  if #opts.patterns > 0 then
    argv[#argv + 1] = "--patterns"
    argv[#argv + 1] = table.concat(opts.patterns, ",")
  end
  for _, re in ipairs(opts.extra) do
    argv[#argv + 1] = "--extra"
    argv[#argv + 1] = re
  end
  for flag, value in pairs{ ["--copy-command"] = opts.copy_command, ["--alphabet"] = opts.alphabet,
                            ["--hint-style"] = opts.hint_style, ["--highlight-style"] = opts.highlight_style } do
    if value ~= "" then
      argv[#argv + 1] = flag
      argv[#argv + 1] = value
    end
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
    run = function(ctx)
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
            command = command(opts, ctx),
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
