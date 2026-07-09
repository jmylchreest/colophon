# colophon

> Write it once. Own it forever. Publish anywhere.

A static site generator **for bloggers** — a simple CLI that turns a folder of Markdown (or any
other supported source, such as an Obsidian vault) into a fast, themed blog. Built on a principle
of **freedom**: own your words, host them anywhere (Cloudflare Pages, an S3/R2 bucket, a git
branch, or any command), and federate them (RSS/Atom/JSON feeds, IndieWeb microformats) so your
writing is never tied to a platform.

## Features

- **Markdown in, static site out** — `md-dir` and Obsidian-vault sources; code highlighting,
  maths (KaTeX), diagrams (Mermaid), callouts, wikilinks, embeds, series, tags and a glossary.
- **Themes** — embedded `press` (with broadsheet/gazette variants), `default` and `minimal`,
  plus community themes in [`contrib/themes/`](contrib/themes); per-environment overrides;
  progressive enhancement, so every theme works with JavaScript off.
- **Static search** — a fully static, sharded index (no server, no third party) with a tiny
  browser reader and optional fuzzy matching. A reusable module under [`search/`](search).
- **Rich media** — embed video/audio with plain image syntax (`![](demo.mp4)` → a player) and
  attach downloadable files (scripts, datasets, PDFs) to a post — copied/routed like images.
- **AI media generation** *(opt-in)* — generate images from a `gen:<prompt>` reference and
  spoken (TTS) audio readings of posts, via configured providers (Google, OpenAI-compatible,
  MiniMax). Results are content-addressed and cached; secrets stay in the environment.
- **Feeds & SEO** — RSS, Atom and JSON feeds (podcast-style enclosures for audio/attachments);
  canonical / Open Graph / Twitter / JSON-LD metadata; sitemap and robots.
- **Authors & personas** — a shown **author** (byline + h-card, Gravatar supported) vs a hidden
  **persona** (a reusable writing voice); `persona context` emits write-as context for an AI.
- **AI authoring skills** — optional Claude Code / opencode skills that drive colophon to write,
  edit, cross-link and publish — the engine supplies the voice, never an LLM or your secrets.
- **Pluggable publishers** — Cloudflare Pages, Cloudflare R2 / S3 (SigV4), local mirror, git
  branch, or any command. Incremental: only changed files upload, orphans are pruned.

## Install

