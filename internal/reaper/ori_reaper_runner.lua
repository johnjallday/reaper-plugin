--[[
  ori-reaper-runner.lua

  Installed + registered ONCE by `reaper-plugin install-runner`. This is a normal
  REAPER *action* (NOT a startup script): it runs only when triggered — from the
  Actions list, a shortcut, or (the point) over the Web Remote interface by its
  command ID.

  On every run it:
    1. records its own stable command ID to ~/.ori-reaper/runner.id, so the agent
       / CLI can discover what to trigger, then
    2. executes the Lua found in ~/.ori-reaper/inbox.lua inside an Undo block, then
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

local chunk, load_err = load(code, "@" .. inbox_path)
if not chunk then
  set_status("error: load: " .. tostring(load_err))
  return
end

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
