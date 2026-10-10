import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = readFileSync(new URL("./themes/default/mermaid-init.js", import.meta.url), "utf8");

function setup() {
  const nodes = ["graph LR; A-->B", "sequenceDiagram\nA->>B: Hello"].map((textContent) => ({
    textContent, visible: true,
    getClientRects() { return this.visible ? [{}] : []; },
    removeAttribute() {},
  }));
  const root = { dataset: { theme: "light" } };
  let notify, systemChanged, theme, running = 0, maxRunning = 0, fail = false;
  const calls = [];
  const system = { matches: false, addEventListener(_, fn) { systemChanged = fn; } };
  const mermaid = {
    initialize(options) { theme = options.theme; },
    async run({ nodes }) {
      running++;
      maxRunning = Math.max(running, maxRunning);
      calls.push({ theme, sources: nodes.map((n) => n.textContent) });
      await new Promise(setImmediate);
      running--;
      if (fail) throw new Error("Invalid diagram");
      nodes.forEach((n) => { n.textContent = "rendered SVG " + theme; });
    },
  };
  const window = { mermaid };
  const context = {
    window, mermaid, matchMedia: () => system,
    document: { documentElement: root, querySelectorAll: () => nodes, querySelector: () => null },
    MutationObserver: class { constructor(fn) { notify = fn; } observe() {} },
  };
  vm.runInNewContext(source, context);
  return { nodes, root, calls, system, render: () => window.colophonMermaid.render(),
    toggle(value) { root.dataset.theme = value; notify(); },
    systemChange(value) { system.matches = value; systemChanged(); },
    fail(value) { fail = value; }, maxRunning: () => maxRunning };
}

test("theme changes preserve diagram sources and serialize rapid renders", async () => {
  const s = setup();
  await s.render();
  s.toggle("dark");
  await s.render();
  assert.equal(s.calls.at(-1).theme, "dark");
  assert.equal(s.calls.at(-1).sources[0], "graph LR; A-->B");
  s.toggle("light");
  s.toggle("dark");
  s.toggle("light");
  await s.render();
  assert.equal(s.calls.at(-1).theme, "default");
  assert.equal(s.maxRunning(), 1);
  assert.equal(s.calls.at(-1).sources[1], "sequenceDiagram\nA->>B: Hello");
});

test("hidden slides render only when visible and use the current theme", async () => {
  const s = setup();
  s.nodes[1].visible = false;
  await s.render();
  assert.equal(s.calls[0].sources.length, 1);
  s.toggle("dark");
  await s.render();
  s.nodes[1].visible = true;
  await s.render();
  assert.equal(s.calls.at(-1).theme, "dark");
  assert.equal(s.calls.at(-1).sources.length, 1);
  assert.equal(s.calls.at(-1).sources[0], "sequenceDiagram\nA->>B: Hello");
});

test("failed rendering keeps readable source and does not poison the queue", async () => {
  const s = setup();
  await s.render();
  s.fail(true);
  s.toggle("dark");
  await assert.rejects(s.render(), /Invalid diagram/);
  assert.equal(s.nodes[0].textContent, "graph LR; A-->B");
  s.fail(false);
  await s.render();
  assert.equal(s.nodes[0].textContent, "rendered SVG dark");
});

test("system theme changes apply when no explicit theme is set", async () => {
  const s = setup();
  delete s.root.dataset.theme;
  await s.render();
  s.systemChange(true);
  await s.render();
  assert.equal(s.calls.at(-1).theme, "dark");
});
