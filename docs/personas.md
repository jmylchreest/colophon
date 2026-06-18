# Authors & personas

colophon separates **who is shown** from **how it's written**:

- An **author** is the **byline readers see** — an identity (a person, or a brand name).
- A **persona** is a **hidden writing voice** the agent writes in — never shown, and
  **shareable across authors**.

A post carries up to two fields: `author:` (the byline) and `persona:` (the voice). The
voice is purely an authoring aid; nothing about a persona is rendered.

## Authors

Authors live in `authors/<id>.yaml` and supply the byline, author page, feed author and
JSON-LD `author`:

```yaml
id: ada                       # the id (defaults to the file stem, e.g. authors/ada.yaml)
name: "Ada Lovelace"          # the byline shown to readers
bio: "Writes about distributed systems."
avatar: assets/avatar.png     # see below — a content-source file, a data: URI, or an https:// URL
urls: ["https://example.com"]
email: ada@example.com
```

The `avatar` may be:

- a **file under a content source** (e.g. `assets/avatar.png`, resolved the same way a
  markdown image embed is — the first source that can open it wins). It is published once to
  `/assets/<name>` and the byline/topbar `src` is root-anchored, so it renders from every page
  depth (and is rewritten to the object-store URL when an `assets/**` route is active).
- a **`data:` URI** (a self-contained inline image), or
- a fully-qualified **`http(s)://` URL** (a hosted image).

`data:` and `http(s)://` avatars pass through untouched; a file path that no source can open
is warned about (no broken `src` is emitted).

A post names one with `author: ada`. If a post sets no `author:`, the **first configured
author** is the default; with no authors at all, the byline is **"Anonymous"** (a post
without an author still builds — it's just unattributed).

```sh
colophon authors               # list bylines (alias: colophon author)
colophon author show ada       # one author's full h-card
colophon authors --json        # machine-readable (for a skill/agent)
```

## Personas (the writing voice)

Personas live in `personas/<id>.yaml` and are **only** a voice — a style/character the agent
writes in, plus the references it may draw on:

```yaml
id: technical
name: "Senior engineer"        # a human label (not shown)
style:
  guide: "Plain, precise, technical. Short sentences. No hype. Senior-engineer perspective."
  references:
    - "https://example.com/glossary"
```

The same persona can be used by **different authors** — Ada and Grace can both publish in the
`technical` voice under their own bylines. A persona's *corpus* is every post written in it,
regardless of author, so the voice stays consistent and the exemplar pool grows.

```sh
colophon persona list            # id, label, and whether a style guide is set
colophon persona list --json     # machine-readable (for a skill/agent)
```

## Write-as context

colophon does **not** generate prose. It emits *context* and the calling agent does the
writing. `persona context` returns a voice's style guide and references plus the most relevant
**exemplars** drawn from the posts written in that voice:

```sh
colophon persona context technical --topic "raft leader election"
colophon persona context technical --topic "raft" --tag distributed --top-k 5 --json
```

- With `--topic`, exemplars are ranked by relevance (a pure-Go BM25 over the persona's posts —
  no embeddings, no API key). Without a topic, the most recent posts are returned.
- `--tag` (repeatable) narrows the corpus to exemplars carrying that tag.
- `--top-k` sets how many exemplars to emit (default 3).
- The persona id is optional when there is a single persona (or one named `default`).

## Finding where to write & what exists

Two commands round out the authoring toolbox (both take `--json`):

```sh
colophon sources               # where each source's content lives + how a post is marked live
colophon posts                 # existing entries: slug, title, type, author, persona, tags
colophon posts --tag go --author ada   # filter, e.g. to find cross-reference targets
```

## Creating & previewing

`colophon new post|page` validates the author and persona, derives a **unique pinned slug**,
writes a frontmatter skeleton to the right source, and reports the disk path and URL — then a
person *or* an agent fills the body (colophon never generates prose):

```sh
colophon new post "Raft leader election" --author ada --persona technical --tag distributed
# wrote:  content/posts/raft-leader-election.md
# slug:   posts/raft-leader-election
# url:    /posts/raft-leader-election/   (preview: colophon serve)
```

- `--author` / `--persona` are validated against `authors/*.yaml` / `personas/*.yaml` (errors
  list the valid ids); both are optional.
- the slug derives from the title and is made unique — `--unique=hash` (default) appends a
  short id on a collision, `--unique=counter` appends `-2`; `--slug` sets it explicitly.
- `--in <source>` chooses the source to write into; `--print` emits to stdout instead of writing.

Preview, and jump straight to a page:

```sh
colophon serve --open=latest     # opens the newest post in the browser
colophon serve --open=sitemap    # also: home | atom | rss | json | robots | <slug>
```
`serve` also prints the home/latest/sitemap/feed URLs at startup, so an agent can read them
without a browser.

This is the core of the agent write-as flow: an agent picks a **voice** (persona) for style
and an **author** for the byline, fetches the write-as context, drafts in that voice, previews,
and publishes — all through the CLI, with deploy secrets resolved server-side and never passed
to the agent.

> Retrieval is built in memory on each call (zero state). A persisted/​semantic index is a
> future option; the command shape stays the same.
