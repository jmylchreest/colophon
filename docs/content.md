# Authoring content

A colophon post is a Markdown file with a YAML *frontmatter* block. Files come from one or
more **sources** (a `content/` folder, an Obsidian vault, …); the source's folder structure
becomes the site's URL structure.

```markdown
---
title: My first post
date: 2026-06-15
description: A one-line summary for feeds and link previews.
tags: [notes, colophon]
draft: false
---

Write the body in **Markdown**.
```

## Frontmatter fields

All fields are optional unless noted.

| Field | Meaning |
|-------|---------|
| `title` | Post title. If omitted, falls back to a leading `# heading` or the file name. |
| `date` | Publish date (`YYYY-MM-DD`). If omitted (Obsidian), the file's modified time is used. |
| `type` | Page type (`post`, `page`, or a custom value). Overrides the date-based default — see [Page types](#page-types). |
| `slug` | Overrides the final URL segment (otherwise derived from the file path). |
| `description` | Summary for feeds, `<meta name="description">` and `og:description`. |
| `tags`, `categories` | Lists for organisation/feeds. |
| `author` | The byline — an `authors/<id>.yaml` id. Defaults to the first author, else "Anonymous". See [Authors & personas](personas.md). |
| `persona` | The hidden writing *voice* (a `personas/<id>.yaml` id) used by the agent; never shown. |
| `hero` | Banner image shown at the top of the post. A path or an Obsidian `"[[image.png]]"`. |
| `image` | Preview/social-card image (`og:image` + index thumbnail). |
| `hero_alt`, `image_alt` | Alt text for those images. Empty = decorative (`alt=""`); set it when the image carries meaning. |
| `hero_fit`, `image_fit` | How the image fills its box — CSS `object-fit`: `cover` (crop, default), `contain` (letterbox), `fill`, `scale-down`, `none`. |
| `hero_position`, `image_position` | Which part shows when cropping — CSS `object-position`, e.g. `top` or `50% 20%`. |
| `lang` | Per-post language (BCP-47, e.g. `fr`), overriding the site `lang`. Emitted as `<html lang>`. |
| `glossary` | `false` turns off automatic [glossary](#glossary) decoration for this post; an explicit `<dfn>` still works. |
| `draft` | `true` keeps the post out of production builds (shown in preview/serve). |
| `publish` | Obsidian whitelist flag, honoured when a source sets `publish_required: true`. |
| `publish_after` | Embargo: not published until this time (ISO 8601, e.g. `2026-07-01T09:00:00Z`). |

Slugs are normalised: each path segment is lower-cased and non-alphanumerics collapse to
single hyphens, so `Archive/My Post.md` → `archive/my-post`.

## Page types

Every entry has a **type** that decides how it's placed and which theme template renders it:

- A **`post`** is chronological — listed on the index, included in feeds, and shown on its tag
  pages.
- A **`page`** is standing chrome — surfaced in the theme's nav menu instead, and kept out of
  the list and feeds (e.g. About, Now).

By default the type is inferred: an entry **with a date** is a `post`, one **without** is a
`page`. Set `type:` in frontmatter to override that, or to use a custom type:

```yaml
---
title: Side Projects
type: project        # a custom type; styled by a theme's project.html if it has one
---
```

- `type: page` makes a *dated* entry standing (nav, not feeds); `type: post` makes a *dateless*
  entry a listed post.
- A custom type (e.g. `project`) is listed like a post, but a theme can give it its own look —
  see [Themes → Page types](themes.md#page-types). This `type` is unrelated to `seo.type`
  (the schema.org type).

## Markdown support

colophon parses [GitHub Flavored Markdown](https://github.github.com/gfm/) — tables,
strikethrough, task lists, autolinks — plus automatic heading IDs (so `## My Heading` is
linkable as `#my-heading`).

### The raw-block contract (progressive enhancement)

Rich blocks are rendered as **semantic HTML that carries its raw source as text**, tagged by
type. colophon itself loads **no JavaScript** — it only guarantees the markup. A theme then
chooses how to present each block: a no-JS/minimal theme shows readable raw text; the default
theme upgrades it with [highlight.js](https://highlightjs.org/),
[KaTeX](https://katex.org/) and [Mermaid](https://mermaid.js.org/).

| You write | colophon emits | Enhanced by |
|-----------|----------------|-------------|
| ` ```go … ``` ` | `<pre><code class="language-go">…</code></pre>` | a syntax highlighter |
| ` ```mermaid … ``` ` | `<pre class="mermaid">…</pre>` | Mermaid |
| `$E=mc^2$` (inline) | `<span class="math math-inline">E=mc^2</span>` | KaTeX |
| `$$ … $$` (display) | `<div class="math math-display">…</div>` | KaTeX |
| `> [!note] Title` … | `<div class="callout callout-note">…</div>` | CSS only (no JS) |

Notes:

- **Maths** is matched on a single line. A currency heuristic leaves prose like `$5 and $10`
  alone. The LaTeX source is preserved verbatim, so it is readable even without KaTeX.
- **Callouts** use Obsidian syntax — a blockquote whose first line is `[!type] Optional
  Title`. The body is normal Markdown. Types map to colours via CSS classes
  (`note`/`info`, `tip`/`success`, `warning`, `danger`, `example`, …).
- **Mermaid** uses the diagram source as the element's text, so it degrades to a readable
  description without the library.

### Links, wikilinks and images

- Standard Markdown links and images work as usual.
- **Wikilinks** resolve across every source at build time — a vault note can link to a post
  in `content/` and vice versa: `[[note]]`, `[[note|alias]]`, `[[note#heading]]`. An
  unresolved link degrades to plain text rather than breaking.
- **Tags** (`tags:` frontmatter) render on each post and on the index, linked to a generated
  page per tag at `/tags/<tag>/` that lists every post sharing it — so tags become sideways
  navigation across entries.
- **Embeds** (`![[image.png]]`, `![[image.png|alt]]`) resolve attachments vault-wide and are
  copied next to the page.
- `![](relative.png)` images are copied beside the page so the relative `src` resolves;
  external (`https://…`) images are left untouched.

### Images and object storage

By default images are co-located with the page and served relatively. A site can **route**
images (or any path glob) to an object store (e.g. Cloudflare R2) instead — see the publisher
configuration. When routing is active the build rewrites those image URLs to the store's
public base, so the page references `https://assets.example.com/…` while the bytes are
uploaded to the store rather than your HTML host.

## Glossary

Drop a `glossary.yaml` (term → definition) at the project root and colophon publishes it as
`glossary.json`; a JS-enabled theme then **automatically** decorates the first occurrence of
each term in your prose with an accessible pop-over (a "dictionary stanza" with the term and
its definition). It is never rendered as a page, and it degrades gracefully — the text-only
theme just shows the words plain.

```yaml
# glossary.yaml
API: "Application Programming Interface — the contract one program exposes for another to call."
SSG: "Static Site Generator — renders content into static HTML served as-is."
```

You write naturally — no markup needed. When you *do* want control over a specific word, three
controls are available (the syntactic sugar):

| You want… | Write… | Effect |
|-----------|--------|--------|
| Turn the whole post off | `glossary: false` in frontmatter | No automatic matching. Explicit forces below still work. |
| **Force** a specific word | `<abbr>API</abbr>` | Always decorated, even mid-post or in an opted-out post — the same `<abbr>` auto-match produces. (`<dfn>` also works.) An `<abbr title="…">` you write yourself is left alone. |
| **Suppress** one word | `<span class="no-gloss">Go</span>` | That occurrence is left alone (use it when a term is also a common word). |

Decoration always skips code, links, headings and existing abbreviations, and only the **first**
occurrence of a term is auto-decorated, so a post is never peppered with repeats.

## Sources

- **`md-dir`** — a directory of Markdown files (default: `content/`).
- **`obsidian`** — an Obsidian vault, read in place. By convention it publishes only notes
  with `publish: true` (unless the source sets `publish_required: false`), derives a missing
  title from a leading `# heading` or the file name, and a missing date from the file's
  modified time.

Multiple sources are merged into one site; deletions and renames flow through the build's
reconciliation, so the output always matches the inputs.
