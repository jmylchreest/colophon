# Design (DRAFT, temporary): Themed presentations / decks

Status: design only — no engine code yet. Scope agreed in conversation; this captures it so we
build against a fixed contract. Move to `docs/design/` when it firms up.

## Overview

Generate a **themed slide deck from existing post content** — a structural *projection* of a
post (or a standalone content file), not a separate authoring format. The engine emits semantic
HTML (one `<section>` per slide) and ships a JS **deck reader** (an engine asset, like
`player.js`/`glossary.js`); the active theme restyles the slides. The deck is **derived only** —
there is no separate "authored deck" markdown. Author intent is expressed with a few inline hints.

The guiding inversion: **slides are cues, speaker notes are the script.** Slides carry headings,
bullets, figures, math and diagrams; ordinary prose becomes the notes.

## Scope

In: derived decks from posts/standalone content; the split algorithm; the `<section>`/notes HTML
contract; author override markup; an engine `deck.js`/`deck.css`; reuse of the existing
math/mermaid/highlight/`gen:` pipeline and theme inheritance.

Deferred / out of scope (for now): speaker-note **summarisation** (notes = prose verbatim at
first); **own-slide vs alongside sizing** heuristics (the reader/theme handle layout); fragments /
incremental reveal; transitions beyond CSS; a native PDF exporter (use the browser's Print → PDF —
no Chromium dependency); an authored-deck mode.

## Activation & addressing

- **Attached to a post:** `slides: true` (or a `slides:` block) in frontmatter → the deck is
  published at `…/<slug>/slides/`, linked from the post. Same source, two artifacts.
- **Standalone:** `type: deck` → rendered as a deck at its own slug.

## Frontmatter config (`slides:`)

A post overrides deck defaults with a `slides:` block. **It overwrites by key, it does NOT deep-merge**
(unlike generation profiles): setting `slides.split` replaces the default split list wholesale.

```yaml
slides:                 # also: `slides: true` to enable with all defaults
  split: [h2]           # the slide boundaries (a LIST) — see targets below
```

### Split targets (`slides.split`, a list)

| target | splits before… |
|--------|----------------|
| `h1` … `h6` | a heading of that level (the heading becomes the slide title) |
| `hr` | a thematic break (`---` / `<hr>`) — no title |
| `splitslide` | an explicit `<splitslide>` marker — no title |
| `image` | a figure / image |
| `table` | a table |
| `code` | a fenced code block |
| `math` | a display-math block |
| `diagram` (`mermaid`) | a Mermaid diagram |
| `audio` / `video` | an embedded media player |
| `text:<match>` | a block whose text **begins with** `<match>` — split on a recurring marker rather than structure |

**Default:** `[h1, h2, h3, h4, h5, h6, hr, splitslide]` — every heading (any level) starts a new
slide. Narrow it to make deeper headings fold into **bullets** instead: `split: [h2]` puts each `h2`
on its own slide and turns the `h3`s under it into the slide's bullet list. The showcase uses
`split: [h2]` so it lands one slide per topic.

## Split algorithm (derived)

Walk the rendered body; start a new slide at any boundary in `slides.split` (default: every heading,
`<hr>`, `<splitslide>`). When the boundary is a heading it becomes the slide **title**; the block
kinds / `hr` / `splitslide` start an untitled slide.

For each slide:

- the **boundary heading** is the slide **title**;
- any **heading still inside the slide** (i.e. below the split level — `h3` under an `h2`-split)
  folds into the slide's **bullets** (`<ul class="slide-bullets"><li>`);
- **block content** in the section — tables, images, code, display math, Mermaid, callouts,
  pull-quotes, blockquotes — is rendered **inline on the slide as-is** (the theme/reader handles
  layout and fit; no build-time sizing logic for now);
- **prose paragraphs** go to the slide's **notes**, *not* the slide — unless inside an explicit
  `<slide>…</slide>` (see overrides).

