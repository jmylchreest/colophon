package s3common

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateBucketName(t *testing.T) {
	t.Parallel()
	valid := []string{"colophon-assets", "abc", "a1-b2-c3", strings.Repeat("a", 63)}
	for _, n := range valid {
		if err := ValidateBucketName(n); err != nil {
			t.Errorf("ValidateBucketName(%q) = %v, want nil", n, err)
		}
	}
	invalid := []string{
		"jzZ0GjHXh0b18s6Ooi6dQ", // uppercase
		"ab",                    // too short
		strings.Repeat("a", 64), // too long
		"assets.example.com",    // dots not allowed
		"-leading",              // must start alphanumeric
		"trailing-",             // must end alphanumeric
		"under_score",           // underscore not allowed
	}
	for _, n := range invalid {
		if err := ValidateBucketName(n); err == nil {
			t.Errorf("ValidateBucketName(%q) = nil, want error", n)
		}
	}
}

// newClient wires a Client to an httptest server.
func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "b", "auto", "AKID", "secret")
	c.HTTPClient = srv.Client()
	return c
}

func TestHead(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		want   bool
		err    bool
	}{
		{"present", http.StatusOK, true, false},
		{"forbidden-counts-as-present", http.StatusForbidden, true, false},
		{"missing", http.StatusNotFound, false, false},
		{"server-error", http.StatusInternalServerError, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) })
			got, err := c.Head(context.Background())
			if (err != nil) != tc.err || got != tc.want {
				t.Errorf("Head() = (%v, %v), want (%v, err=%v)", got, err, tc.want, tc.err)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	t.Parallel()
	t.Run("happy + location hint", func(t *testing.T) {
		t.Parallel()
		var body string
		c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			w.WriteHeader(http.StatusOK)
		})
		if err := c.Create(context.Background(), "weur"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body, "<LocationConstraint>weur</LocationConstraint>") {
			t.Errorf("create body missing location hint: %q", body)
		}
	})
	t.Run("409 already owned is not an error", func(t *testing.T) {
		t.Parallel()
		c := newClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusConflict) })
		if err := c.Create(context.Background(), ""); err != nil {
			t.Errorf("Create on 409 = %v, want nil", err)
		}
	})
}

func TestListPagination(t *testing.T) {
	t.Parallel()
	page := 0
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		var sb strings.Builder
		sb.WriteString(`<?xml version="1.0"?><ListBucketResult>`)
		if r.URL.Query().Get("continuation-token") == "" {
			page = 1
			sb.WriteString(`<Contents><Key>a.html</Key><ETag>"aaa"</ETag></Contents>`)
			sb.WriteString(`<IsTruncated>true</IsTruncated><NextContinuationToken>tok2</NextContinuationToken>`)
		} else {
			page = 2
			sb.WriteString(`<Contents><Key>b.html</Key><ETag>"bbb"</ETag></Contents>`)
			sb.WriteString(`<IsTruncated>false</IsTruncated>`)
		}
		sb.WriteString(`</ListBucketResult>`)
		_, _ = io.WriteString(w, sb.String())
	})
	state, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if page != 2 {
		t.Errorf("continuation not followed (stopped at page %d)", page)
	}
	if state["a.html"] != "aaa" || state["b.html"] != "bbb" {
		t.Errorf("List() = %v, want both pages' keys with trimmed ETags", state)
	}
}

// loggerStub records Detail calls so we can assert Put/Delete logging.
type loggerStub struct{ lines []string }

func (l *loggerStub) Detail(category, label string, kv ...any) {
	l.lines = append(l.lines, fmt.Sprintf("%s/%s %v", category, label, kv))
}

func TestPutDeleteLog(t *testing.T) {
	t.Parallel()
	log := &loggerStub{}
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	c.Name = "r2"
	c.Logger = log
	if err := c.Put(context.Background(), "x.html", []byte("hi")); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(context.Background(), "x.html"); err != nil {
		t.Fatal(err)
	}
	if len(log.lines) != 2 || !strings.Contains(log.lines[0], "put x.html") {
		t.Errorf("logger lines = %v, want a put then a delete", log.lines)
	}
}

func TestPutCORS(t *testing.T) {
	var path, query, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, query = r.URL.Path, r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "b", "auto", "AK", "SK")
	c.HTTPClient = srv.Client()
	if err := c.PutCORS(context.Background(), []string{"*"}); err != nil {
		t.Fatal(err)
	}
	if path != "/b" || query != "cors=" {
		t.Errorf("request = %s?%s, want /b?cors=", path, query)
	}
	for _, want := range []string{
		"<AllowedOrigin>*</AllowedOrigin>",
		"<AllowedMethod>GET</AllowedMethod>",
		"<AllowedHeader>content-type</AllowedHeader>", // R2 rejects "*" here
	} {
		if !strings.Contains(body, want) {
			t.Errorf("CORS body missing %q:\n%s", want, body)
		}
	}
}
