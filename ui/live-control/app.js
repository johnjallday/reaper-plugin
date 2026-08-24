import { createWorkspaceSurfaceSDK } from "./workspace-surface-sdk.js";
import {
  actionOperation,
  folderRows,
  meaningfulStateKey,
  normalizePinnedState,
  planEditLabel,
  promptChips as derivePromptChips,
  reorderPins,
} from "./model.js";

const sdk = createWorkspaceSurfaceSDK();
const nodes = {
  status: document.querySelector("[data-status]"),
  promptChips: document.querySelector("[data-prompt-chips]"),
  askInput: document.querySelector("[data-ask-input]"),
  project: document.querySelector("[data-project]"),
  tempo: document.querySelector("[data-tempo]"),
  position: document.querySelector("[data-position]"),
  trackCount: document.querySelector("[data-track-count]"),
  runner: document.querySelector("[data-runner]"),
  tracks: document.querySelector("[data-tracks]"),
  actions: document.querySelector("[data-actions]"),
  actionResult: document.querySelector("[data-action-result]"),
  pinned: document.querySelector("[data-pinned-actions]"),
  scripts: document.querySelector("[data-scripts]"),
  scriptResult: document.querySelector("[data-script-result]"),
  draft: document.querySelector("#draft-code"),
  draftResult: document.querySelector("[data-draft-result]"),
  proposalPanel: document.querySelector("[data-proposal-panel]"),
  proposalName: document.querySelector("[data-proposal-name]"),
  proposalDescription: document.querySelector("[data-proposal-description]"),
  proposalCode: document.querySelector("[data-proposal-code]"),
  proposalResult: document.querySelector("[data-proposal-result]"),
  planPanel: document.querySelector("[data-plan-panel]"),
  planEdits: document.querySelector("[data-plan-edits]"),
  planResult: document.querySelector("[data-plan-result]"),
  toasts: document.querySelector("[data-toasts]"),
};
let visible = true;
let pollTimer;
let refreshing = false;
let actions = new Map();
let pinnedIDs = [];
let pinRevision = "0";
const defaultPins = ["40026", "40001", "40364"];
let currentPlan = null;
let currentProposal = null;
let lastStateKey = "";
let draggingTrack = false;
let planKey = "";
let proposalKey = "";
let draggingPinID = "";

function text(node, value) {
  if (node) node.textContent = String(value ?? "");
}

function empty(container, message) {
  container?.replaceChildren(
    Object.assign(document.createElement("p"), {
      className: "empty",
      textContent: message,
    }),
  );
}

function renderPromptChips(state) {
  if (!nodes.promptChips) return;
  nodes.promptChips.replaceChildren();
  derivePromptChips(state).forEach((prompt) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "quiet";
    button.textContent = prompt;
    button.addEventListener("click", () => {
      if (!nodes.askInput) return;
      nodes.askInput.value = prompt;
      nodes.askInput.focus();
    });
    nodes.promptChips.append(button);
  });
}

function showOutcomeToast(action) {
  if (!nodes.toasts || ["40029", "40030"].includes(action.id)) return;
  const toast = document.createElement("article");
  toast.className = "outcome-toast";
  const message = document.createElement("span");
  message.textContent = action.undo_summary || action.label;
  toast.append(message);
  if (action.tier === "undoable") {
    const undo = document.createElement("button");
    undo.type = "button";
    undo.className = "quiet";
    undo.textContent = "Undo";
    undo.addEventListener(
      "click",
      () =>
        void runAction(
          actions.get("40029") || {
            id: "40029",
            label: "Undo",
            tier: "undoable",
          },
        ),
    );
    toast.append(undo);
  }
  nodes.toasts.prepend(toast);
  while (nodes.toasts.children.length > 3)
    nodes.toasts.lastElementChild.remove();
}

function trackEditButton(label, operation, value, pressed = false) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "track-control quiet";
  button.textContent = label;
  button.dataset.trackOperation = operation;
  button.dataset.trackValue = String(value ?? "");
  if (operation === "mute" || operation === "solo" || operation === "arm") {
    button.setAttribute("aria-pressed", String(pressed));
  }
  return button;
}

