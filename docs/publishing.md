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
| `cloudflare-r2` | Upload files to Cloudflare R2 (S3 + R2 control-plane: public-URL discovery, `--create` expose). |
| `s3` | Upload files to any S3-compatible store (MinIO, B2, Wasabi, Amazon S3) — pure data plane, no SDK. |
| `tigris` | The `s3` driver with Tigris (Fly.io) defaults — needs only a bucket. |
| `git` | Force-push the built tree to a branch of any git remote (GitHub/GitLab/Codeberg Pages, mirrors, self-hosted). |
| `github-pages` | The `git` driver with GitHub-friendly defaults (branch `gh-pages`). |
| `command` | Run any CLI against the built tree (surge, Netlify, Vercel, rsync, …) — the escape hatch. |

### Configuration and interpolation

colophon has **two distinct interpolation layers** — they look similar (`{…}`) but resolve at
different times, and every driver supports the first:

**1. Config interpolation — `{env:VAR}` (all drivers, all config).** Any string value in the
config may reference the environment, resolved *before the YAML is parsed*, so it works in any
setting of any publisher (or anywhere else in the config):

| Form | Resolves to |
|------|-------------|
| `{env:VAR}` | the value of `VAR`, or empty if unset |
| `{env:VAR:-default}` | the value of `VAR`, or `default` if unset |

Values come from the process environment and from `.env` / `.env.defaults` (loaded first; a real
env var wins over a `.env` entry). `colophon env` lists every `{env:VAR}` a project references,
set or not. This is how non-secret settings stay flexible while **secrets stay in the
environment** — you never write a token into config:

```yaml
publishers:
  - id: r2
    driver: cloudflare-r2
    bucket: "{env:R2_BUCKET:-my-assets}"          # default when unset
    account_id: "{env:CLOUDFLARE_ACCOUNT_ID}"     # required; empty if unset
    public_url: "{env:R2_PUBLIC_URL:-}"           # optional; empty default
```

