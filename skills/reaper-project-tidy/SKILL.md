---
name: reaper-project-tidy
description: Survey an open REAPER project for cosmetic track-color and marker/region cleanup, produce a reviewable proposal, and apply only approved items as one undo step. Use when the user asks to tidy, clean up, or organize a REAPER project without changing its sound.
---

# REAPER Project Tidy (propose first, cosmetic only)

Project Tidy is a two-phase workflow:

1. **Survey** the authoritative open REAPER project with the canonical read-only
   inspector and write a reviewable proposal.
2. **Apply** only the items the user approved with the canonical applier, then
   report applied, skipped, and failed items.

The invariant is absolute: **tidy never changes how the project sounds.** The v1
plan language can color tracks, rename markers/regions, and delete an exact
duplicate marker. It cannot express item edits, FX, routing, gain, pan, tempo,
render, track renaming, or track movement.

Read [`reaper-web-remote`](../reaper-web-remote/SKILL.md) first. It is the single
source for Web Remote port discovery, reachability checks, runner discovery, and
triggering `~/.ori-reaper/inbox.lua`. Do not duplicate or bypass that setup.

## Canonical assets

Resolve paths relative to this skill directory:

- `scripts/inspect_project.lua` — audited no-undo inspector
- `scripts/apply_tidy_plan.lua` — strict two-phase cosmetic applier
- `schema/inspected-state-v1.json` — `state.json` contract
- `schema/edit-plan-v1.json` — reviewed `plan.json` contract
- `schema/apply-result-v1.json` — `apply_result.json` contract
- `schema/examples/` — complete valid examples

Always copy and run these exact scripts. Never ask an LLM to compose replacement
Lua and never append project-specific code to a canonical script. The LLM may
compose the declarative plan JSON, which must validate against the closed v1
contract before it is offered or applied.

### Ori capability tasks

When Ori grants `reaper_live_control` to a CLI-backed workspace task, arbitrary
localhost shell access remains disabled by design. For the Survey phase, call the
trusted agent operation `tidy.survey` with an empty input instead of using
`curl` or writing the runner exchange directly. Ori exposes it to the model as
`plugin_reaper_plugin_reaper_live_control_tidy_survey`; that brokered name is
the one to call. The operation performs the
live preflight, runs the exact canonical inspector through its audited no-undo
path, validates the bounded state, verifies the host-injected project entry,
loads or seeds `conventions.md`, generates only the closed v1 verbs, and
persists the proposal artifacts. Report its bounded result; never recreate its
work with ad-hoc shell commands.

The direct shell procedure below remains the portable path when this skill is
used outside an Ori capability-scoped task.

## Before either phase

1. Follow `reaper-web-remote` to discover the live `$PORT`; do not assume the
   configured port is the listener REAPER actually acquired.
2. Confirm `/_/TRANSPORT` is reachable and `~/.ori-reaper/runner.id` exists. If
   either check fails, stop with the specific setup instruction from that skill.
3. Confirm the task is authorized for `reaper_live_control`. In Ori, survey and
   apply must each be a capability-declared workspace task with
   `required_capabilities: ["reaper_live_control"]`.
4. Do **not** declare `file_fallback_for` and do not parse or edit the `.rpp` as a
   fallback. Live control unavailable means the phase stops honestly.
5. Establish which workspace project should be open. After inspection, compare
   the reported project name/path with the workspace project entry. An empty
   path is valid for a genuinely unsaved project, but it is not permission to
   guess that the correct project is open.

## Running a canonical script

Outside an Ori capability-scoped task, use the serialized runner exchange
described by `reaper-web-remote`. Clear the
old status and the expected output first so stale files cannot be mistaken for
this run:

```sh
ROOT="$HOME/.ori-reaper"
RUNNER_ID=$(tr -d '[:space:]' < "$ROOT/runner.id")
rm -f "$ROOT/last_status.txt"
cp "<skill-dir>/scripts/inspect_project.lua" "$ROOT/inbox.lua"
chmod 0600 "$ROOT/inbox.lua"
curl -sS -m 8 "http://127.0.0.1:$PORT/_/$RUNNER_ID" >/dev/null 2>&1 || true
# Poll briefly for last_status.txt; opening/loading work can outlive curl.
cat "$ROOT/last_status.txt"
```

