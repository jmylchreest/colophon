package cli

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/persona"
)

// NewCmd scaffolds a new entry. It validates the author/persona, derives a unique pinned
// slug, writes a frontmatter skeleton to the right source, and reports the disk path and URL.
// The body is left for a person — or an agent — to fill; colophon never generates prose.
type NewCmd struct {
	Post NewPostCmd `cmd:"" help:"Scaffold a new post (dated, chronological)"`
	Page NewPageCmd `cmd:"" help:"Scaffold a new standing page (nav menu, no date)"`
}

// newOpts are the flags shared by `new post` and `new page`.
type newOpts struct {
	Title   string   `arg:"" help:"Entry title"`
	Author  string   `help:"Byline author id (validated; default: first author / Anonymous)"`
	Persona string   `help:"Writing-voice persona id (validated; optional)"`
	Tag     []string `help:"Tags"`
	Slug    string   `help:"Explicit slug (else derived from the title and made unique)"`
	Unique  string   `default:"hash" enum:"hash,counter" help:"Slug collision strategy: hash | counter"`
	In      string   `help:"Source id to write into (default: the first source)"`
	Print   bool     `help:"Print the file to stdout instead of writing it"`
}

type NewPostCmd struct{ newOpts }

func (c *NewPostCmd) Run() error { return runNew(c.newOpts, "post") }

type NewPageCmd struct{ newOpts }

func (c *NewPageCmd) Run() error { return runNew(c.newOpts, "page") }

func runNew(o newOpts, kind string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if a := strings.TrimSpace(o.Author); a != "" && cfg.Author(a) == nil {
		return fmt.Errorf("unknown author %q (have: %s)", a, strings.Join(authorIDs(cfg), ", "))
	}
	if p := strings.TrimSpace(o.Persona); p != "" && persona.Find(cfg, p) == nil {
		return fmt.Errorf("unknown persona %q (have: %s)", p, strings.Join(persona.IDs(cfg), ", "))
	}

	tgt := pickWriteTarget(cfg, o.In)
	if tgt.dir == "" {
		return fmt.Errorf("source %q has no resolvable location to write into", tgt.sourceID)
	}

	segment := strings.TrimSpace(o.Slug)
	if segment == "" {
		segment = build.Slugify(o.Title)
	}
	if segment == "" {
		return fmt.Errorf("title produced an empty slug; pass --slug")
	}

	prefix := "" // md-dir namespaces posts/pages; obsidian slugs are flat
	if tgt.driver == "md-dir" {
		prefix = kind + "s" // posts | pages
	}
	existing, err := build.Slugs(cfg)
	if err != nil {
		return err
	}
	fullSlug := func(seg string) string {
		if prefix == "" {
			return seg
		}
		return prefix + "/" + seg
	}
	if o.Slug != "" {
		if existing[fullSlug(segment)] {
			return fmt.Errorf("slug %q already exists — pick another", fullSlug(segment))
		}
	} else {
		segment = uniqueSegment(segment, fullSlug, existing, o.Unique)
	}

	rel := segment + ".md"
	if prefix != "" {
		rel = filepath.Join(prefix, rel)
	}
	filePath := filepath.Join(tgt.dir, rel)
	doc := newSkeleton(cfg, o, kind, segment, tgt)

	if o.Print {
		fmt.Printf("# → write to: %s   (source: %s / %s)\n", filePath, tgt.sourceID, tgt.driver)
		fmt.Print(doc)
		return nil
	}
	if _, err := os.Stat(filePath); err == nil {
		return fmt.Errorf("%s already exists", filePath)
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filePath, []byte(doc), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote:  %s\n", filePath)
	fmt.Printf("slug:   %s\n", fullSlug(segment))
	fmt.Printf("url:    /%s/   (preview: colophon serve)\n", fullSlug(segment))
	if p := strings.TrimSpace(o.Persona); p != "" {
		if pv := persona.Find(cfg, p); pv != nil && pv.Name != "" {
			fmt.Printf("voice:  %s — %s\n", p, pv.Name)
		}
	}
	return nil
}

// writeTarget is the resolved place a new entry is written, plus how it's marked publishable.
type writeTarget struct {
	sourceID string
	driver   string
	dir      string // base writable directory
	tagMark  string // obsidian: add this tag to publish (else "")
	flagMark bool   // obsidian: needs `publish: true`
}

func pickWriteTarget(cfg *config.Config, inID string) writeTarget {
	srcs := cfg.Sources
	if len(srcs) == 0 {
		srcs = []config.SourceConfig{{ID: "content", Driver: "md-dir"}}
	}
	chosen := srcs[0]
	if inID != "" {
		for _, s := range srcs {
			if s.ID == inID {
				chosen = s
				break
			}
		}
	}
	get := func(k string) string { v, _ := chosen.Settings[k].(string); return strings.TrimSpace(v) }
	t := writeTarget{sourceID: chosen.ID, driver: chosen.Driver}
	switch chosen.Driver {
	case "obsidian":
		dir := resolveUnder(cfg.Root, get("vault"))
		if p := firstCSV(get("path")); p != "" {
			dir = filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(p, "/")))
		}
		t.dir = dir
		t.tagMark = firstCSV(get("tag"))
		t.flagMark = t.tagMark == ""
	default: // md-dir
		p := get("path")
		if p == "" {
			p = "content"
		}
		t.dir = resolveUnder(cfg.Root, p)
	}
	return t
}

