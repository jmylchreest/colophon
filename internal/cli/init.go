package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/scaffold"
)

// InitCmd scaffolds a new colophon project.
type InitCmd struct {
	Dir   string `arg:"" optional:"" default:"." help:"Target directory"`
	Force bool   `help:"Overwrite an existing colophon.yaml"`
}

func (c *InitCmd) Run() error {
	if _, err := os.Stat(filepath.Join(c.Dir, config.ConfigFile)); err == nil && !c.Force {
		return fmt.Errorf("%s already exists (use --force to overwrite)", config.ConfigFile)
	}
	if err := scaffold.Project(c.Dir); err != nil {
		return err
	}
	fmt.Printf("Initialised colophon project in %s\n", c.Dir)
	fmt.Println("Next: edit colophon.yaml, then run `colophon doctor`.")
	return nil
}
