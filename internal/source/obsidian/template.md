---
# All optional. With the obsidian source, an omitted title falls back to a leading
# "# heading" or the file name, and an omitted date falls back to the file's mtime.
title:
date:                                   # YYYY-MM-DD (Obsidian Templates: {{date:YYYY-MM-DD}})
description:                            # one-line summary for feeds and link previews
tags: []
slug:                                  # overrides the path-derived URL slug
hero:                                  # banner image at the top of the post, e.g. "[[banner.png]]"
image:                                 # preview/social-card image, e.g. "[[preview.png]]"
draft: true                            # not published until set false
publish: true                          # honoured when the source sets publish_required: true
publish_after:                         # embargo until this time, ISO 8601 e.g. 2026-07-01T09:00:00Z

# Optional SEO/social overrides. Everything below is derived from the fields above when
# omitted; an authoring skill can fill this block in. See docs/seo.md.
# seo:
#   title:                             # <title>/og:title, ≤60 chars
#   description:                       # meta description, 140–160 chars
#   keywords: []                       # focus terms
#   canonical:                         # absolute URL override
#   noindex: false                     # keep this page out of search
#   image:                             # absolute social-image URL override
#   social:                            # copy tuned for sharing, distinct from search copy
#     title:
#     description:
---

Write your post in Markdown. Link to other notes with `[[wikilinks]]` or
`[[note|aliased text]]`, and embed attachments with `![[image.png]]`. The folder this note
lives in becomes its URL path.

Rich blocks are supported: `$maths$`, ```` ```mermaid ```` diagrams, fenced code, and
Obsidian `> [!note]` callouts. Set `draft: false` when it's ready to publish.
