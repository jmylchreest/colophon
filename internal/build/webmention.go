package build

import (
	"fmt"
	"html"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// webmentionReceiver returns the configured webmention receiver endpoint, or "".
// The endpoint is the hosted service (webmention.io, Bridgy Fed, …) that accepts
// inbound webmentions on the site's behalf; we only advertise it.
func webmentionReceiver(site core.Site) string {
	iw := site.Federation.IndieWeb
	if iw == nil || iw.Webmention == nil {
		return ""
	}
	return strings.TrimSpace(iw.Webmention.Receiver)
}

// webmentionHead returns the <link rel="webmention"> discovery tag for the page
// <head> when a receiver is configured, else "". This is the tag senders look for
// to know where to deliver a mention (and what Bridgy Fed needs to reach the site).
// It is emitted site-wide (every page), alongside the feed autodiscovery links.
func webmentionHead(site core.Site) string {
	endpoint := webmentionReceiver(site)
	if endpoint == "" {
		return ""
	}
	return fmt.Sprintf(`<link rel="webmention" href="%s">`, html.EscapeString(endpoint))
}
