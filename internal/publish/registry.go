// Package publish maps publisher driver names to implementations. Drivers live in
// sub-packages (internal/publish/<driver>) and self-register from their init() via
// Register, so adding a backend touches only its own package plus a blank import at
// the call site. core defines the Publisher interface; this package wires names to it.
package publish

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
)

// Factory constructs a publisher from its config; root lets drivers resolve relative paths.
type Factory func(root string, cfg config.PublisherConfig) (core.Publisher, error)

var factories = map[string]Factory{}

// Register adds a driver, called from a driver's init(). Duplicate names panic.
func Register(driver string, f Factory) {
	if _, dup := factories[driver]; dup {
		panic("publish: driver already registered: " + driver)
	}
	factories[driver] = f
}

// Open instantiates the publisher for cfg, or errors if its driver is unregistered.
func Open(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	f, ok := factories[cfg.Driver]
	if !ok {
		return nil, fmt.Errorf("unknown publisher driver %q (have: %s)", cfg.Driver, strings.Join(Drivers(), ", "))
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
