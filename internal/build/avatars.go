package build

import (
	"context"
	"path"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
)

// resolveAuthorAvatars turns a file-path author `avatar:` into a published, depth-independent
// asset, mirroring how a markdown image embed is published and routed.
//
// The avatar appears in the topbar on every page (all depths) and the strip is built once for
// the whole site, so the emitted src must be depth-independent. Unlike a co-located markdown
// asset (published beside its page), an avatar is published once at the site root under
// public/assets/<basename> and referenced root-anchored:
//
//   - data: or http(s):// → left untouched (a self-contained or hosted avatar).
//   - a source-relative file → resolved through the content sources (first source whose
//     Open succeeds wins, the same way markdown embeds resolve), published once to
//     assets/<basename>, and emitted as router.AssetURL(out) when an assets rule routes it,
//     else basePath+"assets/"+basename (root-anchored, so it is correct at every depth).
//   - a file that no source can open → warned (a likely broken link, matching the ASSET
//     "missing" warning) and the avatar is cleared so no broken <img src> is emitted.
//
// It mutates cfg.Authors in place so every consumer (the topbar/author-page strip via
// collectAuthors, and the per-page byline via authorVars) reads the resolved value, and
// returns the extra asset refs to publish. seen is the build's dedup set keyed by output
// path, shared with markdown refs so an avatar already carried by a markdown ref isn't copied
// twice.
func resolveAuthorAvatars(cfg *config.Config, srcs []core.Source, router *core.Router, basePath string, seen map[string]bool, log *clog.Logger) []assetRef {
	var assets []assetRef
	ctx := context.Background()
	for i := range cfg.Authors {
		a := &cfg.Authors[i]
		if a.Avatar == "" || !localRef(a.Avatar) {
			continue // data:/http(s):// or empty → pass through untouched
		}
		src, ok := openFromSources(ctx, srcs, a.Avatar)
		if !ok {
			// No source resolves it: warn (no silent 404) and drop it so the theme falls
			// back to initials rather than emitting a broken src.
			log.Step("ASSET", "avatar", "missing", a.Avatar)
			a.Avatar = ""
			continue
		}
		name := path.Base(path.Clean(a.Avatar))
		out := path.Join("assets", name)
		if !seen[out] {
			seen[out] = true
			assets = append(assets, assetRef{src: src, srcPath: a.Avatar, outPath: out})
		}
		if url := router.AssetURL(out); url != "" {
			a.Avatar = url // routed to the object store (e.g. R2)
		} else {
			a.Avatar = basePath + out // root-anchored, depth-independent
		}
	}
	return assets
}

// openFromSources reports the first source that can open ref (mirroring how markdown embeds
// resolve against their owning source); it closes the probe reader, since the build's
// asset-copy pass re-opens through the returned source.
func openFromSources(ctx context.Context, srcs []core.Source, ref string) (core.Source, bool) {
	for _, s := range srcs {
		if rc, err := s.Open(ctx, ref); err == nil {
			_ = rc.Close()
			return s, true
		}
	}
	return nil, false
}

// uniqueSources returns the distinct sources behind the gathered documents, preserving the
// order in which they first appear (so resolution honours source precedence).
func uniqueSources(docs []sourceDoc) []core.Source {
	var out []core.Source
	seen := map[string]bool{}
	for _, sd := range docs {
		if sd.src == nil || seen[sd.src.ID()] {
			continue
		}
		seen[sd.src.ID()] = true
		out = append(out, sd.src)
	}
	return out
}
