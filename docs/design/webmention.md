# Design: webmention

> Status: **designed, not built** · relates to PLAN §10 (Federation & IndieWeb), §6 (build
> pipeline). Builds on the microformats2 markup now shipped in the themes. The config struct
> `federation.indieweb.webmention` already exists (`internal/core/site.go`) but is unread; this
> design wires it. No code yet.

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
| **`colophon webmention fetch`** | **before** `build` (or standalone/scheduled) | pulls received mentions into the cache so the build can emit/bake them |

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
- `fetch` writes a **committed cache** (`.colophon/cache/webmentions/` or content-adjacent) so a
  plain `build` needs no network and is reproducible (same discipline as the generated-image
  cache and audio sidecars).

One JSON-per-post (no sharded manifest like search) because a post page already knows its own key.

## Display: two render paths, one asset

Both paths read the *same* `_mentions/<post>.json` — clean separation from content either way.
This is the progressive-enhancement contract colophon already uses for the audio player and
search box.

- **JS path** — the engine emits a shared `mentions.js` (like `search-ui.js` / `player.js`) when a
  page opts in via a placeholder (`<section data-mentions="<base>/_mentions/<key>.json">`). The
  browser fetches the JSON and renders likes/reposts/replies. No JS → the section stays empty,
  the post is unaffected. **Always current** (re-fetched in the browser, picks up new/removed
  mentions between deploys).
- **Bake path** — at **build** time the engine reads the same JSON from the cache and injects the
  rendered HTML directly. **No JS**, fully static; mentions are **as-of-last-fetch**.

### Decision: JS by default, bake for the no-JS themes

Recommended default: **JS-fetch for the standard themes (press, default, signal, flux, obsidian),
bake for the text-first themes (minimal)** — selectable per theme, and a site may force baking.

Rationale:
- It matches the existing PE split exactly (search + player are JS-enhanced; `minimal` is
  deliberately JS-free), so theme authors meet no new concept.
- JS-fetch keeps the **HTML cacheable/immutable** and the mentions **fresh** without a rebuild —
  important because mentions change far more often than content (a post can keep collecting likes
  for months). Baking would force a rebuild+redeploy to show each new mention.
- Baking stays available for `minimal` and any site that wants zero JS, at the cost of freshness.

Either way the data lives only in `_mentions/` assets; the theme just chooses when to read them.

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

**Spam/moderation:** inbound mentions can be junk. Start minimal — rely on webmention.io's
filtering plus an optional `blocklist` of domains in config; defer per-mention approval/allowlist.

## Fit with existing design

| Concern | Reused pattern |
|---|---|
| Separate JSON asset, R2-routed, cross-origin | `_search/` index + `publish --create` CORS |
| Browser fetch + render, no-JS fallback | `search-ui.js` / `player.js` progressive enhancement |
| Committed, reproducible cache; offline build | generated-image cache + audio sidecars |
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
          # blocklist: [spam.example]   # optional domain filter (later)
```

`<link rel="webmention" href="{{ receiver }}">` is emitted in every page `<head>` when a receiver
is configured (the discovery tag senders look for).

## Key decisions

1. **Separate `webmention` command** (`send` after publish, `fetch` before build) — not folded
   into `build`/`publish`, because of the live-URL ordering constraint.
2. **Received data is a separate `_mentions/` asset** (R2-routed, CORS via `--create`, committed
   cache), never mixed into content — mirrors `_search/`.
3. **One JSON per post**, normalised (`type`/`author`/`url`/`content`), not raw receiver payload.
4. **JS-fetch default + bake for no-JS themes**, both reading the same asset.
5. **Sent-cache** drives correct re-send on link changes; **`aliases:`** is the soft dependency
   for surviving URL changes.
6. webmention.io as the receiver (no self-hosted endpoint) — per PLAN §10/§14.

## Acceptance criteria

- `colophon webmention send` discovers endpoints and POSTs for outbound links; re-run is a no-op
  unless links changed; dropped links trigger a delete-style re-send.
- `colophon webmention fetch` writes normalised `_mentions/<post>.json` into the committed cache;
  offline `build` emits them to output and routes them like `_search/`.
- A JS-enhanced theme shows likes/reposts/replies; with JS off the post is unaffected; `minimal`
  bakes them statically.
- Secrets come only from the environment; a site with no `webmention` config emits nothing.

## Files to create (when built)

- `internal/webmention/` — endpoint discovery + sender + receiver client + normaliser.
- `internal/cli/webmention.go` — the `webmention {send,fetch}` command group.
- build emit for `_mentions/` + `<link rel="webmention">` head tag + optional bake.
- `internal/render/themes/*/…` — a `data-mentions` placeholder + `mentions.js` (engine-emitted).

## Out of scope (future)

- Self-hosted webmention receiver (PLAN §14 leaves this open; webmention.io is the v1 choice).
- Rich moderation UI / per-mention approval.
- Sending *as* specific post types (replies/likes from colophon itself) — colophon publishes
  articles; it links, it doesn't (yet) author reply-posts.
- Fediverse/Bridgy Fed backfeed — a later layer that reuses this send/receive + mf2 substrate.
