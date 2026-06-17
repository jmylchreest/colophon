// Package git implements the "git" publisher (and the "github-pages" alias): it writes the
// built tree onto a nominated branch of a remote repository and force-pushes it, so the branch
// always mirrors the latest build. It works with any git host whose pages are served from a
// branch — GitHub Pages, GitLab Pages, Codeberg Pages, SourceHut, a self-hosted bare repo, a
// mirror — using go-git (no `git` binary required).
//
// The push is a fresh orphan commit force-pushed to the branch: the branch is generated output,
// not a history to preserve. Authentication is HTTPS with a token (GITHUB_TOKEN / GH_TOKEN /
// GIT_TOKEN) for http(s) remotes, or the SSH agent for git@ remotes; a local path needs none.
package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/jmylchreest/colophon/internal/clog"
	pubconfig "github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

func init() {
	publish.Register("git", New)
	// github-pages is the same driver with friendlier defaults (branch gh-pages) so it shows
	// up by name in `colophon doctor`; the provider table derives its public URL.
	publish.Register("github-pages", New)
	publish.RegisterEnv("git", "GITHUB_TOKEN", "GH_TOKEN", "GIT_TOKEN")
	publish.RegisterEnv("github-pages", "GITHUB_TOKEN", "GH_TOKEN", "GIT_TOKEN")
}

// New builds a git publisher. Required: repo (a remote URL or local path). branch defaults to
// gh-pages for the github-pages alias, else main.
func New(root string, cfg pubconfig.PublisherConfig) (core.Publisher, error) {
	get := func(k string) string { s, _ := cfg.Settings[k].(string); return strings.TrimSpace(s) }
	repo := get("repo")
	if repo == "" {
		return nil, fmt.Errorf("git publisher %q: 'repo' is required (a remote URL or local path)", cfg.ID)
	}
	branch := get("branch")
	if branch == "" {
		branch = "main"
		if cfg.Driver == "github-pages" {
			branch = "gh-pages"
		}
	}
	author := get("commit_author")
	if author == "" {
		author = "colophon"
	}
	email := get("commit_email")
	if email == "" {
		email = "colophon@users.noreply.github.com"
	}
	return &publisher{
		id:          cfg.ID,
		driver:      cfg.Driver,
		repo:        repo,
		branch:      branch,
		author:      author,
		email:       email,
		messageTmpl: get("commit_message"),
		publicURL:   strings.TrimRight(get("public_url"), "/"),
	}, nil
}

type publisher struct {
	id          string
	driver      string // "git" or "github-pages" (the registered alias)
	repo        string
	branch      string
	author      string
	email       string
	messageTmpl string
	publicURL   string
	log         *clog.Logger
}

func (p *publisher) SetLogger(l *clog.Logger) { p.log = l }

func (p *publisher) ID() string { return p.id }

func (p *publisher) Driver() string {
	if p.driver != "" {
		return p.driver
	}
	return "git"
}

// --- the core.Publisher methods that don't apply to a git push ---

var errUsePush = errors.New("git: this driver deploys via TreePublisher.Push, not Publisher.Commit")

func (p *publisher) Hash(string, []byte) string { return "" }
func (p *publisher) Protected(string) bool      { return false }
func (p *publisher) Deployed(context.Context) (core.State, bool, error) {
	return nil, false, nil // a git branch can't be enumerated for an incremental diff
}
func (p *publisher) Commit(context.Context, fs.FS, *core.Plan) (core.Result, error) {
	return core.Result{}, errUsePush
}
func (p *publisher) Invalidate(context.Context, []string) error { return nil }

