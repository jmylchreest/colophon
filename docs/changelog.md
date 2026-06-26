# Changelog

User-facing changes by release. Each entry points at the guide where the feature is documented
in full (or where it should be, when end-user docs catch up).

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
