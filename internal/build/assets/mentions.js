// mentions.js — engine-provided webmention responses renderer.
//
// A theme drops a placeholder: <section data-mentions="<src>" ...>. This script fetches the
// source and renders a facepile (likes/reposts) + reply cards. In asset mode <src> is our
// normalised _mentions/<key>.json; in live mode it's the receiver's read API and the element
// carries data-mentions-live + data-mentions-target (+ optional data-mentions-block).
//
// Mention content is third-party, so it is inserted as text (never innerHTML) and blocklisted
// entries are dropped client-side.
(() => {
  "use strict";

  const nodes = document.querySelectorAll("[data-mentions]");
  nodes.forEach((el) => {
    const src = el.getAttribute("data-mentions");
    if (!src) return;
    const live = el.hasAttribute("data-mentions-live");
    const target = el.getAttribute("data-mentions-target") || "";
    const url = live && target ? src + (src.includes("?") ? "&" : "?") + "target=" + encodeURIComponent(target) : src;
    const block = parseBlock(el.getAttribute("data-mentions-block"));

    fetch(url, { headers: { Accept: "application/json" } })
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (!data) return; // 404/error → leave any baked content untouched
        const list = (live ? fromJF2(data) : fromAsset(data)).filter((m) => !blocked(m, block));
        render(el, list);
      })
      .catch(() => {}); // network error → leave baked content
  });

  // ---- source shapes ------------------------------------------------------

  // Our normalised asset: { target, mentions: [{type, author{name,url,photo}, url, content, published}] }
  function fromAsset(data) {
    return Array.isArray(data.mentions) ? data.mentions : [];
  }

  // webmention.io JF2 feed: { children: [{ "wm-property", author{...}, url, content{text|html}, published }] }
  function fromJF2(data) {
    const kids = Array.isArray(data.children) ? data.children : [];
    return kids.map((c) => ({
      type: jf2Type(c["wm-property"]),
      author: {
        name: (c.author && c.author.name) || "",
        url: (c.author && c.author.url) || "",
        photo: (c.author && c.author.photo) || "",
      },
      url: c.url || "",
      content: c.content ? (c.content.text || stripTags(c.content.html) || "") : "",
      published: c.published || "",
    }));
  }

  function jf2Type(p) {
    if (p === "like-of") return "like";
    if (p === "repost-of") return "repost";
    if (p === "in-reply-to") return "reply";
    return "mention";
  }

  // ---- moderation (client-side glob, live mode) ---------------------------

  function parseBlock(raw) {
    if (!raw) return [];
    try { const a = JSON.parse(raw); return Array.isArray(a) ? a : []; } catch (_) { return []; }
  }
  function blocked(m, rules) {
    if (!rules.length) return false;
    const hay = [hostOf(m.author.url), m.author.url, m.author.name, m.url, m.content]
      .filter(Boolean).map((s) => String(s).toLowerCase());
    return rules.some((rule) => {
      const re = globRe(String(rule).toLowerCase());
      return hay.some((h) => re.test(h));
    });
  }
  function globRe(g) {
    const esc = g.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*").replace(/\?/g, ".");
    return new RegExp("^" + esc + "$");
  }
  function hostOf(u) { try { return new URL(u).host; } catch (_) { return ""; } }

  // ---- rendering (text only; never innerHTML for third-party data) --------

  function render(el, mentions) {
    if (!mentions.length) { el.replaceChildren(); return; } // empty section → .responses:empty hides it
    mentions = mentions.slice().sort((a, b) => mtime(b.published) - mtime(a.published)); // newest first
    const faces = mentions.filter(isReaction);
    const replies = mentions.filter((m) => !isReaction(m));
    const frag = document.createDocumentFragment();
    const title = elem("div", "responses-title"); title.textContent = "Responses";
    frag.appendChild(title);
    if (faces.length) frag.appendChild(facepile(faces));
    if (replies.length) frag.appendChild(replyList(replies));
    el.replaceChildren(frag);
  }

  function isReaction(m) { return m.type === "like" || m.type === "repost"; }
  function mtime(s) { const t = Date.parse(s || ""); return isNaN(t) ? 0 : t; }

  function facepile(faces) {
    const ul = elem("ul", "response-faces");
    ul.setAttribute("aria-label", "Reactions");
    faces.forEach((m) => {
      const li = elem("li", "response " + m.type + " h-cite");
      const a = elem("a", "p-author h-card u-url");
      a.href = m.author.url || m.url || "#";
      a.title = ((m.author.name || "") + (m.type === "repost" ? " reposted" : " liked") + " this").trim();
      if (m.author.photo) {
        const img = elem("img", "u-photo");
        img.src = m.author.photo; img.alt = m.author.name || ""; img.loading = "lazy";
        a.appendChild(img);
      } else {
        a.textContent = m.author.name ? m.author.name.slice(0, 1).toUpperCase() : "·";
      }
      li.appendChild(a);
      ul.appendChild(li);
    });
    return ul;
  }

  function replyList(replies) {
    const ol = elem("ol", "response-list");
    replies.forEach((m) => {
      const li = elem("li", "response reply h-cite");
      const who = elem("a", "p-author h-card u-url");
      who.href = m.author.url || m.url || "#";
      if (m.author.photo) {
        const img = elem("img", "u-photo");
        img.src = m.author.photo; img.alt = ""; img.loading = "lazy";
        who.appendChild(img);
      }
      const nm = elem("span", "p-name"); nm.textContent = m.author.name || "Someone";
      who.appendChild(nm);
      li.appendChild(who);
      if (m.url) {
        const perma = elem("a", "response-perma u-url"); perma.href = m.url;
        const silo = siloIcon(hostOf(m.url) || hostOf(m.author.url));
        if (silo) {
          perma.title = silo.label;
          const s = elem("span", "silo"); const ic = svgEl(silo.svg); if (ic) s.appendChild(ic);
          perma.appendChild(s);
        }
        if (m.published) {
          const t = elem("time", "dt-published"); t.dateTime = m.published; t.textContent = shortDate(m.published);
          perma.appendChild(t);
        } else if (!silo) {
          const g = elem("span", "response-go"); g.setAttribute("aria-hidden", "true"); g.textContent = "↗";
          perma.appendChild(g);
        }
        li.appendChild(perma);
      }
      if (m.content) {
        const c = elem("div", "p-content"); c.textContent = m.content;
        li.appendChild(c);
      }
      ol.appendChild(li);
    });
    return ol;
  }

  // ---- source (silo) detection + date ------------------------------------

  function hostOf(u) { try { return new URL(u).host.replace(/^www\./, ""); } catch (_) { return ""; } }
  function shortDate(s) {
    const t = Date.parse(s);
    if (isNaN(t)) return "";
    return new Date(t).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
  }

  const KNOWN_MASTODON = new Set(["hachyderm.io", "fosstodon.org", "mas.to", "mstdn.social",
    "infosec.exchange", "social.coop", "techhub.social", "indieweb.social"]);

  // Returns the icon + label only for a recognised silo; null otherwise (no generic fallback).
  function siloIcon(host) {
    const h = (host || "").toLowerCase();
    if (h.includes("bsky.")) return { svg: ICONS.bluesky, label: "Bluesky" };
    if (h === "github.com" || h.endsWith(".github.com")) return { svg: ICONS.github, label: "GitHub" };
    if (h === "x.com" || h === "twitter.com" || h.endsWith(".x.com") || h.endsWith(".twitter.com")) return { svg: ICONS.x, label: "X" };
    if (h.includes("mastodon") || h.includes("mstdn") || KNOWN_MASTODON.has(h)) return { svg: ICONS.mastodon, label: "Mastodon" };
    return null;
  }

  // svgEl parses a trusted, static icon constant (no user data) into an <svg> element — DOMParser
  // does not execute scripts or load resources.
  function svgEl(markup) {
    try {
      const el = new DOMParser().parseFromString(markup, "image/svg+xml").documentElement;
      return el && el.nodeName.toLowerCase() === "svg" ? el : null;
    } catch (_) { return null; }
  }

  const ICONS = {
    bluesky: '<svg viewBox="0 0 600 530" aria-hidden="true"><path fill="currentColor" d="M135 44c66 50 137 151 163 205 26-54 97-155 163-205 48-36 126-64 126 25 0 18-10 150-16 171-21 73-95 91-161 80 115 20 144 85 81 150-120 124-172-31-185-66-2-6-3-9-3-7 0-2-1 1-3 7-13 35-65 190-185 66-63-65-34-130 81-150-66 11-140-7-161-80-6-21-16-153-16-171 0-89 78-61 126-25Z"/></svg>',
    mastodon: '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M23.27 5.3c-.36-2.66-2.69-4.76-5.45-5.17C17.36.06 15.6 0 12 0h-.03C8.37 0 7.6.06 7.14.13 4.46.53 2.01 2.42 1.42 5.11.83 7.81.77 10.8.88 13.55c.16 3.93.2 4.62.97 6.79.69 1.93 2.62 3.41 4.7 3.99 2.28.64 4.73.75 7.06.31.32-.06.64-.13.95-.21l-.04-2.07s-1.6.37-3.4.31c-1.78-.06-3.66-.19-3.95-2.38a4.5 4.5 0 0 1-.04-.61s1.75.42 3.96.52c1.35.06 2.62-.08 3.91-.23 2.48-.3 4.64-1.82 4.91-3.21.43-2.19.4-5.35.4-5.35 0-3.1-2.05-4.01-2.05-4.01M19.62 14.5h-2.28v-5.6c0-1.17-.49-1.77-1.48-1.77-1.09 0-1.64.71-1.64 2.1v3.04H11.96V9.23c0-1.39-.55-2.1-1.64-2.1-.99 0-1.48.6-1.48 1.77v5.6H6.56V8.73c0-1.17.3-2.1.9-2.79.61-.69 1.42-1.04 2.42-1.04 1.16 0 2.04.45 2.62 1.34l.57.95.57-.95c.58-.89 1.46-1.34 2.62-1.34 1 0 1.81.35 2.42 1.04.6.69.9 1.62.9 2.79z"/></svg>',
    github: '<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>',
    x: '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231Zm-1.161 17.52h1.833L7.084 4.126H5.117Z"/></svg>',
  };

  function elem(tag, cls) { const e = document.createElement(tag); if (cls) e.className = cls; return e; }

  // Strip tags from JF2 html safely: DOMParser does not execute scripts or load resources.
  function stripTags(s) {
    if (!s) return "";
    try { return new DOMParser().parseFromString(s, "text/html").body.textContent || ""; }
    catch (_) { return ""; }
  }
})();
