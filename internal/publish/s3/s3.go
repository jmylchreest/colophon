// Package s3 implements the generic "s3" publisher and the "tigris" alias: it uploads the
// built tree to any S3-compatible object store using the shared S3 wire client
// (internal/publish/s3common). Unlike the cloudflare-r2 driver it carries no control-plane
// code at all — bucket creation and uploads are pure S3 data-plane calls (SigV4), so it works
// with Tigris, MinIO, Backblaze B2, Wasabi, Amazon S3, or anything that speaks S3, with no
// provider SDK.
//
// Tigris (Fly.io's global object store) is registered as a friendly alias with its endpoint
// and region defaulted, so a `driver: tigris` block needs only a bucket. Tigris is plain S3:
// no flyctl, no Fly SDK, no control-plane token — the credentials (its tid_/tsec_ keys) come
// from the environment like any S3 store. Making a bucket public and attaching a custom domain
// are one-time bucket settings done in the Tigris dashboard, so they stay out of the publish
// path; set public_url to the resulting domain.
//
// Credentials never pass through config — they come from the environment (AWS_* for generic
// S3; TIGRIS_* falling back to AWS_* for Tigris). The glue below mirrors the cloudflare-r2
// publisher minus the Cloudflare bits; if a third S3 backend appears, hoist the shared
// Publisher glue into s3common (it isn't worth a base type for two callers).
package s3

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

// tigrisEndpoint is Tigris's global S3 endpoint for access from outside Fly.io (from inside a
// Fly app, fly.storage.tigris.dev is faster, but a publish runs from CI or a laptop). region
// is "auto". Public objects serve from a per-account domain (t3.storage.dev or, for newer
// accounts, t3.tigrisfiles.io) or a custom domain — account-dependent, so we don't guess a
// public URL: set public_url to whatever the bucket actually serves at.
const tigrisEndpoint = "https://t3.storage.dev"

