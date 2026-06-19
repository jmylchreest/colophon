// Package obsidian implements the "obsidian" source: one or more Obsidian vault folders,
// read in place (no copy). Which notes become blog posts is a combination of two filters:
//
//   - Location — `path` is one folder or a list of folders to scan (the union; OR). When a
//     `vault` is set, relative paths resolve under it; with no `path`, the vault root is the
//     single scan location.
//   - Tag — `tag` is one tag or a list (a note matches any; OR), in frontmatter `tags:` or as
//     an inline `#tag`, Forestry/"digital garden" style. When both `path` and `tag` are set a
//     note must satisfy both (a tagged note under one of the paths; AND). With no tags, the
//     Obsidian `publish: true` whitelist gates instead (`publish_required`, default true;
//     `false` keeps every note). A tag, when set, replaces that gate, though an explicit
//     `publish: false` still opts a note out.
//
// A tag-selected note that lacks the blog structure (a title and some body) is warned about
// and skipped. The folder structure maps onto the site, and deletes/renames flow through the
// normal build reconciliation. Wikilinks ([[note]]) resolve later in the build (across every
// source); here the source resolves attachment embeds (![[image.png]]) and hero/image
// frontmatter into paths the build's asset pipeline copies. Attachments resolve by name
// across the vault (or, with no vault, across the union of the scanned paths).
package obsidian

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/source"
	"github.com/jmylchreest/colophon/internal/source/mddir"
	"github.com/jmylchreest/colophon/markdown"
)

func init() { source.Register("obsidian", New) }

// New builds an obsidian source anchored on one `vault`. `path` (scalar or list) selects
// sub-folder(s) within the vault to scan; with none, the whole vault is scanned. `tag`
// (scalar or list) selects notes carrying any of those Obsidian tags; with none, the
// `publish: true` whitelist gates (`publish_required`, default true). An empty `vault` (e.g.
// an unset `{env:VAR:-}`) yields no documents, so an env-driven optional source stays inert.
func New(root string, cfg config.SourceConfig) (core.Source, error) {
	vault := resolveDir(root, str(cfg.Settings["vault"]))

	s := &Source{id: cfg.ID, vault: vault, publishRequired: true}
	if v, ok := cfg.Settings["publish_required"].(bool); ok {
		s.publishRequired = v
	}
	for _, t := range strList(cfg.Settings["tag"]) {
		if n := normalizeTag(t); n != "" {
			s.tags = append(s.tags, n)
		}
	}
	if vault == "" {
		return s, nil // inert optional source
	}
	for _, p := range strList(cfg.Settings["path"]) {
		// A blog path is always relative to the vault; a leading "/" (vault-root style, as
		// Obsidian writes internal paths) is allowed and ignored, so "/Blog/x" == "Blog/x".
		if rel := strings.TrimPrefix(p, "/"); rel != "" {
			s.scanRoots = append(s.scanRoots, filepath.Join(vault, filepath.FromSlash(rel)))
		}
	}
	if len(s.scanRoots) == 0 {
		s.scanRoots = []string{vault}
	}
	return s, nil
}

