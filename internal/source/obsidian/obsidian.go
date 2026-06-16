// Package obsidian implements the "obsidian" source: an Obsidian vault folder. It reads
// the vault in place (no copy). It supports two ways to decide which notes are blog posts:
//
//   - Folder mode (`path` set): publish a folder of notes, keeping those flagged
//     `publish: true` (the Obsidian convention; `publish_required: false` keeps all).
//   - Tag mode (`vault` + `tag`, no `path`): scan the whole vault and publish every note
//     carrying a chosen Obsidian tag (frontmatter `tags:` or an inline `#tag`), the way the
//     Forestry/"digital garden" plugins work. Tag-selected notes that don't meet the blog
//     structure requirements (a title and some body) are warned about and skipped.
//
// The vault's folder structure maps onto the site structure, and deletes/renames flow
// through the normal build reconciliation. Note wikilinks ([[note]]) resolve later in the
// build (across every source); here the source resolves attachment embeds (![[image.png]])
// and hero/image frontmatter — which are vault-relative and Obsidian-specific — into paths
// the build's asset pipeline copies. Attachments resolve vault-wide, so when a `vault` is
// set the whole vault is indexed even if posts live in a sub-folder.
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

// discovery mode for an obsidian source.
const (
	modeOff    = ""       // nothing configured (optional source contributes nothing)
	modeFolder = "folder" // publish a folder, gated by the `publish:` flag
	modeTag    = "tag"     // publish vault notes carrying a chosen tag
)

// New builds an obsidian source. Resolution:
//
//   - `path` set            → folder mode over that folder (relative to `vault` if set,
//     else the project root). Backwards-compatible default.
//   - `vault` + `tag` only  → tag mode: scan the vault, publish notes carrying `tag`.
//   - `vault` only          → an error (no way to select notes).
//   - nothing set           → no documents (an unconfigured optional source).
func New(root string, cfg config.SourceConfig) (core.Source, error) {
	vault := resolveDir(root, str(cfg.Settings["vault"]))

	blog := strings.TrimSpace(str(cfg.Settings["path"]))
	blogDir := ""
	if blog != "" {
		// A blog path is relative to the vault when one is set, else the project root.
		base := root
		if vault != "" {
			base = vault
		}
		blogDir = resolveDir(base, blog)
	}

	tag := normalizeTag(str(cfg.Settings["tag"]))

	publishRequired := true
	if v, ok := cfg.Settings["publish_required"].(bool); ok {
		publishRequired = v
	}

	s := &Source{id: cfg.ID, publishRequired: publishRequired}
	switch {
	case blogDir != "":
		s.mode = modeFolder
		s.notesDir = blogDir
		s.vaultDir = vault
		if s.vaultDir == "" {
			s.vaultDir = blogDir
		}
	case vault != "" && tag != "":
		s.mode = modeTag
		s.notesDir = vault
		s.vaultDir = vault
		s.tag = tag
	case vault != "":
		return nil, fmt.Errorf("obsidian source %q: set `path` (a blog folder) or `tag` (an Obsidian tag to publish) when using a vault", cfg.ID)
	default:
		s.mode = modeOff
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
	mode            string
	notesDir        string // where notes are scanned (blog folder, or vault root in tag mode)
	vaultDir        string // vault root for attachment resolution (== notesDir when no vault)
	tag             string // normalized publish tag (tag mode only)
	publishRequired bool
	warnings        []string
}

func (s *Source) ID() string     { return s.id }
func (s *Source) Driver() string { return "obsidian" }

// Warnings reports non-fatal problems found during the last Documents call (e.g. tagged
// notes skipped for failing the structure checks). It satisfies core.Warner.
func (s *Source) Warnings() []string { return s.warnings }

func (s *Source) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	if s.notesDir == "" {
		return nil, os.ErrNotExist
	}
	return os.Open(filepath.Join(s.notesDir, filepath.FromSlash(ref)))
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

func (s *Source) Documents(ctx context.Context) ([]core.Content, error) {
	s.warnings = nil
	var docs []core.Content
	switch s.mode {
	case modeFolder:
		keep := func(fm markdown.Frontmatter) bool {
			if !s.publishRequired {
				return true
			}
			return fm.Publish != nil && *fm.Publish
		}
		d, err := mddir.Walk(s.notesDir, keep)
		if err != nil {
			return nil, err
		}
		docs = d
	case modeTag:
		all, err := mddir.Walk(s.notesDir, nil)
		if err != nil {
			return nil, err
		}
		for _, d := range all {
			if !noteHasTag(d, s.tag) {
				continue
			}
			if d.Frontmatter.Publish != nil && !*d.Frontmatter.Publish {
				continue // an explicit `publish: false` opts a tagged note back out
			}
			if reason := structureViolation(d); reason != "" {
				s.warnf("#%s note %q %s — skipping", s.tag, d.SourcePath, reason)
				continue
			}
			docs = append(docs, d)
		}
	default:
		return nil, nil // unconfigured optional source
	}
	idx := attachments(s.vaultDir)
	for i := range docs {
		s.fillDefaults(&docs[i])
		s.resolveEmbeds(&docs[i], idx)
	}
	return docs, nil
}

var embedRE = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)

