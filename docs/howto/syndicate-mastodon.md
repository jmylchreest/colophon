# How to syndicate to Mastodon (POSSE)

> Status: **shipped.** The `mastodon` driver, `colophon syndicate`, and the ledger work today.

POSSE = Publish on your Own Site, Syndicate Elsewhere: the post is canonical on your blog, and a
copy is cross-posted to Mastodon linking back to it.

## Steps

1. **Have a Mastodon account** on any instance (e.g. `hachyderm.io`).
2. **Create an access token:** on your instance, **Preferences → Development → New application**;
   give it the **`write:statuses`** (and `write:media` for images) scope; create it; copy the
   **access token**.
3. **Export the token** as a CI secret: `export MASTODON_TOKEN=...`
4. **Configure a syndicator** (`driver: mastodon`):
   ```yaml
   sites:
     - id: main
       federation:
         syndication:
           - id: mastodon
             driver: mastodon
             instance: https://hachyderm.io
             token: "{env:MASTODON_TOKEN}"   # never a literal
   environments:
     - name: production
       syndicate: [mastodon]     # only this env cross-posts; preview/draft never do
   ```
5. **Publish, then syndicate** (syndicate runs after the canonical URL is live):
   ```sh
   colophon publish  --env production --allow-publish
   colophon syndicate --env production --allow-publish
   ```
   The Mastodon post URL is recorded in the syndication ledger and shown as an "Also posted on…"
   (`u-syndication`) link on your post. Re-running is idempotent (the ledger prevents double-posting).

## Notes

- **Commit the syndication ledger** (`.colophon/syndication.json`) — it's authoritative; a fresh CI
  runner without it would re-post. `syndicate` refuses to run blind without it.
- Per post: `syndicate: [mastodon]` to choose targets, `syndicate: false` to skip, `syndicate_text:`
  for a custom blurb. Long posts are truncated with a link back.
- Replies/boosts on the Mastodon copy can flow back to your post via Bridgy backfeed — see
  [Show webmentions](webmentions.md).
- No token to manage? Use `driver: bridgy` with `network: mastodon` instead (Bridgy holds the auth).
