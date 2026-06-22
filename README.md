# reaper-plugin

An Ori / Claude / Codex **plugin** that lets an AI agent control REAPER over its
**Web Remote** HTTP interface using plain shell (curl) and file operations — **no
MCP server**. It ships ready-to-use [skills](skills/) plus an optional helper CLI
(`bin/reaper-plugin`) for the parts that are fiddly in shell, such as registering
ReaScripts in `reaper-kb.ini`.

The plugin manifest lives in [`.claude-plugin/plugin.json`](.claude-plugin/plugin.json).

## How it works

REAPER exposes a **Web Remote** HTTP interface (Preferences → Control/OSC/web →
add "Web browser interface"). With localhost network enabled — the default in the
CLI sandbox posture — an agent can:

- read transport/track state: `curl http://127.0.0.1:$PORT/_/TRANSPORT`, `.../_/TRACK`
- run any action or registered ReaScript by command ID: `curl .../_/<COMMAND_ID>`
- manage ReaScript files directly on disk (write/list/delete)

No MCP tool call and no app automation are required. The [skills](skills/) teach
the agent these workflows; start with
[`reaper-web-remote`](skills/reaper-web-remote/SKILL.md).

## Running new Lua live — the runner

Web Remote can only trigger actions that already have a command ID, so you can't
register-and-run a brand-new script in one shot, and REAPER only reads
`reaper-kb.ini` at launch. To make running arbitrary Lua frictionless, the plugin
installs **one** persistent action — the **runner** — that executes whatever Lua
you hand it.

**One-time setup:**

```bash
./bin/reaper-plugin install-runner   # copy + register the runner action
# then restart REAPER once, and trigger "ori-reaper-runner" once from the Actions
# list (Actions → Show action list…) so it records its command ID.
```

**After that, run any Lua immediately — no restart, no per-script registration:**

```bash
./bin/reaper-plugin exec --content 'reaper.ShowConsoleMsg("hi\n")'
./bin/reaper-plugin exec --file session.lua          # or --file - to read stdin
./bin/reaper-plugin runner-id                         # the runner's command ID
```

How it works: `exec` writes your Lua to `~/.ori-reaper/inbox.lua`, triggers the
runner over Web Remote, and reports the runner's status. `~/.ori-reaper/` is the
**only** path the agent writes to — so inside the Codex sandbox you whitelist just
that one directory (`sandbox_workspace_write.writable_roots`), not REAPER's whole
config tree. The runner wraps every run in an Undo block.

## Helper CLI

The binary also handles the file/registration bits that are awkward in shell:

```bash
# Build
cd /path/to/ori/plugins/reaper-plugin
make build            # or: go build -o bin/reaper-plugin ./cmd/reaper-plugin

# Use
./bin/reaper-plugin --help
./bin/reaper-plugin port                              # resolve the Web Remote port
./bin/reaper-plugin status                            # is REAPER running?
./bin/reaper-plugin tracks                            # current project's tracks
./bin/reaper-plugin register-script --script foo.lua  # give a script a command ID
./bin/reaper-plugin register-all                      # register every script
./bin/reaper-plugin clean-scripts                     # prune stale reaper-kb.ini entries
```

## Environment Variables

- `REAPER_SCRIPTS_DIR` override scripts directory
- `REAPER_WEB_REMOTE_PORT` override web remote port (otherwise auto-detect from `reaper.ini`)
- `REAPER_MARKETPLACE_URL` marketplace URL shown by marketplace operations

## Skills

Ready-made agent skills live in [`skills/`](skills/):

- [`reaper-web-remote`](skills/reaper-web-remote/SKILL.md) — the core Web Remote
  playbook (port discovery, transport/track reads, running command IDs).
- [`reaper-session-setup`](skills/reaper-session-setup/SKILL.md) — "set up a
  session, name and arm my tracks."

The same `SKILL.md` files work across Ori, Claude, and Codex; see
[`skills/README.md`](skills/README.md) for install instructions.
