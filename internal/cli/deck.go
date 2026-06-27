package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yamlv3 "go.yaml.in/yaml/v3"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/markdown"
)

// DeckCmd (SPIKE, experimental) renders a Markdown file to a single self-contained HTML slide
// deck — CSS and the reader JS inlined, so the file works offline. Derived from the post's
// structure (headings → slides/bullets, prose → speaker notes). See slide.md for the design.
type DeckCmd struct {
	File  string `arg:"" help:"Markdown file to turn into a slide deck"`
	Out   string `short:"o" help:"Write the deck HTML here (default: <file>.deck.html; '-' for stdout)"`
	Split string `help:"Override slide boundaries, comma-separated (h1..h6, hr, newslide). Default: the post's slides.split, else h2,hr,newslide"`
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

	// Resolve the split list: --split flag wins, else the post's `slides.split`, else the default.
	// (Frontmatter config overwrites by key — it doesn't merge.)
	var split []string
	switch {
	case strings.TrimSpace(c.Split) != "":
		for _, s := range strings.Split(c.Split, ",") {
			if s = strings.TrimSpace(s); s != "" {
				split = append(split, s)
			}
		}
	default:
		split = frontmatterSplit(raw)
	}

	deck, err := build.BuildDeck(string(body), title, split)
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
	fmt.Println("wrote", out, "("+pluralSlides(deck)+")")
	return nil
}

// frontmatterSplit reads `slides.split` (a list) from a post's frontmatter block, tolerating the
// `slides: true` enable form (which carries no split list). Returns nil when unset.
func frontmatterSplit(raw []byte) []string {
	s := string(raw)
	if !strings.HasPrefix(s, "---") {
		return nil
	}
	rest := s[3:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil
	}
	var doc struct {
		Slides map[string]any `yaml:"slides"`
	}
	if yamlv3.Unmarshal([]byte(rest[:end]), &doc) != nil {
		return nil
	}
	list, _ := doc.Slides["split"].([]any)
	var out []string
	for _, v := range list {
		if str, ok := v.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

func pluralSlides(deck string) string {
	n := strings.Count(deck, `<section class="slide">`)
	if n == 1 {
		return "1 slide"
	}
	return fmt.Sprintf("%d slides", n)
}
