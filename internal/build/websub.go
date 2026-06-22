package build

import (
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// webSubHubs returns the configured WebSub hub URLs (trimmed, blank-dropped,
// de-duplicated, order preserved), or nil. Advertised in every feed as rel="hub".
func webSubHubs(site core.Site) []string {
	if site.Federation.WebSub == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, h := range site.Federation.WebSub.Hubs {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}
