# reaper-mcp

MCP stdio server that exposes REAPER operations through a single tool named `ori-reaper`.

## Features

The tool accepts operation-style payloads compatible with your existing `ori-reaper` usage:

- `list`
- `run`
- `add`
- `delete`
- `list_available_scripts` (URL hint for now)
- `download_script` (URL hint)
- `register_script`
- `register_all_scripts`
- `clean_scripts`
- `get_context`
- `get_status`
- `get_web_remote_port`
- `get_tracks`

## Build

```bash
cd /path/to/ori/mcp/reaper-mcp
go build -o bin/reaper-mcp ./cmd/reaper-mcp
```

## Run (stdio)

```bash
./bin/reaper-mcp
```

## Environment Variables

- `REAPER_SCRIPTS_DIR` override scripts directory
- `REAPER_WEB_REMOTE_PORT` override web remote port (otherwise auto-detect from `reaper.ini`)
- `REAPER_MARKETPLACE_URL` marketplace URL shown by marketplace operations

## Tool Payload Example

```json
{"operation":"get_status"}
```

```json
{"operation":"add","script":"normalize_selected_items","script_type":"lua","content":"-- lua code"}
```

## Skills

Ready-made agent skills that drive this server live in [`skills/`](skills/) — start with
[`reaper-session-setup`](skills/reaper-session-setup/SKILL.md) ("set up a session, name and
arm my tracks"). The same `SKILL.md` works across Ori, Claude, and Codex; see
[`skills/README.md`](skills/README.md) for install instructions.

## Codex MCP Example

Use your Codex MCP server config to run this command over stdio:

- command: `${REPO_ROOT}/mcp/reaper-mcp/bin/reaper-mcp`
- args: `[]`

## Claude MCP Example

Use your Claude MCP server config to run this command over stdio:

- command: `${REPO_ROOT}/mcp/reaper-mcp/bin/reaper-mcp`
- args: `[]`
