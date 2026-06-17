package build

import (
	_ "embed"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/search"
)

// readerJS is the engine's browser reader, vendored from ../../search/search.js and emitted to
// _search/search.js so any theme can drive on-site search. It is kept byte-identical to the
// canonical source by TestEmbeddedReaderMatchesSource.
//
//go:embed assets/search.js
var readerJS []byte

// searchBase is the output directory (and URL path) the static index + reader live under.
const searchBase = "_search"

// searchEnabled reports whether a site opts into visitor-facing search (Site.Search is
// lexical|semantic|off; semantic falls back to the lexical index for now).
func searchEnabled(site core.Site) bool {
	switch site.Search {
	case "lexical", "semantic":
		return true
	}
	return false
}

// writeSearchIndex emits the static search index and the browser reader under _search/ when the
// site enables search. Index files are content-addressed, so the incremental publisher only
// uploads what changed; the reader is a fixed filename.
func writeSearchIndex(write func(string, []byte) error, pages []page, site core.Site, basePath string, log *clog.Logger) error {
	if !searchEnabled(site) {
		return nil
	}
	docs := pagesToSearchDocs(pages, basePath)
	man, err := search.Build(docs, searchWriter{write: write}, search.BuildOptions{})
	if err != nil {
		return err
	}
	if err := write(searchBase+"/search.js", readerJS); err != nil {
		return err
	}
	log.Step("BUILD", "", "search", len(docs), "shards", len(man.Shards))
	return nil
}

// searchWriter adapts the build's write closure to search.Writer, rooting every file under
// _search/.
type searchWriter struct {
	write func(string, []byte) error
}

func (w searchWriter) Put(name string, data []byte) error {
	return w.write(searchBase+"/"+name, data)
}

// pagesToSearchDocs maps built pages to engine documents. ID is the base_path-independent page
// URL (stable across hosting prefixes); the result link is base_path-prefixed and absolute.
func pagesToSearchDocs(pages []page, basePath string) []search.Doc {
	docs := make([]search.Doc, 0, len(pages))
	for _, p := range pages {
		link := "/" + strings.TrimPrefix(strings.TrimSuffix(basePath, "/")+"/"+p.URL, "/")
		var meta map[string]string
		if p.Type != "" {
			meta = map[string]string{"type": p.Type}
		}
		docs = append(docs, search.Doc{
			ID:    p.URL,
			URL:   link,
			Title: p.Title,
			Body:  htmlToText(p.HTML),
			Meta:  meta,
		})
	}
	return docs
}

// SearchIndex builds the in-memory search index for the CLI surface (colophon search). It runs
// the content pipeline but emits nothing, and ignores Site.Search — CLI search is always
// available (PLAN §8), independent of whether the public site exposes a search box.
func SearchIndex(cfg *config.Config, opts Options) (*search.Index, error) {
	if len(cfg.Sites) == 0 {
		return nil, fmt.Errorf("no sites configured")
	}
	site := cfg.Sites[0]
	basePath := resolveBasePath(opts.BasePath, site.BaseURL)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	docs, err := gatherDocuments(cfg, opts.Log)
	if err != nil {
		return nil, err
	}
	router := core.NewRouter(resolveRoutesFromConfig(site.Routing, cfg), opts.Publishers)
	pages, _, _, err := buildPages(docs, opts.IncludeDrafts, now, basePath, site.BaseURL, router)
	if err != nil {
		return nil, err
	}
	return search.NewIndex(pagesToSearchDocs(pages, basePath), search.BuildOptions{})
}

var (
	reScriptStyle = regexp.MustCompile(`(?is)<(script|style)\b[^>]*>.*?</(?:script|style)>`)
	reTag         = regexp.MustCompile(`(?s)<[^>]+>`)
)

// htmlToText reduces rendered page HTML to indexable plain text: drop script/style blocks and
// tags, decode entities, and collapse whitespace. It need not be perfect — it feeds the analyzer.
func htmlToText(s string) string {
	s = reScriptStyle.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
}
