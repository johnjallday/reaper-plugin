import { test } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("project tidy surface is accessible, sandbox-only, and keeps raw plans hidden", async () => {
  const [html, app] = await Promise.all([
    readFile(new URL("./index.html", import.meta.url), "utf8"),
    readFile(new URL("./app.js", import.meta.url), "utf8"),
  ]);
  for (const expected of [
    "aria-live",
    "aria-labelledby",
    "data-survey",
    "data-apply",
    "data-dismiss",
    "Apply selected",
  ]) {
    assert.match(html, new RegExp(expected));
  }
  for (const forbidden of [
    "innerHTML",
    "parent.document",
    "window.parent.",
    "fetch(",
    "OriAskRouting",
    "track_guid",
    "snapshot_position_seconds",
    "plan.json",
  ]) {
    assert.equal(app.includes(forbidden), false, forbidden);
    assert.equal(html.includes(forbidden), false, forbidden);
  }
  assert.equal(app.includes('createTask("survey"'), true);
  assert.equal(app.includes('createTask("apply"'), true);
  assert.equal(app.includes('sdk.invoke("tidy.proposal.dismiss"'), true);
});

test("proposal checklist uses safe text nodes and disables empty apply", async () => {
  const app = await readFile(new URL("./app.js", import.meta.url), "utf8");
  assert.equal(app.includes("textContent = item.line"), true);
  assert.equal(app.includes("textContent = item.reason"), true);
  assert.equal(
    app.includes("nodes.apply.disabled = !canApply(proposal)"),
    true,
  );
  assert.equal(app.includes('input.type = "checkbox"'), true);
  assert.equal(
    app.includes("input.checked = proposal.selected.has(item.id)"),
    true,
  );
});
