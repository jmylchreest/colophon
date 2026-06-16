---
name: colophon-publish
description: Preview, validate, and publish a colophon site — run doctor, build, serve a local preview, and deploy to the configured publishers. Use when the user asks to preview, build, check, deploy, or publish their colophon blog.
---

# Preview & publish a colophon site

Drive colophon's operational commands. Deploy is **gated and explicit**, and deploy secrets
are read from the environment by colophon itself — never handle or pass them.

## When to use

The user wants to preview, validate, or deploy their colophon project.

## Workflow

1. **Validate the project** first:
   ```sh
   colophon doctor          # checks config, sites, publishers, environments
   colophon env             # lists the env vars this project needs (incl. deploy secrets)
   ```
   If `doctor` reports problems, fix them before deploying. If `env` lists unset secrets needed
   by a target, tell the user to set them in their shell — **do not** put secrets in config.

2. **Preview locally** (drafts included where an environment enables them):
   ```sh
   colophon serve --open=latest    # opens the newest post; prints home/sitemap/feed URLs too
   ```
   Review the output with the user. A build also prints any warnings (empty posts, slug
   collisions) — surface those.

3. **Publish — only when the user explicitly approves.** Pick the environment:
   ```sh
   colophon publish --env production
   ```
   - Gated environments (`allow_publish: false`) refuse without `--allow-publish` — only add it
     when the user says so.
   - `--create` provisions a missing destination (e.g. a Pages project / R2 bucket).
   - Publishing is incremental (only changed files upload; orphaned ones are pruned) and reports
     what it deployed.

4. **Report back** the deployed URL(s) from the publish summary.

## Guardrails

- Never deploy without explicit user approval; never pass `--allow-publish` on your own.
- Deploy secrets are env-only — colophon reads them; you never read, log, or move them.
- Drafts/embargoed posts stay out of production builds automatically — don't force them in.
