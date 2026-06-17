// Package telemetry sends colophon's own build/publish events to a statsfactory instance,
// separate from the reader-facing web beacon. It mirrors the pattern tinct uses — a hashed,
// anonymous install id, fire-and-forget delivery that never blocks or fails a command — but
// is config-gated rather than opt-out: it stays silent unless the site's analytics block is
// keyed (and COLOPHON_TELEMETRY=off force-disables it even then).
package telemetry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	statsfactory "github.com/jmylchreest/statsfactory/packages/sdk-go"

	"github.com/jmylchreest/colophon/internal/core"
)

const (
	envOptOut    = "COLOPHON_TELEMETRY" // off|false|0|no force-disables, even when keyed
	flushTimeout = 3 * time.Second
	httpTimeout  = 5 * time.Second
)

// DefaultServerURL and DefaultAppKey are the tool-telemetry credentials baked into the binary
// at release via -ldflags "-X .../internal/telemetry.DefaultServerURL=…". They are empty in
// source and dev builds, so an un-baked, unconfigured colophon reports nothing.
var (
	DefaultServerURL string
	DefaultAppKey    string
)

// Client sends server-side events. A nil or disabled Client is a safe no-op, so call sites
// need no guards.
type Client struct {
	sf         *statsfactory.Client
	distinctID string
	env        string
}

// New builds the tool-telemetry client from colophon's Telemetry config. It returns a disabled
// no-op when the master switch is off, COLOPHON_TELEMETRY opts out, or no credentials resolve
// (config values fall back to the release-baked defaults). env is the environment-name label
// (may be ""); version and root identify the build and locate the anonymous install id.
func New(t core.Telemetry, env, version, root string) *Client {
	c := &Client{env: env}
	if optedOut() || !t.On() {
		return c
	}
	url, key := t.Statsfactory.Resolve(DefaultServerURL, DefaultAppKey)
	if url == "" || key == "" {
		return c
	}
	c.sf = statsfactory.New(statsfactory.Config{
		ServerURL:     url,
		AppKey:        key,
		ClientName:    "colophon",
		ClientVersion: version,
		FlushInterval: 30 * time.Second,
		HTTPClient:    &http.Client{Timeout: httpTimeout},
	})
	c.distinctID = installID(root)
	return c
}

func (c *Client) enabled() bool { return c != nil && c.sf != nil }

// Build records one build: the theme and the total page count (as the metric value).
func (c *Client) Build(theme string, pages int) {
	c.track("build", statsfactory.Dims{"theme": theme}, pages)
}

// Source records the document count contributed by one source, keyed by its driver type —
// this drives the "document count × source type" breakdown.
func (c *Client) Source(driver, id string, docs int) {
	c.track("source_indexed", statsfactory.Dims{"source.type": driver, "source.id": id}, docs)
}

// Publish records one publisher's deploy — its driver type, id and outcome, with the uploaded
// count as the metric value and the deployed total as a dimension. This drives the "published
// docs/executions × publisher type" breakdown.
func (c *Client) Publish(driver, id, status string, uploaded, total int) {
	c.track("publish", statsfactory.Dims{
		"publisher.type": driver,
		"publisher.id":   id,
		"status":         status,
		"docs.total":     total,
	}, uploaded)
}

// Flush sends queued events best-effort within a short timeout, then closes the client. It is
// safe on a disabled client and never returns an error — telemetry must not affect a command.
func (c *Client) Flush() {
	if !c.enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	_ = c.sf.Flush(ctx)
	_ = c.sf.Close()
}

func (c *Client) track(event string, dims statsfactory.Dims, value int) {
	if !c.enabled() {
		return
	}
	if c.env != "" {
		dims["env"] = c.env
	}
	v := float64(value)
	c.sf.TrackWithOptions(event, dims, statsfactory.TrackOptions{DistinctID: c.distinctID, Value: &v})
}

func optedOut() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envOptOut))) {
	case "off", "false", "0", "no":
		return true
	}
	return false
}

// installID returns a stable, anonymous per-project identifier for distinct_id: a SHA-256
// hash of random bytes, generated once and cached at .colophon/telemetry.id (gitignored). The
// raw random value is never stored or sent — only its hash. A failure yields "" (no id sent).
func installID(root string) string {
	path := filepath.Join(root, ".colophon", "telemetry.id")
	if b, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	id := hex.EncodeToString(sum[:])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = os.WriteFile(path, []byte(id), 0o644)
	}
	return id
}
