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
	if err := postJSON(ctx, s.service+"/xrpc/com.atproto.server.createSession", "",
		map[string]string{"identifier": s.handle, "password": s.password}, &session); err != nil {
		return "", fmt.Errorf("bluesky %q: createSession: %w", s.id, err)
	}

	record := map[string]any{
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
	var created struct {
		URI string `json:"uri"`
	}
	if err := postJSON(ctx, s.service+"/xrpc/com.atproto.repo.createRecord", session.AccessJwt,
		map[string]any{"repo": session.DID, "collection": "app.bsky.feed.post", "record": record}, &created); err != nil {
		return "", fmt.Errorf("bluesky %q: createRecord: %w", s.id, err)
	}
	return blueskyPostURL(s.handle, created.URI), nil
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
