package syndicate

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// mastodonSyndicator posts a status to a Mastodon instance with an access token. Config:
// instance (e.g. https://hachyderm.io), token (via {env:VAR}), optional limit. Mastodon
// auto-links URLs, so the canonical link goes in the status text.
type mastodonSyndicator struct {
	id       string
	instance string
	token    string
	limit    int
}

func newMastodonSyndicator(conf core.SyndicatorConf) (*mastodonSyndicator, error) {
	s := &mastodonSyndicator{
		id:       conf.ID,
		instance: strings.TrimRight(confStr(conf.Settings, "instance"), "/"),
		token:    confStr(conf.Settings, "token"),
		limit:    500,
	}
	if s.instance == "" || s.token == "" {
		return nil, fmt.Errorf("syndicator %q (mastodon): set instance and token (token via {env:VAR})", conf.ID)
	}
	return s, nil
}

func (s *mastodonSyndicator) ID() string     { return s.id }
func (s *mastodonSyndicator) Driver() string { return "mastodon" }

func (s *mastodonSyndicator) Syndicate(ctx context.Context, p Post) (string, error) {
	var status struct {
		URL string `json:"url"`
	}
	// Mastodon honours an Idempotency-Key (keyed on the canonical post URL), so a retried create
	// is deduped server-side — making the status POST safe to retry on any transient failure.
	headers := map[string]string{}
	if p.URL != "" {
		headers["Idempotency-Key"] = p.URL
	}
	err := retryIdempotent(ctx, func() error {
		return postJSON(ctx, s.instance+"/api/v1/statuses", s.token, headers,
			map[string]any{"status": mastodonText(p, s.limit)}, &status)
	})
	if err != nil {
		return "", fmt.Errorf("mastodon %q: %w", s.id, err)
	}
	return status.URL, nil
}

// mastodonText composes the status: the blurb (custom text, else the title) and the link, kept
// under the limit by trimming the blurb (the URL is preserved so the link-back always survives).
func mastodonText(p Post, limit int) string {
	blurb := p.Text
	if blurb == "" {
		blurb = p.Title
	}
	if p.URL == "" {
		return limitRunes(blurb, limit)
	}
	room := limit - len([]rune(p.URL)) - 2 // "\n\n"
	if room < 0 {
		room = 0
	}
	blurb = limitRunes(blurb, room)
	return strings.TrimSpace(blurb) + "\n\n" + p.URL
}
