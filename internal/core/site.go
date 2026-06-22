package core

// Site is a published blog target: a theme, a set of personas allowed to publish
// to it, and the publishers its output deploys to.
type Site struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	BaseURL string `yaml:"base_url"`
	Theme   string `yaml:"theme"`
	// Description is the site's one-line summary for search and social: the home/listing pages'
	// meta description, Open Graph/Twitter description, and WebSite/Blog JSON-LD. Empty omits those.
	Description string `yaml:"description,omitempty"`
	// Tagline is the short hero line a theme may show under the site title on the home page (the
	// press theme's lede). Purely presentational, distinct from the SEO Description. Empty: a
	// theme renders no lede.
	Tagline string `yaml:"tagline,omitempty"`
	// Image is the default social-share image for listing pages (og:image/twitter:image): an
	// absolute URL, or a site-root-relative path resolved against base_url. Empty omits it.
	Image string `yaml:"image,omitempty"`
	// Lang is the site's BCP-47 language tag (e.g. "en", "en-GB", "fr"), emitted as
	// <html lang>. A post may override it per-page. Empty defaults to "en".
	Lang string `yaml:"lang,omitempty"`
	// Favicon is a project-root-relative image used as the site icon. Empty falls back
	// to the theme's favicon.svg.
	Favicon string `yaml:"favicon,omitempty"`

	// Personas are the persona IDs allowed to publish here. A persona also declares
	// its sites; a publication must satisfy both sides.
	Personas []string `yaml:"personas"`
	// Publishers are the default deploy targets for this site's output.
	Publishers []string `yaml:"publishers"`
	// Routing optionally maps output path globs to specific publishers with URL
	// rewriting (e.g. assets to S3, HTML to Cloudflare). Empty means the whole tree
	// mirrors to every publisher above.
	Routing []RouteRule `yaml:"routing,omitempty"`

	Federation Federation `yaml:"federation,omitempty"`
	// Search configures the visitor-facing on-site search.
	Search SearchConfig `yaml:"search,omitempty"`
	// Analytics configures privacy-respecting telemetry (statsfactory). Inert until keyed.
	Analytics Analytics `yaml:"analytics,omitempty"`
}

// SearchConfig is the site's `search:` stanza. It accepts a string shorthand
// (`search: lexical`, equivalent to `search: { mode: lexical }`) or the full map form:
//
//	search:
//	  mode: lexical     # off (default) | lexical    [semantic reserved]
//	  fuzzy: true       # typo-tolerant matching (trigram + Levenshtein); larger index
type SearchConfig struct {
	Mode  string `yaml:"mode,omitempty"`
	Fuzzy bool   `yaml:"fuzzy,omitempty"`
}

// UnmarshalText decodes the string shorthand into Mode, so `search: lexical` keeps working
// alongside the map form. (koanf's default decode hook applies this for a scalar value; a map
// value decodes into the struct fields normally.)
func (s *SearchConfig) UnmarshalText(b []byte) error {
	s.Mode = string(b)
	return nil
}

// Enabled reports whether visitor-facing search is on.
func (s SearchConfig) Enabled() bool { return s.Mode == "lexical" || s.Mode == "semantic" }

// FuzzyEnabled reports whether typo-tolerant matching is on (only meaningful when Enabled).
func (s SearchConfig) FuzzyEnabled() bool { return s.Enabled() && s.Fuzzy }

// RouteRule sends output matching Match to Publisher, rewriting matched URLs to
// BaseURL when set.
type RouteRule struct {
	Match     string `yaml:"match"`
	Publisher string `yaml:"publisher"`
	BaseURL   string `yaml:"base_url,omitempty"`
}

// Federation toggles feeds and IndieWeb/fediverse features. Deferred to M4; carried
// here so config validates and round-trips today.
type Federation struct {
	Feeds     []string   `yaml:"feeds,omitempty"`
	IndieWeb  *IndieWeb  `yaml:"indieweb,omitempty"`
	Fediverse *Fediverse `yaml:"fediverse,omitempty"`
	WebSub    *WebSub    `yaml:"websub,omitempty"`
}

// WebSub advertises one or more WebSub hubs in the feeds (rel="hub") so subscribers
// get real-time push, and is pinged after publish to announce new content. Hubs are
// public URLs (e.g. https://pubsubhubbub.appspot.com/), not secrets.
type WebSub struct {
	Hubs []string `yaml:"hubs,omitempty"`
}

type IndieWeb struct {
	// Microformats2 markup (h-entry/h-card/h-feed) is always emitted by the themes — it is
	// invisible, beneficial, and the foundation other IndieWeb features parse — so there is no
	// toggle for it. Webmention stays opt-in below.
	Webmention *WebmentionConf `yaml:"webmention,omitempty"`
}

type WebmentionConf struct {
	Receiver       string             `yaml:"receiver,omitempty"` // advertised <link rel="webmention">
	Driver         string             `yaml:"driver,omitempty"`   // reader driver for received mentions; default jf2
	Source         string             `yaml:"source,omitempty"`   // reader read-API URL; derived from receiver when empty
	Token          string             `yaml:"token,omitempty"`    // read-API token, via {env:VAR}; never a literal
	Display        *WebmentionDisplay `yaml:"display,omitempty"`
	BridgyBackfeed bool               `yaml:"bridgy_backfeed,omitempty"`
}

// WebmentionDisplay selects how received mentions reach the page: live (browser → receiver
// directly), asset (browser → our published _mentions/, also bakeable at build), or disabled
// (nothing shipped). Default disabled. See docs/design/webmention.md.
type WebmentionDisplay struct {
	Mode string `yaml:"mode,omitempty"`
}

type Fediverse struct {
	BridgyFed bool `yaml:"bridgy_fed,omitempty"`
}
