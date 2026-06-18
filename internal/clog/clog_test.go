package clog

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFilters(t *testing.T) {
	tests := []struct {
		spec        string
		wantType    string
		wantPattern string
		wantLevel   string
		wantCount   int
	}{
		{"", "", "", "", 0},
		{"debug", "category", "*", "debug", 1},
		{"category=warn", "category", "*", "warn", 1},
		{"label=info", "label", "*", "info", 1},
		{"source:file:internal/publish/*=debug", "source:file", "internal/publish/*", "debug", 1},
		{"bogus", "", "", "", 0},  // not a level
		{"=debug", "", "", "", 0}, // empty target
		{"a=info,b=warn", "a", "*", "info", 2},
	}
	for _, tt := range tests {
		got := ParseFilters(tt.spec)
		if len(got) != tt.wantCount {
			t.Fatalf("ParseFilters(%q) count = %d, want %d", tt.spec, len(got), tt.wantCount)
		}
		if tt.wantCount == 0 {
			continue
		}
		f := got[0]
		if f.Type != tt.wantType || f.Pattern != tt.wantPattern || f.Level != tt.wantLevel || !f.Enabled {
			t.Errorf("ParseFilters(%q)[0] = %+v, want type=%s pattern=%s level=%s", tt.spec, f, tt.wantType, tt.wantPattern, tt.wantLevel)
		}
	}
}

func TestStepDetailLevels(t *testing.T) {
	var buf bytes.Buffer
	// Default (Info): Step shows, Detail is suppressed.
	l := New(Options{Writer: &buf})
	l.Step("BUILD", "", "pages", 3)
	l.Detail("BUILD", "src", "file", "a.md")
	out := buf.String()
	if !strings.Contains(out, "msg=BUILD") || !strings.Contains(out, "pages=3") {
		t.Errorf("Step not emitted at Info: %q", out)
	}
	if strings.Contains(out, "file=a.md") {
		t.Errorf("Detail leaked at Info: %q", out)
	}

	// Verbose (Debug): Detail now shows with its label attribute.
	buf.Reset()
	v := New(Options{Writer: &buf, Verbose: true})
	v.Detail("BUILD", "src", "file", "a.md")
	if out := buf.String(); !strings.Contains(out, "label=src") || !strings.Contains(out, "file=a.md") {
		t.Errorf("Detail not emitted at Debug: %q", out)
	}
}

func TestCategoryRedundancy(t *testing.T) {
	// Text drops the category attribute (the message carries it); JSON keeps it.
	var text bytes.Buffer
	New(Options{Writer: &text}).Step("BUILD", "", "pages", 3)
	if got := text.String(); strings.Contains(got, "category=") {
		t.Errorf("text should not repeat category: %q", got)
	}

	var js bytes.Buffer
	New(Options{Writer: &js, JSON: true}).Step("BUILD", "", "pages", 3)
	if got := js.String(); !strings.Contains(got, `"category":"BUILD"`) || !strings.Contains(got, `"msg":"BUILD"`) {
		t.Errorf("JSON should keep category as a field: %q", got)
	}
}

func TestVerboseSourceCallSite(t *testing.T) {
	// --verbose attaches source, attributed to the caller (this test file), not clog.go.
	var buf bytes.Buffer
	New(Options{Writer: &buf, Verbose: true}).Step("BUILD", "", "pages", 3)
	out := buf.String()
	if !strings.Contains(out, "source=") || !strings.Contains(out, "clog_test.go") {
		t.Errorf("verbose Step should carry caller source: %q", out)
	}
	// Default (non-verbose) omits source entirely.
	buf.Reset()
	New(Options{Writer: &buf}).Step("BUILD", "", "pages", 3)
	if strings.Contains(buf.String(), "source=") {
		t.Errorf("non-verbose Step should not carry source: %q", buf.String())
	}
}

func TestNilLoggerNoop(t *testing.T) {
	var l *Logger
	l.Step("X", "y") // must not panic
	l.Detail("X", "y")
	if l.Verbose() {
		t.Error("nil logger should report not verbose")
	}
}
