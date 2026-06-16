package core

import (
	"context"
	"io/fs"
)

// ChangeOp is the kind of change a publisher must apply to converge a target.
type ChangeOp int

const (
	OpUpload ChangeOp = iota
	OpDelete
)

// Change is a single delta between the built tree and a publisher's last-known
// state, computed by diffing content hashes against the per-target manifest.
type Change struct {
	Path string
	Op   ChangeOp
	Hash string
}

// Result summarises one publisher run.
type Result struct {
	// Total is how many files the deployment contains; Uploaded is how many of those
	// were actually transferred this run (the rest were already present, unchanged).
	Total    int
	Uploaded int
	Deleted  int
	Bytes    int64
	// URL is the deployed location when the driver exposes one (e.g. a Pages
	// deployment URL); empty otherwise.
	URL string
}

// Publisher deploys the canonical static tree to one destination. Implementations
// are idempotent: Plan diffs the tree against the target's manifest, Apply uploads
// or deletes only what changed (reading bytes from the same tree), and Invalidate
// busts any CDN cache for the changed paths. Each driver lives in its own package
// under internal/publish and self-registers; core knows only this interface.
// Drivers: local, cloudflare-pages, s3-cloudfront, webdav, rsync-ssh, git-pages.
type Publisher interface {
	// ID is the configured publisher id (e.g. "cf-prod").
	ID() string
	// Driver is the implementation key (e.g. "cloudflare-pages").
	Driver() string
	Plan(ctx context.Context, tree fs.FS) ([]Change, error)
	Apply(ctx context.Context, tree fs.FS, changes []Change) (Result, error)
	Invalidate(ctx context.Context, paths []string) error
}

// Provisioner is an optional Publisher capability: create the destination (e.g. a
// Pages project) if it does not already exist. `publish --create` invokes it before
// deploying. Implementations are idempotent and report whether they created anything.
type Provisioner interface {
	Provision(ctx context.Context) (created bool, err error)
}

// Pruner is an optional Publisher capability: remove old deployments after a publish,
// keeping only the most recent few (the count is the driver's own config). It is
// scoped so it never deletes the deployment just made. Returns how many were removed.
type Pruner interface {
	Prune(ctx context.Context) (removed int, err error)
}

// CanonicalURLer is an optional Publisher capability: report the stable public base URL
// the deploy is reachable at — a custom domain, or the platform's stable alias (e.g.
// Cloudflare's <project>.pages.dev for production, <branch>.<project>.pages.dev for a
// preview branch), NOT the per-deployment URL. colophon uses it as the canonical
// base_url for feeds/sitemap when the project hasn't configured one. Returns "" if the
// publisher cannot determine a URL.
type CanonicalURLer interface {
	CanonicalURL(ctx context.Context) (string, error)
}

// SiteManifest is provenance about the published site that a publisher can record, so a
// destination is self-identifying and links back to its origin. An object store writes it
// to .well-known/colophon.json, turning an otherwise anonymous bucket into something that
// names its blog and points at the canonical site, sitemap and feeds.
type SiteManifest struct {
	Generator string            `json:"generator"`
	Site      string            `json:"site,omitempty"`
	Sitemap   string            `json:"sitemap,omitempty"`
	Feeds     map[string]string `json:"feeds,omitempty"`
}

// ManifestWriter is an optional Publisher capability: record a SiteManifest at the
// destination. It is best-effort provenance, not part of the deploy's success criteria.
type ManifestWriter interface {
	WriteManifest(ctx context.Context, m SiteManifest) error
}
