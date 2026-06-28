package build

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
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

// searchUIJS is the engine-provided search-box glue (toggle, panel, keyboarding, highlighted
// results). It's emitted at the site root so ANY theme can add a search box with just the markup
// + CSS — it reads the index location from the markup's data-search-* attributes and lazy-imports
// the reader. Themes re-skin the box purely through CSS on the .search-* classes.
//
//go:embed assets/search-ui.js
var searchUIJS []byte

// searchBase is the output directory (and URL path) the static index + reader live under.
const searchBase = "_search"

// searchEnabled reports whether a site opts into visitor-facing search.
func searchEnabled(site core.Site) bool { return site.Search.Enabled() }

// writeSearchIndex emits the static search index and the browser reader under _search/ when the
// site enables search. Index files are content-addressed, so the incremental publisher only
// uploads what changed; the reader is a fixed filename.
func writeSearchIndex(write func(string, []byte) error, pages []page, site core.Site, basePath, env string, log *clog.Logger) error {
	if !searchEnabled(site) {
		return nil
	}
	docs := pagesToSearchDocs(pages, basePath)
	man, err := search.Build(docs, searchWriter{write: write}, search.BuildOptions{
		ManifestName: searchManifestName(site.ID, env),
		Fuzzy:        site.Search.FuzzyEnabled(),
	})
	if err != nil {
		return err
	}
	if err := write(searchBase+"/search.js", readerJS); err != nil {
		return err
	}
	// The UI glue ships at the site root (same-origin, not routed), so a theme references it as
	// {base_path}search-ui.js regardless of where the routed index lives.
	if err := write("search-ui.js", searchUIJS); err != nil {
		return err
	}
	log.Step("BUILD", "", "search", len(docs), "shards", len(man.Shards))
	return nil
}

// searchManifestName is the per-deployment manifest filename. Several builds can publish to one
// object store; only the manifest (the mutable root) is per-deployment, so their roots don't
// collide — the content-addressed shards/fragments are safely shared. The name is a short,
// deterministic hash of (siteID, env) — both, so two sites that share a bucket and happen to use
// the same env name don't collide — and of the hash, not the names themselves, which would leak
// into the bucket/URLs. The fully-default case (no site, no env) keeps the bare "manifest.json".
func searchManifestName(siteID, env string) string {
	if siteID == "" && env == "" {
		return "manifest.json"
	}
	sum := sha256.Sum256([]byte(siteID + "\x00" + env)) // NUL-separated so (a,bc) != (ab,c)
	return "manifest-" + hex.EncodeToString(sum[:4]) + ".json"
}

// searchBaseURL is where the browser reader loads the index from. When _search/ is routed to an
// object store (so the index stays off a Pages-style file budget), it's that store's absolute URL;
// otherwise it's the local base_path. Because the reader fetches the manifest/shards/fragments
// *and* imports search.js from here, a routed (cross-origin) base requires the store to allow CORS
// GET from the site's origin — unlike routed <img> tags, fetch() and import() are not CORS-exempt.
func searchBaseURL(router *core.Router, basePath string) string {
	if routed := router.AssetURL(searchBase + "/manifest.json"); routed != "" {
		return strings.TrimSuffix(routed, "manifest.json")
	}
	return basePath + searchBase + "/"
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
	pages, _, _, err := buildPages(docs, opts.IncludeDrafts, now, basePath, site.BaseURL, site.Slides, site.Languages, defaultLang(site.Lang), router, nil, nil, false, nil)
	if err != nil {
		return nil, err
	}
	// CLI search is always typo-tolerant (the in-memory trigram map is cheap) — the agent surface
	// should be forgiving regardless of whether the public site enables fuzzy.
	return search.NewIndex(pagesToSearchDocs(pages, basePath), search.BuildOptions{Fuzzy: true})
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
