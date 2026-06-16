package r2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

func TestWriteManifest(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/.well-known/colophon.json") {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	p := &Publisher{
		id: "r2", bucket: "b", endpoint: srv.URL, region: "auto",
		description: "blog.i0.pm assets", publicURL: "https://assets.blog.i0.pm",
		accessKey: "AK", secretKey: "SK", client: srv.Client(),
	}
	err := p.WriteManifest(context.Background(), core.SiteManifest{
		Generator: "colophon", Site: "https://blog.i0.pm", Sitemap: "https://blog.i0.pm/sitemap.xml",
		Feeds: map[string]string{"rss": "https://blog.i0.pm/rss.xml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"generator": "colophon"`,
		`"site": "https://blog.i0.pm"`,
		`"sitemap": "https://blog.i0.pm/sitemap.xml"`,
		`"rss": "https://blog.i0.pm/rss.xml"`,
		`"description": "blog.i0.pm assets"`,
		`"bucket": "b"`,
		`"public_url": "https://assets.blog.i0.pm"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("manifest missing %s\n%s", want, body)
		}
	}
}

// fakeStore is an in-memory S3-ish server: PUT stores the object's MD5, HEAD returns it as
// the ETag (so the publisher's skip-unchanged check works), and "/<bucket>" is the bucket.
type fakeStore struct {
	objects       map[string]string // path -> md5 hex
	bucketCreated bool
	auths         []string
}

func (f *fakeStore) handler(bucket string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.auths = append(f.auths, r.Header.Get("Authorization"))
		isBucket := r.URL.Path == "/"+bucket
		switch r.Method {
		case http.MethodHead:
			if isBucket {
				if f.bucketCreated {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
				return
			}
			if etag, ok := f.objects[r.URL.Path]; ok {
				w.Header().Set("ETag", `"`+etag+`"`)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case http.MethodGet: // ListObjectsV2
			if !isBucket {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			prefix := "/" + bucket + "/"
			var sb strings.Builder
			sb.WriteString(`<?xml version="1.0"?><ListBucketResult>`)
			for path, etag := range f.objects {
				fmt.Fprintf(&sb, `<Contents><Key>%s</Key><ETag>"%s"</ETag></Contents>`,
					strings.TrimPrefix(path, prefix), etag)
			}
			sb.WriteString(`<IsTruncated>false</IsTruncated></ListBucketResult>`)
			_, _ = w.Write([]byte(sb.String()))
		case http.MethodPut:
			if isBucket {
				f.bucketCreated = true
			} else {
				body, _ := io.ReadAll(r.Body)
				f.objects[r.URL.Path] = md5hex(body)
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			delete(f.objects, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func newTestPublisher(t *testing.T) (*Publisher, *fakeStore) {
	t.Helper()
	store := &fakeStore{objects: map[string]string{}}
	srv := httptest.NewServer(store.handler("b"))
	t.Cleanup(srv.Close)
	p := &Publisher{
		id: "r2", bucket: "b", endpoint: srv.URL, region: "auto",
		deleteOrphaned: true, accessKey: "AKID", secretKey: "secret", client: srv.Client(),
	}
	return p, store
}

func TestProvisionCreatesMissingBucket(t *testing.T) {
	p, _ := newTestPublisher(t)
	created, err := p.Provision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Error("expected the missing bucket to be created")
	}
}

func TestProvisionSendsLocationHint(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound) // bucket missing
			return
		}
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	p := &Publisher{id: "r2", bucket: "b", endpoint: srv.URL, region: "auto", location: "weur",
		accessKey: "AKID", secretKey: "secret", client: srv.Client()}
	if _, err := p.Provision(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "<LocationConstraint>weur</LocationConstraint>") {
		t.Errorf("CreateBucket body missing location hint: %q", body)
	}
}

func TestRunUploadsSkipsAndDeletes(t *testing.T) {
	p, store := newTestPublisher(t)
	tree := fstest.MapFS{
		"index.html":             {Data: []byte("<h1>hi</h1>")},
		"posts/p/assets/cat.png": {Data: []byte("img-bytes")},
	}

	res, err := publish.Run(context.Background(), tree, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 2 || res.Total != 2 {
		t.Errorf("first run: uploaded=%d total=%d, want 2/2", res.Uploaded, res.Total)
	}
	if _, ok := store.objects["/b/posts/p/assets/cat.png"]; !ok {
		t.Error("asset was not stored at its keyed path")
	}
	// Every request must carry a SigV4 Authorization header.
	for _, a := range store.auths {
		if !strings.HasPrefix(a, "AWS4-HMAC-SHA256 Credential=AKID/") {
			t.Errorf("request not signed: %q", a)
		}
	}

	// Re-run unchanged: the listing reports the ETags, so nothing transfers or deletes.
	res, err = publish.Run(context.Background(), tree, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 0 || res.Deleted != 0 || res.Total != 2 {
		t.Errorf("re-run: uploaded=%d deleted=%d total=%d, want 0/0/2", res.Uploaded, res.Deleted, res.Total)
	}

	// Drop a file from the tree: the orphaned object is deleted.
	smaller := fstest.MapFS{"index.html": {Data: []byte("<h1>hi</h1>")}}
	res, err = publish.Run(context.Background(), smaller, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 || res.Total != 1 {
		t.Errorf("after removal: deleted=%d total=%d, want 1/1", res.Deleted, res.Total)
	}
	if _, ok := store.objects["/b/posts/p/assets/cat.png"]; ok {
		t.Error("orphaned object was not deleted")
	}
}

func TestValidateBucketName(t *testing.T) {
	valid := []string{"colophon-assets", "abc", "a1-b2-c3", strings.Repeat("a", 63)}
	for _, n := range valid {
		if err := validateBucketName(n); err != nil {
			t.Errorf("validateBucketName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{
		"jzZ0GjHXh0b18s6Ooi6dQ", // uppercase (the reported failure)
		"ab",                    // too short
		strings.Repeat("a", 64), // too long
		"assets.blog.i0.pm",     // dots not allowed
		"-leading",              // must start alphanumeric
		"trailing-",             // must end alphanumeric
		"under_score",           // underscore not allowed
	}
	for _, n := range invalid {
		if err := validateBucketName(n); err == nil {
			t.Errorf("validateBucketName(%q) = nil, want error", n)
		}
	}
}

func TestNewRejectsBadBucket(t *testing.T) {
	_, err := New("", config.PublisherConfig{ID: "r2", Driver: "cloudflare-r2",
		Settings: map[string]any{"bucket": "Uppercase", "account_id": "acct"}})
	if err == nil {
		t.Error("New should reject an invalid bucket name")
	}
}

func TestDeployedRequiresCreds(t *testing.T) {
	p := &Publisher{id: "r2", bucket: "b", endpoint: "https://x", region: "auto"}
	if _, _, err := p.Deployed(context.Background()); err == nil {
		t.Error("Deployed without credentials should error")
	}
}
