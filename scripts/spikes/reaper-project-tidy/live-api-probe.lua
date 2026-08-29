-- Findings-first live API probe for REAPER Project Tidy.
-- This script creates and saves a disposable project in a new project tab.
-- It intentionally leaves that tab active so read-only-runner-probe.lua can
-- measure the trusted runner's post-script undo/refresh effects.

local function json_escape(value)
  return (tostring(value):gsub('[%z\1-\31\\"]', function(char)
    local escapes = {
      ['"'] = '\\"',
      ['\\'] = '\\\\',
      ['\b'] = '\\b',
      ['\f'] = '\\f',
      ['\n'] = '\\n',
      ['\r'] = '\\r',
      ['\t'] = '\\t',
    }
    return escapes[char] or string.format('\\u%04x', string.byte(char))
  end))
end

local function is_array(value)
  local count = 0
  for key, _ in pairs(value) do
    if type(key) ~= 'number' or key < 1 or key % 1 ~= 0 then return false end
    count = count + 1
  end
  for index = 1, count do
    if value[index] == nil then return false end
  end
  return true
end

local function json_encode(value)
  local kind = type(value)
  if kind == 'nil' then return 'null' end
  if kind == 'boolean' or kind == 'number' then return tostring(value) end
  if kind == 'string' then return '"' .. json_escape(value) .. '"' end
  if kind ~= 'table' then error('unsupported JSON value: ' .. kind) end

  local parts = {}
  if is_array(value) then
    for index = 1, #value do parts[#parts + 1] = json_encode(value[index]) end
    return '[' .. table.concat(parts, ',') .. ']'
  end

  local keys = {}
  for key, _ in pairs(value) do keys[#keys + 1] = key end
  table.sort(keys)
  for _, key in ipairs(keys) do
    parts[#parts + 1] = json_encode(key) .. ':' .. json_encode(value[key])
  end
  return '{' .. table.concat(parts, ',') .. '}'
end

local function write_file(path, body)
  local handle, err = io.open(path, 'wb')
  if not handle then error('open ' .. path .. ': ' .. tostring(err)) end
  local ok, write_err = handle:write(body)
  if not ok then handle:close(); error('write ' .. path .. ': ' .. tostring(write_err)) end
  local close_ok, close_err = handle:close()
  if not close_ok then error('close ' .. path .. ': ' .. tostring(close_err)) end
end

local function read_file(path)
  local handle = io.open(path, 'rb')
  if not handle then return nil end
  local body = handle:read('*a')
  handle:close()
  return body
end

local function atomic_write(path, body)
  local temp_path = path .. '.tmp'
  write_file(temp_path, body)
  local ok, err = os.rename(temp_path, path)
  if not ok then os.remove(temp_path); error('rename ' .. path .. ': ' .. tostring(err)) end
end

local home = os.getenv('HOME') or error('HOME is unavailable')
local root = home .. '/.ori-reaper/reaper-away-spike'
os.execute('mkdir -p "' .. root .. '"')

local current = reaper.EnumProjects(-1, '')
local previous_index = nil
local tab_index = 0
while true do
  local project = reaper.EnumProjects(tab_index, '')
  if not project then break end
  if project == current then previous_index = tab_index end
  tab_index = tab_index + 1
end
if previous_index == nil then error('could not identify the original project tab') end
for index = 0, tab_index - 1 do
  local open_project = reaper.EnumProjects(index, '')
  if open_project and reaper.CountTracks(open_project) == 2 then
    local first_track = reaper.GetTrack(open_project, 0)
    local _, first_name = reaper.GetSetMediaTrackInfo_String(first_track, 'P_NAME', '', false)
    if first_name == 'Vox 2' then
      error('an earlier disposable spike tab is still open; close it before rerunning')
    end
  end
end
write_file(root .. '/previous-tab-index.txt', tostring(previous_index))

-- 40859: New project tab. This preserves the user's original project.
reaper.Main_OnCommand(40859, 0)
local project, initial_path = reaper.EnumProjects(-1, '')
if not project or project == current then error('new project tab was not created') end

local changes = {}
local function note_change(label)
  changes[#changes + 1] = {
    label = label,
    count = reaper.GetProjectStateChangeCount(project),
    dirty = reaper.IsProjectDirty(project),
  }
end
note_change('new_project_tab')

reaper.InsertTrackAtIndex(0, true)
local vocal = reaper.GetTrack(project, 0)
if not vocal then error('vocal track was not created') end
note_change('insert_track')

local _, name_set = reaper.GetSetMediaTrackInfo_String(vocal, 'P_NAME', 'Vox 2', true)
if not name_set then error('vocal track name was not set') end
note_change('set_track_name')

local native_color = reaper.ColorToNative(12, 34, 56)
local custom_color = native_color | 0x1000000
reaper.SetMediaTrackInfo_Value(vocal, 'I_CUSTOMCOLOR', custom_color)
note_change('set_track_color')

reaper.InsertTrackAtIndex(1, true)
local drums = reaper.GetTrack(project, 1)
reaper.GetSetMediaTrackInfo_String(drums, 'P_NAME', 'DRUM BUS', true)
note_change('insert_and_name_second_track')

local added_ids = {
  exact_original = reaper.AddProjectMarker2(project, false, 10.0, 0, 'chorus', -1, custom_color),
  exact_duplicate = reaper.AddProjectMarker2(project, false, 10.0, 0, 'chorus', -1, custom_color),
  exact_empty = reaper.AddProjectMarker2(project, false, 10.0, 0, '', -1, 0),
  half_ms = reaper.AddProjectMarker2(project, false, 10.0005, 0, 'chorus', -1, custom_color),
  one_ms = reaper.AddProjectMarker2(project, false, 10.001, 0, 'chorus', -1, custom_color),
  requested_marker = reaper.AddProjectMarker2(project, false, 20.0, 0, 'Verse', 101, custom_color),
  requested_region = reaper.AddProjectMarker2(project, true, 30.0, 40.0, 'chorus 2', 202, custom_color),
}
note_change('add_markers_and_region')

local project_path = root .. '/messy-project.RPP'
reaper.Main_SaveProjectEx(project, project_path, 0)
note_change('save_project')

local track_color = math.floor(reaper.GetMediaTrackInfo_Value(vocal, 'I_CUSTOMCOLOR'))
local red, green, blue = reaper.ColorFromNative(track_color & 0xFFFFFF)
local guid_first = reaper.GetTrackGUID(vocal)
local guid_second = reaper.GetTrackGUID(vocal)

local total, marker_count, region_count = reaper.CountProjectMarkers(project)
local markers = {}
for enum_index = 0, total - 1 do
  local found, is_region, position, region_end, name, id, color = reaper.EnumProjectMarkers3(project, enum_index)
  if found == 0 then error('marker enumeration stopped at index ' .. tostring(enum_index)) end
  local marker_red, marker_green, marker_blue = reaper.ColorFromNative(color & 0xFFFFFF)
  markers[#markers + 1] = {
    enum_index = enum_index,
    id = id,
    is_region = is_region,
    position = position,
    region_end = region_end,
    name = name,
    color_raw = color,
    color_native = color & 0xFFFFFF,
    color_custom_flag = (color & 0x1000000) ~= 0,
    rgb = {marker_red, marker_green, marker_blue},
  }
end
local count_before_reads = reaper.GetProjectStateChangeCount(project)
-- Repeat all read paths that the inspector needs.
local _ = reaper.CountTracks(project)
_ = reaper.CountTrackMediaItems(vocal)
_ = reaper.TrackFX_GetCount(vocal)
_ = reaper.GetMediaTrackInfo_Value(vocal, 'I_FOLDERDEPTH')
_ = reaper.GetTrackGUID(vocal)
_ = reaper.CountProjectMarkers(project)
for enum_index = 0, total - 1 do reaper.EnumProjectMarkers3(project, enum_index) end
local count_after_reads = reaper.GetProjectStateChangeCount(project)

-- Verify POSIX replacement behavior in the same directory used for state files.
local atomic_destination = root .. '/atomic-state.json'
local atomic_temp = atomic_destination .. '.tmp'
write_file(atomic_destination, 'old')
write_file(atomic_temp, 'new')
local rename_ok, rename_error = os.rename(atomic_temp, atomic_destination)
local temp_handle = io.open(atomic_temp, 'rb')
local temp_exists = temp_handle ~= nil
if temp_handle then temp_handle:close() end

local result = {
  reaper_version = reaper.GetAppVersion(),
  original_tab_index = previous_index,
  project = {
    initial_path = initial_path,
    saved_path = project_path,
    dirty_after_save = reaper.IsProjectDirty(project),
  },
  change_counts = changes,
  read_only_counts = {before = count_before_reads, after = count_after_reads},
  track = {
    guid_first = guid_first,
    guid_second = guid_second,
    native_from_rgb = native_color,
    custom_encoded = custom_color,
    stored_color = track_color,
    stored_custom_flag = (track_color & 0x1000000) ~= 0,
    decoded_rgb = {red, green, blue},
  },
  markers = {
    count_return = total,
    marker_count = marker_count,
    region_count = region_count,
    added_ids = added_ids,
    enumerated = markers,
    near_duplicate_deltas_seconds = {0.0005, 0.001},
  },
  atomic_replace = {
    rename_ok = rename_ok == true,
    rename_error = rename_error or '',
    destination_body = read_file(atomic_destination) or '',
    temp_exists_after = temp_exists,
  },
}
atomic_write(root .. '/setup-result.json', json_encode(result) .. '\n')
