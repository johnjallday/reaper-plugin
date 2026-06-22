# Skills

Tool-agnostic agent skills for driving REAPER over its **Web Remote** HTTP
interface using only the agent's built-in **shell** (curl) — no MCP server. A
skill is a `SKILL.md` file (YAML frontmatter + markdown instructions) that
teaches an AI agent *how* to perform a workflow. For the fiddly bits (registering
ReaScripts in `reaper-kb.ini`), the skills call the optional helper binary at
`${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin`.

Because the skills rely only on shell + localhost HTTP, the same `SKILL.md` works
across **Ori**, **Claude** (Code / Desktop), and **Codex** — including inside the
CLI sandbox posture, where localhost network is enabled but MCP tool calls and app
automation are not required.

## Available skills

| Skill | What it does |
|-------|--------------|
| [`reaper-web-remote`](reaper-web-remote/SKILL.md) | Core Web Remote playbook: discover the port, read transport/tracks, run actions and registered ReaScripts by command ID. |
| [`reaper-session-setup`](reaper-session-setup/SKILL.md) | Insert, name, color, and record-arm tracks to a requested layout (writes + registers a ReaScript, runs it via Web Remote). |

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
cp -R skills/reaper-web-remote skills/reaper-session-setup ~/.agents/skills/
```
Then enable/bind the skill to your REAPER workspace.

**Claude (Code / Desktop)**
```bash
cp -R skills/reaper-web-remote skills/reaper-session-setup ~/.claude/skills/
```

**Codex**
```bash
cp -R skills/reaper-web-remote skills/reaper-session-setup ~/.codex/skills/
```

> Make sure REAPER is running with the Web Remote interface enabled before you
> invoke a skill. Installing the whole plugin (rather than copying individual
> skill folders) also makes `${CLAUDE_PLUGIN_ROOT}/bin/reaper-plugin` available.
