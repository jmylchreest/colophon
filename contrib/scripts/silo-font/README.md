# Silo icon font (tree-shaken Nerd Font subset)

colophon shows a small brand icon next to a webmention when it recognises the source silo
(Bluesky, Mastodon, GitHub, X, …). Instead of shipping a multi-MB Nerd Font, we **subset** one
down to just those glyphs and ship the resulting tiny `silos.woff2` with the embedded themes.

This is a **maintainer** tool, run only when a network is added or removed (rare) — that's why it
lives in `contrib/` and not inside colophon itself.

## Requirements

```sh
pip install fonttools          # provides pyftsubset
# plus a woff2 compressor — either the standalone tool…
#   Arch: pacman -S woff2     ·  Debian/Ubuntu: apt install woff2     ·  macOS: brew install woff2
# …or `pip install brotli` (then fontTools can emit woff2 itself)
```

You also need a **current Nerd Font** as the source — **Bluesky and X glyphs only exist in recent
Nerd Fonts releases** (older fonts have GitHub/Mastodon/Twitter but not Bluesky/X). Get one from
<https://www.nerdfonts.com/font-downloads> (e.g. *Symbols Nerd Font*) — any patched font works; the
script reads glyphs by name, so the family doesn't matter.

## Usage

```sh
./treeshake.py --font /path/to/SymbolsNerdFont-Regular.ttf --out ../../../internal/build/assets
```

Outputs into `--out`:

- `silos.woff2` — the subsetted font; declare it `@font-face { font-family: "Colophon Silos"; … }`
- `silos.json` — `{silo: "<hex codepoint>"}`, the source of truth for the CSS + the host→silo map

It resolves each silo to a glyph **by name** from the source font (robust across versions) and
**warns** about any the source lacks — those silos simply show no icon (colophon only shows an icon
when it's certain of the silo).

## Adding a network

1. Add the silo id + candidate Nerd Font glyph names to `SILOS` in `treeshake.py`.
2. Re-run the script (against a Nerd Font that has the glyph).
3. From the new `silos.json`, update:
   - the `@font-face` glyph rules in each theme's CSS (`.silo[data-silo="<id>"]::before { content: "\<cp>" }`)
   - the host→silo mapping in `internal/build/mentions.go` and `internal/build/assets/mentions.js`.

## Why a font, not inline SVG

Centralises icons (one subset, easy to extend) and lets theme editors swap their own. The cost is
a font asset + this build step; the subset is only a couple of KB, so the weight is negligible.
(colophon falls back to nothing — not a generic icon — for unrecognised silos.)
