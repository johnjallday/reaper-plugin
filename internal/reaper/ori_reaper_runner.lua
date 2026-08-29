--[[
  ori-reaper-runner.lua

  Installed + registered ONCE by `reaper-plugin install-runner`. This is a normal
  REAPER *action* (NOT a startup script): it runs only when triggered — from the
  Actions list, a shortcut, or (the point) over the Web Remote interface by its
  command ID.

  On every run it:
    1. records its own stable command ID to ~/.ori-reaper/runner.id, so the agent
       / CLI can discover what to trigger, then
    2. executes the Lua found in ~/.ori-reaper/inbox.lua through either the
       audited no-undo inspector path or the ordinary single-Undo-block path,
       then
    3. writes "ok" or "error: ..." to ~/.ori-reaper/last_status.txt so the caller
       can confirm the outcome.

  The agent only ever writes ~/.ori-reaper/inbox.lua (a sandbox-legal scratch
  path); it never writes into REAPER's Scripts dir or reaper-kb.ini.
]]

local function home()
  return os.getenv("HOME") or os.getenv("USERPROFILE") or ""
end

local ori_dir     = home() .. "/.ori-reaper"
local inbox_path  = ori_dir .. "/inbox.lua"
local id_path     = ori_dir .. "/runner.id"
local status_path = ori_dir .. "/last_status.txt"

-- This exact one-shot header selects the audited inspector path. Ordinary
-- inbox scripts keep the historical mutation path below. The Go exchange
-- refuses this header through its generic script method and accepts it only
-- through the canonical read-only inspection entry point.
local read_only_header = "-- ori-runner-mode: read-only-inspector-v1\n"
local tidy_applier_header = "-- ori-runner-mode: tidy-applier-v1\n"
local read_only_apis = {
  ColorFromNative = true,
  CountProjectMarkers = true,
  CountTrackMediaItems = true,
  CountTracks = true,
  EnumProjectMarkers3 = true,
  EnumProjects = true,
  GetAppVersion = true,
  GetMediaTrackInfo_Value = true,
  GetProjectName = true,
  GetProjectStateChangeCount = true,
  GetTrack = true,
  GetTrackGUID = true,
  GetTrackName = true,
  IsProjectDirty = true,
  TrackFX_GetCount = true,
}

local tidy_applier_apis = {
  ColorToNative = true,
  CountProjectMarkers = true,
  CountTracks = true,
  DeleteProjectMarker = true,
  EnumProjectMarkers3 = true,
  EnumProjects = true,
  GetMediaTrackInfo_Value = true,
  GetProjectName = true,
  GetProjectStateChangeCount = true,
  GetTrack = true,
  GetTrackGUID = true,
  SetMediaTrackInfo_Value = true,
  SetProjectMarker3 = true,
}

local function audit_direct_apis(source, allowed)
  for api in source:gmatch("reaper%s*%.%s*([%a_][%w_]*)") do
    if not allowed[api] then return false, "API " .. api .. " is not allowed in this mode" end
  end
  return true
end

local function audit_read_only(source)
  -- Reject dynamic access/aliasing and code-loading or shell escape hatches.
  -- Canonical inspection uses direct calls to the pure-read allowlist above.
  local forbidden = {
    "reaper%s*%[",
    "=%s*reaper[%s,;%)%}]",
    "%(%s*reaper[%s,%)]",
    "[%s%(=,]_G[%s%[%.]",
    "[%s%(=,]_ENV[%s%[%.]",
    "os%s*%.%s*execute%s*%(",
    "io%s*%.%s*popen%s*%(",
    "%f[%a]load%s*%(",
    "%f[%a]loadfile%s*%(",
    "%f[%a]loadstring%s*%(",
    "%f[%a]dofile%s*%(",
    "%f[%a]require%s*%(",
    "package%s*%.",
    "debug%s*%.",
    "%f[%a]rawget%s*%(",
    "%f[%a]getmetatable%s*%(",
  }
  for _, pattern in ipairs(forbidden) do
    if source:find(pattern) then return false, "forbidden indirection" end
  end
  local ok, err = audit_direct_apis(source, read_only_apis)
  if not ok then
    return false, err:gsub(" is not allowed in this mode$", " is not read-only")
  end
  return true
end

local function audit_tidy_applier(source)
  local forbidden = {
    "reaper%s*%[",
    "=%s*reaper[%s,;%)%}]",
    "%(%s*reaper[%s,%)]",
    "[%s%(=,]_G[%s%[%.]",
    "[%s%(=,]_ENV[%s%[%.]",
    "os%s*%.%s*execute%s*%(",
    "io%s*%.%s*popen%s*%(",
    "%f[%a]load%s*%(",
    "%f[%a]loadfile%s*%(",
    "%f[%a]loadstring%s*%(",
    "%f[%a]dofile%s*%(",
    "%f[%a]require%s*%(",
    "package%s*%.",
    "debug%s*%.",
    "%f[%a]rawget%s*%(",
    "%f[%a]getmetatable%s*%(",
  }
  for _, pattern in ipairs(forbidden) do
    if source:find(pattern) then return false, "forbidden indirection" end
  end
  return audit_direct_apis(source, tidy_applier_apis)