func init() {
	publish.Register("s3", New)
	publish.Register("tigris", New)
	publish.RegisterEnv("s3", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
	publish.RegisterEnv("tigris",
		"TIGRIS_ACCESS_KEY_ID", "TIGRIS_SECRET_ACCESS_KEY", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
}

// New builds a generic S3 publisher. Required: bucket, and (for the bare "s3" driver) an
// endpoint. The "tigris" alias defaults the endpoint and region.
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	get := func(k string) string { s, _ := cfg.Settings[k].(string); return strings.TrimSpace(s) }
	bucket := get("bucket")
	if bucket == "" {
		return nil, fmt.Errorf("%s publisher %q: 'bucket' is required", driverName(cfg.Driver), cfg.ID)
	}
	if err := s3common.ValidateBucketName(bucket); err != nil {
		return nil, fmt.Errorf("%s publisher %q: invalid bucket name %q: %w", driverName(cfg.Driver), cfg.ID, bucket, err)
	}

	endpoint := get("endpoint")
	region := get("region")
	var accessKey, secretKey string
	switch cfg.Driver {
	case "tigris":
		if endpoint == "" {
			endpoint = tigrisEndpoint
		}
		if region == "" {
			region = "auto"
		}
		accessKey = publish.FirstEnv("TIGRIS_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID")
		secretKey = publish.FirstEnv("TIGRIS_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY")
	default: // "s3"
		if endpoint == "" {
			return nil, fmt.Errorf("s3 publisher %q: 'endpoint' is required (e.g. https://s3.us-east-1.amazonaws.com)", cfg.ID)
		}
		if region == "" {
			region = "us-east-1"
		}
		accessKey = os.Getenv("AWS_ACCESS_KEY_ID")
		secretKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
	}

	deleteOrphaned := true
	if v, ok := cfg.Settings["delete_orphaned"].(bool); ok {
		deleteOrphaned = v
	}
	s3 := s3common.New(endpoint, bucket, region, strings.TrimSpace(accessKey), strings.TrimSpace(secretKey))
	s3.Name = cfg.ID
	return &Publisher{
		id:             cfg.ID,
		driver:         cfg.Driver,
		s3:             s3,
		location:       get("location"),
		description:    get("description"),
		publicURL:      strings.TrimRight(get("public_url"), "/"),
		deleteOrphaned: deleteOrphaned,
	}, nil
}

// driverName returns a label for error messages ("s3"/"tigris"), defaulting to "s3".
func driverName(d string) string {
	if d == "" {
		return "s3"
	}
	return d
}

type Publisher struct {
	id             string
	driver         string
	s3             *s3common.Client
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

func (p *Publisher) ID() string { return p.id }

func (p *Publisher) Driver() string { return driverName(p.driver) }

func (p *Publisher) ensureCreds() error {
	if p.s3.AccessKey == "" || p.s3.SecretKey == "" {
		want := "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY"
		if p.driver == "tigris" {
			want = "TIGRIS_ACCESS_KEY_ID and TIGRIS_SECRET_ACCESS_KEY (or AWS_*)"
		}
		return fmt.Errorf("%s publisher %q: set %s", p.Driver(), p.id, want)
	}
	return nil
}

// Deployed lists the bucket into a key → ETag (MD5) manifest so the shared planner can diff the
// tree against it: only new/changed objects upload, orphaned ones delete.
func (p *Publisher) Deployed(ctx context.Context) (core.State, bool, error) {
	if err := p.ensureCreds(); err != nil {
		return nil, false, err
	}
	state, err := p.s3.List(ctx)
	return state, true, err
}

// Hash fingerprints an object for incremental upload. MD5 matches the S3 ETag for
// non-multipart PUTs, so unchanged objects skip re-upload (see publish.MD5Hex).
func (p *Publisher) Hash(name string, b []byte) string { return publish.MD5Hex(b) }

// Protected keeps the provenance manifest and the content-addressed search index (_search/) from
// orphan-deletion, so several environments can share one bucket without pruning each other (see r2).
func (p *Publisher) Protected(name string) bool {
	return strings.HasPrefix(name, ".well-known/") || strings.HasPrefix(name, "_search/")
}

func (p *Publisher) Commit(ctx context.Context, tree fs.FS, plan *core.Plan) (core.Result, error) {
	if err := p.ensureCreds(); err != nil {
		return core.Result{}, err
	}
	res, err := publish.CommitFiles(ctx, tree, p.s3, plan, p.deleteOrphaned)
	if err == nil && p.log != nil {
		p.log.Detail("PUBLISH", p.id, "committed",
			"uploaded", res.Uploaded, "deleted", res.Deleted, "bytes", res.Bytes)
	}
	if err == nil {
		p.applyRedirects(ctx, tree)
	}
	return res, err
}

// applyRedirects upgrades alias stubs to real 301s on S3 *website* hosting by setting the
// x-amz-website-redirect-location header on each redirect object (read from the build's
// _redirects file). Best-effort: the meta-refresh stub already works without it, so a failure
// only warns. No-op when this publisher's tree carries no _redirects (e.g. an assets-only
// target) or on stores that ignore the header (R2/MinIO without website hosting).
func (p *Publisher) applyRedirects(ctx context.Context, tree fs.FS) {
	data, err := fs.ReadFile(tree, "_redirects")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) < 2 || strings.HasPrefix(f[0], "#") {
			continue
		}
		key := strings.Trim(f[0], "/")
		if key == "" {
			continue
		}
		key += "/index.html"
		body, err := fs.ReadFile(tree, key)
		if err != nil {
			continue // stub not in this tree (routing/subpath) — leave it as meta-refresh
		}
		if err := p.s3.PutRedirect(ctx, key, f[1], body); err != nil && p.log != nil {
			p.log.Step("PUBLISH", p.id, "redirect_warning", key, "err", err.Error())
		}
	}
}

func (p *Publisher) Invalidate(ctx context.Context, paths []string) error { return nil }

// CanonicalURL is the configured public_url (or "" if unset). Unlike cloudflare-r2 there is no
// control-plane lookup to discover it: a generic S3/Tigris public domain is account-dependent,
// so set public_url to the bucket's serving domain or a custom domain.
func (p *Publisher) CanonicalURL(ctx context.Context) (string, error) {
	return p.publicURL, nil
}

const manifestKey = ".well-known/colophon.json"

// WriteManifest records provenance at manifestKey so the bucket names its blog and links back
// to the canonical site, sitemap and feeds. The object is public when the bucket is.
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

// Provision creates the bucket if missing (`publish --create`); it's idempotent. It does not
// make the bucket public — for Tigris that's a one-time dashboard setting (no data-plane API),
// which keeps provisioning SDK-free; enable public access there, then set public_url.
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
	// Allow cross-origin GET so a routed search index / assets are fetchable from the site origin
	// (<img> is CORS-exempt but fetch()/ES-module import are not). Runs for an existing bucket too;
	// warn and continue on failure (a store may not support PutBucketCors).
	if err := p.s3.PutCORS(ctx, []string{"*"}); err != nil && p.log != nil {
		p.log.Step("PUBLISH", p.id, "warning", "could not set CORS policy (cross-origin fetch may fail): "+err.Error())
	}
	return created, nil
}
