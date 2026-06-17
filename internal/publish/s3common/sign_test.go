package s3common

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEmptyPayloadHash(t *testing.T) {
	if got := hexSHA256([]byte("")); got != emptyPayloadHash {
		t.Errorf("emptyPayloadHash = %s, want %s", got, emptyPayloadHash)
	}
}

func TestEncodeKey(t *testing.T) {
	cases := map[string]string{
		"posts/hi/assets/cat.png": "posts/hi/assets/cat.png", // safe chars unchanged
		"a b.png":                 "a%20b.png",
		"x+y&z.png":               "x%2By%26z.png",
	}
	for in, want := range cases {
		if got := encodeKey(in); got != want {
			t.Errorf("encodeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSignV4(t *testing.T) {
	fixed := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	mk := func(body string) *http.Request {
		req, _ := http.NewRequest(http.MethodPut, "https://acct.r2.cloudflarestorage.com/bucket/key.png", strings.NewReader(body))
		return req
	}

	req := mk("hello")
	signV4(req, "/bucket/key.png", "AKID", "secret", "auto", hexSHA256([]byte("hello")), fixed)
	auth := req.Header.Get("Authorization")
	for _, want := range []string{
		"AWS4-HMAC-SHA256 Credential=AKID/20260615/auto/s3/aws4_request",
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date",
		"Signature=",
	} {
		if !strings.Contains(auth, want) {
			t.Errorf("Authorization %q missing %q", auth, want)
		}
	}
	if req.Header.Get("X-Amz-Date") != "20260615T120000Z" {
		t.Errorf("X-Amz-Date = %q", req.Header.Get("X-Amz-Date"))
	}

	// Determinism + sensitivity: same inputs → same signature, different body → different.
	req2 := mk("hello")
	signV4(req2, "/bucket/key.png", "AKID", "secret", "auto", hexSHA256([]byte("hello")), fixed)
	if req2.Header.Get("Authorization") != auth {
		t.Error("signing not deterministic for identical inputs")
	}
	req3 := mk("HELLO")
	signV4(req3, "/bucket/key.png", "AKID", "secret", "auto", hexSHA256([]byte("HELLO")), fixed)
	if req3.Header.Get("Authorization") == auth {
		t.Error("a different payload must change the signature")
	}
}
