# Design: federation (IndieWeb, POSSE, syndication)

> Status: **substrate shipped, the rest designed.** Built: microformats2 (h-entry/h-card/h-feed),
> `rel=me`, RSS/Atom/JSON feeds, and `aliases` redirects (URL stability). Webmention is detailed in
> [webmention.md](webmention.md); this is the umbrella — the posture, the reader/syndicator abstractions,
> POSSE, WebSub, and the cross-cutting concerns. No POSSE/WebSub code yet. Relates to PLAN §10.

Goal: let a colophon blog participate fully in the social web — be followable, get replies/likes
back, and cross-post to silos — while staying a **static site**. The organising principle:

## Posture: be a source, not a server

| Be a… | Means | Static-friendly? |
|---|---|---|
| **Source** | emit mf2 + feeds + discovery tags; thin post-publish "notify"/"syndicate" steps; let hosted relays do the rest | ✅ our lane |
| **Server** | run an ActivityPub/AT actor, a receiving endpoint, a reader/Microsub | ❌ needs a live server |

Bridgy Fed needs only **mf2 + webmention, or an RSS/Atom feed** — both already emitted — to make
the *site itself* followable from Mastodon/Bluesky. The heavy lifting is offloaded to relays; we
emit standards and run small, decoupled senders.

## Abstractions only where mechanisms diverge

colophon has two existing naming conventions, split by *shape*: a **list of pluggable
destinations** uses `driver` (publishers and sources are both `{id, driver, settings}`), while the
**single external service that produces/serves content** for a modality uses `provider`
(generation). Federation adds one of each — and matches the convention by shape:

### 1. Reader (`provider`) — reading webmentions back

There's one receiver per site, so this is the *single-service* shape → `provider`, like generation.
The Webmention spec standardises *receiving*, not *reading back*, so each receiver exposes a
different read API. Model it as a `Reader` interface + a `provider` (default `webmention.io`/JF2,
plus a `custom` JF2 source for self-hosted/compatible), selected by config.
Bridgy *backfeed* arrives in your receiver as ordinary webmentions, so it is read through the same
Reader — not a separate provider. Full detail in [webmention.md](webmention.md).

### 2. Syndicator (`driver`) — POSSE (cross-posting)

POSSE matters for anyone with real social reach, and Bridgy-only POSSE gives little control over
per-network formatting/threading and depends on a relay — so native syndication is a first-class
goal. Each target is a `Syndicator`:

```
type Syndicator interface { Syndicate(ctx, post) (siloURL string, err error) }
//   drivers (mirroring publishers/sources):
//     mastodon – instance URL + access token (env); statuses + media API
//     bluesky  – handle + app password (env); AT-proto createRecord + blob upload
//     bridgy   – POST to brid.gy/publish, parse the created silo URL from the response
//     command  – run a user command; stdout = silo URL (empty stdout = fire-and-forget webhook)
```

Syndication is the *list-of-destinations* shape, so it uses **`driver`** — each entry is
`{ id, driver, …settings }`, byte-for-byte the `PublisherConfig`/`SourceConfig` shape (`id` +
`driver` + remaining settings). `driver` is the concrete mechanism (`mastodon`, `bluesky`,
`bridgy`, `command`); `id` is an arbitrary handle that `syndicate:` (per-env and per-post)
references. A site may configure **many** syndicators (the `syndication:` list, like the publishers
list). Bridgy is simply `driver: bridgy` with a `network:` field naming the silo to publish to —
not a special "via". (It really is "publishers, for silos.")

**The `command` syndicator is also a publish webhook.** Mirroring the `command` *publisher*, it
runs a user-defined command per post with interpolated placeholders (`{url}` canonical, `{title}`,
`{slug}`, `{summary}`, `{tags}`, `{json}` = path to a metadata file) and env for secrets — so a
user can wire up anything (a Discord/Slack webhook, a Bluesky CLI, an n8n flow, a custom API). The
trick that makes it both a syndicator and a generic hook: **stdout is the silo URL.** If the command
prints a URL it's recorded in the ledger and rendered as `u-syndication`; if it prints nothing it's
a **fire-and-forget publish webhook**. No separate hook system — the same interface covers both.

