---
name: colophon-write
description: Write a new blog post (or page) for a colophon site in a chosen author's byline and persona's voice. Use when the user asks to draft, write, or create a new post/article/entry for their colophon blog. colophon supplies the voice context and scaffolds the file; you write the prose.
---

# Write a colophon post

colophon is a **context provider** — it gives you the voice, tells you where to write, and
scaffolds the file. **You** write the prose. colophon never generates content and you never
pass it credentials.

## When to use

The user wants a new post/page for a colophon project (a directory with a `colophon.yaml`).

## Workflow

1. **Confirm the byline and voice.** List what exists and pick (ask the user if unsure):
   ```sh
   colophon authors          # the bylines (who it's published as)
   colophon personas list    # the writing voices (e.g. "Senior engineer")
   ```
   The **author** is the shown byline; the **persona** is the hidden writing voice (optional).

2. **Pull the voice + exemplars.** This is how you match the author's style:
   ```sh
   colophon persona context <persona> --topic "<what the post is about>"
   ```
   It returns the persona's style guide, references, and the most relevant past posts (BM25).
   Read these — write in that voice and reuse its conventions (callouts, code, length).

3. **See where it'll be written** (and how a post is marked live):
   ```sh
   colophon sources
   ```

4. **Scaffold the file.** This validates the author/persona, picks a unique pinned slug, and
   writes a frontmatter skeleton — it does *not* write the body:
   ```sh
   colophon new post "Raft leader election" --author <author> --persona <persona> --tag <tags>
   # → wrote: content/posts/raft-leader-election.md   url: /posts/raft-leader-election/
   ```
   Use `--in <source>` to target a specific source; `colophon new page` for a standing page.

5. **Write the body** into that file, in the persona's voice. Frontmatter field meanings are in
   `docs/content.md` (and `docs/seo.md` for the `seo:` block) — don't change the pinned `slug:`.
   Leave `[[wikilinks]]` where you'd cross-link to other posts rather than inventing URLs (the
   `colophon-crosslink` skill resolves them). Keep `draft: true` until the user approves.

6. **Preview.**
   ```sh
   colophon serve --open=latest      # opens the newest post; prints the URL too
   ```

7. **(Optional)** Improve tags/metadata with `colophon-metadata`, then publish with
   `colophon-publish`.

## Guardrails

- Don't invent facts not given by the user or the references. Don't fabricate links — use
  `[[wikilinks]]`.
- Keep posts `draft: true` until the user says to publish. Never deploy from this skill.
- If `colophon new` reports an unknown author/persona, fix the name (it lists the valid ids).
