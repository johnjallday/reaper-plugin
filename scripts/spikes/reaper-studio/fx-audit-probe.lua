-- ori-runner-mode: read-only-inspector-v1
-- REAPER Studio spike, probe A: ask the installed runner's audited no-undo path
-- to call EnumInstalledFX. The 0.10.1 allowlist does not include it, so the
-- expected outcome is "error: read-only audit: API EnumInstalledFX is not
-- read-only" in last_status.txt and no file written: this is what the catalog
-- feature will report as reader_outdated.

local ok, name, ident = reaper.EnumInstalledFX(0)
local home = os.getenv("HOME") or ""
local file = io.open(home .. "/.ori-reaper/reaper-studio-spike/fx-audit-probe-ran.txt", "wb")
if file then
  file:write(tostring(ok) .. "\t" .. tostring(name) .. "\t" .. tostring(ident) .. "\n")
  file:close()
end
