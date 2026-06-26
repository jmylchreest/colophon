package build

import (
	"bytes"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/config"
)

// Entry is a content entry's resolved publishing metadata, exposed for tooling — `colophon
// posts` (listing, cross-referencing) and slug uniqueness in `colophon new`. It renders
// nothing; drafts and embargoed posts are included since this is authoring metadata.
type Entry struct {
	SourcePath  string    `json:"source_path"`
	Slug        string    `json:"slug"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Type        string    `json:"type"`
	Author      string    `json:"author,omitempty"`
	Persona     string    `json:"persona,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Description string    `json:"description,omitempty"`
	Draft       bool      `json:"draft"`
	Date        time.Time `json:"date,omitempty"`
	// Predecessor is the raw frontmatter `predecessor:` (slug/filename of the preceding post in a
	// series), unresolved — tooling (doctor) resolves it against the slug set.
	Predecessor string `json:"predecessor,omitempty"`
	// Syndication (POSSE) frontmatter, resolved for `colophon syndicate`: SyndicateOff opts the
	// post out; SyndicateTargets is the chosen subset of env targets (nil = all); SyndicateText
	// is an optional custom blurb.
	SyndicateOff     bool     `json:"syndicate_off,omitempty"`
	SyndicateTargets []string `json:"syndicate_targets,omitempty"`
	SyndicateText    string   `json:"syndicate_text,omitempty"`
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
		slug := slugFor(d.doc.SourcePath, fm.Slug)
		// Mirror the page's description: an explicit `description:`, else a short excerpt of the
		// rendered body — so consumers (syndication cards, feeds, tooling) get a summary for posts
		// that set none.
		desc := strings.TrimSpace(fm.Description)
		if desc == "" {
			desc = bodyExcerpt(d.doc.Body)
		}
		out = append(out, Entry{
			SourcePath:       d.doc.SourcePath,
			Slug:             slug,
			URL:              slug + "/",
			Title:            fm.Title,
			Type:             resolvePageType(fm),
			Author:           fm.Author,
			Persona:          resolvePersona(fm),
			Tags:             fm.Tags,
			Description:      desc,
			Draft:            fm.Draft,
			Date:             fm.Date,
			Predecessor:      fm.Predecessor,
			SyndicateOff:     fm.Syndicate.Off,
			SyndicateTargets: fm.Syndicate.Targets,
			SyndicateText:    fm.SyndicateText,
		})
	}
	return out, nil
}

// bodyExcerpt renders a post body to a short plain-text excerpt, matching the page's
// `description` fallback, so a post with no explicit `description:` still gets a summary.
func bodyExcerpt(body string) string {
	var buf bytes.Buffer
	if err := sharedMarkdown.Convert([]byte(preprocessCallouts(body)), &buf); err != nil {
		return ""
	}
	return excerpt(buf.String(), 200)
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