// resolveDir expands ~ and makes a configured path absolute relative to base. An empty
// value (e.g. an unset `{env:VAR:-}`) stays empty.
func resolveDir(base, p string) string {
	p = expandHome(strings.TrimSpace(p))
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

type Source struct {
	id              string
	vault           string   // the vault root (absolute); empty → inert source
	scanRoots       []string // folders within the vault to scan (absolute); union (OR)
	tags            []string // normalized publish tags; a note matches any (OR). Empty → no tag filter
	publishRequired bool     // when no tags: keep only `publish: true` notes
	warnings        []string
}

func (s *Source) ID() string     { return s.id }
func (s *Source) Driver() string { return "obsidian" }

// Warnings reports non-fatal problems found during the last Documents call (e.g. a note
// matched by more than one path). It satisfies core.Warner.
func (s *Source) Warnings() []string { return s.warnings }

// Resolve finds a source-relative ref (a note or asset path, possibly with "../" reaching
// elsewhere in the vault) against each scan root then the vault root, returning the first that
// stats as a file — without reading it.
func (s *Source) Resolve(ctx context.Context, ref string) (string, bool) {
	roots := append(append([]string{}, s.scanRoots...), s.vault)
	for _, root := range roots {
		if root == "" {
			continue
		}
		p := filepath.Join(root, filepath.FromSlash(ref))
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	return "", false
}

// Open returns the bytes of a ref, reading from the location Resolve found.
func (s *Source) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	p, ok := s.Resolve(ctx, ref)
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.Open(p)
}

func (s *Source) warnf(format string, args ...any) {
	s.warnings = append(s.warnings, fmt.Sprintf(format, args...))
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func str(v any) string { s, _ := v.(string); return s }

// strList reads a config value that may be a scalar string or a list of strings, and also
// splits comma-separated values, trimming blanks. So `tag: blog`, `tag: [blog, essay]`, and a
// `tag: "{env:TAGS}"` that interpolates to "blog,essay" all yield the same list — letting a
// single environment variable feed a list-valued field.
func strList(v any) []string {
	var out []string
	addCSV := func(s string) {
		for _, part := range strings.Split(s, ",") {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	switch x := v.(type) {
	case string:
		addCSV(x)
	case []string:
		for _, e := range x {
			addCSV(e)
		}
	case []any:
		for _, e := range x {
			if s, ok := e.(string); ok {
				addCSV(s)
			}
		}
	}
	return out
}

func (s *Source) Documents(ctx context.Context) ([]core.Content, error) {
	s.warnings = nil
	if s.vault == "" || len(s.scanRoots) == 0 {
		return nil, nil // inert optional source
	}
	idx := newAttachmentIndex(s.vault, s.scanRoots)
	keep := s.frontmatterKeep()
	seen := map[string]bool{}
	var docs []core.Content
	for _, root := range s.scanRoots {
		if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
			s.warnf("scan path %q does not exist in the vault — nothing published from it (check vault/path)", root)
			continue
		}
		walked, err := mddir.Walk(root, keep)
		if err != nil {
			return nil, err
		}
		for i := range walked {
			d := walked[i]
			if !s.selected(d) {
				continue
			}
			if seen[d.SourcePath] {
				s.warnf("note %q is matched by more than one path — keeping the first", d.SourcePath)
				continue
			}
			seen[d.SourcePath] = true
			s.fillDefaults(&d, root)
			s.resolveEmbeds(&d, root, idx)
			docs = append(docs, d)
		}
	}
	return docs, nil
}

// frontmatterKeep returns a frontmatter-only pre-filter for mddir.Walk when selection can be
// decided without the body, so a non-matching note skips the body-string copy. In tag mode
// selection can depend on inline body tags, so it returns nil (the body is needed) and
// selected() does the full check. The publish-whitelist mode is decidable on frontmatter.
func (s *Source) frontmatterKeep() func(markdown.Frontmatter) bool {
	if len(s.tags) > 0 {
		return nil
	}
	if s.publishRequired {
		return func(fm markdown.Frontmatter) bool {
			return fm.Publish != nil && *fm.Publish
		}
	}
	return nil // no tags, publish not required: keep everything
}

// selected reports whether a note is intended for the blog — the source's only filtering
// concern. With tags configured a note must carry one of them (unless opted out with
// `publish: false`); without tags the `publish: true` whitelist applies per publishRequired.
// Whether a selected note is well-formed (has content) is the build's concern, not ours.
func (s *Source) selected(c core.Content) bool {
	if len(s.tags) > 0 {
		if !noteHasAnyTag(c, s.tags) {
			return false
		}
		return c.Frontmatter.Publish == nil || *c.Frontmatter.Publish
	}
	if s.publishRequired {
		return c.Frontmatter.Publish != nil && *c.Frontmatter.Publish
	}
	return true
}

var embedRE = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)

// attachmentIndex resolves Obsidian attachment names (base name, lower-cased) to absolute
// paths. It indexes the post scan roots eagerly and the rest of the vault only on demand:
// most embeds live alongside the posts, so the full-vault walk is usually skipped entirely;
// when it is needed it runs at most once. When a name exists both under a scan root and
// elsewhere, the scan-root copy wins (attachments near the posts take precedence).
type attachmentIndex struct {
	vault     string
	scoped    map[string]string // base name → abs path, from the scan roots (eager)
	full      map[string]string // base name → abs path, whole vault (lazy)
	fullBuilt bool
}

func newAttachmentIndex(vault string, scanRoots []string) *attachmentIndex {
	ai := &attachmentIndex{vault: vault, scoped: indexAttachments(scanRoots)}
	// If the scan already covers the whole vault, the scoped index is the full index —
	// skip the redundant second walk on a miss.
	if len(scanRoots) == 1 && scanRoots[0] == vault {
		ai.full, ai.fullBuilt = ai.scoped, true
	}
	return ai
}

// lookup resolves an attachment base name, falling back to a one-time full-vault walk only
// when the name isn't found among the scoped (near-the-posts) attachments.
func (ai *attachmentIndex) lookup(name string) (string, bool) {
	key := strings.ToLower(name)
	if p, ok := ai.scoped[key]; ok {
		return p, true
	}
	if !ai.fullBuilt {
		ai.full = indexAttachments([]string{ai.vault})
		ai.fullBuilt = true
	}
	p, ok := ai.full[key]
	return p, ok
}

// indexAttachments indexes every non-markdown file under the given roots by lower-cased base
// name, mapping it to its absolute path. An Obsidian embed resolves by name (not relative to
// the note), so this lets a bare name find a concrete file; first match wins on a clash.
func indexAttachments(roots []string) map[string]string {
	idx := map[string]string{}
	for _, dir := range roots {
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || strings.EqualFold(filepath.Ext(p), ".md") {
				return nil
			}
			key := strings.ToLower(filepath.Base(p))
			if _, seen := idx[key]; !seen {
				idx[key] = p
			}
			return nil
		})
	}
	return idx
}

