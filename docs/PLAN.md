# colophon — Implementation Plan

> Status: **DRAFT for review** · Date: 2026-06-14 · Language: **Go**
>
> A simple, themed Markdown → static blog generator with a local CLI and an MCP
> server, publishing to multiple pluggable static hosts. IndieWeb-leaning,
> persona-aware, agent-friendly.

---

## 1. Principles

- **One canonical build, many destinations.** The generator produces a single
  static tree; *Publishers* fan it out (chain/mirror) to any number of targets.
- **Markdown-in, static-out, no runtime.** Output is plain HTML/CSS, JS-free by
  default (progressive enhancement only — search, webmention display).
- **Persona-first, AI-optional.** Content is attributed to a *persona* (identity),
  not a human. Multi-persona from day one; single-user is just "one default
  persona". Publishing as-is needs no AI; the style engine is an opt-in assist.
- **Agent-native via CLI + skill.** A clean CLI + bundled skill is the agent
  interface (portable, no server). Core is a library so an MCP layer can wrap it
  later. Drafts by default; publishing is explicitly gated.
- **Boring, embeddable dependencies.** Everything compiles into one static
  binary (`CGO_ENABLED=0`), cross-compiles trivially.

---

## 2. Tech stack (Go)

| Concern | Choice | Notes |
|---|---|---|
| CLI | `alecthomas/kong` | struct-tag declarative commands/flags; no cobra/viper |
| Config | `knadh/koanf` | YAML parser + `{env:VAR}` preprocessing; no viper |
| Markdown | `yuin/goldmark` | Hugo's engine; + extensions (GFM, footnotes, wikilink, attributes) |
| Wikilinks | custom goldmark extension | `[[note]]`, `[[note|alias]]`, `![[embed]]` |
| Syntax highlight | `alecthomas/chroma` | build-time, class-based CSS, no cgo |
| Templating | **`pongo2`** (Jinja2/Django syntax) | runtime-loaded, overridable theme files; behind a pluggable engine interface |
| Frontmatter | `adrg/frontmatter` (or hand-rolled) YAML/TOML | |
| Feeds | `gorilla/feeds` + custom JSON Feed | RSS/Atom/JSON |
| Search | **internal pure-Go index** (inverted/BM25 → JSON + tiny client JS) | no external tool; optional semantic layer (§8) |
| Images | `disintegration/imaging` (pure Go) | resize/responsive; optional govips later |
| OG images | `fogleman/gg` or `golang/freetype` | build-time card generation |
| Agent interface | **CLI + bundled skill** (§11) | no server/SDK; MCP optional later |
| MCP (optional, later) | `modelcontextprotocol/go-sdk` | thin wrapper over core, if ever needed |
| Git publisher | `go-git/go-git` | pure-Go, no shelling out |
| S3 publisher | `aws/aws-sdk-go-v2` | S3 + CloudFront invalidation |
| Cloudflare | net/http against CF API (direct upload) | wrangler optional fallback |
| Embeddings (optional) | one OpenAI-compatible client (OpenRouter/OpenAI/Ollama/LM Studio) | retrieval/search only; BM25 fallback needs none |
| Vector store | flat `float32` + brute-force cosine | no DB; `Retriever` iface, HNSW later |

**Templating note.** `templ` was rejected: it is a code generator (compiled in,
no runtime interpreter) so it cannot load user-overridable theme files at runtime.
`pongo2` (Jinja2/Django syntax) loads plain template files at runtime and shares
syntax with Tera (Zola) and Nunjucks (Eleventy), so themes are syntactically
portable across that family. True **Hugo** theme compatibility is *not* a goal —
Hugo themes depend on Hugo's function set/page model/lookup rules, not just Go
`html/template` syntax — but the renderer sits behind an interface so a Go
`html/template` (Hugo-ish) or Liquid (Jekyll-ish) engine could be added later.

---

## 3. Domain model

Persona ≈ a blog identity. Content is written once and bound to one-or-many
personas/sites via a **Publication** — so the same article can be published under
different personas (different voice/byline) and/or to different sites.
Conceptually: `(human + content) ⟷ (persona & site)`.

