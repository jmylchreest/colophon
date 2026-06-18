---
name: colophon-metadata
description: Fill or improve a colophon post's metadata — the description, the seo: block (title/description/keywords/social), tags reusing the site's existing vocabulary, and (rarely) a glossary entry for genuinely obscure jargon. Use when the user asks to add SEO, a summary/description, social copy, tags, or a glossary definition to a post.
---

# colophon post metadata

Populate the frontmatter that drives search, social cards and tag pages. Every field has one
rendering effect (`docs/seo.md`), so set only what helps and let colophon's defaults cover the
rest.

## When to use

The user wants metadata — a description, the `seo:` block, tags, or a glossary entry — on a post:
either a new one as the final pass of `colophon-write`, or an existing one on its own.

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

5. **Glossary — rarely, and only when it clears a high bar.** colophon's optional
   `glossary.yaml` (project root, `term: definition`) auto-decorates the *first* use of a term
   across the whole site. Drive the candidate from **this article's jargon**, and add an entry
   only when a term is **both**:
   - **uncommon and specific** — jargon a typical reader of this post wouldn't know. Skip common
     tech words, well-known acronyms, and anything already clear from context.
   - **definable with 100% accuracy** — write the definition from the article's own usage, not a
     guess. If you are not certain it is correct, do **not** add it.

   Keep it sparse: most posts add nothing; one or two terms at most. Reuse an existing entry
   rather than adding a near-duplicate, and never redefine a term whose meaning differs here.
   ```yaml
   # glossary.yaml (project root) — shared site-wide
   BM25: "A ranking function search engines use to score how well a document matches a query."
   ```
   A post opts out entirely with `glossary: false` in its frontmatter.

6. **Preview** and confirm the rendered head:
   ```sh
   colophon serve --open=<slug>
   ```

## Guardrails

- If `colophon` isn't installed, surface the install command and ask — never install it silently.
- Suggest-by-default: show the block, don't silently rewrite editorial fields the user owns.
- Never invent facts. Don't stuff keywords. Prefer the author's existing tags as seeds.
- `seo.type` is the schema.org type — unrelated to the post's `type:` (post/page).
- Glossary entries are shared site-wide and shown to every reader: add them sparingly, only for
  jargon specific to the article, and only when the definition is certainly correct. When in
  doubt, leave it out — a missing entry is better than a wrong or redundant one.
