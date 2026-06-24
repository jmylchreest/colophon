# Themes

A theme turns colophon's page data into HTML. Themes are
[pongo2](https://github.com/flosch/pongo2) templates (Jinja2/Django syntax) plus static
assets. Three themes ship built in:

- **`default`** — full-featured: hero banners, index thumbnails, and vendored
  highlight.js / KaTeX / Mermaid for code, maths and diagrams, plus self-hosted web fonts.
- **`press`** — colophon.blog's brand theme. Literary-modern (Fraunces over Inter), light &
  dark, drifting glow, ink-blob title reveal, feed popouts. It *inherits* `default` (see
  [base themes](#base-themes-inheriting-another-theme)), so it reuses the same vendored
  libraries and fonts without shipping its own copy. The home-page lede under the title comes
  from the site's optional `tagline:` (presentational, distinct from the SEO `description:`);
  unset renders no lede.
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
| `page.html` | Renders a single entry (post or page). **Required** — the default for every page type. |
| `index.html` | Renders the site index (post list). |
| `<type>.html` | *Optional.* Renders entries of page type `<type>` (e.g. `project.html`); falls back to `page.html`. See [Page types](#page-types). |
| `favicon.svg` | Default site icon (override per-site with `favicon:` pointing at a project file). |
| `theme.yaml` | *Optional.* Theme metadata — a `description`, and `image.genai.system_prompt` (the house style for [generated images](image-generation.md#house-style-theme-system-prompt)). Not copied to the output. |
| *anything else* | Any non-`.html` file is copied verbatim to the output root (CSS, JS, fonts, images). |

Static assets keep their relative path: `themes/mytheme/vendor/app.js` is written to
`/vendor/app.js` and referenced as `{{ base_path }}vendor/app.js`.

### Page types

Every entry has a **type**. By default it's derived from whether the entry has a date — a
dated entry is a `post` (chronological: listed on the index, in feeds, on tag pages), a
dateless one is a `page` (standing chrome: surfaced in the nav menu, not in the list/feeds).
An author can override this with a `type:` in frontmatter (see
[Authoring → page types](content.md#page-types)), including custom types like `project`.

As a theme author you don't have to do anything: **every type renders with `page.html`** unless
you opt in. When you want a type to look different, you have two ways — pick whichever suits.

**1. A dedicated template** — add `themes/<theme>/<type>.html`. An entry of that type renders
with it; any type without its own file falls back to `page.html`. The file is an ordinary
single-entry template and receives the **same variables as `page.html`** (see the table below).
For example, to give `type: project` entries a bespoke layout:

```html
{# themes/mytheme/project.html — renders entries with `type: project` #}
<!doctype html>
<html lang="{{ lang }}">
<head>
  <meta charset="utf-8"><title>{{ meta_title }}</title>
  <link rel="stylesheet" href="{{ base_path }}style.css">{{ seo_head|safe }}
</head>
<body>
  <article class="project">
    <h1>{{ title }}</h1>
    {% if image %}<img class="project-shot" src="{{ image }}" alt="{{ title }}">{% endif %}
    {{ content|safe }}
    {% if tags %}<footer>{% for t in tags %}<a href="{{ t.url }}">{{ t.name }}</a> {% endfor %}</footer>{% endif %}
  </article>
</body>
</html>
```

**2. Branch inside `page.html`** — the `page_type` variable holds the resolved type, so one
template can switch on it without a separate file:

```html
{% if page_type == "project" %}
  <span class="badge">Project</span>
{% elif page_type == "page" %}
  {# a standing page — maybe hide the date/reading-time line #}
{% else %}
  <time>{{ date }}</time> · {{ read_time }} min
{% endif %}
```

**Placement.** A custom type is *listed* (post-like) by default; the built-in `page` is the only
*standing* (nav) type. So `type: page` makes a dated entry standing (it appears in `nav_pages`,
not in the index list or feeds), and `type: post` makes a dateless one listed. You don't render
the nav/list yourself per type — the build routes entries into `nav_pages` (standing) vs `pages`
(listed) for you; your per-type template only styles the single entry.

## The templating language

Templates are [pongo2](https://github.com/flosch/pongo2) — Jinja2/Django syntax:

- `{{ value }}` prints a value (HTML-escaped by default).
- `{{ value|safe }}` prints pre-rendered HTML **without** escaping — required for `content`,
  `feed_head` and `seo_head`.
- `{% if x %}…{% elif y %}…{% else %}…{% endif %}` and `{% for item in list %}…{% endfor %}`.
- Filters chain with `|`, e.g. `{{ title|default:site_title }}`, `{{ tags|length }}`.
- `{# comment #}` (keep it on one line — pongo2 rejects a newline inside `{# … #}`).

> **Always prefix internal links with `{{ base_path }}`** (`{{ base_path }}style.css`,
> `{{ base_path }}{{ p.url }}`). `base_path` makes the theme work whether the site is served
> from `/` or a sub-path.

## Template variables

### `page.html` (a single post)

| Variable | Description |
|----------|-------------|
| `site_title` | The site title. |
| `title`, `date`, `description` | Post metadata (`date` is a date; may be empty). |
| `meta_title` | Pre-resolved `<title>` text (SEO title → title → site title). |
| `content` | The rendered post HTML. Output with `{{ content\|safe }}`. |
| `base_path` | URL prefix for internal links (always starts and ends with `/`). |
| `base_url` | Absolute site root, for canonical/social URLs. |
| `feed_head` | `<link rel="alternate">` feed-discovery tags. Output with `{{ feed_head\|safe }}`. |
| `seo_head` | Full SEO `<head>`: canonical, robots, Open Graph, Twitter, JSON-LD. `{{ seo_head\|safe }}`. |
| `analytics_head` | Analytics provider markup (statsfactory beacon and/or GA loader). Output once before `</body>` with `{{ analytics_head\|safe }}`. Empty when the site configures no analytics. See [Analytics](#analytics). |
| `glossary_head` | Glossary styles + decorator `<link>`/`<script>`. Output once before `</body>` with `{{ glossary_head\|safe }}`. Empty unless the page uses a glossary term. See [Glossary](#glossary). |
| `lang` | The page's BCP-47 language tag — put it on `<html lang="{{ lang }}">`. |
| `favicon` | Favicon filename, or empty. |
| `hero` | Hero banner URL (page-relative, or absolute when routed), or empty. |
| `hero_alt`, `hero_style` | Hero alt text, and a ready `object-fit`/`object-position` style string (may be empty). Use as `alt="{{ hero_alt }}"`{% if hero_style %} `style="{{ hero_style }}"`{% endif %}. |
| `image`, `image_abs` | Preview image href; absolute preview URL for `og:image`. |
| `image_alt`, `image_style` | Card-image alt text and `object-fit` style (the index list items carry `image_alt`/`image_style` too). |
| `tags` | List of `{name, url}` — linked tag chips. Prefix nothing; `url` is ready to use. |
| `category` | Primary category string (first category, else first tag, else empty). |
| `read_time` | Estimated reading time in whole minutes (integer). |
| `toc` | List of `{level, id, text}` headings, for a table of contents. |
| `page_type` | The resolved page type (`post`, `page`, or a custom value) — for branching within a shared template. |
| `draft`, `embargoed`, `embargo_until` | Preview-only flags for not-yet-public posts. |
| `has_code`, `has_math`, `has_mermaid` | True when the post uses that block type — load the matching library only when set. |
| `author_name`, `author_initials`, `author_bio`, `author_url`, `author_avatar` | Author h-card fields for the byline (empty when unset). `author_avatar` is a ready-to-use `src`: a file-path avatar is published to `/assets/<name>` and emitted root-anchored (or as the object-store URL when routed); `data:`/`http(s)://` and `gravatar` avatars resolve to a URL that passes through. |
| `has_audio`, `audio`, `audio_type` | True when the post has an audio reading (recorded or generated TTS); `audio` is its URL and `audio_type` its MIME. See [Audio, video & downloads](#audio-video--downloads). |
| `audio_listen`, `audio_play`, `audio_pause` | Localised player UI strings (figcaption + play/pause aria-labels), in the page's language. Present only when `has_audio`. |
| `has_attachments`, `attachments`, `attachments_html` | Downloads. `attachments_html` is a ready-to-drop-in, no-JS block (`{{ attachments_html\|safe }}`); `attachments` is the structured list — `{url, label, description, name, type, type_label, size, bytes}` — if you'd rather build your own. `has_attachments` is the flag. See [Audio, video & downloads](#audio-video--downloads). |
| `mentions_enabled`, `has_mentions`, `mentions`, `mentions_html`, `mentions_attrs`, `mentions_src` | Webmentions (replies/likes/reposts). What's populated depends on the site's `display.mode` — see [Webmentions](#webmentions-responses). |
| `has_syndication`, `syndication`, `syndication_html` | "Also posted on…" links from the post's `syndication:` frontmatter (absolute URLs). `syndication_html` is a no-JS drop-in of mf2 `u-syndication` links (`{{ syndication_html\|safe }}`, empty when none); `syndication` is the raw URL list to build your own. |

### `index.html` (the post list, and per-tag pages)

| Variable | Description |
|----------|-------------|
| `lang`, `site_title`, `base_path`, `base_url`, `feed_head`, `favicon`, `analytics_head` | As above. (Listing pages carry no prose, so no `glossary_head`.) |
| `heading` | Page heading — the site title on the home page, or `Tagged “<name>”` on a tag page. |
| `tagline` | The site's optional `tagline:`, for a hero lede under the title. Empty when unset — guard with `{% if tagline %}`. |
| `seo_head` | The listing's SEO `<head>` block (canonical, Open Graph/Twitter, JSON-LD). Emit with `{{ seo_head\|safe }}`. |
| `feeds` | List of `{label, href}` for subscribe links. |
| `pages` | List of posts: `{title, url, date, draft, embargoed, embargo_until, image, image_alt, image_style, audio, has_audio, has_attachments, tags, series}`. Prefix `url` with `base_path`. Use `has_audio`/`has_attachments` to flag entries with media (see below). |

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

## Audio, video & downloads

colophon resolves media; the theme decides how it looks. There are four touch-points.

**1. Inline video/audio embeds.** A body embed pointing at a media file (`![](demo.mp4)`,
`![](clip.mp3)`) is rendered for you as `<video class="post-video" controls …>` or
`<audio class="post-inline-audio" controls …>`. You only need to **style** them — make them
responsive in your prose:

```css
.prose .post-video { display: block; max-width: 100%; height: auto; }
.prose .post-inline-audio { display: block; width: 100%; }
```

**2. The audio reading + player.** When `has_audio` is set, a post has a podcast-style reading
(recorded `audio_file:` or generated TTS). The build emits a shared, dependency-free
`player.js` to the site root whenever any page has audio. Opt in with the markup contract — a
container marked `data-audioplayer` with the source and localised labels, plus the script:

```html
{% if has_audio %}
<figure class="post-audio" data-audioplayer data-src="{{ audio }}"
        data-label-play="{{ audio_play }}" data-label-pause="{{ audio_pause }}">
  <figcaption>{{ audio_listen }}</figcaption>
  <audio controls preload="none" src="{{ audio }}"></audio>   <!-- no-JS fallback -->
</figure>
{% endif %}
...
{% if has_audio %}<script defer src="{{ base_path }}player.js"></script>{% endif %}
```

`player.js` progressively enhances the `<figure>` into a play/pause control with a scrubbable
waveform (a `<src>.json` peaks sidecar when present, else peaks decoded from the audio in-browser
on first play and cached, else live Web Audio, else idle); with JS off the native `<audio>` still
plays. Style the enhanced parts via `.post-audio.ap-ready`,
`.ap-toggle`, `.ap-wave`, `.ap-time` — see any bundled theme's CSS.

> The *content* of a generated reading is shaped by authoring hints — type-aware cues for
> code/diagrams/tables, and `<notts>`/`<tts>` to hide or force text. Those are an author concern,
> documented in [Authoring content](content.md); a theme doesn't handle them.

**3. The downloads block.** The engine renders the whole Downloads list for you — a no-JS,
semantic fragment with stable classes. Drop it in wherever you like (the bundled themes put it
below the author box) and style it with CSS:

```html
{{ attachments_html|safe }}   {# empty when the post has none, so no guard needed #}
```

It emits `.post-downloads > .downloads-title + .downloads-list > .dl-item > a.dl`, each row with
`.dl-ico` (paperclip), `.dl-main` (`.dl-label` + optional `.dl-desc`) and `.dl-meta`
(`.dl-type` badge + `.dl-size`). Style those classes to taste.

Prefer your own markup? Ignore the fragment and loop the structured `attachments` list instead —
each entry has `url`, `label`, `description`, `name`, `type`, `type_label`, `size`, `bytes`:

```html
{% if attachments %}<ul class="my-downloads">
  {% for f in attachments %}
  <li><a href="{{ f.url }}" download>{{ f.label }}</a>
      {% if f.description %}<p>{{ f.description }}</p>{% endif %}
      <span>{{ f.type_label }} · {{ f.size }}</span></li>
  {% endfor %}
</ul>{% endif %}
```

**4. Listing markers.** On `index.html`, each `pages` entry carries `has_audio` and
`has_attachments`. The press and contrib themes show a small inert speaker / paperclip mark in
the row's top-right corner so readers can spot posts with media at a glance — copy that pattern,
or surface it however suits your design:

```html
{% if p.has_audio or p.has_attachments %}<span class="post-flags">
  {% if p.has_audio %}<span class="pf" aria-label="Has an audio reading">…speaker svg…</span>{% endif %}
  {% if p.has_attachments %}<span class="pf" aria-label="Has downloadable files">…paperclip svg…</span>{% endif %}
</span>{% endif %}
```

## Microformats2 (IndieWeb)

The bundled themes annotate posts with [microformats2](https://microformats.org/wiki/microformats2)
— invisible class names that let other software (IndieWeb readers, Webmention senders, Bridgy)
understand your content. It's pure markup: no JS, no config (the old `microformats` toggle was
removed — it's always on). If you write your own theme, mirror the pattern so it stays parseable:

- Post page: the post container is `h-entry`, with `p-name` (title), `dt-published` (the `<time>`),
  `e-content` (the rendered body), a `u-url` permalink (`{{ permalink }}` is the absolute URL), and a
  nested `p-author h-card` (`p-name` + `u-url` on the author, `u-photo` on the avatar).
- Listing/index: the list is `h-feed`, each entry `h-entry`, the title link `u-url p-name`, the date
  `dt-published`.
- Identity: author profile links carry `rel="me"`.

When a theme splits the title/byline away from the body (e.g. a hero block), carry the stray
properties into the `h-entry` root as hidden elements (`<data class="p-name" value="…">`, a hidden
`<time class="dt-published">`) — see the `signal`/`obsidian` contrib themes.

## Webmentions (responses)

> Shipped. All bundled + contrib themes carry the responses block; configure
> `federation.indieweb.webmention.display.mode` (and run `colophon webmention fetch`) to populate it.
> See [Show webmentions](howto/webmentions.md).

Webmentions are replies/likes/reposts from other sites, shown under a post. **The engine never
decides how they render — it only exposes the data**, and the site picks a `display.mode`
(`federation.indieweb.webmention.display.mode`). What the engine populates depends on that mode:

| Variable | `live` | `asset` | `disabled` |
|----------|--------|---------|-----------|
| `mentions_enabled` | `true` (unless the post sets `webmentions: false`) | `true` (unless opted out) | **`false`** |
| `has_mentions` | `false` — **not known at build** (JS fills the count) | accurate, from the synced list | `false` |
| `mentions` | empty — no build-time data | structured list (bake your own) | empty |
| `mentions_html` | empty | engine-rendered drop-in block | empty |
| `mentions_src` | the receiver's client-fetch endpoint | your published `_mentions/<post>.json` | unset |

- **`live`** — the browser fetches the receiver directly; most realtime, no rebuild, but no
  build-time data (so no count, no baking) and nothing for no-JS readers.
- **`asset`** — the browser fetches *your* curated `_mentions/` asset (refreshed out-of-band by
  `webmention publish`); because that list also exists at build, the engine fills `mentions` /
  `mentions_html` / `has_mentions`, so a theme **may bake** instead of relying on JS.
- **`disabled`** — nothing is exposed or shipped, regardless of the per-post setting.

Always guard the block on `mentions_enabled` and keep it a **sibling of the content element**, never
inside it (so it's never read aloud by the TTS reading — same rule as the author card/downloads).

**Progressive-enhancement block (what `press` uses).** Bake the asset-mode HTML server-side, then let
the engine-emitted `mentions.js` refresh it (or, in `live` mode, populate it):

```django
{% if mentions_enabled %}
<section class="responses" {{ mentions_attrs|safe }} aria-label="Responses">{{ mentions_html|safe }}</section>
{% endif %}
…
{% if mentions_enabled %}<script defer src="{{ base_path }}mentions.js"></script>{% endif %}
```

`mentions_attrs` is the engine-owned `data-mentions*` wiring (the source URL, plus `data-mentions-live`
+ target + blocklist in `live` mode) — drop it in and the same markup works across modes and reader
drivers. With JS off (and `asset` mode) the baked `mentions_html` still shows; with JS on, `mentions.js`
fetches and renders/refreshes.

**Build your own from the structured list** (`asset` mode), e.g. to restyle or split by type:

```django
{% if has_mentions %}<section class="responses">
  {% for m in mentions %}
  <article class="response {{ m.type }} h-cite">
    {% if m.author.photo %}<img class="u-photo" src="{{ m.author.photo }}" alt="">{% endif %}
    <a class="p-author h-card u-url" href="{{ m.author.url }}">{{ m.author.name }}</a>
    {% if m.content %}<div class="p-content">{{ m.content }}</div>{% endif %}
    <a class="u-url" href="{{ m.url }}"><time class="dt-published">{{ m.published }}</time></a>
  </article>
  {% endfor %}
</section>{% endif %}
```

Each `mentions` item is `{type (like|repost|reply|mention), author{name,url,photo}, url, content,
published}`. A **no-JS / text theme** (e.g. `minimal`) just uses `asset` mode + `{{ mentions_html|safe }}`
and skips the script entirely.

### Silo icons

Responses, the "Also posted on…" (`u-syndication`) links, and the author h-card links show a small
**brand icon** when colophon recognises the source silo (Bluesky, Mastodon, GitHub, X, Reddit,
Hacker News, Threads, Flickr, LinkedIn, Tumblr, GitLab — else a generic website globe). These are
glyphs in a tiny webfont, **`silos.woff2`**, that the engine emits at the site root (when responses
are active) and renders into `<span class="silo">`. A theme just declares the face and styles the
span:

```css
@font-face { font-family: "Colophon Silos"; src: url("silos.woff2") format("woff2"); font-display: swap; }
.silo { font-family: "Colophon Silos"; line-height: 1; }
```

The font is curated from Font Awesome (Brands + Solid) by
[`contrib/scripts/silo-font`](../contrib/scripts/silo-font/) — re-run it to add a network or change
the set. Detection (`host → silo`) lives in `internal/build/mentions.go` + `assets/mentions.js`, kept
in sync with the font's `silos.json` codepoints.

## Analytics

colophon owns the analytics clients; a theme's only job is to **include** them. When a site
configures a provider (statsfactory and/or Google Analytics — see [analytics](analytics.md)),
the build writes that provider's loader to the site root — `analytics-sf.js` for the cookieless
statsfactory beacon, `analytics-ga.js` for the Google Analytics loader — and exposes the
matching `<script>` markup (with each page's dimensions) as the `analytics_head` variable.

A theme leverages both providers with one line, just before `</body>`:

```html
{% if analytics_head %}{{ analytics_head|safe }}{% endif %}
</body>
```

The theme never names a provider: `analytics_head` already contains whichever loaders are
enabled (statsfactory, GA, both, or — when the site configures none — nothing, leaving the line
inert). Every built-in and contrib theme includes it; a JS-enabled custom theme should too.

## Glossary

If the site ships a `glossary.yaml` (see [authoring](content.md#glossary)) and a page uses a
term, the build publishes the data + a small decorator and exposes a `glossary_head` variable.
A theme opts in with one line before `</body>` (right where the analytics line goes):

```html
{% if glossary_head %}{{ glossary_head|safe }}{% endif %}
</body>
```

That's it — no per-theme CSS or JS needed:

- The **decorator and styles are engine-provided** (`glossary.js` + `glossary.css`, written to
  the site root). The decorator wraps terms in `<abbr class="gloss" data-gloss="…">` and shows
  an accessible pop-over on hover/focus (`role="tooltip"` + `aria-describedby`, Escape to
  dismiss).
- The default styling **adapts to your theme** through the token contract: it uses
  `--accent` for the underline and `--elevated`/`--text`/`--muted`/`--border` + `--serif`/`--sans`
  for the dictionary-stanza card (with neutral fallbacks for token-less themes).
- To customise the look, **override `.gloss` and `.gloss-tip`** (and `.gloss-tip .gloss-term`
  / `.gloss-def`) in your own stylesheet — e.g. `press` gives `.gloss` a light wavy underline.
- A **text-only / no-JS theme** simply omits the line: terms stay plain readable text, so the
  page still works correctly without the decorator (this is what `minimal` does).

## Build a theme — step by step

The fastest route is to start from a built-in and change only what you want. A new theme
inherits `default`, so you can ship as little as one CSS file.

**1. Create the theme directory** in your project and point a build at it:

```sh
mkdir -p themes/mytheme
```
```yaml
# colophon.yaml
sites:
  - id: main
    theme: mytheme          # or set it on one environment to preview first
```

**2. Add a stylesheet.** With nothing else present, `mytheme` uses the `default` templates
and your CSS:

```css
/* themes/mytheme/style.css */
body { font-family: Georgia, serif; max-width: 42rem; margin: 2rem auto; }
```

```sh
colophon serve            # open the printed URL; edits live-reload
```

That alone is a working theme. Everything below is optional, added when you want more control.

**3. Take over the post template.** Copy a built-in as a starting point, then edit it:

```sh
colophon themes eject default   # writes themes/default/ — copy what you need into mytheme/
```

A minimal `themes/mytheme/page.html`:

```html
<!doctype html>
<html lang="{{ lang }}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ meta_title }}</title>
  <link rel="stylesheet" href="{{ base_path }}style.css">
  {{ feed_head|safe }}
  {{ seo_head|safe }}
</head>
<body>
  <header><a href="{{ base_path }}">{{ site_title }}</a></header>
  <article>
    <h1>{{ title }}</h1>
    {% if date %}<time>{{ date }}</time>{% endif %}
    {% if read_time %}<span>· {{ read_time }} min read</span>{% endif %}
    {{ content|safe }}
    {% if tags %}<footer>{% for t in tags %}<a href="{{ t.url }}">{{ t.name }}</a> {% endfor %}</footer>{% endif %}
  </article>
</body>
</html>
```

**4. Add the index** (`themes/mytheme/index.html`) — the post list, the nav menu (standing
pages like About), and per-tag pages:

```html
<!doctype html>
<html lang="{{ lang }}">
<head><meta charset="utf-8"><title>{{ heading }}</title>
  <link rel="stylesheet" href="{{ base_path }}style.css">{{ feed_head|safe }}</head>
<body>
  {% if nav_pages %}<nav>{% for n in nav_pages %}<a href="{{ n.url }}">{{ n.title }}</a> {% endfor %}</nav>{% endif %}
  <h1>{{ heading }}</h1>
  <ul>
  {% for p in pages %}
    <li><a href="{{ base_path }}{{ p.url }}">{{ p.title }}</a>
        {% if p.date %}<small>{{ p.date }}</small>{% endif %}</li>
  {% endfor %}
  </ul>
</body>
</html>
```

`nav_pages` is the list of standing pages (`{title, url}`); `pages` is the chronological posts.
The build sorts entries into these two buckets by [page type](#page-types) — you just render
them. (Add the same `nav_pages` block to `page.html` so the menu appears on entries too.)

**5. (Optional) Tailor specific page types.** Add a `<type>.html` template, or branch on the
`page_type` variable inside `page.html`, to give a type its own look. See
[Page types](#page-types). Skip this and every entry just uses `page.html`.

**6. Decide how rich blocks render.** Do nothing (raw text shows, like `minimal`), or enhance
them with the `has_*` gates as shown in [Enhancing rich blocks](#enhancing-rich-blocks). The
vendored libraries are inherited from `default`, so `{{ base_path }}vendor/katex/…` resolves
with no extra files in your theme.

**7. Add your own assets.** Any non-`.html` file under `themes/mytheme/` is copied to the
output root, keeping its path: `themes/mytheme/logo.svg` → `/logo.svg`, referenced as
`{{ base_path }}logo.svg`. Self-host fonts the same way and `@import` them from your CSS.

**8. Build and ship:**

```sh
colophon build --env production    # writes public/ with your theme
```

Checklist: every internal `href`/`src` starts with `{{ base_path }}`; `content`, `feed_head`
and `seo_head` use `|safe`; `{# comments #}` stay on one line. To contribute a theme back, drop
it in [`contrib/themes/`](#community-themes-contribthemes) following the existing ones.
