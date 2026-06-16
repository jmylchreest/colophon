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

```yaml
sources:
  - id: blog
    driver: obsidian
    path: "{env:OBSIDIAN_BLOG:-}"
    publish_required: false   # default true: only notes with `publish: true` ship
```

- **`path`** — the vault folder, absolute / `~`-relative / relative to the project root.
- **`publish_required`** — `true` (default) honours Obsidian's `publish: true` whitelist;
  `false` publishes every note (use for a folder dedicated to the blog).

An empty `path` yields no documents, so an env-driven optional source whose var is unset
just contributes nothing — handy for "one or more vaults".

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
