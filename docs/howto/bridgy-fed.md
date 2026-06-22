# How to federate via Bridgy Fed

> Status: **works today.** Bridgy Fed needs only microformats2 + a feed (and `rel=me`), all of
> which colophon already emits — no colophon code beyond what's shipped.

[Bridgy Fed](https://fed.brid.gy) makes your *site itself* followable from Mastodon and Bluesky:
people follow `@yourdomain`, your posts federate, and replies come back as webmentions — without
you running an ActivityPub server or even having a Mastodon/Bluesky account.

## Steps

1. **Publish your site** with colophon as usual. It already emits:
   - `h-entry`/`h-card`/`h-feed` microformats2, an RSS/Atom/JSON feed, and `rel="me"` on your
     author links. (Confirm the feed link and an `h-card` are on your home page.)
2. **Add a webmention endpoint pointing at Bridgy Fed** so it can receive interactions for you:
   ```yaml
   federation:
     indieweb:
       webmention:
         receiver: https://fed.brid.gy/webmention   # emitted as <link rel="webmention"> on every page
   ```
   colophon emits the `<link rel="webmention">` discovery tag site-wide when `receiver` is set — no
   manual theme edit needed.
3. **Enrol** at <https://fed.brid.gy> and follow its current instructions for your domain (it
   verifies your site, then your handle becomes `@yourdomain@yourdomain`). Bridgy Fed's onboarding
   changes over time, so use its docs as the source of truth: <https://fed.brid.gy/docs>.
4. **Done.** Fediverse/Bluesky users can follow you; new posts federate from your feed; replies
   arrive at the webmention endpoint (see [Show webmentions](webmentions.md) to display them).

## Notes

- This is **federation**, not syndication: there's no separate silo account — your site *is* the
  account. For posting copies *to* your own Mastodon/Bluesky accounts instead, see the syndication
  guides.
- Forward-only is fine: you can be followable without displaying replies; add webmention display
  when you want the conversation on your page.
