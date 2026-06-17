# s3 / tigris publisher

Uploads the (routed) build tree to **any S3-compatible object store** over HTTP with AWS
Signature Version 4 — implemented directly, no AWS SDK and no provider SDK. Works with
[Tigris](https://www.tigrisdata.com/) (Fly.io's global object store), MinIO, Backblaze B2,
Wasabi, Amazon S3, or anything that speaks S3.

It shares the S3 wire client (`internal/publish/s3common`) with the `cloudflare-r2` driver but
carries **no control-plane code** — bucket creation and uploads are pure data-plane calls. Use
it to keep large or numerous assets (images) off a Pages/Workers deployment's file budget,
paired with site **routing**.

## Drivers

| Driver | Defaults | Use for |
| ------ | -------- | ------- |
| `s3` | none — `endpoint` required, `region` `us-east-1` | Amazon S3, MinIO, B2, Wasabi, self-hosted. |
| `tigris` | `endpoint` `https://t3.storage.dev`, `region` `auto` | Tigris on Fly.io — needs only a `bucket`. |

## Tigris without a Fly SDK

Tigris is plain S3, so colophon talks to it with the same SigV4 client as every other store —
**no `flyctl`, no Fly SDK, no control-plane token.** What the data plane covers:

- **Upload / overwrite / delete** objects — `PutObject` / `DeleteObject`.
- **Create the bucket** (`publish --create`) — `CreateBucket`.
- **Incremental diff** — `ListObjectsV2` + ETag/MD5 compare, so unchanged objects skip.

What it does **not** cover (one-time bucket settings, done in the Tigris dashboard):

- **Making the bucket public** — required to serve a site directly from Tigris.
- **Attaching a custom domain** — a CNAME to `<bucket>.t3.storage.dev`.

So provision and publish stay SDK-free; you enable public access once in the dashboard and set
`public_url` to whatever the bucket serves at.

## Credentials (env-only)

Secrets never live in config — they come from the environment:

| Driver | Access key id | Secret key |
| ------ | ------------- | ---------- |
| `s3` | `AWS_ACCESS_KEY_ID` | `AWS_SECRET_ACCESS_KEY` |
| `tigris` | `TIGRIS_ACCESS_KEY_ID` (falls back to `AWS_ACCESS_KEY_ID`) | `TIGRIS_SECRET_ACCESS_KEY` (falls back to `AWS_SECRET_ACCESS_KEY`) |

Tigris keys (created in the dashboard, **Access Keys → Create Access Key**) are prefixed
`tid_` / `tsec_`.

## Config

```yaml
publishers:
  - id: assets
    driver: tigris
    bucket: my-blog-assets
    public_url: "https://my-blog-assets.t3.storage.dev"  # the bucket's public/CDN domain

  - id: minio
    driver: s3
    bucket: my-bucket
    endpoint: "http://localhost:9000"
    region: us-east-1
    public_url: "http://localhost:9000/my-bucket"
```

`public_url` is the only way colophon learns the public base URL for this store: unlike
`cloudflare-r2`, there's no control-plane lookup to discover it, because a public S3/Tigris
domain is account-dependent. Set it to the bucket's serving domain (or a custom domain); a
route that can't resolve a URL stays inactive and its files remain co-located. See
[../../../docs/publishing.md](../../../docs/publishing.md) for routing.

## Settings

| Setting | Required | Purpose |
| ------- | -------- | ------- |
| `bucket` | yes | Target bucket (3–63 chars, S3 naming rules). |
| `endpoint` | `s3` only | S3 endpoint URL. Defaulted for `tigris`. |
| `region` | no | SigV4 region; `us-east-1` (`s3`) / `auto` (`tigris`). |
| `public_url` | for serving | Public base URL of the bucket. |
| `location` | no | `CreateBucket` location/`LocationConstraint` hint. |
| `description` | no | Recorded in the provenance manifest. |
| `delete_orphaned` | no | Delete objects no longer in the build (default `true`). |
