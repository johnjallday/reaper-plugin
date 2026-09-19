# Release notes

## 0.8.0 — Connect projects to the independent Music Production Home

- Reaper Song blueprint **v9** replaces its combined `assistant_program` with
  project-owned `assistant_project` schema 1, version 1. REAPER now owns only
  the required project-local Producer, Mix Engineer, and Songwriter under team
  ID `reaper-song-team`.
- The project declaration references provider `music-project-management`, Home
  program `music-producer-assistant`, Home schema/version 1. Music Project
  Management reciprocally authorizes exactly `reaper-plugin` / `reaper-song` /
  `reaper-song-team` schema/version 1. Either package may be installed first;
  grouped creation fails closed until both compatible providers are enabled.
- REAPER no longer declares Portfolio Manager, Sample Library Manager, Home
  stages/reflection/defaults, or the `music-project-management` skill. Existing
  combined workspaces keep their recorded legacy owner and are not adopted,
  migrated, relinked, reset, or renamed.
- `group_requirement` and `standalone_composition` move to schema 2 and bind the
  local project-team ID. The Required grouped path retains reviewed Home
  create/reuse. The supported customized standalone path retains only the three
  project roles, `.rpp` skeleton, typed tempo/time-signature inputs, starter
  tasks, File-only mode, and optional verified live control.
- Quest schema 1 advances to version **3** without changing its four step IDs.
  Staffing copy now names only this project's team and directs Home staffing to
  Music Project Management. Older saved quest progress is preserved but requires
  the existing explicit Start over flow; no workspace or project data is deleted.
- Require `independent_program_homes_v1` instead of the combined
  `assistant_program_v1`. The existing setup, group-requirement, blueprint-input,
  Workspace Surface, runtime, capability, and confirmation contracts remain.
- Plugin/service version: **0.8.0**; Reaper Song blueprint **v9**; protocol
  **1**; macOS arm64 only. Local deterministic candidate artifact: **8,780,098 bytes**;
  SHA-256
  `1f5ab0f061bddb739461ececc088900ec8f4cee47154ea631bb05ebfdfdad08e`.
- No push, PR, tag, release, remote artifact reachability, reviewed Ori pin
  update, real installation, model-backed execution, or live REAPER validation
  is claimed. A compatible Ori release and separately published Music Project
  Management package must precede any compatible REAPER release.

## 0.7.0 — Ask for tempo and time signature at creation

- Reaper Song declares Ori's new `inputs` block: **Tempo** (number, 40–240,
  step 1, default 120, unit BPM) and **Time signature** (select `4 4` / `3 4` /
  `6 8`, default `4 4`). Ori's Create Workspace Details step asks for them, and
  the scaffold's `TEMPO 120 4 4` line becomes
  `TEMPO {{input.tempo}} {{input.time_signature}}`. The blueprint goes to **v8**.
- There is no free-text input, and `inputs.apply_to` names only
  `{{name}}.rpp`, so nothing a user types can reach any other scaffolded file.
  Musical key is deliberately not an input — the `.rpp` format has no key field.
- The first starter task is reworded: it states the session was created with the
  tempo and time signature chosen at creation, tells the agent to read them from
  the file rather than assert them, and still offers to set the key and to
  change anything else. No values are substituted into task text.
- `requires_host_features` gains **`blueprint_inputs_v1`**. This is the release
  gate: an Ori build without it decodes the blueprint manifest strictly, would
  reject the whole template on sight, and now refuses the release before
  installing instead. **Do not publish this release until an Ori release
  contains blueprint-declared inputs.**
- Plugin/service version: **0.7.0**; Reaper Song blueprint **v8**; protocol
  **1**; macOS arm64 only. Requirements and the setup quest are unchanged, and
  Ori's reviewed `MinimumBlueprintVersion` (7) still admits v8.
- Local deterministic candidate artifact: **8,780,098 bytes**; SHA-256
  `b0f63e5e13607c74294995e8ccbd7802a40baf10b473d4a00d3c5a3a7bf29b29`. Only the
  embedded version string differs from 0.6.1; the binary does not embed the
  blueprint.
