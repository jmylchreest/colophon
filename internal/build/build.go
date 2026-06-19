// Package build is the content pipeline: it reads markdown from content/, renders
// it through the theme engine, and writes the canonical static tree to public/.
//
// This is the M1 thin slice. Deliberately deferred (tracked for later M1+ work):
// incremental/content-hash skipping, feeds, publish_after embargo filtering, and
// multi-persona publications. Draft posts are already excluded from production builds.
package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/render"
	"github.com/jmylchreest/colophon/internal/source"
	"github.com/jmylchreest/colophon/internal/telemetry"
	"github.com/jmylchreest/colophon/markdown"
)

// Result summarises a build.
type Result struct {
	Pages  int
	OutDir string
	// NextEmbargo is the soonest future publish_after across non-draft content (the
	// next time a production build would reveal a new post), or nil if none pending.
	NextEmbargo *time.Time
}

// Options control a single build. They come from an Environment (or defaults).
type Options struct {
	// OutDir is where the canonical tree is written. Required.
	OutDir string
	// IncludeDrafts builds not-yet-public posts — drafts and embargoed (publish_after
	// in the future). Production builds leave both out; preview/serve include them.
	IncludeDrafts bool
	// Now is the build instant for embargo evaluation; zero means time.Now().
	Now time.Time
	// Title, BaseURL and Theme override the site's values when non-empty.
	Title   string
	BaseURL string
	Theme   string
	// BasePath prefixes every internal link (so output can be hosted under a subpath,
	// e.g. /<site>/<env>/ for serve, or /repo/ for project pages). Empty means derive
	// it from BaseURL's path; the result always starts and ends with "/".
	BasePath string
	// Publishers is the environment's deploy targets. It activates asset routing only for
	// routes whose target publisher is deploying (so a build/serve with no targets keeps
	// assets co-located). Empty means no routing.
	Publishers []string
	// Routes overrides the site's routing rules when non-nil — used by publish to supply
	// rules whose base_url has been resolved from the target publisher (e.g. a discovered
	// R2 public URL). Nil uses the site's rules as written.
	Routes []core.RouteRule
	// Log receives SOURCE/BUILD progress lines; nil silences them.
	Log *clog.Logger
	// Env is the environment name, used only as a telemetry label (may be "").
	Env string
	// Telemetry receives build/source/persona events. Nil (e.g. from serve) sends nothing,
	// so preview rebuilds never emit telemetry.
	Telemetry *telemetry.Client
}

// resolveBasePath picks the base path: an explicit value wins, else the path component
// of baseURL, else "/". It is normalised to start and end with a single "/".
func resolveBasePath(explicit, baseURL string) string {
	p := explicit
	if p == "" {
		if u, err := url.Parse(baseURL); err == nil {
			p = u.Path
		}
	}
	p = strings.Trim(p, "/")
	if p == "" {
		return "/"
	}
	return "/" + p + "/"
}

type page struct {
	Title        string
	Date         string
	Published    time.Time // raw frontmatter date, for feeds/sitemap
	Description  string    // frontmatter description or derived excerpt
	URL          string    // base_path-relative, e.g. posts/hello/
	Slug         string    // resolved slug (URL without the trailing slash), the series/wikilink key
	Out          string    // path under public/, e.g. posts/hello/index.html
	SourcePath   string    // origin file (for diagnostics, e.g. slug-collision warnings)
	HTML         string
	Draft        bool   // included only because this is a preview build
	Embargoed    bool   // included only because this is a preview build
	EmbargoUntil string // formatted publish_after, when Embargoed
	Static       bool   // standing page (e.g. About): kept out of the chronological list + feeds, surfaced in nav
	Type         string // page type (post|page|custom): selects the template and placement
	Lang         string // per-post language override (BCP-47); empty → the site language
	GlossaryOff  bool   // post opted out of glossary decoration (frontmatter glossary: false)

	Hero       string // hero banner URL: page-relative when co-located, absolute when routed
	HeroAbs    string // absolute hero URL, used as the og:image fallback when no image: is set
	HeroAlt    string // accessible alt text for the hero; empty → decorative (alt="")
	HeroFit    string // CSS object-fit for the hero (cover|contain|…); empty → theme default
	HeroPos    string // CSS object-position for the hero (e.g. "top"); empty → theme default
	Image      string // preview image href for the index card (rooted path or absolute), or ""
	ImageAlt   string // accessible alt text for the card image; empty → decorative (alt="")
	ImageFit   string // CSS object-fit for the card/preview image; empty → theme default
	ImagePos   string // CSS object-position for the card/preview image; empty → theme default
	ImageAbs   string // absolute preview image URL for og:image, or ""
	Tags       []string
	Categories []string
	Author     string        // author id from frontmatter (the byline)
	Persona    string        // persona id from frontmatter (the hidden writing voice)
	SEO        *markdown.SEO // optional search/social overrides

	HasMath    bool // page uses math — theme loads KaTeX only when true
	HasMermaid bool // page has a mermaid diagram — theme loads Mermaid only when true
	HasCode    bool // page has a code block — theme loads the highlighter only when true

	// Predecessor is the raw frontmatter `predecessor:` (a slug/filename of the immediately
	// preceding post); SeriesTitle is this post's own `series:` title (latest-wins across the
	// chain). Series, below, is the computed series state, set only for posts in a series of ≥2.
	Predecessor string
	SeriesTitle string
	Series      *seriesInfo
}

