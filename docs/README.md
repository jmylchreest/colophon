# colophon documentation

End-user guides for authoring, theming and publishing a colophon site.

- **[Authoring content](content.md)** — frontmatter, supported Markdown, the rich blocks
  (maths, diagrams, callouts, code) and how they render, wikilinks, embeds and images.
- **[Themes](themes.md)** — selecting a theme, per-environment theme overrides, writing or
  overriding a theme, the template variables, and progressive enhancement.
- **[SEO & social](seo.md)** — the `seo:` frontmatter block and the canonical / Open Graph /
  Twitter / JSON-LD metadata colophon emits.
- **[Image & audio generation](image-generation.md)** — `gen:` image prompts, AI or recorded
  post audio (podcast feeds), providers, the `--generate-ai` step, the kill switch, and pruning.
- **[Authors & personas](personas.md)** — the **author** (the shown byline + h-card) vs the
  **persona** (a hidden, shareable writing voice), and the `persona context` command that emits
  write-as context (style guide + relevant exemplars) for an AI author.
- **[Publishing](publishing.md)** — environments vs publishers, credentials, and routing
  assets to an object store.
- **[Syndication (POSSE)](syndication.md)** — cross-post to Mastodon/Bluesky/anywhere with
  `colophon syndicate`: the ledger, gating, and the `command`/`mastodon`/`bluesky`/`bridgy` drivers.
- **[Agent skills](skills.md)** *(design)* — the planned authoring skills (seo, draft, tag,
  social…) and the prompt packs that drive them.

- **[How-to guides](howto/)** — short zero-to-published recipes: federate via Bridgy Fed, show
  webmentions, syndicate to Mastodon/Bluesky. Each notes whether it's shipped or planned.

Design notes and the roadmap live in [PLAN.md](PLAN.md) and [design/](design/).
