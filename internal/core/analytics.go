package core

import "strings"

// Analytics configures privacy-respecting telemetry for a site, sent to a statsfactory
// instance. The same configuration drives two surfaces: a reader-facing web beacon embedded
// in pages (page views + engagement time) and the binary's own build/publish events. It is
// inert until both ServerURL and AppKey are set, so an unconfigured project ships nothing —
// the values usually come from {env:STATSFACTORY_*} placeholders (see the dot-env files), and
// the statsfactory ingest key is a public "sf_live_" key that is safe to embed in page HTML.
type Analytics struct {
	// Provider names the backend; only "statsfactory" is supported. Empty or "off" disables.
	Provider string `yaml:"provider,omitempty"`
	// ServerURL is the statsfactory base URL (no trailing slash); AppKey is its public
	// ingest key. Both empty → analytics is inert.
	ServerURL string `yaml:"server_url,omitempty"`
	AppKey    string `yaml:"app_key,omitempty"`
	// Enabled is the master switch (default true when configured); Web and Server gate the
	// two surfaces individually (each default true when active). An explicit false opts out.
	Enabled *bool `yaml:"enabled,omitempty"`
	Web     *bool `yaml:"web,omitempty"`
	Server  *bool `yaml:"server,omitempty"`
}

// Active reports whether analytics is configured and not disabled: a supported provider,
// both credentials present, and Enabled not explicitly false.
func (a Analytics) Active() bool {
	if a.Enabled != nil && !*a.Enabled {
		return false
	}
	provider := strings.ToLower(strings.TrimSpace(a.Provider))
	if provider == "" || provider == "off" {
		return false
	}
	return provider == "statsfactory" &&
		strings.TrimSpace(a.ServerURL) != "" && strings.TrimSpace(a.AppKey) != ""
}

// WebEnabled reports whether the reader-facing web beacon should be emitted.
func (a Analytics) WebEnabled() bool {
	return a.Active() && (a.Web == nil || *a.Web)
}

// ServerEnabled reports whether the binary should send its own build/publish events.
func (a Analytics) ServerEnabled() bool {
	return a.Active() && (a.Server == nil || *a.Server)
}
