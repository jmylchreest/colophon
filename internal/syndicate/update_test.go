package syndicate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestFingerprintDetectsChange(t *testing.T) {
	base := Post{Title: "Hello", Summary: "a summary", URL: "https://x/p/", Tags: []string{"go"}}
	same := base
	if Fingerprint(base) != Fingerprint(same) {
		t.Error("identical posts must fingerprint the same")
	}
	for _, mut := range []func(*Post){
		func(p *Post) { p.Title = "Goodbye" },
		func(p *Post) { p.Summary = "different" },
		func(p *Post) { p.Text = "custom" },
		func(p *Post) { p.URL = "https://x/q/" },
		func(p *Post) { p.Tags = []string{"rust"} },
	} {
		p := base
		mut(&p)
		if Fingerprint(p) == Fingerprint(base) {
			t.Errorf("a content change must change the fingerprint: %+v", p)
		}
	}
}

func TestUpdaterCapability(t *testing.T) {
	mk := func(driver string, settings map[string]any) Syndicator {
		s, err := Open(core.SyndicatorConf{ID: driver, Driver: driver, Settings: settings})
		if err != nil {
			t.Fatalf("open %s: %v", driver, err)
		}
		return s
	}
	if _, ok := mk("mastodon", map[string]any{"instance": "https://m.example", "token": "t"}).(Updater); !ok {
		t.Error("mastodon should support Update")
	}
	if _, ok := mk("bluesky", map[string]any{"handle": "me", "app_password": "p"}).(Updater); !ok {
		t.Error("bluesky should support Update")
	}
	if _, ok := mk("bridgy", map[string]any{"network": "mastodon"}).(Updater); ok {
		t.Error("bridgy must NOT support Update (one-shot publish)")
	}
}

func TestMastodonUpdate(t *testing.T) {
	var gotMethod, gotPath, gotStatus string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		gotMethod, gotPath, gotStatus = r.Method, r.URL.Path, body["status"].(string)
		_, _ = w.Write([]byte(`{"url":"https://m.example/@me/123"}`))
	}))
	defer srv.Close()

	s, _ := Open(core.SyndicatorConf{ID: "m", Driver: "mastodon", Settings: map[string]any{"instance": srv.URL, "token": "tok"}})
	up := s.(Updater)
	url, err := up.Update(context.Background(), Post{Title: "Edited title", URL: "https://b.example/p/"},
		Record{URL: "https://m.example/@me/123"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("edit must use PUT, got %s", gotMethod)
	}
	if gotPath != "/api/v1/statuses/123" {
		t.Errorf("edit path = %q, want the status id from the recorded URL", gotPath)
	}
	if !strings.Contains(gotStatus, "Edited title") {
		t.Errorf("edit status = %q, want the new content", gotStatus)
	}
	if url != "https://m.example/@me/123" {
		t.Errorf("returned url = %q", url)
	}
}

func TestBlueskyUpdate(t *testing.T) {
	var put map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "createSession"):
			_, _ = w.Write([]byte(`{"accessJwt":"jwt","did":"did:plc:abc"}`))
		case strings.HasSuffix(r.URL.Path, "putRecord"):
			_ = json.Unmarshal(b, &put)
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:abc/app.bsky.feed.post/3kxyz","cid":"c"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	s, _ := Open(core.SyndicatorConf{ID: "b", Driver: "bluesky", Settings: map[string]any{
		"handle": "me.bsky.social", "app_password": "pw", "service": srv.URL}})
	up := s.(Updater)
	prior := Record{URL: "https://bsky.app/profile/me.bsky.social/post/3kxyz"}
	url, err := up.Update(context.Background(), Post{Title: "Edited", URL: "https://b.example/p/"}, prior)
	if err != nil {
		t.Fatal(err)
	}
	if put["rkey"] != "3kxyz" {
		t.Errorf("putRecord rkey = %v, want the rkey from the recorded URL", put["rkey"])
	}
	if put["repo"] != "did:plc:abc" || put["collection"] != "app.bsky.feed.post" {
		t.Errorf("putRecord repo/collection = %v / %v", put["repo"], put["collection"])
	}
	if rec, _ := put["record"].(map[string]any); rec["text"] != "Edited" {
		t.Errorf("putRecord record text = %v", put["record"])
	}
	if url != prior.URL {
		t.Errorf("edited permalink should be unchanged, got %q", url)
	}
}