// Run builds the first configured site into opts.OutDir, applying any environment
// overrides carried by opts.
func Run(cfg *config.Config, opts Options) (Result, error) {
	if len(cfg.Sites) == 0 {
		return Result{}, fmt.Errorf("no sites configured")
	}
	if opts.OutDir == "" {
		return Result{}, fmt.Errorf("build: OutDir is required")
	}
	site := cfg.Sites[0]
	if opts.Title != "" {
		site.Title = opts.Title
	}
	if opts.BaseURL != "" {
		site.BaseURL = opts.BaseURL
	}
	if opts.Theme != "" {
		site.Theme = opts.Theme
	}
	outDir := opts.OutDir
	basePath := resolveBasePath(opts.BasePath, site.BaseURL)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	docs, err := gatherDocuments(cfg, opts.Log)
	if err != nil {
		return Result{}, err
	}
	// Routes: publish supplies fully-resolved rules via opts.Routes; a plain build resolves
	// an empty route base_url from the target publisher's configured public_url (no network).
	routes := opts.Routes
	if routes == nil {
		routes = resolveRoutesFromConfig(site.Routing, cfg)
	}
	router := core.NewRouter(routes, opts.Publishers)
	// Where the browser reader loads the search index from: the routed object-store URL when
	// _search/ is routed, else the local base_path. The manifest name is environment-specific so
	// several environments can share one bucket without their roots colliding. Both are
	// site-global and threaded into every template.
	searchURL := searchBaseURL(router, basePath)
	searchManifest := searchManifestName(site.ID, opts.Env)
	pages, assets, nextEmbargo, err := buildPages(docs, opts.IncludeDrafts, now, basePath, site.BaseURL, router)
	if err != nil {
		return Result{}, err
	}
	opts.Log.Step("BUILD", "", "pages", len(pages), "assets", len(assets), "drafts", opts.IncludeDrafts)

	eng, err := render.New(cfg.Root, site.Theme)
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return Result{}, err
	}

	// written records every output path so a post-build sweep can remove stale files
	// (deleted posts, renamed slugs) that no longer belong in the tree.
	written := make(map[string]struct{})
	write := func(rel string, b []byte) error {
		full := filepath.Join(outDir, rel)
		if err := writeFile(full, b); err != nil {
			return err
		}
		written[full] = struct{}{}
		return nil
	}

	formats := feedFormats(site)
	feedHead := feedDiscoveryLinks(site, formats)
	siteLang := defaultLang(site.Lang)

	favicon, err := writeFavicon(write, eng, cfg.Root, site)
	if err != nil {
		return Result{}, err
	}

	// analyticsListing is the per-page analytics markup for listing pages (index/tag/author —
	// no post dimensions), computed once and threaded through like feedHead. Site analytics is
	// independent of the app's telemetry switch; it is gated by each provider's own config.
	analyticsListing := analyticsHead(site.Analytics, basePath, nil)

	// Bundle each enabled analytics provider's client asset to the site root — and only the
	// enabled one(s).
	if err := emitAnalyticsAssets(write, site.Analytics); err != nil {
		return Result{}, err
	}

	// The glossary ships only where it's used: each page is scanned for terms below, and the
	// data + decorator are emitted after the loop only if some page actually references one.
	glossRE := glossaryMatcher(cfg.Glossary)
	anyGlossary := false

	// Dateless pages (About, Now, …) are standing chrome, not dated posts: they surface in
	// the nav menu rather than the chronological list/feeds. Posts drive the list, tags,
	// authors and feeds; every page (post or static) still renders its own document.
	navPages := navLinks(pages, basePath)

	// Reconstruct post series from backward predecessor links, attaching each member's computed
	// series state to its page (in place, so the posts copies + list maps below carry it). Only
	// chronological posts participate; standing pages are skipped.
	seriesPosts := make([]*page, 0, len(pages))
	for i := range pages {
		if !pages[i].Static {
			seriesPosts = append(seriesPosts, &pages[i])
		}
	}
	computeSeries(seriesPosts, opts.Log)

	posts := make([]page, 0, len(pages))
	for _, p := range pages {
		if !p.Static {
			posts = append(posts, p)
		}
	}

	// Index-item maps for posts and the author strip (one avatar per persona that wrote a
	// post, most-recent-first) are built up front so every page's header can show the strip.
	list := make([]map[string]any, len(posts))
	for i, p := range posts {
		list[i] = map[string]any{"title": p.Title, "url": p.URL, "date": p.Date, "type": p.Type, "draft": p.Draft, "embargoed": p.Embargoed, "embargo_until": p.EmbargoUntil, "image": p.Image, "image_alt": p.ImageAlt, "image_style": imageStyle(p.ImageFit, p.ImagePos), "tags": tagLinks(p.Tags, basePath), "series": seriesListName(&posts[i])}
	}
	// Publish file-path author avatars through the same asset pipeline as markdown embeds,
	// emitting a depth-independent src (the topbar strip is built once for every page depth).
	// This mutates cfg.Authors, so collectAuthors and authorVars below both read the resolved
	// value. Dedup against the markdown refs already collected (keyed by output path).
	seenAssets := make(map[string]bool, len(assets))
	for _, a := range assets {
		seenAssets[a.outPath] = true
	}
	assets = append(assets, resolveAuthorAvatars(cfg, uniqueSources(docs), router, basePath, seenAssets, opts.Log)...)

	authorGroups := collectAuthors(cfg, posts, list, basePath)
	authors := authorStrip(authorGroups)

	// slugSeen guards against two entries resolving to the same slug (URL/output path). A
	// collision would silently overwrite one post, so it is warned with both source files.
	slugSeen := map[string]string{}
	for _, p := range pages {
		if isEmptyContent(p.HTML) {
			opts.Log.Step("BUILD", "", "warn", fmt.Sprintf("post %q has no content", strings.TrimSuffix(p.URL, "/")))
		}
		if first, dup := slugSeen[p.URL]; dup {
			opts.Log.Step("BUILD", "", "warn", fmt.Sprintf("slug %q is produced by both %q and %q — only one survives; set a distinct slug:",
				strings.TrimSuffix(p.URL, "/"), first, p.SourcePath))
		} else {
			slugSeen[p.URL] = p.SourcePath
		}
		author := resolveAuthor(cfg, p.Author)
		pageLang := p.Lang
		if pageLang == "" {
			pageLang = siteLang
		}
		pageGlossary := ""
		if pageNeedsGlossary(p.HTML, p.GlossaryOff, glossRE) {
			anyGlossary = true
			// glossary: false → still load the decorator (so <dfn> can force a term) but
			// with auto-matching off.
			pageGlossary = glossaryHeadTag(basePath, !p.GlossaryOff)
		}
		ctx := map[string]any{
			"lang":            pageLang,
			"nav_pages":       navPages,
			"authors":         authors,
			"site_title":      site.Title,
			"base_url":        site.BaseURL,
			"base_path":       basePath,
			"feed_head":       feedHead,
			"analytics_head":  analyticsHead(site.Analytics, basePath, &p),
			"glossary_head":   pageGlossary,
			"seo_head":        seoHead(site, p, author),
			"meta_title":      metaTitle(p),
			"favicon":         favicon,
			"title":           p.Title,
			"date":            p.Date,
			"description":     p.Description,
			"content":         p.HTML,
			"draft":           p.Draft,
			"embargoed":       p.Embargoed,
			"embargo_until":   p.EmbargoUntil,
			"hero":            p.Hero,
			"hero_alt":        p.HeroAlt,
			"hero_style":      imageStyle(p.HeroFit, p.HeroPos),
			"image":           p.Image,
			"image_alt":       p.ImageAlt,
			"image_style":     imageStyle(p.ImageFit, p.ImagePos),
			"image_abs":       p.ImageAbs,
			"tags":            tagLinks(p.Tags, basePath),
			"category":        pageCategory(p),
			"read_time":       readingTime(p.HTML),
			"toc":             tableOfContents(p.HTML),
			"has_math":        p.HasMath,
			"has_mermaid":     p.HasMermaid,
			"has_code":        p.HasCode,
			"page_type":       p.Type,
			"search":          searchEnabled(site),
			"search_base":     searchURL,
			"search_manifest": searchManifest,
		}
		for k, v := range authorVars(author) {
			ctx[k] = v
		}
		// Series nav (set only when this post is in a series of ≥2). Left unset otherwise, so a
		// theme's {% if series_parts %} stays false.
		for k, v := range seriesVars(&p, basePath) {
			ctx[k] = v
		}
		html, err := eng.Render(templateFor(eng, p.Type), ctx)
		if err != nil {
			return Result{}, err
		}
		if err := write(p.Out, []byte(html)); err != nil {
			return Result{}, err
		}
	}

	// Emit the glossary data + decorator only if some page actually referenced a term, so a
	// glossary.yaml with no matching content (or no glossary at all) ships nothing.
	if anyGlossary {
		if _, err := emitGlossary(write, cfg); err != nil {
			return Result{}, err
		}
	}

	chrome := listingChrome{
		site: site,
		lang: siteLang, siteTitle: site.Title, baseURL: site.BaseURL, basePath: basePath,
		description: site.Description, shareImage: siteShareImage(site),
		feedHead: feedHead, analyticsHead: analyticsListing, favicon: favicon,
		search: searchEnabled(site), searchBase: searchURL, searchManifest: searchManifest,
		authors: authors, navPages: navPages,
	}

	index, err := chrome.render(eng, site.Title, list, map[string]any{
		"feeds":    feedLinks(formats, basePath),
		"seo_head": chrome.seoHead("", site.Title, true),
	})
	if err != nil {
		return Result{}, err
	}
	if err := write("index.html", index); err != nil {
		return Result{}, err
	}

	// Tag pages: one post listing per tag, reusing the index template with a heading. Tag
	// chips on each post (page.html) link here, so tags become cross-entry navigation.
	if err := writeTagPages(write, eng, chrome, posts, list); err != nil {
		return Result{}, err
	}

	// Author pages: one post listing per persona at authors/<id>/, reached from the avatar
	// widget. Same index template + heading, mirroring tag pages.
	if err := writeAuthorPages(write, eng, chrome, authorGroups); err != nil {
		return Result{}, err
	}

	// Copy the theme's static files (style.css, vendored JS/fonts, etc.) verbatim.
	themeAssets, err := eng.Assets()
	if err != nil {
		return Result{}, err
	}
	for _, name := range themeAssets {
		b, err := eng.Asset(name)
		if err != nil {
			return Result{}, err
		}
		if err := write(name, b); err != nil {
			return Result{}, err
		}
	}

	if err := writeFeeds(write, site, cfg, formats, pages); err != nil {
		return Result{}, err
	}
	opts.Log.Detail("BUILD", "", "feeds", strings.Join(formats, " "), "sitemap", true, "robots", true)

	// Copy referenced assets through their owning source. A missing asset warns (a likely
	// broken link) rather than failing the whole build.
	ctx := context.Background()
	for _, a := range assets {
		rc, err := a.src.Open(ctx, a.srcPath)
		if err != nil {
			opts.Log.Step("ASSET", a.src.ID(), "missing", a.srcPath)
			continue
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return Result{}, err
		}
		if err := write(a.outPath, b); err != nil {
			return Result{}, err
		}
		opts.Log.Detail("ASSET", a.src.ID(), "file", a.outPath, "bytes", len(b))
	}

	if err := writeSearchIndex(write, pages, site, basePath, opts.Env, opts.Log); err != nil {
		return Result{}, err
	}

	if err := sweep(outDir, written); err != nil {
		return Result{}, err
	}
	emitBuildTelemetry(opts.Telemetry, site, docs, pages)
	return Result{Pages: len(pages), OutDir: outDir, NextEmbargo: nextEmbargo}, nil
}