// attachments indexes every non-markdown file under dir by lower-cased base name. An
// Obsidian embed resolves vault-wide by name (not relative to the note), so this maps a
// bare name back to a dir-relative path; first match wins on a name clash.
func attachments(dir string) map[string]string {
	idx := map[string]string{}
	if dir == "" {
		return idx
	}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.EqualFold(filepath.Ext(p), ".md") {
			return nil
		}
		if rel, err := filepath.Rel(dir, p); err == nil {
			key := strings.ToLower(filepath.Base(p))
			if _, seen := idx[key]; !seen {
				idx[key] = filepath.ToSlash(rel)
			}
		}
		return nil
	})
	return idx
}

// resolveEmbeds turns Obsidian attachment references into what the build's asset pipeline
// understands: a body ![[name|alt]] becomes ![alt](<note-relative-path>), and a hero/image
// frontmatter ref ([[name]], ![[name]], or a bare name) becomes a note-relative path.
// Targets resolve vault-wide by base name; the relative path is computed from the note's
// real location to the attachment's, so it is correct even when posts live in a vault
// sub-folder and the attachment lives elsewhere in the vault. Unresolved refs are left as-is.
func (s *Source) resolveEmbeds(c *core.Content, idx map[string]string) {
	rel := func(ref string) (string, bool) {
		target := stripEmbed(ref)
		if i := strings.IndexAny(target, "|#"); i >= 0 {
			target = strings.TrimSpace(target[:i])
		}
		vaultRel, ok := idx[strings.ToLower(path.Base(target))]
		if !ok || target == "" {
			return "", false
		}
		noteAbsDir := filepath.Join(s.notesDir, filepath.FromSlash(path.Dir(c.SourcePath)))
		attAbs := filepath.Join(s.vaultDir, filepath.FromSlash(vaultRel))
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
func (s *Source) fillDefaults(c *core.Content) {
	if c.Frontmatter.Title == "" {
		c.Frontmatter.Title, c.Body = titleFromBody(c.Body, c.SourcePath)
	}
	if c.Frontmatter.Date.IsZero() {
		if info, err := os.Stat(filepath.Join(s.notesDir, filepath.FromSlash(c.SourcePath))); err == nil {
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

// noteHasTag reports whether a note carries tag in its frontmatter `tags:` or as an inline
// `#tag`. A configured `blog` also matches a nested `blog/published`.
func noteHasTag(c core.Content, tag string) bool {
	match := func(t string) bool {
		t = normalizeTag(t)
		return t == tag || strings.HasPrefix(t, tag+"/")
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

// structureViolation returns a human-readable reason a tag-selected note is not publishable
// as a blog post, or "" when it is fine. The bar is intentionally low: a title (frontmatter
// `title:` or a leading `# heading`) and some body content. Date falls back to the file
// mtime, so it is not required.
func structureViolation(c core.Content) string {
	hasTitle := c.Frontmatter.Title != ""
	content := c.Body
	if !hasTitle {
		if h1, rest, ok := leadingH1(c.Body); ok && h1 != "" {
			hasTitle, content = true, rest
		}
	}
	if !hasTitle {
		return "has no title (add a frontmatter `title:` or a leading `# heading`)"
	}
	if strings.TrimSpace(content) == "" {
		return "has no body content"
	}
	return ""
}
