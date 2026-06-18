# colophon

> Write it once. Own it forever. Publish anywhere.

A themed Markdown static-site generator with pluggable publishers. Point it at a folder of
Markdown (or an Obsidian vault), pick a theme, and deploy the rendered site to Cloudflare Pages,
an S3/R2 bucket, a git branch, or anywhere a command can reach.

## Features

- **Markdown in, static site out** — `md-dir` and Obsidian-vault sources; goldmark rendering with
  code highlighting, maths (KaTeX), diagrams (Mermaid), callouts, wikilinks and embeds.
- **Themes** — embedded `press` (default, with variants) and `minimal`, plus community themes in
  `contrib/`; per-environment overrides; progressive enhancement (usable without JS).
- **Pluggable publishers** — Cloudflare Pages, Cloudflare R2 / S3 (SigV4), local mirror, git
  branch, or an arbitrary command. Incremental: only changed files upload, orphans are pruned.
- **Static search** — a fully static, sharded index (no server, no third party) with a tiny
  browser reader and optional fuzzy matching. Built as a reusable module under [`search/`](search).
- **Feeds & SEO** — RSS, Atom and JSON feeds; canonical / Open Graph / Twitter / JSON-LD metadata;
  sitemap and robots.
- **Authors & personas** — a shown **author** (byline + h-card) versus a hidden **persona** (a
  reusable writing voice); `persona context` emits write-as context for an AI author.
- **Environments** — one config drives many targets (production, preview, drafts, theme previews)
  with per-env overrides, and `serve` for local preview with live reload.

## Quick start

Requires Go 1.26+.

```bash
git clone git@github.com:jmylchreest/colophon.git
cd colophon
go build -o colophon ./cmd/colophon

./colophon init mysite && cd mysite
../colophon serve     # local preview at http://localhost:8080 with live reload
../colophon build     # render to ./public
../colophon publish --env production --allow-publish
```

## Commands

`init`, `new post|page`, `build`, `serve`, `publish`, `themes`, `authors`, `persona`, `sources`,
`posts`, `search`, `doctor`, `env`. Run `colophon <command> --help` for details.

## A little help from AI

colophon ships agent skills (write, edit, cross-link, metadata, publish) that teach an AI coding
agent to drive it — colophon supplies the voice and scaffolding, the agent writes the prose. It
never calls an LLM or touches your deploy secrets.

Claude Code (this repo is a plugin marketplace):

```text
/plugin marketplace add jmylchreest/colophon
/plugin install colophon-skills@colophon
```

opencode or any other tool — drop the `SKILL.md` folders into its skills directory:

```sh
cp -r contrib/skills/skills/* ~/.config/opencode/skill/   # or ~/.claude/skills/
```

See [`contrib/skills/`](contrib/skills/README.md) for the full list and details.

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

## Documentation

See [`docs/`](docs/README.md) for authoring, theming, publishing and persona guides; design notes
and the roadmap live in [`docs/PLAN.md`](docs/PLAN.md).