func firstCSV(s string) string {
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			return p
		}
	}
	return ""
}

// newSkeleton builds the frontmatter skeleton. `type:` is omitted because the date heuristic
// already yields post (dated) or page (dateless); the slug is pinned so the URL is stable.
func newSkeleton(cfg *config.Config, o newOpts, kind, segment string, tgt writeTarget) string {
	tags := append([]string(nil), o.Tag...)
	if tgt.tagMark != "" {
		tags = append(tags, tgt.tagMark)
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", yamlScalar(o.Title)) // quote free text so a colon/quote can't break the YAML
	fmt.Fprintf(&b, "slug: %s\n", segment)
	if a := strings.TrimSpace(o.Author); a != "" {
		fmt.Fprintf(&b, "author: %s\n", a)
	}
	if p := strings.TrimSpace(o.Persona); p != "" {
		fmt.Fprintf(&b, "persona: %s\n", p)
	}
	if kind == "post" {
		fmt.Fprintf(&b, "date: %s\n", time.Now().Format("2006-01-02"))
	}
	b.WriteString("draft: true\n")
	if len(tags) > 0 {
		fmt.Fprintf(&b, "tags: [%s]\n", strings.Join(tags, ", "))
	}
	if tgt.flagMark {
		b.WriteString("publish: true\n")
	}
	b.WriteString("---\n\n")
	if p := strings.TrimSpace(o.Persona); p != "" {
		if pv := persona.Find(cfg, p); pv != nil && pv.Style.Guide != "" {
			fmt.Fprintf(&b, "<!-- voice (%s): %s -->\n", p, firstLine(pv.Style.Guide))
		}
	}
	fmt.Fprintf(&b, "<!-- write the %s here -->\n", kind)
	return b.String()
}

// yamlScalar renders s as a YAML scalar, quoting it only when needed (e.g. a title containing a
// colon-space or a leading quote) so the generated frontmatter is always valid YAML.
func yamlScalar(s string) string {
	b, err := yaml.Marshal(s)
	if err != nil {
		return strconv.Quote(s)
	}
	return strings.TrimSpace(string(b))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// uniqueSegment returns a slug segment not already in use, disambiguating a collision with a
// short hash (default) or an incrementing counter.
func uniqueSegment(base string, full func(string) string, existing map[string]bool, strategy string) string {
	if !existing[full(base)] {
		return base
	}
	if strategy == "counter" {
		for n := 2; ; n++ {
			cand := fmt.Sprintf("%s-%d", base, n)
			if !existing[full(cand)] {
				return cand
			}
		}
	}
	for salt := 0; ; salt++ { // hash, re-rolling on the rare clash
		cand := base + "-" + shortHash(base, salt)
		if !existing[full(cand)] {
			return cand
		}
	}
}

// shortHash is a short, URL-safe id seeded by the slug, the current time and a re-roll salt.
func shortHash(seed string, salt int) string {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s|%d|%d", seed, time.Now().UnixNano(), salt)
	s := strconv.FormatUint(uint64(h.Sum32()), 36)
	for len(s) < 5 {
		s = "0" + s
	}
	return s[:5]
}
