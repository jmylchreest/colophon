# Analytics

colophon has optional, privacy-respecting analytics powered by
[statsfactory](https://github.com/jmylchreest/statsfactory). It is **inert until keyed** — an
unconfigured project ships nothing — and splits into two surfaces:

- a **reader-facing web beacon** embedded in pages (page views + engagement time), and
- the **binary's own build/publish events** (document counts by source type, deploys by
  publisher type).

Both feed the same statsfactory app, as different event types.

## Configuration

Analytics is a per-site block in `colophon.yaml`:

```yaml
sites:
  - id: main
    # …
    analytics:
      provider: statsfactory
      server_url: "{env:STATSFACTORY_SERVER_URL:-}"
      app_key: "{env:STATSFACTORY_APP_KEY:-}"
      # enabled: true   # master switch (default true when keyed)
      # web: true       # the reader-facing beacon
      # server: true    # the binary's build/publish events
```

It activates only when both `server_url` and `app_key` resolve. The statsfactory ingest key
is a **public `sf_live_` key**, explicitly safe to embed in page HTML — it is not a secret.

### Injecting the credentials

Values usually come from `{env:VAR}` placeholders. colophon loads two dot-env files from the
project root before interpolation, and **never overrides a variable already set in the real
environment**. The precedence is:

```
real environment (e.g. CI secrets)  >  .env (local, gitignored)  >  .env.defaults (committed)
```

So the common setup is: commit your statsfactory endpoint + public key as defaults in
`.env.defaults`, override per-machine in a local `.env`, and override in CI by setting
`STATSFACTORY_SERVER_URL` / `STATSFACTORY_APP_KEY` as repository secrets/variables.

```sh
# .env.defaults  (committed; PUBLIC values only — never deploy secrets)
STATSFACTORY_SERVER_URL=https://stats.example.com
STATSFACTORY_APP_KEY=sf_live_xxxxxxxxxxxxxxxx
```

### GitHub Actions

`colophon init` scaffolds `.github/workflows/deploy.yml`, which builds (and, once a cloud
publisher is configured, publishes) the site with the credentials sourced from the repository's
Actions config. Set them under **Settings → Secrets and variables → Actions**:

- **Variables** (public): `STATSFACTORY_SERVER_URL`, `STATSFACTORY_APP_KEY` — the ingest key
  is a public `sf_live_` key, so a Variable (not a Secret) is appropriate.
- **Secrets** (private): your deploy credentials — `CLOUDFLARE_API_TOKEN`,
  `CLOUDFLARE_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`.

These override `.env.defaults` at build time (real environment wins), so the same repo builds
with the right analytics endpoint locally and in CI.

## The web beacon

When `web` analytics is active, the build writes a single ~2 KB dependency-free
`analytics.js` to the site root and references it from every page. It:

- sends a `page_view` on load and a `page_engagement` event (active milliseconds, as the
  metric value) when the page is hidden or unloaded;
- is **cookieless** — the session id lives in `sessionStorage`, scoped to the tab;
- **honours Do-Not-Track and Global Privacy Control** — it sends nothing when either is set.

It is wired into the **default** and **minimal (text)** themes. A custom theme opts in by
rendering `{{ analytics_head|safe }}` before `</body>`.

Per-page dimensions carried on the beacon: `post.slug`, `post.type`, `post.author`,
`post.tags`, plus `page.path` and `referrer`. The **hidden persona is never sent to the web
beacon** — it is server-side only (see below).

## Server-side events

When `server` analytics is active, `colophon build` and `colophon publish` emit events with a
stable, anonymous install id (a SHA-256 hash cached at `.colophon/telemetry.id`; the raw value
is never stored or sent). Delivery is fire-and-forget — it never blocks or fails a command, and
`colophon serve` previews emit nothing.

| Event | Value | Key dimensions | Answers |
|-------|-------|----------------|---------|
| `page_view` (web) | — | `post.slug`, `post.type`, `post.tags` | most popular posts |
| `page_engagement` (web) | active ms | `post.slug` | engagement time per post |
| `build` | page count | `theme`, `env` | builds over time |
| `source_indexed` | doc count | `source.type`, `source.id`, `env` | document count × source type |
| `content_persona` | post count | `persona`, `env` | output per writing voice |
| `publish` | uploaded | `publisher.type`, `publisher.id`, `status`, `env` | published docs/executions × publisher type |

statsfactory dimensions are arbitrary and defined at ingest time, so these compose into the
pivot/breakdown views in its dashboard.

## Opting out

- Leave the `analytics` block unset (or `provider:` empty) — nothing is emitted.
- Set `enabled: false` (or `web: false` / `server: false`) to disable a surface.
- Set `COLOPHON_TELEMETRY=off` in the environment to force the server-side events off even when
  keyed (`off`, `false`, `0`, `no` all work).
- Readers' browsers opt out automatically via Do-Not-Track / Global Privacy Control.
