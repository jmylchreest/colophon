// Package markdown is colophon's content-file codec: it parses and generates the
// frontmatter-plus-markdown documents colophon and external tools exchange. The
// frontmatter is a typed struct; the body is preserved as raw markdown text.
package markdown

import (
	"bytes"
	"time"

	"go.yaml.in/yaml/v3"
)

// Frontmatter is the typed YAML block at the top of a content file. Unknown keys are
// ignored so authors can carry source-specific metadata (e.g. Obsidian fields).
type Frontmatter struct {
	Title       string    `yaml:"title"`
	Date        time.Time `yaml:"date,omitempty"`
	Slug        string    `yaml:"slug,omitempty"`
	Description string    `yaml:"description,omitempty"`
	Tags        []string  `yaml:"tags,omitempty"`
	Categories  []string  `yaml:"categories,omitempty"`

	// Type selects the page type for templating and placement, e.g. "post" (chronological:
	// listed, in feeds and tags) or "page" (standing: surfaced in the nav menu). When unset it
	// is derived from the presence of a date (dated → post, dateless → page). A theme may add a
	// "<type>.html" template to style a type; otherwise it renders with "page.html". This is
	// distinct from seo.type (the schema.org type).
	Type string `yaml:"type,omitempty"`

	// Hero is a banner image shown at the top of the post; Image is the preview/social
	// card image (Open Graph, index thumbnail). Both are source-relative paths (an
	// Obsidian [[embed]] is accepted too) that the build copies beside the page.
	Hero  string `yaml:"hero,omitempty"`
	Image string `yaml:"image,omitempty"`

	// Persona is shorthand for a single publication as that persona, mutually
	// exclusive with Publications; if both are set, Publications wins.
	Persona      string            `yaml:"persona,omitempty"`
	Publications []PublicationSpec `yaml:"publications,omitempty"`

	// Draft is a manual gate; PublishAfter is a time gate ("not before"). Publish is
	// the Obsidian whitelist flag: nil means "default true" unless a source opts in.
	Draft        bool       `yaml:"draft,omitempty"`
	Publish      *bool      `yaml:"publish,omitempty"`
	PublishAfter *time.Time `yaml:"publish_after,omitempty"`

	Syndicate []string `yaml:"syndicate,omitempty"`

	// SEO is optional search/social metadata. Every field has a single rendering effect
	// (canonical/robots/Open Graph/Twitter/JSON-LD), so it is also the precise target an
	// AI skill fills in. Absent fields fall back to title/description/tags/date/persona.
	SEO *SEO `yaml:"seo,omitempty"`
}

// SEO is per-post search and social metadata. It overrides the derived defaults and is the
// contract an authoring skill populates.
type SEO struct {
	Title       string     `yaml:"title,omitempty"`       // <title>/og:title; ~≤60 chars
	Description string     `yaml:"description,omitempty"` // meta description; ~140–160 chars
	Keywords    []string   `yaml:"keywords,omitempty"`    // focus terms (also JSON-LD keywords)
	Canonical   string     `yaml:"canonical,omitempty"`   // absolute canonical URL override
	NoIndex     bool       `yaml:"noindex,omitempty"`     // emit robots noindex
	Image       string     `yaml:"image,omitempty"`       // absolute social-image URL override
	Type        string     `yaml:"type,omitempty"`        // schema.org type (default BlogPosting)
	Social      *SEOSocial `yaml:"social,omitempty"`      // copy distinct from search copy
}

// SEOSocial holds Open Graph/Twitter copy when it should differ from the search title and
// description.
type SEOSocial struct {
	Title       string `yaml:"title,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// PublicationSpec is one entry of a content file's publications list; zero fields
// inherit from the frontmatter.
type PublicationSpec struct {
	Persona      string     `yaml:"persona"`
	Slug         string     `yaml:"slug,omitempty"`
	Date         *time.Time `yaml:"date,omitempty"`
	PublishAfter *time.Time `yaml:"publish_after,omitempty"`
	Tags         []string   `yaml:"tags,omitempty"`
	Draft        *bool      `yaml:"draft,omitempty"`
}

// Document is a parsed content file: typed frontmatter plus the raw markdown body.
type Document struct {
	Frontmatter Frontmatter
	Body        string
}

var (
	fence = []byte("---")
	bom   = []byte{0xEF, 0xBB, 0xBF}
)

// Parse splits a leading ---delimited YAML block from the markdown body. A file with
// no opening (or closing) fence is treated as all body, no frontmatter.
func Parse(raw []byte) (*Document, error) {
	trimmed := bytes.TrimLeft(bytes.TrimPrefix(raw, bom), " \t\r\n")
	if !bytes.HasPrefix(trimmed, fence) {
		return &Document{Body: string(raw)}, nil
	}
	rest := trimmed[len(fence):]
	if i := bytes.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		rest = nil
	}
	end := bytes.Index(rest, append([]byte("\n"), fence...))
	if end < 0 {
		return &Document{Body: string(raw)}, nil
	}
	var fm Frontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return nil, err
	}
	// Drop the closing fence's line terminator and a single conventional blank line
	// after it, so "---\n\nbody" parses to "body" and round-trips with Marshal.
	body := rest[end+1+len(fence):]
	body = bytes.TrimPrefix(body, []byte("\n"))
	body = bytes.TrimPrefix(body, []byte("\n"))
	return &Document{Frontmatter: fm, Body: string(body)}, nil
}

// Marshal renders the document to bytes: a frontmatter block then the body. The body
// is byte-preserved; the frontmatter is canonical YAML, so key order and any comments
// from a parsed source are not retained.
func (d *Document) Marshal() ([]byte, error) {
	y, err := yaml.Marshal(d.Frontmatter)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.Write(fence)
	buf.WriteByte('\n')
	buf.Write(y)
	buf.Write(fence)
	buf.WriteByte('\n')
	if d.Body != "" {
		buf.WriteByte('\n')
		buf.WriteString(d.Body)
	}
	return buf.Bytes(), nil
}
