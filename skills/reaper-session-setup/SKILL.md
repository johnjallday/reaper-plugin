---
name: reaper-session-setup
description: Set up a REAPER session — insert, name, color, and record-arm tracks to a requested layout. Use when the user asks to "set up a session", "create tracks", "build a recording template", or scaffold a new project in REAPER.
---

# REAPER session setup (shell + Web Remote, no MCP)

You set up REAPER sessions by writing a ReaScript and driving REAPER over its
**Web Remote** HTTP interface with your **shell** — no MCP tool. The
[`reaper-web-remote`](../reaper-web-remote/SKILL.md) skill covers the Web Remote
basics (port discovery, transport/track reads, running command IDs); read it
first. This skill is the session-building playbook on top of it.

REAPER is controlled with ReaScript: you compose a small Lua script and run it
live through the plugin's **runner** (`reaper-plugin exec`), which executes your
Lua inside REAPER without touching the Scripts dir or restarting REAPER.

## Before you act

1. Discover `$PORT` (see reaper-web-remote step 1) and confirm REAPER is reachable:
   `curl -s -m 5 "http://127.0.0.1:$PORT/_/TRANSPORT"`. If it fails, stop and ask
   the user to launch REAPER with the Web Remote interface enabled — you cannot
   control it otherwise.
2. Confirm the runner is set up: `${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin runner-id`.
   If it errors, do the one-time setup in reaper-web-remote step 5
   (`install-runner` → restart REAPER → trigger the runner once), then continue.
3. Read the current session: `curl -s -m 5 "http://127.0.0.1:$PORT/_/TRACK"`. If
   tracks already exist, confirm with the user before adding to or replacing them,
   so you don't clobber existing work.

## Building a session

1. Work out the track layout from the request: count, names, colors, and which
   tracks to record-arm. If anything is unspecified, propose a sensible default
   and confirm.
2. Compose a Lua ReaScript that builds it (template below). The runner already
   wraps execution in an Undo block, so you don't need your own.
3. Run it live: write the Lua to a workspace file and
   `${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin exec --file session.lua`
   (or pipe it: `... exec --file -`). `exec` reports `ok` or `error: …`.
4. Re-read tracks (`curl .../_/TRACK`) to verify, then summarize exactly what you
   created.

## Lua template (adapt the `tracks` table to the request)

```lua
local tracks = {
  { name = "Drums",  color = { 255, 80, 80 },  arm = true  },
  { name = "Bass",   color = { 80, 160, 255 }, arm = true  },
  { name = "Guitar", color = { 80, 220, 120 }, arm = false },
  { name = "Vox",    color = { 240, 200, 80 }, arm = true  },
}

-- The runner already wraps this in an Undo block and refreshes the arrange view.
for i, t in ipairs(tracks) do
  reaper.InsertTrackAtIndex(i - 1, true)
  local tr = reaper.GetTrack(0, i - 1)
  reaper.GetSetMediaTrackInfo_String(tr, "P_NAME", t.name, true)
  reaper.SetTrackColor(tr, reaper.ColorToNative(t.color[1], t.color[2], t.color[3]))
  reaper.SetMediaTrackInfo_Value(tr, "I_RECARM", t.arm and 1 or 0)
end
```

## Alternative: a fresh project file (no running REAPER needed)

If the user wants a brand-new project rather than editing the open one, you can
write a minimal `.RPP` project file directly in the workspace with the tracks
predefined, then ask the user to open it (or open it via a registered "open
project" ReaScript). This avoids the registration/restart step entirely but
creates a new project instead of modifying the current one.

## Guardrails

- Never run a script that deletes tracks or items without explicit confirmation.
- Prefer additive operations, and always use Undo blocks so the user can revert
  cleanly.
- Do not launch or quit REAPER and do not use app automation — only Web Remote,
  the helper CLI, and files.
- Report what you changed: track names, count, and arm state.
