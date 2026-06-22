# How to show webmentions (replies, likes, reposts)

> Status: **partly shipped.** Emitted today: the `<link rel="webmention">` discovery tag (set
> `federation.indieweb.webmention.receiver`), `rel=me`, microformats2, and **`colophon webmention
> send`** (notify the sites you link to). The `webmention fetch`/`publish` commands and the response
> *display* below are the designed interface ([../design/webmention.md](../design/webmention.md)),
> not yet built.

Webmentions let other sites' replies/likes/reposts appear under your posts — "comments without a
database." A static site can't receive POSTs, so a hosted receiver ([webmention.io](https://webmention.io))
collects them and colophon pulls them in at build/refresh time.

## Steps

1. **Sign in to [webmention.io](https://webmention.io)** via IndieAuth, which needs `rel="me"` on the
   **exact URL you sign in with**, linking *bidirectionally* to a provider it can authenticate
   (GitHub is the easy path). colophon emits an author's `rel="me"` (all of their `urls:`) in the
   `<head>` of **their posts and their author feed page**, so:
   - Sign in with your **author feed page** — `https://example.com/authors/<your-id>/` — not the bare
     domain (the home page lists all authors, so it has no `rel="me"`).
   - Make the link **bidirectional**: set your GitHub profile's *website* field to that **same**
     author-page URL (colophon already emits `rel="me"` → your GitHub from your `urls:`).

   webmention.io then gives you:
   - a receiver endpoint: `https://webmention.io/yourdomain/webmention`
   - an **API token** (for reading your mentions back).
2. **Configure it** (token via env, never in config):
   ```yaml
   federation:
     indieweb:
       webmention:
         receiver: https://webmention.io/yourdomain/webmention  # emitted as <link rel="webmention"> (shipped)
         driver: jf2                                            # reader driver (read API); planned
         display:
           mode: asset                                          # live | asset | disabled (planned)
   # export WEBMENTION_IO_TOKEN=...   (CI secret)
   ```
3. **Build** — colophon emits `<link rel="webmention">` site-wide today; the per-post responses block
   (and `fetch`/`publish` below) are the planned display layer.
4. **Pull mentions in:**
   ```sh
   colophon webmention fetch        # writes _mentions/<post>.json (the display data)
   colophon webmention publish      # pushes only _mentions/ to your asset host (R2), on its own schedule
   ```
   JS-rendered themes fetch that asset live, so a scheduled `webmention publish` keeps responses
   fresh **without rebuilding the site**. (No-JS/text themes show them as of the last build.)
5. **Send webmentions** when *you* link to others, so you show up in their comments *(shipped)*:
   ```sh
   colophon webmention send --env production   # run AFTER publish; the source URLs must be live
   ```
   It scans the built output's outbound links (per page's canonical URL), discovers each target's
   endpoint, and POSTs. A sent-cache (`.colophon/cache/webmention-sent.json`) makes re-runs send only
   new links and re-ping removed ones. `--dry-run` reports without sending.
6. **(Optional) Social replies via Bridgy** — connect your silo accounts at <https://brid.gy>; it
   backfeeds replies/likes from Mastodon/Bluesky to your webmention.io endpoint, so they appear the
   same way. No extra colophon config.

## Notes

- Self-hosting: webmention.io is open source, or use a JF2-compatible receiver — point `source:` at
  its API (`driver: jf2`).
- Privacy/spam: a bl[ock]list and avatar caching are part of the design; treat displayed third-party
  content accordingly.
