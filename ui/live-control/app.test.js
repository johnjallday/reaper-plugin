import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { OriWorkspaceSurfaceSDK } from "./workspace-surface-sdk.js";

class Events {
  constructor() {
    this.listeners = new Map();
  }
  addEventListener(type, listener) {
    const listeners = this.listeners.get(type) || new Set();
    listeners.add(listener);
    this.listeners.set(type, listeners);
  }
  removeEventListener(type, listener) {
    this.listeners.get(type)?.delete(listener);
  }
  emit(type, event) {
    for (const listener of this.listeners.get(type) || []) listener(event);
  }
}

function envelope(bridgeId, type, requestId, payload) {
  return {
    protocol_version: 1,
    bridge_id: bridgeId,
    type,
    request_id: requestId,
    payload,
  };
}

function cryptoFixture() {
  return {
    getRandomValues(bytes) {
      bytes.fill(7);
      return bytes;
    },
  };
}

test("author SDK handshake keeps REAPER operation calls inside the parent bridge", async () => {
  const events = new Events();
  const posted = [];
  const parent = {
    postMessage: (message, target) => posted.push({ message, target }),
  };
  const frame = Object.assign(events, { parent });
  const sdk = new OriWorkspaceSurfaceSDK({
    eventTarget: frame,
    parentWindow: parent,
    crypto: cryptoFixture(),
  }).start();
  events.emit("message", {
    source: parent,
    data: envelope("reaper-bridge", "ori.surface.challenge", "handshake", {
      challenge: "challenge",
      surface: {},
    }),
  });
  events.emit("message", {
    source: parent,
    data: envelope("reaper-bridge", "ori.surface.init", "handshake", {
      surface: {
        key: "plugin:reaper-plugin:reaper-live-control:live-control",
        label: "REAPER Live Control",
      },
      features: { operation: true },
    }),
  });
  const pending = sdk.invoke("state.read", {});
  const request = posted.at(-1).message;
  assert.equal(request.type, "ori.surface.operation.invoke");
  assert.deepEqual(request.payload, { operation_id: "state.read", input: {} });
  events.emit("message", {
    source: parent,
    data: envelope(
      "reaper-bridge",
      "ori.surface.response",
      request.request_id,
      { ok: true, result: { connected: false } },
    ),
  });
  assert.deepEqual(await pending, { connected: false });
  sdk.destroy();
});

test("isolated UI keeps accessible controls and no ambient parent/network access", async () => {
  const [html, app] = await Promise.all([
    readFile(new URL("./index.html", import.meta.url), "utf8"),
    readFile(new URL("./app.js", import.meta.url), "utf8"),
  ]);
  for (const expected of [
    "aria-live",
    "aria-labelledby",
    "<label",
    "data-setup",
    "data-close",
  ]) {
    assert.match(html, new RegExp(expected));
  }
  for (const forbidden of [
    "innerHTML",
    "parent.document",
    "window.parent.",
    "fetch(",
  ]) {
    assert.equal(app.includes(forbidden), false, forbidden);
  }
  for (const operation of [
    "state.read",
    "actions.list",
    "scripts.list",
    "draft.validate",
    "draft.run",
  ]) {
    assert.equal(app.includes(operation), true, operation);
  }
});