Class-styled blocks (`.callout`, `.pullquote`, tables, …) are emitted unchanged and **styled by
the deck theme**, which **bases off the active theme** (existing base-inheritance), so they look
right for free.

## Output contract

The engine groups the body into one `<section class="slide">` per slide. Degrades to a **readable
long-form document with no JS** (the talk reads as an article — SEO + a11y for free). Shape:

```html
<section class="slide" aria-label="Slide 3: Pull-quotes" id="slide-3">
  <h2 class="slide-title">Pull-quotes</h2>
  <ul class="slide-bullets"><li>…</li></ul>
  <figure class="pullquote">…</figure>          <!-- blocks render inline, themed -->
  <aside class="notes">…the section's prose…</aside>  <!-- presenter notes, hidden in slide view -->
</section>
```

The `<splitslide>` marker is the **one unifying break mechanism**: heading/`---` splits emit the same
section boundary the author would with a manual `<splitslide>`.

## Author overrides (inline markup, `<slide>` family)

- `<splitslide>` — force a slide break anywhere (a void marker; also the engine's internal break).
- `<slide>…</slide>` — everything inside becomes **one verbatim slide**: an authored escape hatch
  from the derived split. A leading heading is its title; all other content (prose included) stays
  **on** the slide and is *not* pulled into notes.
- `<noslide>…</noslide>` — content dropped from the deck entirely (stays in the post, off both the
  slide and the notes).

## Engine assets & the reader

Shipped by the engine (consistent across themes; theme restyles only):

- **`deck.js`** — the reader: keyboard + swipe navigation, fit-to-viewport scaling, a `?presenter`
  view (current slide + next + notes + timer), URL/`#slide-N` deep links, and reuse of the existing
  math/mermaid/highlight hydration. (Fragments/transitions later.)
- **`deck.css`** — base slide layout/chrome; the active theme overrides tokens + look.

A JS reader **is essential** for the interactive experience — but it lives in the asset/theme
layer, so the **engine output stays markup-only and fully degrades**.

## WCAG AAA (required — see the `wcag-aaa-compliance` decision)

The deck — including the no-JS document — is AAA from the start:

- Semantic landmarks: `<section>` slides, `<aside class="notes">`, a `<nav>` for controls.
- **Keyboard-operable** navigation; a clearly **visible focus indicator** on every control.
- Slide changes announced via `aria-live` (e.g. "Slide 4 of 12: …"); focus moved to the new slide.
- `prefers-reduced-motion`: no slide transitions/animation when set.
- Text contrast ≥ 7:1; never convey state by colour alone; scale/zoom without clipping (the
  fit-to-viewport must not defeat user zoom of the notes/document view).
- Image `alt` carried through from markdown; figures keep captions.
- The presenter view and the printed/notes view are themselves accessible.

## Offline / downloadable deck

A presenter needs the deck to work **offline** (no network at the venue). Plan: a "Download"
control that produces a **single self-contained `.html`** — CSS, `deck.js`, KaTeX/Mermaid output
and images all **inlined** (CSS/JS embedded, images as `data:` URIs), so one file opens anywhere
with no server and no assets. The same file is the Print → PDF source. Two builds of the deck:
the normal published page (linked assets, cacheable) and an on-demand/downloadable inlined bundle.
Math/diagrams must be **pre-rendered** (or their libraries inlined) for the offline copy, since the
browser can't fetch KaTeX/Mermaid at the venue.

## Open questions

- How note **summarisation** plugs in later (reuse the audio/excerpt derivation? an AI pass?).
- Standalone-deck addressing/feed treatment (does a deck appear in lists/feeds, or is it a
  side-artifact like the audio reading?).
- Sizing/pagination ownership once we revisit it (reader-driven vs author hints).

## Decisions to record on build

- presentations-derived-only · presentations-output-contract · presentations-reader-asset
  (record via `aide decision set` when we commit to building).
