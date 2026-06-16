package build

import (
	"time"

	"github.com/jmylchreest/colophon/internal/config"
)

// Entry is a content entry's resolved publishing metadata, exposed for tooling — `colophon
// posts` (listing, cross-referencing) and slug uniqueness in `colophon new`. It renders
// nothing; drafts and embargoed posts are included since this is authoring metadata.
type Entry struct {
	SourcePath string    `json:"source_path"`
	Slug       string    `json:"slug"`
	URL        string    `json:"url"`
	Title      string    `json:"title"`
	Type       string    `json:"type"`
	Author     string    `json:"author,omitempty"`
	Persona    string    `json:"persona,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	Draft      bool      `json:"draft"`
	Date       time.Time `json:"date,omitempty"`
}

// Entries gathers every content document across the configured sources and resolves each to
// its slug, page type, byline author and voice persona — the same way a build would.
func Entries(cfg *config.Config) ([]Entry, error) {
	docs, err := gatherDocuments(cfg, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(docs))
	for _, d := range docs {
		fm := d.doc.Frontmatter
		persona := fm.Persona
		if persona == "" && len(fm.Publications) > 0 {
			persona = fm.Publications[0].Persona
		}
		slug := slugFor(d.doc.SourcePath, fm.Slug)
		out = append(out, Entry{
			SourcePath: d.doc.SourcePath,
			Slug:       slug,
			URL:        slug + "/",
			Title:      fm.Title,
			Type:       resolvePageType(fm),
			Author:     fm.Author,
			Persona:    persona,
			Tags:       fm.Tags,
			Draft:      fm.Draft,
			Date:       fm.Date,
		})
	}
	return out, nil
}

// Slugify turns arbitrary text (e.g. a post title) into a slug segment, using the same
// normalisation the build applies to paths — for `colophon new` to derive a slug.
func Slugify(s string) string { return normalizeSlug(s) }

// Slugs returns the set of slugs already in use across all content, for uniqueness checks.
func Slugs(cfg *config.Config) (map[string]bool, error) {
	entries, err := Entries(cfg)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(entries))
	for _, e := range entries {
		set[e.Slug] = true
	}
	return set, nil
}