// Push writes the built tree into a fresh repo, commits it, and force-pushes it to the
// configured branch of the remote — so the branch becomes an orphan commit holding exactly the
// build. Implements core.TreePublisher.
func (p *publisher) Push(ctx context.Context, tree fs.FS) (core.Result, error) {
	dir, err := os.MkdirTemp("", "colophon-git-*")
	if err != nil {
		return core.Result{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	repo, err := gogit.PlainInit(dir, false)
	if err != nil {
		return core.Result{}, fmt.Errorf("git init: %w", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		return core.Result{}, err
	}

	total, bytes, err := writeTree(tree, dir)
	if err != nil {
		return core.Result{}, err
	}
	if err := wt.AddWithOptions(&gogit.AddOptions{All: true}); err != nil {
		return core.Result{}, err
	}
	if _, err := wt.Commit(p.message(), &gogit.CommitOptions{
		Author: &object.Signature{Name: p.author, Email: p.email, When: time.Now()},
	}); err != nil {
		return core.Result{}, fmt.Errorf("git commit: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{p.repo}}); err != nil {
		return core.Result{}, err
	}

	auth, err := authFor(p.repo)
	if err != nil {
		return core.Result{}, err
	}
	head, err := repo.Head()
	if err != nil {
		return core.Result{}, err
	}
	refspec := config.RefSpec(fmt.Sprintf("+%s:refs/heads/%s", head.Name(), p.branch))
	p.detail("push", p.repo, "branch", p.branch, "files", total)
	if err := repo.PushContext(ctx, &gogit.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refspec},
		Auth:       auth,
		Force:      true,
	}); err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return core.Result{}, fmt.Errorf("git push %s (%s): %w", p.repo, p.branch, err)
	}

	url, _ := p.CanonicalURL(ctx)
	return core.Result{Total: total, Uploaded: total, Bytes: bytes, URL: url}, nil
}

// CanonicalURL is the configured public_url, else the provider-derived Pages URL for a known
// host (github.com → <owner>.github.io/<repo>, …), else "". Set public_url for a custom domain.
func (p *publisher) CanonicalURL(context.Context) (string, error) {
	if p.publicURL != "" {
		return p.publicURL, nil
	}
	host, owner, repo := parseRemote(p.repo)
	if f := providerURL[host]; f != nil {
		return f(owner, repo), nil
	}
	return "", nil
}

func (p *publisher) message() string {
	if p.messageTmpl != "" {
		return p.messageTmpl
	}
	return "colophon: publish " + time.Now().UTC().Format(time.RFC3339)
}

func (p *publisher) detail(action, label string, kv ...any) {
	if p.log != nil {
		p.log.Detail("PUBLISH", p.id, append([]any{action, label}, kv...)...)
	}
}

// writeTree copies every file of the built tree into dir, returning the file count and total
// bytes.
func writeTree(tree fs.FS, dir string) (int, int64, error) {
	var total int
	var bytes int64
	err := fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(tree, p)
		if err != nil {
			return err
		}
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			return err
		}
		total++
		bytes += int64(len(b))
		return nil
	})
	return total, bytes, err
}

// authFor selects a go-git auth method from the remote URL: HTTPS uses a token from the
// environment (GITHUB_TOKEN / GH_TOKEN / GIT_TOKEN) when present; an ssh remote uses the SSH
// agent; a local path needs none.
func authFor(repoURL string) (transport.AuthMethod, error) {
	switch {
	case strings.HasPrefix(repoURL, "http://"), strings.HasPrefix(repoURL, "https://"):
		if tok := firstEnv("GITHUB_TOKEN", "GH_TOKEN", "GIT_TOKEN"); tok != "" {
			return &githttp.BasicAuth{Username: "x-access-token", Password: tok}, nil
		}
		return nil, nil
	case strings.HasPrefix(repoURL, "file://"), strings.HasPrefix(repoURL, "/"),
		strings.HasPrefix(repoURL, "."):
		return nil, nil // local repo, no auth
	default: // ssh: git@host:owner/repo or ssh://git@host/owner/repo
		user := "git"
		if at := strings.Index(repoURL, "@"); at >= 0 {
			if s := strings.TrimPrefix(repoURL[:at], "ssh://"); !strings.ContainsAny(s, "/:") {
				user = s
			}
		}
		a, err := gitssh.NewSSHAgentAuth(user)
		if err != nil {
			return nil, fmt.Errorf("git: ssh auth unavailable (%w); start ssh-agent, or use an https remote with GITHUB_TOKEN", err)
		}
		return a, nil
	}
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
