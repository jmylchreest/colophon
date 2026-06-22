# How-to guides

Short, zero-to-published recipes. The design behind them is in
[../design/federation.md](../design/federation.md) and [../design/webmention.md](../design/webmention.md).

| Guide | Status |
|-------|--------|
| [Federate via Bridgy Fed](bridgy-fed.md) — be followable from Mastodon/Bluesky | **works today** (uses the mf2 + feeds colophon already emits) |
| [Show webmentions](webmentions.md) — replies/likes on your posts | **planned** (config + `colophon webmention` shown are the designed interface) |
| [Syndicate to Mastodon](syndicate-mastodon.md) | **planned** (`colophon syndicate`) |
| [Syndicate to Bluesky](syndicate-bluesky.md) | **planned** (`colophon syndicate`) |

> "Planned" means the `federation:`/`syndication:` config and the `colophon webmention`/`syndicate`
> commands are the agreed design but **not yet implemented**. Steps are written against that design
> so they're ready when it lands. The shipped substrate they build on — microformats2, `rel=me`,
> RSS/Atom/JSON feeds, `aliases` — is real today.
