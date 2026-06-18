package build

import (
	"os"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestSearchManifestName(t *testing.T) {
	if got := searchManifestName(""); got != "manifest.json" {
		t.Errorf("empty env = %q, want manifest.json", got)
	}
	// Deterministic, distinct per env, hashed (no env name leaked).
	prod := searchManifestName("production")
	if prod != searchManifestName("production") {
		t.Error("not deterministic")
	}
	if prod == searchManifestName("preview") {
		t.Error("different envs must differ")
	}
	if !strings.HasPrefix(prod, "manifest-") || !strings.HasSuffix(prod, ".json") || strings.Contains(prod, "production") {
		t.Errorf("name = %q, want manifest-<hash>.json with no env name", prod)
	}
}

func TestSearchBaseURL(t *testing.T) {
	// Unrouted: the reader loads the index from the local base_path.
	if got := searchBaseURL(core.NewRouter(nil, nil), "/"); got != "/_search/" {
		t.Errorf("unrouted = %q, want /_search/", got)
	}
	// Routed to an object store: the reader loads it cross-origin from the store's URL.
	r := core.NewRouter(
		[]core.RouteRule{{Match: "_search/**", Publisher: "r2", BaseURL: "https://cdn.example.com"}},
		[]string{"r2"},
	)
	if got := searchBaseURL(r, "/"); got != "https://cdn.example.com/_search/" {
		t.Errorf("routed = %q, want https://cdn.example.com/_search/", got)
	}
}

// TestEmbeddedReaderMatchesSource guards against drift: the reader colophon emits
// (assets/search.js) must stay byte-identical to the engine's canonical source so the build
// ships exactly the reader the parity tests cover. Copy ../../search/search.js on any change.
func TestEmbeddedReaderMatchesSource(t *testing.T) {
	canonical, err := os.ReadFile("../../search/search.js")
	if err != nil {
		t.Fatal(err)
	}
	if string(readerJS) != string(canonical) {
		t.Error("internal/build/assets/search.js differs from search/search.js — re-copy the canonical reader")
	}
}

func TestHTMLToText(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<p>Hello <b>world</b></p>", "Hello world"},
		{"<h1>Title</h1>\n<p>Body text.</p>", "Title Body text."},
		{`<script>var x=1;</script><p>visible</p>`, "visible"},
		{"a &amp; b &lt;tag&gt;", "a & b <tag>"},
		{"  spaced   out  ", "spaced out"},
	}
	for _, c := range cases {
		if got := htmlToText(c.in); got != c.want {
			t.Errorf("htmlToText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPagesToSearchDocs(t *testing.T) {
	pages := []page{
		{Title: "Hello", URL: "posts/hello/", HTML: "<p>first post body</p>", Type: "post"},
		{Title: "About", URL: "about/", HTML: "<p>about page</p>", Type: "page"},
	}
	docs := pagesToSearchDocs(pages, "/repo/")
	if len(docs) != 2 {
		t.Fatalf("got %d docs", len(docs))
	}
	if docs[0].ID != "posts/hello/" || docs[0].URL != "/repo/posts/hello/" {
		t.Errorf("doc0 id/url = %q/%q", docs[0].ID, docs[0].URL)
	}
	if docs[0].Body != "first post body" || docs[0].Meta["type"] != "post" {
		t.Errorf("doc0 body/meta = %q/%v", docs[0].Body, docs[0].Meta)
	}

	// Empty base path → absolute root-relative link.
	docs = pagesToSearchDocs(pages, "")
	if docs[1].URL != "/about/" {
		t.Errorf("empty base_path link = %q, want /about/", docs[1].URL)
	}
}
