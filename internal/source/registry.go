// Package source maps content-source driver names to implementations. Drivers live in
// sub-packages (internal/source/<driver>) and self-register from their init() via
// Register, so adding an origin touches only its own package plus a blank import at the
// call site. core defines the Source interface; this package wires names to it.
package source

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
)

// Factory constructs a source from its config; root lets drivers resolve relative paths.
type Factory func(root string, cfg config.SourceConfig) (core.Source, error)

var factories = map[string]Factory{}

// Register adds a driver, called from a driver's init(). Duplicate names panic.
func Register(driver string, f Factory) {
	if _, dup := factories[driver]; dup {
		panic("source: driver already registered: " + driver)
	}
	factories[driver] = f
}

// Open instantiates the source for cfg, or errors if its driver is unregistered.
func Open(root string, cfg config.SourceConfig) (core.Source, error) {
	f, ok := factories[cfg.Driver]
	if !ok {
		return nil, fmt.Errorf("unknown source driver %q (have: %s)", cfg.Driver, strings.Join(Drivers(), ", "))
	}
	return f(root, cfg)
}

// Drivers lists the registered driver names, sorted.
func Drivers() []string {
	names := make([]string, 0, len(factories))
	for n := range factories {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
