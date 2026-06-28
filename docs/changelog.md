# Changelog

User-facing changes by release. Each entry points at the guide where the feature is documented
in full (or where it should be, when end-user docs catch up).

## Unreleased

- **Slide decks: styled by the site theme + a fuller reader.** A published deck now links the active
  theme's stylesheet and renders content in the theme's `.prose` class, so quotes, callouts, code,
  tables and Mermaid look like the site and theme authors can style `.slide*` themselves. The reader
  gained: **touch/swipe** navigation and on-screen **prev/next** buttons; on-screen **presenter** and
  **fullscreen** toggles (so they work without a keyboard); a **light/dark** toggle (reusing the
  theme's `data-theme`); a large **mobile presenter card** (the notes fill the phone as a teleprompter
  while the slide shows on the big screen); and an **autocue** that auto-scrolls each slide's notes and
  auto-advances at a reading pace — adjustable live with `+`/`−` (or the on-screen slower/faster
  buttons), shown in the counter and remembered. Stop with Back, restart from the button. Mermaid
  renders lazily per slide (it can't measure a hidden one), and a `<base href>` fixes co-located
  asset URLs in the deck.

## v0.0.32

- **Slide decks render the post's content well by default.** A **cover slide** (title, description,
  author avatar/initials) leads; content is **paginated** to fit (blocks pack onto a slide, overflow
  spills to a continuation slide, an oversized code block truncates with a link back to the post,
  images/video scale to fit); **math, diagrams and syntax highlighting hydrate** from the published
  `/vendor` assets; media (images/audio/video) stays **on the slide**, not in notes; callouts and
  pull-quotes are styled. Prose paragraphs become the **presenter notes** (shown in presenter mode);
  everything else is on the slide — never both. The Downloads-box **Slides** link opens the deck in a
  **new tab**. New keys: **Enter** plays/pauses the slide's media, **Esc** closes the deck (back to
  the post).