// sourceDoc pairs a document with the source it came from, so the build can resolve the
// document's assets (relative image refs) back through that source.
type sourceDoc struct {
	doc core.Content
	src core.Source
}

// gatherDocuments collects the content documents from every configured source (or the
// default md-dir at content/ when none is configured), logging each.
func gatherDocuments(cfg *config.Config, log *clog.Logger) ([]sourceDoc, error) {
	srcs, err := resolveSources(cfg)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	var docs []sourceDoc
	for _, s := range srcs {
		got, err := s.Documents(ctx)
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", s.ID(), err)
		}
		log.Step("SOURCE", s.ID(), "driver", s.Driver(), "docs", len(got))
		if w, ok := s.(core.Warner); ok {
			for _, m := range w.Warnings() {
				log.Step("SOURCE", s.ID(), "warn", m)
			}
		}
		for _, d := range got {
			log.Detail("SOURCE", s.ID(), "file", d.SourcePath)
			docs = append(docs, sourceDoc{doc: d, src: s})
		}
	}
	return docs, nil
}

func resolveSources(cfg *config.Config) ([]core.Source, error) {
	if len(cfg.Sources) == 0 {
		s, err := source.Open(cfg.Root, config.SourceConfig{ID: "content", Driver: "md-dir"})
		if err != nil {
			return nil, err
		}
		return []core.Source{s}, nil
	}
	out := make([]core.Source, 0, len(cfg.Sources))
	for _, sc := range cfg.Sources {
		s, err := source.Open(cfg.Root, sc)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// included is a document that survived the draft/embargo filter, with its resolved slug.
type included struct {
	c         core.Content
	src       core.Source
	slug      string
	embargoed bool
}

// assetRef is a file an output page references and must carry along: read srcPath from
// src, write it to outPath (relative to the output dir).
type assetRef struct {
	src     core.Source
	srcPath string
	outPath string
}

// buildPages renders the gathered documents to pages sorted newest-first, the assets
// they reference, and the soonest pending embargo. A document is skipped from production
// (includeDrafts false) when it is a draft or its publish_after is still in the future;
// preview builds include both, marking embargoed ones. Two passes so wikilinks resolve
// against every other document's URL.
func buildPages(docs []sourceDoc, includeDrafts bool, now time.Time, basePath, baseURL string, router *core.Router) ([]page, []assetRef, *time.Time, error) {
	var items []included
	var next *time.Time
	links := linkResolver{}
	for _, sd := range docs {
		c := sd.doc
		fm := c.Frontmatter
		next = considerEmbargo(next, fm, now)

		embargoed := fm.PublishAfter != nil && now.Before(*fm.PublishAfter)
		if !includeDrafts && (fm.Draft || embargoed) {
			continue
		}
		slug := slugFor(c.SourcePath, fm.Slug)
		links.add(c.SourcePath, slug, basePath)
		items = append(items, included{c: c, src: sd.src, slug: slug, embargoed: embargoed})
	}

	// Collect referenced assets, co-located beside their page so the relative ref still
	// resolves: a ref in posts/hello.md becomes a file under the posts/hello/ output dir.
	var assets []assetRef
	seen := map[string]bool{}
	addAsset := func(it included, ref string) {
		if !localRef(ref) {
			return
		}
		out := path.Clean(path.Join(it.slug, ref))
		if seen[out] {
			return
		}
		seen[out] = true
		assets = append(assets, assetRef{
			src:     it.src,
			srcPath: path.Clean(path.Join(path.Dir(it.c.SourcePath), ref)),
			outPath: out,
		})
	}
	for _, it := range items {
		for _, r := range docRefs(it.c) {
			addAsset(it, r.Ref)
		}
	}

	md := sharedMarkdown

	pages := make([]page, 0, len(items))
	for _, it := range items {
		fm := it.c.Frontmatter
		var buf bytes.Buffer
		body := preprocessCallouts(resolveWikilinks(rewriteAssetURLs(it.c.Body, it.slug, router), links))
		if err := md.Convert([]byte(body), &buf); err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", it.c.SourcePath, err)
		}

		title := fm.Title
		if title == "" {
			title = it.slug
		}
		html := buf.String()
		desc := fm.Description
		if desc == "" {
			desc = excerpt(html, 200)
		}
		pageType := resolvePageType(fm)
		p := page{
			Title:       title,
			Date:        formatDate(fm.Date),
			Published:   fm.Date,
			Description: desc,
			URL:         it.slug + "/", // base_path-relative; templates prepend base_path
			Slug:        it.slug,
			Out:         filepath.Join(filepath.FromSlash(it.slug), "index.html"),
			SourcePath:  it.c.SourcePath,
			HTML:        html,
			Draft:       fm.Draft,
			Embargoed:   it.embargoed,
			Type:        pageType,
			Static:      standingType(pageType), // standing types (page) → nav; posts/custom → list
		}
		if it.embargoed {
			p.EmbargoUntil = fm.PublishAfter.Format("2006-01-02 15:04 MST")
		}
		if localRef(fm.Hero) {
			out := path.Clean(path.Join(it.slug, fm.Hero))
			if url := router.AssetURL(out); url != "" {
				p.Hero, p.HeroAbs = url, url // served from the object store (already absolute)
			} else {
				p.Hero = fm.Hero // co-located beside the page, page-relative
				p.HeroAbs = absURL(baseURL, out)
			}
		}
		if localRef(fm.Image) {
			out := path.Clean(path.Join(it.slug, fm.Image))
			if url := router.AssetURL(out); url != "" {
				p.Image, p.ImageAbs = url, url
			} else {
				p.Image, p.ImageAbs = basePath+out, absURL(baseURL, out)
			}
		}
		p.Lang = fm.Lang
		p.GlossaryOff = fm.Glossary != nil && !*fm.Glossary
		p.HeroAlt, p.ImageAlt = fm.HeroAlt, fm.ImageAlt
		p.HeroFit, p.HeroPos = fm.HeroFit, fm.HeroPosition
		p.ImageFit, p.ImagePos = fm.ImageFit, fm.ImagePosition
		p.HasMermaid = strings.Contains(html, `class="mermaid"`)
		p.HasMath = strings.Contains(html, `class="math`)
		p.HasCode = strings.Contains(html, "<pre><code")
		p.Tags = fm.Tags
		p.Categories = fm.Categories
		p.Author = fm.Author
		p.Persona = fm.Persona
		p.SEO = fm.SEO
		p.Predecessor = fm.Predecessor
		p.SeriesTitle = fm.Series
		pages = append(pages, p)
	}

	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Date > pages[j].Date })
	return pages, assets, next, nil
}

