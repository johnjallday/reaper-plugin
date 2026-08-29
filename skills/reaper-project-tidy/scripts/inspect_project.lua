-- ori-runner-mode: read-only-inspector-v1
-- Canonical REAPER Project Tidy v1 inspector. The trusted runner accepts this
-- file only through its audited no-undo path and allowlists every REAPER API
-- referenced below as a pure read.

local SCHEMA_VERSION = 1
local MAX_OUTPUT_BYTES = 4 * 1024 * 1024
local MAX_TRACKS = 2048
local MAX_MARKERS = 4096
local MAX_NAME_BYTES = 512
local MAX_PATH_BYTES = 4096
local MAX_OBJECT_COUNT = 1000000
local MAX_MARKER_ID = 2147483647
local MAX_CHANGE_COUNT = 9007199254740991
local MAX_POSITION_SECONDS = 315576000
local MAX_FOLDER_DEPTH = 128
local CUSTOM_COLOR_FLAG = 0x1000000
local COLOR_MASK = 0xFFFFFF

local function fail(message)
  error("tidy inspector: " .. tostring(message), 0)
end

local function bounded_string(value, maximum, label, allow_empty)
  if type(value) ~= "string" or #value > maximum or value:find("\0", 1, true) then
    fail(label .. " is invalid")
  end
  if not allow_empty and value:match("^%s*$") then fail(label .. " is empty") end
  return value
end

local function bounded_integer(value, minimum, maximum, label)
  if type(value) ~= "number" or value ~= math.floor(value) or value < minimum or value > maximum then
    fail(label .. " is out of bounds")
  end
  return math.floor(value)
end

local function bounded_position(value, label)
  if type(value) ~= "number" or value ~= value or value == math.huge or value == -math.huge or
      value < 0 or value > MAX_POSITION_SECONDS then
    fail(label .. " is out of bounds")
  end
  return value
end

local function json_string(value)
  bounded_string(value, math.max(MAX_PATH_BYTES, MAX_NAME_BYTES), "JSON string", true)
  return '"' .. value:gsub('[%z\1-\31\\"]', function(character)
    local escapes = {
      ['"'] = '\\"',
      ['\\'] = '\\\\',
      ['\b'] = '\\b',
      ['\f'] = '\\f',
      ['\n'] = '\\n',
      ['\r'] = '\\r',
      ['\t'] = '\\t',
    }
    return escapes[character] or string.format('\\u%04x', string.byte(character))
  end) .. '"'
end

local function json_number(value)
  return string.format("%.17g", value)
end

local function color_json(raw_value)
  local raw = math.floor(tonumber(raw_value) or 0)
  if raw == 0 or (raw & CUSTOM_COLOR_FLAG) == 0 then return "null" end
  local red, green, blue = reaper.ColorFromNative(raw & COLOR_MASK)
  bounded_integer(red, 0, 255, "color red")
  bounded_integer(green, 0, 255, "color green")
  bounded_integer(blue, 0, 255, "color blue")
  return string.format('{"red":%d,"green":%d,"blue":%d}', red, green, blue)
end

