package webmention

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// The reader pulls received mentions back from a hosted endpoint. JF2 (the format webmention.io
// and compatibles serve) is the one driver today; colophon owns the normalised shape, so adding
// another reader driver later means another Fetch* here, not a change to callers.

type jf2Feed struct {
	Children []jf2Item `json:"children"`
}

type jf2Item struct {
	WmProperty string      `json:"wm-property"`
	WmTarget   string      `json:"wm-target"`
	Author     jf2Author   `json:"author"`
	URL        string      `json:"url"`
	Published  string      `json:"published"`
	Content    *jf2Content `json:"content"`
	Name       string      `json:"name"`
}

type jf2Author struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Photo string `json:"photo"`
}

type jf2Content struct {
	Text string `json:"text"`
	HTML string `json:"html"`
}

// ReadEndpoint returns the reader's read-API URL: the explicit source if set, else the
// conventional webmention.io-style /api/mentions.jf2 derived from the receiver's host. "" when
// neither is configured.
func ReadEndpoint(source, receiver string) string {
	if s := strings.TrimSpace(source); s != "" {
		return s
	}
	u, err := url.Parse(strings.TrimSpace(receiver))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/api/mentions.jf2"
}

// FetchJF2 reads every mention for domain from a JF2 endpoint (paged), normalises each, and
// buckets them by target post key (KeyForURL of wm-target). It returns the full current set —
// callers full-regenerate the cache from it, so deletions self-heal.
func FetchJF2(ctx context.Context, client *http.Client, endpoint, domain, token string) (map[string]Mentions, error) {
	if client == nil {
		client = DefaultClient
	}
	const perPage = 200
	out := map[string]Mentions{}
	for page := 0; ; page++ {
		items, err := fetchJF2Page(ctx, client, endpoint, domain, token, perPage, page)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			target := it.WmTarget
			if target == "" {
				continue
			}
			key := KeyForURL(target)
			m := out[key]
			m.Target = target
			m.Mentions = append(m.Mentions, normaliseJF2(it))
			out[key] = m
		}
		if len(items) < perPage {
			break
		}
	}
	return out, nil
}

func fetchJF2Page(ctx context.Context, client *http.Client, endpoint, domain, token string, perPage, page int) ([]jf2Item, error) {
	q := url.Values{
		"domain":   {domain},
		"per-page": {fmt.Sprint(perPage)},
		"page":     {fmt.Sprint(page)},
	}
	if token != "" {
		q.Set("token", token)
	}
	u := endpoint
	if strings.Contains(u, "?") {
		u += "&" + q.Encode()
	} else {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("read endpoint %s returned %s: %s", endpoint, resp.Status, snippet(body))
	}
	var feed jf2Feed
	if err := json.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("decode JF2 from %s: %w", endpoint, err)
	}
	return feed.Children, nil
}

func normaliseJF2(it jf2Item) Mention {
	content := ""
	if it.Content != nil {
		content = it.Content.Text
		if content == "" {
			content = stripTags(it.Content.HTML)
		}
	}
	if content == "" {
		content = it.Name // some senders carry a bare p-name
	}
	return Mention{
		Type:      jf2Type(it.WmProperty),
		Author:    MentionAuthor{Name: it.Author.Name, URL: it.Author.URL, Photo: it.Author.Photo},
		URL:       it.URL,
		Content:   content,
		Published: it.Published,
	}
}

// jf2Type maps a JF2 wm-property to our normalised type.
func jf2Type(p string) string {
	switch p {
	case "like-of":
		return "like"
	case "repost-of":
		return "repost"
	case "in-reply-to":
		return "reply"
	default:
		return "mention"
	}
}

var tagRE = regexp.MustCompile(`(?s)<[^>]+>`)

// stripTags reduces sender-supplied HTML to plain text (the cache stores text; the renderer
// inserts it as textContent). Whitespace is collapsed.
func stripTags(s string) string {
	s = tagRE.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
