export function actionOperation(action = {}) {
  return action.tier === "confirm" || action.needs_confirmation
    ? "actions.run_confirmed"
    : "actions.run_safe";
}

export function meaningfulStateKey(state = {}) {
  return JSON.stringify({
    connected: Boolean(state.connected),
    reason: String(state.reason || ""),
    project: String(state.project || ""),
    tempo: Number(state.tempo || 0),
    time_signature: String(state.time_signature || ""),
    play_state: String(state.play_state || ""),
    position: String(state.position || ""),
    folder_depth_available: Boolean(state.folder_depth_available),
    track_editing_available: Boolean(state.track_editing_available),
    tracks: (state.tracks || []).map((track) => ({
      index: Number(track.index),
      name: String(track.name || ""),
      muted: Boolean(track.muted),
      soloed: Boolean(track.soloed),
      armed: Boolean(track.armed),
      color: Number(track.color || 0),
      folder_depth: Number(track.folder_depth || 0),
    })),
  });
}

export function folderRows(tracks = [], depthAvailable = false) {
  let level = 0;
  return tracks.map((track) => {
    const depth = depthAvailable ? Number(track.folder_depth || 0) : 0;
    if (depth < 0) level = Math.max(0, level + depth);
    const row = {
      ...track,
      level: Math.min(8, level),
      folderParent: depth > 0,
      moveAllowed: depthAvailable && depth <= 0,
    };
    if (depth > 0) level = Math.min(8, level + depth);
    return row;
  });
}

export function promptChips(state = {}) {
  const prompts = [];
  if (!state.connected) prompts.push("Help me restore REAPER live control.");
  if (!state.track_count) prompts.push("Help me start a useful track layout.");
  if (state.tracks?.some((track) => !track.name))
    prompts.push("Help me name the unnamed tracks.");
  if (!state.track_editing_available)
    prompts.push("Explain how to set up the trusted runner.");
  return prompts.slice(0, 4);
}

export function normalizePinnedState(stored, defaults) {
  if (!stored?.found) return [...defaults];
  return Array.isArray(stored.value?.ids) ? stored.value.ids.map(String) : [];
}

export function reorderPins(current, index, delta) {
  const target = index + delta;
  if (
    index < 0 ||
    target < 0 ||
    index >= current.length ||
    target >= current.length
  )
    return [...current];
  const next = [...current];
  [next[index], next[target]] = [next[target], next[index]];
  return next;
}

export function optimisticTrackPatch(track, edit) {
  const next = { ...track };
  switch (edit.operation) {
    case "rename":
      next.name = edit.new_name;
      break;
    case "color":
      next.color = edit.new_color;
      break;
    case "mute":
      next.muted = edit.new_bool;
      break;
    case "solo":
      next.soloed = edit.new_bool;
      break;
    case "arm":
      next.armed = edit.new_bool;
      break;
  }
  return next;
}
