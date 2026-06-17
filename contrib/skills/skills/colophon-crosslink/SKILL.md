---
name: colophon-crosslink
description: Add cross-references between colophon posts — wikilinks and "related posts" — so entries connect to each other. Use when the user asks to link posts together, add related/see-also links, or wire up internal references on their colophon blog.
---

# Cross-link colophon posts

colophon resolves `[[wikilinks]]` across every source at build time (an unresolved link
degrades to plain text). Use them to connect posts instead of inventing URLs.

## When to use

The user wants to add internal links / related-post references between existing entries.

## Requirements

This skill drives the `colophon` CLI. Before the first command, confirm it's installed:

```sh
command -v colophon || echo "colophon not found"
```

If it's missing, **stop and offer** the install — don't install it silently:
`go install github.com/jmylchreest/colophon/cmd/colophon@latest` (or a release binary). Proceed
only once it's on `PATH`.

## Workflow

1. **List the candidate targets** (and their tags, to find relevant ones):
   ```sh
   colophon posts --json                 # every entry: slug, title, tags, type
   colophon posts --tag <topic>          # entries on a related topic
   ```

2. **Pick genuinely related entries** — overlapping tags or subject matter, not every post.
   Prefer 2–5 strong links over a dump.

3. **Add the links** in the source file(s):
   - Inline, where the text references another post: `… as covered in [[Raft leader election]] …`
     (link by the target's title or slug; `[[title|custom text]]` sets the anchor text).
   - Or a trailing section:
     ```markdown
     ## Related
     - [[Raft leader election]]
     - [[Designing for partition tolerance]]
     ```

4. **Verify they resolve.** Rebuild and check none fell back to plain text:
   ```sh
   colophon serve --open=<slug>
   ```
   An unresolved `[[link]]` means the target title/slug didn't match — fix the reference.

## Guardrails

- If `colophon` isn't installed, surface the install command and ask — never install it silently.
- Make link text describe the destination (the target's title/topic), never "click here" or a
  bare slug — this is both better UX and an accessibility requirement.
- Only link to entries that actually exist (from `colophon posts`). Don't invent targets.
- Don't over-link; relevance over volume. Keep edits minimal and preserve voice.
