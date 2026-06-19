# SEO & social metadata

colophon gives every post correct search and social metadata **by default** — canonical URL,
description, Open Graph + Twitter cards, schema.org JSON-LD, and robots — derived from the
post's existing fields (title, description, tags, date, image, persona). An optional `seo:`
frontmatter block overrides any of it, and is the precise target an
[authoring skill](skills.md) can fill in.

## The `seo:` block

```yaml
seo:
  title:        # <title> / og:title; ≤60 chars (else: the post title)
  description:  # meta description; 140–160 chars (else: description / excerpt)
  keywords: []  # focus terms (else: the post's tags)
  canonical:    # absolute URL override (else: base_url + slug)
  noindex: false# robots noindex (drafts & embargoed posts are noindex regardless)
  image:        # absolute social-image URL override (else: the `image` field)
  type:         # schema.org @type (default: BlogPosting)
  social:       # copy tuned for sharing, when it should differ from the search copy
    title:
    description:
```

Every field maps to exactly one piece of output, so what you set is what gets rendered.

## What gets emitted (page `<head>`)

| Output | Source |
|--------|--------|
| `<link rel="canonical">` | `seo.canonical` → `base_url` + slug |
| `<meta name="description">` | `seo.description` → `description` |
| `<meta name="robots">` noindex | `seo.noindex`, or any draft/embargoed post |
| `<meta name="keywords">` | `seo.keywords` → tags |
| Open Graph (`og:type/site_name/url/title/description/image/locale`) | `seo` → post fields; `og:title`/`description` prefer `seo.social.*`; `og:image` is `seo.image` → the `image` field → the `hero` cover art; `og:locale` from the page/site `lang` |
| `article:published_time` / `modified_time` / `tag` / `author` | the post date, tags, and persona |
| Twitter card (`summary_large_image` when an image exists, else `summary`) | the resolved image |
| `<script type="application/ld+json">` **BlogPosting** | headline, description, image, dates, keywords, `author` (persona → Person/Organization), `publisher` (site) |

The JSON-LD author comes from the post's **persona** (`persona:` frontmatter, else the first
configured persona): an `individual` persona renders as a `Person`, a `brand` persona as an
`Organization`, with the persona's first h-card URL as `author.url`.

## Defaults vs. overrides

You never need an `seo:` block — a plain post already produces all of the above from its
title/description/tags/date/image/persona. Use `seo:` to:

- give search a tighter `title`/`description` than the on-page ones,
- write punchier `social:` copy for shares,
- set an explicit `canonical` (e.g. a syndicated original),
- `noindex` a page, or point `image` at an external social card.

Generating a good `seo:` block from the article is what the planned **seo skill**
([skills.md](skills.md)) does.

## Listing pages (home, tags, authors)

The home page and every generated listing — `/tags/<tag>/` and `/authors/<id>/` — also carry
their own metadata: a canonical URL, `description`, website-flavoured Open Graph + Twitter
cards, `og:locale`, and schema.org JSON-LD (a `Blog` for the home page, a `CollectionPage` for
tag/author listings). These draw on two optional site-level fields:

```yaml
sites:
  - id: main
    title: My Blog
    base_url: https://example.com
    description: One line that becomes the home page's meta/OG description.
    image: /assets/social.png   # default share image (absolute URL, or resolved against base_url)
```

Both are optional — unset simply omits the corresponding tags. `description` feeds the listing
pages' `<meta name="description">` and the JSON-LD; `image` is their default `og:image`/
`twitter:image`. Per-tag and per-author listings reuse the same `description`/`image` with their
own heading as the title.
