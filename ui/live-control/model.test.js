import { test } from "node:test";
import assert from "node:assert/strict";
import {
  actionOperation,
  folderRows,
  meaningfulStateKey,
  normalizePinnedState,
  optimisticTrackPatch,
  promptChips,
  reorderPins,
} from "./model.js";

test("action tiers choose the same safe or host-confirmed operation for UI and agent policy", () => {
  assert.equal(actionOperation({ tier: "silent" }), "actions.run_safe");
  assert.equal(actionOperation({ tier: "undoable" }), "actions.run_safe");
  assert.equal(actionOperation({ tier: "confirm" }), "actions.run_confirmed");
  assert.equal(
    actionOperation({ needs_confirmation: true }),
    "actions.run_confirmed",
  );
});

test("meaningful state ignores meters and timestamps but includes every editable project fact", () => {
  const base = {
    connected: true,
    project: "Song",
    track_editing_available: true,
    folder_depth_available: true,
    tracks: [
      {
        index: 1,
        name: "Drums",
        muted: false,
        folder_depth: 0,
        peak_left_db: -20,
      },
    ],
  };
  assert.equal(
    meaningfulStateKey(base),
    meaningfulStateKey({
      ...base,
      checked_at: "later",
      tracks: [{ ...base.tracks[0], peak_left_db: -2 }],
    }),
  );
  assert.notEqual(
    meaningfulStateKey(base),
    meaningfulStateKey({
      ...base,
      tracks: [{ ...base.tracks[0], muted: true }],
    }),
  );
});

test("folder rows bound indentation and refuse parent moves while flat tracks remain movable", () => {
  const rows = folderRows(
    [
      { index: 1, name: "Band", folder_depth: 1 },
      { index: 2, name: "Drums", folder_depth: 0 },
      { index: 3, name: "Bass", folder_depth: -1 },
      { index: 4, name: "Vox", folder_depth: 0 },
    ],
    true,
  );
  assert.deepEqual(
    rows.map((row) => [row.level, row.folderParent, row.moveAllowed]),
    [
      [0, true, false],
      [1, false, true],
      [0, false, true],
      [0, false, true],
    ],
  );
  assert.equal(folderRows([{ folder_depth: 1 }], false)[0].moveAllowed, false);
});

test("prompt chips are ordered capped and absent only when no condition applies", () => {
  assert.deepEqual(
    promptChips({
      connected: true,
      track_count: 1,
      track_editing_available: true,
      tracks: [{ name: "Drums" }],
    }),
    [],
  );
  const chips = promptChips({
    connected: false,
    track_count: 0,
    track_editing_available: false,
    tracks: [{ name: "" }],
  });
  assert.equal(chips.length, 4);
  assert.match(chips[0], /restore/i);
  assert.match(chips[2], /unnamed/i);
});

test("pins distinguish missing from explicit empty and reorder without changing the set", () => {
  assert.deepEqual(normalizePinnedState({ found: false }, ["a", "b"]), [
    "a",
    "b",
  ]);
  assert.deepEqual(
    normalizePinnedState({ found: true, value: { ids: [] } }, ["a"]),
    [],
  );
  assert.deepEqual(reorderPins(["a", "b", "c"], 1, -1), ["b", "a", "c"]);
  assert.deepEqual(reorderPins(["a", "b"], 0, -1), ["a", "b"]);
});

test("optimistic patches touch only the requested track fact for rollback-friendly rendering", () => {
  const track = {
    name: "Old",
    muted: false,
    soloed: false,
    armed: false,
    color: 0,
  };
  assert.deepEqual(
    optimisticTrackPatch(track, { operation: "rename", new_name: "New" }),
    {
      ...track,
      name: "New",
    },
  );
  assert.equal(
    optimisticTrackPatch(track, { operation: "mute", new_bool: true }).muted,
    true,
  );
  assert.equal(track.muted, false, "source remains the rollback snapshot");
});
