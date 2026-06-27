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
| `aliases` | Old/alternate URL paths that redirect here (e.g. after a rename) — see [Redirects](#redirects-aliases). |
| `description` | Summary for feeds, `<meta name="description">` and `og:description`. |
| `tags`, `categories` | Lists for organisation/feeds. |
| `author` | The byline — an `authors/<id>.yaml` id. Defaults to the first author, else "Anonymous". See [Authors & personas](personas.md). |
| `persona` | The hidden writing *voice* (a `personas/<id>.yaml` id) used by the agent; never shown. |
| `hero` | Banner image shown at the top of the post. A path, an Obsidian `"[[image.png]]"`, or a `"gen:<prompt>"` to generate one — see [Image generation](image-generation.md). |
| `image` | Preview/social-card image (`og:image` + index thumbnail). Accepts a path or a `"gen:<prompt>"`. |
| `hero_alt`, `image_alt` | Alt text for those images. Empty = decorative (`alt=""`); set it when the image carries meaning. |
| `hero_fit`, `image_fit` | How the image fills its box — CSS `object-fit`: `cover` (crop, default), `contain` (letterbox), `fill`, `scale-down`, `none`. |
| `hero_position`, `image_position` | Which part shows when cropping — CSS `object-position`, e.g. `top` or `50% 20%`. |
| `audio` | Spoken (TTS) reading of the post. Omit to follow the site default (on when a speech provider is configured); set `true`/`false` to force it. Needs `generation.speech`. See [Image & audio generation](image-generation.md). |
| `audio_file` | Attach a pre-recorded audio file (a path or `[[embed]]`) instead of generating one — no AI. Wins over `audio`. |
| `audio_voice` | Override the reading voice id (generated audio only); else the author's/persona's `voice`, else the site default. |
| `attachments` | Downloadable files shipped with the post (scripts, archives, datasets, PDFs…). A list of paths or `{path, label, feed}` mappings — see [Attachments (downloads)](#attachments-downloads). |
| `syndication` | URLs where this post also lives (e.g. a Mastodon/Bluesky copy you cross-posted). A list of absolute URLs, rendered as mf2 `u-syndication` "Also posted on…" links. The `colophon syndicate` ledger also feeds these automatically. |
| `syndicate` | POSSE control for `colophon syndicate`: `false` opts this post out; a list (`[mastodon]`) picks a subset of the environment's targets; absent = all of them. |
| `syndicate_text` | Optional custom blurb for the syndicated copy (else the driver derives one from the title/summary). |
| `lang` | Per-post language (BCP-47, e.g. `fr`), overriding the site `lang`. Emitted as `<html lang>`. |
| `glossary` | `false` turns off automatic [glossary](#glossary) decoration for this post; an explicit `<abbr>` still works. |
| `draft` | `true` keeps the post out of production builds (shown in preview/serve). |
| `publish` | Obsidian whitelist flag, honoured when a source sets `publish_required: true`. |
| `publish_after` | Embargo: not published until this time (ISO 8601, e.g. `2026-07-01T09:00:00Z`). |
| `predecessor` | The slug (or bare filename) of the post that *immediately precedes* this one in a series — see [Post series](#post-series). |
| `series` | Optional series **title**. Latest-wins: the newest post in the chain that sets it names the series. |

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

## Post series

A post can declare that it follows an earlier post with a single **backward** link, and
colophon reconstructs the whole ordered series from those links — adding "Part N of M" /
previous / next navigation to **every** member.

```yaml
---
title: "Building a Widget, Part Two"
date: 2026-06-11
predecessor: building-a-widget-part-one   # the post just before this one
---
```

- **`predecessor:`** pins the slug (or bare filename, resolved like a `[[wikilink]]`) of the
  immediately preceding post. It's a single linear chain — a post is in at most one series.
- **`series:`** is the optional title. It's **latest-wins**: the name is taken from the *newest*
  post in the chain that sets it; if no member sets it, the series is untitled.

Because colophon rebuilds the whole site every time, a backward pointer is enough — **you never
edit old posts**. Publishing Part Two (which points back at Part One) is what gives Part One its
forward link to Part Two; the engine walks the chain and regenerates both. The series renders
oldest→newest, and the current post is highlighted in the list.

Themes get per-post variables (`series_name`, `series_total`, `series_index`,
`series_parts`, `series_prev`, `series_next`) — set only for posts in a series of two or more —
and a `series` flag on each post-list item. The bundled **press** theme shows the series in the
left rail and marks series entries on the index; other themes can adopt the variables.

`colophon doctor` warns (without failing) when a `predecessor:` doesn't resolve to a known post,
when the links form a cycle, or when two posts name the same predecessor (a branch).

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
| `> [!quote] Attribution` … | `<figure class="pullquote"><blockquote>…</blockquote><figcaption>…</figcaption></figure>` | CSS only (no JS) |

Notes:

- **Maths** is matched on a single line. A currency heuristic leaves prose like `$5 and $10`
  alone. The LaTeX source is preserved verbatim, so it is readable even without KaTeX.
- **Callouts** use Obsidian syntax — a blockquote whose first line is `[!type] Optional
  Title`. The body is normal Markdown. Types map to colours via CSS classes
  (`note`/`info`, `tip`/`success`, `warning`, `danger`, `example`, …).
- **Pull-quotes** are the `[!quote]` callout type — they render as a semantic `<figure>` with
  the text after `[!quote]` as the attribution `<figcaption>` (omit it for an unattributed
  quote). The **press** theme styles this as a large display epigraph; other themes can target
  `.pullquote`. Plain blockquotes (`>` without `[!quote]`) are unchanged.
- **Mermaid** uses the diagram source as the element's text, so it degrades to a readable
  description without the library.

> **Tip — preview every feature in your theme.** `colophon serve --showcase` injects a built-in
> `/showcase/` page (embedded in the binary, never written to your content) that renders every one
> of these blocks — callouts, pull-quotes, tables, maths, diagrams, media, attachments, glossary —
> in your active theme, with the source shown alongside. Handy when writing or styling a theme.

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
- **Generated images** — `![alt](<gen:a prompt here>)` produces the image with an AI provider
  and caches it. Wrap the prompt in `<…>` when it contains spaces. See
  [Image generation](image-generation.md).
- **Video & audio embeds** — an image embed whose target is a media file renders as a player,
  not a broken image. `![A short demo](demo.mp4)` becomes a `<video controls>`; `![](clip.mp3)`
  becomes an `<audio controls>`. The file is copied/routed exactly like an image (so object
  storage works the same), and the embed's alt text becomes the player's `aria-label`. See
  [Embedding video and audio](#embedding-video-and-audio).

### Images and object storage

By default images are co-located with the page and served relatively. A site can **route**
images (or any path glob) to an object store (e.g. Cloudflare R2) instead — see the publisher
configuration. When routing is active the build rewrites those image URLs to the store's
public base, so the page references `https://assets.example.com/…` while the bytes are
uploaded to the store rather than your HTML host.

### Embedding video and audio

There is **no new syntax** — use the markdown image embed you already know, pointing at a media
file. colophon recognises the extension and renders a player instead of an `<img>`:

```markdown
![A short demo](demo.mp4)     <!-- → <video controls>, with the alt as its aria-label -->
![[demo.mp4]]                  <!-- Obsidian embeds work too -->
![](interview.mp3)            <!-- → <audio controls> -->
```

- **Video**: `.mp4`, `.webm`, `.mov`, `.m4v`, `.ogv`. **Audio**: `.mp3`, `.m4a`, `.aac`,
  `.oga`, `.ogg`, `.wav`, `.flac`, `.opus`.
- The file is discovered, copied beside the page, and **routed to object storage** exactly like
  an image — self-hosting "just works", including via R2.
- A direct **external** file URL plays too (e.g. `![](https://cdn.example.com/clip.mp4)`); it is
  left untouched and not copied.
- This is independent of `audio_file:`/`audio:`, which attach a single *podcast-style reading* of
  the whole post (with the themed player and feed enclosure). Inline embeds are just media in the
  body.

> Big files belong in object storage or a CDN, not your Git host. Route a `**/*.mp4` glob to R2
> (see the publisher config) and the embed URL is rewritten automatically.

### Attachments (downloads)

List downloadable files in frontmatter and colophon copies/routes them like images and renders a
**Downloads** block on the post. Each entry is either a bare path or a `{path, label, feed}`
mapping:

```yaml
---
title: Release Notes
attachments:
  - changelog.txt                                   # label defaults to the file name
  - { path: build.sh, label: "Build script", description: "Sets up the toolchain" }
  - { path: dataset.zip, label: "Dataset", description: "Raw measurements", feed: true }
---
```

- Paths resolve **relative to the post** (same rules as an image embed); `[[embed]]` works too.
- `label` sets the link text (defaults to the file name); `description` adds a one-line note
  beneath it. The file's **size** and a short **filetype** badge (ZIP, PDF, MP4…) are shown
  automatically.
- `feed: true` also lists the file as a feed enclosure/attachment (see below). Without it, the
  file is downloadable on the page but stays out of the feeds.
- Posts with attachments get a small paperclip marker in the listing (alongside the audio
  speaker), in the press and contrib themes.

**Attachments in feeds.** A post's audio reading and any `feed: true` attachment are emitted to
the syndication feeds so podcast/feed clients can fetch them:

- **JSON Feed** — every item appears in `attachments` (multiple allowed).
- **Atom** — each is a `<link rel="enclosure">` (multiple allowed).
- **RSS** — carries a single `<enclosure>` per the spec: the audio reading wins, else the first
  `feed: true` attachment.

### Slide decks

A post can be projected into a **themed slide deck** — published at `…/<slug>/slides/`, linked from
the post's Downloads box, and flagged with a slides marker in the listing (alongside the audio and
attachment markers). It's **derived** from the post: headings become slides (or bullets), prose
becomes speaker notes, and other blocks render on the slide. With JavaScript it's a keyboard/swipe
presentation (<kbd>P</kbd> = presenter notes, <kbd>F</kbd> = fullscreen); with JS off the same file
reads as a long-form document.

Set the site default in `colophon.yaml` (`slides.enabled`), then opt a post in or out:

```yaml
---
title: A Short Talk
slides: true                 # or the block form below
# slides:
#   enabled: true
#   split: [h2]              # slide boundaries (a list). default: every heading.
---
```

- **`split`** lists the boundaries: `h1`–`h6`, `hr`, `splitslide`, the block kinds `image`/`table`/
  `code`/`math`/`diagram`/`audio`/`video`, and `text:<match>` (split before a block whose text begins
  with the match). The default splits on every heading; narrow it (e.g. `[h2]`) to fold deeper
  headings into bullets.
- The post's `slides:` **overwrites** the site default by key (it does not deep-merge): a key you set
  replaces that value, keys you omit inherit.
- A site-wide `slides.enabled: true` applies to **listed content** (posts and custom types); standing
  **pages** (About, etc.) don't get a deck from the default — they opt in with their own `slides: true`.
- An **environment** can override the site default (`slides: { enabled, split }` under the environment),
  e.g. decks **on in preview, off in production**.
- Three inline markers mirror the `<tts>` family: `<splitslide>` forces a break, `<slide>…</slide>`
  makes one verbatim slide, and `<noslide>…</noslide>` stays in the post but is kept out of the deck.

### How file references resolve

Two kinds of reference resolve differently:

- **Per-post references** — markdown embeds/images, and a post's `hero`/`image` — resolve
  against *that post's own source* (its driver's rules: a vault searches its scan roots and the
  vault, an `md-dir` resolves dir-relative). They stay driver-relative so a missing embed is a
  real error, not silently masked by another source.
- **Project-level references** — an author `avatar` — resolve across *every* content source and
  then fall back to the **project root**. The same `avatar: assets/me.png` therefore works whether
  the file lives in a content dir, a vault, or the project's own `assets/` — portable across
  drivers.

`colophon doctor` dry-resolves every *defined* reference through the same machinery and warns when
one can't be sourced (a likely broken link). An *undefined* reference is fine — it just means none
was wanted. `data:`/`http(s)://` references always pass through untouched.

## Redirects (aliases)

When you rename a post (or want short links), list the old paths in `aliases:` so the old URLs
keep working:

```yaml
---
title: A Renamed Post
slug: renamed
aliases:
  - old-name            # /old-name/        → /posts/renamed/
  - 2020/legacy-post    # nested paths fine
---
```

Each alias is normalised like a slug (lower-cased, non-alphanumerics → hyphens, `/` kept). For
each, the build emits:

- a **meta-refresh stub** at `<alias>/index.html` → the post (works on *any* static host),
- a line in a root **`_redirects`** file, and
- a root **`.nojekyll`** (so GitHub Pages serves the stubs and the `_search/` index).

How that becomes a redirect depends on the host: **Cloudflare Pages, Netlify and GitLab Pages**
read `_redirects` and serve a real **301**; **S3 static-website** hosting gets a 301 too (colophon
sets the object redirect header on publish); plain object stores (R2, bare S3/MinIO) and GitHub
Pages fall back to the client-side meta-refresh stub. Either way the old URL resolves.

**Collisions** are resolved deterministically with a warning: an alias that matches a real page is
ignored (the page wins), and if two posts claim the same alias the newest wins.

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
| Turn the whole post off | `glossary: false` in frontmatter | No automatic matching. Explicit `<abbr>` forces still work. |
| **Force** a specific word | `<abbr>API</abbr>` | Always decorated, even mid-post or in an opted-out post — the same `<abbr>` auto-match produces. An `<abbr title="…">` you write yourself is left alone. |
| **Suppress** one word | `<noabbr>Go</noabbr>` | That occurrence is left plain (use it when a term is also a common word). The mirror of `<abbr>`. |

Decoration always skips code, links, headings, your own `<abbr title="…">` and anything inside
`<noabbr>`, and only the **first** occurrence of a term is auto-decorated, so a post is never
peppered with repeats.

## Sources

- **`md-dir`** — a directory of Markdown files (default: `content/`).
- **`obsidian`** — an Obsidian vault, read in place. By convention it publishes only notes
  with `publish: true` (unless the source sets `publish_required: false`), derives a missing
  title from a leading `# heading` or the file name, and a missing date from the file's
  modified time.

Multiple sources are merged into one site; deletions and renames flow through the build's
reconciliation, so the output always matches the inputs.