For the inspector, expect `ok` plus a new `state.json`. For the applier, expect
`ok` plus a new `apply_result.json`. Any `error:` status is authoritative: show a
safe summary, do not consume an older output, and do not claim success.

The reserved first lines in the canonical scripts are runner modes, not user
options:

- `read-only-inspector-v1` permits only an audited pure-read REAPER API list,
  suppresses REAPER's automatic action undo point, and performs no arrange
  refresh.
- `tidy-applier-v1` validates and stale-checks before the runner opens one undo
  block. Ordinary scripts cannot use either mode through the generic service
  entry point.

## Phase 1: survey and propose

### 1. Run the inspector

In an Ori capability-scoped task, invoke
`plugin_reaper_plugin_reaper_live_control_tidy_survey` and use its result; the
trusted `tidy.survey` operation performs all of the steps in this Survey phase.
Otherwise:

1. Remove stale `~/.ori-reaper/state.json`.
2. Copy `scripts/inspect_project.lua` to `inbox.lua` and trigger the runner.
3. Require `last_status.txt` to equal `ok`.
4. Read the new `state.json` only after checking that it is a regular,
   non-symlink file no larger than 4 MiB.
5. Validate it against `schema/inspected-state-v1.json` or the plugin's
   executable `DecodeTidyInspectedState` validator.
6. Record `project.project_change_count`. A second unchanged inspection must
   report the same count; the inspector itself creates no undo point.

`tracks[].index` and `markers[].enumeration_index` are presentation positions,
not identities. Tracks are targeted by their opaque braced GUID. Markers and
regions are targeted by `(is_region, id)` because REAPER permits a marker and a
region to share one numeric ID.

### 2. Read conventions

Read `<workspace-project>/conventions.md` before proposing anything. For an old
workspace where it is missing, seed the plugin's Reaper Song defaults first. If
it is empty or unusable, use those defaults and say so. Never invent a color for
an unknown role: leave unknown track names uncolored.

Every proposed item needs a musician-readable line and a reason tied to one of:

- an exact convention, such as “vocals are blue”;
- the configured marker/region casing or numbering rule; or
- an exact duplicate rule visible in the inspected state.

### 3. Build only v1 items

A plan has `schema_version: 1`, one stable `plan_id`, the inspected project name,
path, and change count, and 1–64 uniquely identified items. Stable item IDs use
only letters, digits, `.`, `_`, and `-` and do not change when the proposal is
filtered for apply.

The only verbs and shapes are:

| Verb | Target snapshot | Payload |
|---|---|---|
| `set_track_color` | `track_guid` | RGB `color` |
| `rename_marker` | marker `marker_id` + exact `snapshot_name` | `new_name` |
| `rename_region` | region `marker_id` + exact `snapshot_name` | `new_name` |
| `delete_marker` | marker ID + exact snapshot name + exact position in seconds | a different `survivor_marker_id` |

For deletion, target and survivor must currently share the exact binary64
position and have the same name or one empty name. A 1 ms tolerance is not safe:
REAPER preserves distinct markers at +0.5 ms and +1 ms. Serialize seconds with
enough precision for a binary64 round trip; never round to milliseconds.

Use `schema/examples/edit-plan-v1.json` as the complete example. Unknown verbs,
unknown fields, duplicate JSON keys, malformed payloads, duplicate item IDs,
duplicate mutation targets, unsupported versions, trailing JSON, and oversized
files are whole-plan refusals. Do not silently drop invalid items.

### 4. Persist the proposal

Before showing a proposal, validate the complete plan. Persist the machine JSON
and human Markdown summary under the workspace project's `tidy/` directory with
an ID/timestamp and atomic same-directory temp rename. The summary line format
is:

> target — current → proposed — reason

