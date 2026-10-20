-- ori-runner-mode: read-only-inspector-v1
-- REAPER Studio spike: the state the catalog reader must preserve, read through
-- the installed runner's audited no-undo path with its current allowlist only
-- (no EnumInstalledFX here). It writes one JSON file into the runner exchange
-- folder's reaper-studio-spike/ scratch directory.

local function fail(message)
  error("studio state probe: " .. tostring(message), 0)
end

local function json_string(value)
  return '"' .. tostring(value):gsub('[%z\1-\31\\"]', function(character)
    local escapes = { ['"'] = '\\"', ['\\'] = '\\\\', ['\n'] = '\\n', ['\r'] = '\\r', ['\t'] = '\\t' }
    return escapes[character] or string.format('\\u%04x', string.byte(character))
  end) .. '"'
end

local project, project_path = reaper.EnumProjects(-1, "")
if not project then fail("no open project") end
local name_ok, project_name = reaper.GetProjectName(project, "")
if not name_ok or type(project_name) ~= "string" then project_name = "" end

local change_count = reaper.GetProjectStateChangeCount(project)
local dirty = reaper.IsProjectDirty(project)
local track_count = reaper.CountTracks(project)
local selected = {}
for index = 0, track_count - 1 do
  local track = reaper.GetTrack(project, index)
  if track and reaper.GetMediaTrackInfo_Value(track, "I_SELECTED") == 1 then
    local _, track_name = reaper.GetTrackName(track, "")
    selected[#selected + 1] = string.format('{"index":%d,"name":%s}', index, json_string(track_name or ""))
  end
end

local body = table.concat({
  '{"probe":"state",',
  '"app_version":', json_string(reaper.GetAppVersion()), ',',
  '"project_path":', json_string(project_path or ""), ',',
  '"project_name":', json_string(project_name), ',',
  '"project_change_count":', tostring(change_count), ',',
  '"dirty":', tostring(dirty), ',',
  '"track_count":', tostring(track_count), ',',
  '"selected_tracks":[', table.concat(selected, ","), '],',
  '"os_time":', tostring(os.time()),
  '}\n',
})

local home = os.getenv("HOME") or ""
if home == "" then fail("home is unavailable") end
local target = home .. "/.ori-reaper/reaper-studio-spike/state.json"
local temp = target .. ".tmp-" .. tostring(os.time())
local file = io.open(temp, "wb")
if not file then fail("cannot open " .. temp) end
file:write(body)
file:close()
os.remove(target)
local renamed, rename_error = os.rename(temp, target)
if not renamed then fail("rename: " .. tostring(rename_error)) end
