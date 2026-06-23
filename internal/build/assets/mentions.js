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
      // Left column: source silo + date.
      if (m.url) {
        const perma = elem("a", "response-perma u-url"); perma.href = m.url;
        const silo = siloFor(hostOf(m.url) || hostOf(m.author.url));
        if (silo) {
          perma.title = silo.label;
          const s = elem("span", "silo"); s.setAttribute("aria-hidden", "true"); s.textContent = silo.glyph;
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
      // Right column: author + one-line content preview.
      const body = elem("div", "response-body");
      const who = elem("a", "p-author h-card u-url");
      who.href = m.author.url || m.url || "#";
      if (m.author.photo) {
        const img = elem("img", "u-photo");
        img.src = m.author.photo; img.alt = ""; img.loading = "lazy";
        who.appendChild(img);
      }
      const nm = elem("span", "p-name"); nm.textContent = m.author.name || "Someone";
      who.appendChild(nm);
      body.appendChild(who);
      if (m.content) {
        const c = elem("span", "p-content"); c.textContent = m.content; c.title = m.content;
        body.appendChild(c);
      }
      li.appendChild(body);
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

  // silo id → codepoint in silos.woff2 — KEEP IN SYNC with mentions.go siloGlyph / silos.json.
  const SILO_CP = {
    bluesky: 0xf300, mastodon: 0xf301, github: 0xf302, x: 0xf303, reddit: 0xf304,
    hackernews: 0xf305, threads: 0xf306, flickr: 0xf307, linkedin: 0xf308, tumblr: 0xf309,
    gitlab: 0xf30a, website: 0xf30b,
  };
  const SILO_LABEL = {
    bluesky: "Bluesky", mastodon: "Mastodon", github: "GitHub", x: "X", reddit: "Reddit",
    hackernews: "Hacker News", threads: "Threads", flickr: "Flickr", linkedin: "LinkedIn",
    tumblr: "Tumblr", gitlab: "GitLab", website: "Website",
  };

  // host → silo id (mirrors mentions.go siloForHost). Single-domain silos match exactly; Mastodon
  // is heuristic; any other http(s) host falls back to the generic website globe.
  function siloIdFor(host) {
    const h = (host || "").toLowerCase();
    if (!h) return "";
    if (h.includes("bsky.")) return "bluesky";
    if (h === "github.com" || h.endsWith(".github.com")) return "github";
    if (h === "gitlab.com") return "gitlab";
    if (h === "reddit.com" || h.endsWith(".reddit.com")) return "reddit";
    if (h === "news.ycombinator.com") return "hackernews";
    if (h === "threads.net" || h.endsWith(".threads.net")) return "threads";
    if (h === "flickr.com" || h.endsWith(".flickr.com")) return "flickr";
    if (h === "linkedin.com" || h.endsWith(".linkedin.com")) return "linkedin";
    if (h === "tumblr.com" || h.endsWith(".tumblr.com")) return "tumblr";
    if (h === "x.com" || h === "twitter.com" || h.endsWith(".x.com") || h.endsWith(".twitter.com")) return "x";
    if (h.includes("mastodon") || h.includes("mstdn") || KNOWN_MASTODON.has(h)) return "mastodon";
    return "website";
  }

  // siloFor returns the silo glyph char + label for a host, or null (empty host only).
  function siloFor(host) {
    const id = siloIdFor(host);
    if (!id || !(id in SILO_CP)) return null;
    return { glyph: String.fromCodePoint(SILO_CP[id]), label: SILO_LABEL[id] };
  }

  function elem(tag, cls) { const e = document.createElement(tag); if (cls) e.className = cls; return e; }

  // Strip tags from JF2 html safely: DOMParser does not execute scripts or load resources.
  function stripTags(s) {
    if (!s) return "";
    try { return new DOMParser().parseFromString(s, "text/html").body.textContent || ""; }
    catch (_) { return ""; }
  }
})();
