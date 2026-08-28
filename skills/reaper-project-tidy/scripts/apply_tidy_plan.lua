-- ori-runner-mode: tidy-applier-v1
-- Canonical REAPER Project Tidy v1 applier. This chunk performs bounded JSON
-- parsing, whole-plan validation, and every stale-target check before returning
-- the two-phase execution contract consumed by the trusted runner.

local SCHEMA_VERSION = 1
local MAX_PLAN_BYTES = 1024 * 1024
local MAX_RESULT_BYTES = 1024 * 1024
local MAX_ITEMS = 256
local MAX_TRACKS = 2048
local MAX_MARKERS = 4096
local MAX_ID_BYTES = 64
local MAX_NAME_BYTES = 512
local MAX_PATH_BYTES = 4096
local MAX_REASON_BYTES = 1024
local MAX_RESULT_TEXT_BYTES = 2048
local MAX_MARKER_ID = 2147483647
local MAX_CHANGE_COUNT = 9007199254740991
local MAX_POSITION_SECONDS = 315576000
local MAX_JSON_DEPTH = 32
local MAX_JSON_TOKENS = 20000
local CUSTOM_COLOR_FLAG = 0x1000000

local VERBS = {
  set_track_color = true,
  rename_marker = true,
  rename_region = true,
  delete_marker = true,
}

local JSON_NULL = {}
local JSON_ARRAYS = {}

local function fail(message)
  error("tidy applier: " .. tostring(message), 0)
end

local function read_bounded(path, maximum)
  local file, open_error = io.open(path, "rb")
  if not file then fail("open plan: " .. tostring(open_error)) end
  local size, seek_error = file:seek("end")
  if not size then file:close(); fail("measure plan: " .. tostring(seek_error)) end
  if size < 1 or size > maximum then file:close(); fail("plan size is invalid") end
  assert(file:seek("set", 0))
  local body = file:read("*a")
  file:close()
  if not body or #body ~= size then fail("read plan failed") end
  return body
end

local function utf8_character(codepoint)
  if codepoint <= 0x7F then
    return string.char(codepoint)
  elseif codepoint <= 0x7FF then
    return string.char(0xC0 | (codepoint >> 6), 0x80 | (codepoint & 0x3F))
  elseif codepoint <= 0xFFFF then
    return string.char(
      0xE0 | (codepoint >> 12),
      0x80 | ((codepoint >> 6) & 0x3F),
      0x80 | (codepoint & 0x3F)
    )
  elseif codepoint <= 0x10FFFF then
    return string.char(
      0xF0 | (codepoint >> 18),
      0x80 | ((codepoint >> 12) & 0x3F),
      0x80 | ((codepoint >> 6) & 0x3F),
      0x80 | (codepoint & 0x3F)
    )
  end
  fail("JSON Unicode code point is invalid")
end

