package syndicate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// httpClient is the shared client for the network drivers (Bluesky/Mastodon).
var httpClient = &http.Client{Timeout: 30 * time.Second}

// postJSON POSTs body as JSON to url (optional Bearer token) and decodes a 2xx JSON response
// into out (nil to ignore). A non-2xx returns an error with a truncated body for diagnosis.
func postJSON(ctx context.Context, url, bearer string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s → %s: %s", url, resp.Status, truncate(string(data), 240))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode %s: %w", url, err)
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
