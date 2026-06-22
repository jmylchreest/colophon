# How to syndicate with a command (POSSE, any target)

> Status: **shipped.** The syndication harness — the ledger, gating, `--dry-run`, and the
> `command` driver — works today. Native `mastodon`/`bluesky` drivers are planned
> ([../design/federation.md](../design/federation.md)); the `command` driver lets you wire up any
> target now.

POSSE = Publish on your Own Site, Syndicate Elsewhere. The `command` driver runs a program of your
choice once per new post, so you can cross-post anywhere (a silo's CLI, a webhook, a notifier)
without a built-in driver. colophon records each result in a committed ledger, so re-runs never
double-post.

## Steps

1. **Write a command** that posts one entry. colophon passes the post as environment variables
   (`COLOPHON_POST_URL`, `_TITLE`, `_SUMMARY`, `_TEXT`, `_TAGS`, `_KEY`, `_PUBLISHED`) and as JSON
   on stdin. Print the **created URL** as the first line of stdout (or print nothing for
   fire-and-forget). A non-zero exit is a failure.
   ```sh
   #!/usr/bin/env bash
   # bin/post-to-silo — receives one post via env, prints the silo URL
   set -euo pipefail
   id=$(curl -fsS -X POST https://silo.example/api/posts \
          -H "Authorization: Bearer $SILO_TOKEN" \
          --data-urlencode "text=${COLOPHON_POST_TITLE} ${COLOPHON_POST_URL}" | jq -r .url)
   echo "$id"
   ```

2. **Configure a syndicator** (`driver: command`) and allow it on the env that should post:
   ```yaml
   sites:
     - id: main
       federation:
         syndication:
           - { id: silo, driver: command, command: "./bin/post-to-silo" }
   environments:
     - name: production
       syndicate: [silo]      # only this env cross-posts; preview/draft omit it → never post
   ```

3. **Preview, then post** (run after `publish`, so the canonical URL is live):
   ```sh
   colophon syndicate --env production --dry-run        # shows what would post, writes nothing
   colophon syndicate --env production --allow-publish  # posts new entries, records the ledger
   ```

4. **Commit the ledger** (`.colophon/syndication.json`) — it's authoritative. Without it a fresh
   runner would re-post everything, so a real run refuses to start with no ledger unless you pass
   `--allow-publish` to seed it.

The recorded silo URLs render on each post as mf2 `u-syndication` "Also posted on…" links.

## Notes

- **Safety:** only an env's `syndicate:` targets fire; a gated env (`allow_publish: false`) needs
  `--allow-publish`; `--dry-run` never posts or writes. Post content is passed via env/stdin, never
  interpolated into the command, so it can't inject shell.
- **Per post:** `syndicate: false` to skip one, `syndicate: [silo]` to choose targets,
  `syndicate_text:` for a custom blurb.
- **Secrets** (like `SILO_TOKEN`) come from the environment, never the config.
- Prefer a managed native account? The `mastodon`/`bluesky` drivers (planned) will hold the auth
  for you; until then `command` covers any target.
