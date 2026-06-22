package webmention

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
)

func TestCanonicalURL(t *testing.T) {
	body := []byte(`<html><head><link rel="canonical" href="https://b.example/posts/x/"></head><body>hi</body></html>`)
	if got := CanonicalURL(body); got != "https://b.example/posts/x/" {
		t.Errorf("CanonicalURL = %q", got)
	}
	if got := CanonicalURL([]byte(`<html><head></head></html>`)); got != "" {
		t.Errorf("CanonicalURL without canonical = %q, want empty", got)
	}
}

func TestOutboundLinks(t *testing.T) {
	src := "https://me.example/posts/hello/"
	body := []byte(`<article>
		<a href="https://other.example/a">x</a>
		<a href="https://other.example/a#frag">dup</a>
		<a href="/posts/two/">internal</a>
		<a href="https://me.example/about/">same-host</a>
		<a href="mailto:x@y.z">mail</a>
		<a href="https://third.example/b">y</a>
	</article>`)
	got, err := OutboundLinks(src, body)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://other.example/a", "https://third.example/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("OutboundLinks = %v, want %v", got, want)
	}
}

func TestEndpointFromHTMLAndHeader(t *testing.T) {
	if ep := endpointFromHTML([]byte(`<link rel="webmention" href="/wm">`)); ep != "/wm" {
		t.Errorf("link rel: %q", ep)
	}
	if ep := endpointFromHTML([]byte(`<a rel="noopener webmention" href="https://wm.example/x">x</a>`)); ep != "https://wm.example/x" {
		t.Errorf("a rel multi-token: %q", ep)
	}
	if ep := endpointFromLinkHeader([]string{`<https://wm.example/h>; rel="webmention"`}); ep != "https://wm.example/h" {
		t.Errorf("link header: %q", ep)
	}
	if ep := endpointFromLinkHeader([]string{`<https://x>; rel="other"`}); ep != "" {
		t.Errorf("non-webmention header should be empty: %q", ep)
	}
}

func TestDiscoverPrefersHeaderThenResolves(t *testing.T) {
	// Header endpoint, relative → resolved against the target URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</webmention>; rel="webmention"`)
		w.Write([]byte(`<link rel="webmention" href="/ignored-html-endpoint">`))
	}))
	defer srv.Close()
	ep, err := Discover(context.Background(), srv.Client(), srv.URL+"/post")
	if err != nil {
		t.Fatal(err)
	}
	if ep != srv.URL+"/webmention" {
		t.Errorf("Discover header endpoint = %q, want %q", ep, srv.URL+"/webmention")
	}
}

func TestDiscoverFallsBackToHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><link rel="webmention" href="https://wm.example/endpoint"></head></html>`))
	}))
	defer srv.Close()
	ep, err := Discover(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if ep != "https://wm.example/endpoint" {
		t.Errorf("Discover html endpoint = %q", ep)
	}
}

func TestSend(t *testing.T) {
	var gotSource, gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		gotSource, gotTarget = r.PostFormValue("source"), r.PostFormValue("target")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	if err := Send(context.Background(), srv.Client(), srv.URL, "https://me/p", "https://you/q"); err != nil {
		t.Fatal(err)
	}
	if gotSource != "https://me/p" || gotTarget != "https://you/q" {
		t.Errorf("Send posted source=%q target=%q", gotSource, gotTarget)
	}
}

func TestDiff(t *testing.T) {
	added, dropped := Diff([]string{"a", "b"}, []string{"b", "c"})
	if !reflect.DeepEqual(added, []string{"c"}) {
		t.Errorf("added = %v, want [c]", added)
	}
	if !reflect.DeepEqual(dropped, []string{"a"}) {
		t.Errorf("dropped = %v, want [a]", dropped)
	}
	a2, d2 := Diff(nil, []string{"x"})
	sort.Strings(a2)
	if !reflect.DeepEqual(a2, []string{"x"}) || len(d2) != 0 {
		t.Errorf("first-run diff added=%v dropped=%v", a2, d2)
	}
}
