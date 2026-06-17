package r2

import (
	"context"
	"net/url"
	"path"
)

// providerOps is a recognised backend's control-plane behaviour. A nil hook means the step
// is unsupported, so the driver falls back to explicit config and plain S3.
type providerOps struct {
	// publicURL discovers the bucket's browser-facing base URL (absolute, no trailing slash).
	publicURL func(ctx context.Context, p *Publisher) (string, error)
	// provision runs after the bucket is created, to expose it (e.g. enable the r2.dev URL).
	provision func(ctx context.Context, p *Publisher) error
}

var providerBehaviour = map[providerName]providerOps{
	providerR2: {publicURL: r2PublicURL, provision: r2EnablePublicAccess},
}

// detectProvider resolves the provider for an endpoint via the host glob table.
func detectProvider(endpoint string) providerName {
	host := ""
	if u, err := url.Parse(endpoint); err == nil {
		host = u.Hostname()
	}
	for _, e := range endpointProviders {
		if ok, _ := path.Match(e.hostGlob, host); ok {
			return e.provider
		}
	}
	return providerUnknown
}

func (p *Publisher) ops() providerOps { return providerBehaviour[detectProvider(p.s3.Endpoint)] }

// resolvePublicURL returns the configured public_url, else asks the matching provider to
// discover one, else "" — in which case routing stays inert and assets remain co-located.
func (p *Publisher) resolvePublicURL(ctx context.Context) (string, error) {
	if p.publicURL != "" {
		return p.publicURL, nil
	}
	if f := p.ops().publicURL; f != nil {
		return f(ctx, p)
	}
	return "", nil
}
