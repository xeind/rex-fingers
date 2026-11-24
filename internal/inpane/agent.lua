-- The rex-fingers host agent: one run of hints on panes of a Rex host.
--
-- The plugin and the program run on the app's machine, where every call
-- to the host is a round trip. This script runs on the host, under the
-- host's own `rex do`, in a hidden block the plugin starts. Its calls are
-- local. It reads the panes, sends them to the plugin, draws the frame
-- the program made, reads the hint keys, puts the panes back, and reports
-- the pick. Nothing is installed: the script arrives in the block's argv.
--
-- The plugin reads this block's screen, and writes to its terminal:
--   out  RXF1<...>          the panes, once read
--   in   RUN <bytes>\n...   the frames, restores, and hints, as JSON
--   in   END\n              no hints; put the focus back and stop
--   out  RXF2<...>          the pick, or how the run ended
--   in   W <pane> <bytes>\n...  bytes for a pane's tty, after RXF2
--   in   BYE\n              the last W is sent; stop
-- Its arguments, a JSON object: pane, scope, limit in seconds, reader,
-- the argv and mark of the key reader, and view, the rows the pane shows
-- when it is scrolled up, as the app copied them. With keys = "hidden"
-- the reader is a hidden block, with no pane and no focus: the plugin
-- writes each hint key to its terminal, from an action the key is bound
-- to in a key mode of the app. With keys = "substitute" and
-- reader.substitute, the reader is a substitute for the pane: it starts
-- in the small floating pane, and once the app has loaded it, it takes
-- the pane's place with the same frame, and the pane waits detached.
local terminal = "com.superlogical.terminal"
local args = rex.args
local sid = rex.session_id
local self_id = os.getenv("REX_BLOCK")
local started = os.time()

