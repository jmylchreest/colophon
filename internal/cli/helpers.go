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

// newLogger builds a progress logger whose label column fits the project's source,
// publisher, and environment names (so columns align). Width is clamped to a sane range.
func newLogger(cfg *config.Config, verbose bool) *clog.Logger {
	w := len("content") // the default md-dir source id, used when none are configured
	consider := func(s string) {
		if len(s) > w {
			w = len(s)
		}
	}
	for _, s := range cfg.Sources {
		consider(s.ID)
	}
	for _, p := range cfg.Publishers {
		consider(p.ID)
	}
	for _, e := range cfg.Environments {
		consider(e.Name)
	}
	if w > 24 {
		w = 24
	}
	return clog.New(os.Stdout, verbose, w)
}

// unknownEnvErr reports an unknown --env, listing the configured environments.
func unknownEnvErr(cfg *config.Config, name string) error {
	names := cfg.EnvironmentNames()
	if len(names) == 0 {
		return fmt.Errorf("unknown environment %q (none configured)", name)
	}
	return fmt.Errorf("unknown environment %q (configured: %s)", name, strings.Join(names, ", "))
}
