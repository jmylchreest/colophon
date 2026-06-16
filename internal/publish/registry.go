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

// driverEnv records the environment variables each driver reads (deploy secrets, resolved
// from the environment rather than the config). Populated by RegisterEnv from a driver's init.
var driverEnv = map[string][]string{}

// Register adds a driver, called from a driver's init(). Duplicate names panic.
func Register(driver string, f Factory) {
	if _, dup := factories[driver]; dup {
		panic("publish: driver already registered: " + driver)
	}
	factories[driver] = f
}

// RegisterEnv declares the environment variables a driver reads (e.g. credentials), so
// `colophon env` can report them without instantiating the driver. Called from init.
func RegisterEnv(driver string, vars ...string) {
	driverEnv[driver] = append([]string(nil), vars...)
}

// DriverEnvVars returns the env vars read by the named drivers, deduped and sorted.
func DriverEnvVars(drivers []string) []string {
	set := map[string]struct{}{}
	for _, d := range drivers {
		for _, v := range driverEnv[d] {
			set[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
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
