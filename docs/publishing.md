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

### Secrets and permissions

Deploy credentials are **never** read from config — they come from the environment, so they
never pass through the agent or the YAML:

| Publisher | Secret env vars | Token permission |
|-----------|-----------------|------------------|
| `cloudflare-pages` | `CLOUDFLARE_API_TOKEN` | Account → Cloudflare Pages → **Edit** |
| `cloudflare-r2` | `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` (or `AWS_*`) | R2 → **Object Read & Write** (+ bucket-create for `--create`) |

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
