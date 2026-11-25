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

test("author SDK host intents post their own envelopes and offer no Ask Ori", async () => {
  const events = new Events();
  const posted = [];
  const parent = {
    postMessage: (message, target) => posted.push({ message, target }),
  };
  const sdk = new OriWorkspaceSurfaceSDK({
    eventTarget: Object.assign(events, { parent }),
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
      surface: {},
      features: { confirmation: true, state: true, open_setup: true },
    }),
  });
  assert.equal("askOri" in sdk, false);
  const calls = [
    [() => sdk.confirm("confirm-1"), "ori.surface.host.confirm"],
    [
      () => sdk.createTask("survey", { scope: "all" }),
      "ori.surface.host.create_task",
    ],
    [() => sdk.openSetup(), "ori.surface.host.open_setup"],
    [() => sdk.statusChanged(), "ori.surface.status_changed"],
    [() => sdk.close(), "ori.surface.host.close"],
  ];
  for (const [call, type] of calls) {
    void call().catch(() => {});
    assert.equal(posted.at(-1).message.type, type);
  }
  assert.deepEqual(posted.map(({ message }) => message.payload).slice(-5, -3), [
    { confirmation_id: "confirm-1" },
    { template_id: "survey", variables: { scope: "all" } },
  ]);
  sdk.destroy();
});

test("Ask Ori is retired from the page, its handlers, both SDKs and the manifest", async () => {
  const read = (path) => readFile(new URL(path, import.meta.url), "utf8");
  const [html, app, sdk, tidySDK, manifest] = await Promise.all([
    read("./index.html"),
    read("./app.js"),
    read("./workspace-surface-sdk.js"),
    read("../project-tidy/workspace-surface-sdk.js"),
    read("../../.ori-plugin/plugin.json"),
  ]);
  for (const source of [html, app, sdk]) {
    assert.doesNotMatch(source, /\bask[_ -]?ori|data-ask|prompt-chips/i);
  }
  assert.equal(tidySDK, sdk, "vendored SDK copies stay identical");
  const surfaces = JSON.parse(manifest).capabilities.flatMap(
    (capability) => capability.surfaces || [],
  );
  assert.deepEqual(
    Object.fromEntries(
      surfaces.map((surface) => [surface.id, surface.host_intents]),
    ),
    {
      "live-control": {
        open_setup: { provider_id: "reaper-runtime" },
        confirmation: true,
        state: true,
        close: true,
      },
      "project-tidy": {
        open_setup: { provider_id: "reaper-runtime" },
        confirmation: false,
        state: false,
        close: true,
      },
    },
  );
});

test("bridge transcript fixture uses only messages the SDK sends and no Ask Ori", async () => {
  const read = (path) => readFile(new URL(path, import.meta.url), "utf8");
  const [sdk, transcript] = await Promise.all([
    read("./workspace-surface-sdk.js"),
    read("../../protocol/workspace-surface-v1/valid-bridge-transcript.json"),
  ]);
  const { messages } = JSON.parse(transcript);
  const init = messages.find(
    ({ message }) => message.type === "ori.surface.init",
  );
  assert.equal(init.message.payload.features.ask_ori, false);
  for (const { from, message } of messages) {
    assert.doesNotMatch(message.type, /ask_ori/);
    if (from === "frame") {
      assert.equal(sdk.includes(`"${message.type}"`), true, message.type);
    }
  }
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
