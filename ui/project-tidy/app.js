import { createWorkspaceSurfaceSDK } from "./workspace-surface-sdk.js";
import {
  applyVariables,
  canApply,
  normalizeProposal,
  proposalMessage,
  toggleSelection,
} from "./model.js";

const sdk = createWorkspaceSurfaceSDK();
const nodes = {
  heading: document.querySelector("[data-heading]"),
  message: document.querySelector("[data-message]"),
  conventions: document.querySelector("[data-conventions]"),
  proposal: document.querySelector("[data-proposal]"),
  items: document.querySelector("[data-items]"),
  count: document.querySelector("[data-count]"),
  apply: document.querySelector("[data-apply]"),
  dismiss: document.querySelector("[data-dismiss]"),
  survey: document.querySelector("[data-survey]"),
  empty: document.querySelector("[data-empty]"),
  emptyMessage: document.querySelector("[data-empty-message]"),
  report: document.querySelector("[data-report]"),
  reportTitle: document.querySelector("[data-report-title]"),
  reportItems: document.querySelector("[data-report-items]"),
  undoNote: document.querySelector("[data-undo-note]"),
  outcome: document.querySelector("[data-outcome]"),
};
let proposal = normalizeProposal();
let visible = true;
let refreshing = false;
let pollTimer = null;

function setOutcome(message, error = false) {
  if (!nodes.outcome) return;
  nodes.outcome.textContent = String(message || "");
  nodes.outcome.classList.toggle("is-error", error);
}

function renderReport() {
  const report = proposal.lastReport;
  nodes.report.hidden = !report;
  nodes.reportItems.replaceChildren();
  nodes.undoNote.textContent = report?.undoSummary || "";
  if (!report) return;
  nodes.reportTitle.textContent = report.freshSurveyRecommended
    ? "Fresh survey needed"
    : `Tidy report · ${report.outcome.replaceAll("_", " ")}`;
  const order = { failed: 0, skipped: 1, applied: 2 };
  [...report.items]
    .sort((left, right) => order[left.status] - order[right.status])
    .forEach((item) => {
      const row = document.createElement("article");
      row.className = `report-item is-${item.status}`;
      const status = document.createElement("strong");
      status.textContent = item.status;
      const copy = document.createElement("span");
      copy.textContent = item.detail ? `${item.id} — ${item.detail}` : item.id;
      row.append(status, copy);
      nodes.reportItems.append(row);
    });
}

function render() {
  const open = proposal.open;
  nodes.proposal.hidden = !open;
  nodes.empty.hidden = open;
  nodes.heading.textContent = open
    ? "A pass is ready"
    : "Project care, on your terms";
  nodes.message.textContent = proposalMessage(proposal);
  nodes.emptyMessage.textContent = proposalMessage(proposal);
  nodes.conventions.hidden = !proposal.conventionsNote;
  nodes.conventions.textContent = proposal.conventionsNote;
  nodes.items.replaceChildren();
  if (open) {
    for (const item of proposal.items) {
      const label = document.createElement("label");
      label.className = "proposal-item";
      const input = document.createElement("input");
      input.type = "checkbox";
      input.checked = proposal.selected.has(item.id);
      input.setAttribute("aria-label", `Select ${item.line}`);
      input.addEventListener("change", () => {
        proposal = toggleSelection(proposal, item.id, input.checked);
        renderSelection();
      });
      const copy = document.createElement("span");
      const line = document.createElement("span");
      line.className = "proposal-line";
      line.textContent = item.line;
      const reason = document.createElement("small");
      reason.className = "proposal-reason";
      reason.textContent = item.reason;
      copy.append(line, reason);
      label.append(input, copy);
      nodes.items.append(label);
    }
  }
  renderSelection();
  renderReport();
}

function renderSelection() {
  const selected = proposal.selected?.size || 0;
  nodes.count.textContent = `${selected} selected`;
  nodes.apply.disabled = !canApply(proposal);
}

async function refresh() {
  if (!visible || refreshing) return;
  refreshing = true;
  try {
    proposal = normalizeProposal(await sdk.invoke("tidy.proposal.read", {}));
    render();
  } catch (error) {
    setOutcome(error.message || "Proposal unavailable.", true);
  } finally {
    refreshing = false;
  }
}

async function createTask(templateId, variables, pendingMessage) {
  setOutcome(pendingMessage);
  try {
    const result = await sdk.createTask(templateId, variables);
    setOutcome(
      result.started
        ? "Task started. This panel will refresh when it finishes."
        : "Task created.",
    );
    await sdk.statusChanged();
    await refresh();
  } catch (error) {
    setOutcome(error.message || "Task could not be started.", true);
  }
}

nodes.survey?.addEventListener(
  "click",
  () => void createTask("survey", {}, "Starting a read-only survey…"),
);
nodes.apply?.addEventListener("click", () => {
  const variables = applyVariables(proposal);
  if (variables)
    void createTask("apply", variables, "Starting the approved cosmetic pass…");
});
nodes.dismiss?.addEventListener("click", async () => {
  if (!proposal.open) return;
  setOutcome("Dismissing proposal…");
  try {
    await sdk.invoke("tidy.proposal.dismiss", { proposal_id: proposal.id });
    await sdk.statusChanged();
    await refresh();
    setOutcome("Proposal dismissed. REAPER was not changed.");
  } catch (error) {
    setOutcome(error.message || "Proposal could not be dismissed.", true);
  }
});
document
  .querySelector("[data-setup]")
  ?.addEventListener("click", () => void sdk.openSetup());
document
  .querySelector("[data-close]")
  ?.addEventListener("click", () => void sdk.close());

sdk.on("ready", () => {
  void refresh();
  clearInterval(pollTimer);
  pollTimer = setInterval(() => void refresh(), 2000);
});
sdk.on("visibility", ({ visible: next }) => {
  visible = next;
  if (visible) void refresh();
});
sdk.on("invalidated", () => clearInterval(pollTimer));
