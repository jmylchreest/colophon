package syndicate

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/core"
)

// blueskySyndicator posts to Bluesky over AT-proto: create a session from the handle + an app
// password, then create an app.bsky.feed.post record with an external embed card linking back to
// the canonical post. Config: handle, app_password (via {env:VAR}), optional service.
type blueskySyndicator struct {
	id       string
	service  string
	handle   string
	password string
}

const blueskyLimit = 300 // Bluesky post text limit (graphemes; runes is a safe approximation)

func newBlueskySyndicator(conf core.SyndicatorConf) (*blueskySyndicator, error) {
	s := &blueskySyndicator{
		id:       conf.ID,
		service:  confStr(conf.Settings, "service"),
		handle:   confStr(conf.Settings, "handle"),
		password: confStr(conf.Settings, "app_password"),
	}
	if s.service == "" {
		s.service = "https://bsky.social"
	}
	if s.handle == "" || s.password == "" {
		return nil, fmt.Errorf("syndicator %q (bluesky): set handle and app_password (app_password via {env:VAR})", conf.ID)
	}
	return s, nil
}

func (s *blueskySyndicator) ID() string     { return s.id }
func (s *blueskySyndicator) Driver() string { return "bluesky" }

func (s *blueskySyndicator) Syndicate(ctx context.Context, p Post) (string, error) {
	var session struct {
		AccessJwt string `json:"accessJwt"`
		DID       string `json:"did"`
	}
	// Auth is idempotent → retry any transient failure.
	if err := retryIdempotent(ctx, func() error {
		return postJSON(ctx, s.service+"/xrpc/com.atproto.server.createSession", "", nil,
			map[string]string{"identifier": s.handle, "password": s.password}, &session)
	}); err != nil {
		return "", fmt.Errorf("bluesky %q: createSession: %w", s.id, err)
	}

	record := blueskyRecord(p)
	var created struct {
		URI string `json:"uri"`
	}
	// createRecord is not idempotent (no idempotency key) → retry only when nothing could have
	// been posted (rate limit or unsent), so a mid-flight failure never risks a duplicate skeet.
	if err := retrySafe(ctx, func() error {
		return postJSON(ctx, s.service+"/xrpc/com.atproto.repo.createRecord", session.AccessJwt, nil,
			map[string]any{"repo": session.DID, "collection": "app.bsky.feed.post", "record": record}, &created)
	}); err != nil {
		return "", fmt.Errorf("bluesky %q: createRecord: %w", s.id, err)
	}
	return blueskyPostURL(s.handle, created.URI), nil
}

// Replace refreshes a Bluesky card by atomically deleting and recreating the record at the SAME
// rkey (applyWrites). Bluesky's AppView ignores record edits (putRecord is a visual no-op), so a
// swap is the only way to change what people see; reusing the rkey keeps the permalink (so backlinks
// and the ledger URL stay valid), but because it's a new record the post's likes/reposts/replies
// reset and the timestamp updates. applyWrites is a single transaction, so a failed attempt changes
// nothing and is safe to retry. The run loop only calls this on an explicit --resync.
func (s *blueskySyndicator) Replace(ctx context.Context, p Post, prior Record) (string, error) {
	rkey := lastPathSegment(prior.URL)
	if rkey == "" {
		return "", fmt.Errorf("bluesky %q: no rkey in %q", s.id, prior.URL)
	}
	var session struct {
		AccessJwt string `json:"accessJwt"`
		DID       string `json:"did"`
	}
	if err := retryIdempotent(ctx, func() error {
		return postJSON(ctx, s.service+"/xrpc/com.atproto.server.createSession", "", nil,
			map[string]string{"identifier": s.handle, "password": s.password}, &session)
	}); err != nil {
		return "", fmt.Errorf("bluesky %q: createSession: %w", s.id, err)
	}
	const coll = "app.bsky.feed.post"
	writes := []map[string]any{
		{"$type": "com.atproto.repo.applyWrites#delete", "collection": coll, "rkey": rkey},
		{"$type": "com.atproto.repo.applyWrites#create", "collection": coll, "rkey": rkey, "value": blueskyRecord(p)},
	}
	if err := retryIdempotent(ctx, func() error {
		return postJSON(ctx, s.service+"/xrpc/com.atproto.repo.applyWrites", session.AccessJwt, nil,
			map[string]any{"repo": session.DID, "writes": writes}, nil)
	}); err != nil {
		return "", fmt.Errorf("bluesky %q: applyWrites: %w", s.id, err)
	}
	return prior.URL, nil // the permalink is unchanged (same rkey)
}

// blueskyRecord builds the app.bsky.feed.post record (text + external embed card linking back).
func blueskyRecord(p Post) map[string]any {
	return map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      blueskyText(p),
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"embed": map[string]any{
			"$type": "app.bsky.embed.external",
			"external": map[string]any{
				"uri":         p.URL,
				"title":       limitRunes(p.Title, 300),
				"description": limitRunes(p.Summary, 1000),
			},
		},
	}
}

// blueskyText is the post body: the custom blurb, else the title, trimmed to the limit. The
// link-back is the embed card, so the URL isn't repeated in the text.
func blueskyText(p Post) string {
	t := p.Text
	if t == "" {
		t = p.Title
	}
	return limitRunes(t, blueskyLimit)
}

// blueskyPostURL turns an at:// record URI into the public bsky.app permalink.
//
//	at://did:plc:abc/app.bsky.feed.post/3kxyz → https://bsky.app/profile/<handle>/post/3kxyz
func blueskyPostURL(handle, uri string) string {
	rkey := uri
	if i := strings.LastIndex(uri, "/"); i >= 0 {
		rkey = uri[i+1:]
	}
	if rkey == "" {
		return uri
	}
	return "https://bsky.app/profile/" + handle + "/post/" + rkey
}
