// Keep source text separately because Mermaid replaces it with SVG on first render.
// Pages and decks share one queue so rapid theme changes cannot interleave renders.
(() => {
  "use strict";
  const sources = new WeakMap();
  const rendered = new WeakMap();
  const system = matchMedia("(prefers-color-scheme: dark)");
  let pending = Promise.resolve();
  document.querySelectorAll(".mermaid").forEach((node) => sources.set(node, node.textContent));

  function render(root) {
    pending = pending.catch(() => {}).then(async () => {
      const theme = (document.documentElement.dataset.theme || (system.matches ? "dark" : "light")) === "dark" ? "dark" : "default";
      const scope = root || document.querySelector(".deck .slide.active") || document;
      const nodes = [...scope.querySelectorAll(".mermaid")].filter((node) =>
        node.getClientRects().length && rendered.get(node) !== theme);
      if (!nodes.length || !window.mermaid) return;
      mermaid.initialize({ startOnLoad: false, theme });
      for (const node of nodes) {
        if (!sources.has(node)) sources.set(node, node.textContent);
        node.textContent = sources.get(node);
        node.removeAttribute("data-processed");
      }
      try {
        await mermaid.run({ nodes });
        for (const node of nodes) {
          // A slide hidden during an asynchronous render needs measuring again on return.
          if (node.getClientRects().length) rendered.set(node, theme);
          else rendered.delete(node);
        }
      } catch (error) {
        for (const node of nodes) {
          node.textContent = sources.get(node);
          node.removeAttribute("data-processed");
          rendered.delete(node);
        }
        throw error;
      }
    });
    return pending;
  }

  function refresh() { render().catch(() => {}); }
  window.colophonMermaid = { render };
  new MutationObserver(refresh).observe(document.documentElement, {
    attributes: true, attributeFilter: ["data-theme"],
  });
  system.addEventListener("change", () => {
    if (!document.documentElement.dataset.theme) refresh();
  });
  refresh();
})();
