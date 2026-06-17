package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
)

func TestInterpolate(t *testing.T) {
	t.Parallel()
	vars := map[string]string{"dir": "/tmp/site", "domain": "blog.example.com", "public_url": "https://blog.example.com"}
	cases := []struct {
		in   []string
		want []string
		err  bool
	}{
		{[]string{"surge", "{dir}", "{domain}"}, []string{"surge", "/tmp/site", "blog.example.com"}, false},
		{[]string{"deploy", "--url={public_url}/"}, []string{"deploy", "--url=https://blog.example.com/"}, false},
		{[]string{"echo", "a{{b}}c"}, []string{"echo", "a{b}c"}, false}, // escaped braces
		{[]string{"x", "{nope}"}, nil, true},                            // unknown placeholder
		{[]string{"x", "{dir"}, nil, true},                              // unterminated
	}
	for _, c := range cases {
		got, err := interpolate(c.in, vars)
		if c.err {
			if err == nil {
				t.Errorf("interpolate(%v) = %v, want error", c.in, got)
			}
			continue
		}
		if err != nil || strings.Join(got, "\x00") != strings.Join(c.want, "\x00") {
			t.Errorf("interpolate(%v) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
}

func TestToArgvRejectsStringAndEmpty(t *testing.T) {
	t.Parallel()
	if _, err := toArgv("surge ./ site"); err == nil {
		t.Error("a string command should be rejected (argv list only)")
	}
	if _, err := toArgv([]any{}); err == nil {
		t.Error("an empty command should be rejected")
	}
	if _, err := toArgv(nil); err == nil {
		t.Error("a missing command should be rejected")
	}
	got, err := toArgv([]any{"surge", "{dir}", 42})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2] != "42" {
		t.Errorf("toArgv scalar coercion = %v, want [surge {dir} 42]", got)
	}
}

func TestClassifyAndContentType(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, kind string }{
		{"index.html", "page"},
		{"posts/a/index.htm", "page"},
		{"sitemap.xml", "sitemap"},
		{"rss.xml", "feed"},
		{"robots.txt", "meta"},
		{"assets/cat.png", "asset"},
		{"style.css", "asset"},
	}
	for _, c := range cases {
		if got := classify(c.name); got != c.kind {
			t.Errorf("classify(%q) = %q, want %q", c.name, got, c.kind)
		}
	}
	if ct := contentType("a.css"); ct != "text/css" {
		t.Errorf("contentType(a.css) = %q, want text/css", ct)
	}
	if ct := contentType("a.unknownext"); ct != "application/octet-stream" {
		t.Errorf("contentType(unknown) = %q, want octet-stream", ct)
	}
}

func TestNewRequiresCommand(t *testing.T) {
	t.Parallel()
	if _, err := New("", config.PublisherConfig{ID: "c", Driver: "command",
		Settings: map[string]any{"public_url": "https://x"}}); err == nil {
		t.Error("New without a command should error")
	}
}

// TestPushRunsCommandInDir drives a real child process (this test binary re-invoked as the
// "deploy command" — see TestHelperProcess) and checks it runs in the materialised dir with the
// interpolated args, COLOPHON_* env, and a readable manifest.
func TestPushRunsCommandInDir(t *testing.T) {
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	pub, err := New("", config.PublisherConfig{ID: "c", Driver: "command",
		Settings: map[string]any{
			"public_url": "https://blog.example.com",
			"command":    []any{os.Args[0], "-test.run=TestHelperProcess", "--", "{dir}", "{public_url}"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	gp, ok := pub.(core.TreePublisher)
	if !ok {
		t.Fatal("command driver does not implement core.TreePublisher")
	}
	tree := fstest.MapFS{
		"index.html":       {Data: []byte("<h1>hi</h1>")},
		"assets/style.css": {Data: []byte("body{}")},
	}
	res, err := gp.Push(context.Background(), tree)
	if err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if res.Total != 2 || res.URL != "https://blog.example.com" {
		t.Errorf("Result = %+v, want Total=2 URL=https://blog.example.com", res)
	}
}

func TestPushPropagatesCommandFailure(t *testing.T) {
	pub, _ := New("", config.PublisherConfig{ID: "c", Driver: "command",
		Settings: map[string]any{"command": []any{os.Args[0], "-test.run=TestHelperProcess", "--", "fail"}}})
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	gp := pub.(core.TreePublisher)
	if _, err := gp.Push(context.Background(), fstest.MapFS{"index.html": {Data: []byte("x")}}); err == nil {
		t.Error("a non-zero command exit should surface as an error")
	}
}

// TestHelperProcess is not a real test: it's the program the Push tests exec. It validates the
// environment colophon hands a deploy command, then exits 0 (or 1 on the "fail" sentinel).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	fail := func(format string, v ...any) {
		fmt.Fprintln(os.Stderr, "helper: "+strings.TrimSpace(fmt.Sprintf(format, v...)))
		os.Exit(1)
	}
	if len(args) > 0 && args[0] == "fail" {
		fail("asked to fail")
	}
	if len(args) < 2 {
		fail("want [dir public_url], got %v", args)
	}
	dir, publicURL := args[0], args[1]

	wd, _ := os.Getwd()
	if wd != dir {
		fail("CWD %q != dir arg %q", wd, dir)
	}
	if got := os.Getenv("COLOPHON_OUTPUT_DIR"); got != dir {
		fail("COLOPHON_OUTPUT_DIR %q != %q", got, dir)
	}
	if got := os.Getenv("COLOPHON_PUBLIC_URL"); got != publicURL {
		fail("COLOPHON_PUBLIC_URL %q != %q", got, publicURL)
	}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		fail("index.html not in dir: %v", err)
	}
	manifest := os.Getenv("COLOPHON_MANIFEST")
	if _, err := os.Stat(filepath.Join(dir, filepath.Base(manifest))); err == nil {
		fail("manifest leaked into served dir")
	}
	b, err := os.ReadFile(manifest)
	if err != nil {
		fail("manifest unreadable: %v", err)
	}
	var doc map[string]fileInfo
	if err := json.Unmarshal(b, &doc); err != nil {
		fail("manifest not valid JSON: %v", err)
	}
	if doc["index.html"].Kind != "page" || doc["assets/style.css"].Kind != "asset" {
		fail("manifest classification wrong: %+v", doc)
	}
	os.Exit(0)
}
