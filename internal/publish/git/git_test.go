package git

import (
	"context"
	"testing"
	"testing/fstest"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
)

func TestPushToLocalBareRepo(t *testing.T) {
	t.Parallel()
	bare := t.TempDir()
	if _, err := gogit.PlainInit(bare, true); err != nil {
		t.Fatal(err)
	}
	pub, err := New("", config.PublisherConfig{ID: "gh", Driver: "github-pages",
		Settings: map[string]any{"repo": bare, "branch": "gh-pages"}})
	if err != nil {
		t.Fatal(err)
	}
	gp, ok := pub.(core.TreePublisher)
	if !ok {
		t.Fatal("git driver does not implement core.TreePublisher")
	}

	tree := fstest.MapFS{
		"index.html":         {Data: []byte("<h1>hi</h1>")},
		"posts/a/index.html": {Data: []byte("post a")},
		"assets/style.css":   {Data: []byte("body{}")},
	}
	res, err := gp.Push(context.Background(), tree)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || res.Uploaded != 3 {
		t.Errorf("Result = %+v, want Total/Uploaded = 3", res)
	}

	// Read the pushed branch back out of the bare repo.
	repo, err := gogit.PlainOpen(bare)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Reference(plumbing.NewBranchReferenceName("gh-pages"), true)
	if err != nil {
		t.Fatalf("branch gh-pages was not pushed: %v", err)
	}
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatal(err)
	}
	commitTree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	_ = commitTree.Files().ForEach(func(f *object.File) error {
		c, _ := f.Contents()
		got[f.Name] = c
		return nil
	})
	want := map[string]string{
		"index.html":         "<h1>hi</h1>",
		"posts/a/index.html": "post a",
		"assets/style.css":   "body{}",
	}
	for name, body := range want {
		if got[name] != body {
			t.Errorf("pushed %q = %q, want %q", name, got[name], body)
		}
	}

	// A second push force-replaces the branch (orphan): fewer files this time.
	if _, err := gp.Push(context.Background(), fstest.MapFS{"index.html": {Data: []byte("v2")}}); err != nil {
		t.Fatal(err)
	}
}

func TestCommitIsNotSupported(t *testing.T) {
	t.Parallel()
	pub, _ := New("", config.PublisherConfig{ID: "gh", Driver: "github-pages",
		Settings: map[string]any{"repo": "/tmp/x"}})
	if _, err := pub.Commit(context.Background(), fstest.MapFS{}, &core.Plan{}); err == nil {
		t.Error("Commit should return an error directing the caller to Push")
	}
}

func TestParseRemote(t *testing.T) {
	t.Parallel()
	cases := []struct{ url, host, owner, repo string }{
		{"git@github.com:me/blog.git", "github.com", "me", "blog"},
		{"https://github.com/me/blog.git", "github.com", "me", "blog"},
		{"https://github.com/me/blog", "github.com", "me", "blog"},
		{"ssh://git@codeberg.org/me/pages.git", "codeberg.org", "me", "pages"},
		{"git@gitlab.com:group/site", "gitlab.com", "group", "site"},
		{"/local/path", "", "", ""},
	}
	for _, c := range cases {
		h, o, r := parseRemote(c.url)
		if h != c.host || o != c.owner || r != c.repo {
			t.Errorf("parseRemote(%q) = (%q,%q,%q), want (%q,%q,%q)", c.url, h, o, r, c.host, c.owner, c.repo)
		}
	}
}

func TestCanonicalURL(t *testing.T) {
	t.Parallel()
	cases := []struct{ repo, public, want string }{
		{"git@github.com:me/blog.git", "", "https://me.github.io/blog/"},
		{"git@github.com:me/me.github.io.git", "", "https://me.github.io/"},
		{"https://gitlab.com/me/site.git", "", "https://me.gitlab.io/site/"},
		{"git@codeberg.org:me/pages.git", "", "https://me.codeberg.page/"},
		{"git@example.com:me/blog.git", "", ""},                                                 // unknown host → no URL
		{"git@example.com:me/blog.git", "https://blog.example.com", "https://blog.example.com"}, // public_url wins
	}
	for _, c := range cases {
		pub, err := New("", config.PublisherConfig{ID: "g", Driver: "git",
			Settings: map[string]any{"repo": c.repo, "public_url": c.public}})
		if err != nil {
			t.Fatal(err)
		}
		got, _ := pub.(core.CanonicalURLer).CanonicalURL(context.Background())
		if got != c.want {
			t.Errorf("CanonicalURL(repo=%q public=%q) = %q, want %q", c.repo, c.public, got, c.want)
		}
	}
}
