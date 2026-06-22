---
name: reaper-web-remote
description: Control a running REAPER instance from a CLI agent (Codex/Claude Code) via REAPER's Web Remote HTTP interface, using only the agent's built-in shell. Use when asked to inspect or change REAPER's transport, project, tempo, tracks, or to run registered ReaScripts.
---

# REAPER via Web Remote (shell, no MCP)

You drive REAPER over its **Web Remote** HTTP interface using your **shell** (curl).
This works inside the workspace sandbox because localhost network is enabled — you
do NOT need an MCP tool or app-automation permission.

This plugin also ships an optional helper binary at
`${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin`. Prefer plain `curl` for transport and
track reads; use the helper for the things that are fiddly in shell — registering
ReaScripts in `reaper-kb.ini` (so they get Web Remote command IDs) and pruning
stale registrations. Build it once with `make build` in the plugin directory if
`bin/reaper-plugin` is missing.

## 1. Find the Web Remote port
REAPER's port is in its config. On macOS:

    grep -i '^csurf_' "$HOME/Library/Application Support/REAPER/reaper.ini"

Look for a line like `csurf_0=HTTP 0 2307 ...` — the number (e.g. `2307`) is the port.
If not found, the common default is `2307`. Call it `$PORT`.

Shortcut: `${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin port` prints the resolved port.

## 2. Confirm REAPER is reachable

    curl -s -m 5 "http://127.0.0.1:$PORT/_/TRANSPORT"

A tab-separated `TRANSPORT\t<playstate>\t<pos_sec>\t<repeat>\t<pos_str>\t<pos_beats>`
line means REAPER is up and the Web Remote is live. If curl fails to connect,
REAPER is not running or the Web Remote interface is disabled — stop and ask the
user to launch REAPER with the web browser interface enabled.

## 3. Read the track list
Fetch the current project's tracks (tab-separated `TRACK` rows):

    curl -s -m 5 "http://127.0.0.1:$PORT/_/TRACK"

`${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin tracks` prints the same data as a table.

## 4. Run REAPER actions / registered ReaScripts
Trigger any REAPER **action command ID** (or a ReaScript that has been registered
as an action and thus has a command ID):

    curl -s -m 5 "http://127.0.0.1:$PORT/_/<COMMAND_ID>"

Chain commands with `;`:  `.../_/40044;1016`  (play, then stop).

Useful built-ins: `40023` New project, `40022` Save project as, `1007` Play,
`1016` Stop, `40044` Play/stop. Registered ReaScripts appear in REAPER's Action
List with `_RS…`/custom IDs.

## 5. Run NEW Lua live — the runner
Web Remote can only trigger actions that already have a command ID, so you cannot
register-and-run a brand-new script in one shot. Instead this plugin uses a
**runner**: one persistent action, installed once, that executes whatever Lua you
hand it. You never write into REAPER's Scripts dir or restart REAPER per script.

One-time setup (the user does this once; check `${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin runner-id`
— if it prints an ID, setup is already done):

    ${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin install-runner   # install + register
    # then: restart REAPER once, and trigger "ori-reaper-runner" once from the
    # Actions list so it records its command ID.

After that, run any Lua immediately:

    ${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin exec --content 'reaper.ShowConsoleMsg("hi\n")'
    ${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin exec --file /path/to/script.lua

`exec` writes your Lua to `~/.ori-reaper/inbox.lua`, triggers the runner over Web
Remote, and reports the runner's status (`ok` / `error: …`). It wraps your code in
an Undo block. If you'd rather do it by hand: write `~/.ori-reaper/inbox.lua`, then
`curl -s "http://127.0.0.1:$PORT/_/$(cat ~/.ori-reaper/runner.id)"`.

> `~/.ori-reaper/` is the only path you write to. Inside the Codex sandbox it must
> be a writable root; on Claude Code it is unrestricted.

## Rules
- Always discover `$PORT` first; never hardcode unless step 1 fails.
- Prefer reads (`/_/TRANSPORT`, `/_/TRACK`) before any state-changing action.
- Do not launch or quit REAPER and do not use app automation — only Web Remote,
  the helper CLI, and files.
