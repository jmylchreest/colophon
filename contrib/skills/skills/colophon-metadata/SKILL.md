---
name: colophon-metadata
description: Fill or improve a colophon post's metadata — the description, the seo: block (title/description/keywords/social), and tags reusing the site's existing vocabulary. Use when the user asks to add SEO, a summary/description, social copy, or tags to a post.
---

# colophon post metadata

Populate the frontmatter that drives search, social cards and tag pages. Every field has one
rendering effect (`docs/seo.md`), so set only what helps and let colophon's defaults cover the
rest.

## When to use

The user wants better SEO/social metadata, a description, or tags on an existing post.

## Requirements

This skill drives the `colophon` CLI. Before the first command, confirm it's installed:

```sh
command -v colophon || echo "colophon not found"
```

If it's missing, **stop and offer** the install — don't install it silently:
`go install github.com/jmylchreest/colophon/cmd/colophon@latest` (or a release binary). Proceed
only once it's on `PATH`.

## Workflow

1. **Read the post** and learn the **existing tag vocabulary** (so you reuse tags, keeping tag
   pages coherent rather than spawning near-duplicates):
   ```sh
   colophon posts --json     # every entry's tags → the site's vocabulary
   ```

2. **Write `description`** if missing — a genuine 140–160-char summary that earns the click (no
   "in this post", no teasing). It feeds the meta description, `og:description` and feeds.

3. **Fill the `seo:` block** only where it adds value (see `docs/seo.md` for each field):
   ```yaml
   seo:
     title: "≤60 chars; front-load the primary keyword; the author's voice, not clickbait"
     description: "140–160 chars; unique to this page"
     keywords: [4–8 terms a reader would actually search]   # seed from existing tags
     social:
       title: "punchier share-optimised title"              # only if it beats the search title
   # canonical / noindex / image / type: set only with a specific reason; else omit (defaults apply)
   ```

4. **Tags** — propose 3–6, **reusing existing tags** where they fit; only add a new tag when
   warranted. Update the post's `tags:`.

5. **Preview** and confirm the rendered head:
   ```sh
   colophon serve --open=<slug>
   ```

## Guardrails

- If `colophon` isn't installed, surface the install command and ask — never install it silently.
- Suggest-by-default: show the block, don't silently rewrite editorial fields the user owns.
- Never invent facts. Don't stuff keywords. Prefer the author's existing tags as seeds.
- `seo.type` is the schema.org type — unrelated to the post's `type:` (post/page).
