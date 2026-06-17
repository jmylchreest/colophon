package cli

import (
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/telemetry"
)

// telemetryFor builds colophon's tool-telemetry client from the project's top-level telemetry
// config (credentials fall back to the release-baked defaults). It returns a disabled no-op
// when telemetry is off, so callers can use the result unconditionally (and must Flush it).
// env is the environment-name label.
func telemetryFor(cfg *config.Config, env, root string) *telemetry.Client {
	return telemetry.New(cfg.Telemetry, env, version, root)
}
