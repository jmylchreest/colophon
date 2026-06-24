package syndicate

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// bridgySyndicator delegates POSSE to Bridgy (https://brid.gy): instead of holding silo
// credentials, colophon sends Bridgy a publish webmention (source = the post, target =
// https://brid.gy/publish/<network>) and Bridgy — which already holds your account auth —
// creates the silo post from the page's microformats2 and returns its URL.
//
// Prerequisites the user owns: a connected account at brid.gy for the network, and the post
// must be live (Bridgy fetches it). Config: network (mastodon|bluesky|…), optional endpoint.
type bridgySyndicator struct {
	id       string
	network  string
	endpoint string
}

func newBridgySyndicator(conf core.SyndicatorConf) (*bridgySyndicator, error) {
	s := &bridgySyndicator{
		id:       conf.ID,
		network:  strings.ToLower(confStr(conf.Settings, "network")),
		endpoint: confStr(conf.Settings, "endpoint"),
	}
	if s.endpoint == "" {
		s.endpoint = "https://brid.gy/publish/webmention"
	}
	if s.network == "" {
		return nil, fmt.Errorf("syndicator %q (bridgy): set network (e.g. mastodon, bluesky)", conf.ID)
	}
	return s, nil
}

func (s *bridgySyndicator) ID() string     { return s.id }
func (s *bridgySyndicator) Driver() string { return "bridgy" }

func (s *bridgySyndicator) Syndicate(ctx context.Context, p Post) (string, error) {
	if p.URL == "" {
		return "", fmt.Errorf("bridgy %q: post has no URL (Bridgy must fetch the live source)", s.id)
	}
	form := url.Values{
		"source": {p.URL},
		"target": {"https://brid.gy/publish/" + s.network},
	}
	var resp struct {
		URL   string `json:"url"`
		Error string `json:"error"`
	}
	// Bridgy publish creates the silo post → retry only when nothing could have been created
	// (rate limit or unsent), avoiding a duplicate on an ambiguous mid-flight failure.
	if err := retrySafe(ctx, func() error { return postForm(ctx, s.endpoint, form, &resp) }); err != nil {
		return "", fmt.Errorf("bridgy %q: %w", s.id, err)
	}
	if resp.Error != "" {
		return "", fmt.Errorf("bridgy %q: %s", s.id, resp.Error)
	}
	return resp.URL, nil
}
