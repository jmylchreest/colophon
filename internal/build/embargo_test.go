package build

import (
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

func TestConsiderEmbargo(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	sooner := now.Add(30 * time.Minute)
	past := now.Add(-time.Hour)

	if got := considerEmbargo(nil, markdown.Frontmatter{PublishAfter: &future}, now); got == nil || !got.Equal(future) {
		t.Errorf("future should be tracked, got %v", got)
	}
	if got := considerEmbargo(&future, markdown.Frontmatter{PublishAfter: &sooner}, now); got == nil || !got.Equal(sooner) {
		t.Errorf("sooner should win, got %v", got)
	}
	if got := considerEmbargo(nil, markdown.Frontmatter{PublishAfter: &past}, now); got != nil {
		t.Errorf("past embargo should be ignored, got %v", got)
	}
	if got := considerEmbargo(nil, markdown.Frontmatter{Draft: true, PublishAfter: &future}, now); got != nil {
		t.Errorf("draft embargo should be ignored, got %v", got)
	}
}

func doc(title string, draft bool, publishAfter *time.Time) core.Content {
	return core.Content{
		SourcePath: "posts/" + title + ".md",
		Document: markdown.Document{
			Frontmatter: markdown.Frontmatter{Title: title, Draft: draft, PublishAfter: publishAfter},
			Body:        "body",
		},
	}
}

func TestBuildPagesEmbargo(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	future := time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC)
	past := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	draftFuture := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)

	docs := []sourceDoc{
		{doc: doc("Pub", false, nil)},
		{doc: doc("Future", false, &future)},
		{doc: doc("Past", false, &past)},
		{doc: doc("DF", true, &draftFuture)},
	}
	titles := func(ps []page) map[string]bool {
		m := map[string]bool{}
		for _, p := range ps {
			m[p.Title] = true
		}
		return m
	}

	t.Run("production excludes future + draft", func(t *testing.T) {
		pages, _, next, err := buildPages(docs, false, now, "/", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		got := titles(pages)
		if !got["Pub"] || !got["Past"] || got["Future"] || got["DF"] {
			t.Errorf("got %v", got)
		}
		if next == nil || !next.Equal(future) {
			t.Errorf("next embargo = %v, want 13:00", next)
		}
	})

	t.Run("preview includes embargoed, marked", func(t *testing.T) {
		pages, _, _, err := buildPages(docs, true, now, "/", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !titles(pages)["Future"] || !titles(pages)["DF"] {
			t.Error("preview should include future + draft")
		}
		for _, p := range pages {
			if p.Title == "Future" && !p.Embargoed {
				t.Error("Future should be marked embargoed")
			}
		}
	})

	t.Run("after timestamp auto-includes", func(t *testing.T) {
		later := time.Date(2026, 6, 15, 13, 30, 0, 0, time.UTC)
		pages, _, next, err := buildPages(docs, false, later, "/", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !titles(pages)["Future"] {
			t.Error("Future should auto-include once now is past its embargo")
		}
		if next != nil {
			t.Errorf("no non-draft embargo should remain, got %v", next)
		}
	})
}
