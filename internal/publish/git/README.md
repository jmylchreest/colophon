# git / github-pages publisher

Publishes by **force-pushing the built tree as a single orphan commit** to a nominated branch of
a git remote. Whatever serves that branch — GitHub Pages, GitLab Pages, Codeberg Pages, a mirror,
a self-hosted bare repo — then serves the site. Uses [go-git](https://github.com/go-git/go-git)
(pure Go), so **no `git` binary is required**, and it never touches your working tree (the build
is staged in a temp repo and pushed from there).

Each publish is a fresh orphan commit, so the branch always mirrors exactly the current build —
no history to drift, no stale files to prune.

## Drivers

| Driver | `branch` default | Notes |
| ------ | ---------------- | ----- |
| `git` | `main` | Generic — any remote, any branch. |
| `github-pages` | `gh-pages` | Same driver, GitHub-friendly default; derives the Pages URL. |

## Settings

| Setting | Required | Purpose |
| ------- | -------- | ------- |
| `repo` | yes | Remote URL (`https://…`, `git@host:owner/repo`, `ssh://…`) or a local path. |
| `branch` | no | Branch to force-push (`main`, or `gh-pages` for `github-pages`). |
| `public_url` | no | Canonical URL. Auto-derived for known hosts (below); set it for a custom domain. |
| `commit_author` / `commit_email` | no | Author of the publish commit (`colophon` / `colophon@users.noreply.github.com`). |
| `commit_message` | no | Commit message (default `colophon: publish <timestamp>`). |

All settings support `{env:VAR}` / `{env:VAR:-default}`
[config interpolation](../../../docs/publishing.md#configuration-and-interpolation), e.g.
`repo: "{env:DEPLOY_REPO}"`.

### Public-URL derivation

`public_url` is auto-derived from the remote for known hosts, so you usually don't set it:

| Host | Repo | Derived URL |
|------|------|-------------|
| `github.com` | `me/blog` | `https://me.github.io/blog/` |
| `github.com` | `me/me.github.io` | `https://me.github.io/` (user/org site) |
| `gitlab.com` | `me/site` | `https://me.gitlab.io/site/` |
| `codeberg.org` | `me/pages` | `https://me.codeberg.page/` |

Any other host (self-hosted, or a custom domain via a `CNAME` file) resolves no URL — set
`public_url` explicitly.

## Credentials (env-only)

Authentication follows the remote scheme — secrets never live in config:

| Remote | Auth |
| ------ | ---- |
| `https://…` | A token from `GITHUB_TOKEN` / `GH_TOKEN` / `GIT_TOKEN` (e.g. a GitHub fine-grained PAT with **Contents: write**, or `GITHUB_TOKEN` in Actions). |
| `git@…` / `ssh://…` | Your SSH agent. |
| local path | None. |

## Config

```yaml
publishers:
  - id: pages
    driver: github-pages                 # branch defaults to gh-pages
    repo: "git@github.com:me/blog.git"    # SSH → ssh-agent

  - id: gitlab
    driver: git
    repo: "https://gitlab.com/me/site.git"   # GIT_TOKEN → HTTPS push
    branch: pages
```

See [docs/publishing.md](../../../docs/publishing.md#git-based-hosting-github--gitlab--codeberg-pages)
for per-provider walkthroughs.