function trackColorSelect(track) {
  const select = document.createElement("select");
  select.className = "track-color-select";
  select.dataset.trackOperation = "color";
  select.setAttribute(
    "aria-label",
    `Color ${track.name || `track ${track.index}`}`,
  );
  for (const [label, color] of [
    ["No color", 0],
    ["Blue", 0x01f08a44],
    ["Green", 0x0144cc88],
    ["Gold", 0x0144ccff],
    ["Rose", 0x01cc66aa],
  ]) {
    const option = document.createElement("option");
    option.value = String(color);
    option.textContent = label;
    option.selected = track.color === color;
    select.append(option);
  }
  return select;
}

function renderTracks(tracks = [], depthAvailable = false) {
  if (!nodes.tracks || draggingTrack) return;
  nodes.tracks.replaceChildren();
  if (!tracks.length) {
    empty(nodes.tracks, "No tracks in the current project.");
    return;
  }
  folderRows(tracks, depthAvailable).forEach((track) => {
    const row = document.createElement("article");
    row.className = "track";
    row.dataset.trackIndex = String(track.index);
    row.dataset.trackName = track.name || "";
    row.dataset.moveAllowed = String(track.moveAllowed);
    row.draggable = track.moveAllowed;
    row.style.setProperty("--folder-depth", String(track.level));
    row.setAttribute(
      "aria-label",
      `${track.name || `Track ${track.index}`}, folder depth ${track.level}`,
    );
    const number = document.createElement("span");
    number.textContent = String(track.index);
    const identity = document.createElement("div");
    identity.className = "track-identity";
    const name = document.createElement("strong");
    name.textContent = track.name || `Track ${track.index}`;
    const state = document.createElement("small");
    state.textContent = !depthAvailable
      ? "Folder depth unavailable · moving disabled"
      : track.folderParent
        ? `Folder parent · depth ${track.level}`
        : track.folder_depth < 0
          ? `Folder close · depth ${track.level}`
          : `Track · depth ${track.level}`;
    identity.append(name, state);
    const controls = document.createElement("div");
    controls.className = "track-controls";
    controls.append(
      trackEditButton("Rename", "rename", ""),
      trackColorSelect(track),
      trackEditButton("M", "mute", !track.muted, track.muted),
      trackEditButton("S", "solo", !track.soloed, track.soloed),
      trackEditButton("R", "arm", !track.armed, track.armed),
      trackEditButton("↑", "move", Math.max(1, track.index - 1)),
      trackEditButton("↓", "move", Math.min(tracks.length, track.index + 1)),
    );
    if (!track.moveAllowed) {
      controls
        .querySelectorAll('[data-track-operation="move"]')
        .forEach((button) => {
          button.disabled = true;
          button.title = track.folderParent
            ? "Folder parents move as a group and are read-only in this version."
            : "Folder depth is unavailable, so moving is disabled.";
        });
    }
    row.append(number, identity, controls);
    nodes.tracks.append(row);
  });
}

async function refreshState() {
  if (!visible || refreshing) return;
  refreshing = true;
  try {
    const state = await sdk.invoke("state.read", {});
    text(
      nodes.status,
      state.connected
        ? state.play_state || "Connected"
        : state.reason || "Offline",
    );
    text(nodes.project, state.project || "No verified project");
    text(
      nodes.tempo,
      state.tempo ? `${state.tempo} BPM · ${state.time_signature || "—"}` : "—",
    );
    text(nodes.position, state.position || "—");
    text(nodes.trackCount, state.track_count ?? 0);
    text(nodes.runner, state.track_editing_available ? "Ready" : "Read only");
    const stateKey = meaningfulStateKey(state);
    if (stateKey !== lastStateKey) {
      lastStateKey = stateKey;
      renderTracks(state.tracks, state.folder_depth_available);
      renderPromptChips(state);
    }
    await loadPlan();
    await loadProposal();
  } catch (error) {
    text(nodes.status, error.code || "Unavailable");
  } finally {
    refreshing = false;
  }
}

