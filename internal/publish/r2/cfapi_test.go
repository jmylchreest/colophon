package r2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/publish/s3common"
)

func TestDetectProvider(t *testing.T) {
	cases := map[string]providerName{
		"https://abc123.r2.cloudflarestorage.com":         providerR2,
		"https://abc123.eu.r2.cloudflarestorage.com":      providerR2, // EU jurisdiction
		"https://abc123.fedramp.r2.cloudflarestorage.com": providerR2, // FedRAMP
		"https://s3.us-east-1.amazonaws.com":              providerUnknown,
		"http://localhost:9000":                           providerUnknown,
	}
	for endpoint, want := range cases {
		if got := detectProvider(endpoint); got != want {
			t.Errorf("detectProvider(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

func TestR2Account(t *testing.T) {
	cases := map[string]string{
		"https://acct.r2.cloudflarestorage.com":         "acct",
		"https://acct.eu.r2.cloudflarestorage.com":      "acct",
		"https://acct.fedramp.r2.cloudflarestorage.com": "acct",
	}
	for endpoint, want := range cases {
		if got := r2Account(endpoint); got != want {
			t.Errorf("r2Account(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

// cfMock serves the R2 domain endpoints with canned JSON.
func cfMock(t *testing.T, custom, managed string) *cfAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/domains/custom"):
			_, _ = w.Write([]byte(`{"success":true,"result":` + custom + `}`))
		case strings.HasSuffix(r.URL.Path, "/domains/managed"):
			_, _ = w.Write([]byte(`{"success":true,"result":` + managed + `}`))
		default:
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return newCFAPI("token", srv.URL)
}

func r2Publisher(cf *cfAPI, publicURL string) *Publisher {
	return &Publisher{
		id: "r2", publicURL: publicURL, cf: cf,
		s3: &s3common.Client{Bucket: "b", Endpoint: "https://acct.r2.cloudflarestorage.com"},
	}
}

func TestPublicURLConfigShortCircuits(t *testing.T) {
	// An explicit public_url wins and makes no API call (cf is nil).
	p := r2Publisher(nil, "https://assets.example.com")
	got, err := p.resolvePublicURL(context.Background())
	if err != nil || got != "https://assets.example.com" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestPublicURLPrefersShortestCustomDomain(t *testing.T) {
	cf := cfMock(t,
		`{"domains":[{"domain":"cdn.assets.example.com","enabled":true},{"domain":"assets.example.com","enabled":true},{"domain":"x.blog.example.com","enabled":false}]}`,
		`{"domain":"pub-xyz.r2.dev","enabled":true}`)
	p := r2Publisher(cf, "")
	got, err := p.resolvePublicURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://assets.example.com" {
		t.Errorf("got %q, want the shortest enabled custom domain", got)
	}
}

func TestPublicURLFallsBackToManaged(t *testing.T) {
	cf := cfMock(t, `{"domains":[]}`, `{"domain":"pub-xyz.r2.dev","enabled":true}`)
	got, err := r2Publisher(cf, "").resolvePublicURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://pub-xyz.r2.dev" {
		t.Errorf("got %q, want the managed r2.dev URL", got)
	}
}

func TestPublicURLEmptyWhenNothingEnabled(t *testing.T) {
	cf := cfMock(t, `{"domains":[]}`, `{"domain":"pub-xyz.r2.dev","enabled":false}`)
	got, err := r2Publisher(cf, "").resolvePublicURL(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("got %q, want empty when no domain is enabled", got)
	}
}

// enableTracker serves the R2 domain endpoints and records whether the managed domain was
// turned on (PUT .../domains/managed).
func enableTracker(t *testing.T, custom, managed string, enabled *bool) *cfAPI {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/domains/managed"):
			*enabled = true
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/domains/custom"):
			_, _ = w.Write([]byte(`{"success":true,"result":` + custom + `}`))
		case strings.HasSuffix(r.URL.Path, "/domains/managed"):
			_, _ = w.Write([]byte(`{"success":true,"result":` + managed + `}`))
		}
	}))
	t.Cleanup(srv.Close)
	return newCFAPI("token", srv.URL)
}

func TestEnablePublicAccessOnlyWhenNeeded(t *testing.T) {
	// Nothing public yet → r2.dev gets enabled.
	enabled := false
	p := r2Publisher(enableTracker(t, `{"domains":[]}`, `{"domain":"pub-x.r2.dev","enabled":false}`, &enabled), "")
	if err := r2EnablePublicAccess(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Error("expected r2.dev to be enabled when nothing else exposes the bucket")
	}

	// A connected custom domain already exposes it → r2.dev is NOT enabled (minimal surface).
	enabled = false
	p = r2Publisher(enableTracker(t, `{"domains":[{"domain":"assets.example.com","enabled":true}]}`, `{"enabled":false}`, &enabled), "")
	if err := r2EnablePublicAccess(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("must not enable r2.dev when a custom domain already serves the bucket")
	}

	// An explicit public_url also means we leave it alone.
	enabled = false
	p = r2Publisher(enableTracker(t, `{"domains":[]}`, `{"enabled":false}`, &enabled), "https://cdn.example.com")
	if err := r2EnablePublicAccess(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("must not enable r2.dev when public_url is configured")
	}
}

func TestGenericS3HasNoDiscovery(t *testing.T) {
	// A non-R2 endpoint never discovers; it relies on explicit public_url.
	p := &Publisher{id: "s3", cf: cfMock(t, `{"domains":[]}`, `{}`),
		s3: &s3common.Client{Bucket: "b", Endpoint: "https://s3.amazonaws.com"}}
	got, err := p.resolvePublicURL(context.Background())
	if err != nil || got != "" {
		t.Errorf("generic S3 should not discover: got %q, %v", got, err)
	}
}
