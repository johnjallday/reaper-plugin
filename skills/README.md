# Skills

Tool-agnostic agent skills for driving REAPER through the `reaper-plugin` MCP server (tool name `ori-reaper`). A skill is a
`SKILL.md` file (YAML frontmatter + markdown instructions) that teaches an AI agent *how* to use
the MCP for a specific workflow. The MCP gives the agent hands; the skill is the playbook.

Because `reaper-plugin` is a standard MCP server, the same `SKILL.md` works across **Ori**,
**Claude** (Code / Desktop), and **Codex** — the instructions are identical and the frontmatter is
compatible (tool-specific fields such as `required_mcp_servers` are ignored by tools that don't
use them).

## Available skills

| Skill | What it does |
|-------|--------------|
| [`reaper-session-setup`](reaper-session-setup/SKILL.md) | Insert, name, color, and record-arm tracks to a requested layout. |

## Prerequisites

- REAPER running with the **Web Remote** interface enabled
  (Preferences → Control/OSC/web → add "Web browser interface").
- The `ori-reaper` MCP server (from the `reaper-plugin` bundle) registered in your AI tool (see the repo [README](../README.md)).

## Install

Each tool loads skills from its own skills directory; installing a skill means copying its folder
there. From the repo root:

**Ori**
```bash
cp -R skills/reaper-session-setup ~/.agents/skills/
```
Then enable/bind the skill to your REAPER workspace.

**Claude (Code / Desktop)**
```bash
cp -R skills/reaper-session-setup ~/.claude/skills/
```

**Codex**
```bash
cp -R skills/reaper-session-setup ~/.codex/skills/
```

> Make sure the `ori-reaper` server is configured in the same tool, and REAPER is running, before
> you invoke a skill.