**Download a prebuilt binary** — grab the archive for your OS/arch from the
[Releases](https://github.com/jmylchreest/colophon/releases) page, extract it, and put `colophon`
on your `PATH`:

```bash
# example: Linux x86-64 (swap in your platform's asset name)
curl -sSL https://github.com/jmylchreest/colophon/releases/latest/download/colophon_VERSION_linux_amd64.tar.gz | tar -xz
sudo mv colophon /usr/local/bin/
```

Prebuilt for Linux, macOS and Windows on amd64/arm64; each release lists `checksums.txt`.

**Or with Go** (1.26+): `go install github.com/jmylchreest/colophon/cmd/colophon@latest`, or build
from a clone — `git clone … && cd colophon && go build -o colophon ./cmd/colophon`.

## Quick start

```bash
colophon init mysite && cd mysite
colophon new post "Hello World"   # scaffold a post in content/
colophon serve                    # preview at http://localhost:8080 with live reload
colophon build                    # render the static site to ./public
```

`colophon init` writes an annotated `colophon.yaml`. Run `colophon <command> --help` for any of:
`init`, `new`, `build`, `serve`, `publish`, `themes`, `authors`, `persona`, `sources`, `posts`,
`search`, `doctor`, `env`.

## Configure & publish

A minimal `colophon.yaml` — a site, a theme, a feed, and where to deploy:

```yaml
sites:
  - id: main
    title: "My Site"
    base_url: "https://example.com"
    theme: press
    search: lexical
    federation:
      feeds: [rss, atom, json]

publishers:
  - id: cf
    driver: cloudflare-pages
    project: my-site
    account_id: "{env:CLOUDFLARE_ACCOUNT_ID}"   # non-secrets can interpolate the environment

environments:
  - name: production
    publish: [cf]
    allow_publish: false        # safety latch — deploying requires --allow-publish
```

Deploy credentials are **never** written in config — they're read from the environment, so a
token never passes through the YAML (or an AI agent). Set the ones your publisher needs, then:

```bash
export CLOUDFLARE_API_TOKEN=…        # Account → Cloudflare Pages → Edit
colophon publish --env production --allow-publish --create   # --create provisions the target
```

Credentials by publisher:

| Publisher | Secret environment variables | Token permission |
|-----------|------------------------------|------------------|
| `cloudflare-pages` | `CLOUDFLARE_API_TOKEN` | Account → Cloudflare Pages → **Edit** |
| `cloudflare-r2` | `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` (or `AWS_*`) | R2 → **Object Read & Write** |
| `s3` / `tigris` | `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Object read & write |
| `git` / `github-pages` | `GITHUB_TOKEN` / `GH_TOKEN` (HTTPS remotes) | Repo contents → **write** (SSH uses your agent) |

You can route assets (images, audio, attachments) to R2/S3 while HTML stays on your host — see
**[Publishing](docs/publishing.md)** for object storage, routing, `--create` CORS, and the
`command` escape hatch (surge, Netlify, rsync, …).

## AI media generation

Opt in with a `generation:` block; keys come from the environment. Reference a prompt anywhere an
image goes (`hero: "gen:a lighthouse on a rocky coast"`), or give a post an audio reading:

```yaml
generation:
  image:
    provider: minimax            # google | openai | xai | minimax | together | deepinfra | custom
  speech:
    provider: minimax            # text-to-speech reading → themed player + podcast enclosure
    voice: "English_Graceful_Lady"
```

| Image provider | Default model | API key |
|---|---|---|
| `google` | `gemini-3.1-flash-image` | `GEMINI_API_KEY` |
| `minimax` | `image-01` | `MINIMAX_API_KEY` |
| `openai` | `gpt-image-1` | `OPENAI_API_KEY` |
| `xai` | `grok-imagine-image-quality` | `XAI_API_KEY` |

Full details — caching, providers, house style, the `--generate-ai` step and the kill
switch — in **[Image & audio generation](docs/image-generation.md)**.

## Themes

Pick a theme with `theme:` in `colophon.yaml`. List, inspect or eject the bundled ones with
`colophon themes`; community themes live in [`contrib/themes/`](contrib/themes) (flux, obsidian,
signal). To customise, eject a theme or write your own — see **[Themes](docs/themes.md)** for the
template variables, progressive-enhancement contract, and base-theme inheritance.

## AI authoring skills

colophon ships agent skills (write, edit, cross-link, metadata, publish) that teach an AI coding
agent to drive it — the engine supplies the voice and scaffolding, the agent writes the prose.
It never calls an LLM or touches your deploy secrets.

The binary installs them into whatever harness you use:

```sh
colophon skills detect    # list detected harnesses + per-skill status (installed/outdated/…)
colophon skills install   # install or update the skills into the detected harnesses
```

Skills go to the tool-neutral `~/.agents/skills/` (read by **Codex, opencode, Cursor, Copilot**),
plus `~/.claude/skills/` (**Claude Code**) and `~/.gemini/skills/` (**Gemini CLI**). Each installed
file carries a version marker, so re-running `install` updates stale copies and won't overwrite
local edits without `--force`. Scope with `--harness=…`, `--dir=PATH`, or `--all`.

For **Claude Code**, `install` asks whether to use the self-updating marketplace plugin or copy
files (set `--claude=marketplace|files|skip` to choose non-interactively). The plugin (this repo
is a marketplace) is:

```text
/plugin marketplace add jmylchreest/colophon
/plugin install colophon-skills@colophon
```

See [`contrib/skills/`](contrib/skills/README.md) for the full list.

## Documentation

| Guide | |
|-------|--|
| [Authoring content](docs/content.md) | Frontmatter, Markdown, rich blocks, media embeds, attachments, wikilinks |
| [Themes](docs/themes.md) | Selecting/writing themes, template variables, progressive enhancement |
| [Publishing](docs/publishing.md) | Environments vs publishers, credentials, object-storage routing |
| [Image & audio generation](docs/image-generation.md) | `gen:` prompts, TTS readings, providers, caching |
| [Authors & personas](docs/personas.md) | Bylines & h-cards (Gravatar) vs hidden writing voices |
| [SEO & social](docs/seo.md) | The `seo:` block and the metadata colophon emits |

Design notes and the roadmap live in [`docs/PLAN.md`](docs/PLAN.md).

## Layout

```
cmd/colophon     CLI entrypoint
internal/        engine — render, build, sources, publishers, serve, config
search/          reusable static-search module (Go + JS, parity-tested)
contrib/themes/  community themes
contrib/skills/  AI agent skills (Claude Code / opencode plugin)
docs/            authoring, theming and publishing guides
fixtures/        end-to-end example sites
```

## License

[Apache License 2.0](LICENSE) © John Mylchreest.