```
Content                      # the article (markdown), written once
  id, body, assets[]
  authored_via -> operator   # provenance/audit (human or agent) — NOT the byline

Persona (kind: individual | brand)   # ≈ a blog identity
  id, display_name, byline
  h-card: avatar, bio, urls, email
  style:
    guide              # system prompt / written style rules
    references[]       # links, glossaries, source docs
    corpus             # = the content published AS this persona (its publications)
  sites[]              # site(s) this persona publishes to (usually one)
  operators[]          # individual ⇒ 1 ; brand ⇒ many

Site
  id, title, base_url, theme, config
  personas[]   # personas allowed to publish here (restriction)
  publishers[] # deploy targets

Publication                  # binds Content × Persona (→ resolved site[])
  content_ref, persona_ref
  overrides: slug, date, publish_after, tags, draft, publish
  # byline/style/theme/publishers all derive from persona + site

Publisher (interface)  cloudflare-pages | s3-cloudfront | webdav | rsync-ssh | git-pages | local
```

Relationships & rules:
- `human → persona` is **1:many**; `persona(individual)` → exactly one operator;
  `persona(brand)` → many operators (the only sanctioned shared case).
- **One Content → many Publications** (same article, different persona/site).
- **Byline + style corpus attach to the persona**; provenance attaches to operator.
- A persona's **style corpus = the content published as that persona** (scopes retrieval).
- **Restriction**: persona declares `sites[]`, site declares allowed `personas[]`;
  a publication must satisfy both.

---

## 4. On-disk layout (a "blog project")

```
myblog/
├── colophon.yaml                 # site(s) + publishers config
├── personas/
│   ├── technical.yaml         # style guide, references, h-card, kind, operators
│   └── personal.yaml
├── content/                   # plain markdown (md-dir source + synced pull sources)
│   ├── posts/2026/hello.md
│   └── pages/about.md
├── assets/                    # asset bytes (packed) and/or pointer files (*.asset → external)
├── themes/
│   └── default/               # templates + css (overridable)
├── .colophon/
│   ├── manifests/             # per-publisher content-hash manifests (incremental)
│   ├── corpus/                # persona retrieval indexes
│   └── cache/                 # webmention cache, build cache
└── public/                    # build output (canonical static tree)
```

Frontmatter (per post):
```yaml
title: Hello World
date: 2026-06-14
persona: technical          # → one publication as 'technical' (sugar)
tags: [go, ssg]
draft: false                # manual gate: never published until flipped
publish: true               # Obsidian-compatible whitelist flag
publish_after: 2026-06-20T09:00:00Z   # optional "not before" / embargo (time gate)
syndicate: [mastodon]       # optional POSSE targets
```

`persona:` is sugar for a single-entry `publications:`. To publish the same
content under multiple personas/sites, use the list form (per-publication
overrides allowed):
```yaml
publications:
  - persona: technical
  - persona: personal
    slug: hello-from-me
    publish_after: 2026-07-01
```
v0.1 implements the singular `persona:` path; multi-publication lands in M2/M3.

### Scheduled / embargoed posts (`publish_after`)

A post may carry `publish_after` (a "not before" timestamp). Semantics:

| Context | Behaviour while `now < publish_after` |
|---|---|
| `colophon serve` / `colophon build --preview` | **Included**, marked `EMBARGOED until …` for drafting/preview |
| `colophon build` (production) | **Excluded** from HTML, feeds, sitemap, search |
| Any production build after the timestamp | **Included automatically**, no flag change |

`draft` vs `publish_after`: `draft: true` is a manual gate (you flip it);
`publish_after` is a time gate (auto-publishes, but only when a build runs past it).

**Static-site caveat:** publication is build-time, so an embargoed post appears
only when a build runs *after* its timestamp. colophon supports this two ways:
1. **Scheduled rebuild** — CI cron runs `colophon publish`; posts roll out on the
   first build past their time.
2. **Next-wake reporting** — `colophon build` prints the next pending `publish_after`,
   and `colophon next-build-time` returns it, so CI (or `/schedule`) can fire exactly
   then instead of polling.

Works via MCP too: an agent can `create_draft` with `publish_after`, preview it,
and it goes live on the next post-embargo build with no further action.

---

