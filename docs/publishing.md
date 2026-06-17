# Publishing

colophon separates **what/where** (environments) from **how** (publishers).

- A **publisher** is a deploy mechanism: copy to a folder, upload to Cloudflare Pages, push
  to an object store. Publishers are pure mechanism and carry no policy.
- An **environment** is a named build+deploy profile: which publishers to deploy to, whether
  to include drafts, and optional overrides (title, base_url, **theme**).

```yaml
publishers:
  - id: local
    driver: local
    path: ./dist

environments:
  - name: production
    publish: [local]
    allow_publish: false   # safety latch: requires --allow-publish to deploy
```

```sh
colophon publish --env production --allow-publish
```

## Publishers

| Driver | Purpose |
|--------|---------|
| `local` | Copy the built tree to a directory (offline preview / diffing). |
| `cloudflare-pages` | Deploy the site to Cloudflare Pages (direct upload). |
| `cloudflare-r2` | Upload files to an S3-compatible object store (R2, S3, MinIO). |
| `git` | Force-push the built tree to a branch of any git remote (GitHub/GitLab/Codeberg Pages, mirrors, self-hosted). |
| `github-pages` | The `git` driver with GitHub-friendly defaults (branch `gh-pages`). |

### Secrets and permissions

Deploy credentials are **never** read from config — they come from the environment, so they
never pass through the agent or the YAML:

| Publisher | Secret env vars | Token permission |
|-----------|-----------------|------------------|
| `cloudflare-pages` | `CLOUDFLARE_API_TOKEN` | Account → Cloudflare Pages → **Edit** |
| `cloudflare-r2` | `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` (or `AWS_*`) | R2 → **Object Read & Write** (+ bucket-create for `--create`) |
| `git` / `github-pages` | `GITHUB_TOKEN` / `GH_TOKEN` / `GIT_TOKEN` (HTTPS remotes only) | Repo contents → **write** (e.g. a GitHub fine-grained PAT or `GITHUB_TOKEN` in Actions). SSH remotes use the agent — no token. |

Non-secret settings (account id, bucket, project) may use `{env:VAR}` interpolation in config.
Per-publisher detail lives in each driver's README
([cloudflare-pages](../internal/publish/cloudflare/README.md),
[cloudflare-r2](../internal/publish/r2/README.md)).

### Provisioning with `--create`

`colophon publish --env <name> --create` provisions destinations before deploying:
`cloudflare-pages` creates the Pages project, `cloudflare-r2` creates the bucket. Both are
idempotent — an existing destination is left untouched.

### Generic S3 / MinIO

The `cloudflare-r2` driver is plain S3 (SigV4). Point it at any S3-compatible store with an
`endpoint` (and a `region`) instead of an `account_id`:

```yaml
publishers:
  - id: s3
    driver: cloudflare-r2
    bucket: my-assets
    endpoint: "https://s3.us-east-1.amazonaws.com"   # or http://localhost:9000 (MinIO)
    region: us-east-1
    public_url: "https://my-assets.s3.us-east-1.amazonaws.com"
```

## Git-based hosting (GitHub / GitLab / Codeberg Pages)

