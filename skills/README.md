# Skills

Tool-agnostic agent skills for driving REAPER over its **Web Remote** HTTP
interface using only the agent's built-in **shell** (curl) — no MCP server. A
skill is a `SKILL.md` file (YAML frontmatter + markdown instructions) that
teaches an AI agent *how* to perform a workflow. For the fiddly bits (registering
ReaScripts in `reaper-kb.ini`), the skills call the optional helper binary at
`${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin`.

The same `SKILL.md` works across **Ori**, **Claude** (Code / Desktop), and
**Codex**. Portable use relies on shell + localhost HTTP. Ori's
capability-scoped Codex posture instead keeps arbitrary localhost access off and
exposes the exact Project Tidy survey/apply operations through its authorized
brokered tool loop; no private plugin service is attached wholesale.

## Available skills

| Skill | What it does |
|-------|--------------|
| [`reaper-web-remote`](reaper-web-remote/SKILL.md) | Core Web Remote playbook: discover the port, read transport/tracks, run actions and registered ReaScripts by command ID. |
| [`reaper-session-setup`](reaper-session-setup/SKILL.md) | Insert, name, color, and record-arm tracks to a requested layout (writes + registers a ReaScript, runs it via Web Remote). |
| [`reaper-project-tidy`](reaper-project-tidy/SKILL.md) | Survey convention-aware cosmetic cleanup, review every proposal row, and apply only checked changes as one REAPER undo step. |

## Prerequisites

- REAPER running with the **Web Remote** interface enabled
  (Preferences → Control/OSC/web → add "Web browser interface").
- Localhost network access from the agent (default in the CLI sandbox posture).
- Optional: build the helper CLI once (`make build` in the repo root) so
  `bin/reaper-plugin` exists for ReaScript registration.

## Install

Each tool loads skills from its own skills directory; installing a skill means
copying its folder there. From the repo root:

**Ori**
```bash
cp -R skills/reaper-web-remote skills/reaper-session-setup skills/reaper-project-tidy ~/.agents/skills/
```
Then enable/bind the skill to your REAPER workspace.

**Claude (Code / Desktop)**
```bash
cp -R skills/reaper-web-remote skills/reaper-session-setup skills/reaper-project-tidy ~/.claude/skills/
```

**Codex**
```bash
cp -R skills/reaper-web-remote skills/reaper-session-setup skills/reaper-project-tidy ~/.codex/skills/
```

> Make sure REAPER is running with the Web Remote interface enabled before you
> invoke a skill. Installing the whole plugin (rather than copying individual
> skill folders) also makes `${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin` available.
