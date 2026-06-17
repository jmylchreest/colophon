# Analytics & telemetry

colophon has two distinct, privacy-respecting telemetry surfaces with **different owners**:

| | **Site analytics** | **Tool telemetry** |
|---|---|---|
| Answers | "how is *my blog* doing?" | "how is *colophon* used?" |
| Owner | the **site owner** | the colophon **maintainer** |
| Surface | a web beacon in deployed pages | the binary reporting its own runs |
| Destination | the site owner's statsfactory | the maintainer's (release-baked) statsfactory |
| Config | `sites[].analytics` (per site) | `telemetry` (top level) |

Both are off unless configured, and a single master switch (`telemetry.enabled`) governs
everything.

## The master switch

```yaml
telemetry:
  enabled: true        # master switch over ALL telemetry — this and every site's analytics
```

`telemetry.enabled: false` disables the tool telemetry **and** every site's reader beacon. The
environment variable `COLOPHON_TELEMETRY=off` additionally force-disables the tool telemetry
(`off`, `false`, `0`, `no`).

## Site analytics (reader beacon)

Per-site, one block per provider — your data, your instance:

```yaml
sites:
  - id: main
    analytics:
      statsfactory:                       # cookieless, DNT-respecting
        server_url: "{env:STATSFACTORY_SERVER_URL:-}"
        app_key: "{env:STATSFACTORY_APP_KEY:-}"
      google_analytics:                   # GA4 — sets cookies, brings its own consent duties
        measurement_id: "{env:GA_MEASUREMENT_ID:-}"
```

Each provider is independent and inert until configured. The **statsfactory** beacon is a
~2 KB dependency-free `analytics.js` written once to the site root and referenced by every
page. It:

- sends `page_view` on load and `page_engagement` (active milliseconds, as the metric value)
  on hide/unload;
- is **cookieless** (session id in `sessionStorage`, per tab);
- **honours Do-Not-Track / Global Privacy Control** — sends nothing when either is set.

Its public per-page dimensions are `post.slug`, `post.type`, `post.author`, `post.tags`, plus
`page.path` and `referrer`. The statsfactory ingest key is a **public `sf_live_` key**, safe to
embed in pages. The **hidden persona is never sent to the beacon**.

**Google Analytics** (GA4) ships its own loader asset, `analytics-ga.js`, which injects
Google's `gtag.js`. Each provider's asset is written to the site root **only when that provider
is enabled** — `analytics.js` for statsfactory, `analytics-ga.js` for GA, both if both, nothing
if neither.

Both providers are wired into the **default** and **minimal (text)** themes; a custom theme
opts in by rendering `{{ analytics_head|safe }}` before `</body>`.

> Google Analytics sets cookies and carries consent obligations the cookieless beacon does
> not — enable it only if that fits your privacy posture.

### Injecting site credentials

Values usually come from `{env:VAR}` placeholders. colophon loads two dot-env files from the
project root before interpolation and **never overrides a variable already set in the real
environment**:

```
real environment (e.g. CI secrets)  >  .env (local, gitignored)  >  .env.defaults (committed)
```

So: commit your statsfactory endpoint + public key in `.env.defaults`, override per-machine in
a local `.env`, and override in CI via repository Variables/Secrets.

**GitHub Actions** — `colophon init` scaffolds `.github/workflows/deploy.yml`. Set under
*Settings → Secrets and variables → Actions*:

- **Variables** (public): `STATSFACTORY_SERVER_URL`, `STATSFACTORY_APP_KEY` — the ingest key is
  public, so a Variable (not a Secret) is right.
- **Secrets** (private): deploy credentials — `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ACCOUNT_ID`,
  `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`.

## Tool telemetry (colophon's own usage)

`colophon build` and `colophon publish` report colophon's *own* operation — never your content
— to the maintainer, so usage is understood. It is anonymous (a `distinct_id` that is a SHA-256
hash cached at `.colophon/telemetry.id`; the raw value is never stored or sent), and
fire-and-forget — it never blocks or fails a command, and `colophon serve` previews emit
nothing.

Credentials default to values **baked into the binary at release**, so a released colophon
reports by default (opt-out); a source/dev build has no baked creds and reports nothing. To
build a release with telemetry:

```sh
go build -ldflags "\
  -X github.com/jmylchreest/colophon/internal/telemetry.DefaultServerURL=https://stats.example.com \
  -X github.com/jmylchreest/colophon/internal/telemetry.DefaultAppKey=sf_live_xxxxxxxx" \
  ./cmd/colophon
```

A project may override the destination (e.g. to self-host the maintainer role) under
`telemetry.statsfactory`.

## Event model

statsfactory dimensions are arbitrary and defined at ingest time, so these compose into pivot
and breakdown views.

| Surface | Event | Value | Key dimensions | Answers |
|---------|-------|-------|----------------|---------|
| Site | `page_view` | — | `post.slug`, `post.type`, `post.tags`, `post.author` | most popular posts |
| Site | `page_engagement` | active ms | `post.slug` | engagement time per post |
| Tool | `build` | page count | `theme`, `env` | builds over time |
| Tool | `source_indexed` | doc count | `source.type`, `source.id`, `env` | document count × source type |
| Tool | `publish` | uploaded | `publisher.type`, `publisher.id`, `status`, `env` | published docs/executions × publisher type |

## Opting out — summary

- **Everything:** `telemetry.enabled: false`.
- **Tool telemetry only:** `COLOPHON_TELEMETRY=off`, or `telemetry.statsfactory.enabled: false`.
- **A site provider:** omit it, or set its `enabled: false`.
- **Readers** opt out of the beacon automatically via Do-Not-Track / Global Privacy Control.