- **Slide decks (`slides:`).** A post can be projected into a themed slide deck, published at
  `…/<slug>/slides/`, linked from the Downloads box, and flagged with a marker in the listing. It's
  derived from the post (headings → slides/bullets, prose → speaker notes, other blocks on the
  slide); with JS it's a keyboard/swipe presentation (presenter notes, fullscreen), and with JS off
  the same file reads as a long-form document. Configure with `slides.enabled`/`slides.split` at the
  site level and override per post (`slides: true`/`false` or the block form; overwrites by key).
  Split targets: `h1`–`h6`, `hr`, `splitslide`, `image`/`table`/`code`/`math`/`diagram`/`audio`/
  `video`, and `text:<match>`. Inline markers `<splitslide>`, `<slide>…</slide>` and `<noslide>…
  </noslide>` mirror the `<tts>` family. See [Authoring content → Slide decks](content.md#slide-decks).

## v0.0.31

- **Bluesky: refresh a card via an atomic swap, only on `--resync`.** The earlier "edit in place"
  for Bluesky (v0.0.29) was a no-op — Bluesky's AppView ignores record edits, so the public card
  never changed. colophon now refreshes a Bluesky card by atomically deleting and recreating the
  record at the **same rkey** (`applyWrites`): the card re-indexes and the **permalink is kept**,
  but it's a new record so **likes/reposts/replies reset** and the timestamp updates. Because that's
  lossy, it runs **only on `--resync`** (an explicit opt-in); automatic edit-on-change now **skips**
  Bluesky with a note. **Mastodon** still edits in place automatically (no engagement loss).
- **Accessibility: a sweep toward WCAG AAA** (see the `wcag-aaa-compliance` decision). Engine: code
  blocks, Mermaid and display math are keyboard-focusable scroll regions (2.1.1); tables are wrapped
  in a focusable `.table-scroll` (semantics preserved, no `display:block` hack); GFM task-list
  checkboxes get an `aria-label`. Press theme: a visible keyboard-focus indicator on every control;
  `role="img"` on the audio/attachment markers; the home page hero moved inside `<main>`; and a
  contrast pass — `--muted`/`--faint` raised to ≥7:1 and a new `--link` token (≥7:1) for accent
  text (links, inline code, badges, pull-quote attribution), with `--accent` kept for decoration.

## v0.0.30

- **Syndication card descriptions fall back to a body excerpt.** A post with no `description:`
  frontmatter previously syndicated with an empty summary (a bare Bluesky/Mastodon card);
  `build.Entries` now mirrors the page — explicit `description:`, else a short excerpt of the
  rendered body. Also fixes empty descriptions in feeds for such posts. Re-run
  `colophon syndicate --resync` once to push the new descriptions onto existing cards.
- **`syndicate` skips, doesn't fail, an entry with no recorded silo URL.** Ledger entries posted
  via a fire-and-forget driver (Bridgy) have no editable handle; `--resync` now reports them as
  `skipped` with a note instead of erroring the whole run non-zero.

## v0.0.29

### Content & themes

- **Pull-quotes / epigraphs.** A `> [!quote] Attribution` callout renders as a semantic
  `<figure class="pullquote">` with the attribution as `<figcaption>` (omit it for an unattributed
  quote); the **press** theme styles it as a large display quote. See
  [Authoring content → Markdown support](content.md#markdown-support).
- **Glossary reference links.** A `glossary.yaml` term can carry reference links, rendered as
  citation-style superscripts after the decorated term. See
  [Authoring content → Glossary](content.md#glossary).
- **Tables are styled in press** (borders, padding, header underline, row hover, horizontal
  scroll on narrow screens). goldmark's per-column alignment is preserved.
- **`colophon serve --showcase`.** Injects a built-in `/showcase/` page — embedded in the binary,
  never written to your content — that renders *every* content feature (callouts, pull-quotes,
  tables, maths, diagrams, image/video/audio embeds, attachments, glossary, post hero/description/
  audio reading) in your active theme, with the source shown alongside. The single living
  reference for what a theme can style.

### Generation (image & speech)

- **Named generation profiles.** Each modality (`generation.image` / `generation.speech`) takes a
  default block plus named `profiles:` that inherit it and override only what they set. Select a
  profile with the same key at every scope — `image_profile:` / `speech_profile:` — on an
  *environment*, in a *post's frontmatter*, or per image via `<gen:…?profile=name>`; narrowest
  scope wins. Fully annotated in [`colophon.reference.yaml`](colophon.reference.yaml); see also
  [Image & audio generation](image-generation.md).
- **Provider-agnostic pronunciation dictionaries** with a bundled British dict (`pronunciation_dict:
  en_GB`): `ipa:` entries render to each provider's phoneme mechanism (ElevenLabs uploads a
  versioned dictionary; MiniMax sends them inline), `say:` entries substitute as plain text.
- **Speech: headings pause.** A heading now ends on a sentence boundary in the spoken reading, so
  it no longer runs into the next paragraph.
- **Speech: the waveform is precomputed** from the same audio (no second render) and shipped as the
  `<audio>.json` sidecar; the in-browser visualiser is the fallback when it's absent.

### Syndication (POSSE)

- **Edit a syndicated copy when the post changes.** The ledger stores a content fingerprint; a
  later run edits the existing silo copy in place (Mastodon `PUT`, Bluesky `putRecord`) instead of
  skipping — keeping its permalink/likes/replies. Bridgy/command can't edit and are left as-is. The
  first run after upgrading **backfills** fingerprints without editing.
- **`colophon syndicate --resync`.** A one-shot that re-edits every already-syndicated copy to its
  current content, ignoring fingerprints — to catch up copies created before the feature. Entries
  with no recorded silo URL (e.g. posted via Bridgy) are skipped, not failed.

  See [Syndication (POSSE) → Editing a syndicated copy](syndication.md#editing-a-syndicated-copy-when-the-post-changes).