// resolvePersona returns the persona a document is written as: an explicit `persona:` wins, else
// the persona of its first publication (the write-as target), else "".
func resolvePersona(fm markdown.Frontmatter) string {
	if fm.Persona != "" {
		return fm.Persona
	}
	if len(fm.Publications) > 0 {
		return fm.Publications[0].Persona
	}
	return ""
}

// resolvePageType returns a page's type: an explicit frontmatter `type:` wins, else it is
// derived from the presence of a date — dated → "post", dateless → "page". The date heuristic
// stays the default and `type:` overrides it.
func resolvePageType(fm markdown.Frontmatter) string {
	if t := strings.ToLower(strings.TrimSpace(fm.Type)); t != "" {
		return t
	}
	if fm.Date.IsZero() {
		return "page"
	}
	return "post"
}

// standingType reports whether a page type is standing chrome (the nav menu) rather than a
// chronological post (list, feeds, tags). The built-in "page" is standing; "post" and any
// custom type are listed.
func standingType(t string) bool { return t == "page" }

// templateFor picks the per-page-type template: the theme's "<type>.html" if it provides one,
// else the default single-entry template "page.html".
func templateFor(eng render.Engine, pageType string) string {
	if name := pageType + ".html"; eng.HasTemplate(name) {
		return name
	}
	return "page.html"
}