## 5. Config schema (`colophon.yaml`)

```yaml
sites:
  - id: main                  # internal handle, referenced elsewhere
    title: "John's Blog"       # templates, feeds, OG
    base_url: https://blog.example.com   # canonical root → absolute links, feeds, sitemap, mf2
    theme: default             # which themes/<dir> to render with
    personas: [technical, personal]      # personas ALLOWED here (restriction, §3)
    publishers: [cf-prod, s3-mirror]     # default deploy targets
    routing:                   # optional: route output subsets to different publishers
      - match: "assets/**"
        publisher: s3-mirror
        base_url: https://cdn.example.com   # asset URLs rewritten to this
      - match: "**"
        publisher: cf-prod     # everything else (HTML) → Cloudflare
    federation:
      feeds: [rss, atom, json] # always at least one
      indieweb:
        microformats: true
        webmention:
          receiver: https://webmention.io/blog.example.com/webmention
          bridgy_backfeed: true
      fediverse:
        bridgy_fed: true       # opt-in; forward-only
    search: lexical            # ON-SITE visitor search: lexical | semantic | off

publishers:
  - id: cf-prod
    driver: cloudflare-pages
    project: "{env:CF_PAGES_PROJECT}"    # values support {env:VAR} / {env:VAR:-default}
  - id: s3-mirror
    driver: s3-cloudfront
    bucket: "{env:S3_BUCKET}"
    distribution_id: E123ABC
    region: eu-west-1
  - id: local
    driver: local
    path: ./public
```

- **`search:`** controls the *visitor-facing* search feature on the generated
  site (not anything about publishing). `lexical` works offline; `semantic` needs
  an embedder/endpoint (§8).
- **Env interpolation:** any value may contain `{env:VAR}` (or `{env:VAR:-default}`),
  resolved at config load (server-side).
- **Secrets:** env vars / OS keyring, **never** passed through the MCP layer.

---

## 6. Build pipeline

```
sources → load → normalize → model → render → assets → federation → search → public/
```

1. **Load** — each enabled source emits raw documents (md-dir, obsidian).
2. **Normalize** — resolve wikilinks/embeds, apply `publish`/`draft`/`publish_after`
   filters, expand `persona:`/`publications:` into Publications.
3. **Model** — build the content graph (taxonomies, backlinks, per-persona archives).
4. **Render** — goldmark → HTML, apply theme templates, chroma highlight.
5. **Assets** — resolve asset references, copy/optimize images, responsive sizes,
   OG images; pack into output or upload to external store + URL-rewrite (§6a).
6. **Federation** — emit feeds, mf2 markup, sitemap, OG/Twitter meta; send webmentions (CI).
7. **Search** — build internal pure-Go index (inverted/BM25 → JSON + tiny client
   JS); optionally compute embeddings for the semantic layer (§8).
8. **Output** — canonical tree in `public/`; ready for Publishers.

Incremental: content-hash each input → skip unchanged render units; per-publisher
manifest drives incremental upload (§9).

### 6a. Assets, asset stores & references

An asset reference in content (`![alt](path)`, `![[image.png]]`, or `asset:<id>`)
resolves at build to bytes governed by a per-asset/per-glob **policy**:

- **packed** (default, zero-config): bytes live in the repo; files over a size
  threshold are **auto-routed to git-LFS** → copied into `public/` and carried by
  the pages publisher.
- **external**: repo holds only a small **pointer file** (`image.png.asset` → JSON
  `{store, key, hash, mime, w, h}`); at build, colophon ensures the object is uploaded
  to the asset store and **rewrites the page URL to the asset CDN base**. Bytes
  never touch git.

Asset stores are pluggable (local-git | git-lfs | any object publisher e.g. S3).
**AI image generation** (build step or MCP tool) writes generated bytes to the
store per policy — external-by-default keeps unreviewed AI output out of git; it
resolves like any other asset, reviewed via preview rather than a commit gate.
This is also what re-hosts **Notion's expiring image URLs** on sync (§7).

Combined with publisher **routing** (§9), this delivers "assets → S3, pages →
Cloudflare": route `assets/**` to one publisher (external) and `**` to another.

---

## 7. Sources

