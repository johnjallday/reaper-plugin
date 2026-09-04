# reaper-plugin

An Ori / Claude / Codex **plugin** that lets an AI agent control REAPER over its
**Web Remote** HTTP interface. Portable skills continue to use plain shell and
file operations with no native MCP grant. Ori can additionally install the
private, brokered MCP stdio service declared by
[`.ori-plugin/plugin.json`](.ori-plugin/plugin.json) to provide a sandboxed
Workspace Surface, runtime setup provider, Reaper Song blueprint, and
per-agent-grant-gated operations.

Portable identity lives in [`.claude-plugin/plugin.json`](.claude-plugin/plugin.json).

## How it works

REAPER exposes a **Web Remote** HTTP interface (Preferences → Control/OSC/web →
add "Web browser interface"). With localhost network enabled — the default in the
CLI sandbox posture — an agent can:

- read transport/track state: `curl http://127.0.0.1:$PORT/_/TRANSPORT`, `.../_/TRACK`
- run any action or registered ReaScript by command ID: `curl .../_/<COMMAND_ID>`
- manage ReaScript files directly on disk (write/list/delete)

No MCP tool call and no app automation are required for the portable shell
workflow. Ori's private service is never attached wholesale as native MCP; its
UI and agent calls go through declared schemas, grants, scopes, and host-owned
confirmation. The [skills](skills/) teach the portable workflows; start with
[`reaper-web-remote`](skills/reaper-web-remote/SKILL.md).

## Running new Lua live — the runner

Web Remote can only trigger actions that already have a command ID, so you can't
register-and-run a brand-new script in one shot, and REAPER only reads
`reaper-kb.ini` at launch. To make running arbitrary Lua frictionless, the plugin
installs **one** persistent action — the **runner** — that executes whatever Lua
you hand it.

**One-time setup:**

```bash
./bin/reaper-plugin install-runner   # stage the runner in REAPER's Scripts folder
# In REAPER's Actions list, load and run "ori-reaper-runner" once so REAPER
# registers it and the runner records its command ID. No restart is required.
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

## Project Tidy

The Reaper Song blueprint includes **Project Tidy**, a propose-first cosmetic
cleanup workflow. Survey reads the authoritative open project through an
audited no-undo inspector, applies workspace `conventions.md`, and writes a
reviewable plan for only four verbs: color a track, rename a marker, rename a
region, or delete an exact duplicate marker. Apply runs only checked rows in one
REAPER undo step and persists applied/skipped/failed reports. Neither phase has
an `.rpp` fallback or a vocabulary for sound-affecting edits.

Ori capability-scoped Codex tasks keep arbitrary localhost access disabled and
call exact brokered `tidy.survey` / `tidy.apply_selection` operations. Portable
Claude/Codex skill use can still follow the documented shell runner path.

## Ori Workspace Surface development

The first supported service artifact is **macOS arm64**. Version **0.5.0** uses
Workspace Surface protocol v1 and requires an Ori build that implements both
`assistant_program_v1` and `specialist_setup_journey_v1`. Reaper Song blueprint
v4 declares host-owned new/existing project connection modes, `.rpp` selection
constraints, mode-filtered starter tasks, and independently scoped Home/project
roles; none of that inert metadata selects a picker, scanner, path, or adapter.

The reviewed candidate artifact is 8,763,458 bytes with SHA-256
`ad4680d371024d43e2264c00b1a2e2a5a532343d31073e3754a94662bea2cb9d`.
The manifest targets those exact bytes at the future `v0.5.0` GitHub release.
That tag and asset are not published by candidate preparation, so production
installation remains unavailable until a human release owner publishes them.
Build and verify the candidate reproducibly with:

```bash
make artifact-local
make test
make test-ui
make release-package VERSION=v0.5.0
```

The build pins Go 1.25.0, disables VCS stamping, uses `-trimpath`, and clears the
build ID; repeating it from unchanged Go source produces identical bytes across
maintainer machines and release-metadata commits. Install by Git URL or local path through Ori's Plugins UI or
`POST /api/plugins/install`, review the complete trust disclosure, confirm, and
enable. The service starts lazily only when an attached workspace asks for
status/setup/an operation. Other platforms remain explicitly unsupported and
must not launch the artifact.

Ori workspaces created from the retired compiled Reaper Song template are
**not migrated**. Installing this plugin never imports legacy pins, grants,
setup history, template provenance, tasks, or project metadata. Create a new
workspace from the plugin-contributed Reaper Song blueprint for the supported
full-parity path. A manual capability attachment is fresh plugin state, not a
migration.

For a sandboxed disposable Ori demo that should use the real user's REAPER
configuration, launch Ori with `REAPER_PLUGIN_HOME=/absolute/user/home`; this is
an operator environment setting, never manifest/browser/workspace input.

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
- `REAPER_PLUGIN_HOME` operator-only private-service home override for isolated demos

## Skills

Ready-made agent skills live in [`skills/`](skills/):

- [`reaper-web-remote`](skills/reaper-web-remote/SKILL.md) — the core Web Remote
  playbook (port discovery, transport/track reads, running command IDs).
- [`reaper-session-setup`](skills/reaper-session-setup/SKILL.md) — "set up a
  session, name and arm my tracks."
- [`reaper-project-tidy`](skills/reaper-project-tidy/SKILL.md) — survey
  cosmetic track-color and marker cleanup, review every item, and apply only
  checked changes as one undo step.

The same `SKILL.md` files work across Ori, Claude, and Codex; see
[`skills/README.md`](skills/README.md) for install instructions.
