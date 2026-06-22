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

func TestBlueskySyndicator(t *testing.T) {
	var sessionBody, recordBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "createSession"):
			json.Unmarshal(b, &sessionBody)
			w.Write([]byte(`{"accessJwt":"jwt123","did":"did:plc:abc"}`))
		case strings.HasSuffix(r.URL.Path, "createRecord"):
			if r.Header.Get("Authorization") != "Bearer jwt123" {
				t.Errorf("createRecord auth = %q", r.Header.Get("Authorization"))
			}
			json.Unmarshal(b, &recordBody)
			w.Write([]byte(`{"uri":"at://did:plc:abc/app.bsky.feed.post/3kxyz","cid":"c"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	s, err := Open(core.SyndicatorConf{ID: "bsky", Driver: "bluesky", Settings: map[string]any{
		"handle": "me.bsky.social", "app_password": "app-pw", "service": srv.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	url, err := s.Syndicate(context.Background(), Post{Title: "Hello world", URL: "https://b.example/posts/x/", Summary: "a summary"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://bsky.app/profile/me.bsky.social/post/3kxyz" {
		t.Errorf("bluesky url = %q", url)
	}
	if sessionBody["identifier"] != "me.bsky.social" || sessionBody["password"] != "app-pw" {
		t.Errorf("session body = %v", sessionBody)
	}
	rec, _ := recordBody["record"].(map[string]any)
	if rec["text"] != "Hello world" {
		t.Errorf("record text = %v", rec["text"])
	}
	if embed, _ := rec["embed"].(map[string]any); embed["$type"] != "app.bsky.embed.external" {
		t.Errorf("embed = %v", rec["embed"])
	}
}

func TestMastodonSyndicator(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &body)
		w.Write([]byte(`{"url":"https://hachyderm.io/@me/123"}`))
	}))
	defer srv.Close()

	s, err := Open(core.SyndicatorConf{ID: "m", Driver: "mastodon", Settings: map[string]any{
		"instance": srv.URL, "token": "tok",
	}})
	if err != nil {
		t.Fatal(err)
	}
	url, err := s.Syndicate(context.Background(), Post{Title: "Hi", URL: "https://b.example/posts/x/"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://hachyderm.io/@me/123" {
		t.Errorf("mastodon url = %q", url)
	}
	status, _ := body["status"].(string)
	if !strings.Contains(status, "Hi") || !strings.Contains(status, "https://b.example/posts/x/") {
		t.Errorf("status text = %q", status)
	}
}

func TestMastodonTextKeepsLink(t *testing.T) {
	long := strings.Repeat("x", 600)
	got := mastodonText(Post{Text: long, URL: "https://b.example/p/"}, 500)
	if !strings.HasSuffix(got, "https://b.example/p/") {
		t.Error("link must survive truncation")
	}
	if len([]rune(got)) > 500 {
		t.Errorf("over limit: %d runes", len([]rune(got)))
	}
}

func TestBridgySyndicator(t *testing.T) {
	var gotSource, gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotSource, gotTarget = r.PostFormValue("source"), r.PostFormValue("target")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"url":"https://hachyderm.io/@me/999"}`))
	}))
	defer srv.Close()

	s, err := Open(core.SyndicatorConf{ID: "bf", Driver: "bridgy", Settings: map[string]any{
		"network": "mastodon", "endpoint": srv.URL,
	}})
	if err != nil {
		t.Fatal(err)
	}
	url, err := s.Syndicate(context.Background(), Post{URL: "https://b.example/posts/x/", Title: "Hi"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://hachyderm.io/@me/999" {
		t.Errorf("bridgy url = %q", url)
	}
	if gotSource != "https://b.example/posts/x/" || gotTarget != "https://brid.gy/publish/mastodon" {
		t.Errorf("bridgy posted source=%q target=%q", gotSource, gotTarget)
	}
}

func TestBridgyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"no Bridgy account for this domain"}`))
	}))
	defer srv.Close()
	s, _ := Open(core.SyndicatorConf{ID: "bf", Driver: "bridgy", Settings: map[string]any{"network": "bluesky", "endpoint": srv.URL}})
	if _, err := s.Syndicate(context.Background(), Post{URL: "https://b/x/"}); err == nil {
		t.Error("expected error from Bridgy 4xx")
	}
}

func TestDriverConfigErrors(t *testing.T) {
	if _, err := Open(core.SyndicatorConf{ID: "x", Driver: "bluesky", Settings: map[string]any{"handle": "h"}}); err == nil {
		t.Error("bluesky without app_password should error")
	}
	if _, err := Open(core.SyndicatorConf{ID: "x", Driver: "mastodon", Settings: map[string]any{"instance": "https://i"}}); err == nil {
		t.Error("mastodon without token should error")
	}
	if _, err := Open(core.SyndicatorConf{ID: "x", Driver: "bridgy", Settings: map[string]any{}}); err == nil {
		t.Error("bridgy without network should error")
	}
}
