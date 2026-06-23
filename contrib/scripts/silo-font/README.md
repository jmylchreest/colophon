# Silo icon font (curated multi-source subset)

colophon shows a small brand icon next to a webmention when it recognises the source silo
(Bluesky, Mastodon, GitHub, X, …). Rather than ship a multi-MB icon font, we **curate** just the
glyphs we use — pulling each from whichever pack has the best/most-current version — and **merge**
them into one tiny `silos.woff2` shipped with the embedded themes.

No single pack has everything: Bluesky and X live in **current Font Awesome 6 Brands**, while other
marks are fine from a **Nerd Font**. So the build is config-driven over multiple sources.

This is a **maintainer** tool — run only when a silo is added or removed (rare) — which is why it's
in `contrib/` and not inside colophon.

## Requirements

```sh
pip install fonttools          # subset + merge
# + a woff2 compressor:
#   Arch: pacman -S woff2   ·  Debian/Ubuntu: apt install woff2   ·  macOS: brew install woff2
```

## Configure (`silos.toml`)

```toml
output = "../../../internal/build/assets"   # where silos.woff2 + silos.json land
family = "Colophon Silos"
codepoint_start = "F300"                     # our PUA base; silos get sequential codepoints

[sources.nerd]
path = "/usr/share/fonts/.../SymbolsNerdFont-Regular.ttf"   # local path…
[sources.fa-brands]
url  = "https://…/Font Awesome 6 Brands-Regular-400.otf"    # …or a url (fetched + cached in .cache/)

[[silo]]
id = "bluesky"   # the id colophon's host→silo map uses
source = "fa-brands"
glyph = "bluesky"          # a glyph name in that source, or "U+XXXX"
```

Get **Font Awesome 6 Brands** (free desktop OTF) from <https://fontawesome.com/download> (or the
`@fortawesome/fontawesome-free` npm package, `otfs/Font Awesome 6 Brands-Regular-400.otf`), and a
current **Nerd Font** / Symbols Nerd Font from <https://www.nerdfonts.com/>.

## Build

```sh
./build.py            # reads silos.toml; fetches/locates sources, merges, writes the outputs
```

For each silo it scales the source to a common em, extracts the named glyph, reassigns it our PUA
codepoint, and merges all of them into one font. It writes:

- `silos.woff2` — the merged subset (a couple of KB); declare `@font-face { font-family: "Colophon Silos"; src: url(silos.woff2) }`
- `silos.json` — `{silo: "<hex codepoint>"}`, the source of truth for the CSS + the host→silo map

Any silo whose glyph isn't in its source is **skipped with a warning** (colophon then shows no icon
for it — it only shows an icon when certain of the silo).

## Wire it up (once, after a build)

From `silos.json`:

- add an `@font-face` for `Colophon Silos` and a rule per silo in each theme's CSS, e.g.
  `.silo[data-silo="bluesky"]::before { font-family: "Colophon Silos"; content: "\f300" }`
- map source host → silo id in `internal/build/mentions.go` and `internal/build/assets/mentions.js`
  (the build/JS set `data-silo="<id>"`), replacing the inline-SVG icons.

## Adding a network

Add a `[[silo]]` (and a `[sources.…]` if it's a new pack), re-run `./build.py`, then update the CSS
rules + host→silo maps from the new `silos.json`. That's the whole loop.
