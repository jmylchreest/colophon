package s3

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
	"github.com/jmylchreest/colophon/internal/publish/s3common"
)

func TestTigrisDefaultsEndpointAndRegion(t *testing.T) {
	t.Setenv("TIGRIS_ACCESS_KEY_ID", "tid_x")
	t.Setenv("TIGRIS_SECRET_ACCESS_KEY", "tsec_x")
	pub, err := New("", config.PublisherConfig{ID: "t", Driver: "tigris",
		Settings: map[string]any{"bucket": "my-site"}})
	if err != nil {
		t.Fatal(err)
	}
	p := pub.(*Publisher)
	if p.s3.Endpoint != tigrisEndpoint {
		t.Errorf("endpoint = %q, want %q", p.s3.Endpoint, tigrisEndpoint)
	}
	if p.s3.Region != "auto" {
		t.Errorf("region = %q, want auto", p.s3.Region)
	}
	if p.s3.AccessKey != "tid_x" || p.s3.SecretKey != "tsec_x" {
		t.Errorf("creds = %q/%q, want tigris keys", p.s3.AccessKey, p.s3.SecretKey)
	}
	if p.Driver() != "tigris" {
		t.Errorf("Driver() = %q, want tigris", p.Driver())
	}
}

func TestTigrisFallsBackToAWSCreds(t *testing.T) {
	t.Setenv("TIGRIS_ACCESS_KEY_ID", "")
	t.Setenv("TIGRIS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "shh")
	pub, err := New("", config.PublisherConfig{ID: "t", Driver: "tigris",
		Settings: map[string]any{"bucket": "my-site"}})
	if err != nil {
		t.Fatal(err)
	}
	p := pub.(*Publisher)
	if p.s3.AccessKey != "AKIA" || p.s3.SecretKey != "shh" {
		t.Errorf("creds = %q/%q, want AWS fallback", p.s3.AccessKey, p.s3.SecretKey)
	}
}

func TestGenericS3RequiresEndpoint(t *testing.T) {
	_, err := New("", config.PublisherConfig{ID: "s", Driver: "s3",
		Settings: map[string]any{"bucket": "bkt"}})
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("s3 without endpoint should error about endpoint, got %v", err)
	}
}

func TestNewRejectsBadBucket(t *testing.T) {
	_, err := New("", config.PublisherConfig{ID: "s", Driver: "tigris",
		Settings: map[string]any{"bucket": "Bad_Name"}})
	if err == nil {
		t.Error("New should reject an invalid bucket name")
	}
}

func TestCanonicalURLIsPublicURL(t *testing.T) {
	pub, err := New("", config.PublisherConfig{ID: "t", Driver: "tigris",
		Settings: map[string]any{"bucket": "bkt", "public_url": "https://cdn.example.com/"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := pub.(core.CanonicalURLer).CanonicalURL(context.Background())
	if got != "https://cdn.example.com" {
		t.Errorf("CanonicalURL = %q, want trimmed public_url", got)
	}
}

func TestDeployedRequiresCreds(t *testing.T) {
	p := &Publisher{id: "t", driver: "tigris",
		s3: &s3common.Client{Bucket: "b", Endpoint: "https://x", Region: "auto"}}
	_, _, err := p.Deployed(context.Background())
	if err == nil || !strings.Contains(err.Error(), "TIGRIS_ACCESS_KEY_ID") {
		t.Errorf("Deployed without creds should name the Tigris env vars, got %v", err)
	}
}

// fakeStore is an in-memory S3-ish server: PUT stores the object's MD5, HEAD returns it as the
// ETag, GET lists, and "/<bucket>" is the bucket itself.
type fakeStore struct {
	objects       map[string]string
	bucketCreated bool
}

func (f *fakeStore) handler(bucket string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		case http.MethodGet:
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
				f.objects[r.URL.Path] = publish.MD5Hex(body)
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
		id: "t", driver: "tigris", deleteOrphaned: true,
		s3: &s3common.Client{Name: "t", Bucket: "b", Endpoint: srv.URL, Region: "auto",
			AccessKey: "AKID", SecretKey: "secret", HTTPClient: srv.Client()},
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
	// Idempotent: a second run finds it and reports no creation.
	p2, store := newTestPublisher(t)
	store.bucketCreated = true
	created, err = p2.Provision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("an existing bucket should not be reported as created")
	}
}

func TestRunUploadsSkipsAndDeletes(t *testing.T) {
	p, store := newTestPublisher(t)
	tree := fstest.MapFS{
		"index.html":         {Data: []byte("<h1>hi</h1>")},
		"assets/cat.png":     {Data: []byte("img-bytes")},
		"posts/a/index.html": {Data: []byte("post a")},
	}
	res, err := publish.Run(context.Background(), tree, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 3 || res.Total != 3 {
		t.Errorf("first run: uploaded=%d total=%d, want 3/3", res.Uploaded, res.Total)
	}
	if _, ok := store.objects["/b/assets/cat.png"]; !ok {
		t.Error("asset was not stored at its keyed path")
	}

	res, err = publish.Run(context.Background(), tree, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 0 || res.Deleted != 0 {
		t.Errorf("re-run: uploaded=%d deleted=%d, want 0/0", res.Uploaded, res.Deleted)
	}

	smaller := fstest.MapFS{"index.html": {Data: []byte("<h1>hi</h1>")}}
	res, err = publish.Run(context.Background(), smaller, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 2 {
		t.Errorf("after removal: deleted=%d, want 2", res.Deleted)
	}
}
