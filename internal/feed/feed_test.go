package feed

import (
	"encoding/json"
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

func sample() (Site, []Item) {
	s := Site{Title: "My Blog", BaseURL: "https://blog.example.com", Author: "Me"}
	items := []Item{
		{
			Title:       "Hello",
			URL:         "https://blog.example.com/posts/hello/",
			Description: "a summary",
			Content:     "<p>body &amp; more</p>",
			Published:   time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC),
		},
	}
	return s, items
}

func TestRSSWellFormed(t *testing.T) {
	s, items := sample()
	out, err := RSS(s, items)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(out, new(rssRoot)); err != nil {
		t.Fatalf("RSS is not well-formed XML: %v", err)
	}
	str := string(out)
	for _, want := range []string{
		"<rss version=\"2.0\">",
		"<link>https://blog.example.com/posts/hello/</link>",
		"isPermaLink=\"true\"",
		"Sun, 14 Jun 2026 09:00:00 +0000", // RFC 1123Z per RSS spec
	} {
		if !strings.Contains(str, want) {
			t.Errorf("RSS missing %q\n%s", want, str)
		}
	}
}

func TestAtomWellFormed(t *testing.T) {
	s, items := sample()
	out, err := Atom(s, items)
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(out, new(atomRoot)); err != nil {
		t.Fatalf("Atom is not well-formed XML: %v", err)
	}
	str := string(out)
	for _, want := range []string{
		"2026-06-14T09:00:00Z", // RFC 3339 per RFC 4287
		"type=\"html\"",
		"<id>https://blog.example.com/posts/hello/</id>",
	} {
		if !strings.Contains(str, want) {
			t.Errorf("Atom missing %q\n%s", want, str)
		}
	}
}

func TestJSONFeed(t *testing.T) {
	s, items := sample()
	out, err := JSON(s, items)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("JSON feed invalid: %v", err)
	}
	if doc["version"] != "https://jsonfeed.org/version/1.1" {
		t.Errorf("version = %v", doc["version"])
	}
	its, ok := doc["items"].([]any)
	if !ok || len(its) != 1 {
		t.Fatalf("items = %v", doc["items"])
	}
}

func TestSitemap(t *testing.T) {
	out, err := Sitemap([]SitemapEntry{
		{URL: "https://blog.example.com/"},
		{URL: "https://blog.example.com/posts/hello/", LastMod: time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := xml.Unmarshal(out, new(urlset)); err != nil {
		t.Fatalf("sitemap not well-formed: %v", err)
	}
	if !strings.Contains(string(out), "<lastmod>2026-06-14</lastmod>") {
		t.Errorf("sitemap missing lastmod\n%s", out)
	}
}
