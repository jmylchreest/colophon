package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/colophon/internal/render"
)

// ThemesCmd groups theme inspection and customisation commands.
type ThemesCmd struct {
	List  ThemesListCmd  `cmd:"" help:"List the built-in themes"`
	Eject ThemesEjectCmd `cmd:"" help:"Copy a built-in theme into themes/<name>/ to customise"`
}

// ThemesListCmd prints the built-in theme names.
type ThemesListCmd struct{}

func (c *ThemesListCmd) Run() error {
	for _, n := range render.BuiltinThemes() {
		fmt.Println(n)
	}
	return nil
}

// ThemesEjectCmd copies a built-in theme to themes/<name>/ in the project so it can be
// edited; the on-disk copy then overrides the built-in per file.
type ThemesEjectCmd struct {
	Name  string `arg:"" help:"Built-in theme to copy (e.g. default, minimal)"`
	Force bool   `help:"Overwrite an existing themes/<name>/ directory"`
}

func (c *ThemesEjectCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	rel := filepath.Join("themes", c.Name)
	dest := filepath.Join(root, rel)
	if !c.Force {
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", rel)
		}
	}
	written, err := render.ExtractTheme(c.Name, dest)
	if err != nil {
		return err
	}
	fmt.Printf("Ejected %q to %s (%d files).\n", c.Name, rel, len(written))
	fmt.Printf("Edit the files there; set `theme: %s` (or an environment's theme) to use your copy.\n", c.Name)
	return nil
}