-- trace appends a line to /tmp/rex-fingers-trace on the host, when that
-- file exists there: create it to see what a run did, remove it after.
local trace_path = "/tmp/rex-fingers-trace"
local tracing = io.open(trace_path, "r")
if tracing then tracing:close() end
local function trace(...)
  if not tracing then return end
  local f = io.open(trace_path, "a")
  if not f then return end
  local parts = { os.date("%H:%M:%S") }
  for i = 1, select("#", ...) do parts[#parts + 1] = tostring((select(i, ...))) end
  f:write(table.concat(parts, " "), "\n")
  f:close()
end

-- send prints a message for the plugin as one unbroken word, so the
-- terminal's wrapping and trimming cannot change it.
local function send(mark, value)
  local text = rex.json.encode(value):gsub("[^%w]", function(c)
    return string.format("%%%02X", c:byte())
  end)
  io.stdout:write(mark .. "<" .. text .. ">\r\n")
  io.stdout:flush()
end

local function call(block, method, more)
  local a = { session_id = sid, block_id = block }
  for k, v in pairs(more or {}) do a[k] = v end
  return rex.block.call(terminal, method, a)
end

-- tail returns the end of a format call on a pane, and whether that is
-- the whole output. The output holds the whole scrollback, and only the
-- screen is wanted.
local function tail(block, format_args, bytes)
  format_args.max_bytes = bytes
  local r = call(block, "format", format_args)
  if not r then return "", true end
  if not r.id or r.eof or not r.total then return r.content, true end
  local parts, offset = {}, math.max(0, r.total - bytes)
  local whole = offset == 0
  while true do
    local b = call(block, "blob_read", { id = r.id, offset = offset, max_bytes = 983040 })
    if not b then break end
    parts[#parts + 1] = b.data
    offset = offset + #b.data
    if b.eof or #b.data == 0 then break end
  end
  return table.concat(parts), whole
end

-- last_lines keeps the last n lines of text that sep divides. It also
-- says whether a line stood before them, so that the first one kept is a
-- whole line and not the end of one a tail cut.
local function last_lines(text, sep, n)
  local at, count = {}, 0
  local from = 1
  while true do
    local i, j = text:find(sep, from, true)
    if not i then break end
    count = count + 1
    at[count] = j + 1
    from = j + 1
  end
  if count < n then return text, false end
  return text:sub(at[count - n + 1]), true
end

-- The most bytes read from the end of a pane's output to find a page the
-- pane is scrolled to, and the most rows of VT sent to the plugin for it:
-- the plugin reads the message in one call, which holds 1 MiB.
local deepest, widest = 8 * 1024 * 1024, 300000

-- screen_vt returns the pane's screen rows as VT. A row of the VT starts
-- with its own colours, so the rows cut from those above draw the same.
-- More bytes are read until they hold every row. With rows, it returns
-- that many rows from the end, the scrollback above the screen too.
local function screen_vt(pane, rows)
  local most = rows and deepest or 983040
  rows = rows or pane.rows
  local bytes = math.max(32768, rows * pane.cols * 8)
  while true do
    local data, whole = tail(pane.block, { format = "vt" }, bytes)
    local vt, complete = last_lines(data, "\r\n", rows)
    if whole or complete or bytes >= most then return vt end
    bytes = math.min(most, bytes * 4)
  end
end

-- depth returns how many rows from the end of the pane's output hold the
-- page view, or nil when the page is not found. The page is searched by
-- one of its rows, the longest that fits a row of the pane: the app may
-- have joined a longer one from two rows. The program finds the page
-- exactly; this only says how far up to read, so it adds two screens.
local function depth(pane, view)
  local key = ""
  for line in view:gmatch("[^\r\n]+") do
    line = line:gsub("%s+$", "")
    if #line > #key and #line < pane.cols then key = line end
  end
  if key == "" then return nil end
  local bytes = math.max(32768, pane.rows * pane.cols * 8)
  while true do
    local data, whole = tail(pane.block, { format = "text" }, bytes)
    local at, from = nil, 1
    while true do
      local i = data:find(key, from, true)
      if not i then break end
      at, from = i, i + 1
    end
    if at then
      local _, below = data:sub(at):gsub("\n", "")
      return below + 2 * pane.rows
    end
    if whole or bytes >= deepest then return nil end
    bytes = math.min(deepest, bytes * 4)
  end
end

local function mode_set(modes, number)
  for _, m in ipairs((modes and modes.modes) or {}) do
    if m.number == number then return m.set end
  end
  return true
end

-- read_pane returns what the program needs of one pane, or nil for a pane
-- with no process or no terminal. With view, the rows a scrolled pane
-- shows, it reads the output down from that page. A page not found, or
-- too far up to send, leaves the screen alone, which the program hints.
local function read_pane(block, view)
  local process = call(block, "process")
  local proc = process and (process.child or process.foreground)
  if not proc then return nil end
  local ps = rex.exec({ "ps", "-o", "tty=", "-p", tostring(proc.pid) }, { timeout = 2 })
  local name = ps and ps.stdout:gsub("%s", "") or ""
  if name == "" or name == "?" or name == "??" then return nil end
  local size = call(block, "size")
  if not size then return nil end
  local modes = call(block, "modes")
  local pane = { block = block, tty = "/dev/" .. name, rows = size.rows, cols = size.columns,
                 cursor = mode_set(modes, 25), wrap = mode_set(modes, 7) }
  local rows = view and depth(pane, view)
  if rows then
    local vt = screen_vt(pane, rows)
    if #vt <= widest then
      pane.vt = vt
      pane.text = last_lines((tail(block, { format = "text", unwrap = true, trim = true }, math.max(32768, #vt))), "\n", rows)
      return pane
    end
  end
  pane.vt = screen_vt(pane)
  pane.text = last_lines((tail(block, { format = "text", unwrap = true, trim = true }, math.max(32768, pane.rows * pane.cols * 8))), "\n", pane.rows)
  return pane
end

local ours = { ["fingers-reader"] = true, ["fingers-writer"] = true, ["fingers-agent"] = true,
               ["fingers-dummy"] = true }

-- A substitute in a pane's place carries the pane's ID in its label, so
-- that a later run can put back the pane of a run that died.
local substitute_label = "fingers-substitute:"
local function substituted(label)
  return label and label:sub(1, #substitute_label) == substitute_label and label:sub(#substitute_label + 1) or nil
end
local function is_ours(label)
  return label ~= nil and (ours[label] or substituted(label) ~= nil)
end

local function rect_of(node, block)
  if node == nil then return nil end
  if node.block_id == block then return node.rect end
  if node.split then return rect_of(node.split.before, block) or rect_of(node.split.after, block) end
  return nil
end

-- corner is a rectangle a thousandth of the window in size at the top
-- right corner of rect. Rex refuses a float of no size or outside the
-- window.
local function corner(rect)
  local w, h = 0.001, 0.001
  rect = rect or { 0, 0, 1, 1 }
  return { math.max(0, math.min(rect[1] + rect[3] - w, 1 - w)), math.max(0, math.min(rect[2], 1 - h)), w, h }
end

-- A float this small is one of ours: the reader's, or the one a pane
-- waits in on its way back.
local function small(layer)
  return layer.kind ~= "tiled" and layer.bounds ~= nil and layer.bounds[3] < 0.01
end

-- layer_of finds the layer and the window that hold a block, and the
-- block's rectangle in the window.
local function layer_of(view, id)
  for _, w in ipairs((view and view.windows) or {}) do
    for _, layer in ipairs(w.layers or {}) do
      for _, b in ipairs(layer.blocks or {}) do
        if b.block_id == id then
          local r = rect_of(layer.layout, id)
          if r and layer.kind ~= "tiled" and layer.bounds then
            local lb = layer.bounds
            r = { lb[1] + r[1] * lb[3], lb[2] + r[2] * lb[4], r[3] * lb[3], r[4] * lb[4] }
          end
          return layer, w.window_id, r
        end
      end
    end
  end
end

-- The client ID of this script once it owns the run's blocks. A block's
-- size follows its owner, so the pane keeps its size out of the window.
local mine

-- asked tells whether a client other than this script has asked a size
-- for the block, after the server time since when that is given. The app
-- asks one when it has loaded a block, about 1.3 s after the block is
-- placed on a host 200 ms away. A block swapped into view before that
-- shows as an empty pane.
local function asked(block, since)
  local r = call(block, "size")
  for _, d in ipairs((r and r.desired) or {}) do
    if d.client_id ~= mine and (not since or d.updated_at > since) then return true end
  end
  return false
end

-- give_back puts a pane back in the place its substitute holds, from
-- whatever step a run left it at, and closes the substitute. A detached
-- block is placed only in a new split, so the pane goes into a small
-- float beside a dummy first, and is swapped from there. With wait, the
-- swap waits until the app has loaded the pane again, and each turn calls
-- wait, which passes on the keys typed meanwhile.
local function give_back(sub, pane, wait)
  local view = rex.session.view{ session_id = sid }
  local list = rex.call("session.list_blocks", { session_id = sid })
  -- Without an answer nothing is known about the pane: change nothing,
  -- so the substitute and its label stay for another try.
  if not view or not list then return false end
  local alive = false
  for _, b in ipairs((list and list.blocks) or {}) do
    if b.block_id == pane then alive = true end
  end
  local sub_layer, window, rect = layer_of(view, sub)
  local pane_layer = layer_of(view, pane)
  if alive and not pane_layer and sub_layer then
    local made = rex.session.new_layer{
      session_id = sid, window_id = window, bounds = corner(rect), focus = false,
      layout = rex.layout.block{
        flavor = terminal .. ".shell", label = "fingers-dummy",
        -- The program names the float; without it the block only waits.
        options = { shell = "none", exit = { on_completion = true },
                    command = (args.reader and args.reader.dummy) or { "sleep", "30" } },
      },
    }
    local dummy = made and made.block_ids and made.block_ids[1]
    if dummy then
      local newest = ""
      local r = call(pane, "size")
      for _, d in ipairs((r and r.desired) or {}) do
        if d.client_id ~= mine and d.updated_at > newest then newest = d.updated_at end
      end
      rex.session.move_block{ session_id = sid, block_id = pane, anchor_block_id = dummy }
      if wait then
        -- 75 turns are 1.5 s; the app took 1.3 s on a host 200 ms away.
        local turns, seen_at = 0, nil
        while turns < 75 and not (seen_at and turns - seen_at >= 8) do
          turns = turns + 1
          if not seen_at and asked(pane, newest) then seen_at = turns end
          wait()
          rex.sleep(0.02)
        end
      end
      view = rex.session.view{ session_id = sid }
      pane_layer = layer_of(view, pane)
    end
  end
  if alive and pane_layer and small(pane_layer) then
    if sub_layer then
      rex.session.swap_blocks{ session_id = sid, block_id = pane, other_block_id = sub }
    else
      -- No substitute to swap with: the pane stays a float, at a size
      -- that can be used.
      for _, b in ipairs(pane_layer.blocks or {}) do
        if b.block_id ~= pane then rex.call("block.close", { session_id = sid, block_id = b.block_id }) end
      end
      rex.session.resize_layer{ session_id = sid, layer_id = pane_layer.layer_id, bounds = { 0.2, 0.2, 0.6, 0.6 } }
    end
  end
  view = rex.session.view{ session_id = sid }
  local now_layer = layer_of(view, sub)
  -- The pane must be home before its substitute goes, or nothing names
  -- it any more. When the float could not be made, or the swap failed,
  -- the pane is placed in a split beside the substitute instead: the
  -- layout changes for a moment, and the pane is not lost.
  local home = layer_of(view, pane)
  if alive and now_layer and (not home or small(home)) then
    rex.session.move_block{ session_id = sid, block_id = pane, anchor_block_id = sub }
    view = rex.session.view{ session_id = sid }
    now_layer = layer_of(view, sub)
    home = layer_of(view, pane)
  end
  if alive and now_layer and not small(now_layer) and (not home or small(home)) then
    -- Still not placed: the substitute stays, with its label, for the
    -- next run to try again.
    return false
  end
  if now_layer and small(now_layer) then
    rex.session.close_layer{ session_id = sid, layer_id = now_layer.layer_id }
  else
    rex.call("block.close", { session_id = sid, block_id = sub })
  end
  -- A float of the way back that the swap did not empty.
  for _, w in ipairs((rex.session.view{ session_id = sid } or {}).windows or {}) do
    for _, layer in ipairs(w.layers or {}) do
      local only_dummies = #(layer.blocks or {}) > 0
      for _, b in ipairs(layer.blocks or {}) do
        if b.label ~= "fingers-dummy" then only_dummies = false end
      end
      if only_dummies and small(layer) then
        rex.session.close_layer{ session_id = sid, layer_id = layer.layer_id }
      end
    end
  end
  if alive then
    rex.session.focus_block{ session_id = sid, block_id = pane }
    if mine then rex.call("session.disown_block", { session_id = sid, block_ids = { pane } }) end
  end
end

-- sweep closes what a run that died left in the session: its reader,
-- which may hold the focus, its writers, and its agent. A pane whose
-- substitute still holds its place goes back first.
--
-- A label alone does not make a block ours: a user may name a pane
-- fingers-reader. So a block is closed only when its label is ours and
-- its foreground program is one a run starts, or it has none. On a Mac
-- a block started without a shell reports login there; a pane with a
-- shell reports the shell or what runs in it.
local programs = { login = true, sh = true, dash = true, rex = true, hexdump = true,
                   dd = true, cat = true, sleep = true, stty = true }
-- A substitute whose pane could not be put back is kept, with its label,
-- which is all that still names the pane.
local unresolved = {}
local function leftover(b)
  if not is_ours(b.label) or b.block_id == self_id or unresolved[b.block_id] then return false end
  -- A question that fails says nothing about the block: leave it.
  local process = call(b.block_id, "process")
  if not process then return false end
  local name = process.foreground and process.foreground.name
  return name == nil or programs[name] == true
end
local function sweep()
  local list = rex.call("session.list_blocks", { session_id = sid })
  for _, b in ipairs((list and list.blocks) or {}) do
    local pane = substituted(b.label)
    if pane and leftover(b) and give_back(b.block_id, pane) == false then
      unresolved[b.block_id] = true
    end
  end
  local view = rex.session.view{ session_id = sid }
  for _, w in ipairs((view and view.windows) or {}) do
    for _, layer in ipairs(w.layers or {}) do
      local all = #(layer.blocks or {}) > 0
      for _, b in ipairs(layer.blocks or {}) do
        if not leftover(b) then all = false end
      end
      if all and layer.kind ~= "tiled" then
        rex.session.close_layer{ session_id = sid, layer_id = layer.layer_id }
      end
    end
  end
  list = rex.call("session.list_blocks", { session_id = sid })
  for _, b in ipairs((list and list.blocks) or {}) do
    if leftover(b) then
      rex.call("block.close", { session_id = sid, block_id = b.block_id })
    end
  end
end

-- place finds the window of block, its rectangle, and the terminal panes
-- of that window, block first. A trigger that arrived on a reader left by
-- a dead run is for the pane that has the focus once the reader is gone.
local function place(block)
  local view = rex.session.view{ session_id = sid }
  local function find(id)
    for _, w in ipairs((view and view.windows) or {}) do
      local blocks, found, rect = {}, false, nil
      for _, layer in ipairs(w.layers or {}) do
        for _, b in ipairs(layer.blocks or {}) do
          if b.creator_name == terminal and not is_ours(b.label) then
            if b.block_id == id then found = true else blocks[#blocks + 1] = b.block_id end
          end
        end
        local r = rect_of(layer.layout, id)
        if r then
          if layer.kind ~= "tiled" and layer.bounds then
            local b = layer.bounds
            r = { b[1] + r[1] * b[3], b[2] + r[2] * b[4], r[3] * b[3], r[4] * b[4] }
          end
          rect = r
        end
      end
      if found then
        table.insert(blocks, 1, id)
        return w.window_id, rect, blocks
      end
    end
  end
  local window, rect, blocks = find(block)
  if window then return window, rect, blocks end
  for _, w in ipairs((view and view.windows) or {}) do
    if w.window_id == view.active_window_id and w.focused_block_id then
      return find(w.focused_block_id)
    end
  end
end

-- open_reader starts the key reader in a floating pane at the pane's top
-- right corner, a thousandth of the window in size, with the focus. The
-- app sends a host tab's keys to the focused block of the host's session.
local function open_reader(window, rect, command, stays)
  local layer = rex.session.new_layer{
    session_id = sid, window_id = window, bounds = corner(rect), focus = true,
    layout = rex.layout.block{
      flavor = terminal .. ".shell", label = "fingers-reader",
      -- A substitute stays when its program ends: its label names the
      -- pane to put back.
      options = { shell = "none", command = command, exit = { on_completion = not stays } },
    },
  }
  return layer
end

local function write_tty(pane, data)
  local f = io.open(pane.tty, "w")
  if not f then return false end
  f:write(data)
  f:close()
  return true
end

-- linger keeps the block's program alive after its report. A block whose
-- program has ended closes, and the plugin could no longer read it.
local function linger()
  while io.read("*l") do end
end

-- The run.
sweep()
local window, rect, blocks = place(args.pane)
if not window then
  send("RXF2", { status = "ignored" })
  return linger()
end
local origin = blocks[1]
if args.scope ~= "window" then blocks = { origin } end
-- The reader opens first, so a key typed before the hints show waits in
-- it and does not reach the pane's program.
local hidden = args.keys == "hidden"
local layer, reader
-- The substitute writes its terminal's path and then each key to a file
-- here on the host, since its screen shows the frame.
local key_file
if args.keys == "substitute" and args.reader.substitute then
  local made = rex.exec({ "mktemp" }, { timeout = 2 })
  local path = made and made.ok and made.stdout:gsub("%s+$", "") or ""
  if path ~= "" then key_file = path end
end
-- Set when the substitute holds the pane's place.
local swapped = false
if key_file then
  local command = {}
  for _, a in ipairs(args.reader.substitute) do command[#command + 1] = a end
  command[#command + 1] = key_file
  layer = open_reader(window, rect, command, true)
  reader = layer and layer.block_ids and layer.block_ids[1]
elseif hidden then
  local b = rex.call("session.new_block", {
    session_id = sid, flavor = terminal .. ".shell", label = "fingers-reader",
    options = { shell = "none", command = args.reader.command, exit = { on_completion = true } },
  })
  reader = b and b.block_id
else
  layer = open_reader(window, rect, args.reader.command)
  reader = layer and layer.block_ids and layer.block_ids[1]
end

local panes = {}
for _, block in ipairs(blocks) do
  local pane = read_pane(block, block == origin and args.view or nil)
  if pane then panes[#panes + 1] = pane end
end

-- pass is set once the keys are read: it sends the pane the keys typed
-- into the substitute while the pane is on its way back.
local pass = function() end
local function leave()
  if hidden then
    -- The focus never left the pane.
    if reader then rex.call("block.close", { session_id = sid, block_id = reader }) end
    return
  end
  if swapped then
    give_back(reader, origin, pass)
  else
    if layer then rex.session.close_layer{ session_id = sid, layer_id = layer.layer_id } end
    rex.session.focus_block{ session_id = sid, block_id = origin }
    if mine then rex.call("session.disown_block", { session_id = sid, block_ids = { origin } }) end
  end
  if key_file then os.remove(key_file) end
end

if #panes == 0 or panes[1].block ~= origin or not reader then
  -- The reader had the focus: give it back and let the reader stay a
  -- moment, as at the end of a run.
  if reader and not hidden then
    rex.session.focus_block{ session_id = sid, block_id = origin }
    rex.sleep(0.8)
  end
  leave()
  send("RXF2", { status = "failed", reader = reader, error = reader and "no process in the pane" or "the key reader did not start" })
  return linger()
end

local gathered = { reader = reader, panes = {} }
for i, p in ipairs(panes) do
  gathered.panes[i] = { block = p.block, rows = p.rows, cols = p.cols, cursor = p.cursor, wrap = p.wrap, vt = p.vt, text = p.text }
end
send("RXF1", gathered)

-- read_mode sets how a read of this block's terminal waits: for min
-- bytes, or with min 0 for tenths of a second at most, after which
-- io.read returns nil. It returns false when the terminal is not known or
-- stty takes neither -F nor -f; reads then wait as before.
local own_tty = os.getenv("REX_FINGERS_TTY") or ""
local function read_mode(min, tenths)
  if own_tty == "" then return false end
  for _, flag in ipairs({ "-F", "-f" }) do
    local r = rex.exec({ "stty", flag, own_tty, "min", tostring(min), "time", tostring(tenths) }, { timeout = 2 })
    if r and r.ok then return true end
  end
  return false
end

-- message reads one message of the plugin: its word, its numbers, and
-- its body. With limited, reads give up after a time and it returns nil;
-- a body is read with no limit, since a read that gives up cuts it short.
local function message(limited)
  local head = io.read("*l")
  if not head then return nil end
  local word, rest = head:match("^(%u+)%s*(.*)$")
  if not word then return "?", {}, "" end
  local numbers = {}
  for n in rest:gmatch("%d+") do numbers[#numbers + 1] = tonumber(n) end
  local body = ""
  if word == "RUN" or word == "W" then
    if limited then read_mode(1, 0) end
    body = io.read(numbers[#numbers] or 0) or ""
    if limited then read_mode(0, limited) end
  end
  return word, numbers, body
end

local word, _, body = message()
local run = word == "RUN" and rex.json.decode(body)
trace("run", word, "reader", reader, "key file", key_file, "pane", origin)
-- A reader in its float stays for linger_reader seconds after a run has
-- ended. Rex for Mac snapshot 2107 sent one more key event to the
-- reader's block right after the key that picked, and any key typed just
-- after a run with no hints; the block was closed by then, the host
-- answered 404, and the app lost its connection to the host (2026-11-24).
-- The focus goes back to the pane at once.
local linger_reader = 0.8
if not run then
  -- No hints: the plugin said END and leaves this block alone.
  if not hidden then
    rex.session.focus_block{ session_id = sid, block_id = origin }
    rex.sleep(linger_reader)
  end
  leave()
  return
end

for i, p in ipairs(panes) do write_tty(p, run.panes[i].frame) end

-- now_vt is every pane's screen as it is now. Compared with the screens a
-- moment after the frame went up, it tells whether a pane wrote output
-- during the run. The restore the program made repaints the screen as it
-- was captured, which is right only for a pane that wrote nothing.
local function now_vt()
  local all = {}
  for i, p in ipairs(panes) do all[i] = screen_vt(p) end
  return all
end

local function same(a, b)
  for i = 1, #a do
    if a[i] ~= b[i] then return false end
  end
  return true
end

-- The keys. A letter is a hint key, a capital is Shift, a control byte is
-- Ctrl. Escape, q, Space, and Ctrl+C end the run.
local status, pick, modifier, failure
local typed, seen, baseline, turn = "", 0, nil, 0
local function press(letter, mod)
  local longer = typed .. letter
  local continues = false
  for _, t in ipairs(run.targets) do
    if t.hint == longer then
      status, pick, modifier = "pick", t, mod
      return
    end
    if t.hint:sub(1, #longer) == longer then continues = true end
  end
  if continues then typed = longer end
end

-- keys returns every byte typed so far, or nil when the reader is gone.
-- The reader prints each as two hex digits on its screen. The substitute
-- writes them to its file, after a first line with its terminal's path.
local substitute_tty
local function keys()
  if not key_file then
    local r = call(reader, "format", { format = "text", unwrap = true, trim = true })
    if not r then return nil end
    local hex = ((r.content or ""):match(args.reader.mark .. "(.*)") or ""):gsub("%s", "")
    -- A last digit alone is half of a byte still being printed.
    if #hex % 2 == 1 then hex = hex:sub(1, -2) end
    return (hex:gsub("%x%x", function(pair) return string.char(tonumber(pair, 16)) end))
  end
  -- Every 25th turn, half a second, checks that the block is still there.
  if turn % 25 == 0 and not call(reader, "size") then return nil end
  local f = io.open(key_file, "rb")
  if not f then return nil end
  local data = f:read("*a")
  f:close()
  local first = data:find("\n", 1, true)
  if not first then return "" end
  if not substitute_tty then
    local path = data:sub(1, first - 1)
    substitute_tty = path:sub(1, 5) == "/dev/" and { tty = path } or false
  end
  return data:sub(first + 1)
end

pass = function()
  local bytes = keys()
  if bytes and #bytes > seen then
    call(origin, "write", { data = bytes:sub(seen + 1) })
    seen = #bytes
  end
end

-- substitute_turn moves the substitute one step toward the pane's place:
-- it takes the pane's size and frame, waits for the app to load it, and
-- changes places with the pane. A pick before that ends the run as with
-- the reader. The label FINGERS shows while it is a small float.
local label = "\27[1;1H\27[7m FINGERS \27[0m"
local step, step_turn = 0, 0
local function substitute_turn()
  if not substitute_tty then return end
  if step == 0 then
    write_tty(substitute_tty, "\27[?25l" .. label)
    local owned = rex.call("session.own_block", { session_id = sid, block_ids = { reader, origin }, mode = "take" })
    mine = owned and owned.blocks and owned.blocks[1] and owned.blocks[1].owner
    if not mine then
      step = -1
      return
    end
    call(reader, "resize", { columns = panes[1].cols, rows = panes[1].rows })
    call(origin, "resize", { columns = panes[1].cols, rows = panes[1].rows })
    step, step_turn = 1, turn
  elseif step == 1 and turn - step_turn >= 2 then
    write_tty(substitute_tty, run.panes[1].frame .. label)
    step = 2
  elseif step == 2 and asked(reader) then
    step, step_turn = 3, turn
  elseif step == 3 and turn - step_turn >= 8 then
    -- The frame once more, without the label, and the pane's title.
    local title = call(origin, "title")
    local name = title and title.title and title.title:gsub("%c", "") or ""
    write_tty(substitute_tty, run.panes[1].frame .. (name ~= "" and ("\27]2;" .. name .. "\7") or ""))
    rex.sleep(0.03)
    -- The label is what names the pane to a later run, so no swap
    -- without it.
    local labeled = rex.session.set_block_label{ session_id = sid, block_id = reader, label = substitute_label .. origin }
    if labeled and rex.session.swap_blocks{ session_id = sid, block_id = origin, other_block_id = reader } then
      rex.session.detach_block{ session_id = sid, block_id = origin }
      swapped, layer = true, nil
      step = 4
    else
      rex.session.set_block_label{ session_id = sid, block_id = reader, label = "fingers-reader" }
      step = -1
    end
  end
end

while not status do
  turn = turn + 1
  if os.time() - started >= args.limit then
    status = "timeout"
    break
  end
  local bytes = keys()
  if not bytes then
    -- The reader is gone: somebody closed it, or it never ran.
    trace("reader gone at turn", turn, "key file", key_file)
    status = "cancel"
    if turn <= 5 and seen == 0 and not key_file then failure = "the key reader ended at once; it needs stty and hexdump on the host" end
    break
  end
  for n = seen + 1, #bytes do
    if not status then
      seen = n
      if not baseline then baseline = now_vt() end
      local b = bytes:byte(n)
      trace("key byte", b, "turn", turn)
      if b == 27 or b == 3 or b == 32 or b == 113 then
        status = "cancel"
      elseif b >= 97 and b <= 122 then
        press(string.char(b), "main")
      elseif b >= 65 and b <= 90 then
        press(string.char(b + 32), "shift")
      elseif b >= 1 and b <= 26 and b ~= 9 and b ~= 10 and b ~= 13 then
        press(string.char(b + 96), "ctrl")
      end
    end
  end
  if key_file and not status then
    local before = step
    substitute_turn()
    if step ~= before then trace("substitute step", step, "turn", turn) end
  end
  -- The terminal has drawn the frame by the fifth turn, 100 ms on.
  if not baseline and turn >= 5 then baseline = now_vt() end
  if not status then rex.sleep(0.02) end
end

-- The end of the run: the panes as they were, a paste, the focus, and
-- then the report. A pane that wrote during the run is left to the
-- plugin, which has the program work out the rows to repaint.
local current = now_vt()
local dirty = baseline ~= nil and not same(current, baseline)
trace("end", status, "turn", turn, "swapped", swapped, "dirty", dirty, "hint", pick and pick.hint)
if not dirty then
  for i, p in ipairs(panes) do write_tty(p, run.panes[i].restore) end
end
local pasted = false
if status == "pick" and not dirty and run.actions[modifier] == ":paste:" then
  pasted = call(origin, "write", { data = pick.text }) ~= nil
end
-- With the substitute in the pane's place, the report goes first: the
-- plugin copies at once, and the pane comes back about 1.5 s later, when
-- the app has loaded it again. The plugin leaves this block to the agent
-- then.
-- A reader in its float stays for linger_reader seconds after the
-- report too, as said above.
local returning = swapped or (not hidden and reader ~= nil)
if not returning then
  leave()
elseif not swapped then
  rex.session.focus_block{ session_id = sid, block_id = origin }
end
send("RXF2", { status = status, hint = pick and pick.hint, modifier = modifier, dirty = dirty,
               pasted = pasted, reader = reader, error = failure, current = dirty and current or nil,
               returning = returning or nil })

-- A pane that wrote during the run still shows the hints. The plugin has
-- the program work out the rows to repaint and sends them, then BYE. A
-- plugin that is gone sends nothing: Rex stopped its action, or the app
-- no longer passes its calls. The hints must not stay up, so when no word
-- comes in plugin_wait tenths of a second, the restores made at the start
-- are written after all. They repaint the screen as it was then: what the
-- pane printed since is drawn over, which is wrong, and less wrong than
-- hint letters left in a pane.
local plugin_wait = 60
local limited = dirty and read_mode(0, plugin_wait) and plugin_wait or nil
local heard = false
-- A plugin told that the pane is on its way back sends words only for a
-- pane that wrote during the run.
while not returning or dirty do
  local w, numbers, data = message(limited)
  if not w then break end
  heard = true
  if w ~= "W" then break end
  local pane = panes[(numbers[1] or 0) + 1]
  if pane then write_tty(pane, data) end
end
if dirty and limited and not heard then
  for i, p in ipairs(panes) do write_tty(p, run.panes[i].restore) end
end
if returning then
  if swapped then
    -- The substitute shows the pane as it is now, without the hints,
    -- until the pane is back.
    if substitute_tty then
      write_tty(substitute_tty, "\27[0m\27[H\27[2J" .. (screen_vt(panes[1]):gsub("\r\n$", "")))
    end
  else
    rex.sleep(linger_reader)
  end
  leave()
  -- This block must outlive the plugin's read of the report: over a slow
  -- link the plugin reads it a second later. The plugin answers BYE; a
  -- plugin that is gone sends nothing, and the wait ends by itself.
  -- For a pane that wrote during the run, that wait is over already.
  if not dirty and not heard and read_mode(0, plugin_wait) then message(plugin_wait) end
end