var imageRE = regexp.MustCompile(`!\[[^\]]*\]\(\s*(?:<([^>]+)>|([^)\s]+))`)

// imageRefs returns the link destinations of markdown image syntax (![alt](dest)). The
// <…> destination form is recognised so refs with spaces survive. Obsidian ![[embed]] is
// rewritten to this form by the obsidian source before the build sees it.
func imageRefs(body string) []string {
	matches := imageRE.FindAllStringSubmatch(body, -1)
	refs := make([]string, 0, len(matches))
	for _, m := range matches {
		ref := m[1]
		if ref == "" {
			ref = m[2]
		}
		refs = append(refs, ref)
	}
	return refs
}

// docRef is one local file a document points at, tagged with what role it plays (for
// diagnostics). Ref is the reference exactly as written — callers apply localRef themselves.
type docRef struct{ Kind, Ref string }

// docRefs returns every file a document references — its markdown image embeds and its
// hero/image frontmatter. It is the single source of truth for "what does this post point
// at", shared by asset publishing (buildPages) and doctor's preflight (MissingAssets) so the
// two can't drift as new reference-bearing fields are added.
func docRefs(doc core.Content) []docRef {
	fm := doc.Frontmatter
	embeds := imageRefs(doc.Body)
	refs := make([]docRef, 0, len(embeds)+2)
	refs = append(refs, docRef{Kind: "hero", Ref: fm.Hero}, docRef{Kind: "image", Ref: fm.Image})
	for _, r := range embeds {
		refs = append(refs, docRef{Kind: "embed", Ref: r})
	}
	return refs
}