local function parse_json(text)
  local parser = {text = text, length = #text, position = 1, depth = 0, tokens = 0}

  function parser:error(message)
    fail("invalid plan JSON at byte " .. tostring(self.position) .. ": " .. message)
  end

  function parser:skip_space()
    while self.position <= self.length do
      local byte = self.text:byte(self.position)
      if byte ~= 0x20 and byte ~= 0x09 and byte ~= 0x0A and byte ~= 0x0D then break end
      self.position = self.position + 1
    end
  end

  function parser:token()
    self.tokens = self.tokens + 1
    if self.tokens > MAX_JSON_TOKENS then self:error("too many values") end
  end

  function parser:hex4()
    local value = 0
    for _ = 1, 4 do
      local byte = self.text:byte(self.position)
      if not byte then self:error("incomplete Unicode escape") end
      local digit
      if byte >= 48 and byte <= 57 then digit = byte - 48
      elseif byte >= 65 and byte <= 70 then digit = byte - 55
      elseif byte >= 97 and byte <= 102 then digit = byte - 87
      else self:error("invalid Unicode escape") end
      value = value * 16 + digit
      self.position = self.position + 1
    end
    return value
  end

  function parser:string()
    if self.text:byte(self.position) ~= 34 then self:error("expected string") end
    self.position = self.position + 1
    local output = {}
    local output_bytes = 0
    local segment_start = self.position
    local function add(value)
      output_bytes = output_bytes + #value
      if output_bytes > MAX_PATH_BYTES then self:error("string exceeds limit") end
      output[#output + 1] = value
    end
    while self.position <= self.length do
      local byte = self.text:byte(self.position)
      if byte == 34 then
        if self.position > segment_start then add(self.text:sub(segment_start, self.position - 1)) end
        self.position = self.position + 1
        return table.concat(output)
      elseif byte == 92 then
        if self.position > segment_start then add(self.text:sub(segment_start, self.position - 1)) end
        self.position = self.position + 1
        local escape = self.text:sub(self.position, self.position)
        self.position = self.position + 1
        local simple = {['"'] = '"', ['\\'] = '\\', ['/'] = '/', b = '\b', f = '\f', n = '\n', r = '\r', t = '\t'}
        if simple[escape] then
          add(simple[escape])
        elseif escape == "u" then
          local codepoint = self:hex4()
          if codepoint >= 0xD800 and codepoint <= 0xDBFF then
            if self.text:sub(self.position, self.position + 1) ~= "\\u" then self:error("missing low surrogate") end
            self.position = self.position + 2
            local low = self:hex4()
            if low < 0xDC00 or low > 0xDFFF then self:error("invalid low surrogate") end
            codepoint = 0x10000 + ((codepoint - 0xD800) << 10) + (low - 0xDC00)
          elseif codepoint >= 0xDC00 and codepoint <= 0xDFFF then
            self:error("unexpected low surrogate")
          end
          add(utf8_character(codepoint))
        else
          self:error("invalid string escape")
        end
        segment_start = self.position
      elseif byte < 0x20 then
        self:error("unescaped control character")
      else
        self.position = self.position + 1
      end
    end
    self:error("unterminated string")
  end

  function parser:number()
    local start = self.position
    if self.text:sub(self.position, self.position) == "-" then self.position = self.position + 1 end
    local first = self.text:byte(self.position)
    if first == 48 then
      self.position = self.position + 1
      local following = self.text:byte(self.position)
      if following and following >= 48 and following <= 57 then self:error("leading zero") end
    elseif first and first >= 49 and first <= 57 then
      repeat
        self.position = self.position + 1
        first = self.text:byte(self.position)
      until not first or first < 48 or first > 57
    else
      self:error("invalid number")
    end
    if self.text:sub(self.position, self.position) == "." then
      self.position = self.position + 1
      local digit = self.text:byte(self.position)
      if not digit or digit < 48 or digit > 57 then self:error("invalid fraction") end
      repeat
        self.position = self.position + 1
        digit = self.text:byte(self.position)
      until not digit or digit < 48 or digit > 57
    end
    local exponent = self.text:sub(self.position, self.position)
    if exponent == "e" or exponent == "E" then
      self.position = self.position + 1
      local sign = self.text:sub(self.position, self.position)
      if sign == "+" or sign == "-" then self.position = self.position + 1 end
      local digit = self.text:byte(self.position)
      if not digit or digit < 48 or digit > 57 then self:error("invalid exponent") end
      repeat
        self.position = self.position + 1
        digit = self.text:byte(self.position)
      until not digit or digit < 48 or digit > 57
    end
    local value = tonumber(self.text:sub(start, self.position - 1))
    if not value or value ~= value or value == math.huge or value == -math.huge then self:error("number is out of range") end
    return value
  end

  function parser:value()
    self:skip_space()
    self:token()
    local character = self.text:sub(self.position, self.position)
    if character == '"' then return self:string() end
    if character == "-" or character:match("%d") then return self:number() end
    if self.text:sub(self.position, self.position + 3) == "true" then self.position = self.position + 4; return true end
    if self.text:sub(self.position, self.position + 4) == "false" then self.position = self.position + 5; return false end
    if self.text:sub(self.position, self.position + 3) == "null" then self.position = self.position + 4; return JSON_NULL end
    if character ~= "{" and character ~= "[" then self:error("unexpected token") end

    self.depth = self.depth + 1
    if self.depth > MAX_JSON_DEPTH then self:error("nesting exceeds limit") end
    local result = {}
    if character == "[" then
      JSON_ARRAYS[result] = true
      self.position = self.position + 1
      self:skip_space()
      if self.text:sub(self.position, self.position) ~= "]" then
        while true do
          result[#result + 1] = self:value()
          self:skip_space()
          local separator = self.text:sub(self.position, self.position)
          if separator == "]" then break end
          if separator ~= "," then self:error("expected array separator") end
          self.position = self.position + 1
        end
      end
      self.position = self.position + 1
    else
      self.position = self.position + 1
      self:skip_space()
      if self.text:sub(self.position, self.position) ~= "}" then
        while true do
          self:skip_space()
          local key = self:string()
          if result[key] ~= nil then self:error("duplicate object field " .. key) end
          self:skip_space()
          if self.text:sub(self.position, self.position) ~= ":" then self:error("expected object colon") end
          self.position = self.position + 1
          result[key] = self:value()
          self:skip_space()
          local separator = self.text:sub(self.position, self.position)
          if separator == "}" then break end
          if separator ~= "," then self:error("expected object separator") end
          self.position = self.position + 1
        end
      end
      self.position = self.position + 1
    end
    self.depth = self.depth - 1
    return result
  end

  local value = parser:value()
  parser:skip_space()
  if parser.position <= parser.length then parser:error("trailing content") end
  return value
end

local function is_object(value)
  return type(value) == "table" and value ~= JSON_NULL and not JSON_ARRAYS[value]
end

local function is_array(value)
  return type(value) == "table" and JSON_ARRAYS[value] == true
end

local function exact_object(value, fields, label)
  if not is_object(value) then fail(label .. " must be an object") end
  local allowed = {}
  for _, field in ipairs(fields) do allowed[field] = true end
  local count = 0
  for key, _ in pairs(value) do
    if not allowed[key] then fail(label .. " has unknown field " .. tostring(key)) end
    count = count + 1
  end
  if count ~= #fields then
    for _, field in ipairs(fields) do
      if value[field] == nil then fail(label .. " is missing " .. field) end
    end
    fail(label .. " field count is invalid")
  end
end

local function valid_text(value, maximum, required, label)
  if type(value) ~= "string" or #value > maximum or value:find("\0", 1, true) or
      (required and value:match("^%s*$")) then
    fail(label .. " is invalid")
  end
  return value
end

local function valid_integer(value, minimum, maximum, label)
  if type(value) ~= "number" or value ~= math.floor(value) or value < minimum or value > maximum then
    fail(label .. " is invalid")
  end
  return math.floor(value)
end

local function valid_position(value, label)
  if type(value) ~= "number" or value ~= value or value == math.huge or value == -math.huge or
      value < 0 or value > MAX_POSITION_SECONDS then
    fail(label .. " is invalid")
  end
  return value
end

local function valid_stable_id(value, label)
  valid_text(value, MAX_ID_BYTES, true, label)
  if not value:match("^[A-Za-z0-9][A-Za-z0-9._-]*$") then fail(label .. " has invalid characters") end
  return value
end

local function valid_guid(value)
  valid_text(value, 38, true, "track GUID")
  if not value:match('^%{%x%x%x%x%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%x%x%x%x%x%x%x%x%}$') then
    fail("track GUID is malformed")
  end
  return value
end

local function valid_rgb(value)
  exact_object(value, {"red", "green", "blue"}, "color")
  return {
    red = valid_integer(value.red, 0, 255, "color red"),
    green = valid_integer(value.green, 0, 255, "color green"),
    blue = valid_integer(value.blue, 0, 255, "color blue"),
  }
end

local function validate_plan(value)
  exact_object(value, {"schema_version", "plan_id", "inspected_project", "items"}, "plan")
  if value.schema_version ~= SCHEMA_VERSION then fail("unsupported schema version") end
  local plan = {schema_version = SCHEMA_VERSION, plan_id = valid_stable_id(value.plan_id, "plan ID")}

  exact_object(value.inspected_project, {"name", "path", "project_change_count"}, "inspected project")
  plan.inspected_project = {
    name = valid_text(value.inspected_project.name, MAX_NAME_BYTES, true, "project name"),
    path = valid_text(value.inspected_project.path, MAX_PATH_BYTES, false, "project path"),
    project_change_count = valid_integer(
      value.inspected_project.project_change_count, 0, MAX_CHANGE_COUNT, "inspected project change count"
    ),
  }

  if not is_array(value.items) or #value.items < 1 or #value.items > MAX_ITEMS then fail("items array is invalid") end
  plan.items = {}
  local item_ids, mutation_targets = {}, {}
  for index, raw in ipairs(value.items) do
    local label = "item " .. tostring(index)
    exact_object(raw, {"id", "verb", "target", "payload", "reason"}, label)
    local item = {
      id = valid_stable_id(raw.id, label .. " ID"),
      verb = valid_text(raw.verb, 32, true, label .. " verb"),
      reason = valid_text(raw.reason, MAX_REASON_BYTES, true, label .. " reason"),
    }
    if item_ids[item.id] then fail("duplicate item ID " .. item.id) end
    item_ids[item.id] = true
    if not VERBS[item.verb] then fail("unknown verb " .. item.verb) end

    if item.verb == "set_track_color" then
      exact_object(raw.target, {"track_guid"}, label .. " target")
      exact_object(raw.payload, {"color"}, label .. " payload")
      item.target = {track_guid = valid_guid(raw.target.track_guid)}
      item.payload = {color = valid_rgb(raw.payload.color)}
      item.mutation_key = "track:" .. item.target.track_guid
    elseif item.verb == "rename_marker" or item.verb == "rename_region" then
      exact_object(raw.target, {"marker_id", "snapshot_name"}, label .. " target")
      exact_object(raw.payload, {"new_name"}, label .. " payload")
      item.target = {
        marker_id = valid_integer(raw.target.marker_id, 0, MAX_MARKER_ID, label .. " marker ID"),
        snapshot_name = valid_text(raw.target.snapshot_name, MAX_NAME_BYTES, false, label .. " snapshot name"),
      }
      item.payload = {new_name = valid_text(raw.payload.new_name, MAX_NAME_BYTES, true, label .. " new name")}
      if item.payload.new_name == item.target.snapshot_name then fail(label .. " rename is a no-op") end
      item.mutation_key = (item.verb == "rename_region" and "region:" or "marker:") .. tostring(item.target.marker_id)
    else
      exact_object(raw.target, {"marker_id", "snapshot_name", "snapshot_position_seconds"}, label .. " target")
      exact_object(raw.payload, {"survivor_marker_id"}, label .. " payload")
      item.target = {
        marker_id = valid_integer(raw.target.marker_id, 0, MAX_MARKER_ID, label .. " marker ID"),
        snapshot_name = valid_text(raw.target.snapshot_name, MAX_NAME_BYTES, false, label .. " snapshot name"),
        snapshot_position_seconds = valid_position(raw.target.snapshot_position_seconds, label .. " snapshot position"),
      }
      item.payload = {
        survivor_marker_id = valid_integer(raw.payload.survivor_marker_id, 0, MAX_MARKER_ID, label .. " survivor ID"),
      }
      if item.payload.survivor_marker_id == item.target.marker_id then fail(label .. " survivor is the deletion target") end
      item.mutation_key = "marker:" .. tostring(item.target.marker_id)
    end
    if mutation_targets[item.mutation_key] then fail("duplicate mutation target " .. item.mutation_key) end
    mutation_targets[item.mutation_key] = true
    plan.items[#plan.items + 1] = item
  end
  return plan
end

local function marker_key(is_region, id)
  return (is_region and "region:" or "marker:") .. tostring(id)
end

local home = os.getenv("HOME") or os.getenv("USERPROFILE")
if not home or home == "" then fail("home directory is unavailable") end
local plan_path = home .. "/.ori-reaper/plan.json"
local result_path = home .. "/.ori-reaper/apply_result.json"
local plan = validate_plan(parse_json(read_bounded(plan_path, MAX_PLAN_BYTES)))

local project, project_path = reaper.EnumProjects(-1, "")
if not project then fail("no open project") end
local name_ok, project_name = reaper.GetProjectName(project, "")
if not name_ok or type(project_name) ~= "string" or project_name:match("^%s*$") then project_name = "[Unsaved project]" end
if project_name ~= plan.inspected_project.name or (project_path or "") ~= plan.inspected_project.path then
  fail("open project does not match the inspected project")
end
local before_count = valid_integer(reaper.GetProjectStateChangeCount(project), 0, MAX_CHANGE_COUNT, "project change count")

local tracks = {}
local track_count = valid_integer(reaper.CountTracks(project), 0, MAX_TRACKS, "track count")
for index = 0, track_count - 1 do
  local track = reaper.GetTrack(project, index)
  if not track then fail("track disappeared during preparation") end
  local guid = valid_guid(reaper.GetTrackGUID(track))
  if tracks[guid] then fail("duplicate live track GUID") end
  tracks[guid] = track
end

local markers = {}
local marker_count = valid_integer(reaper.CountProjectMarkers(project), 0, MAX_MARKERS, "marker count")
for enumeration_index = 0, marker_count - 1 do
  local found, is_region, position, region_end, name, id, color = reaper.EnumProjectMarkers3(project, enumeration_index)
  if found == 0 then fail("marker disappeared during preparation") end
  id = valid_integer(id, 0, MAX_MARKER_ID, "live marker ID")
  position = valid_position(position, "live marker position")
  local key = marker_key(is_region, id)
  if markers[key] then fail("duplicate typed live marker ID") end
  markers[key] = {
    id = id,
    is_region = is_region,
    position = position,
    region_end = is_region and valid_position(region_end, "live region end") or 0,
    name = valid_text(name or "", MAX_NAME_BYTES, false, "live marker name"),
    color = math.floor(tonumber(color) or 0),
  }
end

local prepared = {}
local has_mutations = false
local function skipped(item, reason)
  prepared[#prepared + 1] = {id = item.id, verb = item.verb, status = "skipped", reason = reason}
end
local function pending(item, operation)
  prepared[#prepared + 1] = {id = item.id, verb = item.verb, status = "pending", operation = operation}
  has_mutations = true
end

for _, item in ipairs(plan.items) do
  if item.verb == "set_track_color" then
    local track = tracks[item.target.track_guid]
    if not track then
      skipped(item, "track GUID no longer exists")
    else
      local color = item.payload.color
      local native = reaper.ColorToNative(color.red, color.green, color.blue) | CUSTOM_COLOR_FLAG
      pending(item, function()
        return reaper.SetMediaTrackInfo_Value(track, "I_CUSTOMCOLOR", native)
      end)
    end
  elseif item.verb == "rename_marker" or item.verb == "rename_region" then
    local is_region = item.verb == "rename_region"
    local current = markers[marker_key(is_region, item.target.marker_id)]
    if not current then
      skipped(item, is_region and "region ID no longer exists" or "marker ID no longer exists")
    elseif current.name ~= item.target.snapshot_name then
      skipped(item, "snapshot name changed")
    else
      local new_name = item.payload.new_name
      pending(item, function()
        return reaper.SetProjectMarker3(
          project, current.id, current.is_region, current.position, current.region_end, new_name, current.color
        )
      end)
    end
  else
    local current = markers[marker_key(false, item.target.marker_id)]
    local survivor = markers[marker_key(false, item.payload.survivor_marker_id)]
    if not current then
      skipped(item, "marker ID no longer exists")
    elseif current.name ~= item.target.snapshot_name then
      skipped(item, "snapshot name changed")
    elseif current.position ~= item.target.snapshot_position_seconds then
      skipped(item, "snapshot position changed")
    elseif not survivor then
      skipped(item, "surviving marker no longer exists")
    elseif survivor.position ~= current.position then
      skipped(item, "markers are no longer at the exact same position")
    elseif survivor.name ~= current.name and survivor.name ~= "" and current.name ~= "" then
      skipped(item, "markers are no longer exact duplicates")
    else
      pending(item, function()
        return reaper.DeleteProjectMarker(project, current.id, false)
      end)
    end
  end
end

local function apply()
  for _, item in ipairs(prepared) do
    if item.status == "pending" then
      local ok, result = pcall(item.operation)
      item.operation = nil
      if ok and result ~= false then
        item.status = "applied"
      else
        item.status = "failed"
        item.error = tostring(ok and "REAPER API refused the change" or result)
      end
    end
  end
end

local function json_string(value)
  return '"' .. tostring(value):gsub('[%z\1-\31\\"]', function(character)
    local escapes = {['"'] = '\\"', ['\\'] = '\\\\', ['\b'] = '\\b', ['\f'] = '\\f', ['\n'] = '\\n', ['\r'] = '\\r', ['\t'] = '\\t'}
    return escapes[character] or string.format('\\u%04x', string.byte(character))
  end) .. '"'
end

local function result_text(value, fallback)
  local text = tostring(value or "")
  if text:match("^%s*$") then text = fallback end
  if #text > MAX_RESULT_TEXT_BYTES then text = text:sub(1, MAX_RESULT_TEXT_BYTES) end
  return text
end

local function write_result_atomic(body)
  if #body > MAX_RESULT_BYTES then fail("apply result exceeds limit") end
  local token = tostring(project):gsub("[^%w]", "") .. "-" .. tostring(os.time())
  local temp_path = home .. "/.ori-reaper/.apply_result.json.tmp-" .. token
  os.remove(temp_path)
  local file, open_error = io.open(temp_path, "wb")
  if not file then fail("temporary apply result: " .. tostring(open_error)) end
  local wrote, write_error = file:write(body)
  if not wrote then file:close(); os.remove(temp_path); fail("write apply result: " .. tostring(write_error)) end
  local closed, close_error = file:close()
  if not closed then os.remove(temp_path); fail("close apply result: " .. tostring(close_error)) end
  local renamed, rename_error = os.rename(temp_path, result_path)
  if not renamed then os.remove(temp_path); fail("replace apply result: " .. tostring(rename_error)) end
end

local function finalize(after_count, fatal_error)
  after_count = valid_integer(after_count, before_count, MAX_CHANGE_COUNT, "final project change count")
  if fatal_error then
    for _, item in ipairs(prepared) do
      if item.status == "pending" then
        item.status = "failed"
        item.error = result_text(fatal_error, "tidy apply failed")
      end
    end
  end

  local parts = {
    '{"schema_version":1,"plan_id":', json_string(plan.plan_id),
    ',"project_change_count_before":', tostring(before_count),
    ',"project_change_count_after":', tostring(after_count),
    ',"items":[',
  }
  for index, item in ipairs(prepared) do
    if item.status == "pending" then fail("apply result contains a pending item") end
    if index > 1 then parts[#parts + 1] = ',' end
    parts[#parts + 1] = '{"id":' .. json_string(item.id) .. ',"verb":' .. json_string(item.verb) ..
      ',"status":' .. json_string(item.status)
    if item.status == "skipped" then
      parts[#parts + 1] = ',"reason":' .. json_string(result_text(item.reason, "target changed"))
    elseif item.status == "failed" then
      parts[#parts + 1] = ',"error":' .. json_string(result_text(item.error, "REAPER API failed"))
    end
    parts[#parts + 1] = '}'
  end
  parts[#parts + 1] = ']}\n'
  write_result_atomic(table.concat(parts))
end

return {
  project = project,
  has_mutations = has_mutations,
  apply = apply,
  finalize = finalize,
}
