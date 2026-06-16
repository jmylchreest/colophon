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

A source is anchored on one **vault**, then narrowed by optional **paths** (where) and
**tags** (which):

```yaml
sources:
  - id: vault
    driver: obsidian
    vault: "{env:OBSIDIAN_VAULT:-}"   # the vault root — the one required setting
    path: [essays, til]               # optional: sub-folder(s) to scan; omit → whole vault
    tag: [blog, essay]                # optional: publish notes carrying any of these tags
    publish_required: true            # default true (used only when no tags are set)
```

- **`vault`** — the vault root (absolute / `~` / relative to the project root). Attachments
  (`![[image.png]]`) resolve **across this whole vault**, so posts in a sub-folder still reach
  assets elsewhere. An empty value (e.g. an unset `{env:VAR:-}`) yields no documents, so an
  env-driven source whose var is unset just contributes nothing.
- **`path`** — a folder, or a **list** of folders, within the vault to scan (the union; a note
  under *any* of them is in scope). Omit it to scan the whole vault. Paths are vault-relative
  and a leading `/` (vault-root style) is ignored, so `/Blog` and `Blog` are the same. Slugs
  are relative to the scanned folder, so `path: Blog` publishes `Blog/hello.md` at `/hello/`.
- **`tag`** — a tag, or a **list** of tags. A note matches if it carries **any** of them — in
  frontmatter `tags:` **or** as an inline `#tag`, Forestry / "digital-garden" style (leading
  `#` optional, case-insensitive; `blog` also matches the nested `#blog/published`). An
  explicit `publish: false` still opts a note out.

Both `path` and `tag` also accept a **comma-separated string**, so a single environment
variable can feed a list: `path: "{env:BLOG_PATH}"` with `BLOG_PATH=Blog,Notes/pub` scans both.
- **`publish_required`** — applies **only when no tags are set**: `true` (default) honours
  Obsidian's `publish: true` whitelist; `false` publishes every scanned note.

When both `path` and `tag` are set a note must satisfy **both** (under a scanned path **and**
carrying a tag). Tags, when present, replace the publish-flag gate.

### Selection vs. format

The source only decides **selection** — which notes you *intended* to publish. Whether a
selected note is **well-formed** is the build's concern: it warns (e.g. `post "x" has no
content`) but still publishes, so a stub never silently disappears and inclusion is never
gated on format. Obsidian note names are unique across a vault, so flat slugs rarely collide;
if two scanned paths surface the same note name, the first wins and the source warns.

### Multiple vaults

One source = one vault. To publish from several vaults, add one source per vault — the build
merges them:

```yaml
sources:
  - { id: work,     driver: obsidian, vault: "{env:WORK_VAULT:-}",     tag: blog }
  - { id: personal, driver: obsidian, vault: "{env:PERSONAL_VAULT:-}", path: posts }
```

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
