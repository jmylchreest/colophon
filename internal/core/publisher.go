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

// State is a destination's managed contents as path (slash-separated) → content hash — a
// publisher's "manifest" of what is currently deployed. The generic planner diffs the build
// tree against it.
type State map[string]string

// Plan is the reconciliation between a build tree and a publisher's deployed State, computed
// by the generic incremental planner (publish.Run). Desired is every file in the tree mapped
// to its hash; Upload is the new-or-changed subset (each carrying its Hash); Delete is the
// managed paths that are gone from the tree (orphans), minus any the publisher Protects.
type Plan struct {
	Desired State
	Upload  []Change
	Delete  []Change
}

// Publisher deploys the canonical static tree to one destination. The generic planner
// (publish.Run) diffs the tree against the publisher's Deployed state into a Plan and hands
// it to Commit, so each driver only describes its destination — how to read its state, hash a
// file, and apply changes — and incremental publishing is shared. Each driver lives in its own
// package under internal/publish and self-registers; core knows only this interface.
// Drivers: local, cloudflare-pages, cloudflare-r2, and (later) s3, webdav, rsync-ssh, git-pages.
type Publisher interface {
	// ID is the configured publisher id (e.g. "cf-prod").
	ID() string
	// Driver is the implementation key (e.g. "cloudflare-pages").
	Driver() string
	// Deployed reports the destination's current managed state as path → hash. ok is false
	// when the backend cannot (or need not) enumerate — a content-addressed/transactional
	// platform such as Pages — in which case the planner treats every file as an upload and
	// computes no deletes, leaving removals to the platform's own immutable-snapshot semantics.
	Deployed(ctx context.Context) (state State, ok bool, err error)
	// Hash computes a file's content hash in the same scheme Deployed reports.
	Hash(name string, b []byte) string
	// Protected reports a managed path the planner must never delete (e.g. a provenance
	// manifest the publisher itself writes).
	Protected(name string) bool
	// Commit applies plan to the destination and returns the run summary. Per-file backends
	// honour plan.Upload/plan.Delete; transactional backends use plan.Desired. Total is the
	// size of the deployed tree (len(plan.Desired)). Result.URL is set only when the driver
	// exposes a deployed location (e.g. a Pages deployment URL); it is "" otherwise (e.g. R2).
	Commit(ctx context.Context, tree fs.FS, plan *Plan) (Result, error)
	// Invalidate busts any CDN cache for the changed paths (a no-op where deploys are immutable).
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

// TreePublisher is an alternative Publisher shape for destinations that take the entire built
// tree at once rather than an incremental per-file diff: a git branch (force-push replaces it),
// or an external deploy CLI (surge, netlify, rsync — handed a directory and asked to deploy it).
// The whole tree is the desired state; there is no plan to compute, so the publish orchestrator
// dispatches a driver implementing this to Push instead of the incremental publish.Run.
//
// A TreePublisher must still satisfy Publisher (for driver listing / capability checks); its
// Commit should return an error explaining that the driver uses Push, and Deployed/Hash may
// return safe zero values.
type TreePublisher interface {
	Push(ctx context.Context, tree fs.FS) (Result, error)
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
