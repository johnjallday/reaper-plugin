---
name: reaper-session-setup
description: Set up a REAPER session — insert, name, color, and record-arm tracks to a requested layout. Use when the user asks to "set up a session", "create tracks", "build a recording template", or scaffold a new project in REAPER.
required_mcp_servers:
  - ori-reaper
---

You set up REAPER sessions by driving the `ori-reaper` MCP tool. REAPER is controlled with
ReaScript: you compose a small Lua script, `add` it, then `run` it.

## Before you act

1. Call `get_status`. If REAPER is not running, stop and ask the user to launch REAPER with the
   Web Remote interface enabled — you cannot control it otherwise.
2. Call `get_tracks` to see the current session. If tracks already exist, confirm with the user
   before adding to or replacing them, so you don't clobber existing work.

## Building a session

1. Work out the track layout from the request: count, names, colors, and which tracks to
   record-arm. If anything is unspecified, propose a sensible default and confirm.
2. Compose a Lua ReaScript that builds it (template below). Always wrap changes in an Undo block.
3. `add` the script — `operation: "add"`, `script_type: "lua"`, a descriptive `filename`, and the
   `content`.
4. `run` the script by its name.
5. Call `get_tracks` again to verify, then summarize exactly what you created.

## Lua template (adapt the `tracks` table to the request)

```lua
local tracks = {
  { name = "Drums",  color = { 255, 80, 80 },  arm = true  },
  { name = "Bass",   color = { 80, 160, 255 }, arm = true  },
  { name = "Guitar", color = { 80, 220, 120 }, arm = false },
  { name = "Vox",    color = { 240, 200, 80 }, arm = true  },
}

reaper.Undo_BeginBlock()
for i, t in ipairs(tracks) do
  reaper.InsertTrackAtIndex(i - 1, true)
  local tr = reaper.GetTrack(0, i - 1)
  reaper.GetSetMediaTrackInfo_String(tr, "P_NAME", t.name, true)
  reaper.SetTrackColor(tr, reaper.ColorToNative(t.color[1], t.color[2], t.color[3]))
  reaper.SetMediaTrackInfo_Value(tr, "I_RECARM", t.arm and 1 or 0)
end
reaper.Undo_EndBlock("Ori: session setup", -1)
reaper.TrackList_AdjustWindows(false)
reaper.UpdateArrange()
```

## Guardrails

- Never run a script that deletes tracks or items without explicit confirmation.
- Prefer additive operations, and always use Undo blocks so the user can revert cleanly.
- Report what you changed: track names, count, and arm state.
