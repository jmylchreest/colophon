package core

import "strings"

// Telemetry is the colophon APP's own usage reporting (the tool → its maintainer). It is
// distinct from site Analytics and governs only itself: Enabled (default true) toggles whether
// the app initialises its telemetry at all. Credentials default to values baked into the
// binary at release (see internal/telemetry) and may be overridden here.
type Telemetry struct {
	Enabled      *bool                 `yaml:"enabled,omitempty"`
	Statsfactory TelemetryStatsfactory `yaml:"statsfactory,omitempty"`
}

// On reports whether the app should initialise its own telemetry (enabled unless set false).
func (t Telemetry) On() bool { return t.Enabled == nil || *t.Enabled }

// TelemetryStatsfactory is the tool-telemetry destination. Empty fields fall back to the
// release-baked defaults, so a configured colophon usually leaves this unset.
type TelemetryStatsfactory struct {
	Enabled   *bool  `yaml:"enabled,omitempty"`
	ServerURL string `yaml:"server_url,omitempty"`
	AppKey    string `yaml:"app_key,omitempty"`
}

// Resolve returns the effective server URL and key: the configured values, each falling back
// to the corresponding release-baked default. A provider explicitly disabled returns empty.
func (s TelemetryStatsfactory) Resolve(defURL, defKey string) (url, key string) {
	if s.Enabled != nil && !*s.Enabled {
		return "", ""
	}
	url = strings.TrimSpace(s.ServerURL)
	if url == "" {
		url = strings.TrimSpace(defURL)
	}
	key = strings.TrimSpace(s.AppKey)
	if key == "" {
		key = strings.TrimSpace(defKey)
	}
	return url, key
}
