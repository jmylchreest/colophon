package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/clog"
)

// SearchCmd queries the site's content with the same BM25 engine that powers on-site search.
// It always works (independent of the site's public search setting) — it's the agent/CLI
// surface (PLAN §8). It builds an in-memory index from the current content and ranks the query.
type SearchCmd struct {
	Query string `arg:"" optional:"" help:"Search query"`
	Limit int    `help:"Maximum results" default:"20"`
	JSON  bool   `help:"Output JSON"`
}

func (c *SearchCmd) Run() error {
	if strings.TrimSpace(c.Query) == "" {
		return fmt.Errorf(`provide a query, e.g. colophon search "tigris"`)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	// A discard logger: indexing is a query-time means, not a build — its progress/warnings
	// shouldn't print (and must never pollute --json on stdout).
	quiet := clog.New(io.Discard, false, 8)
	ix, err := build.SearchIndex(cfg, build.Options{Log: quiet})
	if err != nil {
		return err
	}
	limit := c.Limit
	if limit <= 0 {
		limit = 20
	}
	results := ix.Search(c.Query, limit)
	if c.JSON {
		return writeJSON(results)
	}
	if len(results) == 0 {
		fmt.Println("No results.")
		return nil
	}
	for _, r := range results {
		title := r.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("%-40s %s\n", r.URL, title)
	}
	return nil
}
