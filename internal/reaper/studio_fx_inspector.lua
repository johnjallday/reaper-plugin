-- ori-runner-mode: read-only-inspector-v1
-- Canonical REAPER Studio plugin-list inspector. The trusted runner accepts
-- this file only through its audited no-undo path, and every REAPER API below
-- is on the pure-read allowlist: it enumerates the installed FX list and
-- writes it to fx_list.json in the runner exchange folder. It loads no plugin
-- and touches no project.

local SCHEMA_VERSION = 1
local MAX_OUTPUT_BYTES = 4 * 1024 * 1024
local MAX_ROWS = 20000
local MAX_TEXT_BYTES = 1024

local function fail(message)
  error("studio fx inspector: " .. tostring(message), 0)
end

local function bounded_text(value, label)
  if type(value) ~= "string" or #value > MAX_TEXT_BYTES or value:find("\0", 1, true) then
    fail(label .. " is invalid")
  end
  return value
end

local function json_string(value)
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

local chunks = {}
local output_bytes = 0
local function append(value)
  output_bytes = output_bytes + #value
  if output_bytes > MAX_OUTPUT_BYTES then fail("fx_list.json exceeds the output limit") end
  chunks[#chunks + 1] = value
end

local app_version = bounded_text(reaper.GetAppVersion(), "application version")

append('{"schema_version":' .. tostring(SCHEMA_VERSION))
append(',"app_version":' .. json_string(app_version))
append(',"fx":[')
local count = 0
while count < MAX_ROWS do
  local found, name, ident = reaper.EnumInstalledFX(count)
  if not found then break end
  name = bounded_text(name or "", "FX name")
  ident = bounded_text(ident or "", "FX identifier")
  if count > 0 then append(',') end
  append('{"name":' .. json_string(name) .. ',"ident":' .. json_string(ident) .. '}')
  count = count + 1
end
append('],"count":' .. tostring(count) .. '}')

local body = table.concat(chunks) .. "\n"
if #body > MAX_OUTPUT_BYTES then fail("fx_list.json exceeds the output limit") end

local home = os.getenv("HOME") or os.getenv("USERPROFILE")
if not home or home == "" then fail("home directory is unavailable") end
local list_path = home .. "/.ori-reaper/fx_list.json"
local token = tostring(count) .. "-" .. tostring(os.time())
local temp_path = home .. "/.ori-reaper/.fx_list.json.tmp-" .. token
os.remove(temp_path)

local file, open_error = io.open(temp_path, "wb")
if not file then fail("temporary list file: " .. tostring(open_error)) end
local wrote, write_error = file:write(body)
if not wrote then
  file:close()
  os.remove(temp_path)
  fail("write temporary list file: " .. tostring(write_error))
end
local closed, close_error = file:close()
if not closed then
  os.remove(temp_path)
  fail("close temporary list file: " .. tostring(close_error))
end
local renamed, rename_error = os.rename(temp_path, list_path)
if not renamed then
  os.remove(temp_path)
  fail("replace list file: " .. tostring(rename_error))
end
