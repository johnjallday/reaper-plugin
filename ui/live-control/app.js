import { createWorkspaceSurfaceSDK } from "./workspace-surface-sdk.js";

const sdk = createWorkspaceSurfaceSDK();
const nodes = {
  status: document.querySelector("[data-status]"),
  project: document.querySelector("[data-project]"),
  tempo: document.querySelector("[data-tempo]"),
  position: document.querySelector("[data-position]"),
  trackCount: document.querySelector("[data-track-count]"),
  runner: document.querySelector("[data-runner]"),
  tracks: document.querySelector("[data-tracks]"),
  actions: document.querySelector("[data-actions]"),
  actionResult: document.querySelector("[data-action-result]"),
  scripts: document.querySelector("[data-scripts]"),
  scriptResult: document.querySelector("[data-script-result]"),
  draft: document.querySelector("#draft-code"),
  draftResult: document.querySelector("[data-draft-result]"),
};
let visible = true;
let pollTimer;
let refreshing = false;
let actions = new Map();

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

function renderTracks(tracks = []) {
  if (!nodes.tracks) return;
  nodes.tracks.replaceChildren();
  if (!tracks.length) {
    empty(nodes.tracks, "No tracks in the current project.");
    return;
  }
  tracks.forEach((track) => {
    const row = document.createElement("article");
    row.className = "track";
    const number = document.createElement("span");
    number.textContent = String(track.index);
    const name = document.createElement("strong");
    name.textContent = track.name || `Track ${track.index}`;
    const state = document.createElement("small");
    state.textContent =
      [track.muted && "Muted", track.soloed && "Solo", track.armed && "Armed"]
        .filter(Boolean)
        .join(" · ") || "Ready";
    row.append(number, name, state);
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
    renderTracks(state.tracks);
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
  pollTimer = setInterval(() => void refreshState(), 1500);
}

function operationForAction(action) {
  return action.tier === "confirm" || action.needs_confirmation
    ? "actions.run_confirmed"
    : "actions.run_safe";
}

async function runAction(action) {
  text(nodes.actionResult, `Running ${action.label}…`);
  try {
    const result = await sdk.invoke(operationForAction(action), {
      action_id: action.id,
    });
    text(nodes.actionResult, result.summary || `${action.label} completed.`);
    await sdk.statusChanged();
    await refreshState();
  } catch (error) {
    text(nodes.actionResult, `${error.code || "failed"} · ${error.message}`);
  }
}

function renderActions(items = []) {
  if (!nodes.actions) return;
  nodes.actions.replaceChildren();
  actions = new Map(items.map((item) => [item.id, item]));
  if (!items.length)
    return empty(nodes.actions, "No REAPER actions are available.");
  items.forEach((action) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "action";
    button.dataset.catalogAction = action.id;
    const label = document.createElement("strong");
    label.textContent = action.label;
    const detail = document.createElement("small");
    detail.textContent = `${action.source} · ${action.tier || (action.needs_confirmation ? "confirm" : "silent")}`;
    button.append(label, detail);
    nodes.actions.append(button);
  });
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
  .querySelector("[data-ask]")
  ?.addEventListener(
    "click",
    () =>
      void sdk.askOri(
        "Help me understand the current REAPER project and safe next action.",
      ),
  );
document
  .querySelector("[data-setup]")
  ?.addEventListener("click", () => void sdk.openSetup());
document
  .querySelector("[data-close]")
  ?.addEventListener("click", () => void sdk.close());
window.addEventListener("pagehide", () => clearInterval(pollTimer));