function schedulePoll() {
  clearInterval(pollTimer);
  if (!visible) return;
  void refreshState();
  pollTimer = setInterval(() => void refreshState(), 1000);
}

async function runAction(action) {
  text(nodes.actionResult, `Running ${action.label}…`);
  try {
    const result = await sdk.invoke(actionOperation(action), {
      action_id: action.id,
    });
    text(nodes.actionResult, result.summary || `${action.label} completed.`);
    showOutcomeToast(action);
    await sdk.statusChanged();
    await refreshState();
  } catch (error) {
    text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
  }
}

function renderProposal(result) {
  currentProposal = result?.proposal || null;
  if (!nodes.proposalPanel) return;
  nodes.proposalPanel.hidden = !currentProposal;
  const nextKey = JSON.stringify(currentProposal);
  if (!currentProposal) {
    proposalKey = "";
    return;
  }
  if (nextKey === proposalKey) return;
  proposalKey = nextKey;
  text(nodes.proposalName, currentProposal.name);
  text(
    nodes.proposalDescription,
    currentProposal.description || currentProposal.filename,
  );
  text(nodes.proposalCode, currentProposal.code);
  text(nodes.proposalResult, currentProposal.test_summary || result.summary);
  const save = document.querySelector("[data-proposal-save]");
  if (save) save.disabled = !currentProposal.tested;
}

async function loadProposal() {
  try {
    renderProposal(await sdk.invoke("proposals.current", {}));
  } catch (_error) {
    if (nodes.proposalPanel) nodes.proposalPanel.hidden = true;
  }
}

function renderPlan(result) {
  currentPlan = result?.plan || null;
  if (!nodes.planPanel) return;
  nodes.planPanel.hidden = !currentPlan;
  const nextKey = JSON.stringify(currentPlan);
  if (!currentPlan) {
    planKey = "";
    return;
  }
  if (nextKey === planKey) return;
  planKey = nextKey;
  nodes.planEdits?.replaceChildren();
  currentPlan.edits.forEach((edit) => {
    const row = document.createElement("p");
    row.textContent = planEditLabel(edit);
    nodes.planEdits.append(row);
  });
  text(nodes.planResult, result.summary);
}

async function loadPlan() {
  try {
    renderPlan(await sdk.invoke("plans.current", {}));
  } catch (error) {
    if (nodes.planPanel) nodes.planPanel.hidden = true;
  }
}

function renderPinnedActions() {
  if (!nodes.pinned) return;
  nodes.pinned.replaceChildren();
  const available = pinnedIDs.map((id) => actions.get(id)).filter(Boolean);
  if (!available.length) {
    empty(
      nodes.pinned,
      "No actions pinned. This explicit empty choice survives reloads.",
    );
    return;
  }
  available.forEach((action, index) => {
    const item = document.createElement("article");
    item.className = "pinned-action";
    item.draggable = true;
    item.dataset.pinnedAction = action.id;
    const run = document.createElement("button");
    run.type = "button";
    run.textContent = action.label;
    run.addEventListener("click", () => void runAction(action));
    const controls = document.createElement("div");
    controls.className = "pin-controls";
    for (const [label, delta] of [
      ["Move left", -1],
      ["Move right", 1],
    ]) {
      const move = document.createElement("button");
      move.type = "button";
      move.className = "quiet";
      move.textContent = label;
      move.disabled = index + delta < 0 || index + delta >= pinnedIDs.length;
      move.addEventListener("click", () => {
        void savePins(reorderPins(pinnedIDs, index, delta));
      });
      controls.append(move);
    }
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "quiet";
    remove.textContent = "Unpin";
    remove.addEventListener(
      "click",
      () => void savePins(pinnedIDs.filter((id) => id !== action.id)),
    );
    controls.append(remove);
    item.append(run, controls);
    nodes.pinned.append(item);
  });
}

