// Package r2 implements the "cloudflare-r2" publisher: it uploads the built tree to an
// S3-compatible object store (Cloudflare R2, or any S3/MinIO via an explicit endpoint).
// It exists so large or numerous assets (images) can be served from object storage instead
// of consuming a Pages/Workers deployment's file budget — paired with site routing, which
// sends matched paths here and rewrites their URLs to the store's public base.
//
// The S3 wire protocol lives in internal/publish/s3common (shared with other S3 backends);
// this package adds only the Cloudflare control-plane bits (public-URL discovery, r2.dev
// enablement — see cfapi.go / provider.go).
//
// Credentials never pass through config: the access key id and secret are read from the
// environment (R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY, falling back to AWS_*). Bucket,
// account/endpoint and the public base URL come from the publisher config.
package r2

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
	"github.com/jmylchreest/colophon/internal/publish/s3common"
)

func init() {
	publish.Register("cloudflare-r2", New)
	// R2 uses S3 data-plane keys (AWS_* are accepted as fallbacks) and CLOUDFLARE_API_TOKEN
	// for the control-plane discovery / r2.dev enable.
	publish.RegisterEnv("cloudflare-r2", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "CLOUDFLARE_API_TOKEN")
}

// New builds an R2 publisher. Required: bucket, and either account_id (→ the R2 endpoint)
// or an explicit endpoint (for generic S3/MinIO). region defaults to "auto" (R2).
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	get := func(k string) string { s, _ := cfg.Settings[k].(string); return strings.TrimSpace(s) }
	bucket := get("bucket")
	if bucket == "" {
		return nil, fmt.Errorf("r2 publisher %q: 'bucket' is required", cfg.ID)
	}
	if err := s3common.ValidateBucketName(bucket); err != nil {
		return nil, fmt.Errorf("r2 publisher %q: invalid bucket name %q: %w", cfg.ID, bucket, err)
	}
	endpoint := get("endpoint")
	if endpoint == "" {
		account := get("account_id")
		if account == "" {
			return nil, fmt.Errorf("r2 publisher %q: set 'account_id' or 'endpoint'", cfg.ID)
		}
		endpoint = "https://" + account + ".r2.cloudflarestorage.com"
	}
	region := get("region")
	if region == "" {
		region = "auto"
	}
	deleteOrphaned := true
	if v, ok := cfg.Settings["delete_orphaned"].(bool); ok {
		deleteOrphaned = v
	}
	s3 := s3common.New(endpoint, bucket, region,
		firstEnv("R2_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID"),
		firstEnv("R2_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY"))
	s3.Name = cfg.ID
	p := &Publisher{
		id:             cfg.ID,
		s3:             s3,
		location:       get("location"),
		description:    get("description"),
		publicURL:      strings.TrimRight(get("public_url"), "/"),
		deleteOrphaned: deleteOrphaned,
	}
	// A control-plane token (shared with the Pages publisher) enables public-URL discovery
	// for providers that support it; the endpoint→provider glob table (provider.go) decides
	// which lookups actually apply, so a generic S3/MinIO endpoint just ignores it.
	if token := strings.TrimSpace(os.Getenv("CLOUDFLARE_API_TOKEN")); token != "" {
		p.cf = newCFAPI(token, cloudflareAPIBase)
	}
	return p, nil
}

type Publisher struct {
	id             string
	s3             *s3common.Client // the shared S3 wire client
	cf             *cfAPI           // Cloudflare control plane (nil without a token)
	location       string
	description    string
	publicURL      string
	deleteOrphaned bool
	log            *clog.Logger
}

func (p *Publisher) SetLogger(l *clog.Logger) {
	p.log = l
	p.s3.Logger = l // *clog.Logger satisfies s3common.Logger (Detail)
}

func (p *Publisher) ID() string     { return p.id }
func (p *Publisher) Driver() string { return "cloudflare-r2" }

func (p *Publisher) ensureCreds() error {
	if p.s3.AccessKey == "" || p.s3.SecretKey == "" {
		return fmt.Errorf("r2 publisher %q: set R2_ACCESS_KEY_ID and R2_SECRET_ACCESS_KEY", p.id)
	}
	return nil
}

// Deployed lists the bucket into a key → ETag (MD5) manifest. The shared planner diffs the
// tree against it, so only new/changed objects upload and orphaned ones are deleted.
func (p *Publisher) Deployed(ctx context.Context) (core.State, bool, error) {
	if err := p.ensureCreds(); err != nil {
		return nil, false, err
	}
	state, err := p.s3.List(ctx)
	return state, true, err
}

