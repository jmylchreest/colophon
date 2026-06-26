// colophon glossary decorator. Fetches the published glossary (term -> {def, links}), wraps the
// first occurrence of each term in the article in <abbr class="gloss" data-gloss="…">, and shows
// a themeable, accessible popover on hover/focus. An entry may carry reference links: these are
// baked after the term as citation-style superscript anchors (one glyph for a single link,
// numbers for several) — real links, so they work without an interactive popover, and the card
// lists them as a numbered legend. The theme owns the look: it styles .gloss (e.g. a wavy
// underline), .gloss-tip (the card) and .gloss-ref (the superscripts). Accessibility: the
// trigger is focusable, the popover is role="tooltip" linked via aria-describedby, it shows on
// focus as well as hover, and Escape dismisses it (WCAG 1.4.13). No dependency, no framework.
(function () {
  "use strict";

  var self = document.currentScript;
  if (!self) return;
  var url = self.getAttribute("data-glossary");
  if (!url) return;

  fetch(url, { credentials: "omit" })
    .then(function (r) { return r.ok ? r.json() : null; })
    .then(function (raw) { var g = normalise(raw); if (Object.keys(g).length) start(g); })
    .catch(function () { /* never let the glossary break the page */ });

  // normalise accepts both the current shape ({term: {def, links}}) and the legacy flat shape
  // ({term: "definition"}), returning entry objects {def, links} keyed by term.
  function normalise(raw) {
    var out = {};
    if (!raw) return out;
    Object.keys(raw).forEach(function (t) {
      var v = raw[t];
      if (typeof v === "string") out[t] = { def: v, links: [] };
      else if (v && typeof v === "object") out[t] = { def: v.def || "", links: v.links || [] };
    });
    return out;
  }

  // SAFE_URL permits only navigable schemes (and scheme-relative/relative URLs), so a config
  // typo can never bake a javascript:/data: link into the page.
  function safeURL(u) {
    u = String(u || "").trim();
    if (!u) return "";
    if (/^(https?:|mailto:|\/|#|\.)/i.test(u)) return u;
    return "";
  }

  // validLinks keeps only entries with a navigable URL, normalising {href, label} — so the marker
  // and the card legend share one view of what's clickable and number it identically.
  function validLinks(links) {
    var out = [];
    (links || []).forEach(function (l) {
      var href = safeURL(l && l.url);
      if (href) out.push({ href: href, label: (l.label || href).trim() });
    });
    return out;
  }

  // markText is the visible marker for the nth (0-based) of count links: a "↗" glyph for a lone
  // link, otherwise its 1-based number. Shared by the superscript and the legend so they agree.
  function markText(count, i) { return count === 1 ? "↗" : String(i + 1); }

  // refMarker builds the citation superscript for an entry's links, or null when none are
  // navigable. Each is a real anchor; external links open in a new tab.
  function refMarker(term, links) {
    var valid = validLinks(links);
    if (!valid.length) return null;
    var sup = document.createElement("sup");
    sup.className = "gloss-refs";
    valid.forEach(function (l, i) {
      var a = document.createElement("a");
      a.className = "gloss-ref";
      a.href = l.href;
      a.textContent = markText(valid.length, i);
      a.setAttribute("aria-label", term + " reference: " + l.label);
      if (/^https?:/i.test(l.href)) { a.target = "_blank"; a.rel = "noopener noreferrer"; }
      sup.appendChild(a);
    });
    return sup;
  }

  // Tags whose text must not be auto-decorated: code, links, existing abbreviations, headings,
  // and <noabbr> — the author's "leave this word alone" opt-out.
  var SKIP = { CODE: 1, PRE: 1, KBD: 1, SAMP: 1, A: 1, ABBR: 1, NOABBR: 1, BUTTON: 1, SCRIPT: 1,
    STYLE: 1, H1: 1, H2: 1, H3: 1, H4: 1, H5: 1, H6: 1 };

  function start(gloss) {
    var root = document.querySelector(".prose") ||
      document.querySelector("article") || document.querySelector("main");
    if (!root) return;
    setupPopover(gloss);
    var lower = {};
    Object.keys(gloss).forEach(function (t) { lower[t.toLowerCase()] = t; });
    var used = {};
    forceMarked(root, gloss, lower, used); // author-forced terms (<dfn>) always apply
    // A post with `glossary: false` sets data-gloss-auto="off": honour explicit forces but
    // skip automatic matching.
    if (self.getAttribute("data-gloss-auto") !== "off") {
      autoMatch(root, gloss, lower, used);
    }
  }

  // forceMarked decorates author-marked terms: an <abbr> (the same element auto-match produces)
  // whose text is a glossary term. This lets an author force a specific occurrence regardless of
  // the first-occurrence auto-match. An <abbr> that already carries its own title is left alone —
  // that's the author's one-off abbreviation, not a glossary term.
  function forceMarked(root, gloss, lower, used) {
    var marks = root.querySelectorAll("abbr");
    for (var i = 0; i < marks.length; i++) {
      var el = marks[i];
      if (el.getAttribute("data-gloss") || el.title) continue;
      var key = lower[(el.textContent || "").trim().toLowerCase()];
      if (!key) continue;
      el.classList.add("gloss");
      el.setAttribute("data-gloss", gloss[key].def);
      el.setAttribute("data-gloss-term", key);
      el.setAttribute("tabindex", "0");
      var sup = refMarker(key, gloss[key].links);
      if (sup) el.insertAdjacentElement("afterend", sup);
      used[key] = true; // don't auto-match it again elsewhere
    }
  }

  function autoMatch(root, gloss, lower, used) {
    var terms = Object.keys(gloss).sort(function (a, b) { return b.length - a.length; });
    var escaped = terms.map(function (t) { return t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"); });
    var matcher = new RegExp("\\b(" + escaped.join("|") + ")\\b", "gi");
    walk(root, gloss, lower, matcher, used);
  }

  // skip reports an element the decorator must not descend into: code/links/headings/<noabbr>,
  // or anything already decorated (.gloss).
  function skip(el) {
    return SKIP[el.tagName] || (el.classList && el.classList.contains("gloss"));
  }

  function walk(node, gloss, lower, matcher, used) {
    var kids = Array.prototype.slice.call(node.childNodes);
    for (var i = 0; i < kids.length; i++) {
      var n = kids[i];
      if (n.nodeType === 3) {
        wrap(n, gloss, lower, matcher, used);
      } else if (n.nodeType === 1 && !skip(n)) {
        walk(n, gloss, lower, matcher, used);
      }
    }
  }

  function wrap(textNode, gloss, lower, matcher, used) {
    var text = textNode.nodeValue;
    matcher.lastIndex = 0;
    var m, last = 0, frag = null;
    while ((m = matcher.exec(text))) {
      var key = lower[m[1].toLowerCase()];
      if (!key || used[key]) continue; // one decoration per term, per article
      used[key] = true;
      if (!frag) frag = document.createDocumentFragment();
      if (m.index > last) frag.appendChild(document.createTextNode(text.slice(last, m.index)));
      var abbr = document.createElement("abbr");
      abbr.className = "gloss";
      abbr.setAttribute("data-gloss", gloss[key].def);
      abbr.setAttribute("data-gloss-term", key); // canonical headword (glossary casing)
      abbr.setAttribute("tabindex", "0");
      abbr.textContent = m[1];
      frag.appendChild(abbr);
      var sup = refMarker(key, gloss[key].links); // citation links ride just after the term
      if (sup) frag.appendChild(sup);
      last = m.index + m[1].length;
    }
    if (frag) {
      if (last < text.length) frag.appendChild(document.createTextNode(text.slice(last)));
      textNode.parentNode.replaceChild(frag, textNode);
    }
  }

  function setupPopover(gloss) {
    var tip = document.createElement("div");
    tip.className = "gloss-tip";
    tip.id = "gloss-tip";
    tip.setAttribute("role", "tooltip");
    tip.hidden = true;
    // Structural styles are inline so the popover positions correctly without theme CSS; the
    // theme owns appearance via the .gloss-tip class.
    tip.style.position = "absolute";
    tip.style.zIndex = "120";
    tip.style.maxWidth = "18rem";
    tip.style.pointerEvents = "none";
    document.body.appendChild(tip);
    var current = null;

    function show(el) {
      var def = el.getAttribute("data-gloss");
      if (!def) return;
      // Build a small dictionary stanza: headword + definition (textContent, so no injection).
      tip.textContent = "";
      var head = document.createElement("span");
      head.className = "gloss-term";
      head.textContent = el.getAttribute("data-gloss-term") || el.textContent;
      var body = document.createElement("span");
      body.className = "gloss-def";
      body.textContent = def;
      tip.appendChild(head);
      tip.appendChild(body);
      // Legend: name each reference link so the bare superscript markers are decipherable. The
      // links live in the article (the markers), so the legend is plain text — the card stays a
      // non-interactive tooltip.
      var entry = gloss[el.getAttribute("data-gloss-term")];
      var links = validLinks(entry && entry.links);
      if (links.length) {
        var legend = document.createElement("span");
        legend.className = "gloss-legend";
        links.forEach(function (l, i) {
          var row = document.createElement("span");
          row.className = "gloss-legend-item";
          var mark = document.createElement("span");
          mark.className = "gloss-legend-mark";
          mark.textContent = markText(links.length, i);
          var label = document.createElement("span");
          label.textContent = l.label;
          row.appendChild(mark);
          row.appendChild(label);
          legend.appendChild(row);
        });
        tip.appendChild(legend);
      }
      tip.hidden = false;
      el.setAttribute("aria-describedby", "gloss-tip");
      current = el;
      place(el);
    }
    function hide() {
      if (!current) return;
      tip.hidden = true;
      current.removeAttribute("aria-describedby");
      current = null;
    }
    function place(el) {
      var r = el.getBoundingClientRect();
      var tw = tip.offsetWidth, th = tip.offsetHeight;
      var above = r.top - th - 8 >= 4;
      var top = above ? r.top - th - 8 : r.bottom + 8;
      var left = Math.max(8, Math.min(r.left + r.width / 2 - tw / 2, window.innerWidth - tw - 8));
      tip.style.top = (window.scrollY + top) + "px";
      tip.style.left = (window.scrollX + left) + "px";
      tip.setAttribute("data-placement", above ? "above" : "below");
    }

    function target(e) {
      return e.target && e.target.closest ? e.target.closest(".gloss[data-gloss]") : null;
    }
    document.addEventListener("mouseover", function (e) { var el = target(e); if (el) show(el); });
    document.addEventListener("mouseout", function (e) { if (target(e) === current) hide(); });
    document.addEventListener("focusin", function (e) { var el = target(e); if (el) show(el); else hide(); });
    document.addEventListener("keydown", function (e) { if (e.key === "Escape") hide(); });
    window.addEventListener("scroll", function () { if (current) place(current); }, { passive: true });
    window.addEventListener("resize", hide);
  }
})();
