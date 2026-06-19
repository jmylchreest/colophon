package build

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"

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
//   - a source-relative file → resolved via LocateAsset (each source's driver-specific Resolve,
//     then the project root as a portable fallback), published once to assets/<basename>, and
//     emitted as router.AssetURL(out) when an assets rule routes it, else
//     basePath+"assets/"+basename (root-anchored, so it is correct at every depth).
//   - a file nothing can source → warned (a likely broken link, matching the ASSET "missing"
//     warning) and the avatar is cleared so no broken <img src> is emitted.
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
		src, ok := LocateAsset(ctx, srcs, cfg.Root, a.Avatar)
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

// LocateAsset returns the first source that can source ref. It mirrors markdown-embed resolution
// (each source's driver-specific Resolve) and then falls back to the project root, so a
// project-level ref like an author avatar is portable across drivers: the same `avatar:
// assets/x.png` resolves whether the file lives in a source's tree (the obsidian vault, a
// content dir) or the project's own assets/. The returned source can Open the ref to read it.
func LocateAsset(ctx context.Context, srcs []core.Source, root, ref string) (core.Source, bool) {
	for _, s := range srcs {
		if _, ok := s.Resolve(ctx, ref); ok {
			return s, true
		}
	}
	rs := rootSource{root}
	if _, ok := rs.Resolve(ctx, ref); ok {
		return rs, true
	}
	return nil, false
}

// rootSource resolves assets relative to the project root — the driver-independent fallback for
// project-level refs (an author avatar, a favicon) that don't belong to any one content source.
type rootSource struct{ root string }

func (rootSource) ID() string                                        { return "(project)" }
func (rootSource) Driver() string                                    { return "root" }
func (rootSource) Documents(context.Context) ([]core.Content, error) { return nil, nil }

func (s rootSource) Resolve(_ context.Context, ref string) (string, bool) {
	p := filepath.Join(s.root, filepath.FromSlash(ref))
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
		return p, true
	}
	return "", false
}

func (s rootSource) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	p, ok := s.Resolve(ctx, ref)
	if !ok {
		return nil, os.ErrNotExist
	}
	return os.Open(p)
}

// MissingRef is a defined local file reference that can't be sourced.
type MissingRef struct {
	Kind  string // "avatar" | "hero" | "image" | "embed"
	Owner string // author id, or the post that references it
	Ref   string // the reference as written
}

// MissingAssets dry-resolves every DEFINED local file reference and returns those that can't be
// sourced — the machinery doctor uses to preflight broken references without a full build. Author
// avatars resolve project-level (any source, then the project root, so the value is portable);
// post hero/image and markdown embeds resolve against the post's own source (dir-relative),
// mirroring the build. data:/http(s) and unset references are skipped, and it reads no bytes.
func MissingAssets(cfg *config.Config) ([]MissingRef, error) {
	docs, err := gatherDocuments(cfg, clog.Discard())
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	var missing []MissingRef

	srcs := uniqueSources(docs)
	for _, a := range cfg.Authors {
		if a.Avatar != "" && localRef(a.Avatar) {
			if _, ok := LocateAsset(ctx, srcs, cfg.Root, a.Avatar); !ok {
				missing = append(missing, MissingRef{Kind: "avatar", Owner: a.ID, Ref: a.Avatar})
			}
		}
	}

	for _, sd := range docs {
		owner := sd.doc.Frontmatter.Title
		if owner == "" {
			owner = sd.doc.SourcePath
		}
		dir := path.Dir(sd.doc.SourcePath)
		check := func(kind, ref string) {
			if ref == "" || !localRef(ref) {
				return
			}
			if _, ok := sd.src.Resolve(ctx, path.Clean(path.Join(dir, ref))); !ok {
				missing = append(missing, MissingRef{Kind: kind, Owner: owner, Ref: ref})
			}
		}
		check("hero", sd.doc.Frontmatter.Hero)
		check("image", sd.doc.Frontmatter.Image)
		for _, ref := range imageRefs(sd.doc.Body) {
			check("embed", ref)
		}
	}
	return missing, nil
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