The `git` driver publishes by **force-pushing the built tree as a single orphan commit** to a
nominated branch of a git remote. Whatever serves that branch — GitHub Pages, GitLab Pages,
Codeberg Pages, a mirror, a self-hosted bare repo — then serves the site. It uses
[go-git](https://github.com/go-git/go-git) (pure Go), so **no `git` binary is required**.

Because each publish is a fresh orphan commit, the branch always mirrors exactly the current
build — there's no history to drift and no stale files to prune. It never touches your working
tree: the build is staged in a temp repo and pushed from there.

| Setting | Default | Purpose |
|---------|---------|---------|
| `repo` | *(required)* | Remote URL (`https://…`, `git@host:owner/repo`, `ssh://…`) or local path. |
| `branch` | `main` (`gh-pages` for `github-pages`) | The branch to force-push. |
| `public_url` | *(provider-derived)* | The site's canonical URL. Auto-derived for known hosts (below); set it for a custom domain. |
| `commit_author` / `commit_email` | `colophon` / `colophon@users.noreply.github.com` | Author of the publish commit. |
| `commit_message` | `colophon: publish <timestamp>` | Commit message. |

`public_url` is auto-derived from the remote for known hosts, so you usually don't set it:

| Host | Repo | Derived URL |
|------|------|-------------|
| `github.com` | `me/blog` | `https://me.github.io/blog/` |
| `github.com` | `me/me.github.io` | `https://me.github.io/` (user/org site) |
| `gitlab.com` | `me/site` | `https://me.gitlab.io/site/` |
| `codeberg.org` | `me/pages` | `https://me.codeberg.page/` |

Anything else (a self-hosted host, a custom domain via a `CNAME`) resolves no URL — set
`public_url` explicitly.

### GitHub Pages

```yaml
publishers:
  - id: pages
    driver: github-pages              # branch defaults to gh-pages
    repo: "git@github.com:me/blog.git"   # SSH: pushes via your ssh-agent

environments:
  - name: production
    publish: [pages]
    allow_publish: true
```

In GitHub Actions, use an HTTPS remote and the workflow token instead of SSH:

```yaml
  - id: pages
    driver: github-pages
    repo: "https://github.com/me/blog.git"   # GITHUB_TOKEN → push over HTTPS
```

Then point the repo's **Settings → Pages** at the `gh-pages` branch.

### GitLab Pages

```yaml
  - id: pages
    driver: git
    repo: "git@gitlab.com:me/site.git"
    branch: pages                      # match your .gitlab-ci.yml `pages` job source
```

GitLab serves Pages from a CI job, so the branch is whatever your `pages:` job builds from.

### Codeberg Pages

```yaml
  - id: pages
    driver: git
    repo: "git@codeberg.org:me/pages.git"
    branch: pages                      # Codeberg serves the `pages` branch
```

### Any git remote

`git` is not GitHub-specific — push to a mirror, a self-hosted Forgejo/Gitea, or a local bare
repo (handy for tests). Set `public_url` since the host is unknown:

```yaml
  - id: mirror
    driver: git
    repo: "git@git.example.com:web/site.git"
    branch: deploy
    public_url: "https://www.example.com"
```

**Authentication** follows the remote scheme: an `https://` remote uses a token from
`GITHUB_TOKEN` / `GH_TOKEN` / `GIT_TOKEN`; an `git@…` / `ssh://` remote uses your SSH agent; a
local path needs neither.

## Per-environment overrides

An environment can override any publisher *setting* via `overrides`, keyed by publisher id —
so one publisher definition serves several environments. For example, a single `local`
publisher can write a **distinct output directory per environment** (handy for previewing a
different theme side by side) without defining a publisher per env:

```yaml
publishers:
  - id: local
    driver: local
    path: ./dist            # default output dir

environments:
  - name: dist
    publish: [local]        # → ./dist
  - name: text
    publish: [local]
    theme: minimal
    overrides:
      local:
        path: ./dist-text   # same publisher, a different dir for this env
```

Overrides also carry per-environment publisher tweaks like a Cloudflare Pages `branch`.

## Routing assets to an object store

Shipping large or numerous images with a Pages/Workers deployment can exhaust its file
budget. **Routing** sends matching paths to a different publisher — typically images to an
object store — and rewrites their URLs to that store's public base.

```yaml
sites:
  - id: main
    routing:
      - match: "**/assets/**"          # glob; ** crosses slashes, * does not
        publisher: r2                  # rewrite target inherited from the r2 publisher

publishers:
  - id: r2
    driver: cloudflare-r2
    bucket: "{env:R2_BUCKET:-my-assets}"
    account_id: "{env:CLOUDFLARE_ACCOUNT_ID}"
    public_url: "{env:R2_PUBLIC_URL:-}"  # optional; auto-discovered for R2 (see below)

environments:
  - name: production
    publish: [cf, r2]    # HTML to Pages, routed images to R2
```

How it works:

1. **Build** rewrites every routed image reference to the route's URL + path, so the HTML
   points at the object store.
2. **Publish** partitions the tree: the route's publisher (`r2`) receives only the matched
   files; every other publisher (`cf`) receives the unrouted remainder.

The route's URL is resolved as: the route's own `base_url`, else the target publisher's
`public_url`, else — on `publish`, for Cloudflare R2 with a `CLOUDFLARE_API_TOKEN` — the
bucket's **auto-discovered** URL (a connected custom domain, preferring the shortest, else
the `r2.dev` managed URL). So `publish --create` provisions the bucket, enables `r2.dev`,
and images serve from it with no URL in config; connect a custom domain later and the next
publish prefers it automatically.

A rule is **inactive until a URL resolves _and_ its publisher is deploying** in the
environment. With nothing resolvable, routing is a no-op: images stay co-located and the
whole tree goes to the default publisher — so local builds and previews work with no object
store configured.
