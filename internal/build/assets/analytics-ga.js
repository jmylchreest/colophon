// colophon Google Analytics (GA4) loader. Reads the measurement id from its own <script>
// tag's data-ga-id attribute, injects Google's gtag.js, and configures it. gtag.js itself is
// served by Google and cannot be self-hosted; this loader is the colophon-owned part, written
// to the site root only when Google Analytics is enabled.
(function () {
  "use strict";

  var self = document.currentScript;
  if (!self) return;

  var id = self.getAttribute("data-ga-id");
  if (!id) return;

  var tag = document.createElement("script");
  tag.async = true;
  tag.src = "https://www.googletagmanager.com/gtag/js?id=" + encodeURIComponent(id);
  document.head.appendChild(tag);

  window.dataLayer = window.dataLayer || [];
  function gtag() {
    window.dataLayer.push(arguments);
  }
  window.gtag = gtag;
  gtag("js", new Date());
  gtag("config", id);
})();
