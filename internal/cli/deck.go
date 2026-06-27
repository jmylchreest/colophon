package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/markdown"
)

// DeckCmd (SPIKE, experimental) renders a Markdown file to a single self-contained HTML slide
// deck — CSS and the reader JS inlined, so the file works offline. Derived from the post's
// structure (headings → slides/bullets, prose → speaker notes). See slide.md for the design.
type DeckCmd struct {
	File string `arg:"" help:"Markdown file to turn into a slide deck"`
	Out  string `short:"o" help:"Write the self-contained deck HTML here (default: <file>.deck.html; '-' for stdout)"`
}

func (c *DeckCmd) Run() error {
	raw, err := os.ReadFile(c.File)
	if err != nil {
		return err
	}
	fm, body, err := markdown.ParseFrontmatter(raw)
	if err != nil {
		return err
	}
	title := fm.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(c.File), filepath.Ext(c.File))
	}
	deck, err := build.BuildDeck(string(body), title)
	if err != nil {
		return err
	}
	if c.Out == "-" {
		fmt.Print(deck)
		return nil
	}
	out := c.Out
	if out == "" {
		out = strings.TrimSuffix(c.File, filepath.Ext(c.File)) + ".deck.html"
	}
	if err := os.WriteFile(out, []byte(deck), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", out)
	return nil
}
