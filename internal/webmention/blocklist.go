package webmention

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// Blocklist is the committed, declarative spam filter: glob rules over a mention's attributes,
// applied at fetch (and again at build, defensively) so it survives the full-regenerate cache.
// A bare string matches the author/source domain or author URL; a {field: glob} mapping targets
// one field (domain, url, author.name, author.url, content, type).
//
//	# .colophon/webmention-block.yml
//	- "*.spam.example"
//	- author.url: "https://troll.example/*"
//	- content: "*free crypto*"
type Blocklist struct{ rules []blockRule }

type blockRule struct {
	field string
	glob  string
	re    *regexp.Regexp
}

// BlocklistPath is the committed blocklist location under a project root.
func BlocklistPath(root string) string {
	return filepath.Join(root, ".colophon", "webmention-block.yml")
}

// LoadBlocklist reads the blocklist; a missing file yields an empty (no-op) list.
func LoadBlocklist(root string) (*Blocklist, error) {
	b, err := os.ReadFile(BlocklistPath(root))
	if os.IsNotExist(err) {
		return &Blocklist{}, nil
	}
	if err != nil {
		return nil, err
	}
	var items []yaml.Node
	if err := yaml.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	bl := &Blocklist{}
	for _, it := range items {
		switch it.Kind {
		case yaml.ScalarNode:
			bl.add("", it.Value)
		case yaml.MappingNode:
			for i := 0; i+1 < len(it.Content); i += 2 {
				bl.add(it.Content[i].Value, it.Content[i+1].Value)
			}
		}
	}
	return bl, nil
}

func (b *Blocklist) add(field, glob string) {
	glob = strings.TrimSpace(glob)
	if glob == "" {
		return
	}
	b.rules = append(b.rules, blockRule{
		field: strings.ToLower(strings.TrimSpace(field)),
		glob:  glob,
		re:    globRegex(glob),
	})
}

// Empty reports whether the blocklist has no rules (so callers can skip work).
func (b *Blocklist) Empty() bool { return b == nil || len(b.rules) == 0 }

// Match reports whether a mention hits any rule.
func (b *Blocklist) Match(m Mention) bool {
	if b.Empty() {
		return false
	}
	for _, r := range b.rules {
		for _, v := range ruleValues(r.field, m) {
			if v != "" && r.re.MatchString(strings.ToLower(v)) {
				return true
			}
		}
	}
	return false
}

// Filter returns the mentions that pass (are not blocked), preserving order.
func (b *Blocklist) Filter(ms []Mention) []Mention {
	if b.Empty() {
		return ms
	}
	out := ms[:0:0]
	for _, m := range ms {
		if !b.Match(m) {
			out = append(out, m)
		}
	}
	return out
}

// ClientPatterns returns the raw glob patterns to ship to the browser for live-mode client-side
// filtering. The client matches each against any field (coarser than the server's field-specific
// rules), which is acceptable for spam-hiding; semantic rules (later) never go to the client.
func (b *Blocklist) ClientPatterns() []string {
	if b.Empty() {
		return nil
	}
	out := make([]string, 0, len(b.rules))
	for _, r := range b.rules {
		out = append(out, r.glob)
	}
	return out
}

// ruleValues returns the mention attribute values a rule's field matches against. The empty
// field (bare-string shorthand) matches the author/source domain and the author URL.
func ruleValues(field string, m Mention) []string {
	switch field {
	case "domain":
		return []string{host(m.Author.URL), host(m.URL)}
	case "url":
		return []string{m.URL}
	case "author.url":
		return []string{m.Author.URL}
	case "author.name":
		return []string{m.Author.Name}
	case "content":
		return []string{m.Content}
	case "type":
		return []string{m.Type}
	default: // shorthand
		return []string{host(m.Author.URL), host(m.URL), m.Author.URL}
	}
}

func host(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return ""
}

// globRegex compiles a case-insensitive glob (*, ?) anchored to the whole value.
func globRegex(g string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range g {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
