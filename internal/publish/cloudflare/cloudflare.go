// Package cloudflare implements the "cloudflare-pages" publisher: it deploys the
// built tree to a Cloudflare Pages project via the direct-upload API (no wrangler).
//
// The API token is read only from the CLOUDFLARE_API_TOKEN environment variable, never
// from config, so deploy secrets stay out of the project and the agent layer.
package cloudflare

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"lukechampine.com/blake3"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

func init() {
	publish.Register("cloudflare-pages", New)
	publish.RegisterEnv("cloudflare-pages", "CLOUDFLARE_ACCOUNT_ID", "CLOUDFLARE_API_TOKEN")
}

// upload batch limits, kept well under the API's per-request ceilings.
const (
	maxBatchFiles = 1000
	maxBatchBytes = 40 << 20
)

type Publisher struct {
	id        string
	project   string
	accountID string
	branch    string
	prune     pruneSpec // how many old deployments to keep for this branch after a deploy
	api       *apiClient
	log       *clog.Logger
}

func (p *Publisher) SetLogger(l *clog.Logger) { p.log = l }

// New builds a cloudflare-pages publisher. Required: "project" (config) and a
// CLOUDFLARE_API_TOKEN env var. account_id comes from config or CLOUDFLARE_ACCOUNT_ID;
// branch defaults to "main" (deploy to the project's production branch to update the
// main domain). prune defaults to keeping the newest deployment per branch; it accepts
// a count (>=1), a duration ("3w"), or "never"/0 to keep all.
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	p := &Publisher{id: cfg.ID, branch: "main", prune: pruneSpec{mode: pruneCount, count: 1}}

	p.project, _ = cfg.Settings["project"].(string)
	if p.project == "" {
		return nil, fmt.Errorf("cloudflare-pages %q: missing 'project'", cfg.ID)
	}
	if b, ok := cfg.Settings["branch"].(string); ok && b != "" {
		p.branch = b
	}
	if v, ok := cfg.Settings["prune"]; ok {
		spec, err := parsePrune(v)
		if err != nil {
			return nil, fmt.Errorf("cloudflare-pages %q: invalid prune: %w", cfg.ID, err)
		}
		p.prune = spec
	}
	if p.accountID, _ = cfg.Settings["account_id"].(string); p.accountID == "" {
		p.accountID = os.Getenv("CLOUDFLARE_ACCOUNT_ID")
	}
	if p.accountID == "" {
		return nil, fmt.Errorf("cloudflare-pages %q: missing account_id (set it in config or CLOUDFLARE_ACCOUNT_ID)", cfg.ID)
	}

	token := os.Getenv("CLOUDFLARE_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("cloudflare-pages %q: CLOUDFLARE_API_TOKEN is not set", cfg.ID)
	}
	p.api = newAPIClient(token)
	return p, nil
}

func (p *Publisher) ID() string     { return p.id }
func (p *Publisher) Driver() string { return "cloudflare-pages" }

// Deployed returns ok=false: Pages is content-addressed and transactional, so the planner
// marks every file an upload (no enumeration, no deletes) and Commit lets the server decide
// what to transfer (check-missing) and which paths to serve (the full deployment manifest).
func (p *Publisher) Deployed(ctx context.Context) (core.State, bool, error) { return nil, false, nil }

func (p *Publisher) Hash(name string, b []byte) string { return hashAsset(name, b) }
func (p *Publisher) Protected(name string) bool        { return false }