var imageRewriteRE = regexp.MustCompile(`(!\[[^\]]*\]\()\s*(?:<([^>]+)>|([^)\s]+))`)

// rewriteAssetURLs rewrites local image destinations that a route binds to an object store
// so they point at the store's absolute public URL (e.g. https://assets.example.com/…)
// instead of the co-located relative path. Unrouted and external refs are left untouched.
func rewriteAssetURLs(body, slug string, router *core.Router) string {
	if !router.Active() {
		return body
	}
	return imageRewriteRE.ReplaceAllStringFunc(body, func(m string) string {
		sub := imageRewriteRE.FindStringSubmatch(m)
		dest := sub[2]
		if dest == "" {
			dest = sub[3]
		}
		if !localRef(dest) {
			return m
		}
		if url := router.AssetURL(path.Clean(path.Join(slug, dest))); url != "" {
			return sub[1] + "<" + url + ">"
		}
		return m
	})
}

// localRef reports whether a ref is a relative path to copy (not external, root-absolute,
// or a fragment).
func localRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "#") {
		return false
	}
	if i := strings.IndexByte(ref, ':'); i >= 0 && i < strings.IndexByte(ref+"/", '/') {
		return false // has a scheme (http:, data:, mailto:)
	}
	return true
}

// considerEmbargo returns the sooner of next and fm's publish_after, considering only
// non-draft posts whose embargo is still in the future. Drafts are ignored because
// they stay gated regardless of time, so they never trigger an automatic build.
func considerEmbargo(next *time.Time, fm markdown.Frontmatter, now time.Time) *time.Time {
	if fm.Draft || fm.PublishAfter == nil {
		return next
	}
	pa := *fm.PublishAfter
	if !pa.After(now) {
		return next
	}
	if next == nil || pa.Before(*next) {
		return &pa
	}
	return next
}