The `Source` interface abstracts origin; everything converges on plain markdown in
`content/` so the rest of the pipeline is uniform.

- **md-dir** (v0.1): folder of markdown + frontmatter; git-friendly LCD. *filesystem.*
- **obsidian-vault** (M5): point at a vault; `publish: true` whitelist, `[[wikilink]]`
  + `![[embed]]` resolution (incl. media), attachment handling. *filesystem, read in
  place.* The normalize layer isolates Obsidian quirks.
- **notion** (later): **API pull** → convert blocks to markdown → `colophon sync` writes
  into `content/`. Must download & re-host images (Notion serves expiring S3 URLs)
  via the asset store (§6a).
- **hackmd** (later): **API pull** (already markdown) → `colophon sync` into `content/`.
- **mcp-writes**: agent-created content lands in `content/` as normal markdown.

Design call: **pull sources (Notion/HackMD) sync *into* `content/`** rather than
read live at build — unifying representation and giving git history/diffs.
Filesystem sources (md-dir, Obsidian) are read in place.

---

## 8. Persona (identity) + optional style engine + search

**Persona is identity & attribution first — AI is optional.** The default path:
mark content `persona: technical` (or a human author) and publish it as-is. No
embeddings, no model, nothing AI. A fresh install needs no API key.

### Optional style-assist (only when generating via AI)

colophon does **not** generate — it **emits context**; the calling agent (e.g. Claude
via the skill, §11) is the intelligence and writes the prose. When asked for the
"write-as" context, colophon returns: **style guide + reference material + top-K
relevant exemplars** retrieved from that persona's own publications.

- **Indexing** (`colophon persona index <id>`): chunk → embed → store under
  `.colophon/corpus/<persona>/` (regenerable, not committed).
- **Dual chunking**: coarse (per-post / per-section) for *style* exemplars (preserve
  voice); fine (sliding window) for *search* recall. Same content, two indexes.
- **Pluggable embedder**: one **OpenAI-compatible** client (`base_url` + `model` +
  key) → works with OpenRouter / OpenAI / Ollama / LM Studio. **BM25/keyword
  fallback needs no model** and is the v0.1 default (zero-config).
- **Retrieval**: embed (or BM25) the query → top-K chunks + always include style
  guide + references. Exposed via the CLI (`colophon persona context <id> --topic …`).

### Vector store (no DB)

- Per index: `.colophon/corpus/<name>/vectors.f32` (raw `float32` N×D) + `meta.json`
  (per-row id, content_ref, span, persona, preview) + header (`dim`, `embedder-id`,
  content hashes for staleness/reindex).
- Query: **brute-force cosine** over the in-memory matrix → top-K. Sub-ms at blog
  scale, dependency-free. Behind a `Retriever` interface so a pure-Go **HNSW** index
  can drop in later without touching callers.

### Search (two surfaces, both internal)

- **Public site search = pure-Go lexical index** (inverted/BM25 → compact JSON +
  tiny client script). Fully static, no external tool, always available.
- **Semantic search** reuses the embedding subsystem. Catch: queries must be
  embedded *at search time*, which needs a model then. So:
  - **Agent/CLI**: `colophon search` does embedding retrieval locally — fully semantic.
  - **Public site**: lexical by default; semantic only if the user opts into a tiny
    OpenRouter-backed query endpoint, else it falls back to lexical.

---

## 9. Publisher abstraction

```go
type Publisher interface {
    ID() string
    Plan(ctx context.Context, tree fs.FS) ([]Change, error) // diff vs manifest
    Apply(ctx context.Context, changes []Change) (Result, error)
    Invalidate(ctx context.Context, paths []string) error   // cache bust
}
```

- **Manifest-driven incremental**: each publisher keeps a content-hash manifest
  (`.colophon/manifests/<id>.json` and/or remote). `Plan` diffs local hashes vs
  manifest → upload changed, delete removed. Idempotent.
- **Drivers**:
  - `local` — copy to a dir (also the v0.1 smoke test).
  - `cloudflare-pages` — direct-upload API (hashed file manifest is native to CF).
  - `s3-cloudfront` — `PutObject` changed keys + `CreateInvalidation` for changed paths.
  - `webdav` — PROPFIND + PUT/DELETE.
  - `rsync-ssh` — delegate to rsync delta transfer.
  - `git-pages` — `go-git` commit+push only the diff (GitHub/GitLab Pages).
