package r2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cloudflareAPIBase = "https://api.cloudflare.com/client/v4"

// cfAPI is a minimal Cloudflare control-plane client (bearer token + standard envelope),
// used for R2 public-URL discovery and provisioning. The S3 data-plane is separate.
type cfAPI struct {
	token string
	base  string
	hc    *http.Client
}

func newCFAPI(token, base string) *cfAPI {
	return &cfAPI{token: token, base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: time.Minute}}
}

type cfEnvelope struct {
	Success bool            `json:"success"`
	Errors  []cfError       `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *cfAPI) call(ctx context.Context, method, apiPath string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+apiPath, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	var env cfEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	if !env.Success {
		if len(env.Errors) > 0 {
			return fmt.Errorf("cloudflare api %s: %s", apiPath, env.Errors[0].Message)
		}
		return fmt.Errorf("cloudflare api %s: %s", apiPath, resp.Status)
	}
	if out != nil && len(env.Result) > 0 {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

// --- Cloudflare R2 domain behaviour ---

// r2Account recovers the account id from an R2 endpoint host: the first DNS label. This
// holds for the standard host (<account>.r2.cloudflarestorage.com) and the jurisdiction
// variants (<account>.eu… / <account>.fedramp…).
func r2Account(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if i := strings.IndexByte(host, '.'); i >= 0 {
		return host[:i]
	}
	return host
}

type r2Domain struct {
	Domain  string `json:"domain"`
	Enabled bool   `json:"enabled"`
}

func (c *cfAPI) r2ManagedDomain(ctx context.Context, account, bucket string) (r2Domain, error) {
	var d r2Domain
	err := c.call(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/r2/buckets/%s/domains/managed", account, bucket), nil, &d)
	return d, err
}

func (c *cfAPI) r2EnableManagedDomain(ctx context.Context, account, bucket string) error {
	return c.call(ctx, http.MethodPut, fmt.Sprintf("/accounts/%s/r2/buckets/%s/domains/managed", account, bucket), map[string]bool{"enabled": true}, nil)
}

func (c *cfAPI) r2CustomDomains(ctx context.Context, account, bucket string) ([]r2Domain, error) {
	var out struct {
		Domains []r2Domain `json:"domains"`
	}
	err := c.call(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/r2/buckets/%s/domains/custom", account, bucket), nil, &out)
	return out.Domains, err
}

// r2PublicURL prefers a connected custom domain (the shortest, when several are enabled),
// else the managed r2.dev URL when enabled, else "". API errors (e.g. a token lacking R2
// permission) are returned, not swallowed, so the caller can explain why routing stayed off.
func r2PublicURL(ctx context.Context, p *Publisher) (string, error) {
	if p.cf == nil {
		return "", nil // no control-plane token: rely on an explicit public_url
	}
	account := r2Account(p.endpoint)
	customs, err := p.cf.r2CustomDomains(ctx, account, p.bucket)
	if err != nil {
		return "", err
	}
	best := ""
	for _, d := range customs {
		if d.Enabled && (best == "" || len(d.Domain) < len(best)) {
			best = d.Domain
		}
	}
	if best != "" {
		return "https://" + best, nil
	}
	m, err := p.cf.r2ManagedDomain(ctx, account, p.bucket)
	if err != nil {
		return "", err
	}
	if m.Enabled && m.Domain != "" {
		return "https://" + m.Domain, nil
	}
	return "", nil // bucket exists but no public domain is enabled
}

// r2EnablePublicAccess makes the bucket reachable with the least exposure: if it is already
// public (a configured public_url, a connected custom domain, or r2.dev already on) it does
// nothing; only when nothing exposes it does it enable the managed r2.dev domain. Without a
// control-plane token there is nothing to do.
func r2EnablePublicAccess(ctx context.Context, p *Publisher) error {
	if p.cf == nil {
		return nil
	}
	// Already reachable? (Checked directly, not via resolvePublicURL, to avoid an init cycle
	// through the provider table that holds this very function.)
	if p.publicURL != "" {
		return nil
	}
	if u, _ := r2PublicURL(ctx, p); u != "" {
		return nil // a domain already exposes it — keep the public surface minimal
	}
	return p.cf.r2EnableManagedDomain(ctx, r2Account(p.endpoint), p.bucket)
}
