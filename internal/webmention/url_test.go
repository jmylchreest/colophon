package webmention

import "testing"

func TestSafeURL(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "JaVaScRiPt:alert(1)", "java\nscript:alert(1)", "data:text/html,test", "//example.com", "/relative", "https:///missing", "https://example.com/\x00", `https://example.com\@evil.test`} {
		if got := SafeURL(raw); got != "" {
			t.Errorf("SafeURL(%q) = %q", raw, got)
		}
	}
	for _, raw := range []string{"https://example.com/post?a=1&b=2", "http://example.com/photo.png", "HTTPS://example.com/"} {
		if got := SafeURL(raw); got != raw {
			t.Errorf("SafeURL(%q) = %q", raw, got)
		}
	}
}