// resolveEmbeds turns Obsidian attachment references into what the build's asset pipeline
// understands: a body ![[name|alt]] becomes ![alt](<note-relative-path>), and a hero/image
// frontmatter ref ([[name]], ![[name]], or a bare name) becomes a note-relative path.
// Targets resolve vault-wide by base name; the relative path is computed from the note's
// real location to the attachment's, so it is correct even when posts live in a vault
// sub-folder and the attachment lives elsewhere in the vault. Unresolved refs are left as-is.
func (s *Source) resolveEmbeds(c *core.Content, root string, idx *attachmentIndex) {
	rel := func(ref string) (string, bool) {
		target := stripEmbed(ref)
		if i := strings.IndexAny(target, "|#"); i >= 0 {
			target = strings.TrimSpace(target[:i])
		}
		if target == "" {
			return "", false
		}
		attAbs, ok := idx.lookup(path.Base(target))
		if !ok {
			return "", false
		}
		noteAbsDir := filepath.Join(root, filepath.FromSlash(path.Dir(c.SourcePath)))
		r, err := filepath.Rel(noteAbsDir, attAbs)
		if err != nil {
			return "", false
		}
		return filepath.ToSlash(r), true
	}

	c.Body = embedRE.ReplaceAllStringFunc(c.Body, func(m string) string {
		inner := embedRE.FindStringSubmatch(m)[1]
		alt := ""
		if i := strings.Index(inner, "|"); i >= 0 {
			alt = strings.TrimSpace(inner[i+1:])
		}
		r, ok := rel(inner)
		if !ok {
			return m
		}
		return "![" + alt + "](<" + r + ">)"
	})

	if r, ok := rel(c.Frontmatter.Hero); ok {
		c.Frontmatter.Hero = r
	}
	if r, ok := rel(c.Frontmatter.Image); ok {
		c.Frontmatter.Image = r
	}
}

