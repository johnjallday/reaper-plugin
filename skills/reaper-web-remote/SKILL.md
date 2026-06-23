---
name: reaper-web-remote
description: Control a running REAPER instance from a CLI agent (Codex/Claude Code) via REAPER's Web Remote HTTP interface, using only the agent's built-in shell. Use when asked to inspect or change REAPER's transport, project, tempo, tracks, or to run registered ReaScripts.
---

# REAPER via Web Remote (shell, no MCP)

You drive REAPER over its **Web Remote** HTTP interface using your **shell**: `curl`
for reads/triggers and a plain file write to run new Lua. You do NOT need an MCP
tool, a helper binary, or app-automation permission — just curl + files.

## 1. Find the Web Remote port
REAPER's port is in its config. On macOS:

    grep -i '^csurf_' "$HOME/Library/Application Support/REAPER/reaper.ini"

Look for a line like `csurf_0=HTTP 0 2307 ...` — the number (e.g. `2307`) is the
port. If not found, the common default is `2307`. Call it `$PORT`.

## 2. Confirm REAPER is reachable

    curl -s -m 5 "http://127.0.0.1:$PORT/_/TRANSPORT"

A tab-separated `TRANSPORT\t<playstate>\t<pos_sec>\t...` line means REAPER is up
and the Web Remote is live. If curl can't connect, REAPER isn't running or the Web
Remote interface is disabled — stop and ask the user to enable it
(Preferences → Control/OSC/web → add "Web browser interface").

## 3. Read the track list

    curl -s -m 5 "http://127.0.0.1:$PORT/_/TRACK"

Tab-separated `TRACK` rows: index, name, ..., and flags. Read before you change.

## 4. Run actions / registered ReaScripts by command ID

    curl -s -m 5 "http://127.0.0.1:$PORT/_/<COMMAND_ID>"

Chain with `;`:  `.../_/40044;1016`. Useful built-ins: `40023` New project,
`40859` New project tab, `40022` Save project as, `1007` Play, `1016` Stop,
`40044` Play/stop. Web Remote can only trigger actions that already have a command
ID — for anything else, use the runner below.

## 5. Run NEW Lua live — the runner
The plugin ships a **runner**: one persistent REAPER action that executes whatever
Lua you place in `~/.ori-reaper/inbox.lua`. To run new Lua you only need curl + a
file write — no binary:

    mkdir -p ~/.ori-reaper
    cat > ~/.ori-reaper/inbox.lua <<'LUA'
    reaper.ShowConsoleMsg("hello from the runner\n")
    LUA
    curl -s -m 5 "http://127.0.0.1:$PORT/_/$(cat ~/.ori-reaper/runner.id)"
    cat ~/.ori-reaper/last_status.txt      # "ok" or "error: …"

The runner wraps your Lua in an Undo block. Examples of things only the runner can
do (Web Remote actions can't): set tempo `reaper.SetCurrentBPM(0, 120, true)`, save
to a path `reaper.Main_SaveProjectEx(0, "/path/Song.RPP", 0)`, name/color/arm
tracks.

### If `~/.ori-reaper/runner.id` does not exist
The runner has not been set up on this machine. **Do not try to install it
yourself** (it needs a one-time human step). Stop and tell the user to run the
one-time setup once: `reaper-plugin install-runner`, then in REAPER load the
staged `ori-reaper-runner.lua` via Actions → Show action list → New action ▾ →
Load ReaScript, and Run it once. After that, `runner.id` exists and you can use
the runner.

## Rules
- Always discover `$PORT` first; never hardcode unless step 1 fails.
- Prefer reads (`/_/TRANSPORT`, `/_/TRACK`) before any state-changing action.
- Do not launch or quit REAPER and do not use app automation — only Web Remote +
  the `~/.ori-reaper/inbox.lua` runner.
