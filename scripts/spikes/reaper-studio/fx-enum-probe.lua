-- REAPER Studio spike, probe B: enumerate REAPER's installed FX list with
-- EnumInstalledFX and record the undo/dirty/change-count state as seen at the
-- start of the run. This file carries NO reserved runner header on purpose: the
-- installed 0.10.1 runner only allows EnumInstalledFX through its ordinary
-- (undo-block-wrapped) inbox path. Its side effects on an empty unsaved project
-- are part of what the spike measures; the product reader never uses this path.

local MAX_FX = 50000
local MAX_BYTES = 16 * 1024 * 1024

local function json_string(value)
  return '"' .. tostring(value):gsub('[%z\1-\31\\"]', function(character)
    local escapes = { ['"'] = '\\"', ['\\'] = '\\\\', ['\n'] = '\\n', ['\r'] = '\\r', ['\t'] = '\\t' }
    return escapes[character] or string.format('\\u%04x', string.byte(character))
  end) .. '"'
end

local project, project_path = reaper.EnumProjects(-1, "")
local undo_label = reaper.Undo_CanUndo2(project)
local redo_label = reaper.Undo_CanRedo2(project)
local change_count = reaper.GetProjectStateChangeCount(project)
local dirty = reaper.IsProjectDirty(project)
local play_state = reaper.GetPlayState()
local started = os.clock()

local rows = {}
local bytes = 0
local count = 0
local max_name, max_ident = 0, 0
local prefixes = {}
local index = 0
while index < MAX_FX do
  local ok, name, ident = reaper.EnumInstalledFX(index)
  if not ok then break end
  count = count + 1
  name = tostring(name or "")
  ident = tostring(ident or "")
  if #name > max_name then max_name = #name end
  if #ident > max_ident then max_ident = #ident end
  local prefix = name:match("^([^:]+):") or "(none)"
  prefixes[prefix] = (prefixes[prefix] or 0) + 1
  local row = string.format('{"index":%d,"name":%s,"ident":%s}', index, json_string(name), json_string(ident))
  bytes = bytes + #row + 1
  if bytes > MAX_BYTES then break end
  rows[#rows + 1] = row
  index = index + 1
end
local elapsed = os.clock() - started

local prefix_rows = {}
for prefix, total in pairs(prefixes) do
  prefix_rows[#prefix_rows + 1] = json_string(prefix) .. ":" .. tostring(total)
end
table.sort(prefix_rows)

local body = table.concat({
  '{"probe":"fx-enum",',
  '"app_version":', json_string(reaper.GetAppVersion()), ',',
  '"project_path":', json_string(project_path or ""), ',',
  '"undo_label_at_start":', json_string(undo_label or ""), ',',
  '"redo_label_at_start":', json_string(redo_label or ""), ',',
  '"project_change_count_at_start":', tostring(change_count), ',',
  '"dirty_at_start":', tostring(dirty), ',',
  '"play_state_at_start":', tostring(play_state), ',',
  '"fx_count":', tostring(count), ',',
  '"max_name_bytes":', tostring(max_name), ',',
  '"max_ident_bytes":', tostring(max_ident), ',',
  '"elapsed_cpu_seconds":', string.format("%.4f", elapsed), ',',
  '"prefix_counts":{', table.concat(prefix_rows, ","), '},',
  '"fx":[', table.concat(rows, ","), ']}\n',
})

local home = os.getenv("HOME") or ""
local target = home .. "/.ori-reaper/reaper-studio-spike/fx-list.json"
local temp = target .. ".tmp-" .. tostring(os.time())
local file = io.open(temp, "wb")
if not file then error("cannot open " .. temp, 0) end
file:write(body)
file:close()
os.remove(target)
local renamed, rename_error = os.rename(temp, target)
if not renamed then error("rename: " .. tostring(rename_error), 0) end
