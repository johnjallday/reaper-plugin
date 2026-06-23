---
name: reaper-session-setup
description: Set up a REAPER session — insert, name, color, and record-arm tracks to a requested layout. Use when the user asks to "set up a session", "create tracks", "build a recording template", or scaffold a new project in REAPER.
---

# REAPER session setup (shell + Web Remote, no MCP)

You set up REAPER sessions by writing a ReaScript and running it live through the
plugin's **runner** — all with your shell (curl + a file write), no MCP tool and no
binary. Read [`reaper-web-remote`](../reaper-web-remote/SKILL.md) first; it covers
port discovery, transport/track reads, and the runner mechanism. This skill is the
session-building playbook on top of it.

## Before you act

1. Discover `$PORT` (reaper-web-remote step 1) and confirm REAPER is reachable:
   `curl -s -m 5 "http://127.0.0.1:$PORT/_/TRANSPORT"`. If it fails, stop and ask
   the user to launch REAPER with the Web Remote interface enabled.
2. Confirm the runner is set up: check that `~/.ori-reaper/runner.id` exists
   (`cat ~/.ori-reaper/runner.id`). If it's missing, follow reaper-web-remote
   step 5 ("If runner.id does not exist") — ask the user to do the one-time setup;
   do not try to install it yourself.
3. Read the current session: `curl -s -m 5 "http://127.0.0.1:$PORT/_/TRACK"`. If
   tracks already exist, confirm with the user before adding to or replacing them,
   so you don't clobber existing work.

## Building a session

1. Work out the layout from the request: count, names, colors, and which tracks to
   record-arm. If anything is unspecified, propose a sensible default and confirm.
2. Compose a Lua ReaScript that builds it (template below). The runner already
   wraps execution in an Undo block, so you don't need your own.
3. Run it live — write the Lua to the inbox and trigger the runner:

        mkdir -p ~/.ori-reaper
        cat > ~/.ori-reaper/inbox.lua <<'LUA'
        -- your Lua here (see template)
        LUA
        curl -s -m 5 "http://127.0.0.1:$PORT/_/$(cat ~/.ori-reaper/runner.id)"
        cat ~/.ori-reaper/last_status.txt      # expect "ok"

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
-- Insert at the end so existing tracks are left untouched:
local base = reaper.CountTracks(0)
for i, t in ipairs(tracks) do
  local idx = base + (i - 1)
  reaper.InsertTrackAtIndex(idx, true)
  local tr = reaper.GetTrack(0, idx)
  reaper.GetSetMediaTrackInfo_String(tr, "P_NAME", t.name, true)
  reaper.SetTrackColor(tr, reaper.ColorToNative(t.color[1], t.color[2], t.color[3]))
  reaper.SetMediaTrackInfo_Value(tr, "I_RECARM", t.arm and 1 or 0)
end
```

## Setting tempo / saving the project (into the workspace)

Tempo and saving are not Web Remote actions — run them through the runner. **Save
the project into the workspace, not into `~/Music` or your home directory.** Your
shell's current working directory *is* the workspace's files folder, so resolve it
and bake the absolute path into the inbox. (The runner runs inside REAPER, a
separate process, so it cannot see your shell's cwd — you must pass an absolute
path.)

    WS_DIR="$(pwd)"                       # the workspace files folder
    mkdir -p ~/.ori-reaper
    cat > ~/.ori-reaper/inbox.lua <<LUA
    reaper.SetCurrentBPM(0, 113, true)
    reaper.Main_SaveProjectEx(0, "$WS_DIR/House Test.RPP", 0)
    LUA
    curl -s -m 5 "http://127.0.0.1:$PORT/_/$(cat ~/.ori-reaper/runner.id)"
    cat ~/.ori-reaper/last_status.txt

The heredoc is **unquoted** (`<<LUA`, not `<<'LUA'`) so the shell expands `$WS_DIR`
into the Lua before the runner reads it; the Lua has no other `$` of its own. Name
the file from the request (e.g. the project name + `.RPP`).

## Guardrails

- Never run a script that deletes tracks or items without explicit confirmation.
- Prefer additive operations; the runner's Undo block lets the user revert cleanly.
- **Save projects, renders, and exports into the workspace files folder (your
  current working directory), never into `~/Music` or the home directory — that is
  how they show up in the workspace.**
- Do not launch or quit REAPER and do not use app automation — only Web Remote +
  the runner inbox.
- Report what you changed: track names, count, arm state, tempo, and the saved path.