- **Chain/mirror**: `publishers: [a, b, c]` deploy concurrently; per-target result
  reported; optional `require: [a]` gate so a primary must succeed.
- **Routing** (§5 `routing:`): path globs map output subsets to specific publishers
  with optional `base_url` URL-rewrite — e.g. `assets/** → s3` (external CDN) and
  `** → cloudflare-pages` (HTML). Default (no `routing:`) sends the whole tree to
  every listed publisher (pure mirror).

---

## 10. Federation & feeds  *(designed now, deferred to M4+ — parked)*

- **Always**: RSS 2.0 + Atom + JSON Feed + sitemap.xml + OpenGraph/Twitter cards.
- **IndieWeb**: mf2 (`h-entry` per post, `h-card` from persona), Webmention send
  (build/CI step), receive+display via webmention.io + Bridgy (cached JSON at build).
- **Fediverse**: opt-in Bridgy Fed reusing the feed/mf2 layer. Documented limits:
  forward-only, no backfill, skips posts >2 weeks old. **No self-hosted ActivityPub.**
- **POSSE**: `syndicate:` frontmatter drives optional cross-posting hooks (later).

---

## 11. Agent interface — CLI-first + skill (MCP optional later)

**Decision:** the agent interface is the **CLI plus a bundled skill**, not an MCP
server. More portable (any agent/automation that can run a shell), no long-running
process, no SDK dependency, everything stays git-versioned files. Persona is just a
`--persona` arg, so no session-scoping machinery is needed.

**Architecture rule:** build a **`core` library**; the CLI is a thin shell over it;
the skill wraps the CLI. If structured resources/prompts are ever wanted, an **MCP
server becomes another thin wrapper over the same core** — not a rewrite. So MCP is
an *optional later layer*, not a v0.1 requirement.

- **Discovery**: `colophon persona list --json` (the skill enumerates personas first).
- **Write-as flow** (the agent is the intelligence; colophon provides context + files):
  ```
  colophon persona context technical --topic "..."   # → style guide + top-K exemplars (text)
  # agent writes the post in that voice
  colophon new post --persona technical --file draft.md
  colophon preview                                    # → local URL
  colophon publish --allow-publish                    # gated (refuses without the flag)
  ```
- **Security**:
  - **draft-by-default** — nothing goes live implicitly.
  - **`publish` gated by `--allow-publish`** (flag/config); refuses otherwise.
  - **operator → persona** authorization on writes; `authored_via` audit trail.
  - **deploy secrets never pass through the agent** — resolved server-side at config
    load (env / keyring).
  - team-grade option: `git-pages` publisher → commit/PR, human/CI merges to deploy.

### 11a. Bundled skill
A markdown skill instructs the agent how to drive the CLI (list personas → fetch
write-as context → create draft → preview → publish). Ships with colophon; portable to
any skill-capable agent. JSON output (`--json`) on read commands makes parsing
robust.

---

## 12. CLI surface (cobra)

```
colophon init                       # scaffold a blog project
colophon new post [--persona X]     # create a draft
colophon build                      # sources → public/  (prints next pending embargo)
colophon next-build-time            # next publish_after timestamp (for CI/scheduling)
colophon serve                      # local preview (live reload, shows embargoed)
colophon publish [--allow-publish]  # build + deploy/mirror (gated)
colophon persona list|add|index     # manage personas + style corpus (--json)
colophon persona context X --topic  # emit style guide + top-K exemplars (write-as)
colophon search "query" [--json]    # semantic/lexical retrieval over content
colophon sync [source]              # pull API sources (notion/hackmd) into content/
colophon doctor                     # validate config/credentials
# colophon mcp                      # optional later layer (thin wrapper over core)
```

Read commands support `--json` so the skill (§11a) can parse output robustly.

---

## 13. Milestones

### M0 — Skeleton (after this plan is approved)
- repo scaffold, `go.mod`, cobra CLI skeleton, config load (`colophon.yaml`), domain types.

