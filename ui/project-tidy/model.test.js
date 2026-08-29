import { test } from "node:test";
import assert from "node:assert/strict";
import {
  applyVariables,
  canApply,
  normalizeProposal,
  proposalMessage,
  normalizeReport,
  toggleSelection,
} from "./model.js";

test("open proposal defaults every validated item checked and exposes no raw plan", () => {
  const proposal = normalizeProposal({
    proposal_id: "proposal-1",
    state: "open",
    items: [
      {
        item_id: "one",
        line: "Track Vox — no color → blue",
        reason: "vocals are blue",
      },
      {
        item_id: "two",
        line: "Marker 2 — chorus → Chorus",
        reason: "Title Case",
      },
    ],
    plan: { forbidden: true },
  });
  assert.equal(proposal.open, true);
  assert.deepEqual([...proposal.selected], ["one", "two"]);
  assert.equal("plan" in proposal, false);
  assert.equal(
    proposalMessage(proposal),
    "2 cosmetic changes ready for review.",
  );
});

test("selection payload preserves proposal order and excludes unchecked items", () => {
  let proposal = normalizeProposal({
    proposal_id: "proposal-1",
    state: "open",
    items: [
      { item_id: "one", line: "One", reason: "Reason one" },
      { item_id: "two", line: "Two", reason: "Reason two" },
    ],
  });
  proposal = toggleSelection(proposal, "one", false);
  assert.equal(canApply(proposal), true);
  assert.deepEqual(applyVariables(proposal), {
    proposal_id: "proposal-1",
    selected_item_ids: ["two"],
  });
  proposal = toggleSelection(proposal, "two", false);
  assert.equal(canApply(proposal), false);
  assert.equal(applyVariables(proposal), null);
});

test("invalid duplicate and oversized rows fail closed", () => {
  const proposal = normalizeProposal({
    proposal_id: "proposal-1",
    state: "open",
    items: [
      { item_id: "one", line: "One", reason: "Reason" },
      { item_id: "one", line: "Duplicate", reason: "Reason" },
      { item_id: "bad", line: "", reason: "Reason" },
    ],
  });
  assert.deepEqual(
    proposal.items.map((item) => item.id),
    ["one"],
  );
});

test("last report keeps failed and skipped rows prominent and recommends a fresh survey", () => {
  const report = normalizeReport({
    proposal_id: "proposal-1",
    outcome: "fully_skipped",
    fresh_survey_recommended: true,
    undo_summary: "One undo.",
    items: [
      {
        id: "one",
        verb: "rename_marker",
        status: "skipped",
        reason: "snapshot changed",
      },
      {
        id: "two",
        verb: "set_track_color",
        status: "failed",
        error: "API failed",
      },
    ],
  });
  assert.equal(report.freshSurveyRecommended, true);
  assert.deepEqual(
    report.items.map((item) => [item.status, item.detail]),
    [
      ["skipped", "snapshot changed"],
      ["failed", "API failed"],
    ],
  );
});

test("already tidy and dismissed states cannot apply", () => {
  for (const state of ["already_tidy", "dismissed", "superseded", "none"]) {
    const proposal = normalizeProposal({
      proposal_id: "proposal-1",
      state,
      items: [],
    });
    assert.equal(canApply(proposal), false);
    assert.match(
      proposalMessage(proposal),
      /survey|dismissed|superseded|cosmetic/i,
    );
  }
});
