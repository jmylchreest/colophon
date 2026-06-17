// search-ui.js — press theme search box. It reads the index location from [data-search-base],
// lazy-imports the engine reader (emitted at _search/search.js) on first open, runs a debounced
// query, and supports keyboarding: "/" focuses search from anywhere, Up/Down move through results,
// Enter opens the active (or first) hit, Escape closes. Progressive enhancement: it does nothing
// without JS, and the toggle is .js-gated in CSS.
const root = document.querySelector('[data-search]');
if (root) {
  const base = root.dataset.searchBase;
  const toggle = root.querySelector('.search-toggle');
  const panel = root.querySelector('.search-panel');
  const input = root.querySelector('.search-input');
  const list = root.querySelector('.search-results');
  const empty = root.querySelector('.search-empty');

  // The input is a combobox driving the results listbox (so screen readers track the active hit).
  input.setAttribute('role', 'combobox');
  input.setAttribute('aria-autocomplete', 'list');
  input.setAttribute('aria-expanded', 'false');
  list.setAttribute('role', 'listbox');

  let reader = null;
  let hits = [];
  let active = -1;

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
    input.setAttribute('aria-expanded', 'true');
    input.focus();
    ensureReader();
  }
  function close() {
    panel.hidden = true;
    toggle.setAttribute('aria-expanded', 'false');
    input.setAttribute('aria-expanded', 'false');
  }
  function isTyping(el) {
    return el && (el.isContentEditable || el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT');
  }

  toggle.addEventListener('click', () => (panel.hidden ? open() : close()));

  document.addEventListener('keydown', (e) => {
    // "/" focuses search from anywhere — unless the user is already typing in a field.
    if (e.key === '/' && !isTyping(e.target)) {
      e.preventDefault();
      if (panel.hidden) open();
      else input.focus();
    } else if (e.key === 'Escape' && !panel.hidden) {
      close();
      toggle.focus();
    }
  });
  document.addEventListener('click', (e) => {
    if (!root.contains(e.target) && !panel.hidden) close();
  });

  function setActive(i) {
    if (!hits.length) { active = -1; return; }
    active = (i + hits.length) % hits.length; // wrap around
    hits.forEach((a, n) => a.classList.toggle('is-active', n === active));
    const el = hits[active];
    el.scrollIntoView({ block: 'nearest' });
    input.setAttribute('aria-activedescendant', el.id);
  }

  input.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setActive(active + 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setActive(active - 1);
    } else if (e.key === 'Enter') {
      const target = active >= 0 ? hits[active] : hits[0];
      if (target) location.href = target.href;
    }
  });

  let timer = 0;
  let seq = 0;
  input.addEventListener('input', () => {
    clearTimeout(timer);
    timer = setTimeout(run, 120);
  });

  async function run() {
    const q = input.value.trim();
    const mine = ++seq;
    if (!q) { render([], ''); return; }
    const r = await ensureReader();
    const results = await r.query(q, 8);
    if (mine === seq) render(results, q); // ignore a stale response that resolved out of order
  }

  function render(results, q) {
    list.textContent = '';
    hits = [];
    active = -1;
    input.removeAttribute('aria-activedescendant');
    empty.hidden = !(q && results.length === 0);
    results.forEach((r, i) => {
      const a = document.createElement('a');
      a.href = r.url;
      a.className = 'search-hit';
      a.id = 'search-hit-' + i;
      a.setAttribute('role', 'option');
      const title = document.createElement('span');
      title.className = 'search-hit-title';
      title.textContent = r.title || r.url;
      const ex = document.createElement('span');
      ex.className = 'search-hit-excerpt';
      ex.textContent = r.excerpt || '';
      a.append(title, ex);
      a.addEventListener('mouseenter', () => setActive(i)); // hover and keyboard share the highlight
      const li = document.createElement('li');
      li.setAttribute('role', 'presentation');
      li.append(a);
      list.append(li);
      hits.push(a);
    });
  }
}
