package cli

import (
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/telemetry"
)

// telemetryFor builds a server-side telemetry client from the first site's analytics config.
// It returns a disabled no-op when no site is configured or analytics isn't keyed, so callers
// can use the result unconditionally (and must Flush it). env is the environment name label.
func telemetryFor(cfg *config.Config, env, root string) *telemetry.Client {
	var a core.Analytics
	if len(cfg.Sites) > 0 {
		a = cfg.Sites[0].Analytics
	}
	return telemetry.New(a, env, version, root)
}