nodes.pinned?.addEventListener("dragstart", (event) => {
  const item = event.target.closest("[data-pinned-action]");
  if (!item) return;
  draggingPinID = item.dataset.pinnedAction;
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData("text/plain", draggingPinID);
});
nodes.pinned?.addEventListener("dragover", (event) => {
  if (event.target.closest("[data-pinned-action]")) event.preventDefault();
});
nodes.pinned?.addEventListener("drop", (event) => {
  event.preventDefault();
  const target = event.target.closest("[data-pinned-action]")?.dataset.pinnedAction;
  const source = event.dataTransfer.getData("text/plain") || draggingPinID;
  draggingPinID = "";
  const from = pinnedIDs.indexOf(source);
  const to = pinnedIDs.indexOf(target);
  if (from < 0 || to < 0 || from === to) return;
  const next = [...pinnedIDs];
  next.splice(to, 0, next.splice(from, 1)[0]);
  void savePins(next);
});
nodes.pinned?.addEventListener("dragend", () => {
  draggingPinID = "";
});

async function loadPins() {
  try {
    const stored = await sdk.getState("pinned-actions");
    pinRevision = stored.revision || "0";
    pinnedIDs = normalizePinnedState(stored, defaultPins);
    if (!stored.found) {
      const saved = await sdk.setState(
        "pinned-actions",
        { ids: pinnedIDs },
        { schemaVersion: 1, expectedRevision: "0" },
      );
      pinRevision = saved.revision || pinRevision;
    }
    renderPinnedActions();
  } catch (error) {
    empty(nodes.pinned, error.message || "Pinned actions unavailable.");
  }
}

async function savePins(next) {
  try {
    const saved = await sdk.setState(
      "pinned-actions",
      { ids: next },
      { schemaVersion: 1, expectedRevision: pinRevision },
    );
    pinRevision = saved.revision || pinRevision;
    pinnedIDs = [...next];
    renderPinnedActions();
  } catch (error) {
    text(
      nodes.actionResult,
      `${error.code || "state_conflict"} · ${error.message}`,
    );
    await loadPins();
  }
}

function renderActions(items = []) {
  if (!nodes.actions) return;
  nodes.actions.replaceChildren();
  actions = new Map(items.map((item) => [item.id, item]));
  if (!items.length)
    return empty(nodes.actions, "No REAPER actions are available.");
  items.forEach((action) => {
    const card = document.createElement("article");
    card.className = "action-card";
    const button = document.createElement("button");
    button.type = "button";
    button.className = "action";
    button.dataset.catalogAction = action.id;
    const label = document.createElement("strong");
    label.textContent = action.label;
    const detail = document.createElement("small");
    detail.textContent = `${action.source} · ${action.tier || (action.needs_confirmation ? "confirm" : "silent")}`;
    button.append(label, detail);
    const pin = document.createElement("button");
    pin.type = "button";
    pin.className = "quiet pin-toggle";
    pin.textContent = pinnedIDs.includes(action.id) ? "Unpin" : "Pin";
    pin.addEventListener("click", () => {
      const next = pinnedIDs.includes(action.id)
        ? pinnedIDs.filter((id) => id !== action.id)
        : [...pinnedIDs, action.id];
      void savePins(next);
    });
    card.append(button, pin);
    nodes.actions.append(card);
  });
  renderPinnedActions();
}

async function loadActions() {
  try {
    const result = await sdk.invoke("actions.list", {});
    renderActions(result.actions);
  } catch (error) {
    empty(nodes.actions, error.message || "Action catalog unavailable.");
  }
}

