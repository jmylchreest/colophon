package cli

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
)

// DoctorCmd validates the project config and reports problems.
type DoctorCmd struct{}

func (c *DoctorCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	fmt.Printf("OK  %s\n", root)
	fmt.Printf("  sites:        %d\n", len(cfg.Sites))
	fmt.Printf("  publishers:   %d\n", len(cfg.Publishers))
	fmt.Printf("  environments: %d (%s)\n", len(cfg.Environments), strings.Join(cfg.EnvironmentNames(), ", "))
	fmt.Printf("  personas:     %d\n", len(cfg.Personas))
	return nil
}
