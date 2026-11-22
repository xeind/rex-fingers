-- rex-fingers: letter hints over the paths, URLs, hashes, and other strings
-- on a pane. Press the hint to copy the match, Shift+hint to paste it into
-- the pane, Ctrl+hint to open it.
--
-- With overlay = "float", the action opens a floating pane exactly over the
-- focused pane and runs the rex-fingers program in it. That program redraws
-- the captured pane with hints and closes itself once a hint is pressed.
--
-- With overlay = "pane", the program draws the hints into the focused
-- pane's own tty. The hint letters are function key bindings, bound once
-- when the config loads. While a run is active on the focused pane they take
-- the key; otherwise they return nothing and the key goes on to the pane.
-- The pane is repainted from the capture when a hint is picked or the run is
-- cancelled. There is no key mode, so Rex for Mac shows no key list.
local M = {}

local terminal = "com.superlogical.terminal"

-- Every option, with its default. The names and defaults follow
-- tmux-fingers where Rex allows it.
local defaults = {
  -- Absolute path of the rex-fingers binary. Empty looks it up on PATH:
  -- the float through the login shell, which is slower to start; the
  -- in-pane runs on the Rex server's PATH, /usr/bin:/bin:/usr/sbin:/sbin
  -- on a Mac, which holds no rex-fingers.
  binary = "",
  -- Key to bind, or false to bind nothing. tmux-fingers' key is
  -- "ctrl+b>shift+f".
  key = "alt+f",
  -- Built-in pattern names to enable. Empty enables all but digit.
  patterns = {},
  -- Extra regular expressions, Go syntax.
  extra = {},
  -- What a hint does: ":copy:", ":paste:", ":open:", ":reveal:", or a shell command
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
  -- Where the hint sits: "left" or "right" covers that end of the match;
  -- "gutter" uses the blank cells before it and falls back to "right".
  hint_position = "left",
  -- SGR parameters for the hint letters and the rest of the match. The
  -- defaults name theme palette slots, so they follow the theme.
  hint_style = "",
  highlight_style = "",
  -- Dim everything that is not a match, so the hints stand out.
  dim = true,
  -- Show the picked match in reverse video for a moment before the pane
  -- comes back, as feedback that it was copied.
  flash = false,
  -- Where the hints are drawn: "pane", the pane itself, or "float", a
  -- floating pane over the pane, two rows shorter.
  overlay = "pane",
  -- Key that hints every pane of the window at once, or false to bind
  -- nothing. Always draws in the panes, whatever overlay says. A pick
  -- pastes into the pane the key was pressed in.
  window_key = "alt+shift+f",
  -- Hint the rows a pane shows when it is scrolled up, not its live
  -- screen. In-pane drawing only. Rex reports no scroll position, so each
  -- trigger has the app copy the visible rows in copy mode: copy mode
  -- flashes, and the clipboard is saved and put back, text only.
  follow_scroll = false,
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

-- open_floats returns the fingers floating panes in the window that holds
-- block_id, which may be one of them.
local function open_floats(session_id, block_id)
  local view, err = rex.session.view{ session_id = session_id }
  if not view then error(err, 0) end
  for _, w in ipairs(view.windows or {}) do
    local ids, found = {}, false
    for _, layer in ipairs(w.layers or {}) do
      for _, b in ipairs(layer.blocks or {}) do
        if b.block_id == block_id then found = true end
        if layer.kind ~= "tiled" and b.label == "fingers" and b.creator_name == terminal then
          ids[#ids + 1] = b.block_id
        end
      end
    end
    if found then return ids end
  end
  return {}
end

-- dump writes one format call on a pane, read in blobs if it is large, to
-- a temp file and returns the path. target holds session_id and block_id.
local function dump(target, format_args)
  local args = { session_id = target.session_id, block_id = target.block_id, max_bytes = 983040 }
  for k, v in pairs(format_args) do args[k] = v end
  local r, err = rex.block.call(terminal, "format", args)
  if not r then error(err, 0) end
  local parts, offset = { r.content }, #r.content
  while r.id and not r.eof do
    r, err = rex.block.call(terminal, "blob_read", { session_id = target.session_id, block_id = target.block_id, id = r.id, offset = offset })
    if not r then error(err, 0) end
    parts[#parts + 1] = r.data
    offset = offset + #r.data
  end
  local path = os.tmpname()
  local f = assert(io.open(path, "w"))
  f:write(table.concat(parts))
  f:close()
  return path
end

-- capture writes the pane's screen to temporary files before the program
-- starts, so the program draws as soon as it starts.
local function capture(ctx)
  local size, err = rex.block.call(terminal, "size", { session_id = ctx.session_id, block_id = ctx.block_id })
  if not size then error(err, 0) end
  -- The VT dump redraws the screen row by row. The unwrapped text says
  -- which rows the terminal wrapped, so a match can span them.
  local vt = dump(ctx, { format = "vt" })
  local text = dump(ctx, { format = "text", unwrap = true, trim = true })
  return vt, text, size
end

-- currents captures every pane of a run as VT, as it is now, so the
-- program can tell whether a pane wrote output since the hints went up.
-- The text format drops blank rows at the foot of the screen; the VT keeps
-- every screen row. Returns one path per block of the run.
local function currents(run)
  local paths = {}
  for i, id in ipairs(run.blocks) do
    paths[i] = dump({ session_id = run.session_id, block_id = id }, { format = "vt" })
  end
  return paths
end

-- option_flags appends the flags that pick and style the hints and say what
-- a hint does. args are the action's own arguments, which override the
-- setup options for one run.
local function option_flags(argv, opts, args)
  args = args or {}
  local function pick(name)
    local v = args[name]
    if v == nil or v == "" then v = opts[name] end
    return v
  end
  argv[#argv + 1] = "--hint-position"
  argv[#argv + 1] = opts.hint_position
  if not opts.dim then argv[#argv + 1] = "--no-dim" end
  if opts.flash then
    argv[#argv + 1] = "--flash"
    argv[#argv + 1] = "150"
  end
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

local function binary(opts)
  return opts.binary ~= "" and opts.binary or "rex-fingers"
end

-- command builds the argument list of the program in the floating pane.
local function command(opts, ctx, args)
  local path, text, size = capture(ctx)
  local argv = { binary(opts), "show", "--session", ctx.session_id, "--block", ctx.block_id,
                 "--vt-file", path, "--text-file", text, "--rows", tostring(size.rows),
                 "--cols", tostring(size.columns) }
  return option_flags(argv, opts, args)
end

-- The in-pane run. state holds one run per block: the argv the program
-- needs on every call, and the hint letters typed so far.
local state = {}

local function mode_set(modes, number)
  for _, m in ipairs(modes.modes or {}) do
    if m.number == number then return m.set end
  end
  return true
end

-- exec runs the program with argv and the extra flags, and decodes the
-- JSON object it prints.
local function exec(argv, ...)
  local full = {}
  for _, a in ipairs(argv) do full[#full + 1] = a end
  for _, a in ipairs({ ... }) do full[#full + 1] = a end
  -- A key function may run 5 seconds; after that Rex drops the key.
  local r, err = rex.exec(full, { timeout = 4 })
  if not r then return nil, err end
  if not r.ok then return nil, "rex-fingers: " .. (r.stderr ~= "" and r.stderr or ("exit " .. tostring(r.code))) end
  return rex.json.decode(r.stdout)
end

-- finish forgets the run. With restore, it also repaints the pane as it
-- was.
local function finish(block_id, run, restore)
  state[block_id] = nil
  for _, id in ipairs((run and run.blocks) or {}) do state[id] = nil end
  if run and restore then exec(run.argv("restore", currents(run))) end
end

-- recover_pane puts a pane back when a run was left on it without
-- restore, such as after a config reload, and the pane still shows the
-- hints. The record of the run here is gone, so the program decides from
-- the state it kept beside the capture.
local function recover_pane(opts, ctx)
  local target = { session_id = ctx.session_id, block_id = ctx.block_id }
  local process = rex.block.call(terminal, "process", target)
  local fg = process and (process.foreground or process.child)
  if not fg then return end
  local path, text, size = capture(ctx)
  exec({ binary(opts), "restore", "--recover", "--pid", tostring(fg.pid), "--vt-file", path,
         "--text-file", text, "--rows", tostring(size.rows), "--cols", tostring(size.columns) })
  os.remove(path)
  os.remove(text)
end

local function start_in_pane(opts, ctx, args)
  local target = { session_id = ctx.session_id, block_id = ctx.block_id }
  local modes, err = rex.block.call(terminal, "modes", target)
  if not modes then error(err, 0) end
  local process
  process, err = rex.block.call(terminal, "process", target)
  if not process then error(err, 0) end
  local fg = process.foreground or process.child
  if not fg then error("Fingers: no process in the pane", 0) end
  local pwd = rex.block.call(terminal, "pwd", target)
  local path, text, size = capture(ctx)
  local cursor = mode_set(modes, 25) and "true" or "false"
  local wrap = mode_set(modes, 7) and "true" or "false"
  -- Flags that draw, press, and restore all take.
  local tail = { "--vt-file", path, "--text-file", text, "--rows", tostring(size.rows), "--cols", tostring(size.columns),
                 "--cursor-visible=" .. cursor, "--wraparound=" .. wrap,
                 "--cwd", (pwd and pwd.pwd) or "" }
  option_flags(tail, opts, args)
  local r
  r, err = exec({ binary(opts), "draw" }, "--begin", "--pid", tostring(fg.pid), unpack(tail))
  if not r then
    os.remove(path)
    os.remove(text)
    error(err, 0)
  end
  if r.hints == 0 then
    os.remove(path)
    os.remove(text)
    return { hints = 0 }
  end
  -- argv builds a press or restore call; current is the pane's text
  -- captured now, one path per block.
  local function argv(sub, current)
    local a = { binary(opts), sub, "--tty", r.tty }
    for _, x in ipairs(tail) do a[#a + 1] = x end
    if current and current[1] then
      a[#a + 1] = "--current-file"
      a[#a + 1] = current[1]
    end
    return a
  end
  local run = { argv = argv, typed = "", busy = false,
                session_id = ctx.session_id, origin = ctx.block_id, blocks = { ctx.block_id } }
  state[ctx.block_id] = run
  return { hints = r.hints }
end

-- window_blocks lists the terminal blocks of the window that holds
-- block_id, that block first. Layout nodes carry no flavor, so the
-- layers' block lists are read instead.
local function window_blocks(session_id, block_id)
  local view, err = rex.session.view{ session_id = session_id }
  if not view then error(err, 0) end
  for _, w in ipairs(view.windows or {}) do
    local ids, found = {}, false
    for _, layer in ipairs(w.layers or {}) do
      for _, b in ipairs(layer.blocks or {}) do
        if b.creator_name == terminal then
          if b.block_id == block_id then found = true else ids[#ids + 1] = b.block_id end
        end
      end
    end
    if found then
      table.insert(ids, 1, block_id)
      return ids
    end
  end
  error("rex-fingers: pane not found in the session", 0)
end

-- pane_spec captures one pane and returns its --pane value for draw and
-- the capture's path, or nil when the pane has no process to draw into.
local function pane_spec(session_id, block_id)
  local target = { session_id = session_id, block_id = block_id }
  local process = rex.block.call(terminal, "process", target)
  local fg = process and (process.foreground or process.child)
  if not fg then return nil end
  local modes = rex.block.call(terminal, "modes", target) or {}
  local path, text, size = capture(target)
  local spec = string.format("pid=%d,vt=%s,text=%s,rows=%d,cols=%d,cursor=%s,wrap=%s",
    fg.pid, path, text, size.rows, size.columns,
    mode_set(modes, 25) and "true" or "false", mode_set(modes, 7) and "true" or "false")
  return spec, path, text
end

-- start_panes draws hints into the terminal panes in blocks at once. The
-- panes share one hint list. The pane the key was pressed in comes first:
-- a paste lands there and a command runs in its directory.
--
-- With await, a count, the program does not draw yet. It returns the ttys
-- and waits in the background for the app to copy the rows the first
-- await panes show, in the order of blocks, then draws the page each of
-- them is scrolled to. The caller queues those copies. It returns the
-- result and the blocks that take part.
local function start_panes(opts, ctx, args, blocks, await)
  local pwd = rex.block.call(terminal, "pwd", { session_id = ctx.session_id, block_id = ctx.block_id })
  local specs, paths, members = {}, {}, {}
  for _, id in ipairs(blocks) do
    local spec, path, text = pane_spec(ctx.session_id, id)
    if spec then
      specs[#specs + 1] = spec
      paths[#paths + 1] = path
      paths[#paths + 1] = text
      members[#members + 1] = id
    end
  end
  if #specs == 0 then error("Fingers: no process in the panes", 0) end
  local function cleanup() for _, p in ipairs(paths) do os.remove(p) end end
  local tail = { "--cwd", (pwd and pwd.pwd) or "" }
  option_flags(tail, opts, args)
  local argv = { binary(opts), "draw", "--begin" }
  if await then
    argv[#argv + 1] = "--await-views"
    argv[#argv + 1] = tostring(await)
  end
  for _, s in ipairs(specs) do
    argv[#argv + 1] = "--pane"
    argv[#argv + 1] = s
  end
  local r, err = exec(argv, unpack(tail))
  if not r then
    cleanup()
    error(err, 0)
  end
  -- A waiting draw has no count yet; the program deletes the captures
  -- itself when it finds no hints.
  if not await and r.hints == 0 then
    cleanup()
    return { hints = 0 }, {}
  end
  -- Later calls name each pane by the tty draw found, not by pid, and
  -- carry each pane's text captured now when current is given.
  local function argv(sub, current)
    local a = { binary(opts), sub }
    for i, s in ipairs(specs) do
      local spec = s:gsub("^pid=%d+", "tty=" .. r.panes[i].tty)
      if current and current[i] then spec = spec .. ",current=" .. current[i] end
      a[#a + 1] = "--pane"
      a[#a + 1] = spec
    end
    for _, x in ipairs(tail) do a[#a + 1] = x end
    return a
  end
  local run = { argv = argv, typed = "", busy = false,
                session_id = ctx.session_id, origin = ctx.block_id, blocks = members }
  for _, id in ipairs(members) do state[id] = run end
  return { hints = r.hints, waiting = r.waiting, panes = #members }, members
end

function M.setup(user)
  local opts = {}
  for k, v in pairs(defaults) do opts[k] = v end
  for k, v in pairs(user or {}) do
    if defaults[k] == nil then error("rex-fingers: unknown option " .. tostring(k), 0) end
    opts[k] = v
  end
  if opts.overlay ~= "float" and opts.overlay ~= "pane" then
    error("rex-fingers: overlay must be \"float\" or \"pane\"", 0)
  end
  -- The in-pane keys are bound only when a run can draw in a pane.
  local in_pane_keys = opts.overlay == "pane" or opts.window_key

  local function start(ctx, args, scope)
    if scope == "window" then
      return (start_panes(opts, ctx, args, window_blocks(ctx.session_id, ctx.block_id)))
    end
    return start_in_pane(opts, ctx, args)
  end

  -- follow_scroll hints the page each pane is scrolled to. Rex reports no scroll
  -- position, so the app copies the rows each pane shows, in copy mode,
  -- and the program takes them from the clipboard. The program is started
  -- first: it saves the clipboard, returns, and waits in the background.
  -- Then the copies are queued; the app runs them after this action
  -- returns. Nothing calls back into Lua, because the one way the app has,
  -- a key sent to the pane, moves a scrolled pane to its last row.
  --
  -- Copy mode acts on the focused pane alone, so only that pane's page is
  -- read. In window scope the other panes are hinted on their live
  -- screens. Reading them in turn with pane.focus_next between the copies
  -- worked when the rex CLI ran the queue, one action per call. On a key
  -- press the app runs the whole queue at once, and the focus has not
  -- moved by the time the next copy runs: every copy was of the first
  -- pane.
  local function follow_scroll(ctx, args, scope)
    local blocks = { ctx.block_id }
    if scope == "window" then blocks = window_blocks(ctx.session_id, ctx.block_id) end
    local result = start_panes(opts, ctx, args, blocks, 1)
    rex.client.queue("pane.copy_mode.enter")
    rex.client.queue("pane.copy_mode.move", { to = "screen_top" })
    rex.client.queue("pane.copy_mode.select", { mode = "line" })
    rex.client.queue("pane.copy_mode.move", { to = "screen_bottom" })
    rex.client.queue("pane.copy_mode.yank")
    -- The yank leaves copy mode but keeps its selection on screen, a grey
    -- box over the whole pane. Starting a new selection in copy mode and
    -- leaving drops it; entering and leaving alone does not.
    rex.client.queue("pane.copy_mode.enter")
    rex.client.queue("pane.copy_mode.select", { mode = "char" })
    rex.client.queue("pane.copy_mode.exit")
    return result
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
      scope = "string?",
      follow_scroll = "string?",
    },
    run = function(ctx, args)
      if not ctx.block_id then error("Fingers: no focused pane", 0) end
      local scope = args.scope or "pane"
      if scope ~= "pane" and scope ~= "window" then
        error("Fingers: scope must be \"pane\" or \"window\"", 0)
      end
      if scope == "window" and not in_pane_keys then
        error("Fingers: window scope needs overlay = \"pane\" or window_key", 0)
      end
      -- The key toggles: with hints already up in the pane, it puts the
      -- pane back.
      local active = state[ctx.block_id]
      if active then
        finish(ctx.block_id, active, true)
        return { closed = 1 }
      end
      -- With hints already up in a float, it sends that float Escape,
      -- which closes it and gives the pane focus back.
      local floats = open_floats(ctx.session_id, ctx.block_id)
      if #floats > 0 then
        for _, id in ipairs(floats) do
          rex.block.call(terminal, "write", { session_id = ctx.session_id, block_id = id, data = "\27" })
        end
        return { closed = #floats }
      end
      -- Window scope always draws in the panes: a float covers one pane,
      -- and only the in-pane keys let one key reach every pane.
      if opts.overlay == "pane" or scope == "window" then
        -- A run is left behind when the config reloads or the app
        -- restarts. draw --begin picks it up from the state the program
        -- kept, if the pane still shows its hints, and starts again from
        -- the pane as it was.
        -- rex do from a shell has origin "api" and no app to copy the
        -- rows, so it hints the live screen. follow_scroll = "true" on the
        -- call says an app is there, as with rex -C; "false" turns it
        -- off for one binding.
        local follow = opts.follow_scroll and ctx.origin ~= "api" and ctx.origin ~= "cli"
        if args.follow_scroll == "true" then follow = opts.follow_scroll end
        if args.follow_scroll == "false" then follow = false end
        -- A machine with no command that reads the clipboard cannot
        -- follow; the live screen is hinted then.
        if follow then
          local ok, result = pcall(follow_scroll, ctx, args, scope)
          if ok then return result end
          rex.log("warn", "Fingers: follow_scroll: " .. tostring(result))
        end
        return start(ctx, args, scope)
      end
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
  if opts.window_key then
    rex.bind(opts.window_key, "rex_fingers", { scope = "window" })
  end

  -- The in-pane keys, used by overlay = "pane" and by window scope under
  -- either overlay. They are function bindings with no key mode, so Rex
  -- lists no keys. Rex cannot bind a key while a run is active, so every
  -- key below is bound when the config loads. Each one checks for a run on
  -- the focused pane: with none it returns nothing, and Rex hands the key
  -- to the pane as if it were unbound.
  rex.action{
    name = "rex_fingers_cancel",
    title = "Fingers: Cancel",
    run = function(ctx)
      local run = state[ctx.block_id]
      if not run then pcall(recover_pane, opts, ctx) end
      finish(ctx.block_id, run, true)
    end,
  }

  -- handle sends one hint key to the program and acts on its answer.
  local function handle(ctx, run, key, mod)
    local r, err = exec(run.argv("press", currents(run)), "--typed", run.typed, "--key", key, "--modifier", mod)
    if not r then error(err, 0) end
    if not r.done then
      run.typed = r.typed or ""
      return
    end
    finish(ctx.block_id, run, false)
    if r.paste then
      local ok, perr = rex.block.call(terminal, "write",
        { session_id = ctx.session_id, block_id = run.origin or ctx.block_id, data = r.paste })
      if not ok then error(perr, 0) end
    end
    if r.error then error("Fingers: " .. r.error, 0) end
  end

  -- press runs a hint key and takes it. A key that lands while the
  -- previous one still runs is dropped, not raced, and also taken, so the
  -- letter does not reach the pane. A busy mark older than 5 seconds, the
  -- longest a key function runs, is stale.
  local function press(ctx, key, mod)
    local run = state[ctx.block_id]
    if not run then return nil end
    local now = os.time()
    if run.busy and now - run.busy < 5 then return true end
    run.busy = now
    local ok, err = pcall(handle, ctx, run, key, mod)
    run.busy = nil
    if not ok then
      rex.log("error", "Fingers: " .. tostring(err))
      if state[ctx.block_id] == run then pcall(finish, ctx.block_id, run, true) end
    end
    return true
  end

  -- cancel puts the pane back and takes the key.
  local function cancel(ctx)
    local run = state[ctx.block_id]
    if not run then return nil end
    pcall(finish, ctx.block_id, run, true)
    return true
  end

  -- swallow takes a key during a run, so it does not type into the pane.
  local function swallow(ctx)
    if state[ctx.block_id] then return true end
    return nil
  end

  -- A float with no window key reads its own keys, so nothing below is
  -- bound and no key the user types passes through Lua.
  if not in_pane_keys then return end

  -- Never bind a key that starts or ends a sequence: tmux's prefix, or the
  -- keys that open fingers.
  local reserved = { ["ctrl+b"] = true }
  if opts.key then reserved[opts.key] = true end
  if opts.window_key then reserved[opts.window_key] = true end
  local function bind_key(key, fn)
    if not reserved[key] then rex.bind(key, fn) end
  end

  -- A modifier gets a hint binding only when its action is set. Every
  -- layout leaves out c, i, m, q, and n. A key that is no hint is not
  -- bound: a bound key does not return a scrolled pane to its last row
  -- (REX-FACTS.md), so during a run it types into the pane instead.
  local mods = { { "", "main" } }
  for _, m in ipairs({ { "shift+", "shift" }, { "ctrl+", "ctrl" }, { "alt+", "alt" } }) do
    if opts[m[2] .. "_action"] ~= "" then mods[#mods + 1] = m end
  end
  local hinted = {}
  for key in (opts.alphabet ~= "" and opts.alphabet or "abdefghjkloprstuvwxyz"):gmatch("%w") do
    hinted[key:lower()] = true
  end
  for key in ("abcdefghijklmnopqrstuvwxyz"):gmatch(".") do
    if hinted[key] then
      for _, m in ipairs(mods) do
        if not (key == "q" and m[2] == "main") and not (key == "c" and m[2] == "ctrl") then
          bind_key(m[1] .. key, function(ctx) return press(ctx, key, m[2]) end)
        end
      end
    end
  end
  -- Space closes the hints too: typed during a run, it went to the
  -- program unseen under a scrolled page. Bound, it does not return a
  -- scrolled pane when no run is up.
  for _, key in ipairs({ "escape", "q", "ctrl+c", "space" }) do bind_key(key, cancel) end
  -- Enter stays taken: one that reached the pane would run the prompt.
  bind_key("enter", swallow)
end

return M
