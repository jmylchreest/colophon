package core

import "strings"

// Analytics configures a site's reader-facing analytics (page views, engagement), one block
// per provider. Each provider is independent and the build emits a beacon for every configured
// one. This is reader data, owned by the site owner — distinct from the tool's own usage
// reporting (see Telemetry). It is additionally subject to the top-level telemetry master
// switch: Telemetry.Enabled=false disables every provider here too.
type Analytics struct {
	Statsfactory    AnalyticsStatsfactory `yaml:"statsfactory,omitempty"`
	GoogleAnalytics AnalyticsGoogle       `yaml:"google_analytics,omitempty"`
}

// Any reports whether at least one analytics provider is configured.
func (a Analytics) Any() bool {
	return a.Statsfactory.Configured() || a.GoogleAnalytics.Configured()
}

// AnalyticsStatsfactory points the cookieless statsfactory beacon at the site owner's instance.
// The ingest key is a public "sf_live_" key, safe to embed in pages.
type AnalyticsStatsfactory struct {
	Enabled   *bool  `yaml:"enabled,omitempty"`
	ServerURL string `yaml:"server_url,omitempty"`
	AppKey    string `yaml:"app_key,omitempty"`
}

// Configured reports whether the statsfactory beacon should be emitted: credentials present
// and not explicitly disabled.
func (a AnalyticsStatsfactory) Configured() bool {
	if a.Enabled != nil && !*a.Enabled {
		return false
	}
	return strings.TrimSpace(a.ServerURL) != "" && strings.TrimSpace(a.AppKey) != ""
}

// AnalyticsGoogle configures Google Analytics (GA4). Unlike the statsfactory beacon, GA sets
// cookies and carries its own consent obligations, so it is opt-in per site and independent of
// the cookieless beacon.
type AnalyticsGoogle struct {
	Enabled       *bool  `yaml:"enabled,omitempty"`
	MeasurementID string `yaml:"measurement_id,omitempty"`
}

// Configured reports whether a GA tag should be emitted.
func (a AnalyticsGoogle) Configured() bool {
	if a.Enabled != nil && !*a.Enabled {
		return false
	}
	return strings.TrimSpace(a.MeasurementID) != ""
}
