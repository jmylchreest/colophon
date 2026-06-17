# cloudflare-r2 publisher

Uploads the (routed) build tree to an **S3-compatible object store** over HTTP with
AWS Signature Version 4 — implemented directly, no AWS SDK. Built for
[Cloudflare R2](https://developers.cloudflare.com/r2/), but works with Amazon S3 or
MinIO via an `endpoint` override.

It exists to keep large or numerous assets (images) off a Pages/Workers deployment's
file budget: paired with site **routing**, the build sends matching paths here and
rewrites their URLs to the store's public base, while the rest of the site deploys to
Pages.

## Credentials (env-only)

Like the Pages publisher, secrets never live in config — they come from the environment:

| What             | Source                                                        |
| ---------------- | ------------------------------------------------------------- |
| Access key id    | `R2_ACCESS_KEY_ID` env var (falls back to `AWS_ACCESS_KEY_ID`) |
| Secret key       | `R2_SECRET_ACCESS_KEY` env var (falls back to `AWS_SECRET_ACCESS_KEY`) |
| API token (opt.) | `CLOUDFLARE_API_TOKEN` env var — enables public-URL discovery and `--create`'s expose step (R2 only) |

For R2, create the keys at **dash.cloudflare.com → R2 → Manage R2 API Tokens →
Create API Token**.

### Token permissions

| Operation                          | R2 token permission |
| ---------------------------------- | ------------------- |
| Publish (upload, overwrite) assets | **Object Read & Write** |
| `publish --create` (make a bucket) | **Admin Read & Write** (bucket-level) |

- **Object Read & Write**, scoped to the target bucket, is enough for normal publishing.
  The publisher does a `HEAD` per file to skip unchanged objects (ETag = MD5) and `PUT`s
  the rest.
- Bucket creation via `--create` needs a token allowed to create buckets
  (**Admin Read & Write**). If you create the bucket in the dashboard once, an
  Object-scoped token is sufficient thereafter.
- For Amazon S3, the equivalent IAM actions are `s3:PutObject` + `s3:GetObject`
  (+ `s3:CreateBucket` and `s3:ListBucket` for `--create`).

> **One token for both.** `CLOUDFLARE_API_TOKEN` is shared with the Pages publisher and is
> used here for public-URL discovery and `--create`'s public-access step. If you publish to
> Pages *and* R2 in the same run, that token needs **both** Account · Cloudflare Pages · Edit
> **and** Account · Workers R2 Storage · Edit. A Pages-only token will fail the R2 calls with
> an authentication error (the bucket is still created, but public access isn't enabled).

## Securing the bucket for web delivery

R2's public access is **read-only by design**: a public bucket (via the managed `r2.dev`
domain or a custom domain) serves `GET <key>` only — **no public listing and no public
writes**. So the minimal, correct posture for web assets is simply "public read via a
domain", which is what `--create` sets up. To keep it tight:

- **Least-privilege keys** — scope the `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` to
  **Object Read & Write on this one bucket**. Writes never depend on public access.
- **Assets only** — the bucket should hold only routed assets. colophon enforces this: when
  routing can't resolve a public URL, the R2 target is **skipped** (not handed a full mirror
  of the site), so HTML/feeds never land in a public assets bucket by accident.
- **Prefer a custom domain** over `r2.dev` for production — it serves through your Cloudflare
  zone (CDN caching, TLS, WAF, the option to add response headers), whereas `r2.dev` is a
  rate-limited managed endpoint best used for testing.

## Required values

| What         | Where it comes from                                                  |
| ------------ | -------------------------------------------------------------------- |
| `bucket`     | config — the destination bucket                                      |
| `account_id` | config (or `CLOUDFLARE_ACCOUNT_ID`) — used to derive the R2 endpoint |
| `endpoint`   | config — set this instead of `account_id` for S3/MinIO              |
| `region`     | config — SigV4 *signing* region; defaults to `auto` (R2)            |
| `location`   | config — optional bucket-creation location hint (see below)         |
| `public_url` | config — the store's public base URL (used by routing, see below)   |

Two different hosts are in play:

- The **S3 API endpoint** is where colophon *uploads* — `https://<account_id>.r2.cloudflarestorage.com`,
  derived from `account_id` (or set `endpoint` directly). It is not browser-facing. For a
  **jurisdiction** bucket, set `endpoint` to the variant host —
  `https://<account_id>.eu.r2.cloudflarestorage.com` (EU) or `.fedramp.…` (FedRAMP); provider
  detection and discovery still apply.
- The **public URL** (`public_url` / `R2_PUBLIC_URL`) is where *browsers fetch* the assets,
  and what the build rewrites image URLs to. R2 buckets are private by default, so this
  exists only once you expose the bucket (below).

### Public URL: r2.dev or a custom domain

In **R2 → your bucket → Settings → Public access**, either:

- **`r2.dev` URL** — a managed `https://pub-<hash>.r2.dev` address. Quick, but rate-limited
  and dev-grade. `--create` **enables this automatically**, so a freshly created bucket is
  immediately reachable.
