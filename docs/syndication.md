# Syndication (POSSE)

**POSSE** — *Publish on your Own Site, Syndicate Elsewhere* — keeps the canonical copy of a post on
your blog and pushes copies to social accounts ("silos") that link back. colophon does this with
`colophon syndicate`: it walks your published posts, sends each to the configured **syndicators**,
and records the result in a committed **ledger** so re-runs never double-post. The recorded silo
URLs render on the post as `u-syndication` "Also posted on…" chips, each with the silo's brand icon
and network name (e.g. "Bluesky") — the same [silo icons](themes.md#silo-icons) used by responses.

This page is the full reference; the [how-to guides](howto/) are quick per-silo recipes.

## How it works

```
colophon publish   --env production --allow-publish   # 1. canonical post goes live first
colophon syndicate --env production --allow-publish    # 2. push copies to the silos, record the ledger
```

1. **Publish first.** Syndication links back to the canonical URL, and some drivers (Bridgy) fetch
   the live page — so the post must be deployed before you syndicate.
2. **`syndicate`** gathers eligible posts (type `post`, not draft, not opted out), works out each
   one's targets, and for every `(post, target)` *not already in the ledger*, calls the driver.
3. Each driver returns the **silo URL**, which is written to `.colophon/syndication.json` and, on the
   next build, rendered as a `u-syndication` link.

It performs **irreversible external actions** (posting to real accounts), so it's fenced:

- Only an environment's `syndicate:` targets ever fire — a `preview`/`draft` env that omits the key
  **never** posts.
- A gated env (`allow_publish: false`, typically production) needs `--allow-publish`.
- `--dry-run` shows exactly what would post and writes nothing.
- If the ledger file is missing, a real run **refuses to start** (it would re-post your whole back
  catalogue) unless you pass `--allow-publish` to seed it.

### The ledger — commit it

`.colophon/syndication.json` is authoritative: it's how colophon knows a post is already syndicated.
**Commit it to your repo.** Without it, a fresh CI runner would treat every post as new and re-post
everything. It maps post → driver → `{url, syndicated_at}`; `encoding/json` sorts keys so diffs stay
clean.

> **`.gitignore` gotcha:** to commit the ledger out of an otherwise-ignored `.colophon/`, ignore the
> directory's *contents*, not the directory — git can't re-include a file under a wholly-ignored dir:
> ```gitignore
> /.colophon/*                       # build trees, caches
> !/.colophon/syndication.json       # …but keep the ledger
> ```
> `/.colophon/` (trailing slash) followed by a `!` negation **silently fails** — the ledger stays
> ignored. `colophon init` scaffolds the correct form.

### Persisting the ledger in CI (commit it back)

The catch on GitHub Actions (or any CI): `colophon syndicate` *writes* the ledger in the runner, but
that runner is thrown away. Unless the workflow **commits the updated ledger back to the repo**, the
next run checks out the old ledger and **re-posts everything**. So a CI syndication step is two
parts — syndicate, then commit back:

```yaml
permissions:
  contents: write            # needed to push the ledger back
concurrency:
  group: deploy              # serialise: two overlapping runs must not both post before committing
jobs:
  deploy:
    steps:
      # … build + publish (the post must be live before you syndicate) …
      - name: Syndicate
        env:
          MASTODON_TOKEN: ${{ secrets.MASTODON_TOKEN }}
          BLUESKY_APP_PASSWORD: ${{ secrets.BLUESKY_APP_PASSWORD }}
        run: colophon syndicate --env production --allow-publish
      - name: Commit syndication ledger
        run: |
          git diff --quiet .colophon/syndication.json && exit 0
          git config user.name  "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add .colophon/syndication.json
          git commit -m "chore(syndication): update ledger [skip ci]"
          git push
```

Three things make this safe:

- **`contents: write`** — the default workflow is read-only; the commit-back needs write.
- **`[skip ci]`** in the commit message — so pushing the ledger doesn't trigger the workflow again
  (it would self-terminate anyway once the ledger is current, but this avoids the extra run).
- **`concurrency:`** — serialises runs, so two in-flight deploys can't each syndicate before either
  commits, which would double-post.

`colophon init` scaffolds these as commented steps in the Deploy workflow. *(Alternative, not yet
built: keeping the ledger in the asset store like the `_mentions/` pipeline, avoiding commit-back —
at the cost of versioning. Commit-back is the supported path today.)*

### Per-post controls (frontmatter)

| Frontmatter | Effect |
|-------------|--------|
| `syndicate: false` | Don't syndicate this post at all. |
| `syndicate: [bsky]` | Only these targets (a subset of the env's `syndicate:` list). |
| *(omitted)* | All of the environment's targets. |
| `syndicate_text: "…"` | A custom blurb for the silo copy (else the driver uses the title). |
| `syndication: [url, …]` | Manually-added "Also posted on…" URLs, shown alongside ledger ones. |

### Configuration shape

Syndicators are configured like sources/publishers — `{id, driver, …settings}` — under the site, and
each environment lists the ids it may post to:

```yaml
sites:
  - id: main
    federation:
      syndication:
        - { id: mastodon, driver: mastodon, instance: https://hachyderm.io, token: "{env:MASTODON_TOKEN}" }
        - { id: bsky,     driver: bluesky,  handle: me.bsky.social, app_password: "{env:BLUESKY_APP_PASSWORD}" }
environments:
  - name: production
    syndicate: [mastodon, bsky]   # preview/draft omit this → never syndicate
```

Secrets (tokens, app passwords) **only** come from the environment via `{env:VAR}` — never written as
literals.

### Scheduling

`syndicate` is idempotent (the ledger guards it), so it's safe to run after every publish, or on a
cron. A typical CI step runs `publish` then `syndicate` for production.

---

## Drivers

Four drivers, picked by `driver:`. All share the harness above (ledger, gating, `--dry-run`); they
differ only in **how the silo post is created** and **where the auth lives**.

### `command` — run any program (you own the integration)

**For:** any target without a built-in driver — a silo's CLI, a webhook, an internal system, a
notifier. Maximum flexibility; colophon holds no silo credentials.

**How:** runs your program once per post. The post is passed as environment variables
(`COLOPHON_POST_URL`, `_TITLE`, `_SUMMARY`, `_TEXT`, `_TAGS`, `_KEY`, `_PUBLISHED`) and as JSON on
stdin. The **first line of stdout** is taken as the silo URL (print nothing for fire-and-forget); a
non-zero exit is a failure. Post content is never interpolated into the command, so it can't inject
shell.

```yaml
federation:
  syndication:
    - { id: silo, driver: command, command: "./bin/post-to-silo" }
```
```sh
#!/usr/bin/env bash
# bin/post-to-silo — receives one post via env/stdin, prints the created URL
set -euo pipefail
curl -fsS -X POST https://silo.example/api/posts \
  -H "Authorization: Bearer $SILO_TOKEN" \
  --data-urlencode "text=${COLOPHON_POST_TITLE} ${COLOPHON_POST_URL}" | jq -r .url
```

### `mastodon` — post to a Mastodon account you control

**For:** cross-posting to your own Mastodon (any instance). You hold the access token.

**How:** `POST <instance>/api/v1/statuses` with `Authorization: Bearer <token>`. The status text is
the blurb (custom text, else the title) plus the canonical link; Mastodon auto-links it and renders a
preview card. The status's `url` is recorded. Text is trimmed to 500 chars but the link is always
preserved.

```yaml
federation:
  syndication:
    - id: mastodon
      driver: mastodon
      instance: https://hachyderm.io
      token: "{env:MASTODON_TOKEN}"
```
**Set up:** on your instance, *Preferences → Development → New application* with the `write:statuses`
scope; copy the access token into `MASTODON_TOKEN`.

### `bluesky` — post to a Bluesky account you control

**For:** cross-posting to your own Bluesky. You hold an app password.

**How:** AT-proto — `createSession` (handle + app password) gets a token, then `createRecord` writes
an `app.bsky.feed.post` with an **external embed card** linking back to the canonical post. Returns
the `https://bsky.app/profile/<handle>/post/<id>` permalink. Text (custom or title) is capped at 300
characters; the link-back is the card.

```yaml
federation:
  syndication:
    - id: bsky
      driver: bluesky
      handle: me.bsky.social
      app_password: "{env:BLUESKY_APP_PASSWORD}"
      # service: https://bsky.social   # optional; default
```
**Set up:** *Settings → Privacy and security → App passwords → Add* (don't use your main password);
copy it into `BLUESKY_APP_PASSWORD`.

### `bridgy` — let Bridgy post for you (no credentials in colophon)

**For:** cross-posting *without* colophon holding any silo tokens — [Bridgy](https://brid.gy) holds
your account auth. Good when you'd rather not manage credentials, or you syndicate to several
networks through one mechanism.

**How:** colophon sends Bridgy a *publish webmention* — `source` = your post, `target` =
`https://brid.gy/publish/<network>`. Bridgy fetches your post's microformats2 (which colophon emits)
and creates the silo post on your behalf, returning its URL. The post must be **live** (Bridgy
fetches it), and you must have **connected the account at brid.gy first**.

```yaml
federation:
  syndication:
    - { id: mast-via-bridgy, driver: bridgy, network: mastodon }
    - { id: bsky-via-bridgy, driver: bridgy, network: bluesky }
```
**Set up:** connect your silo account(s) at <https://brid.gy> and follow its instructions for your
domain. No tokens go in colophon.

> **`bridgy` driver vs Bridgy Fed:** different things. This driver is *syndication* (POSSE copies to
> accounts you have). [Bridgy **Fed**](howto/bridgy-fed.md) makes your *site itself* followable from
> the fediverse (no silo account) — that's federation, configured under `webmention`, not here.

## Choosing a driver

| You want… | Use |
|-----------|-----|
| Direct control, own the token, one account | `mastodon` / `bluesky` |
| No credentials in colophon; already use Bridgy; several networks | `bridgy` |
| A silo with no built-in driver, a webhook, or custom logic | `command` |

You can mix them — list several syndicators and put their ids in the env's `syndicate:`.

## See also

- Quick recipes: [Mastodon](howto/syndicate-mastodon.md) · [Bluesky](howto/syndicate-bluesky.md) ·
  [command](howto/syndicate-command.md)
- [Webmentions](howto/webmentions.md) — replies/likes back on your posts (the receive side).
- Design rationale: [design/federation.md](design/federation.md).
