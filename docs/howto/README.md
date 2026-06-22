# How-to guides

Short, zero-to-published recipes. The design behind them is in
[../design/federation.md](../design/federation.md) and [../design/webmention.md](../design/webmention.md).

| Guide | Status |
|-------|--------|
| [Federate via Bridgy Fed](bridgy-fed.md) — be followable from Mastodon/Bluesky | **works today** (uses the mf2 + feeds colophon already emits) |
| [Show webmentions](webmentions.md) — replies/likes on your posts | **shipped** (`colophon webmention fetch/publish` + display modes) |
| [Syndicate with a command](syndicate-command.md) — POSSE to any target | **shipped** (the `command` driver) |
| [Syndicate to Mastodon](syndicate-mastodon.md) | **shipped** (native `mastodon` driver) |
| [Syndicate to Bluesky](syndicate-bluesky.md) | **shipped** (native `bluesky` driver) |
| Syndicate via Bridgy — [no-credentials POSSE](../syndication.md#bridgy--let-bridgy-post-for-you-no-credentials-in-colophon) | **shipped** (`bridgy` driver) |

> Syndication ships in full: the harness (ledger, env/per-post gating, `--dry-run`) plus the
> `command`, `mastodon`, `bluesky`, and `bridgy` drivers. The complete reference (every driver, how
> each works, when to use which) is [../syndication.md](../syndication.md).