- **Custom domain** (recommended) — connect e.g. `assets.blog.i0.pm`. If that hostname is in
  a zone on your Cloudflare account, R2 **creates the DNS (CNAME) record and TLS for you** —
  no manual DNS entry.

The bucket name and the domain are independent: a bucket named `colophon-assets` can serve
at `assets.blog.i0.pm`.

### Automatic public-URL discovery

You usually don't need to set `public_url` at all. When it is empty **and** the endpoint is
a real R2 host **and** `CLOUDFLARE_API_TOKEN` is set, the publisher asks Cloudflare for the
bucket's public URL, in order:

1. a **connected custom domain** (the shortest, if several are enabled), else
2. the **managed `r2.dev` URL** (if enabled), else
3. empty — routing stays inert and assets remain co-located.

So the turnkey flow is: `publish --create` (creates the bucket and enables `r2.dev`) → images
serve from the discovered `r2.dev` URL. Later connect `assets.blog.i0.pm` in the dashboard,
and the next publish discovers and prefers it — no config change. Set `public_url` explicitly
only to override discovery (and it is **required** for generic S3/MinIO, which has none).

Discovery uses the same `CLOUDFLARE_API_TOKEN` as the Pages publisher; it needs **R2 read**
(Object Read & Write is fine). `--create`'s enable-public-access step needs **Admin Read &
Write**.

### `region` vs `location`

`region` is only the SigV4 **signing** region — `auto` for R2. The bucket's physical
location is a separate **creation-time hint** set via `location`, used only by `--create`:

- R2 jurisdiction hints: `wnam`, `enam`, `weur`, `eeur`, `apac`, `oc`.
- Generic S3: the region's `LocationConstraint` (e.g. `eu-west-1`).

Leave `location` unset to let the store auto-locate. It has no effect on an existing bucket.

## Config

```yaml
publishers:
  # Cloudflare R2
  - id: r2
    driver: cloudflare-r2
    bucket: "{env:R2_BUCKET:-my-assets}"
    account_id: "{env:CLOUDFLARE_ACCOUNT_ID}"
    public_url: "{env:R2_PUBLIC_URL:-}"     # e.g. https://assets.example.com

  # Generic S3 / MinIO (endpoint + region instead of account_id)
  - id: s3
    driver: cloudflare-r2
    bucket: my-assets
    endpoint: "https://s3.us-east-1.amazonaws.com"   # or http://localhost:9000 for MinIO
    region: us-east-1
    public_url: "https://my-assets.s3.us-east-1.amazonaws.com"
```

For a non-Cloudflare store, prefer the dedicated [`s3` / `tigris`](../s3/README.md) driver — same
S3 wire protocol, without R2's control-plane bits.

Every setting supports `{env:VAR}` / `{env:VAR:-default}`
[config interpolation](../../../docs/publishing.md#configuration-and-interpolation) (as the
`bucket` / `account_id` / `public_url` above show); only the SigV4 keys and `CLOUDFLARE_API_TOKEN`
come from the environment directly, never config.

## Routing assets here

A publisher only uploads; **routing** decides what goes to it. On the site, bind a path
glob to this publisher and give it the store's public base, then list the publisher in the
environment:

```yaml
sites:
  - id: main
    routing:
      - match: "**/assets/**"   # ** crosses slashes, * does not
        publisher: r2           # rewrite target inherited from the r2 publisher
        # base_url:             # optional override; omit to inherit public_url / discovery

environments:
  - name: production
    publish: [cf, r2]           # HTML to Pages, routed images to R2
```

A route's rewrite target is resolved as: its own `base_url`, else the target publisher's
`public_url`, else (on publish) the publisher's **discovered** URL. The route is **inactive
until a URL resolves _and_ its publisher is deploying** in the environment — so a local
build, a preview, or an env that doesn't list `r2` keeps the assets co-located and never
strips them. See [docs/publishing.md](../../../docs/publishing.md).

## Bucket identity (`.well-known/colophon.json`)

R2 has no native bucket description or tag field, so on publish the driver writes a small
provenance manifest to **`.well-known/colophon.json`** in the bucket — turning an otherwise
anonymous bucket into a self-describing one that links back to its blog:

```json
{
  "generator": "colophon",
  "site": "https://blog.i0.pm",
  "sitemap": "https://blog.i0.pm/sitemap.xml",
  "feeds": { "rss": "https://blog.i0.pm/rss.xml", "atom": "…", "json": "…" },
  "description": "colophon assets for https://blog.i0.pm",
  "bucket": "colophon-assets",
  "public_url": "https://assets.blog.i0.pm"
}
```

Set a human label with the optional `description` config field. It is a *private-use*
[well-known URI](https://www.rfc-editor.org/rfc/rfc8615) (the namespace signals discoverable
origin metadata) and is **public** when the bucket is — so it carries provenance only, never
secrets. The at-a-glance identifier in the R2 dashboard is still the **bucket name**, so name
buckets per blog too.

## Provisioning

`colophon publish --env <name> --create` creates the bucket first if it is missing
(idempotent). For Cloudflare R2 (region `auto`) the empty CreateBucket call is enough; a
generic S3 in a non-default region may need the bucket created out of band.
