# Design: webmention

> Status: **partly built** · relates to PLAN §10 (Federation & IndieWeb), §6 (build pipeline).
> Builds on the microformats2 markup now shipped in the themes. **Shipped:** the
> `<link rel="webmention">` discovery tag (`internal/build/webmention.go`); **`colophon webmention
> send`** — outbound-link scan + endpoint discovery + POST + sent-cache (`internal/webmention/`,
> `internal/cli/webmention.go`). **Not yet built:** `fetch`/`publish`, the `_mentions/` assets,
> display modes, and moderation — the rest of this design.

Goal: let a colophon site participate in [Webmention](https://www.w3.org/TR/webmention/) — the
W3C standard for "site A notified site B that it linked to / replied to / liked B's post" —
while staying **fully static** (no server, no database) and keeping received data in **clean,
separate assets** rather than mixed into the author's content. Two halves: **sending** (we tell
others we linked them) and **receiving + display** (others' mentions appear under our posts).

It deliberately reuses five patterns colophon already has, rather than inventing new ones — see
[Fit with existing design](#fit-with-existing-design).

## Why a separate command (not part of build)

The two operations run at different points in the lifecycle, so neither belongs inside `build`:

| Operation | When | Why |
|-----------|------|-----|
| **`colophon webmention send`** | **after** `publish` | the source URL must be live so the receiver can fetch it back and verify the link |
| **`colophon webmention fetch`** | **before** `build`, or standalone/scheduled | pulls received mentions into the local cache so a bake build can read them |
| **`colophon webmention publish`** | **on its own cadence** (e.g. cron) | `fetch` + push **only** the `_mentions/` prefix to the object store, decoupled from the site build — see [Separate publish pipeline](#separate-publish-pipeline) |

So `webmention` is a command group alongside `build`/`publish`. A typical CI flow:

```
colophon webmention fetch     # refresh received mentions → committed cache
colophon build                # emit _mentions/ assets (+ bake for no-JS themes)
colophon publish --env production --allow-publish
colophon webmention send      # now that the new post is live, notify the sites it links to
```

`fetch` is also fine to run on a schedule (cron) to keep mentions fresh without a content change.

## Receiving: data as a separate asset

We don't run a server, so we can't accept inbound POSTs. Mentions are received by a **hosted
receiver** — [webmention.io](https://webmention.io) (free; the page advertises it via
`<link rel="webmention">`). `fetch` reads them back through its JSON API (token in `{env:…}`,
never config) and normalises them into **one JSON file per post**, served as its own asset
namespace — exactly mirroring the `_search/` index:

```
_mentions/<post-path>.json     e.g. _mentions/posts/hello-world.json
```

Each file is a small, normalised list (not the raw webmention.io payload):

```json
{
  "target": "https://blog.example.com/posts/hello-world/",
  "updated": "2026-06-22T10:00:00Z",
  "mentions": [
    { "type": "like",   "author": {"name": "Ada", "url": "https://ada.example", "photo": "https://…/ada.jpg"},
      "url": "https://ada.example/likes/1", "published": "2026-06-21T09:00:00Z" },
    { "type": "reply",  "author": {"name": "Bob", "url": "https://bob.example", "photo": "…"},
      "url": "https://bob.example/notes/2", "published": "…", "content": "Nice post!" }
  ]
}
```

- `type` is normalised to `like | repost | reply | mention` (from the sender's mf2 / wm-property).
- author fields come from the sender's `h-card` — which is *why* mf2 shipped first.
- The build emits these to the output tree under `_mentions/`, **routed to R2** like `_search/`,
  and they get **CORS for free** from `publish --create` (same `GET/HEAD` rule the JS search
  index already relies on for cross-origin fetch).
- `fetch` writes a local cache (`.colophon/cache/webmentions/`). This cache is **not** treated
  like the generated-image cache — see below.

One JSON-per-post (no sharded manifest like search) because a post page already knows its own key.

### Not reproducible — and that's fine

Unlike the gen-image cache (a deterministic function of the prompt, committed for reproducible
builds), received mentions are **external, time-varying state** — other people's posts. The local
`.colophon/cache/webmentions/` is therefore a **derived export, not preserved state**: webmention.io
is the source of truth and `fetch` **fully regenerates the export from it every run**.

`fetch` queries the whole domain — `GET /api/mentions.jf2?domain=<domain>&token={env:…}` returns
the complete current set (JF2, newest-first), paged via `per-page`+`page` — then buckets by
`target` and writes one JSON per post, replacing whatever was there. Properties of that:

1. **Stateless / idempotent.** A fresh CI runner with an empty cache just rebuilds it from the API
   (needs only network + token). "Empty" means "rebuild it," never "lost data."
2. **Self-reconciling.** Because each run takes the full current set, deleted/edited mentions
   correct themselves — no stale entries to prune. (`since`/`since_id` enable delta fetches later
   if volume ever warrants; full-regenerate is the simple default.)
3. **Graceful when empty.** Until `fetch` runs, a missing `_mentions/<post>.json` renders nothing
   and never fails the build — mentions are always additive chrome.
4. **The JS path removes the build dependency entirely.** A JS-rendering theme ships only a
   placeholder; the browser fetches the separately-published `_mentions/` from R2 (see
   [Separate publish pipeline](#separate-publish-pipeline)), refreshed out-of-band.

So committing the JSON to the repo is *optional* (it only helps a baked build show mentions with no
network, at the cost of churny commits); the default is "regenerated by `fetch` in CI / on a
schedule," not "committed."

### Deletions & pruning (full-replace, not merge)

"Self-reconciling" only holds if regenerate is a **full replace of the `_mentions/` namespace**,
not a per-post upsert — otherwise a mention webmention.io has deleted lingers in a stale file.
Concretely:

- A deleted mention isn't in the next `fetch` result, so the rewritten per-post file omits it.
- **Zero-mentions edge:** when a post loses *all* its mentions there's nothing to write for it.
  `fetch` must therefore **clear the namespace and rewrite wholesale** so that post ends up with
  **no file** (not a leftover from a prior run), and **`publish` must prune** objects no longer
  present from the store — which the existing incremental publisher already does (the `Pruner`:
  "only changed files upload, orphans are pruned"). The stale object is deleted from R2.
- The JS path then **404s → renders nothing**; a baked rebuild finds no file → renders nothing.
- This is **eventually consistent**: a deletion persists until the next `fetch`+`publish` (JS) or
  rebuild (bake). The scheduled `webmention publish` keeps that window short without a site build.

Implication for `publish`: it must apply orphan pruning **scoped to the `_mentions/` prefix** so a
mentions refresh never deletes content objects (and vice versa).

## Display: a per-site `mode` (`live` / `asset` / `disabled`)

How responses reach the page is a **per-site setting** (`federation.indieweb.webmention.display.mode`).
There are exactly **three** values — there is *no* separate "baked" mode (baking is a theme choice
*within* `asset`, see below). The mode decides **where the browser fetches from** and **whether the
engine ships anything at all**; both active modes use the same `mentions.js` + the same
[moderation pipeline](#moderation-a-distilled-committed-blocklist).

| Mode | What ships per page | Browser fetches from | Build-time data? | Freshness | Privacy |
|------|---------------------|----------------------|------------------|-----------|---------|
| **`live`** | placeholder + JS | the **receiver directly** | **no** | next page load | visitor hits the receiver |
| **`asset`** | placeholder + JS (+ optional bake) | **our `_mentions/` on R2** | **yes** (synced list) | the refresh cron | self-hosted |
| **`disabled`** | **nothing** (zero counts) | — | — | — | — |

**Per-page toggle.** When the mode is active (`live`/`asset`), webmentions are **on by default for
every post**, opt-out per page via frontmatter (`webmentions: false`). In `disabled` the page setting
is ignored — nothing is embedded or shipped anywhere, `has_mentions` is false and `mentions` is empty.

### `live` — JS straight to the receiver (no fetch/publish/build)

The engine ships, on each enabled page, an **engine-provided placeholder** + the shared `mentions.js`;
the browser calls the receiver's read API directly (webmention.io exposes a public, CORS-enabled,
token-free read endpoint). **A new mention shows on the next page load** — no `fetch`, no `publish`,
no rebuild, no asset to host. Lowest-infra, most realtime.

Crucially, in `live` the engine has **no build-time data** — so `has_mentions`/count are **not known
at build**. The placeholder ships whenever the page is enabled (not gated on a count), and JS fills
in the count (and hides the section if it resolves to zero). So themes **cannot bake** in `live` mode.

Because each reader speaks its own API, the **JS is parameterised by the reader provider**: the
provider declares a small **client descriptor** (endpoint URL template, query params, and the
mapping from its response → our normalised shape). One shared `mentions.js` consumes the descriptor
and handles any JF2-shaped provider (webmention.io and compatibles); a provider with an exotic API
may ship its own client module. So *"the fetch JS is provided by the specific driver"* — yes, via
the descriptor, without forking the renderer per provider.

Moderation still applies: the **distilled glob blocklist is shipped to the client** (it's
spam-hiding, not a secret) and filtered in-browser. Semantic rules can't run client-side (no
embeddings in the browser), so a site that needs semantic moderation should use `asset`. Trade-offs
to accept: every visitor's browser hits a third party (their uptime/rate-limits become yours; a
privacy leak), and no-JS/RSS readers see nothing.

### `asset` — JS against our published `_mentions/`, with optional build-time bake

The default is the same placeholder + `mentions.js`, but pointed at *our* server-curated
`_mentions/<key>.json` on R2. Freshness comes from a scheduled **`webmention publish`** — `fetch` +
push of **only** the `_mentions/` prefix, **no site build** (see
[Separate publish pipeline](#separate-publish-pipeline)) — near-realtime to whatever cron cadence you
pick, with full server-side moderation (glob + semantic), self-hosting, and provider-neutrality.

**Because the synced mentions list already exists at build time in this mode, the engine also exposes
it to the template** (`mentions`/`mentions_html`/`has_mentions`) so a theme that *wants* to
**hard-code / bake** them into the HTML can — instead of, or alongside, the JS placeholder. This is
plausible precisely because we already have the built/synced list; it's a **theme capability, not a
mode**. Baked output is then as-of-last-build (the JS placeholder is what stays cron-fresh), and it's
the no-JS escape hatch for text-first themes like `minimal`.

### Template surface

Rendering remains a **template responsibility** — the engine only **exposes the data**, exactly as
`attachments`/`attachments_html` do:

| Var | `live` | `asset` | `disabled` |
|-----|--------|---------|-----------|
| `has_mentions` | unknown at build → `false` (JS fills count) | accurate (from synced list) | `false` |
| `mentions` | empty (no build-time data) | structured `[{type, author{name,url,photo}, url, content, published}]` — bake your own | empty |
| `mentions_html` | empty | engine-rendered drop-in block (empty when none) | empty |
| `mentions_src` | provider client-descriptor endpoint | our `_mentions/<key>.json` | unset |
| `mentions_enabled` | `true` unless page opted out | `true` unless page opted out | `false` |

Bundled themes drop the placeholder + `mentions.js` for the JS modes; `minimal` (no-JS) uses
`asset` + the baked `mentions_html`. The mode is overridable per site.

## Audio / TTS: mentions are never spoken

A real hazard for the bake path — handled by an invariant. The TTS reading is generated from the
**post's markdown body** (`registerTTS(slug, html, …)`, where `html` is the converted content)
*before* the theme runs. Mentions are **theme chrome** rendered as a sibling of `{{ content }}`
— like the author card, downloads and tags, none of which are spoken. So:

> **Invariant:** mentions render *outside* the content / `e-content` body the TTS extractor reads.
> Themes must keep the mentions block a sibling of the content element, never inside it.

This means baked mentions are excluded from audio for free (the TTS source predates theming); the
guard just stops a theme from accidentally nesting them into the content element.

## Sending

`colophon webmention send` works off the **built output** (or the deployed URLs):

1. Scan each published post's HTML for outbound links (`http(s)://` to other origins).
2. For each target, discover its endpoint — an HTTP `Link: rel="webmention"` header, else a
   `<link rel="webmention">` / `<a rel="webmention">` in the body.
3. POST `source=<post URL>&target=<their URL>` to the endpoint.
4. Maintain a per-post **sent-cache** of the link set last sent. On re-run, send only **new**
   targets — and **re-send to dropped targets** so that if you edit a post to remove a link, the
   receiver re-checks, sees the link gone, and removes its mention (the spec's update/delete path).

The sent-cache is what makes "we changed the context after sending" correct rather than silent.

## The mutability question (and how it plays with existing choices)

Three cases, by what changes:

- **Post *content* changes** → **fine.** Received mentions target the *URL*, not the prose;
  edits are normal. Senders replied to the address.
- **Post *URL/slug* changes** → **the real gotcha.** webmention.io holds mentions under the old
  URL; the new page finds none; inbound links rot. Mitigations: (1) keep slugs stable
  (link-rot discipline); (2) once the backlog **`aliases:` / redirects** feature lands, `fetch`
  queries the old keys too and the old URL 302s to the new; (3) `doctor` warns when a post that
  has cached mentions changes slug. **This makes `aliases:` a soft prerequisite for robust
  webmention** and is the main cross-feature dependency.
- **A sender deletes/edits a mention, or we drop a sent link** → reconciled by re-running:
  `fetch` always returns the current set (deletions vanish); `send`'s sent-cache re-pings dropped
  targets. JS display is always-current; baked display reconciles on the next fetch+build.

**Spam/moderation:** inbound mentions can be junk — handled by a declarative, committed blocklist
plus an optional moderation skill; see [Moderation](#moderation-a-distilled-committed-blocklist).

## Moderation: a distilled, committed blocklist

Inbound mentions are third-party content, so the author needs a way to drop spam/abuse — and it has
to **survive `fetch`'s full regenerate** (editing the generated `_mentions/` JSON is pointless; the
next fetch overwrites it). So moderation is **declarative and committed**, applied as a filter step
over the normalised list, and reused by every display mode.

**The blocklist is rules over normalised mention attributes, glob-matched.** Matchable fields:
`domain`, `url`, `author.name`, `author.url`, `content`, `type`. A bare string is shorthand for
`domain`/`author.url`; the structured form targets a field:

```yaml
# .colophon/webmention-block.yml  (committed)
- "*.spam.example"            # shorthand: domain/author.url glob
- author.url: "https://troll.example/*"
- content: "*free crypto*"
- domain: "*.cn.example"
```

- **One filter pipeline, two execution sites.** Applied **server-side in `fetch`** (full power) for
  `asset`/`baked`; for `live` the **distilled blocklist is shipped to the client** and the same glob
  rules run in `mentions.js` (it's spam-hiding, not a secret). Glob rules run in both places;
  semantic rules (below) are server-only.
- **Semantic moderation (future).** When the semantic subsystem lands, a rule kind scores a
  mention's `content` against a concept ("spam"/"abuse") via embeddings + a threshold, slotting into
  the same server-side pipeline. Not available client-side, so semantic-moderated sites use
  `asset`/`baked`. (Ties into [decision `search`] — the shared embedding subsystem.)
- **A `moderate-mentions` skill** (an agent skill alongside the other authoring skills) scans the
  current mention set, flags likely spam/abuse, and either auto-filters or **presents a decision
  list** for the author to confirm. Crucially it **distills** confirmed cases into *generalised*
  glob (and later semantic) rules appended to the blocklist — so the list stays **small and
  effective** rather than an ever-growing pile of individual URLs.
- **Receiver-side delete** (e.g. webmention.io's dashboard) remains the quick one-off: delete there,
  the next `fetch` won't return it. The committed blocklist is the version-controlled, reproducible
  path; per-mention approval queues are deferred.

## Separate publish pipeline

The `_mentions/` assets are published **independently of the content build/deploy**, so you can
update one without the other. colophon already partitions output by path at publish time (the
router sends `_search/**` to the R2 publisher while content goes to Pages); the same machinery
publishes *only* the `_mentions/**` prefix.

```
colophon webmention publish --env production    # fetch + write _mentions/ + push ONLY that prefix to R2
```

So the two cadences are decoupled:

- **Content pipeline** (`build` → `publish`): ships HTML + JS placeholders. Never depends on the
  mentions cache; a fresh runner is fine.
- **Webmention pipeline** (`webmention publish`, e.g. hourly cron): refreshes `_mentions/` on R2.
  The JS path picks it up in the browser with no site rebuild.

This is the strongest reason JS is the default render path — it's the only one that benefits from
the decoupling (a baked theme still needs a content rebuild to reflect new mentions, so baking
suits low-frequency / no-JS sites). Implementation reuses the per-publisher routing
(`router.Owns`/`Keep`) plus a publish that only materialises the `_mentions/` tree.

## Search: mentions as down-ranked results (optional, `asset` mode)

Replies carry real text ("someone said X about post Y"), so indexing them into the site's lexical
search is sensible — and plausible, because we already hold the normalised list. Design:

- **Only `asset` mode.** A static index needs the data at build time; `live` has none. So
  search-indexed mentions are an `asset`-mode / `fetch`-driven feature (off in `live`/`disabled`).
- **Only text-bearing kinds.** Index `reply`/`mention` (they have `content`); skip `like`/`repost`
  (nothing to match). Each indexed doc carries `content`, `author`, the **target post** it lives
  under, and `kind: mention`.
- **A separate, down-ranked shard.** Rather than mixing mention docs into the content index (which
  would churn it on every mentions refresh), emit a **second search shard** for mentions, merged
  client-side. This keeps it on the **webmention publish cadence** (consistent with `_mentions/`
  decoupling) and lets the UI treat it differently: a **rank penalty** plus grouping so mention
  hits sort **below** content hits — or render in a distinct "Mentions" group **appended to the
  bottom** of the results list. Configurable; default down-ranked-and-grouped.
- **A hit links on-site:** to the post's responses anchor (keeps the reader on the site), citing the
  mention's source URL. Freshness is as-of-last index refresh.
- **Blocklist-gated.** Mentions are filtered through the same [moderation pipeline](#moderation-a-distilled-committed-blocklist)
  *before* indexing — blocked mentions are neither displayed nor searchable.

This reuses the internal pure-Go BM25 index (decision `search`): add a `kind` field + a rank weight,
emit the extra shard, and teach the search UI to group/penalise `kind: mention`. Marked a **later
enhancement** (after Tier 2 display), gated by an explicit `search_index: true` under `webmention`.

## Fit with existing design

| Concern | Reused pattern |
|---|---|
| Separate JSON asset, R2-routed, cross-origin | `_search/` index + `publish --create` CORS |
| Browser fetch + render, no-JS fallback | `search-ui.js` / `player.js` progressive enhancement |
| Template data exposure (`mentions`/`mentions_html`) | `attachments`/`attachments_html` |
| Per-path publish partitioning | the router's `_search/**` → R2 split (`router.Owns`/`Keep`) |
| Read token from environment, not config | all secrets via `{env:…}` |
| Config wiring | `federation.indieweb.webmention.receiver` (already present, unread) |
| Parsing mention authors | the mf2 `h-card`/`h-entry` just shipped |

## Config

```yaml
sites:
  - id: main
    federation:
      indieweb:
        webmention:
          receiver: https://webmention.io/blog.example.com/webmention  # the rel=webmention endpoint
          # token read from env, e.g. WEBMENTION_IO_TOKEN, never written here
          provider: jf2                # reader provider (read API + client descriptor); default jf2
          display:
            mode: asset                # live | asset | disabled   (see Display)
          # blocklist lives in .colophon/webmention-block.yml (committed), not inline
```

`mode: live` needs nothing else; `asset` uses the `webmention fetch`/`publish` pipeline (and can be
baked at build by the theme); `disabled` ships nothing. Per page: `webmentions: false` opts a post
out when the mode is active.

`<link rel="webmention" href="{{ receiver }}">` is emitted in every page `<head>` when a receiver
is configured (the discovery tag senders look for).

## Key decisions

1. **Separate `webmention` command** (`send` after publish, `fetch` before build, `publish` on its
   own cadence) — not folded into `build`/`publish`, because of the live-URL ordering constraint
   and the decoupled-refresh goal.
2. **Received data is a separate `_mentions/` asset** (R2-routed, CORS via `--create`), never mixed
   into content — mirrors `_search/`. The cache is **not reproducible** (external state); builds
   are **graceful when it's empty**, and freshness comes from `fetch`/`publish`, not the repo.
3. **One JSON per post**, normalised (`type`/`author`/`url`/`content`), not raw receiver payload.
4. **Display is a per-site `mode`** — `live` | `asset` | `disabled` (there is **no** separate baked
   mode). `live` = browser→receiver direct (most realtime, client-side glob blocklist, **no
   build-time data** so no count/bake, no no-JS); `asset` = browser→our R2 asset refreshed by a cron
   `publish` (full moderation, self-hosted) **and** the synced list is exposed at build so a theme
   *may* bake it (baking is a theme capability within `asset`, not a mode); `disabled` = nothing
   ships, zero counts, page toggle ignored. Per-page opt-out via `webmentions: false`. The engine
   exposes `mentions`/`mentions_html`/`has_mentions`/`mentions_src`/`mentions_enabled`; the theme
   renders. `live` mode's fetch JS is parameterised by the **reader provider's client descriptor**.
4b. **Moderation is a declarative, committed blocklist** of glob rules over mention attributes
   (`.colophon/webmention-block.yml`), re-applied every `fetch` (and shipped to the client in `live`
   mode). Future **semantic** rules run server-side; a **`moderate-mentions` skill** distills
   confirmed spam into small, general rules. Survives full-regenerate because it's declarative.
5. **TTS invariant**: mentions live outside the content body the speech extractor reads, so they're
   never spoken — true for both paths.
6. **Separate publish pipeline**: `webmention publish` pushes only `_mentions/` to the store, so
   mentions and content update independently (reuses per-publisher path routing).
7. **Sent-cache** drives correct re-send on link changes; **`aliases:`** is the soft dependency
   for surviving URL changes.
8. webmention.io as the receiver (no self-hosted endpoint) — per PLAN §10/§14.

## Acceptance criteria

- `colophon webmention send` discovers endpoints and POSTs for outbound links; re-run is a no-op
  unless links changed; dropped links trigger a delete-style re-send.
- `colophon webmention fetch` writes normalised JSON into the local cache; `webmention publish`
  pushes only `_mentions/**` to the store, independent of a content deploy.
- A build with an **empty** cache succeeds and shows no mentions (graceful); a build with the cache
  present exposes `mentions`/`mentions_html` to templates and emits `_mentions/` assets.
- A mention deleted on webmention.io disappears after the next `fetch`+`publish`; a post that drops
  to **zero** mentions has its `_mentions/<post>.json` removed and the orphan pruned from the store
  (JS path 404s → renders nothing). Pruning is scoped to the `_mentions/` prefix.
- A JS-enhanced theme shows likes/reposts/replies; with JS off the post is unaffected; `minimal`
  bakes them statically. Mentions never appear in a post's TTS audio.
- `display.mode: live` renders mentions with **no** `fetch`/`publish`/rebuild (browser → receiver),
  honouring the shipped glob blocklist client-side, with no build-time count; `asset` renders from
  the published `_mentions/` (and exposes the synced list so a theme may bake it at build);
  `disabled` ships nothing (zero counts, `webmentions:` page setting ignored). `webmentions: false`
  opts a single post out when the mode is active.
- A blocklisted mention (glob over domain/url/author/content) never appears in any mode; the
  blocklist survives a full `fetch` regenerate (it's committed, not edited into the export).
- Secrets come only from the environment; a site with no `webmention` config emits nothing.

## Files to create (when built)

- `internal/webmention/` — endpoint discovery + sender (sent-cache) + reader **provider** (server
  fetch **+ client descriptor**) + normaliser + the **blocklist filter pipeline**.
- `internal/cli/webmention.go` — the `webmention {send,fetch,publish}` command group; `publish`
  supports the incremental (changed-mention-posts-only) re-render for `baked`.
- build: expose `mentions`/`mentions_html`/`has_mentions`/`mentions_src` to templates, emit
  `_mentions/` assets + the `<link rel="webmention">` head tag; in `live` mode emit the provider
  client descriptor + the distilled blocklist for client-side filtering.
- `internal/render/themes/*/…` — a `data-mentions` placeholder + engine-emitted `mentions.js`
  (parameterised by the provider descriptor; applies glob blocklist in `live`) and a pongo-baked
  block (text themes), kept outside the content/`e-content` element.
- `contrib/skills/` (+ wiring) — a `moderate-mentions` skill that flags spam/abuse and distills
  confirmed cases into blocklist rules.

## Out of scope (future)

- Self-hosted webmention receiver (PLAN §14 leaves this open; webmention.io is the v1 choice).
- Rich moderation UI / per-mention approval.
- Sending *as* specific post types (replies/likes from colophon itself) — colophon publishes
  articles; it links, it doesn't (yet) author reply-posts.
- Fediverse/Bridgy Fed backfeed — a later layer that reuses this send/receive + mf2 substrate.