local chunks = {}
local output_bytes = 0
local function append(value)
  output_bytes = output_bytes + #value
  if output_bytes > MAX_OUTPUT_BYTES then fail("state.json exceeds the output limit") end
  chunks[#chunks + 1] = value
end

local project, project_path = reaper.EnumProjects(-1, "")
if not project then fail("no open project") end
project_path = bounded_string(project_path or "", MAX_PATH_BYTES, "project path", true)

local name_ok, project_name = reaper.GetProjectName(project, "")
if not name_ok or type(project_name) ~= "string" or project_name:match("^%s*$") then
  project_name = "[Unsaved project]"
end
project_name = bounded_string(project_name, MAX_NAME_BYTES, "project name", false)

local project_change_count = bounded_integer(
  reaper.GetProjectStateChangeCount(project), 0, MAX_CHANGE_COUNT, "project change count"
)
local save_dirty = reaper.IsProjectDirty(project) ~= 0
local track_count = bounded_integer(reaper.CountTracks(project), 0, MAX_TRACKS, "track count")
local marker_total = bounded_integer(reaper.CountProjectMarkers(project), 0, MAX_MARKERS, "marker count")

append('{"schema_version":' .. tostring(SCHEMA_VERSION))
append(',"project":{"name":' .. json_string(project_name))
append(',"path":' .. json_string(project_path))
append(',"save_dirty":' .. tostring(save_dirty))
append(',"project_change_count":' .. tostring(project_change_count) .. '}')

append(',"tracks":[')
for index = 0, track_count - 1 do
  local track = reaper.GetTrack(project, index)
  if not track then fail("track " .. tostring(index) .. " disappeared") end
  local guid = bounded_string(reaper.GetTrackGUID(track), 38, "track GUID", false)
  if not guid:match('^%{%x%x%x%x%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%x%x%x%x%x%x%x%x%}$') then
    fail("track GUID is malformed")
  end
  local name_ok, track_name = reaper.GetTrackName(track, "")
  if not name_ok then track_name = "" end
  track_name = bounded_string(track_name or "", MAX_NAME_BYTES, "track name", true)
  local folder_depth = bounded_integer(
    reaper.GetMediaTrackInfo_Value(track, "I_FOLDERDEPTH"), -MAX_FOLDER_DEPTH, MAX_FOLDER_DEPTH, "folder depth"
  )
  local item_count = bounded_integer(reaper.CountTrackMediaItems(track), 0, MAX_OBJECT_COUNT, "item count")
  local fx_count = bounded_integer(reaper.TrackFX_GetCount(track), 0, MAX_OBJECT_COUNT, "FX count")
  local color = color_json(reaper.GetMediaTrackInfo_Value(track, "I_CUSTOMCOLOR"))

  if index > 0 then append(',') end
  append('{"guid":' .. json_string(guid))
  append(',"index":' .. tostring(index))
  append(',"name":' .. json_string(track_name))
  append(',"color":' .. color)
  append(',"folder_depth":' .. tostring(folder_depth))
  append(',"item_count":' .. tostring(item_count))
  append(',"fx_count":' .. tostring(fx_count) .. '}')
end
append(']')

append(',"markers":[')
for enumeration_index = 0, marker_total - 1 do
  local found, is_region, position, region_end, marker_name, marker_id, raw_color =
    reaper.EnumProjectMarkers3(project, enumeration_index)
  if found == 0 then fail("marker " .. tostring(enumeration_index) .. " disappeared") end
  marker_id = bounded_integer(marker_id, 0, MAX_MARKER_ID, "marker ID")
  position = bounded_position(position, "marker position")
  marker_name = bounded_string(marker_name or "", MAX_NAME_BYTES, "marker name", true)

  if enumeration_index > 0 then append(',') end
  append('{"enumeration_index":' .. tostring(enumeration_index))
  append(',"id":' .. tostring(marker_id))
  append(',"is_region":' .. tostring(is_region))
  append(',"position_seconds":' .. json_number(position))
  if is_region then
    region_end = bounded_position(region_end, "region end")
    if region_end <= position then fail("region end must follow its start") end
    append(',"end_seconds":' .. json_number(region_end))
  end
  append(',"name":' .. json_string(marker_name))
  append(',"color":' .. color_json(raw_color) .. '}')
end
append(']}')

-- A concurrent edit would make the assembled snapshot internally stale. The
-- pure-read inspector itself must leave this count unchanged.
local final_change_count = bounded_integer(
  reaper.GetProjectStateChangeCount(project), 0, MAX_CHANGE_COUNT, "final project change count"
)
if final_change_count ~= project_change_count then fail("project changed during inspection") end

local body = table.concat(chunks) .. "\n"
if #body > MAX_OUTPUT_BYTES then fail("state.json exceeds the output limit") end

local home = os.getenv("HOME") or os.getenv("USERPROFILE")
if not home or home == "" then fail("home directory is unavailable") end
local state_path = home .. "/.ori-reaper/state.json"
local token = tostring(project):gsub("[^%w]", "") .. "-" .. tostring(os.time())
local temp_path = home .. "/.ori-reaper/.state.json.tmp-" .. token
os.remove(temp_path)

local file, open_error = io.open(temp_path, "wb")
if not file then fail("temporary state file: " .. tostring(open_error)) end
local wrote, write_error = file:write(body)
if not wrote then
  file:close()
  os.remove(temp_path)
  fail("write temporary state file: " .. tostring(write_error))
end
local closed, close_error = file:close()
if not closed then
  os.remove(temp_path)
  fail("close temporary state file: " .. tostring(close_error))
end
local renamed, rename_error = os.rename(temp_path, state_path)
if not renamed then
  os.remove(temp_path)
  fail("replace state file: " .. tostring(rename_error))
end
