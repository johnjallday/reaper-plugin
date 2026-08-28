-- Confirm whether marker and region displayed IDs occupy separate namespaces.
-- Selects the disposable spike tab, creates marker 303 and region 303, records
-- enumeration output, and leaves the spike tab active for post-runner-probe.lua.

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
local target = nil
local tab_index = 0
while true do
  local project = reaper.EnumProjects(tab_index, '')
  if not project then break end
  if reaper.CountTracks(project) == 2 then
    local track = reaper.GetTrack(project, 0)
    local _, name = reaper.GetSetMediaTrackInfo_String(track, 'P_NAME', '', false)
    if name == 'Vox 2' then target = project; break end
  end
  tab_index = tab_index + 1
end
if not target then error('disposable spike project was not found') end
reaper.SelectProjectInstance(target)

local before = reaper.GetProjectStateChangeCount(target)
local marker_return = reaper.AddProjectMarker2(target, false, 50, 0, 'Shared ID marker', 303, 0)
local region_return = reaper.AddProjectMarker2(target, true, 60, 70, 'Shared ID region', 303, 0)
local rows = {}
local total = reaper.CountProjectMarkers(target)
for enum_index = 0, total - 1 do
  local found, is_region, position, region_end, name, id = reaper.EnumProjectMarkers3(target, enum_index)
  if found ~= 0 and id == 303 then
    rows[#rows + 1] = string.format(
      '{"enum_index":%d,"id":%d,"is_region":%s,"position":%.17g,"end":%.17g,"name":%q}',
      enum_index, id, tostring(is_region), position, region_end, name
    )
  end
end
local after_inside_undo = reaper.GetProjectStateChangeCount(target)
write_atomic(root .. '/marker-id-result.json', string.format(
  '{"before":%d,"after_inside_undo":%d,"marker_return":%d,"region_return":%d,"rows":[%s]}\n',
  before, after_inside_undo, marker_return, region_return, table.concat(rows, ',')
))