**2. Command interpolation — `{dir}`, `{public_url}`, … (the `command` driver only).** The
`command` publisher additionally interpolates its `command` argv *at publish time* with runtime
values (the materialised directory, the manifest path, …) and the publisher's own settings — see
[Run any CLI](#run-any-cli-the-command-publisher). The two layers compose: `{env:VAR}` is
substituted when the config loads, then `{placeholder}` when the command runs, so a single
`command` entry can use both.

Per-driver settings and their interpolation are documented in each driver's README — linked from
the [Publishers](#publishers) table targets below and listed in
[Secrets and permissions](#secrets-and-permissions).

### Secrets and permissions

Deploy credentials are **never** read from config — they come from the environment, so they
never pass through the agent or the YAML:

| Publisher | Secret env vars | Token permission |
|-----------|-----------------|------------------|
| `cloudflare-pages` | `CLOUDFLARE_API_TOKEN` | Account → Cloudflare Pages → **Edit** |
| `cloudflare-r2` | `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` (or `AWS_*`) | R2 → **Object Read & Write** (+ bucket-create for `--create`) |
| `s3` | `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Object read & write (+ bucket-create for `--create`) |
| `tigris` | `TIGRIS_ACCESS_KEY_ID` / `TIGRIS_SECRET_ACCESS_KEY` (or `AWS_*`) | Tigris access key (`tid_`/`tsec_`) — Editor |
| `git` / `github-pages` | `GITHUB_TOKEN` / `GH_TOKEN` / `GIT_TOKEN` (HTTPS remotes only) | Repo contents → **write** (e.g. a GitHub fine-grained PAT or `GITHUB_TOKEN` in Actions). SSH remotes use the agent — no token. |

Non-secret settings (account id, bucket, project) may use `{env:VAR}`
[config interpolation](#configuration-and-interpolation). Each driver's README documents its
settings and interpolation:
[local](../internal/publish/local/README.md),
[cloudflare-pages](../internal/publish/cloudflare/README.md),
[cloudflare-r2](../internal/publish/r2/README.md),
[s3 / tigris](../internal/publish/s3/README.md),
[git / github-pages](../internal/publish/git/README.md),
[command](../internal/publish/command/README.md).

### Provisioning with `--create`

`colophon publish --env <name> --create` provisions destinations before deploying:
`cloudflare-pages` creates the Pages project; `cloudflare-r2` / `s3` / `tigris` create the
bucket. All are idempotent — an existing destination is left untouched.

For the object stores, `--create` also sets a **CORS policy** allowing cross-origin `GET`/`HEAD`
from any origin (via the S3 `PutBucketCors` API — the only way to configure CORS on R2, which has
no dashboard for it). This matters when assets are fetched with `fetch()` or imported as an ES
module rather than via an `<img>`/`<script>` tag — notably a **routed search index** (below): a
cross-origin `<img>` needs no CORS, but `fetch()`/`import()` do. The step is best-effort: if a
store doesn't support `PutBucketCors`, the publish warns and continues, and you set CORS manually.

### Generic S3 / MinIO / Backblaze / Wasabi

The `s3` driver is plain S3 (SigV4) with no control-plane code — point it at any S3-compatible
store with an `endpoint` and `region`:

```yaml
publishers:
  - id: s3
    driver: s3
    bucket: my-assets
    endpoint: "https://s3.us-east-1.amazonaws.com"   # or http://localhost:9000 (MinIO)
    region: us-east-1
    public_url: "https://my-assets.s3.us-east-1.amazonaws.com"
```

Credentials come from `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`. `publish --create` creates
the bucket (idempotent). `public_url` is how colophon learns the public base URL — there's no
control-plane lookup, so set it (a route with no resolvable URL stays inactive).

### Tigris (Fly.io)

[Tigris](https://www.tigrisdata.com/) is Fly.io's global object store, and it's **plain S3** —
colophon talks to it with the same client as any S3 store, so **no `flyctl` / Fly SDK /
control-plane token is involved.** The `tigris` driver is the `s3` driver with the endpoint
(`https://t3.storage.dev`) and region (`auto`) defaulted, so it needs only a bucket:

```yaml
publishers:
  - id: assets
    driver: tigris
    bucket: my-blog-assets
    public_url: "https://my-blog-assets.t3.storage.dev"   # the bucket's public/CDN domain
```

Credentials come from `TIGRIS_ACCESS_KEY_ID` / `TIGRIS_SECRET_ACCESS_KEY` (the `tid_`/`tsec_`
keys, falling back to `AWS_*`). `publish --create` creates the bucket. Two things are one-time
**dashboard** settings (Tigris has no data-plane API for them, which is what keeps publishing
SDK-free): **make the bucket public** to serve a site from it, and optionally **attach a custom
domain** (a CNAME to `<bucket>.t3.storage.dev`). Newer accounts serve public content from
`t3.tigrisfiles.io` — set `public_url` to whatever the bucket actually serves at.

> Provisioning credentials is separate: `flyctl storage create` issues a bucket + keys, but
> that's a one-time setup step, not part of `colophon publish`. colophon only consumes the keys
> from the environment.

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

## Run any CLI: the `command` publisher

When no built-in driver fits, the `command` driver runs an arbitrary CLI against the built tree
— so any deploy tool that takes a directory (surge, Netlify, Vercel, Wrangler, exe.dev, Azure
SWA, `rsync`, `scp`, `aws s3 sync`, a bespoke script) is a publisher with no driver of its own.

colophon materialises the (routed) tree to a temp directory and runs your command there, with
the directory as its working dir, the parent environment inherited (so the tool's own token var
flows through), `COLOPHON_*` context injected, and `CI=true`. A non-zero exit fails the publish.

```yaml
publishers:
  - id: surge
    driver: command
    command: ["surge", "{dir}", "myblog.surge.sh"]   # argv list — never a shell
    public_url: "https://myblog.surge.sh"            # SURGE_TOKEN comes from the env

  - id: rsync
    driver: command
    host: "deploy@example.com:/var/www/blog"          # any custom setting → {host}
    command: ["rsync", "-az", "--delete", "{dir}/", "{host}"]
    public_url: "https://www.example.com"
```

The command is an **argv list executed directly — never through a shell**, so there's no shell
injection surface. For a one-liner with pipes or `&&`, make the shell explicit:
`["sh", "-c", "aws s3 sync {dir} s3://bucket --cache-control max-age=3600"]`.

**Interpolation.** Every argument is interpolated with `{placeholder}` tokens drawn from your own
publisher settings plus colophon runtime values (which win on a clash): `{dir}` / `{output_dir}`
(the materialised tree, also the CWD), `{manifest}` (a JSON file classifying each path as
page/asset/feed/… with content-type and size, written *beside* the tree so it isn't published
unless you reference it), `{public_url}`, `{id}`, `{file_count}`, and any setting you declare
(`{host}`, `{domain}`, `{project}`, …). Unknown placeholders error, so a typo fails loudly.
Per-environment `overrides` vary any setting, so one `command` publisher can target staging vs
production. The same values are exposed as `COLOPHON_OUTPUT_DIR` / `COLOPHON_MANIFEST` /
`COLOPHON_PUBLIC_URL` / … env vars.

> **Secrets stay in the environment.** colophon never injects a token into the command line — the
> child inherits the environment and the target tool reads its own `$SURGE_TOKEN` / `$VERCEL_TOKEN`
> there, matching colophon's env-only rule and the deploy-CLI best practice of keeping credentials
> out of argv (where they'd leak into process listings and shell history). The command runs with
> your privileges from your own config — same trust as a Makefile — and is gated behind
> `--allow-publish` like every deploy.

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

## Redirects (aliases)

A post's `aliases:` ([Authoring → Redirects](content.md#redirects-aliases)) produce, at build
time, a meta-refresh stub per old URL, a root `_redirects` file, and a root `.nojekyll`. How that
becomes a redirect depends on the host:

| Host | Result |
|------|--------|
| Cloudflare Pages, Netlify, GitLab Pages | real **301** from `_redirects` |
| S3 static-website hosting | real **301** — the `s3` publisher sets the object redirect header on publish |
| Cloudflare R2, bare S3/MinIO, local | client-side **meta-refresh** stub (no in-bucket redirects) |
| GitHub Pages | client-side **meta-refresh** stub; the `.nojekyll` is what keeps `_search/` and the stubs from being stripped |

So redirects work everywhere; hosts that support server-side rules get a true 301, the rest fall
back to the (always-emitted) stub. `colophon doctor` warns about alias conflicts before you ship.

## On-site search

colophon builds a **static search index** at build time — a sharded, content-addressed BM25 index
a tiny browser reader queries client-side (no server, no service). Enable it per site with the
`search` stanza:

```yaml
sites:
  - id: main
    search:
      mode: lexical     # off (default) | lexical
      fuzzy: true       # opt-in typo tolerance (trigram + Levenshtein); roughly doubles the index
```

The string shorthand `search: lexical` still works (equivalent to `mode: lexical`, no fuzzy).

- **`mode`** — `lexical` turns search on; omitted/`off` leaves it out entirely (no index, no box).
- **`fuzzy`** — when on, a query token that finds no exact/**prefix** match falls back to
  typo-tolerant matching (so "wikilnk" finds "wikilinks"). It's opt-in because the trigram index
  it needs roughly doubles the index size — a cost a low-bandwidth search shouldn't pay unasked.

Results are **prefix-matched** by default ("wiki" → "wikilinks"), with query-aware highlighting
and an occurrence count. The index + reader ship only when search is on, under `_search/`; a theme
renders the box (the `press`, `press-gazette` and `press-broadsheet` themes include one). `colophon search "<query>"` queries
the same engine from the CLI (always fuzzy), in text or `--json`.

For large sites, the index can be **routed to an object store** to keep it off a Pages-style file
budget — see [Routing the search index](#routing-the-search-index) below (it also covers the CORS
that a cross-origin index needs, set automatically by `--create`).

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

### Routing the search index

The static search index (`_search/**`) can be routed to the object store too, to keep it off a
Pages-style file budget:

```yaml
routing:
  - match: "_search/**"
    publisher: r2
```

When routed, colophon points the browser reader at the store's URL automatically (the
`search_base` it emits follows the route). Because the reader loads the index with `fetch()` and
imports `search.js` as a module — neither CORS-exempt — the bucket must allow cross-origin `GET`;
`publish --create` sets that policy for you (see [Provisioning](#provisioning-with---create)).
Unrouted, the index stays on the same origin and no CORS is involved.
