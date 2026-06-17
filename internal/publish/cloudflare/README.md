# cloudflare-pages publisher

Deploys the built site to a [Cloudflare Pages](https://developers.cloudflare.com/pages/)
project using the **direct-upload** API — the same protocol `wrangler pages deploy`
uses, implemented directly over HTTP. No `wrangler`, no Node, no Git connection.

## API token permissions

Create a token at **dash.cloudflare.com → My Profile → API Tokens → Create Token →
Create Custom Token**, with exactly one permission:

| Type    | Resource             | Access |
| ------- | -------------------- | ------ |
| Account | **Cloudflare Pages** | Edit   |

- Scope **Account Resources** to the account that owns the Pages project (or *All
  accounts*).
- No Zone or User permissions are required.
- `Cloudflare Pages: Edit` covers the whole deploy flow: minting an upload token,
  uploading assets, and creating the deployment. `Read` is **not** sufficient — the
  deployment call needs write access.

This single permission is broad enough to deploy to **any** Pages project in the
account. For tighter blast radius, use a token scoped to a dedicated deploy account.

## Required values

| What         | Where it comes from                                              |
| ------------ | --------------------------------------------------------------- |
| API token    | `CLOUDFLARE_API_TOKEN` env var **only** (never read from config) |
| Account ID   | `account_id` in config, or `CLOUDFLARE_ACCOUNT_ID` env var       |
| Project name | `project` in config (the Pages project must already exist)       |

The token is deliberately env-only so deploy secrets never live in the project repo
or pass through the agent layer. The account ID is on the right-hand sidebar of any
account page in the dashboard.

## Config

The publisher is pure mechanism (the driver + project + account). Which **environment**
deploys to it — and on which branch — is decided in `environments`:

```yaml
publishers:
  - id: cf
    driver: cloudflare-pages
    project: my-blog                       # Pages project name
    account_id: "{env:CLOUDFLARE_ACCOUNT_ID}"   # or omit and set the env var

environments:
  - name: production
    publish: [cf]
    allow_publish: false                   # gate; see below
    overrides:
      cf: { branch: main }                 # → Cloudflare Production environment

  - name: preview
    publish: [cf]
    include_drafts: true
    overrides:
      cf: { branch: preview }              # → Cloudflare Preview environment
```

Then:

```sh
export CLOUDFLARE_API_TOKEN=...        # the token created above
export CLOUDFLARE_ACCOUNT_ID=...       # if not set in config
colophon publish --env production --allow-publish
```

Settings (`project`, `account_id`, the `branch` override) support `{env:VAR}` /
`{env:VAR:-default}`
[config interpolation](../../../docs/publishing.md#configuration-and-interpolation), as the
`account_id` above shows. The API token only ever comes from the environment, never config.

### `allow_publish` (deploy gate)

`allow_publish` is an **environment** setting that defaults to **true**, so an
environment deploys on a plain `colophon publish --env <name>`. Set it to **false** to
gate the environment — then `publish` skips it unless `--allow-publish` is passed. Use
it as a safety latch on production so a deploy is always deliberate.

### `--create` (provision the project)

Pass `colophon publish --create` to create the Pages project (Direct Upload) if it
doesn't exist yet, instead of creating it manually first (see below). It is
idempotent: if the project already exists, `--create` does nothing.

### `prune` (deployment retention)

Cloudflare keeps every deployment forever and never auto-cleans them; past ~100 you
can't even delete the project without pruning first. After each deploy colophon prunes
old deployments **for that branch**. `prune` accepts three forms (default: keep `1`):

| Value | Meaning |
| ----- | ------- |
| a count, e.g. `1`, `5` | keep the newest N deployments (N **≥ 1**) |
| a duration, e.g. `3w`, `21d`, `72h`, `2 weeks` | keep deployments newer than the window |
| `never` / `off` | keep everything (no pruning) |

`prune: 0` is rejected as ambiguous — Cloudflare never deletes the live deployment, so
the floor is `1` (keep only the newest); use `never` to keep everything.

```yaml
publishers:
  - id: cf
    driver: cloudflare-pages
    project: my-blog
    prune: 1              # keep only the newest deployment per branch (default)

environments:
  - name: preview
    publish: [cf]
    overrides:
      cf: { prune: 3w }   # on preview, keep the last 3 weeks of deployments
```

A count can never be below 1 — you always keep at least the live deployment — so `0`
isn't "keep none", it's the separate "keep all" case. Duration mode likewise always
keeps the newest deployment even if it's older than the window (it holds the branch
alias). Pruning is **branch-scoped**, so pruning `preview` never touches `production`,
and it's best-effort: a prune failure warns but does not fail the publish.

## Environments → Cloudflare environments

`branch` (set per environment via `overrides`) controls which Cloudflare environment a
deploy lands in. To update the project's **main domain** (`<project>.pages.dev` and any
custom domains), `branch` must equal the project's **production branch** (set when the
project was created, usually `main`) — that lands in Cloudflare's **Production**
environment. Any other branch lands in the **Preview** environment with its own URL.
colophon hardcodes nothing: you name the environment and pick the branch; Cloudflare
derives its own environment from the branch. The deploy URL is printed after `publish`.

## Creating the project

The project must exist before the first deploy, and it must be a **Direct Upload**
project — do **not** create a framework/Git-connected one, since colophon is the
generator and owns the build output.

### Via the API (no dashboard, no wrangler)

With `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` exported, create the project
with the same token used to deploy. The body carries no `build_config`/Git settings,
which is what makes it a Direct Upload project:

```sh
curl -s -X POST \
  "https://api.cloudflare.com/client/v4/accounts/$CLOUDFLARE_ACCOUNT_ID/pages/projects" \
  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${CF_PAGES_PROJECT:-colophon-e2e}\",\"production_branch\":\"main\"}" \
  | jq '.success, .errors, .result.subdomain'
```

- The body is double-quoted so the shell expands `${CF_PAGES_PROJECT:-colophon-e2e}`;
  `export` the variable so `curl` sees it.
- The project **name** must match what colophon publishes to (`project:` in config /
  `CF_PAGES_PROJECT`), and `production_branch` must match the publisher's `branch`
  (default `main`) for `publish` to update the main `*.pages.dev` domain.
- Names are lowercase alphanumeric plus hyphens. Success prints `true`, `[]`, and the
  assigned subdomain.

### Via the dashboard

**Workers & Pages → Create → Pages → Upload assets**, name it, create. Then deploy
with `colophon publish --allow-publish`.

## Protocol

1. `GET /accounts/{account_id}/pages/projects/{project}/upload-token` → short-lived JWT.
2. Hash every output file: `blake3(base64(content) + extension)`, hex, first 32 chars.
3. `POST /pages/assets/check-missing` (JWT) → hashes the server still needs.
4. `POST /pages/assets/upload` (JWT) → missing files, base64-encoded, in batches.
5. `POST /pages/assets/upsert-hashes` (JWT) → keep the full hash set alive.
6. `POST /accounts/{account_id}/pages/projects/{project}/deployments` (token,
   multipart) with a `manifest` mapping `/path → hash` for the whole tree.

Steps 1, 5, 6 authenticate with the account API token; 3 and 4 use the JWT from step 1.