**Bridgy is transparent for receiving, a syndicator for sending** — the two are unrelated:

- *Backfeed (inbound):* Bridgy polls your connected silo accounts on its own schedule and POSTs
  webmentions to your receiver. colophon never calls Bridgy; replies arrive and are read via the
  Reader. **Not modeled** — it's invisible infrastructure.
- *POSSE (outbound):* Bridgy does **not** auto-publish new posts, so automating cross-posting means
  colophon **actively** POSTs to `brid.gy/publish` and records the returned silo URL. That's an
  explicit syndication action → it's a `Syndicator` driver like the rest. (Implementation note:
  Bridgy verifies the source links to `brid.gy/publish/{silo}`, so the driver includes that link
  in the source it sends.)

So `driver: bridgy` buys cross-posting to networks without a native driver (or without holding
their API tokens yourself), at the cost of per-network formatting control.

Syndication runs as a **post-publish step** (`colophon syndicate`, after the canonical URL is live),
decoupled and best-effort like webmention send — it never blocks the deploy.

## The syndication ledger — and why it's different from the webmention cache

A sidecar ledger (e.g. `.colophon/syndication.json`) maps `post → {network: {url, time}}`. It is
the idempotency key (don't repost on rebuild), the `u-syndication` data ("Also posted on…"), and the
backfeed-pairing key.

**Crucial contrast with webmentions:** the webmention export is *regenerable* (re-fetch from the
receiver), so an empty CI runner is fine. The syndication ledger is **authoritative and NOT
regenerable** — you cannot reliably re-derive "which Mastodon post is the copy of this entry." So:

- **The ledger must be durable** — committed to the repo (it's small and append-mostly) or kept in
  persistent storage. A fresh runner *without* it would **re-POSSE everything (double-post)**.
- Therefore `syndicate` must **refuse to run, or run dry, when the ledger is absent/stale** unless
  explicitly forced — the opposite of the webmention "graceful when empty" rule.

This is the single biggest operational gotcha in the whole federation surface; the design must make
double-posting structurally hard (commit the ledger; idempotent against it; `--dry-run` default in
CI without a ledger).

## What does NOT need an abstraction

| Piece | Why |
|---|---|
| Webmention **send** | one spec-standard algorithm (discover endpoint, POST) |
| `rel=webmention`, `rel=hub` tags | config strings |
| **Bridgy backfeed** | lands in your receiver → read via the Reader |
| **WebSub ping** | one protocol; the hub is a config URL |

## WebSub (instant feed push)

Emit `<link rel="hub" href="…">` in the feeds and **ping the hub on publish** so readers and
aggregators update immediately instead of polling. Thin: a discovery tag + one POST in the
post-publish step. Hubs are hosted (Superfeedr, websubhub.com). No provider abstraction.

## Cross-cutting considerations (apply regardless of which pieces ship)

1. **Where work runs.** Static = no server; every action is a build/CI step or a hosted relay.
2. **State & idempotency.** The syndication ledger (durable, committed) and webmention sent-cache;
   never repeat actions on a rebuild; a rebuild is not a new post.
3. **Loops & dedup.** Don't webmention-loop; dedup backfed responses; `u-syndication` ties copies to
   the canonical.
4. **Edits/deletes.** Default: post-once; silo copies are point-in-time and don't track edits
   (optional propagation later). Canonical is the source of truth; `aliases` keep old URLs resolving.
5. **Identity & SEO.** `rel=me`, `u-syndication`, canonical URLs; copies cite the canonical to avoid
   duplicate-content problems.
6. **Secrets.** Per-network tokens, Bridgy OAuth (their side), webmention.io token, WebSub — all
   env-only; more features = more CI-secret surface.
7. **Privacy/moderation.** Displaying backfed third-party content (avatars, replies) → block/allow
   lists, avatar caching/proxying, opt-in, spam handling.
8. **Failure/decoupling.** Network steps flake/throttle → best-effort, non-blocking, retryable, and
   decoupled from the content build (the separate publish pipeline).
9. **Per-network formatting.** Char limits (Mastodon ~500 instance-variable, Bluesky 300), link-back,
   hashtags, media + alt, threading long posts; per-post custom syndication text.
10. **Selective syndication.** `syndicate:` frontmatter chooses targets (opt-in/out) per post.
11. **Display freshness.** JS-rendered mentions are *not* tied to page regeneration: the browser
    fetches the `_mentions/` asset live, so a scheduled `webmention publish` (refresh that asset, no
    site rebuild) updates them near-live. Only the no-JS *bake* path is as-fresh-as-the-last-build.

## Environments: read everywhere, write only where enabled

Federation splits cleanly across environments, and the split is a **safety property**:

- **Webmention reading is site-domain-scoped, so it's shared.** webmention.io keys mentions by your
  *production* target URLs, so the `webmention` config lives at the **site** level and every
  environment inherits it. A **preview build reads the same production mentions** (previewing how
  real responses look) — no separate receiver, no extra config.
- **Syndication is environment-gated and off by default**, like `allow_publish`. Which syndicators
  fire is an *environment* decision (`environments[].syndicate: [ids]`); an env that omits it —
  notably **preview/draft** — never cross-posts. `colophon syndicate` also takes the same kind of
  deploy latch. This makes double-posting (or POSSEing a draft) structurally impossible from preview.

So: **read in every environment, write only where explicitly enabled.**

## Config sketch

```yaml
sites:
  - id: main
    federation:
      feeds: [rss, atom, json]
      websub:
        hub: https://pubsubhubbub.superfeedr.com     # rel=hub + ping on publish
      indieweb:
        webmention:
          endpoint: https://webmention.io/blog.example.com/webmention   # advertised rel=webmention
          source:   https://webmention.io/api/mentions.jf2              # read API (JF2 reader)
      syndication:                             # a list — many syndicators per site (id + driver + settings)
        - { id: mastodon, driver: mastodon, instance: https://hachyderm.io }   # token from env MASTODON_TOKEN
        - { id: bluesky,  driver: bluesky,  handle: me.bsky.social }           # app password from env
        - { id: discord,  driver: command,  command: "curl -sf -X POST $DISCORD_WEBHOOK -d @{json}" }  # webhook: no stdout → fire-and-forget
        - { id: twitter,  driver: bridgy,   network: twitter }                 # via brid.gy/publish/twitter

environments:
  - name: production
    publish: [cf, r2]
    syndicate: [mastodon, bluesky, discord, twitter]   # which syndicators fire here
  - name: preview
    publish: [cf-preview]
    # no `syndicate:` → preview never cross-posts, but still reads the production webmentions above
```

Per post: `syndicate: [mastodon, bluesky]` (else all the env's configured ids), `syndicate: false`
to opt out, and an optional custom blurb (`syndicate_text:`). Resolved silo URLs are written to the
ledger and rendered as `u-syndication` links on the post; manually-added `syndication:` frontmatter
is honoured too. (`webmention` sits at the site level, so every environment — preview included —
reads the same production mentions; `syndicate:` is per-environment, so only the envs that list it
ever post.)

## Phasing

- **Tier 1 — cheap static + thin send:** `rel=webmention` tag, `u-syndication`, WebSub `rel=hub` +
  ping, webmention send. Unlocks Bridgy + Bridgy Fed + instant feeds with little stateful code.
- **Tier 2 — receive/display:** webmention `fetch` + `_mentions/` assets ([webmention.md](webmention.md)).
- **Tier 3 — POSSE:** the `Syndicator` abstraction + sidecar ledger + `colophon syndicate`, with
  `bridgy`, `mastodon`, `bluesky`, `command` drivers and per-network formatting.

## Out of scope

- Running an ActivityPub/AT actor or a reader/Microsub (delegate to Bridgy Fed / hosted readers).
- A self-hosted webmention receiver (use webmention.io hosted or self-host *it*).
- Propagating edits/deletes to silo copies (point-in-time by default).
