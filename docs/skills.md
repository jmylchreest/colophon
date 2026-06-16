# Agent skills & prompt packs (design)

> **Status: design.** This describes the planned authoring skills and the prompts colophon
> would furnish them with. The *contracts* they target — the frontmatter schema
> ([SEO](seo.md), tags, persona) and the `markdown.Document` round-trip — already exist.
> The skills themselves are not built yet.

## The model

colophon is the **context provider**; the agent (LLM) does the writing. A skill is a small,
deterministic wrapper that:

1. gathers **context** — the article body, the persona's style guide + retrieved corpus
   exemplars, and the site's facts (title, base_url, existing tags),
2. furnishes a **prompt pack** — a system prompt with a best-practice rubric and the output
   schema,
3. forces **structured output** — the model returns a validated object (a frontmatter block
   or body fragment), never free text,
4. **merges** it back via `markdown.Document` so the body is byte-preserved and only the
   intended fields change.

The frontmatter schema is the contract: because every field has exactly one rendering
effect, the model can see the consequence of everything it writes, and nothing it can't set
affects the output.

## Shared infrastructure

| Piece | Role |
|-------|------|
| **Persona context** (`colophon persona context`) | Emits the persona's style guide + top-K corpus exemplars (BM25/embedding retrieval). Every writing skill prepends it so output matches the author's voice. |
| **Structured output** | Each skill defines a JSON Schema; the runtime validates and re-prompts on mismatch. |
| **`markdown.Document`** | Parse → patch frontmatter/body → re-marshal, preserving the body and unrelated fields. |
| **Surface** | A CLI verb (`colophon <skill> <file>`) and the matching MCP tool, sharing one implementation. |

## Skill catalogue

| Skill | Produces | Consumes |
|-------|----------|----------|
| **seo** | the `seo:` block | body + tags + persona |
| **draft** | body from a brief/outline | brief + persona context |
| **outline** | a heading skeleton | topic + persona |
| **expand** | fills a section | surrounding body + persona |
| **retitle** | `title` + `slug` candidates | body |
| **tag** | `tags` suggestions | body + the site's existing tag vocabulary |
| **social** | `seo.social` + syndication copy | body + target network |
| **alt-text** | `![alt]` for images/embeds | the image + nearby text |
| **summary** | `description` / TL;DR | body |

All are **suggest-by-default**: they write a block you review, never silently overwrite
editorial fields. `--apply` patches in place.

---

## Prompt pack: `seo`

The flagship, since its contract ([seo.md](seo.md)) and templating now exist.

**Inputs furnished**

- the rendered article text (HTML stripped),
- the resolved page facts: site title, `base_url` + slug (→ canonical), date, existing tags,
- persona style guide (so the title/description sound like the author, not generic SEO mush).

**Output schema** — the `seo:` object (`title`, `description`, `keywords`, `canonical`,
`noindex`, `image`, `type`, `social{title,description}`). Forced via structured output.

**System prompt (sketch)**

```
You write SEO metadata for a blog post, in the author's voice (style guide below).
Return ONLY the seo object. Follow these rules:

- title: ≤60 characters. Front-load the primary keyword. Match the post's actual content
  and search intent. Voice = the author's, not clickbait.
- description: 140–160 characters. A genuine summary that earns the click; no teasing,
  no "in this post". Unique to this page.
- keywords: 4–8 focus terms a reader would actually search; no stuffing.
- social.title / social.description: only if a punchier share-optimised version helps;
  otherwise omit and the search copy is reused.
- canonical / noindex / image / type: set only when you have a specific reason; otherwise
  omit and colophon's defaults apply.

Never invent facts not in the article. Prefer the author's existing tags as keyword seeds.
```

**Inputs block (sketch)**

```
## Author style guide
{{ persona.style.guide }}

## Site
title: {{ site.title }}   url: {{ canonical }}   existing tags: {{ all_tags }}

## Article
{{ body_text }}
```

The model returns e.g.:

```yaml
seo:
  title: "Rendering math, diagrams and code from one Markdown file"
  description: "How colophon turns a single note into a page with KaTeX, Mermaid and
    highlighted code — and degrades to readable text without JavaScript."
  keywords: [static site generator, markdown, katex, mermaid, progressive enhancement]
  social:
    title: "One Markdown file → math, diagrams, code"
```

`colophon seo --apply post.md` merges that under `seo:`, body untouched; the next build
renders the canonical/OG/Twitter/JSON-LD from it.

---

## Prompt pack: `draft` / `outline` / `expand`

The writing skills. Each prepends **persona context** so output is in-voice, and takes a
brief or the surrounding body.

**`outline`** — input: a topic + angle. Output: a heading tree (`##`/`###`) with one-line
intents per section. Rubric: match the persona's typical structure; no body prose yet.

**`draft`** — input: an outline (or brief) + persona context. Output: the markdown body.
Rubric: the author's voice and formatting conventions (callouts, code fences, length);
cite only what's in the references; leave `[[wikilink]]` placeholders for cross-links rather
than inventing URLs.

**`expand`** — input: a section heading + the surrounding body. Output: that section's prose
only. Rubric: continuity with the existing voice and tense; no repetition of nearby points.

---

## Prompt pack: `tag`, `social`, `alt-text`, `summary`

- **tag** — input: body + the **site's existing tag vocabulary**. Output: 3–6 tags, *reusing
  existing tags where they fit* (avoid near-duplicate taxonomy), only proposing new ones when
  warranted. This keeps tag pages ([content.md](content.md)) coherent.
- **social** — input: body + target (Mastodon/Bluesky/X/LinkedIn). Output: `seo.social` plus
  a per-network post for `syndicate`. Rubric: each network's norms (length, hashtags, link
  placement) and the author's voice.
- **alt-text** — input: an image + the paragraph around it. Output: concise, descriptive alt
  text (not "image of"), for accessibility and image SEO.
- **summary** — input: body. Output: a `description` (and optionally a longer TL;DR callout).

---

## Why this shape

- **One contract, many skills.** Every skill writes into the same typed frontmatter the
  templates already render, so adding a skill never needs a templating change.
- **Voice-preserving.** Persona context is the common prefix, so SEO copy, drafts and social
  posts all sound like the same author.
- **Reviewable & reversible.** Structured output + `markdown.Document` round-trip means a
  skill patches exactly the fields it owns and nothing else; suggest-by-default keeps a human
  in the loop for editorial fields.
