package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
)

// findRoot walks up from the working directory to the project root (the directory
// containing colophon.yaml).
func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, config.ConfigFile)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not a colophon project (no colophon.yaml found in this or any parent directory)")
		}
		dir = parent
	}
}

// notImplemented reports a command planned for a later milestone.
func notImplemented(milestone string) error {
	return fmt.Errorf("not implemented yet (planned for %s)", milestone)
}

// newLogger builds colophon's progress logger. --verbose drops the level to Debug; the
// COLOPHON_LOG_FORMAT (text|json) and COLOPHON_LOG (RUST_LOG-style filter spec) env vars tune
// the output format and per-attribute/source verbosity. Logs go to stderr so --json data on
// stdout stays clean.
func newLogger(verbose bool) *clog.Logger {
	return clog.New(clog.Options{
		Writer:  os.Stderr,
		Verbose: verbose,
		JSON:    strings.EqualFold(os.Getenv("COLOPHON_LOG_FORMAT"), "json"),
		Filter:  os.Getenv("COLOPHON_LOG"),
	})
}

// unknownEnvErr reports an unknown --env, listing the configured environments.
func unknownEnvErr(cfg *config.Config, name string) error {
	names := cfg.EnvironmentNames()
	if len(names) == 0 {
		return fmt.Errorf("unknown environment %q (none configured)", name)
	}
	return fmt.Errorf("unknown environment %q (configured: %s)", name, strings.Join(names, ", "))
}
