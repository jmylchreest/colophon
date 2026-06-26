package syndicate

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

	"github.com/jmylchreest/colophon/internal/retry"
)

// httpClient is the shared client for the network drivers (Bluesky/Mastodon).
var httpClient = &http.Client{Timeout: 30 * time.Second}

// postJSON POSTs body as JSON to url (optional Bearer token + extra headers) and decodes a 2xx
// JSON response into out (nil to ignore). A non-2xx returns an error with a truncated body for
// diagnosis; transport/5xx/429 failures are classified for the retry layer.
func postJSON(ctx context.Context, url, bearer string, headers map[string]string, body, out any) error {
	return sendJSON(ctx, http.MethodPost, url, bearer, headers, body, out)
}

// putJSON is postJSON with PUT — for edits (Mastodon's PUT /api/v1/statuses/:id).
func putJSON(ctx context.Context, url, bearer string, headers map[string]string, body, out any) error {
	return sendJSON(ctx, http.MethodPut, url, bearer, headers, body, out)
}

// sendJSON sends body as JSON via method to url (optional Bearer + extra headers) and decodes a
// 2xx JSON response into out (nil to ignore). A non-2xx returns an error with a truncated body;
// transport/5xx/429 failures are classified for the retry layer.
func sendJSON(ctx context.Context, method, url, bearer string, headers map[string]string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return retry.FromDoErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return retry.FromStatus(resp.StatusCode, retry.RetryAfter(resp.Header), fmt.Errorf("%s → %s: %s", url, resp.Status, truncate(string(data), 240)))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s: %w", url, err)
		}
	}
	return nil
}

// retryIdempotent retries any transient/rate-limit failure — for safe-to-repeat calls (reads,
// auth, or a create made idempotent by an Idempotency-Key).
func retryIdempotent(ctx context.Context, fn func() error) error {
	return retry.Do(ctx, retry.Default(), fn)
}

// retrySafe retries ONLY when nothing could have been created — a rate limit or a provably-unsent
// request — for non-idempotent create calls (Bluesky createRecord, Bridgy publish), so an
// ambiguous mid-flight failure never risks a duplicate post.
func retrySafe(ctx context.Context, fn func() error) error {
	p := retry.Default()
	p.Retryable = retry.SafeRetryable
	return retry.Do(ctx, p, fn)
}

// postForm POSTs form values (application/x-www-form-urlencoded), requests JSON, and decodes a
// 2xx response into out (nil to ignore) — for APIs like Bridgy's publish endpoint.
func postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return retry.FromDoErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return retry.FromStatus(resp.StatusCode, retry.RetryAfter(resp.Header), fmt.Errorf("%s → %s: %s", endpoint, resp.Status, truncate(string(data), 240)))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s: %w", endpoint, err)
		}
	}
	return nil
}

// confStr reads a string setting (driver Settings come from YAML, post-{env:} interpolation).
func confStr(settings map[string]any, key string) string {
	if v, ok := settings[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// lastPathSegment returns the final path segment of a URL — the Mastodon status id / Bluesky
// rkey embedded in a recorded permalink.
func lastPathSegment(u string) string {
	u = strings.TrimRight(strings.TrimSpace(u), "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		return u[i+1:]
	}
	return u
}

// firstURL returns the first non-empty URL — the edited permalink, falling back to the prior one.
func firstURL(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// limitRunes truncates s to at most n runes, appending an ellipsis when it had to cut.
func limitRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