// Commit uploads any missing assets and creates a deployment from the full manifest
// (plan.Desired). New and changed files are "missing" hashes and get uploaded; removed files
// are simply absent from the manifest, so the immutable deployment stops serving them.
func (p *Publisher) Commit(ctx context.Context, tree fs.FS, plan *core.Plan) (core.Result, error) {
	res := core.Result{Total: len(plan.Desired)}

	paths := make([]string, 0, len(plan.Desired))
	for name := range plan.Desired {
		paths = append(paths, name)
	}
	sort.Strings(paths) // deterministic upload order

	manifest := make(map[string]string, len(plan.Desired))
	pathFor := make(map[string]string, len(plan.Desired))
	var hashes []string
	for _, name := range paths {
		h := plan.Desired[name]
		manifest["/"+name] = h
		if _, seen := pathFor[h]; !seen {
			pathFor[h] = name
			hashes = append(hashes, h)
		}
	}

	jwt, err := p.api.uploadToken(ctx, p.accountID, p.project)
	if err != nil {
		return res, err
	}
	missing, err := p.api.checkMissing(ctx, jwt, hashes)
	if err != nil {
		return res, err
	}
	p.log.Detail("PUBLISH", p.id, "branch", p.branch, "missing", len(missing), "of", len(hashes))

	batch := make([]uploadItem, 0, len(missing))
	var batchBytes int64
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := p.api.upload(ctx, jwt, batch); err != nil {
			return err
		}
		batch = batch[:0]
		batchBytes = 0
		return nil
	}
	for _, h := range missing {
		rel := pathFor[h]
		b, err := fs.ReadFile(tree, rel)
		if err != nil {
			return res, err
		}
		if len(batch) >= maxBatchFiles || batchBytes+int64(len(b)) > maxBatchBytes {
			if err := flush(); err != nil {
				return res, err
			}
		}
		batch = append(batch, uploadItem{
			Key:      h,
			Value:    base64.StdEncoding.EncodeToString(b),
			Base64:   true,
			Metadata: uploadMetadata{ContentType: contentType(rel)},
		})
		batchBytes += int64(len(b))
		res.Uploaded++
		res.Bytes += int64(len(b))
	}
	if err := flush(); err != nil {
		return res, err
	}

	if err := p.api.upsertHashes(ctx, jwt, hashes); err != nil {
		return res, err
	}

	dep, err := p.api.createDeployment(ctx, p.accountID, p.project, p.branch, manifest)
	if err != nil {
		return res, err
	}
	p.log.Detail("PUBLISH", p.id, "deployment", dep.ID, "branch", p.branch)
	res.URL = dep.URL
	return res, nil
}

// Invalidate is a no-op: each Pages deployment is immutable and served fresh.
func (p *Publisher) Invalidate(ctx context.Context, paths []string) error { return nil }

// Provision creates the Pages project (Direct Upload) if it does not exist yet.
func (p *Publisher) Provision(ctx context.Context) (bool, error) {
	exists, err := p.api.projectExists(ctx, p.accountID, p.project)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if err := p.api.createProject(ctx, p.accountID, p.project, p.branch); err != nil {
		return false, err
	}
	return true, nil
}

// Prune deletes old deployments for this publisher's branch per its prune spec. It is
// branch-scoped, so pruning one environment never affects another, and it never
// deletes the most recent deployment (which holds the branch alias).
func (p *Publisher) Prune(ctx context.Context) (int, error) {
	if p.prune.mode == pruneNever {
		return 0, nil
	}
	all, err := p.api.listDeployments(ctx, p.accountID, p.project)
	if err != nil {
		return 0, err
	}

	var mine []deploymentInfo
	for _, d := range all {
		if d.Branch() == p.branch {
			mine = append(mine, d)
		}
	}
	sort.Slice(mine, func(i, j int) bool { return mine[i].CreatedOn.After(mine[j].CreatedOn) })

	removed := 0
	for _, d := range p.prune.toDelete(mine, time.Now()) {
		if err := p.api.deleteDeployment(ctx, p.accountID, p.project, d.ID); err != nil {
			continue // best-effort: skip any the API refuses, keep going
		}
		removed++
	}
	return removed, nil
}

// CanonicalURL returns the project's stable public URL for this publisher's branch — a
// custom domain or the pages.dev alias (subdomain for the production branch, else the
// branch alias), never the per-deployment URL.
func (p *Publisher) CanonicalURL(ctx context.Context) (string, error) {
	proj, err := p.api.getProject(ctx, p.accountID, p.project)
	if err != nil {
		return "", err
	}
	isProd := proj.ProductionBranch == "" || p.branch == proj.ProductionBranch
	if isProd && len(proj.Domains) > 0 {
		return "https://" + proj.Domains[0], nil
	}
	if proj.SubDomain == "" {
		return "", nil
	}
	if isProd {
		return "https://" + proj.SubDomain, nil
	}
	return "https://" + sanitizeBranch(p.branch) + "." + proj.SubDomain, nil
}

// sanitizeBranch approximates Cloudflare's branch-alias slug: lower-case, non-alphanumeric
// runs to hyphens.
func sanitizeBranch(b string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(b) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('-')
		}
	}
	return strings.Trim(sb.String(), "-")
}

// hashAsset implements wrangler's Pages hashing: blake3 over the base64 content plus
// the file extension (no dot), hex, first 32 chars.
func hashAsset(p string, content []byte) string {
	b64 := base64.StdEncoding.EncodeToString(content)
	ext := path.Ext(p)
	if ext != "" {
		ext = ext[1:]
	}
	sum := blake3.Sum256([]byte(b64 + ext))
	return hex.EncodeToString(sum[:])[:32]
}

func contentType(p string) string {
	if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
