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
    const faces = mentions.filter((m) => m.type === "like" || m.type === "repost");
    const replies = mentions.filter((m) => m.type !== "like" && m.type !== "repost");
    const frag = document.createDocumentFragment();
    const title = elem("div", "responses-title"); title.textContent = "Responses";
    frag.appendChild(title);
    if (faces.length) frag.appendChild(facepile(faces));
    if (replies.length) frag.appendChild(replyList(replies));
    el.replaceChildren(frag);
  }

  function facepile(faces) {
    const ul = elem("ul", "response-faces");
    faces.forEach((m) => {
      const li = elem("li", "response " + m.type + " h-cite");
      const a = elem("a", "p-author h-card u-url");
      a.href = m.author.url || m.url || "#";
      a.title = m.author.name || "";
      if (m.author.photo) {
        const img = elem("img", "u-photo");
        img.src = m.author.photo; img.alt = m.author.name || ""; img.loading = "lazy";
        a.appendChild(img);
      } else {
        a.textContent = m.author.name || "?";
      }
      li.appendChild(a);
      ul.appendChild(li);
    });
    return ul;
  }

  function replyList(replies) {
    const ul = elem("ul", "response-replies");
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
      if (m.content) {
        const c = elem("div", "p-content"); c.textContent = m.content;
        li.appendChild(c);
      }
      if (m.url) {
        const perma = elem("a", "u-url response-perma"); perma.href = m.url;
        if (m.published) {
          const t = elem("time", "dt-published"); t.dateTime = m.published; t.textContent = m.published;
          perma.appendChild(t);
        } else { perma.textContent = "permalink"; }
        li.appendChild(perma);
      }
      ul.appendChild(li);
    });
    return ul;
  }

  function elem(tag, cls) { const e = document.createElement(tag); if (cls) e.className = cls; return e; }

  // Strip tags from JF2 html safely: DOMParser does not execute scripts or load resources.
  function stripTags(s) {
    if (!s) return "";
    try { return new DOMParser().parseFromString(s, "text/html").body.textContent || ""; }
    catch (_) { return ""; }
  }
})();
