// press theme behaviours. Loaded with `defer`, so the DOM is ready when this runs. Every
// initialiser is guarded by element presence, so the same file serves the home, archive
// and post templates — each simply omits the widgets it doesn't use.

(function () {
  'use strict';

  // Dark/light toggle. The pre-paint inline script in <head> sets the initial theme to
  // avoid a flash; this wires the switch and persists the choice.
  function initTheme() {
    var tt = document.getElementById('themeToggle'), root = document.documentElement;
    var sys = window.matchMedia ? matchMedia('(prefers-color-scheme: light)') : null;
    function saved() { try { return localStorage.getItem('press-theme'); } catch (e) { return null; } }
    function apply(t) { root.dataset.theme = t; if (tt) tt.setAttribute('aria-checked', t === 'dark'); }
    // default to the system theme; a saved choice (set by the toggle) overrides it.
    apply(root.dataset.theme || saved() || (sys && sys.matches ? 'light' : 'dark'));
    if (tt) tt.addEventListener('click', function () {
      var t = root.dataset.theme === 'dark' ? 'light' : 'dark';
      apply(t);
      try { localStorage.setItem('press-theme', t); } catch (e) {}
    });
    // follow OS changes while the visitor hasn't made an explicit choice
    if (sys) sys.addEventListener('change', function (e) { if (!saved()) apply(e.matches ? 'light' : 'dark'); });
  }

  // Reading-progress bar at the top of a post.
  function initProgress() {
    var bar = document.getElementById('progress');
    if (!bar) return;
    window.addEventListener('scroll', function () {
      var d = document.documentElement;
      bar.style.width = (d.scrollTop / (d.scrollHeight - d.clientHeight) * 100) + '%';
    }, { passive: true });
  }

  // Author avatar strip: a horizontal track that reveals chevrons only when the avatars
  // overflow their box. Sorted most-recent-first by the build.
  function initAuthors() {
    var nav = document.querySelector('.authors');
    if (!nav) return;
    var track = nav.querySelector('.auth-track');
    var prev = nav.querySelector('.auth-chev.prev');
    var next = nav.querySelector('.auth-chev.next');
    if (!track) return;
    var MIN = 5; // only offer the carousel past this many avatars (and only if they overflow)
    function update() {
      var show = track.children.length > MIN && track.scrollWidth > track.clientWidth + 1;
      if (prev) prev.hidden = !show;
      if (next) next.hidden = !show;
      if (show) {
        // dim a chevron when that edge is exhausted, full when there's more to reveal
        if (prev) prev.disabled = track.scrollLeft <= 0;
        if (next) next.disabled = track.scrollLeft + track.clientWidth >= track.scrollWidth - 1;
      }
    }
    function page(dir) { track.scrollBy({ left: dir * track.clientWidth * 0.8, behavior: 'smooth' }); }
    if (prev) prev.addEventListener('click', function () { page(-1); });
    if (next) next.addEventListener('click', function () { page(1); });
    track.addEventListener('scroll', update, { passive: true });
    window.addEventListener('resize', update);
    update();
  }

  // Shared caption tooltip for [data-tip] elements (feed icons, author avatars). A single
  // position:fixed node placed under the hovered icon — fixed positioning escapes the
  // drawer/carousel overflow clipping that hid the old in-flow tooltips.
  function initTips() {
    var els = document.querySelectorAll('[data-tip]');
    if (!els.length) return;
    var tip = document.createElement('div');
    tip.className = 'cap-tip';
    document.body.appendChild(tip);
    function show(el) {
      var text = el.getAttribute('data-tip');
      if (!text) return;
      tip.textContent = text;
      var r = el.getBoundingClientRect();
      tip.style.left = (r.left + r.width / 2) + 'px';
      tip.style.top = (r.bottom + 8) + 'px';
      tip.classList.add('show');
    }
    function hide() { tip.classList.remove('show'); }
    els.forEach(function (el) {
      el.addEventListener('mouseenter', function () { show(el); });
      el.addEventListener('mouseleave', hide);
      el.addEventListener('focus', function () { show(el); });
      el.addEventListener('blur', hide);
    });
    window.addEventListener('scroll', hide, { passive: true });
  }

  // Feed drawer: a chevron that slides the RSS/Atom/JSON icons out horizontally.
  function initFeeds() {
    var wrap = document.querySelector('.feeds');
    var btn = document.getElementById('feedsToggle');
    if (!wrap || !btn) return;
    btn.addEventListener('click', function (e) {
      e.stopPropagation();
      var open = wrap.getAttribute('data-open') === 'true';
      wrap.setAttribute('data-open', open ? 'false' : 'true');
      btn.setAttribute('aria-expanded', open ? 'false' : 'true');
    });
  }

  // Pop-out nav menu (static pages) behind the animated hamburger button.
  function initMenu() {
    var btn = document.getElementById('menuToggle');
    if (!btn) return;
    var panel = document.getElementById('menuPanel');
    function isOpen() { return btn.getAttribute('aria-expanded') === 'true'; }
    function set(open) { btn.setAttribute('aria-expanded', open ? 'true' : 'false'); }
    btn.addEventListener('click', function (e) { e.stopPropagation(); set(!isOpen()); });
    document.addEventListener('click', function (e) {
      if (isOpen() && panel && !panel.contains(e.target) && !btn.contains(e.target)) set(false);
    });
    document.addEventListener('keydown', function (e) { if (e.key === 'Escape') set(false); });
  }

  // Infinite scroll for the post list: render a first batch, then reveal further batches as
  // a sentinel near the bottom scrolls into view. Each row fades in as it enters. The whole
  // effect is gated on the `js` class (set pre-paint), so without JS every row is visible.
  function initPostList() {
    var list = document.querySelector('.post-list[data-reveal]');
    if (!list || !('IntersectionObserver' in window)) return;
    var step = parseInt(list.getAttribute('data-reveal'), 10) || 8;
    var rows = Array.prototype.slice.call(list.querySelectorAll('.post-row'));
    if (rows.length <= step) { rows.forEach(function (r) { r.classList.add('in'); }); return; }
    var shown = 0;
    rows.forEach(function (r) { r.style.display = 'none'; });

    var fade = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) { e.target.classList.add('in'); fade.unobserve(e.target); }
      });
    }, { rootMargin: '0px 0px -8% 0px' });

    var more;
    function showMore() {
      var batch = rows.slice(shown, shown + step);
      batch.forEach(function (r) { r.style.display = ''; fade.observe(r); });
      shown += batch.length;
      if (shown >= rows.length && more) more.disconnect();
    }
    showMore();

    var sentinel = document.querySelector('.reveal-sentinel');
    if (sentinel) {
      more = new IntersectionObserver(function (entries) {
        if (entries[0].isIntersecting) showMore();
      }, { rootMargin: '300px' });
      more.observe(sentinel);
    }
  }

  // Ambient side-lights "focus" mode: while you're scrolling, fade the edge glows out so they
  // don't distract, then ease them back after a stretch of stillness. Pure decoration — a no-op
  // where --glow-1/--glow-2 are transparent (e.g. the broadsheet variant).
  function initFocusLights() {
    var root = document.documentElement, timer;
    var IDLE = 3 * 60 * 1000; // focus ends after 3 minutes without scrolling
    window.addEventListener('scroll', function () {
      root.classList.add('focus');
      clearTimeout(timer);
      timer = setTimeout(function () { root.classList.remove('focus'); }, IDLE);
    }, { passive: true });
  }

  initTheme();
  initFocusLights();
  initProgress();
  initAuthors();
  initTips();
  initFeeds();
  initMenu();
  initPostList();
})();
