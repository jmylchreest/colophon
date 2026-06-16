// Package r2 implements the "cloudflare-r2" publisher: it uploads the built tree to an
// S3-compatible object store (Cloudflare R2, or any S3/MinIO via an explicit endpoint).
// It exists so large or numerous assets (images) can be served from object storage instead
// of consuming a Pages/Workers deployment's file budget — paired with site routing, which
// sends matched paths here and rewrites their URLs to the store's public base.
//
// Credentials never pass through config: the access key id and secret are read from the
// environment (R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY, falling back to AWS_*). Bucket,
// account/endpoint and the public base URL come from the publisher config.
package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

func init() {
	publish.Register("cloudflare-r2", New)
	// R2 uses S3 data-plane keys (AWS_* are accepted as fallbacks) and CLOUDFLARE_API_TOKEN
	// for the control-plane discovery / r2.dev enable.
	publish.RegisterEnv("cloudflare-r2", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY", "CLOUDFLARE_API_TOKEN")
}

// bucketNameRE matches the S3/R2 naming rules below the length check: lowercase letters,
// numbers and hyphens, beginning and ending with a letter or number (no dots).
var bucketNameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// validateBucketName checks a bucket name against the S3/R2 rules, so a typo fails with a
// clear message up front rather than a 400 from the API mid-publish.
func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return fmt.Errorf("must be 3–63 characters")
	}
	if !bucketNameRE.MatchString(name) {
		return fmt.Errorf("must be lowercase letters, numbers and hyphens, starting and ending with a letter or number")
	}
	return nil
}

// New builds an R2 publisher. Required: bucket, and either account_id (→ the R2 endpoint)
// or an explicit endpoint (for generic S3/MinIO). region defaults to "auto" (R2).
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	get := func(k string) string { s, _ := cfg.Settings[k].(string); return strings.TrimSpace(s) }
	bucket := get("bucket")
	if bucket == "" {
		return nil, fmt.Errorf("r2 publisher %q: 'bucket' is required", cfg.ID)
	}
	if err := validateBucketName(bucket); err != nil {
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
	p := &Publisher{
		id:             cfg.ID,
		bucket:         bucket,
		endpoint:       strings.TrimRight(endpoint, "/"),
		region:         region,
		location:       get("location"),
		description:    get("description"),
		publicURL:      strings.TrimRight(get("public_url"), "/"),
		deleteOrphaned: deleteOrphaned,
		accessKey:      firstEnv("R2_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID"),
		secretKey:      firstEnv("R2_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY"),
		client:         &http.Client{Timeout: 2 * time.Minute},
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
	bucket         string
	endpoint       string
	region         string
	location       string
	description    string
	publicURL      string
	deleteOrphaned bool
	cf             *cfAPI
	accessKey      string
	secretKey      string
	client         *http.Client
	log            *clog.Logger
}

func (p *Publisher) SetLogger(l *clog.Logger) { p.log = l }

func (p *Publisher) ID() string     { return p.id }
func (p *Publisher) Driver() string { return "cloudflare-r2" }

func (p *Publisher) ensureCreds() error {
	if p.accessKey == "" || p.secretKey == "" {
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
	state, err := p.listObjects(ctx)
	return state, true, err
}

// Hash fingerprints an object for incremental upload. MD5 is intentional: it matches the R2
// ETag for non-multipart PUTs, so unchanged objects skip re-upload (see publish.MD5Hex).
func (p *Publisher) Hash(name string, b []byte) string { return publish.MD5Hex(b) }

// Protected keeps the provenance manifest (.well-known/colophon.json, written by
// WriteManifest) from being deleted as an orphan, since the build tree never contains it.
func (p *Publisher) Protected(name string) bool {
	return strings.HasPrefix(name, ".well-known/")
}

// Put uploads an object; Delete removes one — the per-file apply surface the planner drives.
func (p *Publisher) Put(ctx context.Context, name string, b []byte) error {
	if err := p.put(ctx, name, b); err != nil {
		return err
	}
	p.log.Detail("PUBLISH", p.id, "put", name, "bytes", len(b))
	return nil
}

func (p *Publisher) Delete(ctx context.Context, name string) error {
	if err := p.deleteObject(ctx, name); err != nil {
		return err
	}
	p.log.Detail("PUBLISH", p.id, "delete", name)
	return nil
}

func (p *Publisher) Commit(ctx context.Context, tree fs.FS, plan *core.Plan) (core.Result, error) {
	if err := p.ensureCreds(); err != nil {
		return core.Result{}, err
	}
	return publish.CommitFiles(ctx, tree, p, plan, p.deleteOrphaned)
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
	}{m, p.description, p.bucket, p.publicURL}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return p.put(ctx, manifestKey, b)
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
	exists, err := p.bucketExists(ctx)
	if err != nil {
		return false, err
	}
	created := false
	if !exists {
		resp, err := p.do(ctx, http.MethodPut, "/"+p.bucket, createBucketBody(p.location))
		if err != nil {
			return false, err
		}
		status := resp.StatusCode
		drain(resp)
		switch {
		case status/100 == 2:
			created = true
		case status == http.StatusConflict: // already owned (race) — fine
		default:
			return false, fmt.Errorf("r2 create bucket %s: %s", p.bucket, http.StatusText(status))
		}
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
	return created, nil
}

func (p *Publisher) bucketExists(ctx context.Context) (bool, error) {
	resp, err := p.do(ctx, http.MethodHead, "/"+p.bucket, nil)
	if err != nil {
		return false, err
	}
	defer drain(resp)
	switch resp.StatusCode {
	case http.StatusNotFound:
		return false, nil
	case http.StatusOK, http.StatusForbidden: // 403: exists but listing denied — treat as present
		return true, nil
	default:
		return false, fmt.Errorf("r2 head bucket %s: %s", p.bucket, resp.Status)
	}
}

// listResult is the subset of an S3 ListObjectsV2 response colophon needs.
type listResult struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key  string `xml:"Key"`
		ETag string `xml:"ETag"`
	} `xml:"Contents"`
}

