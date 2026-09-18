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

## Plugin-owned guided setup

The plugin now carries the complete inert `reaper_setup` quest in
[`.ori-plugin/plugin.json`](.ori-plugin/plugin.json). Reaper Song references it
with `"setup_quest": "reaper_setup"`. On an Ori build with setup-quest support,
Plugins, Templates, the workspace picker, and the accepted assistant alias
resume the same saved setup; no assistant acceptance is needed for the first
three entry points.

Ori still owns the UI and all execution: group creation or reuse and workspace
creation/import. Installing the plugin is no longer a step of this quest: Ori
generates its own "Install Ori REAPER Plugin" quest from its reviewed registry
and hands off to `reaper_setup` once the plugin is installed. The existing
`setup_wizard` continues after workspace creation and is the one place live
control is set up and verified. File-only mode, project-specific live
permissions, exact-project verification, staffing scopes, and confirmation
boundaries are unchanged. Merely opening setup grants nothing.

Quest schema **1**, version **2** has four steps: `project`, `workspace`,
`staffing`, `summary`. Its launch copy names only the group screen. Saved
progress on the older five-step version does not migrate; Ori offers "Start
over", and the group, project and team read as complete again because they
exist. Quest declarations are read-only in Ori.

## Ori Workspace Surface development

The first supported service artifact is **macOS arm64**. Candidate version
**0.7.0** uses Workspace Surface protocol v1 and requires an Ori build with
`assistant_program_v1`, `specialist_setup_journey_v1`, `setup_quests_v2`,
`template_group_requirements_v1`, and `blueprint_inputs_v1`. Older hosts must
refuse this contract rather than ignore its placement rules. An Ori build that
knows only `setup_quests_v1` refuses 0.6.x, an Ori build with `setup_quests_v2`
refuses 0.5.x, and an Ori build without `blueprint_inputs_v1` refuses 0.7.x —
it would reject the whole blueprint manifest on sight, because a host that does
not know the `inputs` block decodes the template strictly and fails.

Reaper Song blueprint v8 asks for **Tempo** (40–240 BPM, default 120) and
**Time signature** (4/4, 3/4, or 6/8, default 4/4) in Ori's Create Workspace
Details step, and writes them into the scaffolded session file's `TEMPO` line.
There is no free-text input, and substitution reaches only the one file the
blueprint listed in `inputs.apply_to`. Musical key stays out of creation: the
first starter task offers it, and the `.rpp` format has no key field anyway.

Blueprint v8 (the v6 group contract, paired with quest
version 2) declares Music Production Home as a **Required** group
with reviewed create-or-reuse behavior. The declaration names only the stable
`music-producer-assistant` program; Ori resolves the current user, plugin owner,
and exact Home. Folder names and ordinary parentage grant no Assistant Program
membership or project authority. Creating from the plugin original reviews the
fixed Required destination. **Customize** creates a source-linked variant where
a user may select None or Recommended; its validated standalone composition
keeps the one `.rpp` project, project-local Producer/Mix Engineer/Songwriter,
starter tasks, File-only mode, optional live-control setup, and exact-project
safeguards, while creating no Home, portfolio, shared stage, link, or Home role.
Its roles declare no `type`: Ori retired the agent Type field and ignores the
key, and every host that accepts this contract already does.

This is a clean-start contract. Existing Ori REAPER workspaces are not adopted,
regrouped, reset, or migrated by name. Connect a fresh external `.rpp` folder
through the reviewed existing-project flow; its files stay in place and grouping
still grants no filesystem or runtime permission.

The local `v0.7.0` candidate artifact is 8,780,098 bytes with SHA-256
`b0f63e5e13607c74294995e8ccbd7802a40baf10b473d4a00d3c5a3a7bf29b29`.
The manifest names the future `v0.7.0` release URL, but local deterministic bytes
are not proof that a remote asset exists or is reachable. Publication and Ori's
reviewed source/artifact pin update are separate approvals; existing installed
plugins and reviewed pins are not rewritten by this candidate.
Build and verify the artifact reproducibly with:

```bash
make artifact-local
make test
make test-ui
make release-package VERSION=v0.7.0
```

Packaging is local and does not publish. Pushing a `v*` tag triggers the release
workflow and uploads the verified binary plus its checksum; that requires a
separate release-owner approval. See [quest migration](docs/setup-quest-migration.md)
for the ownership/resume test, validation boundaries, and remaining rollout steps.
The older [v0.5.0 record](docs/release-v0.5.0.md) describes that historical release.

The build pins Go 1.25.13, disables VCS stamping, uses `-trimpath`, and clears the
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