### M1 — Thin vertical slice (v0.1 core)
- **md-dir source** → goldmark render → **default theme** → `public/`.
- **Publishers**: `local` + `cloudflare-pages`, manifest-based incremental.
- **Feeds**: RSS + Atom.
- **`publish_after` embargo**: production build filter + `--preview` inclusion +
  next-build-time reporting.
- `colophon init / build / serve / publish / next-build-time`.

### M2 — Personas (multi-capable from day one)
- persona config + ownership, bylines, per-persona archive pages, h-card.
- `colophon persona` commands; corpus indexing + retrieval.

### M3 — Agent interface: CLI ergonomics + bundled skill (v0.1 completion)
- `--json` read commands, `persona context`, draft-by-default, gated `publish`.
- bundled skill driving the write-as → draft → preview → publish flow.
- (MCP server explicitly out of scope — optional later layer over the same core.)

### M4 — IndieWeb + more feeds + search
- mf2 markup, JSON Feed, sitemap, OG/Twitter cards, Webmention send + display.
- internal lexical search (public) + semantic `colophon search` (local embedding).
- multi-publication (`publications:` list — same content, many personas/sites).

### M5 — Obsidian source + more publishers + assets
- vault source (wikilinks/embeds/publish flag), `s3-cloudfront`, `webdav`,
  `rsync-ssh`, `git-pages`.
- asset system: packed (git/git-LFS) + external (pointer files + asset store) +
  publisher **routing** (assets→S3, pages→Cloudflare).

### M6 — Polish / advanced
- OG-image generation + **AI image generation** (external-by-default), responsive
  images, Bridgy Fed opt-in, POSSE hooks, incremental build cache.
- additional pull sources: **Notion** (with image re-hosting), **HackMD**.

---

## 14. Decisions log

**Resolved:**
- **Config format** → **YAML** (supports `#` comments; matches frontmatter; nests
  cleanly). `colophon init` ships an annotated, commented default `colophon.yaml`.
- **Embedder** → **OpenAI-compatible endpoint** (one client: `base_url` + `model` +
  key; works with OpenRouter / OpenAI / Ollama / LM Studio). **BM25 is the
  zero-config default**; embedder is pure opt-in for semantic search + better
  exemplar retrieval.
- **Default asset policy** → **packed**, with **auto-LFS** for files over a size
  threshold; `external` (S3 pointer) remains opt-in per-glob (§6a).

**Still open (non-blocking):**
- **Theme system depth**: one minimal default + override dir first; pluggable engine
  so a Hugo-ish `html/template` or Liquid renderer could be added later.
- **Webmention receiver** (decide at M4): hosted webmention.io vs self-hosted endpoint.
- **Public semantic search** (decide at M4): optional query endpoint vs public =
  lexical only (semantic stays agent/CLI-side).

---

## 15. Backlog — designed, not yet built

### i18n / multi-lingual (relates §10 feeds; see `docs/design/obsidian.md` for sources)
- **Language per content** via frontmatter `lang` (BCP-47, e.g. `en`, `pt-BR`), with
  source-level and site-level defaults. **Unconfigured default = `en`.**
- **Translation linking by convention** — derived from the source structure (shared
  slug across language path-prefixes), *not* manual `translation_key`s. The source
  provides the grouping per content; a source may also set a default `lang` for all its
  documents.
- **URL**: language as a path prefix `/<lang>/…` (reuses the `base_path` infra).
- **Per-language feeds** — required because RSS 2.0 `<language>` is channel-level /
  mono-lingual; Atom (`xml:lang`) and JSON Feed 1.1 (`language`) also support per-item
  language, but per-language feeds are the clean cross-format choice. Plus `<html lang>`
  and `hreflang` alternates in the sitemap and page `<head>`.

### Slug redirects for moved/renamed articles (relates §6, §10)
- **`aliases:` frontmatter** → emit a `_redirects` file (Cloudflare Pages / Netlify
  native 301s) **plus** portable HTML `<meta http-equiv="refresh">` + canonical stubs
  for any static host.
- **Later:** a stable `id`/`uuid` in frontmatter → auto-track renames via a durable
  `id → slug` map (the one piece of committed, non-scratch state); pairs well with
  Obsidian note renames.
```
