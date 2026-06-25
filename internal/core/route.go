package core

import (
	"regexp"
	"strings"
)

// Router applies a site's RouteRules: it decides which publisher owns an output path and,
// for asset paths bound to an object store, the absolute URL the build should rewrite
// references to. A rule is only active when it carries a base_url (the store's public
// base) — without one there is nowhere to rewrite to, so routing is a no-op and every file
// goes to the default publishers. This keeps a fixture that wires routing fully functional
// before the object store's URL/credentials exist.
type Router struct {
	routes []compiledRoute
}

type compiledRoute struct {
	re        *regexp.Regexp
	publisher string
	baseURL   string
}

// NewRouter compiles the rules in effect for a build/deploy. A rule is active only when it
// has a base_url (somewhere to rewrite to) *and* its target publisher is among the deploying
// publishers — so routing never strips assets that nothing will upload. With no deploying
// publishers (a plain build or serve), routing is inert and every asset stays co-located.
func NewRouter(rules []RouteRule, deployingPublishers []string) *Router {
	if len(deployingPublishers) == 0 {
		return &Router{}
	}
	active := make(map[string]bool, len(deployingPublishers))
	for _, p := range deployingPublishers {
		active[p] = true
	}
	var rs []compiledRoute
	for _, r := range rules {
		if strings.TrimSpace(r.BaseURL) == "" || !active[r.Publisher] {
			continue
		}
		rs = append(rs, compiledRoute{
			re:        regexp.MustCompile(globToRegex(r.Match)),
			publisher: r.Publisher,
			baseURL:   strings.TrimRight(r.BaseURL, "/"),
		})
	}
	return &Router{routes: rs}
}

// Active reports whether any rule is in effect.
func (r *Router) Active() bool { return r != nil && len(r.routes) > 0 }

// AssetURL returns the absolute public URL for an output path routed to an object store,
// or "" if no active rule matches (the asset stays co-located and served relatively).
func (r *Router) AssetURL(outPath string) string {
	if r == nil {
		return ""
	}
	for _, c := range r.routes {
		if c.re.MatchString(outPath) {
			return c.baseURL + "/" + outPath
		}
	}
	return ""
}

// Owns reports whether any active rule targets the given publisher.
func (r *Router) Owns(publisher string) bool {
	if r == nil {
		return false
	}
	for _, c := range r.routes {
		if c.publisher == publisher {
			return true
		}
	}
	return false
}

// Keep reports whether the given publisher should receive the file at outPath. A route's
// target publisher receives only the files its rules match; any other (default) publisher
// receives only files no rule claims.
func (r *Router) Keep(publisher, outPath string) bool {
	if r == nil || len(r.routes) == 0 {
		return true
	}
	matchedAny, owns := false, false
	for _, c := range r.routes {
		if c.publisher == publisher {
			owns = true
		}
		if c.re.MatchString(outPath) {
			if c.publisher == publisher {
				return true
			}
			matchedAny = true
		}
	}
	if owns {
		return false // a route-target publisher gets only its own routed files
	}
	return !matchedAny // a default publisher gets only unrouted files
}

// globToRegex converts a path glob to an anchored regexp. "**" matches any characters
// including "/"; "*" matches within a single path segment; "?" matches one non-slash char.
func globToRegex(pattern string) string {
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// "**/" matches any number of leading path segments *including none*, so a
				// pattern like "**/assets/**" matches a root-level "assets/x", not just a
				// nested "dir/assets/x". A bare "**" (not a path segment) matches anything.
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					b.WriteString("(?:.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '[', ']', '{', '}', '^', '$', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('$')
	return b.String()
}
