package build

import (
	"path"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/markdown"
)

// translation is one language a post is available in — for the language selector and the
// <link rel="alternate" hreflang> tags. Posts are linked by a shared language-neutral slug.
type translation struct {
	Lang    string // BCP-47 code, e.g. "es"
	Label   string // native language name for the selector, e.g. "Español"
	URL     string // base_path-relative URL of that translation
	Abs     string // absolute URL (for hreflang)
	Current bool   // the translation being rendered now
	Default bool   // the site's default-language version (hreflang x-default)
}

// langNames maps common BCP-47 primary subtags to their native names for the selector. Unknown
// codes fall back to the uppercased tag.
var langNames = map[string]string{
	"en": "English", "es": "Español", "fr": "Français", "de": "Deutsch", "it": "Italiano",
	"pt": "Português", "nl": "Nederlands", "sv": "Svenska", "no": "Norsk", "da": "Dansk",
	"fi": "Suomi", "is": "Íslenska", "pl": "Polski", "cs": "Čeština", "sk": "Slovenčina",
	"ru": "Русский", "uk": "Українська", "bg": "Български", "sr": "Српски", "hr": "Hrvatski",
	"ja": "日本語", "zh": "中文", "ko": "한국어", "th": "ไทย", "vi": "Tiếng Việt",
	"ar": "العربية", "he": "עברית", "fa": "فارسی", "hi": "हिन्दी", "bn": "বাংলা",
	"tr": "Türkçe", "el": "Ελληνικά", "ro": "Română", "hu": "Magyar", "id": "Bahasa Indonesia",
	"cy": "Cymraeg", "ga": "Gaeilge", "ca": "Català", "eu": "Euskara", "gl": "Galego",
}

// langLabel returns the native name for a language code, falling back to the primary subtag's name
// (pt-BR → Português) and finally the uppercased tag.
func langLabel(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if n, ok := langNames[c]; ok {
		return n
	}
	if i := strings.IndexByte(c, '-'); i > 0 {
		if n, ok := langNames[c[:i]]; ok {
			return n
		}
	}
	return strings.ToUpper(code)
}

// splitLangFromPath strips a `.<lang>` infix from a source path (e.g. posts/foo.es.md) when <lang>
// is a configured, non-default language, returning the de-languaged path and the language. Anything
// else returns (src, def) — so a file with an unrelated dot (my.notes.md) is untouched.
func splitLangFromPath(src string, langs []string, def string) (string, string) {
	ext := path.Ext(src) // ".md"
	stem := strings.TrimSuffix(src, ext)
	lext := path.Ext(stem) // ".es" or ""
	if lext != "" {
		code := strings.TrimPrefix(lext, ".")
		for _, l := range langs {
			if strings.EqualFold(l, code) && !strings.EqualFold(l, def) {
				return strings.TrimSuffix(stem, lext) + ext, l
			}
		}
	}
	return src, def
}

// resolveLangSlug resolves a document to its routed slug THE way the build does — the single
// source of truth shared by buildPages, Entries (doctor/posts/new/syndicate/serve) and
// AliasConflicts, so tooling never disagrees with the build about where a page publishes:
// strip a `.<lang>` translation infix from the source path, fall back to an explicit
// frontmatter `lang:` when there is no infix, honour the frontmatter slug override, and route
// non-default languages under `/<lang>/…`. transKey is the language-neutral slug that groups
// a post with its sibling translations.
func resolveLangSlug(src string, fm markdown.Frontmatter, langs []string, defLang string) (slug, transKey, lang string) {
	deLanged, lang := splitLangFromPath(src, langs, defLang)
	if strings.EqualFold(lang, defLang) && fm.Lang != "" {
		lang = fm.Lang
	}
	transKey = slugFor(deLanged, fm.Slug)
	slug = transKey
	if !strings.EqualFold(lang, defLang) {
		slug = path.Join(normalizeSlug(lang), slug)
	}
	return slug, transKey, lang
}

// siteLangs returns the configured language set and default language for slug routing,
// matching what build.Run derives from the first site.
func siteLangs(cfg *config.Config) (langs []string, defLang string) {
	if len(cfg.Sites) == 0 {
		return nil, defaultLang("")
	}
	return cfg.Sites[0].Languages, defaultLang(cfg.Sites[0].Lang)
}

// groupTranslations links pages that share a language-neutral slug (transKey) into each other's
// Translations list, sorted default-first then by code, with the current/default flags set. Pages
// with no sibling translations are left untouched.
func groupTranslations(pages []page, defLang, basePath, baseURL string) {
	byKey := map[string][]int{}
	for i := range pages {
		if pages[i].TransKey != "" {
			byKey[pages[i].TransKey] = append(byKey[pages[i].TransKey], i)
		}
	}
	for _, idxs := range byKey {
		if len(idxs) < 2 {
			continue
		}
		base := make([]translation, 0, len(idxs))
		for _, j := range idxs {
			base = append(base, translation{
				Lang:    pages[j].Lang,
				Label:   langLabel(pages[j].Lang),
				URL:     basePath + pages[j].URL,
				Abs:     absURL(baseURL, pages[j].URL),
				Default: strings.EqualFold(pages[j].Lang, defLang),
			})
		}
		sort.SliceStable(base, func(a, b int) bool {
			if base[a].Default != base[b].Default {
				return base[a].Default
			}
			return base[a].Lang < base[b].Lang
		})
		for _, j := range idxs {
			list := make([]translation, len(base))
			copy(list, base)
			for k := range list {
				list[k].Current = strings.EqualFold(list[k].Lang, pages[j].Lang)
			}
			pages[j].Translations = list
		}
	}
}

// defaultTranslation reports whether a page's translation group includes the site-default-
// language version, and whether the page carrying ts IS that version. (false, false) for a
// monolingual page.
func defaultTranslation(ts []translation) (hasDefault, isCurrentDefault bool) {
	for _, t := range ts {
		if t.Default {
			hasDefault = true
			if t.Current {
				isCurrentDefault = true
			}
		}
	}
	return hasDefault, isCurrentDefault
}

// transVars projects a page's translations into the template context.
func transVars(ts []translation) []map[string]any {
	out := make([]map[string]any, len(ts))
	for i, t := range ts {
		out[i] = map[string]any{
			"lang": t.Lang, "label": t.Label, "url": t.URL,
			"current": t.Current, "default": t.Default,
		}
	}
	return out
}
