-- Observe the disposable project's count after the preceding read-only script
-- has returned through the mutation-oriented trusted runner, then restore the
-- user's originally selected project tab.

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
local project, path = reaper.EnumProjects(-1, '')
if not project or reaper.CountTracks(project) ~= 2 then
  error('disposable spike project is not active: ' .. tostring(path))
end
local first_track = reaper.GetTrack(project, 0)
local _, first_name = reaper.GetSetMediaTrackInfo_String(first_track, 'P_NAME', '', false)
if first_name ~= 'Vox 2' then error('unexpected active project: ' .. tostring(path)) end
local count_after_runner = reaper.GetProjectStateChangeCount(project)

local previous_index = tonumber(read_file(root .. '/previous-tab-index.txt'))
local previous, previous_path = reaper.EnumProjects(previous_index, '')
if not previous then error('original project tab no longer exists') end
reaper.SelectProjectInstance(previous)

write_atomic(root .. '/post-runner-result.json', string.format(
  '{"spike_path":%q,"count_after_read_runner":%d,"restored_tab_index":%d,"restored_path":%q}\n',
  path, count_after_runner, previous_index, previous_path
))