// stripEmbed unwraps an Obsidian [[wikilink]] / ![[embed]] to its inner target, so a
// frontmatter value written either way (or as a bare name) resolves the same.
func stripEmbed(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "!")
	v = strings.TrimSuffix(strings.TrimPrefix(v, "[["), "]]")
	return strings.TrimSpace(v)
}

// fillDefaults supplies title/date the Obsidian way when frontmatter omits them: the title
// from a leading `# heading` (stripped from the body) else the file name, and the date from
// the file's modification time.
func (s *Source) fillDefaults(c *core.Content, root string) {
	if c.Frontmatter.Title == "" {
		c.Frontmatter.Title, c.Body = titleFromBody(c.Body, c.SourcePath)
	}
	if c.Frontmatter.Date.IsZero() {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(c.SourcePath))); err == nil {
			c.Frontmatter.Date = info.ModTime()
		}
	}
}

// titleFromBody derives a title: a leading level-1 heading (removed from the returned body
// to avoid a duplicate H1), else the file name without extension.
func titleFromBody(body, sourcePath string) (title, newBody string) {
	if h1, rest, ok := leadingH1(body); ok {
		return h1, rest
	}
	name := path.Base(sourcePath)
	return strings.TrimSuffix(name, path.Ext(name)), body
}

// leadingH1 splits a leading `# heading` line off the body. ok is false when the body does
// not start (after blank lines) with an ATX H1.
func leadingH1(body string) (h1, rest string, ok bool) {
	trimmed := strings.TrimLeft(body, " \t\r\n")
	r, found := strings.CutPrefix(trimmed, "# ")
	if !found {
		return "", body, false
	}
	if nl := strings.IndexByte(r, '\n'); nl >= 0 {
		return strings.TrimSpace(r[:nl]), strings.TrimLeft(r[nl+1:], "\r\n"), true
	}
	return strings.TrimSpace(r), "", true
}

// --- tag-mode discovery ---

// normalizeTag lower-cases a tag and drops a leading '#', so `#Blog`, `Blog` and `blog`
// are equivalent.
func normalizeTag(t string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
}

var (
	fenceRE     = regexp.MustCompile("(?s)```.*?```")
	inlineTagRE = regexp.MustCompile(`(?:^|[\s(])#([A-Za-z0-9_][A-Za-z0-9_/-]*)`)
)

// inlineTags extracts Obsidian inline tags (`#tag`, nested `#a/b`) from a note body, after
// removing fenced code blocks so `#include`-style code doesn't read as a tag. A run with no
// letter (e.g. `#123`) is not a tag, matching Obsidian.
func inlineTags(body string) []string {
	body = fenceRE.ReplaceAllString(body, "")
	var out []string
	for _, m := range inlineTagRE.FindAllStringSubmatch(body, -1) {
		t := m[1]
		if strings.IndexFunc(t, func(r rune) bool {
			return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		}) < 0 {
			continue
		}
		out = append(out, strings.ToLower(t))
	}
	return out
}

// noteHasAnyTag reports whether a note carries any of the wanted tags (already normalized),
// in its frontmatter `tags:` or as an inline `#tag`. A wanted `blog` also matches a nested
// `blog/published`.
func noteHasAnyTag(c core.Content, wanted []string) bool {
	match := func(t string) bool {
		t = normalizeTag(t)
		for _, w := range wanted {
			if t == w || strings.HasPrefix(t, w+"/") {
				return true
			}
		}
		return false
	}
	for _, t := range c.Frontmatter.Tags {
		if match(t) {
			return true
		}
	}
	for _, t := range inlineTags(c.Body) {
		if match(t) {
			return true
		}
	}
	return false
}
