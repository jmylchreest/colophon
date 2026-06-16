package markdown

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	src := []byte("---\ntitle: Hello\ndate: 2026-06-14\ndraft: true\n---\n\n# Body\n\ntext\n")
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Frontmatter.Title != "Hello" {
		t.Errorf("title = %q", doc.Frontmatter.Title)
	}
	if !doc.Frontmatter.Draft {
		t.Error("draft should be true")
	}
	if got, want := doc.Frontmatter.Date, time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("date = %v, want %v", got, want)
	}
	if doc.Body != "# Body\n\ntext\n" {
		t.Errorf("body = %q", doc.Body)
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	src := []byte("# Just markdown\n")
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Frontmatter.Title != "" {
		t.Errorf("expected empty frontmatter, got %q", doc.Frontmatter.Title)
	}
	if doc.Body != string(src) {
		t.Errorf("body = %q", doc.Body)
	}
}

func TestRoundTrip(t *testing.T) {
	doc := &Document{
		Frontmatter: Frontmatter{Title: "Hello", Persona: "technical", Draft: true},
		Body:        "Body **text**.\n",
	}
	out, err := doc.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Frontmatter, doc.Frontmatter) {
		t.Errorf("frontmatter round-trip: got %+v want %+v", got.Frontmatter, doc.Frontmatter)
	}
	if got.Body != doc.Body {
		t.Errorf("body round-trip: got %q want %q", got.Body, doc.Body)
	}
	if strings.Contains(string(out), "date:") {
		t.Errorf("zero date should be omitted, got:\n%s", out)
	}
}