function renderScripts(items = []) {
  if (!nodes.scripts) return;
  nodes.scripts.replaceChildren();
  if (!items.length)
    return empty(nodes.scripts, "No shared custom scripts yet.");
  items.forEach((script) => {
    const row = document.createElement("article");
    row.className = "script-row";
    const copy = document.createElement("div");
    const name = document.createElement("strong");
    name.textContent = script.name;
    const description = document.createElement("small");
    description.textContent = script.description || script.filename;
    copy.append(name, description);
    const controls = document.createElement("div");
    controls.className = "script-controls";
    const run = document.createElement("button");
    run.type = "button";
    run.textContent = "Run";
    run.addEventListener(
      "click",
      () =>
        void runAction({
          id: script.id,
          label: script.name,
          source: "custom",
          tier: script.needs_confirmation ? "confirm" : "silent",
          needs_confirmation: script.needs_confirmation,
        }),
    );
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "quiet";
    remove.textContent = "Delete";
    remove.addEventListener("click", async () => {
      text(nodes.scriptResult, "Waiting for host review…");
      try {
        await sdk.invoke("scripts.delete", { id: script.id });
        text(nodes.scriptResult, `${script.name} deleted.`);
        await loadScripts();
        await loadActions();
      } catch (error) {
        text(
          nodes.scriptResult,
          `${error.code || "failed"} · ${error.message}`,
        );
      }
    });
    controls.append(run, remove);
    row.append(copy, controls);
    nodes.scripts.append(row);
  });
}

async function loadScripts() {
  try {
    const result = await sdk.invoke("scripts.list", {});
    renderScripts(result.scripts);
  } catch (error) {
    empty(nodes.scripts, error.message || "Script library unavailable.");
  }
}

sdk.on("ready", () => {
  schedulePoll();
  void loadPins();
  void loadActions();
  void loadScripts();
});
sdk.on("visibility", (event) => {
  visible = Boolean(event.visible);
  schedulePoll();
});
sdk.on("invalidated", () => {
  clearInterval(pollTimer);
  text(nodes.status, "Session ended");
});

document
  .querySelector("[data-refresh]")
  ?.addEventListener("click", () => void refreshState());
nodes.tracks?.addEventListener("dragstart", (event) => {
  const row = event.target.closest("[data-track-index]");
  if (!row || row.dataset.moveAllowed !== "true") {
    event.preventDefault();
    return;
  }
  draggingTrack = true;
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData(
    "text/plain",
    JSON.stringify({
      index: Number(row.dataset.trackIndex),
      name: row.dataset.trackName || "",
    }),
  );
});
nodes.tracks?.addEventListener("dragover", (event) => {
  if (event.target.closest("[data-track-index]")) event.preventDefault();
});
nodes.tracks?.addEventListener("drop", async (event) => {
  event.preventDefault();
  const target = event.target.closest("[data-track-index]");
  try {
    const source = JSON.parse(event.dataTransfer.getData("text/plain"));
    const newIndex = Number(target?.dataset.trackIndex);
    if (!target || !source.index || source.index === newIndex) return;
    text(nodes.actionResult, "Moving track…");
    const result = await sdk.invoke("tracks.edit", {
      edit: {
        operation: "move",
        index: source.index,
        expected_name: source.name,
        new_index: newIndex,
      },
    });
    text(nodes.actionResult, result.summary || "Track moved.");
  } catch (error) {
    text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
  } finally {
    draggingTrack = false;
    lastStateKey = "";
    await refreshState();
  }
});
nodes.tracks?.addEventListener("dragend", () => {
  draggingTrack = false;
  void refreshState();
});
nodes.tracks?.addEventListener("change", async (event) => {
  const select = event.target.closest('select[data-track-operation="color"]');
  const row = select?.closest("[data-track-index]");
  if (!select || !row) return;
  text(nodes.actionResult, "Applying color…");
  try {
    const result = await sdk.invoke("tracks.edit", {
      edit: {
        operation: "color",
        index: Number(row.dataset.trackIndex),
        expected_name: row.dataset.trackName || "",
        new_color: Number(select.value),
      },
    });
    text(nodes.actionResult, result.summary || "Track color applied.");
    lastStateKey = "";
    renderTracks(
      result.state?.tracks || [],
      Boolean(result.state?.folder_depth_available),
    );
  } catch (error) {
    text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
    lastStateKey = "";
    await refreshState();
  }
});
document
  .querySelector("[data-tracks]")
  ?.addEventListener("click", async (event) => {
    const button = event.target.closest("[data-track-operation]");
    const row = button?.closest("[data-track-index]");
    if (!button || button.tagName !== "BUTTON" || !row || button.disabled)
      return;
    const operation = button.dataset.trackOperation;
    const edit = {
      operation,
      index: Number(row.dataset.trackIndex),
      expected_name: row.dataset.trackName || "",
    };
    if (operation === "rename") {
      const next = window.prompt("New track name", row.dataset.trackName || "");
      if (!next?.trim()) return;
      edit.new_name = next.trim();
      row.querySelector("strong").textContent = edit.new_name;
    } else if (operation === "color") {
      edit.new_color = Number(button.dataset.trackValue);
    } else if (operation === "move") {
      edit.new_index = Number(button.dataset.trackValue);
    } else {
      edit.new_bool = button.dataset.trackValue === "true";
      button.setAttribute("aria-pressed", String(edit.new_bool));
    }
    text(nodes.actionResult, `Applying ${operation}…`);
    try {
      const result = await sdk.invoke("tracks.edit", { edit });
      text(nodes.actionResult, result.summary || "Track edit applied.");
      lastStateKey = "";
      renderTracks(
        result.state?.tracks || [],
        Boolean(result.state?.folder_depth_available),
      );
      await sdk.statusChanged();
    } catch (error) {
      text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
      await refreshState();
    }
  });
