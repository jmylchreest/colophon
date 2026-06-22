// Package websub implements the publisher side of WebSub (W3C Recommendation,
// https://www.w3.org/TR/websub/): pinging a hub after publish so it pushes the updated
// feed to subscribers in real time. colophon is a static site, so it only *advertises*
// hubs in its feeds (rel="hub") and *notifies* them here — the hub does the fan-out.
package websub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultClient is a short-timeout client; a hub ping is fire-and-forget and must never
// hold up a publish.
var DefaultClient = &http.Client{Timeout: 15 * time.Second}

// Ping notifies a hub that a topic (feed URL) has new content, per WebSub §7
// ("hub.mode=publish"). A 2xx response means accepted. The body is read and discarded so
// the connection can be reused.
func Ping(ctx context.Context, client *http.Client, hub, topic string) error {
	if client == nil {
		client = DefaultClient
	}
	form := url.Values{"hub.mode": {"publish"}, "hub.url": {topic}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, hub, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hub %s returned %s", hub, resp.Status)
	}
	return nil
}
