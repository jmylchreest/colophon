package build

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

// fakeSource serves a fixed set of files by source-relative path.
type fakeSource struct{ files map[string]string }

func (fakeSource) ID() string                                        { return "fake" }
func (fakeSource) Driver() string                                    { return "fake" }
func (fakeSource) Documents(context.Context) ([]core.Content, error) { return nil, nil }
func (s fakeSource) Open(_ context.Context, ref string) (io.ReadCloser, error) {
	body, ok := s.files[ref]
	if !ok {
		return nil, io.EOF
	}
	return io.NopCloser(strings.NewReader(body)), nil
}
func (s fakeSource) Resolve(_ context.Context, ref string) (string, bool) {
	_, ok := s.files[ref]
	return ref, ok
}

func TestResolveAttachments(t *testing.T) {
	src := fakeSource{files: map[string]string{
		"posts/run.sh":   "echo hi\n",               // 8 bytes
		"posts/data.zip": strings.Repeat("x", 2048), // 2048 bytes
	}}
	doc := core.Content{SourcePath: "posts/media.md"}
	doc.Frontmatter = markdown.Frontmatter{Attachments: []markdown.Attachment{
		{Path: "run.sh"},
		{Path: "data.zip", Label: "Dataset", Description: "Raw measurements", Feed: true},
	}}
	it := included{c: doc, src: src, slug: "posts/media"}

	got := resolveAttachments(context.Background(), it, "/", "https://x.test", core.NewRouter(nil, nil))
	if len(got) != 2 {
		t.Fatalf("want 2 attachments, got %d", len(got))
	}

	run := got[0]
	if run.Label != "run.sh" { // default label is the file name
		t.Errorf("default label: got %q", run.Label)
	}
	if run.URL != "/posts/media/run.sh" {
		t.Errorf("co-located url: got %q", run.URL)
	}
	if run.Abs != "https://x.test/posts/media/run.sh" {
		t.Errorf("absolute url: got %q", run.Abs)
	}
	if run.Bytes != 8 {
		t.Errorf("size: got %d want 8", run.Bytes)
	}
	if run.Feed {
		t.Errorf("run.sh should not be a feed attachment")
	}
	if run.Type != "application/x-sh" && run.Type != "text/x-sh" && run.Type == "" {
		t.Errorf("expected a mime type for .sh, got %q", run.Type)
	}

	data := got[1]
	if data.Label != "Dataset" {
		t.Errorf("explicit label: got %q", data.Label)
	}
	if !data.Feed {
		t.Errorf("data.zip should be a feed attachment")
	}
	if data.Type != "application/zip" {
		t.Errorf("zip mime: got %q", data.Type)
	}
	if data.Bytes != 2048 || data.Size == "" {
		t.Errorf("size: got %d (%q)", data.Bytes, data.Size)
	}
	if data.Desc != "Raw measurements" {
		t.Errorf("description: got %q", data.Desc)
	}
	if data.TypeLabel != "ZIP" || run.TypeLabel != "SH" {
		t.Errorf("type labels: got %q / %q", data.TypeLabel, run.TypeLabel)
	}
}

func TestAttachmentsHTML(t *testing.T) {
	if got := attachmentsHTML(nil); got != "" {
		t.Errorf("empty attachments should render nothing, got %q", got)
	}
	as := []pageAttachment{
		{URL: "/posts/p/run.sh", Label: "Build script", Desc: "Sets up the toolchain", TypeLabel: "SH", Size: "2.9 KB"},
		{URL: "/posts/p/data.zip", Label: "Dataset", TypeLabel: "ZIP", Size: "2.3 MB"},
	}
	out := attachmentsHTML(as)
	for _, want := range []string{
		`class="post-downloads"`, `<a class="dl" href="/posts/p/run.sh" download>`,
		`class="dl-label">Build script<`, `class="dl-desc">Sets up the toolchain<`,
		`class="dl-type">SH<`, `class="dl-size">2.9 KB<`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("fragment missing %q in:\n%s", want, out)
		}
	}
	// The second item has no description, so no dl-desc span should be emitted for it.
	if strings.Count(out, "dl-desc") != 1 {
		t.Errorf("expected exactly one dl-desc, got %d:\n%s", strings.Count(out, "dl-desc"), out)
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{0: "", 512: "512 B", 2048: "2.0 KB", 5 * 1024 * 1024: "5.0 MB"}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}
