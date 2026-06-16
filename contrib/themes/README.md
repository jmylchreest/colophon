# contrib/themes

Community / showcase themes that ship in the repo but are **not** embedded in the
`colophon` binary. The built-in themes (`default`, `minimal`, `press`) live in
[`internal/render/themes/`](../../internal/render/themes); everything here is meant to be
copied into a project.

## Themes

| Theme | Style |
|-------|-------|
| `flux` | Minimal serif editorial (Libre Baskerville / IBM Plex Sans), light & dark. |
| `signal` | Mono / terminal (JetBrains Mono / Syne), light & dark, reading progress. |
| `obsidian` | Magazine layout with a full-bleed hero, light & dark. |

## Using one

Copy it into your project's `themes/` directory and select it in `colophon.yaml`:

```sh
cp -r contrib/themes/flux myblog/themes/flux
```

```yaml
sites:
  - id: main
    theme: flux
```

Each theme carries only its own `page.html`, `index.html` and `style.css`. On-disk themes
inherit the built-in `default` theme, so the vendored web fonts and the highlight.js / KaTeX /
Mermaid libraries are supplied from `default` at build time — the theme doesn't duplicate
them. See [`docs/themes.md`](../../docs/themes.md) for the template variables and the
raw-block contract.

> The `fixtures/mixed` demo project symlinks these directories into its own `themes/` so
> `colophon serve` can preview them; that's a convenience for development, not a requirement.
