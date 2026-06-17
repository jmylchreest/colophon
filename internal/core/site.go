package core

// Site is a published blog target: a theme, a set of personas allowed to publish
// to it, and the publishers its output deploys to.
type Site struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	BaseURL string `yaml:"base_url"`
	Theme   string `yaml:"theme"`
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
	// Search selects the visitor-facing on-site search: lexical | semantic | off.
	Search string `yaml:"search,omitempty"`
	// Analytics configures privacy-respecting telemetry (statsfactory). Inert until keyed.
	Analytics Analytics `yaml:"analytics,omitempty"`
}

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
}

type IndieWeb struct {
	Microformats bool            `yaml:"microformats,omitempty"`
	Webmention   *WebmentionConf `yaml:"webmention,omitempty"`
}

type WebmentionConf struct {
	Receiver       string `yaml:"receiver,omitempty"`
	BridgyBackfeed bool   `yaml:"bridgy_backfeed,omitempty"`
}

type Fediverse struct {
	BridgyFed bool `yaml:"bridgy_fed,omitempty"`
}
