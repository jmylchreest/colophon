// Package obsidian implements the "obsidian" source: an Obsidian vault folder. It reads
// the vault in place (no copy) and, by Obsidian convention, publishes only notes with
// `publish: true` in their frontmatter. The vault's folder structure maps onto the site
// structure, and deletes/renames flow through the normal build reconciliation.
//
// Note wikilinks ([[note]]) resolve later in the build (across every source); here the
// source resolves attachment embeds (![[image.png]]) and hero/image frontmatter, which
// are vault-relative and Obsidian-specific, into paths the build's asset pipeline copies.
package obsidian

import (
	"context"
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

// New builds an obsidian source over a vault folder. An unset path yields no documents;
// publish_required (default true) keeps only notes flagged `publish: true`.
func New(root string, cfg config.SourceConfig) (core.Source, error) {
	dir, _ := cfg.Settings["path"].(string)
	dir = expandHome(strings.TrimSpace(dir))
	if dir != "" && !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	publishRequired := true
	if v, ok := cfg.Settings["publish_required"].(bool); ok {
		publishRequired = v
	}
	return &Source{id: cfg.ID, dir: dir, publishRequired: publishRequired}, nil
}

type Source struct {
	id              string
	dir             string
	publishRequired bool
}

func (s *Source) ID() string     { return s.id }
func (s *Source) Driver() string { return "obsidian" }

func (s *Source) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	if s.dir == "" {
		return nil, os.ErrNotExist
	}
	return os.Open(filepath.Join(s.dir, filepath.FromSlash(ref)))
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

func (s *Source) Documents(ctx context.Context) ([]core.Content, error) {
	if s.dir == "" {
		return nil, nil // unconfigured optional source
	}
	keep := func(fm markdown.Frontmatter) bool {
		if !s.publishRequired {
			return true
		}
		return fm.Publish != nil && *fm.Publish
	}
	docs, err := mddir.Walk(s.dir, keep)
	if err != nil {
		return nil, err
	}
	idx := attachments(s.dir)
	for i := range docs {
		s.fillDefaults(&docs[i])
		resolveEmbeds(&docs[i], idx)
	}
	return docs, nil
}

var embedRE = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)

// attachments indexes every non-markdown file in the vault by lower-cased base name. An
// Obsidian embed resolves vault-wide by name (not relative to the note), so this maps a
// bare name back to a concrete vault-relative path; first match wins on a name clash.
func attachments(dir string) map[string]string {
	idx := map[string]string{}
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
// frontmatter ref (written as [[name]], ![[name]], or a bare name) becomes a note-relative
// path. Targets resolve vault-wide by base name; an unresolved ref is left untouched.
func resolveEmbeds(c *core.Content, idx map[string]string) {
	rel := func(ref string) (string, bool) {
		target := stripEmbed(ref)
		if i := strings.IndexAny(target, "|#"); i >= 0 {
			target = strings.TrimSpace(target[:i])
		}
		vaultPath, ok := idx[strings.ToLower(path.Base(target))]
		if !ok || target == "" {
			return "", false
		}
		r, err := filepath.Rel(filepath.FromSlash(path.Dir(c.SourcePath)), filepath.FromSlash(vaultPath))
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

// fillDefaults supplies title/date the Obsidian way when frontmatter omits them: the
// title from a leading `# heading` (stripped from the body) else the file name, and the
// date from the file's modification time.
func (s *Source) fillDefaults(c *core.Content) {
	if c.Frontmatter.Title == "" {
		c.Frontmatter.Title, c.Body = titleFromBody(c.Body, c.SourcePath)
	}
	if c.Frontmatter.Date.IsZero() {
		if info, err := os.Stat(filepath.Join(s.dir, filepath.FromSlash(c.SourcePath))); err == nil {
			c.Frontmatter.Date = info.ModTime()
		}
	}
}

// titleFromBody derives a title: a leading level-1 heading (removed from the returned
// body to avoid a duplicate H1), else the file name without extension.
func titleFromBody(body, sourcePath string) (title, newBody string) {
	trimmed := strings.TrimLeft(body, " \t\r\n")
	if rest, ok := strings.CutPrefix(trimmed, "# "); ok {
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			return strings.TrimSpace(rest[:nl]), strings.TrimLeft(rest[nl+1:], "\r\n")
		}
		return strings.TrimSpace(rest), ""
	}
	name := path.Base(sourcePath)
	return strings.TrimSuffix(name, path.Ext(name)), body
}
