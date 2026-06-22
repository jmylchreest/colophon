---
name: colophon-moderate-mentions
description: Review a colophon site's received webmentions for spam/abuse and distill the bad ones into small, general glob rules in the committed blocklist (.colophon/webmention-block.yml), so moderation survives the cache's full regenerate. Use when the user asks to moderate, clean up, or block webmention spam/replies.
---

# colophon webmention moderation

Received webmentions are third-party content, so some will be spam or abuse. colophon's
moderation is a **declarative, committed blocklist** of glob rules over a mention's attributes,
applied at `webmention fetch` (and again at build). It must be declarative because `fetch`
**fully regenerates** the cache each run — editing the generated JSON is pointless, it gets
overwritten. Your job: find the bad mentions and turn them into the **smallest, most general
rules** that catch them (and their likely siblings) without catching legitimate replies.

## When to use

The user wants to moderate inbound mentions — "block this spammer", "clean up the replies",
"my webmentions have spam". Not for *sending* (that's `webmention send`).

## Requirements

This skill drives the `colophon` CLI. Before the first command, confirm it's installed:

```sh
command -v colophon || echo "colophon not found"
```

If missing, **stop and offer** the install — don't install silently:
`go install github.com/jmylchreest/colophon/cmd/colophon@latest` (or a release binary).

## Workflow

1. **Refresh and inspect** the current mentions:
   ```sh
   colophon webmention fetch -v        # pulls + applies the existing blocklist; writes the cache
   ```
   Then read `.colophon/cache/webmentions/**/*.json` — each is `{target, mentions: [{type,
   author{name,url,photo}, url, content, published}]}`.

2. **Classify.** For each mention decide: keep (genuine reply/like/repost), or block (spam,
   abuse, link farms, off-topic promotion). When unsure, **lean keep** and surface it to the
   user — false positives silence real people.

3. **Distill, don't enumerate.** This is the point of the skill: instead of listing every bad
   URL, find the *pattern* and write one rule. Prefer, in order:
   - a **domain** rule when a whole host is bad: `- "*.spam.example"`
   - an **author.url** glob for one persistent actor: `- author.url: "https://troll.example/*"`
   - a **content** glob for a spam phrase: `- content: "*free crypto*"`
   - a **type** rule to drop a whole interaction kind: `- type: "bookmark"`

   Collapse several individual bad mentions into the fewest rules that cover them. Keep the list
   **small and effective** — re-read existing rules and merge rather than append duplicates.

4. **Write the blocklist** at `.colophon/webmention-block.yml` (create if absent). It's a YAML
   list; each item is a bare string (matches the author/source **domain** or author URL) or a
   `{field: glob}` mapping. Fields: `domain`, `url`, `author.name`, `author.url`, `content`,
   `type`. Globs use `*` (any run) and `?` (one char), case-insensitive, anchored to the value.

   ```yaml
   # .colophon/webmention-block.yml — committed; survives fetch's full regenerate
   - "*.linkfarm.example"
   - author.url: "https://troll.example/*"
   - content: "*casino*"
   ```

5. **Verify** the rules catch what you intended and nothing else:
   ```sh
   colophon webmention fetch -v        # re-run: the log shows "blocked N"; confirm the count
   ```
   Spot-check the cache that a legitimate reply you expected to keep is still present.

6. **Commit** the blocklist (it's authoritative and must travel with the repo) and **publish**
   the refreshed mentions:
   ```sh
   git add .colophon/webmention-block.yml
   colophon webmention publish --env production --allow-publish   # deploys only _mentions/
   ```

## Notes

- **Present borderline calls to the user** rather than auto-blocking — moderation is editorial.
- **Quick one-off:** the receiver's own dashboard (e.g. webmention.io) can delete a single
  mention; the next `fetch` won't return it. The committed blocklist is the version-controlled,
  reproducible path and the right home for anything recurring.
- **`live` display mode** ships the *glob* rules to the browser for client-side filtering, so a
  `domain`/`author.url`/`content` glob works there too; field precision and any future semantic
  rules apply server-side only.
- Don't block by `published` date or one-off `url`s unless truly necessary — those don't
  generalise and bloat the list.
