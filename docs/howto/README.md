# How-to guides

Short, zero-to-published recipes. The design behind them is in
[../design/federation.md](../design/federation.md) and [../design/webmention.md](../design/webmention.md).

| Guide | Status |
|-------|--------|
| [Federate via Bridgy Fed](bridgy-fed.md) — be followable from Mastodon/Bluesky | **works today** (uses the mf2 + feeds colophon already emits) |
| [Show webmentions](webmentions.md) — replies/likes on your posts | **shipped** (`colophon webmention fetch/publish` + display modes) |
| [Syndicate with a command](syndicate-command.md) — POSSE to any target | **shipped** (the `command` driver + ledger + gating) |
| [Syndicate to Mastodon](syndicate-mastodon.md) | **planned** (native `mastodon` driver) |
| [Syndicate to Bluesky](syndicate-bluesky.md) | **planned** (native `bluesky` driver) |

> The syndication **harness** (the ledger, env/per-post gating, `--dry-run`) and the `command`
> driver ship today; the native `mastodon`/`bluesky` drivers are the next slices. "Planned" guides
> are written against the agreed design so they're ready when those drivers land.
