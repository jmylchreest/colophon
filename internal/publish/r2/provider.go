package r2

import (
	"context"
	"net/url"
	"path"
)

// r2HostGlob matches a Cloudflare R2 endpoint host — the standard host and the jurisdiction
// variants (<acct>.eu… / <acct>.fedramp…), since the * spans the extra label.
const r2HostGlob = "*.r2.cloudflarestorage.com"

// isR2Endpoint reports whether endpoint is a Cloudflare R2 host. The driver speaks plain S3 to
// every backend; only a real R2 endpoint additionally gets R2's control-plane features (public-
// URL discovery, r2.dev enablement), so a generic S3/MinIO endpoint skips those Cloudflare calls.
// (If a second backend ever needs its own control-plane path, reintroduce a host→provider table.)
func isR2Endpoint(endpoint string) bool {
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Hostname()
	}
	ok, _ := path.Match(r2HostGlob, host)
	return ok
}

// resolvePublicURL returns the configured public_url, else asks R2 to discover one (a custom or
// managed domain) when the endpoint is R2, else "" — in which case routing stays inert and assets
// remain co-located.
func (p *Publisher) resolvePublicURL(ctx context.Context) (string, error) {
	if p.publicURL != "" {
		return p.publicURL, nil
	}
	if isR2Endpoint(p.s3.Endpoint) {
		return r2PublicURL(ctx, p)
	}
	return "", nil
}