document
  .querySelector("[data-track-undo]")
  ?.addEventListener("click", async () => {
    text(nodes.actionResult, "Undoing latest track edit…");
    try {
      const result = await sdk.invoke("tracks.undo", {});
      text(nodes.actionResult, result.summary);
      lastStateKey = "";
      renderTracks(
        result.state?.tracks || [],
        Boolean(result.state?.folder_depth_available),
      );
    } catch (error) {
      text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
    }
  });
document.querySelector("[data-actions]")?.addEventListener("click", (event) => {
  const button = event.target.closest("[data-catalog-action]");
  const action = actions.get(button?.dataset.catalogAction);
  if (action) void runAction(action);
});
document.querySelectorAll("[data-action]").forEach((button) => {
  button.addEventListener("click", () => {
    const action = actions.get(button.dataset.action) || {
      id: button.dataset.action,
      label: button.textContent.trim(),
      source: "builtin",
      tier: button.classList.contains("danger") ? "confirm" : "silent",
    };
    void runAction(action);
  });
});
document
  .querySelector("[data-raw-form]")
  ?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const command = String(
      new FormData(event.currentTarget).get("command") || "",
    ).trim();
    text(nodes.actionResult, "Waiting for host review…");
    try {
      const result = await sdk.invoke("actions.run_raw_confirmed", {
        action_id: command,
      });
      text(nodes.actionResult, result.summary || "Raw command completed.");
      await refreshState();
    } catch (error) {
      text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
    }
  });
document
  .querySelector("[data-proposal-test]")
  ?.addEventListener("click", async () => {
    if (!currentProposal) return;
    text(nodes.proposalResult, "Waiting for host review…");
    try {
      renderProposal(
        await sdk.invoke("proposals.test", { proposal_id: currentProposal.id }),
      );
    } catch (error) {
      text(
        nodes.proposalResult,
        `${error.code || "failed"} · ${error.message}`,
      );
    }
  });
document
  .querySelector("[data-proposal-save]")
  ?.addEventListener("click", async () => {
    if (!currentProposal?.tested) return;
    text(nodes.proposalResult, "Saving makes this script global…");
    try {
      const result = await sdk.invoke("proposals.save", {
        proposal_id: currentProposal.id,
      });
      text(nodes.proposalResult, result.summary);
      renderProposal(result);
      await loadScripts();
      await loadActions();
    } catch (error) {
      text(
        nodes.proposalResult,
        `${error.code || "failed"} · ${error.message}`,
      );
    }
  });
