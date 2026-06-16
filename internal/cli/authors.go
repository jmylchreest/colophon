package cli

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
)

// AuthorsCmd lists the bylines (authors) and shows one's full detail. Aliased to the
// singular `author`.
type AuthorsCmd struct {
	List AuthorsListCmd `cmd:"" default:"1" help:"List authors (the bylines)"`
	Show AuthorsShowCmd `cmd:"" help:"Show one author's full details"`
}

// AuthorsListCmd lists the configured authors (id + name).
type AuthorsListCmd struct {
	JSON bool `help:"Output JSON"`
}

func (c *AuthorsListCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if c.JSON {
		return writeJSON(cfg.Authors)
	}
	if len(cfg.Authors) == 0 {
		fmt.Println("No authors. Add authors/<id>.yaml (the default byline is \"Anonymous\").")
		return nil
	}
	for _, a := range cfg.Authors {
		fmt.Printf("%-16s %s\n", a.ID, a.Name)
	}
	return nil
}

// AuthorsShowCmd prints one author's full identity (the rich h-card).
type AuthorsShowCmd struct {
	Author string `arg:"" help:"Author id"`
	JSON   bool   `help:"Output JSON"`
}

func (c *AuthorsShowCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	a := cfg.Author(c.Author)
	if a == nil {
		return fmt.Errorf("unknown author %q (have: %s)", c.Author, strings.Join(authorIDs(cfg), ", "))
	}
	if c.JSON {
		return writeJSON(a)
	}
	fmt.Printf("%s (%s)\n", a.Name, a.ID)
	if a.Bio != "" {
		fmt.Printf("Bio:    %s\n", a.Bio)
	}
	if a.Avatar != "" {
		fmt.Printf("Avatar: %s\n", a.Avatar)
	}
	if a.Email != "" {
		fmt.Printf("Email:  %s\n", a.Email)
	}
	for _, u := range a.URLs {
		fmt.Printf("URL:    %s\n", u)
	}
	return nil
}

func authorIDs(cfg *config.Config) []string {
	out := make([]string, len(cfg.Authors))
	for i, a := range cfg.Authors {
		out[i] = a.ID
	}
	return out
}
