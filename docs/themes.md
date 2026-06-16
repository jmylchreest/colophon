# Themes

A theme turns colophon's page data into HTML. Themes are
[pongo2](https://github.com/flosch/pongo2) templates (Jinja2/Django syntax) plus static
assets. Three themes ship built in:

- **`default`** — full-featured: hero banners, index thumbnails, and vendored
  highlight.js / KaTeX / Mermaid for code, maths and diagrams, plus self-hosted web fonts.
- **`press`** — colophon.blog's brand theme. Literary-modern (Fraunces over Inter), light &
  dark, drifting glow, ink-blob title reveal, feed popouts. It *inherits* `default` (see
  [base themes](#base-themes-inheriting-another-theme)), so it reuses the same vendored
  libraries and fonts without shipping its own copy.
- **`minimal`** — plain, readable text. No JavaScript and no web fonts; rich blocks show as
  their raw source (the [raw-block contract](content.md#the-raw-block-contract-progressive-enhancement)).

More themes (`flux`, `signal`, `obsidian`) live in [`contrib/themes/`](#community-themes-contribthemes)
and are installed by copying them into your project.

## Selecting a theme

Set it on the site, and optionally override it per environment — handy for previewing a theme
before promoting it to production:

```yaml
sites:
  - id: main
    theme: default        # site default

environments:
  - name: production
    # inherits theme: default
  - name: text
    theme: minimal        # this environment builds with the minimal theme
```

Precedence: **environment `theme` > site `theme` > `default`**. Build or serve an environment
to see its theme:

```sh
colophon build --env text     # builds public/ with the minimal theme
colophon serve                # serves every environment, each with its own theme
```

## Inspecting and ejecting themes

```sh
colophon themes list            # default, minimal, press
colophon themes eject minimal   # copies the built-in into themes/minimal/ to edit
colophon themes eject default   # full default theme, incl. its vendored libraries
```

`eject` writes a built-in theme to `themes/<name>/` in your project; the on-disk copy then
overrides the built-in (use `--force` to overwrite an existing directory). It's the easiest
way to start customising — eject, then edit only the files you care about. Ejecting an
overlay theme (e.g. `press`) writes only *its own* files; the base theme's inherited assets
stay in the binary and still resolve at build, so the eject stays small.

## Supplying your own theme

Put files under `themes/<name>/` in your project root and set `theme: <name>` (or eject one
to start from). Files there **override the built-in default per file**, so you only write
what you want to change:

```
themes/
  mytheme/
    page.html      # overrides the post template
    style.css      # overrides the stylesheet
    logo.svg       # a new static asset, copied to the output root
```

An unknown theme name with no `themes/<name>/` directory falls back to the `default` theme.

### Base themes (inheriting another theme)

A theme can inherit another theme's templates and static assets by declaring a base. For an
on-disk theme this is automatic: any `themes/<name>/` directory **inherits `default`**, so it
only needs the files it changes (this is why dropping in a single `style.css` works). A
built-in theme inherits explicitly via a one-line `base` file naming the base theme — the
built-in `press` theme contains `base` → `default`, so it reuses the default's vendored
libraries and fonts and supplies only its own `page.html`, `index.html` and `style.css`.

Resolution order, highest precedence first: your project's `themes/<name>/` → the theme's
own files → its base theme's files. The `base` marker is never copied to the output.

### Community themes (`contrib/themes/`)

The colophon repo ships extra themes under `contrib/themes/` that are **not** baked into the
binary. To use one, copy it into your project and select it:

```sh
cp -r contrib/themes/flux myblog/themes/flux
# then, in colophon.yaml:  theme: flux
```

Because on-disk themes inherit `default`, a contrib theme only carries its own templates and
`style.css`; the vendored libraries and fonts come from the built-in `default` at build time.

### Theme files

| File | Role |
|------|------|
| `page.html` | Renders a single post. **Required.** |
| `index.html` | Renders the site index (post list). |
| `favicon.svg` | Default site icon (override per-site with `favicon:` pointing at a project file). |
| *anything else* | Any non-`.html` file is copied verbatim to the output root (CSS, JS, fonts, images). |

Static assets keep their relative path: `themes/mytheme/vendor/app.js` is written to
`/vendor/app.js` and referenced as `{{ base_path }}vendor/app.js`.

## Template variables

### `page.html`

| Variable | Description |
|----------|-------------|
| `site_title` | The site title. |
| `title`, `date`, `description` | Post metadata (`date` is `YYYY-MM-DD`; may be empty). |
| `content` | The rendered post HTML. Output with `{{ content|safe }}`. |
| `base_path` | URL prefix for internal links (always starts and ends with `/`). Prefix every internal href/src with it. |
| `base_url` | Absolute site root, for canonical/social URLs. |
| `feed_head` | `<link rel="alternate">` feed-discovery tags. Output with `{{ feed_head|safe }}`. |
| `favicon` | Favicon filename, or empty. |
| `hero` | Hero banner URL (page-relative, or absolute when routed), or empty. |
| `image` | Preview image href, or empty. |
| `image_abs` | Absolute preview image URL for `og:image`, or empty. |
| `draft`, `embargoed`, `embargo_until` | Preview-only flags for not-yet-public posts. |
| `has_code`, `has_math`, `has_mermaid` | True when the post uses that block type — load the matching library only when set. |

### `index.html`

| Variable | Description |
|----------|-------------|
| `site_title`, `base_path`, `base_url`, `feed_head`, `favicon` | As above. |
| `feeds` | List of `{label, href}` for subscribe links. |
| `pages` | List of posts: `{title, url, date, draft, embargoed, embargo_until, image}`. Prefix `url` with `base_path`. |

## Enhancing rich blocks

colophon emits the raw-block markup; **how to enhance it is entirely the theme's choice**.
The `default` theme loads vendored libraries from `themes/default/vendor/`, gated on the
`has_*` flags so a page only pulls in what it uses:

```html
{% if has_math %}
<link rel="stylesheet" href="{{ base_path }}vendor/katex/katex.min.css">
<script defer src="{{ base_path }}vendor/katex/katex.min.js"></script>
<script>/* render every .math element with katex */</script>
{% endif %}
```

Your theme is free to do something else with the same markup: load the libraries from a CDN,
swap in a different highlighter, or — like the `minimal` theme — do nothing and let the raw
text stand. The markup contract (`<pre class="mermaid">`, `<span class="math …">`,
`<pre><code class="language-…">`, `<div class="callout …">`) does not change.
