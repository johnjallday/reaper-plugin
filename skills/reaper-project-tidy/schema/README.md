# REAPER Project Tidy v1 JSON contracts

These three closed JSON Schemas freeze the exchange between the canonical Lua
primitives, the plugin service, workspace artifacts, and the proposal surface:

- `inspected-state-v1.json` — read-only `state.json`
- `edit-plan-v1.json` — reviewed `plan.json`
- `apply-result-v1.json` — per-item `apply_result.json`

`internal/reaper/tidy.go` is the executable validator. It additionally enforces
cross-field rules JSON Schema cannot conveniently express: contiguous zero-based
presentation indexes, unique track GUIDs and typed marker IDs, unique stable
item IDs and mutation targets, region end after start, rename non-no-ops, exact
per-verb target/payload shapes, result counts that do not decrease, and exact
result-to-plan row binding. All decoders reject duplicate keys, unknown fields,
trailing documents, oversized input, unsupported schema versions, and malformed
items before returning any value for execution.

## Identity and positions

- Track identity is the opaque braced `guid`; `index` is presentation only.
- Marker identity is `(is_region, id)`; `enumeration_index` is presentation
  only. REAPER permits a marker and region to share one numeric ID.
- Marker and region positions are seconds serialized with enough precision for
  a binary64 round trip. `delete_marker` uses exact snapshot positions. There
  is deliberately no 1 ms dedupe tolerance.
- An empty snapshot marker name is valid and therefore required fields are
  distinguished from omitted fields.

## Mutation language

v1 contains exactly four verbs:

1. `set_track_color`
2. `rename_marker`
3. `rename_region`
4. `delete_marker`

The contract cannot express track renames, item movement, FX, routing, gain,
pan, tempo, or render changes. `delete_marker` must name a different surviving
marker ID; the applier is responsible for proving both current markers are exact
duplicates before deletion.

## Result finalization

The mutation runner owns one undo block for the whole accepted plan. It must
sample `project_change_count_after` and atomically finalize the result only
after `Undo_EndBlock`; the live spike proved that state-change counts do not
settle authoritatively from inside the block.

See `examples/` for complete documents and
`scripts/spikes/reaper-project-tidy/` for the live REAPER evidence that settled
these choices.