// Hash fingerprints an object for incremental upload. MD5 is intentional: it matches the R2
// ETag for non-multipart PUTs, so unchanged objects skip re-upload (see publish.MD5Hex).
func (p *Publisher) Hash(name string, b []byte) string { return publish.MD5Hex(b) }

// Protected keeps a file from being deleted as an orphan. It covers the provenance manifest
// (.well-known/colophon.json, never in the build tree) and the content-addressed search index
// under _search/ — the latter so several environments can share one bucket without each
// orphan-deleting the others' shards (the shards are immutable and deduped; only each env's own
// manifest file differs).
func (p *Publisher) Protected(name string) bool {
	return strings.HasPrefix(name, ".well-known/") || strings.HasPrefix(name, "_search/")
}

func (p *Publisher) Commit(ctx context.Context, tree fs.FS, plan *core.Plan) (core.Result, error) {
	if err := p.ensureCreds(); err != nil {
		return core.Result{}, err
	}
	res, err := publish.CommitFiles(ctx, tree, p.s3, plan, p.deleteOrphaned)
	if err == nil {
		// An aggregate closing line so --verbose isn't only the per-object put/delete noise.
		p.log.Detail("PUBLISH", p.id, "committed",
			"uploaded", res.Uploaded, "deleted", res.Deleted, "bytes", res.Bytes)
	}
	return res, err
}

func (p *Publisher) Invalidate(ctx context.Context, paths []string) error { return nil }

// CanonicalURL reports the bucket's public base URL: the configured public_url, or one
// discovered from the provider (R2 custom/managed domains), or "".
func (p *Publisher) CanonicalURL(ctx context.Context) (string, error) {
	return p.resolvePublicURL(ctx)
}

// manifestKey is the object key for the provenance manifest. A private-use well-known URI
// (RFC 8615): unregistered, but the namespace signals discoverable origin metadata.
const manifestKey = ".well-known/colophon.json"

// WriteManifest records provenance at manifestKey so the bucket names its blog and links
// back to the canonical site, sitemap and feeds — turning an anonymous bucket into a
// self-describing one. The object is public when the bucket is.
func (p *Publisher) WriteManifest(ctx context.Context, m core.SiteManifest) error {
	if err := p.ensureCreds(); err != nil {
		return err
	}
	if m.Generator == "" {
		m.Generator = "colophon"
	}
	doc := struct {
		core.SiteManifest
		Description string `json:"description,omitempty"`
		Bucket      string `json:"bucket"`
		PublicURL   string `json:"public_url,omitempty"`
	}{m, p.description, p.s3.Bucket, p.publicURL}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return p.s3.Put(ctx, manifestKey, b)
}

// Provision makes the bucket ready for web delivery (`publish --create`): it creates the
// bucket if missing, then ensures public access is enabled. Both steps are idempotent, so
// re-running it fixes a bucket created earlier (e.g. before the API token had R2 access).
// A `location` hint is sent on creation (R2 jurisdiction wnam/enam/weur/eeur/apac/oc, or an
// S3 LocationConstraint); otherwise the store auto-locates. Returns whether it created the
// bucket this run.
func (p *Publisher) Provision(ctx context.Context) (bool, error) {
	if err := p.ensureCreds(); err != nil {
		return false, err
	}
	exists, err := p.s3.Head(ctx)
	if err != nil {
		return false, err
	}
	created := false
	if !exists {
		if err := p.s3.Create(ctx, p.location); err != nil {
			return false, err
		}
		created = true
	}
	// Ensure the bucket is publicly reachable (the provider enables r2.dev only when nothing
	// already exposes it — see r2EnablePublicAccess). Runs for an existing bucket too, and a
	// failure doesn't undo the create — warn and continue.
	if prov := p.ops().provision; prov != nil {
		if err := prov(ctx, p); err != nil {
			p.log.Step("PUBLISH", p.id, "warning",
				"could not enable public access (CLOUDFLARE_API_TOKEN needs R2 Admin Read & Write): "+err.Error())
		}
	}
	// Allow cross-origin GET so routed assets/search are fetchable from the site origin (<img> is
	// CORS-exempt but fetch()/ES-module import are not). Idempotent; warn and continue on failure.
	if err := p.s3.PutCORS(ctx, []string{"*"}); err != nil {
		p.log.Step("PUBLISH", p.id, "warning", "could not set CORS policy (cross-origin fetch of routed search/assets may fail): "+err.Error())
	}
	return created, nil
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
