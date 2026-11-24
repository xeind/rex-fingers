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
-- Its arguments, a JSON object: pane, scope, limit in seconds, and reader,
-- the argv and mark of the key reader.
local terminal = "com.superlogical.terminal"
local args = rex.args
local sid = rex.session_id
local self_id = os.getenv("REX_BLOCK")
local started = os.time()

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

-- screen_vt returns the pane's screen rows as VT. A row of the VT starts
-- with its own colours, so the rows cut from those above draw the same.
-- More bytes are read until they hold every row.
local function screen_vt(pane)
  local bytes = math.max(32768, pane.rows * pane.cols * 8)
  while true do
    local data, whole = tail(pane.block, { format = "vt" }, bytes)
    local vt, complete = last_lines(data, "\r\n", pane.rows)
    if whole or complete or bytes >= 983040 then return vt end
    bytes = math.min(983040, bytes * 4)
  end
end

local function mode_set(modes, number)
  for _, m in ipairs((modes and modes.modes) or {}) do
    if m.number == number then return m.set end
  end
  return true
end

-- read_pane returns what the program needs of one pane, or nil for a pane
-- with no process or no terminal.
local function read_pane(block)
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
  pane.vt = screen_vt(pane)
  pane.text = last_lines((tail(block, { format = "text", unwrap = true, trim = true }, math.max(32768, pane.rows * pane.cols * 8))), "\n", pane.rows)
  return pane
end

local ours = { ["fingers-reader"] = true, ["fingers-writer"] = true, ["fingers-agent"] = true }

local function rect_of(node, block)
  if node == nil then return nil end
  if node.block_id == block then return node.rect end
  if node.split then return rect_of(node.split.before, block) or rect_of(node.split.after, block) end
  return nil
end

-- sweep closes what a run that died left in the session: its reader,
-- which may hold the focus, its writers, and its agent.
local function sweep()
  local view = rex.session.view{ session_id = sid }
  for _, w in ipairs((view and view.windows) or {}) do
    for _, layer in ipairs(w.layers or {}) do
      for _, b in ipairs(layer.blocks or {}) do
        if b.label == "fingers-reader" and layer.kind ~= "tiled" then
          rex.session.close_layer{ session_id = sid, layer_id = layer.layer_id }
        end
      end
    end
  end
  local list = rex.call("session.list_blocks", { session_id = sid })
  for _, b in ipairs((list and list.blocks) or {}) do
    if (b.label == "fingers-agent" or b.label == "fingers-writer") and b.block_id ~= self_id then
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
          if b.creator_name == terminal and not ours[b.label] then
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
local function open_reader(window, rect)
  local w, h = 0.001, 0.001
  rect = rect or { 0, 0, 1, 1 }
  local x = math.max(0, math.min(rect[1] + rect[3] - w, 1 - w))
  local y = math.max(0, math.min(rect[2], 1 - h))
  local layer = rex.session.new_layer{
    session_id = sid, window_id = window, bounds = { x, y, w, h }, focus = true,
    layout = rex.layout.block{
      flavor = terminal .. ".shell", label = "fingers-reader",
      options = { shell = "none", command = args.reader.command, exit = { on_completion = true } },
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
local layer = open_reader(window, rect)
local reader = layer and layer.block_ids and layer.block_ids[1]

local panes = {}
for _, block in ipairs(blocks) do
  local pane = read_pane(block)
  if pane then panes[#panes + 1] = pane end
end

local function leave()
  if layer then rex.session.close_layer{ session_id = sid, layer_id = layer.layer_id } end
  rex.session.focus_block{ session_id = sid, block_id = origin }
end

if #panes == 0 or panes[1].block ~= origin or not reader then
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
if not run then
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

while not status do
  turn = turn + 1
  if os.time() - started >= args.limit then
    status = "timeout"
    break
  end
  local r = call(reader, "format", { format = "text", unwrap = true, trim = true })
  if not r then
    -- The reader is gone: somebody closed it, or it never ran.
    status = "cancel"
    if turn <= 5 and seen == 0 then failure = "the key reader ended at once; it needs stty and hexdump on the host" end
    break
  end
  local hex = ((r.content or ""):match(args.reader.mark .. "(.*)") or ""):gsub("%s", "")
  local n = 0
  for pair in hex:gmatch("%x%x") do
    n = n + 1
    if n > seen and not status then
      seen = n
      if not baseline then baseline = now_vt() end
      local b = tonumber(pair, 16)
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
  -- The terminal has drawn the frame by the fifth turn, 100 ms on.
  if not baseline and turn >= 5 then baseline = now_vt() end
  if not status then rex.sleep(0.02) end
end

-- The end of the run: the panes as they were, a paste, the focus, and
-- then the report. A pane that wrote during the run is left to the
-- plugin, which has the program work out the rows to repaint.
local current = now_vt()
local dirty = baseline ~= nil and not same(current, baseline)
if not dirty then
  for i, p in ipairs(panes) do write_tty(p, run.panes[i].restore) end
end
local pasted = false
if status == "pick" and not dirty and run.actions[modifier] == ":paste:" then
  pasted = call(origin, "write", { data = pick.text }) ~= nil
end
leave()
send("RXF2", { status = status, hint = pick and pick.hint, modifier = modifier, dirty = dirty,
               pasted = pasted, reader = reader, error = failure, current = dirty and current or nil })

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
while true do
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
