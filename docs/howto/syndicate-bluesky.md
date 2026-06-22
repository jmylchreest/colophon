# How to syndicate to Bluesky (POSSE)

> Status: **shipped.** The `bluesky` driver, `colophon syndicate`, and the ledger work today.

POSSE = Publish on your Own Site, Syndicate Elsewhere: the post is canonical on your blog, and a
copy is cross-posted to Bluesky linking back to it.

## Steps

1. **Have a Bluesky account** — note your handle (e.g. `me.bsky.social`).
2. **Create an app password:** **Settings → Privacy and security → App passwords → Add** (don't use
   your main password). Copy it.
3. **Export it** as a CI secret: `export BLUESKY_APP_PASSWORD=...`
4. **Configure a syndicator** (`driver: bluesky`):
   ```yaml
   sites:
     - id: main
       federation:
         syndication:
           - id: bluesky
             driver: bluesky
             handle: me.bsky.social
             app_password: "{env:BLUESKY_APP_PASSWORD}"   # never a literal
   environments:
     - name: production
       syndicate: [bluesky]      # only this env cross-posts; preview/draft never do
   ```
5. **Publish, then syndicate:**
   ```sh
   colophon publish  --env production --allow-publish
   colophon syndicate --env production --allow-publish
   ```
   colophon authenticates (handle + app password → AT-proto session), creates the post (with a
   link card back to the canonical), records the Bluesky URL in the ledger, and renders it as an
   "Also posted on…" (`u-syndication`) link. Idempotent via the ledger.

## Notes

- Bluesky's limit is **300 characters** — long posts are truncated with a link back; set
  `syndicate_text:` per post for a custom blurb.
- **Commit the syndication ledger** (`.colophon/syndication.json`); without it a fresh runner would
  re-post, so `syndicate` refuses to run blind.
- Replies/likes/reposts on the Bluesky copy can flow back to your post via Bridgy backfeed — see
  [Show webmentions](webmentions.md).
- Prefer not to manage credentials? Use `driver: bridgy` with `network: bluesky`.
