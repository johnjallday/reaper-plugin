-- Run after live-api-probe.lua while the disposable tab is active.
-- It performs only reads and leaves that tab active so the next probe can
-- observe whether the current trusted runner's Undo_EndBlock/refresh changed
-- the project state-change count.

local function write_atomic(path, body)
  local temp_path = path .. '.tmp'
  local handle, err = io.open(temp_path, 'wb')
  if not handle then error(tostring(err)) end
  assert(handle:write(body))
  assert(handle:close())
  local ok, rename_err = os.rename(temp_path, path)
  if not ok then os.remove(temp_path); error(tostring(rename_err)) end
end

local home = os.getenv('HOME') or error('HOME is unavailable')
local root = home .. '/.ori-reaper/reaper-away-spike'
local project, path = reaper.EnumProjects(-1, '')
if not project or reaper.CountTracks(project) ~= 2 then
  error('disposable spike project is not active: ' .. tostring(path))
end
local first_track = reaper.GetTrack(project, 0)
local _, first_name = reaper.GetSetMediaTrackInfo_String(first_track, 'P_NAME', '', false)
if first_name ~= 'Vox 2' then error('unexpected active project: ' .. tostring(path)) end

local before = reaper.GetProjectStateChangeCount(project)
local track_count = reaper.CountTracks(project)
local guids = {}
for index = 0, track_count - 1 do
  local track = reaper.GetTrack(project, index)
  guids[#guids + 1] = reaper.GetTrackGUID(track)
  reaper.GetSetMediaTrackInfo_String(track, 'P_NAME', '', false)
  reaper.GetMediaTrackInfo_Value(track, 'I_CUSTOMCOLOR')
  reaper.GetMediaTrackInfo_Value(track, 'I_FOLDERDEPTH')
  reaper.CountTrackMediaItems(track)
  reaper.TrackFX_GetCount(track)
end
local total = reaper.CountProjectMarkers(project)
local ids = {}
for enum_index = 0, total - 1 do
  local found, is_region, position, region_end, name, id = reaper.EnumProjectMarkers3(project, enum_index)
  if found == 0 then error('marker enumeration stopped early') end
  ids[#ids + 1] = string.format('%d:%s:%.17g:%.17g:%s', id, tostring(is_region), position, region_end, name)
end
local after = reaper.GetProjectStateChangeCount(project)
local body = string.format(
  '{"active_path":%q,"before_reads":%d,"after_reads":%d,"track_guids":%q,"marker_rows":%q}\n',
  path, before, after, table.concat(guids, ','), table.concat(ids, '|')
)
-- Lua %q is a valid JSON string for these controlled ASCII values.
write_atomic(root .. '/read-result.json', body)