document
  .querySelector("[data-proposal-discard]")
  ?.addEventListener("click", async () => {
    if (!currentProposal) return;
    try {
      const result = await sdk.invoke("proposals.discard", {
        proposal_id: currentProposal.id,
      });
      text(nodes.proposalResult, result.summary);
      renderProposal(result);
    } catch (error) {
      text(
        nodes.proposalResult,
        `${error.code || "failed"} · ${error.message}`,
      );
    }
  });
document
  .querySelector("[data-plan-apply]")
  ?.addEventListener("click", async () => {
    if (!currentPlan) return;
    text(nodes.planResult, "Waiting for host review…");
    try {
      const result = await sdk.invoke("plans.apply", {
        plan_id: currentPlan.id,
      });
      text(nodes.planResult, result.summary);
      renderPlan(result);
      await refreshState();
    } catch (error) {
      text(nodes.planResult, `${error.code || "failed"} · ${error.message}`);
    }
  });
document
  .querySelector("[data-plan-cancel]")
  ?.addEventListener("click", async () => {
    if (!currentPlan) return;
    try {
      const result = await sdk.invoke("plans.cancel", {
        plan_id: currentPlan.id,
      });
      text(nodes.planResult, result.summary);
      renderPlan(result);
    } catch (error) {
      text(nodes.planResult, `${error.code || "failed"} · ${error.message}`);
    }
  });
document
  .querySelector("[data-script-form]")
  ?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const values = new FormData(form);
    text(nodes.scriptResult, "Waiting for host review…");
    try {
      const script = await sdk.invoke("scripts.create", {
        filename: String(values.get("filename") || "").trim(),
        name: String(values.get("name") || "").trim(),
        description: "Created from the isolated REAPER Live Control surface.",
        needs_confirmation: true,
        code: String(values.get("code") || ""),
      });
      text(nodes.scriptResult, `${script.name} saved to the global library.`);
      form.reset();
      await loadScripts();
      await loadActions();
    } catch (error) {
      text(nodes.scriptResult, `${error.code || "failed"} · ${error.message}`);
    }
  });
document
  .querySelector("[data-validate]")
  ?.addEventListener("click", async () => {
    try {
      const result = await sdk.invoke("draft.validate", {
        code: nodes.draft?.value || "",
      });
      text(nodes.draftResult, result.summary);
    } catch (error) {
      text(nodes.draftResult, error.message);
    }
  });
document
  .querySelector("[data-run-draft]")
  ?.addEventListener("click", async () => {
    text(nodes.draftResult, "Waiting for host review…");
    try {
      const result = await sdk.invoke("draft.run", {
        code: nodes.draft?.value || "",
      });
      text(nodes.draftResult, result.summary);
      await refreshState();
    } catch (error) {
      text(nodes.draftResult, `${error.code || "failed"} · ${error.message}`);
    }
  });
document
  .querySelector("[data-ask-form]")
  ?.addEventListener("submit", (event) => {
    event.preventDefault();
    const context = String(
      new FormData(event.currentTarget).get("context") || "",
    ).trim();
    if (!context) return;
    void sdk.askOri(context.slice(0, 1000));
  });
document.querySelector("[data-ask]")?.addEventListener("click", () => {
  if (!nodes.askInput) return;
  if (!nodes.askInput.value)
    nodes.askInput.value =
      "Help me understand the current REAPER project and safe next action.";
  nodes.askInput.focus();
});
document
  .querySelector("[data-setup]")
  ?.addEventListener("click", () => void sdk.openSetup());
document
  .querySelector("[data-close]")
  ?.addEventListener("click", () => void sdk.close());
window.addEventListener("keydown", (event) => {
  if (event.key !== "Escape" || !draggingPinID) return;
  draggingPinID = "";
  renderPinnedActions();
});
window.addEventListener("pagehide", () => clearInterval(pollTimer));
