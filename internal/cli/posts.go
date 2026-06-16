package cli

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/colophon/internal/build"
)

// PostsCmd lists the existing content entries (slug, title, type, byline, voice, tags) for
// editing and cross-referencing. Filters narrow it by author, persona or tag.
type PostsCmd struct {
	Author  string   `help:"Only entries with this author id"`
	Persona string   `help:"Only entries with this persona id"`
	Tag     []string `help:"Only entries carrying any of these tags"`
	JSON    bool     `help:"Output JSON"`
}

func (c *PostsCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	entries, err := build.Entries(cfg)
	if err != nil {
		return err
	}
	var keep []build.Entry
	for _, e := range entries {
		if c.Author != "" && e.Author != c.Author {
			continue
		}
		if c.Persona != "" && e.Persona != c.Persona {
			continue
		}
		if len(c.Tag) > 0 && !anyTag(e.Tags, c.Tag) {
			continue
		}
		keep = append(keep, e)
	}
	if c.JSON {
		return writeJSON(keep)
	}
	if len(keep) == 0 {
		fmt.Println("No matching entries.")
		return nil
	}
	for _, e := range keep {
		var meta []string
		meta = append(meta, e.Type)
		if e.Author != "" {
			meta = append(meta, "author="+e.Author)
		}
		if e.Persona != "" {
			meta = append(meta, "persona="+e.Persona)
		}
		if len(e.Tags) > 0 {
			meta = append(meta, "tags=["+strings.Join(e.Tags, ",")+"]")
		}
		if e.Draft {
			meta = append(meta, "draft")
		}
		title := e.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("%-28s %-28s %s\n", e.Slug, title, strings.Join(meta, " "))
	}
	return nil
}

func anyTag(have, want []string) bool {
	for _, w := range want {
		for _, h := range have {
			if strings.EqualFold(h, w) {
				return true
			}
		}
	}
	return false
}