A survey with no items is a successful “project already tidy” artifact, not an
empty invalid edit plan. Surveying never writes `plan.json` into the runner
exchange and never changes REAPER.

## Review boundary

Stop after presenting the proposal. Items default checked, but the user may
uncheck any item or dismiss the proposal. Dismissal touches only workspace
proposal state. A newer survey supersedes an older open proposal.

Apply receives only the stable IDs the user selected. Rebuild a selected-only
plan from the already validated persisted proposal; never trust item bodies,
capabilities, paths, or verbs supplied by a frame/browser request.

## Phase 2: apply approved items

### 1. Re-run preflight and inspect

Repeat the live-control preflight and canonical inspection immediately before
apply. Compare its project name/path to the plan and compare its change count to
`inspected_project.project_change_count`.

A changed count is not itself a reason to guess or bulk-refuse. Continue to the
canonical applier, whose per-item checks skip stale targets safely. A different
open project is a hard stop.

### 2. Stage the selected plan atomically

1. Validate the selected-only document again against the v1 plan contract.
2. Require at least one selected item.
3. Write it to a mode-`0600` temp file inside `~/.ori-reaper`.
4. Close the temp file and atomically rename it to
   `~/.ori-reaper/plan.json`.
5. Remove any stale `last_status.txt` and `apply_result.json`.

Do not alter snapshots while filtering. In particular, do not refresh names or
positions to make a stale item pass.

### 3. Run the canonical applier

Copy `scripts/apply_tidy_plan.lua` to `inbox.lua` and trigger the same runner.
The applier performs these phases synchronously:

1. bounded JSON parse with duplicate-key rejection;
2. exact schema/verb/payload validation for the whole plan;
3. stable GUID and typed marker/region lookup;
4. snapshot and exact-duplicate checks, producing pending or skipped rows;
5. one runner-owned undo block for all pending cosmetic mutations;
6. result finalization after `Undo_EndBlock`, when the project change count is
   authoritative; and
7. atomic replacement of `apply_result.json`.

A malformed or unknown-verb plan reaches no mutation and creates no empty undo
point. A fully stale plan writes skipped rows but likewise creates no undo point.

### 4. Read and report the result

Require a regular, non-symlink `apply_result.json` no larger than 1 MiB and
validate it against `schema/apply-result-v1.json`. Bind each result row to the
selected plan in the same order by both stable item ID and verb.

Statuses mean:

- `applied` — the approved cosmetic mutation ran;
- `skipped` — the live target no longer matched; show the exact reason; or
- `failed` — the approved API call failed; show the bounded error.

If every item was skipped, tell the user to run a fresh survey. Do not present a
fully skipped apply as success. For any applied item, include this exact sentence
in the workspace artifact and note:

> All applied changes are a single undo step in REAPER (one Ctrl+Z reverts everything).

Persist a bounded JSON and Markdown report under the workspace `tidy/` directory
and create the human report with the existing `workspace_notes` tool. The file
artifact remains the recoverable source if note creation fails.

## Failure handling

- **REAPER/Web Remote/runner unavailable:** stop and point to setup; no `.rpp`
  fallback.
- **Wrong project:** stop before staging or triggering an apply.
- **Inspector status error or missing state:** do not propose from an older
  `state.json`.
- **Plan validation error:** reject the whole plan and request regeneration.
- **Stale item:** preserve it as a skipped result; never retarget by index/name.
- **All items skipped:** request a fresh survey.
- **Apply status error:** do not read a stale result and do not claim an undoable
  change occurred.
- **Report/note write failure:** preserve and report the validated runner result;
  never rerun mutations merely to regenerate prose.

## Guardrails

- Propose first. Never auto-apply in v1.
- Run only the checked-in canonical scripts.
- Keep plan JSON declarative and limited to the four verbs.
- Never target tracks by index or markers by enumeration index.
- Never use a millisecond dedupe tolerance.
- Never launch/quit REAPER or use app automation.
- Never edit the `.rpp` as fallback.
- Never create a second undo block inside the applier.
- Report every selected item as applied, skipped, or failed.
