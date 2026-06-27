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

## Split algorithm (derived)

Walk the rendered body; start a new slide at a **boundary**:

1. the **shallowest heading level present** in the body (H1 if any H1 exists, else H2); or
2. a thematic break (`---` / `<hr>`); or
3. an explicit `<newslide>` marker (see below).

For each slide:

- the **split-level heading** is the slide **title**;
- the **next heading level down** (H2 or H3) becomes the slide's **bullets** (`<ul><li>`); deeper
  headings nest as sub-bullets;
- **block content** in the section — tables, images, code, display math, Mermaid, callouts,
  pull-quotes, blockquotes — is rendered **inline on the slide as-is** (the theme/reader handles
  layout and fit; no build-time sizing logic for now);
- **prose paragraphs** go to the slide's **notes**, *not* the slide — unless wrapped in `<slide>`
  (see overrides).

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

The `<newslide>` marker is the **one unifying break mechanism**: heading/`---` splits emit the same
section boundary the author would with a manual `<newslide>`.

## Author overrides (inline markup, family of `<tts>`/`<noslide>` style)

- `<newslide>` — force a slide break anywhere (also the engine's internal break marker).
- `<slide>…</slide>` — force content **onto** the slide that wouldn't be there by default (e.g.
  pull a specific paragraph/sentence onto the slide for context).
- `<noslide>…</noslide>` — force content **off** the slide (stays in the post and the notes, but
  not on the slide).

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
