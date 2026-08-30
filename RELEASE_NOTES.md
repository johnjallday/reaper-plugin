# Release notes

## 0.5.0 — Shared Music Producer Assistant

This release makes the Reaper Song blueprint opt in to Ori's generic shared
assistant-program host contract. Creating a song links an inert Producer Home;
the user explicitly hires a named Producer plus bounded Mix Engineer and
Songwriter roles, then progresses from Helper to Collaborator through accepted
work only.

### Added

- Blueprint-owned roster prompts, stage copy, reflection rubric, and
  collaborator suggestion capability requirements.
- Reaper Song blueprint version 3 with no automatically seeded legacy producer.
- Explicit `assistant_program_v1` host compatibility requirement so older Ori
  builds fail closed instead of silently dropping the assistant declaration.

### Safety and compatibility

- Conversation never controls REAPER. Suggested changes enter Action Center and
  Backlog, then retain ordinary confirmation, readiness, capability, filesystem,
  runtime, and undo gates.
- Reflection is host-bounded and read-only; only user-approved learnings can
  affect later prompts.
- Plugin/service version: `0.5.0`
- Reaper Song blueprint version: `3`
- Required Ori host feature: `assistant_program_v1`
- Workspace Surface protocol: `1`
- Service artifact: macOS arm64 only
- Release SHA-256 and size are pinned in `.ori-plugin/plugin.json`; publication
  remains a separate, tag-triggered release-owner action.

## 0.4.0 — Project Tidy

This release adds a propose-first, sound-safe cleanup workflow to plugin-created
Reaper Song workspaces.

### Added

- Audited read-only inspector and strict cosmetic applier with one REAPER undo
  block for all selected changes.
- Convention-aware proposals for track colors, marker/region naming, and exact
  duplicate markers, with durable plan/summary/report artifacts.
- Project Folder action, proposal badge, accessible review checklist, dismiss /
  supersede lifecycle, selected-only Apply, and recoverable workspace notes.
- Capability-scoped `tidy.survey` and `tidy.apply_selection` agent operations so
  Codex CLI tasks can use Ori's broker without arbitrary localhost shell access.

### Safety and compatibility

- The closed plan language still has exactly four verbs and cannot express item,
  FX, routing, gain, pan, tempo, render, track rename, or track movement edits.
- Survey creates no REAPER undo point. Apply rejects malformed plans before
  mutation, stale-checks every target, and runs checked pending rows as one undo.
- Plugin/service version: `0.4.0`
- Reaper Song blueprint version: `2`
- Workspace Surface protocol: `1`
- Service artifact: macOS arm64 only
- Release artifact size: `8,763,458` bytes
- Release SHA-256: `591d8703f5bebe6339e61f983faa896cd7cf82dce4d2915b2fb7dab6df096778`

## 0.3.0 — Workspace Surface

This release adds the optional Ori Workspace Surface adapter while preserving
the existing Claude/Codex skills and helper CLI.

### Added

- Workspace Surface protocol v1 manifest and MCP stdio private service.
- Sandboxed Live Control station/modal with one-second visible polling.
- Live state, transport/actions, registered and global scripts, host-reviewed
  script proposals, namespaced pinned actions, guarded track edits and specific
  Undo, and all-or-nothing bulk plans.
- Plugin-owned Reaper Song blueprint, runtime/setup provider, agent operations,
  starter tasks, project seed, and explicit project-file fallback declaration.
- Reproducible macOS arm64 build, digest verifier, clean Ori install verifier,
  race/UI/security CI, and an opt-in disposable live-project parity test.

### Compatibility and trust

- Plugin version: `0.3.0`
- Workspace Surface protocol: `1`
- Minimum Ori protocol: `1`
- Service artifact: macOS arm64 only
- Release artifact size: `8,476,866` bytes
- Release SHA-256: `06fdf6623a17737cbad38241584e198f2061956127b29efb3dc4219365ecfa93`

The service is trusted native code. Ori keeps browser code in an opaque-origin
sandbox and mediates every UI/agent operation through workspace ownership,
attachment, schema, grant, policy, confirmation, timeout, output, and redaction
checks. Unsupported platforms remain unavailable and never launch another
artifact.

Legacy workspaces from Ori's retired compiled Reaper Song template are not
migrated or automatically attached. No pins, grants, setup history, provenance,
tasks, or project metadata are imported. The supported path is a new workspace
created from this plugin's blueprint.

The release asset is selected by exact platform and downloaded over HTTPS;
Ori verifies this digest before making the managed copy executable.