end

local function write_file(path, text)
  local f = io.open(path, "w")
  if not f then return false end
  f:write(text)
  f:close()
  return true
end

local function set_status(text)
  write_file(status_path, text)
end

-- 1) Persist our own command ID for discovery. Prefer the stable named ID
--    (e.g. "_RS<hash>"), which the Web Remote interface accepts as /_/<id> and
--    which survives REAPER restarts; fall back to the numeric ID.
local _, _, _, cmd_id = reaper.get_action_context()
local named = reaper.ReverseNamedCommandLookup(cmd_id) -- returns e.g. "RS123…" or nil
local trigger_id
if named and named ~= "" then
  trigger_id = (named:sub(1, 1) == "_") and named or ("_" .. named)
else
  trigger_id = tostring(cmd_id)
end
write_file(id_path, trigger_id)

-- 2) Load + run the inbox.
local inf = io.open(inbox_path, "r")
if not inf then
  set_status("error: no inbox at " .. inbox_path)
  return
end
local code = inf:read("*a")
inf:close()
if not code or code:match("^%s*$") then
  set_status("error: inbox is empty")
  return
end

local read_only = code:sub(1, #read_only_header) == read_only_header
local tidy_applier = code:sub(1, #tidy_applier_header) == tidy_applier_header
if read_only then
  -- A non-deferred ReaScript action creates an automatic undo/state-change
  -- point even when its body only reads. One empty defer suppresses that host
  -- behavior; the inspector itself remains synchronous and is already done
  -- before the callback runs on REAPER's next cycle.
  reaper.defer(function() end)
  local audit_ok, audit_err = audit_read_only(code)
  if not audit_ok then
    set_status("error: read-only audit: " .. tostring(audit_err))
    return
  end
elseif tidy_applier then
  local audit_ok, audit_err = audit_tidy_applier(code)
  if not audit_ok then
    reaper.defer(function() end)
    set_status("error: tidy applier audit: " .. tostring(audit_err))
    return
  end
end

local chunk, load_err = load(code, "@" .. inbox_path)
if not chunk then
  if tidy_applier then reaper.defer(function() end) end
  set_status("error: load: " .. tostring(load_err))
  return
end

if read_only then
  -- Deliberately no undo calls, window adjustment, arrange refresh, or other
  -- REAPER mutation/refresh API in the canonical inspection path.
  local ok, run_err = pcall(chunk)
  if ok then
    set_status("ok")
  else
    set_status("error: run: " .. tostring(run_err))
  end
  return
end

-- Canonical tidy applier: parse, validate, and stale-check before opening the
-- undo block. The chunk returns a closed execution contract; only its apply
-- function runs inside the one runner-owned undo boundary. The finalizer runs
-- after Undo_EndBlock so apply_result.json receives the settled change count.
if tidy_applier then
  local prepared_ok, contract_or_error = pcall(chunk)
  if not prepared_ok or type(contract_or_error) ~= "table" or
      type(contract_or_error.apply) ~= "function" or type(contract_or_error.finalize) ~= "function" or
      type(contract_or_error.has_mutations) ~= "boolean" or contract_or_error.project == nil then
    reaper.defer(function() end)
    set_status("error: tidy prepare: " .. tostring(contract_or_error))
    return
  end

  local contract = contract_or_error
  local apply_ok, apply_error = true, nil
  if contract.has_mutations then
    reaper.Undo_BeginBlock()
    apply_ok, apply_error = pcall(contract.apply)
    reaper.Undo_EndBlock("Ori: tidy project", -1)
    reaper.TrackList_AdjustWindows(false)
    reaper.UpdateArrange()
  else
    -- A fully stale/skipped plan creates no empty automatic action undo point.
    reaper.defer(function() end)
  end

  local after_count = reaper.GetProjectStateChangeCount(contract.project)
  local finalize_ok, finalize_error = pcall(
    contract.finalize,
    after_count,
    apply_ok and nil or tostring(apply_error)
  )
  if not finalize_ok then
    set_status("error: tidy finalize: " .. tostring(finalize_error))
  elseif not apply_ok then
    set_status("error: tidy apply: " .. tostring(apply_error))
  else
    set_status("ok")
  end
  return
end

-- Historical behavior for ordinary scripts: one mutation path, one
-- runner-owned undo block, and the existing refresh behavior.
reaper.Undo_BeginBlock()
local ok, run_err = pcall(chunk)
reaper.Undo_EndBlock("Ori: run inbox", -1)
reaper.TrackList_AdjustWindows(false)
reaper.UpdateArrange()

if ok then
  set_status("ok")
else
  set_status("error: run: " .. tostring(run_err))
end
