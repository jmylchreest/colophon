---
name: colophon-edit
description: Revise an existing colophon blog post — retitle, expand a section, tighten, or fix — while preserving the author's voice and the frontmatter contract. Use when the user asks to edit, rewrite, expand, shorten, or fix an existing post on their colophon site.
---

# Edit a colophon post

Revise an existing entry in place, keeping its voice and its frontmatter intact. colophon
locates the entry and supplies the voice; you do the editing.

## When to use

The user wants to change an existing post in a colophon project (not create a new one).

## Workflow

1. **Find the entry.** List entries to get the source file and metadata:
   ```sh
   colophon posts                       # slug, title, type, author, persona, tags
   colophon posts --author <a> --tag <t>  # narrow it
   ```
   Match the user's description to a row; the file is under the source's directory (see
   `colophon sources`). Read the file.

2. **Re-load the voice** so edits sound consistent with the rest of the post:
   ```sh
   colophon persona context <persona-from-frontmatter> --topic "<the post's subject>"
   ```
   If the post has no `persona:`, infer the voice from the post itself and similar posts.

3. **Edit the file in place.** Preserve:
   - the `slug:` (pinned — changing it breaks the URL; use `aliases:` for intentional moves),
   - the existing frontmatter fields you're not explicitly changing (byline `author:`, `date:`),
   - the author's voice, tense and formatting conventions.
   Make the smallest change that satisfies the request. For new cross-links, use `[[wikilinks]]`.

4. **Preview the change.**
   ```sh
   colophon serve --open=<slug>     # opens that post; or --open=latest
   ```

## Guardrails

- Don't rewrite more than asked. Don't silently change the title's slug or the byline.
- Keep the post's draft/publish state unless the user asks to change it.
- If the edit affects metadata (description/SEO/tags), hand off to `colophon-metadata`.
