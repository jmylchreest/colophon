package cloudflare

import (
	"regexp"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestNewPruneSetting(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct")
	t.Setenv("CLOUDFLARE_API_TOKEN", "tok")

	t.Run("default is count 1", func(t *testing.T) {
		pub, err := New("/", config.PublisherConfig{ID: "cf", Settings: map[string]any{"project": "b"}})
		if err != nil {
			t.Fatal(err)
		}
		if got := pub.(*Publisher).prune; got.mode != pruneCount || got.count != 1 {
			t.Errorf("prune = %+v, want count 1", got)
		}
	})

	t.Run("invalid value errors", func(t *testing.T) {
		_, err := New("/", config.PublisherConfig{ID: "cf", Settings: map[string]any{"project": "b", "prune": -2}})
		if err == nil {
			t.Fatal("expected error for negative count")
		}
	})
}

func TestHashAsset(t *testing.T) {
	h := hashAsset("index.html", []byte("<h1>hi</h1>"))
	if !hex32.MatchString(h) {
		t.Fatalf("hash %q is not 32 hex chars", h)
	}
	if got := hashAsset("index.html", []byte("<h1>hi</h1>")); got != h {
		t.Error("hash is not deterministic")
	}
	// Extension is part of the hash input, so it must affect the result.
	if hashAsset("index.css", []byte("<h1>hi</h1>")) == h {
		t.Error("extension should change the hash")
	}
}

func TestNewValidation(t *testing.T) {
	t.Setenv("CLOUDFLARE_ACCOUNT_ID", "acct123")

	t.Run("missing project", func(t *testing.T) {
		t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
		if _, err := New("/", config.PublisherConfig{ID: "cf", Driver: "cloudflare-pages"}); err == nil {
			t.Fatal("expected error for missing project")
		}
	})

	t.Run("missing token", func(t *testing.T) {
		t.Setenv("CLOUDFLARE_API_TOKEN", "")
		cfg := config.PublisherConfig{ID: "cf", Driver: "cloudflare-pages", Settings: map[string]any{"project": "blog"}}
		if _, err := New("/", cfg); err == nil {
			t.Fatal("expected error for missing token")
		}
	})

	t.Run("valid", func(t *testing.T) {
		t.Setenv("CLOUDFLARE_API_TOKEN", "tok")
		cfg := config.PublisherConfig{ID: "cf", Driver: "cloudflare-pages", Settings: map[string]any{"project": "blog"}}
		pub, err := New("/", cfg)
		if err != nil {
			t.Fatal(err)
		}
		if pub.Driver() != "cloudflare-pages" {
			t.Errorf("driver = %q", pub.Driver())
		}
	})
}
