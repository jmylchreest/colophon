// Package webmention implements the sender half of Webmention (W3C Recommendation,
// https://www.w3.org/TR/webmention/): after a post is live, notify every site it links to
// so the link can appear as a response there. colophon never receives webmentions (it is
// static); receiving is delegated to a hosted endpoint. This package only sends.
package webmention

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// DefaultClient follows redirects (endpoint discovery relies on the final URL) and has a
// modest timeout; a hung target must not stall a send sweep.
var DefaultClient = &http.Client{Timeout: 20 * time.Second}

// CanonicalURL returns the page's <link rel="canonical"> href, or "" — the authoritative
// live URL to use as the webmention source. Built pages bake this from base_url.
func CanonicalURL(body []byte) string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "link" {
			rel, href := attr(n, "rel"), attr(n, "href")
			if hasToken(rel, "canonical") && href != "" {
				found = href
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

// OutboundLinks parses rendered HTML and returns the absolute http(s) links that point to
// a different origin than source (the page's own URL). De-duplicated, document order. These
// are the targets to notify; same-origin links are internal navigation and skipped.
func OutboundLinks(source string, body []byte) ([]string, error) {
	base, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse source %q: %w", source, err)
	}
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	seen := make(map[string]struct{})
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if href := attr(n, "href"); href != "" {
				if u, err := base.Parse(href); err == nil && isHTTP(u) && !sameHost(u, base) {
					u.Fragment = ""
					s := u.String()
					if _, dup := seen[s]; !dup {
						seen[s] = struct{}{}
						out = append(out, s)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out, nil
}

// Discover finds target's webmention endpoint, per the spec's precedence: an HTTP
// Link: rel="webmention" header first, then the first <link>/<a rel="webmention"> in the
// body. The endpoint is resolved against the final (post-redirect) URL. "" means none.
func Discover(ctx context.Context, client *http.Client, target string) (string, error) {
	if client == nil {
		client = DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	final := resp.Request.URL // after redirects

	if ep := endpointFromLinkHeader(resp.Header.Values("Link")); ep != "" {
		return resolve(final, ep), nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if ep := endpointFromHTML(body); ep != "" {
		return resolve(final, ep), nil
	}
	return "", nil
}

// Send POSTs source/target (form-encoded) to the endpoint. A 2xx is success; the spec also
// allows 201/202 (queued).
func Send(ctx context.Context, client *http.Client, endpoint, source, target string) error {
	if client == nil {
		client = DefaultClient
	}
	form := url.Values{"source": {source}, "target": {target}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("endpoint %s returned %s", endpoint, resp.Status)
	}
	return nil
}

// endpointFromLinkHeader scans HTTP Link header values for rel="webmention".
func endpointFromLinkHeader(values []string) string {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			urlPart, relOK := "", false
			for i, seg := range strings.Split(part, ";") {
				seg = strings.TrimSpace(seg)
				if i == 0 {
					urlPart = strings.Trim(seg, "<>")
					continue
				}
				if k, val, ok := splitParam(seg); ok && strings.EqualFold(k, "rel") && hasToken(val, "webmention") {
					relOK = true
				}
			}
			if relOK && urlPart != "" {
				return urlPart
			}
		}
	}
	return ""
}

// endpointFromHTML returns the href of the first <link> or <a> with rel="webmention".
func endpointFromHTML(body []byte) string {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return ""
	}
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && (n.Data == "link" || n.Data == "a") {
			if hasToken(attr(n, "rel"), "webmention") {
				if href := attr(n, "href"); href != "" {
					found = href
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val
		}
	}
	return ""
}

// hasToken reports whether a space-separated attribute (rel) contains token (case-insensitive).
func hasToken(rel, token string) bool {
	for _, f := range strings.Fields(rel) {
		if strings.EqualFold(f, token) {
			return true
		}
	}
	return false
}

func splitParam(s string) (key, val string, ok bool) {
	k, v, found := strings.Cut(s, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"`), true
}

func resolve(base *url.URL, ref string) string {
	if base == nil {
		return ref
	}
	if u, err := base.Parse(ref); err == nil {
		return u.String()
	}
	return ref
}

func isHTTP(u *url.URL) bool { return u.Scheme == "http" || u.Scheme == "https" }

func sameHost(a, b *url.URL) bool { return strings.EqualFold(a.Host, b.Host) }
