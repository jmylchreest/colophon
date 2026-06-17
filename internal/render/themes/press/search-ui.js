// search-ui.js — press theme search box. It reads the index location from [data-search-base],
// lazy-imports the engine reader (emitted at _search/search.js) on first open, and runs a
// debounced query. Progressive enhancement: it does nothing without JS, and the toggle is
// .js-gated in CSS, so a no-JS visitor never sees a dead control.
const root = document.querySelector('[data-search]');
if (root) {
  const base = root.dataset.searchBase;
  const toggle = root.querySelector('.search-toggle');
  const panel = root.querySelector('.search-panel');
  const input = root.querySelector('.search-input');
  const list = root.querySelector('.search-results');
  const empty = root.querySelector('.search-empty');

  let reader = null;
  async function ensureReader() {
    if (!reader) {
      const mod = await import(base + 'search.js');
      reader = mod.createReader({ base });
    }
    return reader;
  }

  function open() {
    panel.hidden = false;
    toggle.setAttribute('aria-expanded', 'true');
    input.focus();
    ensureReader();
  }
  function close() {
    panel.hidden = true;
    toggle.setAttribute('aria-expanded', 'false');
  }

  toggle.addEventListener('click', () => (panel.hidden ? open() : close()));
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !panel.hidden) { close(); toggle.focus(); }
  });
  document.addEventListener('click', (e) => {
    if (!root.contains(e.target) && !panel.hidden) close();
  });

  let timer = 0;
  let seq = 0;
  input.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(run, 120);
  });
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      const first = list.querySelector('a.search-hit');
      if (first) location.href = first.href;
    }
  });

  async function run() {
    const q = input.value.trim();
    const mine = ++seq;
    if (!q) { render([], ''); return; }
    const r = await ensureReader();
    const results = await r.query(q, 8);
    if (mine === seq) render(results, q); // ignore a stale response that finished out of order
  }

  function render(results, q) {
    list.textContent = '';
    empty.hidden = !(q && results.length === 0);
    for (const r of results) {
      const a = document.createElement('a');
      a.href = r.url;
      a.className = 'search-hit';
      const title = document.createElement('span');
      title.className = 'search-hit-title';
      title.textContent = r.title || r.url;
      const ex = document.createElement('span');
      ex.className = 'search-hit-excerpt';
      ex.textContent = r.excerpt || '';
      a.append(title, ex);
      const li = document.createElement('li');
      li.append(a);
      list.append(li);
    }
  }
}
