const STATES = new Set([
  "none",
  "open",
  "dismissed",
  "superseded",
  "already_tidy",
  "applied",
  "apply_skipped",
  "apply_failed",
]);

function safeText(value, maximum) {
  const text = typeof value === "string" ? value.trim() : "";
  return text && text.length <= maximum ? text : "";
}

export function normalizeReport(value) {
  if (!value || typeof value !== "object") return null;
  const outcome = safeText(value.outcome, 32);
  if (!new Set(["applied", "partial", "fully_skipped", "failed"]).has(outcome))
    return null;
  const items = (Array.isArray(value.items) ? value.items : [])
    .map((item) => ({
      id: safeText(item?.id, 64),
      verb: safeText(item?.verb, 32),
      status: safeText(item?.status, 16),
      detail: safeText(item?.reason || item?.error, 2048),
    }))
    .filter(
      (item) =>
        item.id && ["applied", "skipped", "failed"].includes(item.status),
    );
  return {
    proposalId: safeText(value.proposal_id, 64),
    outcome,
    createdAt: safeText(value.created_at, 64),
    items,
    freshSurveyRecommended: value.fresh_survey_recommended === true,
    undoSummary: safeText(value.undo_summary, 256),
  };
}

export function normalizeProposal(value = {}) {
  const state = STATES.has(value.state) ? value.state : "none";
  const proposalId = safeText(value.proposal_id, 64);
  const seen = new Set();
  const items = (Array.isArray(value.items) ? value.items : [])
    .map((item) => ({
      id: safeText(item?.item_id, 64),
      line: safeText(item?.line, 2048),
      reason: safeText(item?.reason, 1024),
    }))
    .filter((item) => {
      if (!item.id || !item.line || !item.reason || seen.has(item.id))
        return false;
      seen.add(item.id);
      return true;
    });
  const open = state === "open" && Boolean(proposalId) && items.length > 0;
  return {
    id: proposalId,
    state,
    open,
    alreadyTidy: state === "already_tidy" || value.already_tidy === true,
    createdAt: safeText(value.created_at, 64),
    conventionsNote: safeText(value.conventions_note, 2048),
    items,
    selected: new Set(open ? items.map((item) => item.id) : []),
    lastReport: normalizeReport(value.last_report),
  };
}

export function toggleSelection(proposal, itemId, checked) {
  const selected = new Set(proposal?.selected || []);
  if (checked) selected.add(itemId);
  else selected.delete(itemId);
  return { ...proposal, selected };
}

export function canApply(proposal) {
  return Boolean(
    proposal?.open &&
    proposal.selected instanceof Set &&
    proposal.selected.size > 0,
  );
}

export function applyVariables(proposal) {
  if (!canApply(proposal)) return null;
  return {
    proposal_id: proposal.id,
    selected_item_ids: proposal.items
      .map((item) => item.id)
      .filter((itemId) => proposal.selected.has(itemId)),
  };
}

export function proposalMessage(proposal) {
  switch (proposal?.state) {
    case "open":
      return `${proposal.items.length} cosmetic change${proposal.items.length === 1 ? "" : "s"} ready for review.`;
    case "already_tidy":
      return "The latest survey found no supported cosmetic changes.";
    case "dismissed":
      return "The latest proposal was dismissed. REAPER was not changed.";
    case "superseded":
      return "This proposal was superseded by a newer survey.";
    case "applied":
      return "The reviewed proposal has been applied.";
    case "apply_skipped":
      return "Every selected item was stale. Run a fresh survey.";
    case "apply_failed":
      return "The latest apply reported failures. Review the report below.";
    default:
      return "Run a survey to compare the open project with your studio conventions.";
  }
}
