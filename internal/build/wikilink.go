package build

import (
	"path"
	"regexp"
	"strings"
)

// linkResolver maps a wikilink target (a note's filename or path, lower-cased) to its
// site href. It is built from every document in the build, so any note can link to any
// other by name regardless of folder.
type linkResolver map[string]string

// add registers a document under both its slug path and its bare filename, so both
// [[posts/hello]] and [[hello]] resolve. href already includes the base path.
func (r linkResolver) add(sourcePath, slug, basePath string) {
	href := basePath + slug + "/"
	r[strings.ToLower(slug)] = href
	r[strings.ToLower(base(slug))] = href

	noExt := strings.TrimSuffix(sourcePath, path.Ext(sourcePath))
	r[strings.ToLower(noExt)] = href
	r[strings.ToLower(base(noExt))] = href
}

func base(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

var wikiRE = regexp.MustCompile(`(!?)\[\[([^\]\n]+)\]\]`)

// resolveWikilinks rewrites Obsidian-style [[target]] / [[target|alias]] /
// [[target#heading]] into standard markdown links using r, before goldmark parses.
// Unresolved links degrade to their alias/target text rather than a broken link. Image
// and note embeds (![[...]]) are left untouched pending the asset pipeline.
func resolveWikilinks(body string, r linkResolver) string {
	return wikiRE.ReplaceAllStringFunc(body, func(m string) string {
		sub := wikiRE.FindStringSubmatch(m)
		embed, inner := sub[1] == "!", sub[2]

		target, alias := inner, ""
		if i := strings.Index(inner, "|"); i >= 0 {
			target, alias = inner[:i], inner[i+1:]
		}
		heading := ""
		if i := strings.Index(target, "#"); i >= 0 {
			target, heading = target[:i], target[i+1:]
		}
		target = strings.TrimSpace(target)

		if embed {
			return m // embeds need the asset pipeline; leave as-is for now
		}

		href, ok := r[strings.ToLower(target)]
		if !ok {
			if alias != "" {
				return alias
			}
			return target
		}
		if heading != "" {
			href += "#" + headingAnchor(heading)
		}
		text := alias
		if text == "" {
			text = target
		}
		return "[" + text + "](" + href + ")"
	})
}

// headingAnchor approximates goldmark's auto heading id: lower-case, spaces to hyphens,
// drop other punctuation.
func headingAnchor(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	return b.String()
}
