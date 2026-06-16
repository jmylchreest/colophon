# obsidian source

Reads an [Obsidian](https://obsidian.md) vault folder in place (no copy) as a content
source. The folder structure maps onto the site, and deletes/renames flow through the
normal build reconciliation. `[[wikilinks]]` resolve at build time, and `![[embeds]]`
(attachments) are resolved vault-wide by name and copied next to the page.

## Attachments, hero & preview images

- **`![[image.png]]`** embeds resolve to the matching file anywhere in the vault and are
  copied beside the page; `![[image.png|alt text]]` sets the alt text.
- **`hero:`** frontmatter is a banner image shown at the top of the post.
- **`image:`** frontmatter is the preview/social-card image (Open Graph + index thumbnail).

Both frontmatter fields accept an Obsidian ref (`"[[banner.png]]"`) or a bare name; quote
the `[[…]]` form so YAML doesn't read it as a list.

## Rich blocks

Notes can use `$inline$` / `$$display$$` maths, ` ```mermaid ` diagrams, fenced code, and
Obsidian `> [!note]` callouts. colophon renders these as progressive-enhancement HTML: the
raw source is always present (readable without JavaScript), and the default theme upgrades
it with KaTeX, Mermaid and a syntax highlighter, loaded only on pages that use them.

## Config

There are two ways to decide which notes are blog posts.

### Folder mode — a dedicated blog folder

```yaml
sources:
  - id: blog
    driver: obsidian
    path: "{env:OBSIDIAN_BLOG:-}"
    publish_required: false   # default true: only notes with `publish: true` ship
```

- **`path`** — the folder of notes, absolute / `~`-relative / relative to the project root
  (or relative to `vault` when one is set).
- **`publish_required`** — `true` (default) honours Obsidian's `publish: true` whitelist;
  `false` publishes every note (use for a folder dedicated to the blog).

### Tag mode — publish by an Obsidian tag (digital-garden style)

When you don't keep posts in one folder, point at the whole vault and pick a tag. Every note
carrying that tag — in frontmatter `tags:` **or** as an inline `#tag` — becomes blog-eligible,
the way the Forestry / "digital garden" plugins work.

```yaml
sources:
  - id: vault
    driver: obsidian
    vault: "{env:OBSIDIAN_VAULT:-}"
    tag: blog                 # publishes notes tagged #blog (and nested #blog/*)
```

- **`vault`** — the vault root. Attachments (`![[image.png]]`) resolve across the *whole*
  vault, so this also works in folder mode when posts sit in a sub-folder.
- **`tag`** — the Obsidian tag that marks a note for publishing (a leading `#` is optional;
  matching is case-insensitive; `blog` also matches the nested `#blog/published`). A note may
  still opt back out with `publish: false`.

**Structure requirements.** A tag-selected note must have a **title** (frontmatter `title:`
or a leading `# heading`) and some **body** content. Notes that match the tag but fail these
checks are **warned about and skipped** during the build (the warning names the note and the
reason), so a half-written note never ships by accident.

Set either `path` or `tag` when a `vault` is configured — a vault with no selector is a
config error. An empty `path`/`vault` (e.g. an unset `{env:VAR:-}`) yields no documents, so
an env-driven optional source whose var is unset just contributes nothing — handy for "one
or more vaults".

## Paths and `$HOME`

colophon interpolates `{env:VAR}` to the variable's **literal value** — it does not
expand `$HOME` or `~` itself (except a leading `~`, as a convenience). So:

```sh
export OBSIDIAN_BLOG="$HOME/OBSIDIAN/Blog/blog.i0.pm"   # shell expands $HOME → absolute
```
Equivalently, `path: "{env:HOME}/OBSIDIAN/Blog/blog.i0.pm"` works (colophon resolves
`{env:HOME}`). A *literal* `$HOME` left inside the value would not be expanded.

## New posts

`template.md` (next to this file) is a starter note: copy it into your vault and fill it
in. It's `draft: true`, so it never publishes until you flip that — and isn't itself
published if it lives in the blog folder. Templates ideally live in your Obsidian
Templates folder rather than the published folder.
