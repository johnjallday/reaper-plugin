-- Record the marker-ID probe's count after the runner closes its undo block,
-- then restore the original project tab.

local function read_file(path)
  local handle, err = io.open(path, 'rb')
  if not handle then error(tostring(err)) end
  local body = handle:read('*a')
  handle:close()
  return body
end

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
local project = reaper.EnumProjects(-1, '')
if not project or reaper.CountTracks(project) ~= 2 then error('spike project is not active') end
local track = reaper.GetTrack(project, 0)
local _, name = reaper.GetSetMediaTrackInfo_String(track, 'P_NAME', '', false)
if name ~= 'Vox 2' then error('spike project is not active') end
local count_after_runner = reaper.GetProjectStateChangeCount(project)

local previous_index = tonumber(read_file(root .. '/previous-tab-index.txt'))
local previous = reaper.EnumProjects(previous_index, '')
if not previous then error('original project tab no longer exists') end
reaper.SelectProjectInstance(previous)
write_atomic(root .. '/marker-post-result.json', string.format(
  '{"count_after_marker_runner":%d,"restored_tab_index":%d}\n',
  count_after_runner, previous_index
))
