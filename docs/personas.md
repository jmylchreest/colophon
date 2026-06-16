# Personas

A **persona** is a blog identity. In colophon, content is attributed to a persona — not to a
human — so the byline, h-card and (optional) writing style all attach to the persona. A single
person can own several personas, and a *brand* persona can be shared by several operators.

Personas live in `personas/<id>.yaml`:

```yaml
id: technical
display_name: "A. Researcher"
byline: "by A. Researcher"
kind: individual            # individual (one operator) | brand (shared byline)
hcard:                       # IndieWeb identity → byline, author meta, JSON-LD
  name: "A. Researcher"
  bio: "Writes about distributed systems."
  avatar: avatar.png
  urls: ["https://example.com"]
style:                       # optional — only used for AI-assisted writing
  guide: "Plain, precise, technical. Short sentences. No hype."
  references:
    - "https://example.com/glossary"
sites: [main]                # sites this persona may publish to
operators: []                # humans/agents allowed to write as this persona
```

A post selects its persona in frontmatter with `persona: technical` (shorthand for a single
publication). The persona drives the on-page byline/h-card, the feed author, and the JSON-LD
`author` (a `Person` for `individual`, an `Organization` for `brand`). Publishing content
as-is needs none of the `style` block — that is consulted only for AI-assisted writing.

## Listing personas

```sh
colophon persona list            # id, name, kind, and whether a style guide is set
colophon persona list --json     # machine-readable (for a skill/agent)
```

## Write-as context

colophon does **not** generate prose. It emits *context* and the calling agent does the
writing. `persona context` returns the persona's style guide and references plus the most
relevant **exemplars** drawn from that persona's own published content:

```sh
colophon persona context technical --topic "raft leader election"
colophon persona context technical --topic "raft" --top-k 5 --json
```

- With `--topic`, exemplars are ranked by relevance (a pure-Go BM25 over the persona's posts —
  no embeddings, no API key). Without a topic, the most recent posts are returned.
- `--top-k` sets how many exemplars to emit (default 3).
- The persona id is optional when there is a single persona (or one named `default`).

This is the core of the agent write-as flow: an agent lists personas, fetches the write-as
context for one, writes a draft in that voice, previews it, and publishes — all through the
CLI, with deploy secrets resolved server-side and never passed to the agent.

> Retrieval is built in memory on each call (zero state). A persisted/​semantic index is a
> future option; the command shape stays the same.
