// colophon analytics beacon — a tiny, dependency-free statsfactory client.
// Reads its config + page dimensions from its own <script> tag's data-* attributes, sends a
// page_view on load and a page_engagement (active milliseconds, as the event value) when the
// page is hidden or unloaded. Cookieless: the session id lives in sessionStorage (per tab).
// Honours Do-Not-Track / Global Privacy Control by sending nothing.
(function () {
  "use strict";

  var script = document.currentScript;
  if (!script) return;

  var base = script.getAttribute("data-sf-url");
  var key = script.getAttribute("data-sf-key");
  if (!base || !key) return;

  // Respect browser privacy signals.
  if (
    navigator.doNotTrack === "1" ||
    window.doNotTrack === "1" ||
    navigator.globalPrivacyControl === true
  ) {
    return;
  }

  var endpoint = base.replace(/\/+$/, "") + "/v1/events";

  var sid;
  try {
    sid = sessionStorage.getItem("sf_sid");
    if (!sid) {
      sid =
        typeof crypto !== "undefined" && crypto.randomUUID
          ? crypto.randomUUID()
          : String(Date.now()) + "-" + Math.random().toString(36).slice(2);
      sessionStorage.setItem("sf_sid", sid);
    }
  } catch (e) {
    sid = String(Date.now()) + "-" + Math.random().toString(36).slice(2);
  }

  function dimensions() {
    var d = { "page.path": location.pathname };
    var slug = script.getAttribute("data-sf-slug");
    var type = script.getAttribute("data-sf-type");
    var author = script.getAttribute("data-sf-author");
    var tags = script.getAttribute("data-sf-tags");
    if (slug) d["post.slug"] = slug;
    if (type) d["post.type"] = type;
    if (author) d["post.author"] = author;
    if (tags) d["post.tags"] = tags.split(",");
    if (document.referrer) d["referrer"] = document.referrer;
    return d;
  }

  function send(event, value) {
    var ev = { event: event, session_id: sid, dimensions: dimensions() };
    if (value != null) ev.value = value;
    try {
      fetch(endpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer " + key,
        },
        body: JSON.stringify({ events: [ev] }),
        keepalive: true,
        mode: "cors",
        credentials: "omit",
      }).catch(function () {});
    } catch (e) {
      /* never let telemetry break the page */
    }
  }

  send("page_view");

  // Engagement: accumulate only the time the page is actually visible, and flush it (resetting
  // the accumulator) whenever the page is hidden or unloaded. value is the active milliseconds;
  // sub-second glances are ignored. Multiple flushes sum server-side.
  var acc = 0;
  var resume = Date.now();
  var visible = !document.hidden;

  function flush() {
    if (visible) {
      acc += Date.now() - resume;
      visible = false;
    }
    if (acc >= 1000) {
      send("page_engagement", Math.round(acc));
      acc = 0;
    }
  }

  document.addEventListener("visibilitychange", function () {
    if (document.hidden) {
      flush();
    } else {
      resume = Date.now();
      visible = true;
    }
  });
  window.addEventListener("pagehide", flush);
})();
