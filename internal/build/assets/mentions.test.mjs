import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = readFileSync(new URL("./mentions.js", import.meta.url), "utf8");

class Element {
  children = [];
  attributes = {};
  setAttribute(name, value) { this.attributes[name] = value; }
  getAttribute(name) { return this.attributes[name] ?? null; }
  hasAttribute(name) { return name in this.attributes; }
  appendChild(child) { this.children.push(child); }
  replaceChildren(...children) { this.children = children; }
}

for (const live of [false, true]) {
  test(`sender URLs are validated in ${live ? "live" : "asset"} mentions`, async () => {
    const section = new Element();
    section.setAttribute("data-mentions", "/mentions.json");
    if (live) section.setAttribute("data-mentions-live", "");
    const mentions = ["like", "reply"].flatMap((type) => [
      { type, author: { name: "Unsafe", url: "javascript:alert(1)", photo: "data:text/html,bad" },
        url: "java\nscript:alert(2)", content: "Keep the reply" },
      { type, author: { name: "Safe", url: "https://example.com/author", photo: "https://example.com/photo.png" },
        url: "https://example.com/reply", content: "Safe reply" },
    ]);
    const data = live ? { children: mentions.map((m) => ({ ...m,
      "wm-property": m.type === "like" ? "like-of" : "in-reply-to",
      content: { text: m.content },
    })) } : { mentions };
    vm.runInNewContext(source, {
      URL, document: { querySelectorAll: () => [section],
        createElement: () => new Element(), createDocumentFragment: () => new Element() },
      fetch: async () => ({ ok: true, json: async () => data }),
    });
    await new Promise(setImmediate);
    const urls = [];
    function visit(node) {
      for (const property of ["href", "src"]) {
        if (node[property]) urls.push(node[property]);
      }
      node.children.forEach(visit);
    }
    visit(section);
    assert.ok(urls.includes("https://example.com/author"));
    assert.ok(urls.includes("https://example.com/photo.png"));
    assert.ok(urls.includes("https://example.com/reply"));
    assert.ok(urls.every((url) => url === "#" || url.startsWith("https://")), JSON.stringify(urls));
  });
}