- No publication, remote artifact reachability, reviewed Ori pin update, or live
  REAPER validation is claimed. Those are separate approvals.

## 0.6.1 — Drop the retired role type

- Reaper Song blueprint roles no longer declare `"type"`. Ori retired the agent
  Type field (johnjallday/ori-agent#490) and ignores the key, so the five
  `assistant_program.roles[]` entries drop it. Nothing else in the template
  changes, and the blueprint stays **v7**. Closes #9.
- Every Ori host that accepts this manifest already ignores the key: 0.6.x
  requires `setup_quests_v2`, which Ori first shipped after #490. Role staffing,
  models, and prompts are unchanged on those hosts.
- A test now fails if any role or agent declares `type` again, and the v4
  migration comparison accounts for the removal without rewriting its frozen
  fixture.
- Plugin/service version: **0.6.1**; Reaper Song blueprint **v7**; protocol
  **1**; macOS arm64 only. Requirements and the setup quest are unchanged.
- Local deterministic candidate artifact: **8,780,098 bytes**; SHA-256
  `88c7dfd5ebf6a855ae41994a080c2339f392514f68ff47366463b5a84c5eb8c8`. Only the
  embedded version string differs from 0.6.0; the binary does not embed the
  blueprint.
- No publication, remote artifact reachability, reviewed Ori pin update, or live
  REAPER validation is claimed. Those are separate approvals.

## 0.6.0 — Four-step setup quest

- `reaper_setup` is now quest version **2** with four steps: `project`,
  `workspace`, `staffing`, `summary`. The `integration` step is removed because
  Ori generates its own install quest from its reviewed registry and hands off
  to this quest once the plugin is installed.
- `workspace_launch` keeps only `group_title` and `group_name`. The runtime
  preparation screen is gone; the workspace `setup_wizard` is the one place
  live control is set up and verified.
- Require `setup_quests_v2` instead of `setup_quests_v1`. Ori hosts without
  `setup_quests_v2` refuse this manifest, and hosts with it refuse 0.5.x.
- Saved progress on quest version 1 does not migrate. Ori offers "Start over";
  the existing group, project and team read as complete on the fresh run.
- Plugin/service version: **0.6.0**; Reaper Song blueprint **v7**; protocol
  **1**; macOS arm64 only. The blueprint template, `setup_wizard`, group
  requirement and standalone composition are unchanged.
- Local deterministic candidate artifact: **8,780,098 bytes**; SHA-256
  `4def4fec14ecf083b0358c686c608514d4b9afff99dd810f1184213312770119`.
- No publication, remote artifact reachability, reviewed Ori pin update, or live
  REAPER validation is claimed. Those are separate approvals.

See [migration and verification](docs/setup-quest-migration.md#060-addendum-install-step-moved-to-ori).

## 0.5.2 — Reviewed template group requirements

- Reaper Song blueprint **v6** declares a Required Music Production Home using
  strict `group_requirement` v1 with reviewed `offer_create` behavior and the
  stable `music-producer-assistant` target.
- Add strict `standalone_composition` v1 prompts for the project-local Producer,
  Mix Engineer, and Songwriter. Source-linked None or Recommended variants keep
  project files, tasks, skills, runtime modes, and exact-project controls while
  removing Home, portfolio, shared-stage, cross-project, and Home-role claims.
- Require `template_group_requirements_v1`; older Ori hosts must refuse the
  candidate rather than silently bypass placement review.
- Preserve `reaper_setup` v1, its five steps, the post-workspace setup wizard,
  File-only behavior, live-control gates, `.rpp` selection, and external-folder
  containment. Existing Ori REAPER records are not inferred, adopted, or reset.
- Plugin/service version: **0.5.2**; Reaper Song blueprint **v6**; protocol **1**;
  macOS arm64 only.
- Local deterministic candidate artifact: **8,780,098 bytes**; SHA-256
  `999dda3764c85487329322bbf7df775fb389316b9c8e4320bf8dd62c5fa91987`.
- No remote artifact reachability, publication, reviewed Ori pin update, install,
  or live REAPER validation is claimed. Those are separate approvals and
  evidence boundaries.

See [template group contract and verification](docs/template-group-requirements-v0.5.2.md).

## 0.5.1 — Plugin-owned setup quest

- The plugin manifest now owns `reaper_setup`; Reaper Song blueprint **v5**
  references it with `setup_quest`. Ori renders the quest and retains execution
  authority, confirmations, group reuse, and workspace creation/import.
- Require `setup_quests_v1` in addition to both existing host features. Older Ori
  builds must refuse rather than silently ignore the quest.
- Preserve quest schema/version **1**, all five step IDs and display copy, and
  the entire post-workspace template aside from its new reference. Existing
  wizard, file-only mode, live-control disclosures, project connection, roles,
  and starter tasks are unchanged.
- Plugin/service version: **0.5.1**; protocol **1**, macOS arm64 only.
- Release artifact: **8,780,098 bytes**; SHA-256
  `baeca80db6b156c25207784355d3c2a4717169533f02d85d543e4ea7a5dd6633`.
- Publication does not update installed plugins, grant project access, or
  unlock Ori's reviewed-install gate. This version requires the host support in
  [Ori PR #466](https://github.com/johnjallday/ori-agent/pull/466), followed by a
  reviewed-source/artifact pin update in Ori. That host PR remains open at
  release preparation, with its README Contract check unresolved. Older hosts
  must not install this version; existing v0.5.0 users retain compatibility
  setup. No live REAPER validation is claimed.

See [migration and verification](docs/setup-quest-migration.md).

## 0.5.0 — Specialist setup and scoped assistants

This release adds the inert contracts required by Ori's generic
specialist setup journey. Reaper Song blueprint v4 supports separately reviewed
new-project and attach-existing flows while keeping all filesystem selection,
containment, workspace creation, and mutations in the host.

### Added

- Exact `new_project` and `existing_project` connection modes, with attach
  discovery constrained to `.rpp` entries and starter tasks filtered by mode.
- Assistant Program schema v2 role scope: one required Home Music Portfolio
  Manager; required per-project Producer, Mix Engineer, and Songwriter; and one
  optional Home Sample Library Manager associated with `sample_library`.
- Required `specialist_setup_journey_v1` host feature alongside the existing
  Assistant Program feature, so unsupported Ori builds fail before install.
- Runtime prerequisite/readiness disclosure names the exact staged runner
  destination and required manual Action List registration. Staging does not
  count as registration or live verification.
- Bounded verification reason codes distinguish offline, wrong/missing project,
  timeout, unsafe exchange, runner failure, and invalid response.

### Safety and compatibility

- The plugin declaration remains data only. It contains no picker, scanner,
  absolute path, command, route, setup action, or authority grant.
- Existing-project connection does not write Ori metadata into the selected
  folder and does not imply live control, staffing, indexing, or task execution.
- Build toolchain: Go `1.25.13`, replacing the original candidate's Go 1.25.0
  standard library. CI and release publishing check the actual binary with
  `govulncheck`, in addition to tests and artifact identity checks.
- Plugin/service version: `0.5.0`
- Reaper Song blueprint version: `4`
- Workspace Surface protocol: `1`
- Release artifact: macOS arm64, `8,780,098` bytes
- Release SHA-256: `2bbf6b77418119cb21e827a407c8d5886e3effdb593ec0ad274e20d7d69c2ca9`
- Publication does not unlock Ori's reviewed installation gate. Ori still needs
  a separate final-source-pin and enablement change after remote-byte verification.
- Live REAPER control has not been revalidated for this version against a real
  session. Publication was authorized with that limitation outstanding;
  deterministic tests and local installation do not replace separately approved
  disposable-project live validation or grant application access.
- The current scoped-team host browser suite passes 11/11. The older coordinated
  command remains 3/4: one host test expects obsolete shared-roster behavior.
  That test still needs reconciliation; this release does not claim it passed.
  See [release preparation](docs/release-v0.5.0.md) for the complete evidence.

## 0.4.1 — Shared Music Producer Assistant

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
- Plugin/service version: `0.4.1`
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