// listObjects enumerates the bucket via ListObjectsV2 (paginated) into a key → ETag (MD5)
// manifest. The signed canonical query string is built by url.Values.Encode (sorted, encoded).
func (p *Publisher) listObjects(ctx context.Context) (core.State, error) {
	state := core.State{}
	token := ""
	for {
		params := url.Values{"list-type": {"2"}}
		if token != "" {
			params.Set("continuation-token", token)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/"+p.bucket, nil)
		if err != nil {
			return nil, err
		}
		req.URL.RawQuery = params.Encode()
		signV4(req, "/"+p.bucket, p.accessKey, p.secretKey, p.region, emptyPayloadHash, now())
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode/100 != 2 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			drain(resp)
			return nil, fmt.Errorf("r2 list %s: %s: %s", p.bucket, resp.Status, strings.TrimSpace(string(msg)))
		}
		var lr listResult
		err = xml.NewDecoder(resp.Body).Decode(&lr)
		drain(resp)
		if err != nil {
			return nil, fmt.Errorf("r2 list %s: %w", p.bucket, err)
		}
		for _, c := range lr.Contents {
			state[c.Key] = strings.Trim(c.ETag, `"`)
		}
		if !lr.IsTruncated || lr.NextContinuationToken == "" {
			return state, nil
		}
		token = lr.NextContinuationToken
	}
}

// deleteObject removes an object; a missing object is not an error (already gone).
func (p *Publisher) deleteObject(ctx context.Context, key string) error {
	resp, err := p.do(ctx, http.MethodDelete, p.objectPath(key), nil)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("r2 delete %s: %s", key, resp.Status)
	}
	return nil
}

// put uploads an object with a content type inferred from its extension.
func (p *Publisher) put(ctx context.Context, key string, body []byte) error {
	resp, err := p.do(ctx, http.MethodPut, p.objectPath(key), body)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("r2 put %s: %s: %s", key, resp.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

func (p *Publisher) objectPath(key string) string { return "/" + p.bucket + "/" + encodeKey(key) }

// do builds, signs and sends a request to an already-encoded path. A non-nil body is sent
// with its SHA-256 and a content type inferred from the path; a nil body signs as empty.
func (p *Publisher) do(ctx context.Context, method, encodedPath string, body []byte) (*http.Response, error) {
	hash := emptyPayloadHash
	var r io.Reader
	if body != nil {
		hash = hexSHA256(body)
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.endpoint+encodedPath, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
		if ct := mime.TypeByExtension(path.Ext(encodedPath)); ct != "" {
			req.Header.Set("Content-Type", ct)
		}
	}
	signV4(req, encodedPath, p.accessKey, p.secretKey, p.region, hash, now())
	return p.client.Do(req)
}

func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// createBucketBody returns the CreateBucket request body carrying the location hint, or nil
// (auto-locate) when no location is configured. Location values are restricted tokens.
func createBucketBody(location string) []byte {
	if location == "" {
		return nil
	}
	return []byte("<CreateBucketConfiguration><LocationConstraint>" + location + "</LocationConstraint></CreateBucketConfiguration>")
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// now is overridable in tests.
var now = func() time.Time { return time.Now() }