// NextEmbargo returns the soonest future publish_after across non-draft documents from
// every configured source — the next instant a production build would publish something
// new — or nil if nothing is pending.
func NextEmbargo(cfg *config.Config, now time.Time) (*time.Time, error) {
	docs, err := gatherDocuments(cfg, nil)
	if err != nil {
		return nil, err
	}
	var next *time.Time
	for _, sd := range docs {
		next = considerEmbargo(next, sd.doc.Frontmatter, now)
	}
	return next, nil
}

// slugFor derives a clean site path from a content-relative file path, honouring an
// explicit frontmatter slug for the final segment. The .md extension is dropped, a
// trailing /index is collapsed (foo/index.md -> foo), folder structure is preserved,
// and every segment is normalised (lower-case, spaces/punctuation -> single hyphens)
// so "Archive/My Post.md" -> "archive/my-post".
func slugFor(rel, override string) string {
	s := filepath.ToSlash(rel)
	s = strings.TrimSuffix(s, filepath.Ext(s))
	s = strings.TrimSuffix(s, "/index")
	if override != "" {
		if dir := pathDir(s); dir != "" {
			s = dir + "/" + override
		} else {
			s = override
		}
	}
	return normalizeSlug(s)
}

// normalizeSlug slugifies each path segment and drops empties.
func normalizeSlug(s string) string {
	var out []string
	for _, seg := range strings.Split(s, "/") {
		if seg = slugifySegment(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// slugifySegment lower-cases a single segment, keeping [a-z0-9] and collapsing any run
// of other characters to a single hyphen, with hyphens trimmed from the ends.
func slugifySegment(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			hyphen = false
		} else if !hyphen && b.Len() > 0 {
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func pathDir(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[:i]
	}
	return ""
}

// tagLinks maps a page's tags to {name, url} entries pointing at their tag pages.
func tagLinks(tags []string, basePath string) []map[string]any {
	out := make([]map[string]any, 0, len(tags))
	for _, t := range tags {
		if s := normalizeSlug(t); s != "" {
			out = append(out, map[string]any{"name": t, "url": basePath + "tags/" + s + "/"})
		}
	}
	return out
}

// writeTagPages renders a listing page per tag at tags/<slug>/, reusing the index template
// (with a heading and the tag's posts). list[i] is the index-item map for pages[i].
// listingChrome is the site-wide context every listing page (home, per-tag, per-author) renders
// the index template with. Only the heading and the post list vary between them, so bundling the
// shared chrome here keeps that 13-key map in one place and spares the writers a long, position-
// sensitive parameter list.
type listingChrome struct {
	site                               core.Site
	lang, siteTitle, baseURL, basePath string
	description, shareImage            string // site tagline + absolute default share image (og:)
	feedHead, analyticsHead, favicon   string
	search                             bool
	searchBase, searchManifest         string
	authors, navPages                  []map[string]any
}

// seoHead builds the SEO <head> block for one listing page. urlPath is the page's site-root-
// relative path ("" for the home page, "tags/go/" for a tag index); title is its og:title
// (the site title for home, the listing heading otherwise); home selects the Blog vs
// CollectionPage JSON-LD. The canonical URL is the site base joined with urlPath.
func (c listingChrome) seoHead(urlPath, title string, home bool) string {
	canonical := strings.TrimRight(c.baseURL, "/") + "/" + strings.TrimLeft(urlPath, "/")
	return listingSEOHead(c.site, canonical, title, c.description, c.shareImage, home)
}

// render renders the index template for one listing page: the shared chrome plus this page's
// heading and posts, overlaid with any extra keys (the home page's feed links).
func (c listingChrome) render(eng render.Engine, heading string, pages []map[string]any, extra map[string]any) ([]byte, error) {
	ctx := map[string]any{
		"lang":            c.lang,
		"site_title":      c.siteTitle,
		"base_url":        c.baseURL,
		"base_path":       c.basePath,
		"feed_head":       c.feedHead,
		"analytics_head":  c.analyticsHead,
		"favicon":         c.favicon,
		"heading":         heading,
		"tagline":         c.site.Tagline,
		"authors":         c.authors,
		"nav_pages":       c.navPages,
		"pages":           pages,
		"search":          c.search,
		"search_base":     c.searchBase,
		"search_manifest": c.searchManifest,
	}
	for k, v := range extra {
		ctx[k] = v
	}
	html, err := eng.Render("index.html", ctx)
	if err != nil {
		return nil, err
	}
	return []byte(html), nil
}

func writeTagPages(write func(string, []byte) error, eng render.Engine, chrome listingChrome, pages []page, list []map[string]any) error {
	type group struct {
		name  string
		items []map[string]any
	}
	groups := map[string]*group{}
	var slugs []string
	for i, p := range pages {
		for _, t := range p.Tags {
			s := normalizeSlug(t)
			if s == "" {
				continue
			}
			g := groups[s]
			if g == nil {
				g = &group{name: t}
				groups[s] = g
				slugs = append(slugs, s)
			}
			g.items = append(g.items, list[i])
		}
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		g := groups[s]
		heading := "Tagged “" + g.name + "”"
		html, err := chrome.render(eng, heading, g.items, map[string]any{
			"seo_head": chrome.seoHead("tags/"+s+"/", heading, false),
		})
		if err != nil {
			return err
		}
		if err := write("tags/"+s+"/index.html", html); err != nil {
			return err
		}
	}
	return nil
}

// writeFavicon copies the site icon into the output and returns its filename for the theme
// to link, or "" if none is available. A project-root favicon (site.Favicon) wins; else the
// theme's favicon.svg is used. The output keeps the source extension so the browser can
// infer the type.
func writeFavicon(write func(string, []byte) error, eng render.Engine, root string, site core.Site) (string, error) {
	if site.Favicon != "" {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(site.Favicon)))
		if err != nil {
			return "", fmt.Errorf("favicon %s: %w", site.Favicon, err)
		}
		name := "favicon" + strings.ToLower(path.Ext(site.Favicon))
		return name, write(name, b)
	}
	b, err := eng.Asset("favicon.svg")
	if err != nil {
		return "", nil // theme ships no default favicon; skip the link
	}
	return "favicon.svg", write("favicon.svg", b)
}

// resolveRoutesFromConfig fills an empty route base_url from its target publisher's
// configured public_url, so a route can omit base_url and inherit the store's URL. Publish
// resolves further (provider discovery); this is the config-only fallback a plain build uses.
func resolveRoutesFromConfig(routes []core.RouteRule, cfg *config.Config) []core.RouteRule {
	if len(routes) == 0 {
		return routes
	}
	out := make([]core.RouteRule, len(routes))
	for i, r := range routes {
		out[i] = r
		if r.BaseURL != "" {
			continue
		}
		if pc := cfg.Publisher(r.Publisher); pc != nil {
			if u, _ := pc.Settings["public_url"].(string); u != "" {
				out[i].BaseURL = u
			}
		}
	}
	return out
}

// absURL joins a base_url root and a base_path-relative page path into an absolute URL
// (for og:image and similar), returning "" when there is no path.
func absURL(base, p string) string {
	if p == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(p, "/")
}

// siteShareImage returns the absolute default social-share image for listing pages: site.Image
// unchanged when it is already absolute, else resolved against base_url, else "".
func siteShareImage(site core.Site) string {
	if isAbsURL(site.Image) {
		return site.Image
	}
	return absURL(site.BaseURL, site.Image)
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// sweep deletes every file under root that this build did not write, then removes any
// directories left empty. It reconciles the output tree to exactly match the inputs,
// so deleting a post or renaming a slug never leaves an orphan behind. root is always a
// colophon-owned output dir (public/ or .colophon/...), so this only removes our files.
func sweep(root string, keep map[string]struct{}) error {
	var stale []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := keep[p]; !ok {
			stale = append(stale, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := removeFiles(stale); err != nil {
		return err
	}
	return removeEmptyDirs(root)
}

// removeFiles unlinks paths with bounded parallelism. A cold rebuild that renamed a slug on
// a large site can orphan thousands of files; overlapping the unlinks hides per-call I/O
// latency. A small batch stays sequential to avoid goroutine overhead. The first error wins.
func removeFiles(paths []string) error {
	const workers = 8
	if len(paths) < 2*workers {
		for _, p := range paths {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
		return nil
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for _, p := range paths {
		sem <- struct{}{}
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := os.Remove(p); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	return firstErr
}

// ReconcileDirs removes any immediate subdirectory of base whose name is not in keep.
// It is how an orchestrator drops the build-output trees of deleted targets (e.g. a
// removed environment). A missing base is not an error. It only ever touches dirs that
// are purely build output — never durable scratch like cache/ or corpus/.
func ReconcileDirs(base string, keep map[string]bool) error {
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && !keep[e.Name()] {
			if err := os.RemoveAll(filepath.Join(base, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Deepest first, so a dir is removed only after its (now-empty) children.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		if entries, _ := os.ReadDir(d); len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
	return nil
}
