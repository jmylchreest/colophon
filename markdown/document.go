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
	Lang        string    `yaml:"lang,omitempty"` // per-post BCP-47 override of the site language
	Description string    `yaml:"description,omitempty"`
	Tags        []string  `yaml:"tags,omitempty"`
	Categories  []string  `yaml:"categories,omitempty"`

	// Type selects the page type for templating and placement, e.g. "post" (chronological:
	// listed, in feeds and tags) or "page" (standing: surfaced in the nav menu). When unset it
	// is derived from the presence of a date (dated → post, dateless → page). A theme may add a
	// "<type>.html" template to style a type; otherwise it renders with "page.html". This is
	// distinct from seo.type (the schema.org type).
	Type string `yaml:"type,omitempty"`

	// Predecessor pins the slug (or bare filename, resolved like a wikilink) of the post that
	// immediately precedes this one in a series — a single backward link. The engine walks these
	// edges to reconstruct the whole ordered chain and regenerate every member with series nav, so
	// older posts never need editing. A post is in at most one series.
	Predecessor string `yaml:"predecessor,omitempty"`
	// Series is the optional series title. Latest-wins: the name is taken from the newest post in
	// the chain that sets it; if no member sets it, the series is untitled.
	Series string `yaml:"series,omitempty"`

	// Hero is a banner image shown at the top of the post; Image is the preview/social
	// card image (Open Graph, index thumbnail). Both are source-relative paths (an
	// Obsidian [[embed]] is accepted too) that the build copies beside the page.
	Hero  string `yaml:"hero,omitempty"`
	Image string `yaml:"image,omitempty"`

	// HeroAlt/ImageAlt are the accessible alt text for the banner and card images. Empty
	// means decorative (alt=""), which is correct for a purely ornamental banner; set them
	// when the image carries meaning a screen-reader user would otherwise miss.
	HeroAlt  string `yaml:"hero_alt,omitempty"`
	ImageAlt string `yaml:"image_alt,omitempty"`

	// HeroFit/ImageFit choose how the image fills its box (CSS object-fit: cover|contain|
	// fill|scale-down|none); *Position picks which part shows when cropping (CSS
	// object-position, e.g. "top" or "50% 20%"). Empty → the theme's default (usually cover).
	HeroFit       string `yaml:"hero_fit,omitempty"`
	HeroPosition  string `yaml:"hero_position,omitempty"`
	ImageFit      string `yaml:"image_fit,omitempty"`
	ImagePosition string `yaml:"image_position,omitempty"`

	// Audio attaches a podcast-style audio reading of the post. Two sources: AudioFile points
	// at a pre-recorded file (a source-relative path or [[embed]], copied like hero), which
	// wins when set; otherwise Audio: true generates a TTS reading when speech generation is
	// configured. AudioVoice overrides the generated voice id (else the author's/persona's
	// voice, else the site default).
	Audio      *bool  `yaml:"audio,omitempty"` // tri-state: unset → site default; true/false → explicit
	AudioFile  string `yaml:"audio_file,omitempty"`
	AudioVoice string `yaml:"audio_voice,omitempty"`

	// Attachments are downloadable files published with the post — scripts, archives,
	// datasets, PDFs, etc. Each entry is either a bare source-relative path (or [[embed]])
	// or a {path, label, feed} mapping. The build copies/routes them exactly like images.
	// label sets the link text (default: the file name); feed: true also lists the file as
	// an enclosure/attachment in the feeds.
	Attachments []Attachment `yaml:"attachments,omitempty"`

	// Author is the byline shown to readers — the id of an authors/*.yaml entry. Empty
	// falls back to the first configured author, else "Anonymous".
	Author string `yaml:"author,omitempty"`

	// Persona is the hidden writing voice (a personas/*.yaml id) used by the agent to write
	// in a consistent style; it is never shown. Mutually exclusive with Publications; if both
	// are set, Publications wins.
	Persona      string            `yaml:"persona,omitempty"`
	Publications []PublicationSpec `yaml:"publications,omitempty"`

	// Glossary opts this post out of glossary decoration when false (default on). An author
	// can also suppress a single term with <span class="no-gloss">…</span> or force one with
	// <dfn>term</dfn> in the body.
	Glossary *bool `yaml:"glossary,omitempty"`

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

// Attachment is one downloadable file shipped with a post. It accepts two YAML shapes for
// authoring convenience: a bare scalar path (- foo.zip) or a mapping (- {path: foo.zip,
// label: "Download", feed: true}).
type Attachment struct {
	Path        string `yaml:"path"`                  // source-relative path or [[embed]]
	Label       string `yaml:"label,omitempty"`       // link text; defaults to the file name
	Description string `yaml:"description,omitempty"` // one-line description shown under the label
	Feed        bool   `yaml:"feed,omitempty"`        // also emit as a feed enclosure/attachment
}

// UnmarshalYAML accepts either a scalar path or a {path,label,feed} mapping.
func (a *Attachment) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		return value.Decode(&a.Path)
	}
	type rawAttachment Attachment
	var r rawAttachment
	if err := value.Decode(&r); err != nil {
		return err
	}
	*a = Attachment(r)
	return nil
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
	fm, body, err := ParseFrontmatter(raw)
	if err != nil {
		return nil, err
	}
	return &Document{Frontmatter: fm, Body: string(body)}, nil
}

// ParseFrontmatter is the cheap half of Parse: it splits the leading ---fenced YAML block
// and returns the typed frontmatter plus the body as a sub-slice of raw (no copy). A caller
// that filters on frontmatter alone (e.g. a source's publish gate) can decide before paying
// to copy the body into a string. A file with no opening/closing fence is all body, no
// frontmatter. The returned body aliases raw — copy it (string(body)) before retaining.
func ParseFrontmatter(raw []byte) (Frontmatter, []byte, error) {
	trimmed := bytes.TrimLeft(bytes.TrimPrefix(raw, bom), " \t\r\n")
	if !bytes.HasPrefix(trimmed, fence) {
		return Frontmatter{}, raw, nil
	}
	rest := trimmed[len(fence):]
	if i := bytes.IndexByte(rest, '\n'); i >= 0 {
		rest = rest[i+1:]
	} else {
		rest = nil
	}
	end := bytes.Index(rest, append([]byte("\n"), fence...))
	if end < 0 {
		return Frontmatter{}, raw, nil
	}
	var fm Frontmatter
	if err := yaml.Unmarshal(rest[:end], &fm); err != nil {
		return Frontmatter{}, nil, err
	}
	// Drop the closing fence's line terminator and a single conventional blank line
	// after it, so "---\n\nbody" parses to "body" and round-trips with Marshal.
	body := rest[end+1+len(fence):]
	body = bytes.TrimPrefix(body, []byte("\n"))
	body = bytes.TrimPrefix(body, []byte("\n"))
	return fm, body, nil
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
