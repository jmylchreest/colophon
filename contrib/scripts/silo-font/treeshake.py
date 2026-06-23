#!/usr/bin/env python3
"""Tree-shake a Nerd Font down to colophon's webmention "silo" glyphs.

colophon shows a small brand icon next to a webmention when it recognises the source silo
(Bluesky, Mastodon, GitHub, X, …). Rather than ship a multi-MB Nerd Font, we subset one to just
those glyphs and ship the tiny woff2 with the embedded themes. This only needs re-running when a
network is added or removed — rare — so it lives here in contrib, not inside colophon.

It resolves each silo to a glyph *by name* from the source font's cmap (so it works across Nerd
Font versions and codepoint shuffles) and warns about any the source lacks. Bluesky and X are only
in recent Nerd Fonts releases — point --font at a current one (https://www.nerdfonts.com/).

Usage:
    ./treeshake.py --font /path/to/SomeNerdFont-Regular.ttf \
                   --out ../../../internal/build/assets

Outputs into --out:
    silos.woff2   the subsetted font (declare it @font-face as font-family: "Colophon Silos")
    silos.json    {silo: "<hex codepoint>"} manifest — update the CSS + Go/JS host→silo map from this

Requires: fontTools (pyftsubset) — `pip install fonttools brotli`.
"""

import argparse
import json
import os
import subprocess
import sys

from fontTools.ttLib import TTFont

# silo id -> ordered candidate Nerd Font glyph names; the first present in the source font wins.
# Names follow the Nerd Fonts cheat-sheet (fa- = Font Awesome, md- = Material, etc.).
SILOS = {
    "bluesky": ["fa-bluesky", "md-bluesky", "fae-bluesky"],
    "mastodon": ["fa-mastodon", "md-mastodon"],
    "github": ["fa-github", "fa-square_github", "oct-mark_github", "dev-github_badge"],
    "x": ["fa-x_twitter", "fa-square_x_twitter", "fa-twitter"],
    "linkedin": ["fa-linkedin", "fa-linkedin_in", "dev-linkedin"],
    "reddit": ["fa-reddit", "fa-reddit_alien"],
    "rss": ["fa-rss", "cod-rss"],
}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--font", required=True, help="source Nerd Font (.ttf/.otf) with the brand glyphs")
    ap.add_argument("--out", default=".", help="output directory for silos.woff2 + silos.json")
    args = ap.parse_args()

    font = TTFont(args.font, fontNumber=0)
    name_to_cp: dict[str, int] = {}
    for cp, gname in font.getBestCmap().items():
        name_to_cp.setdefault(gname, cp)

    manifest: dict[str, str] = {}
    missing: list[str] = []
    for silo, candidates in SILOS.items():
        cp = next((name_to_cp[c] for c in candidates if c in name_to_cp), None)
        if cp is None:
            missing.append(silo)
            continue
        manifest[silo] = f"{cp:04x}"

    if not manifest:
        sys.exit("error: none of the silo glyphs were found in the source font")

    os.makedirs(args.out, exist_ok=True)
    woff2 = os.path.join(args.out, "silos.woff2")
    unicodes = ",".join("U+" + cp for cp in manifest.values())

    # Subset to a temporary TTF, then compress to woff2. We use the standalone `woff2_compress`
    # (woff2 package) rather than fontTools' --flavor=woff2, which needs the brotli Python module.
    tmp_ttf = os.path.join(args.out, "silos.subset.ttf")
    subprocess.run(
        ["pyftsubset", args.font, f"--unicodes={unicodes}",
         f"--output-file={tmp_ttf}", "--layout-features=", "--no-hinting",
         "--desubroutinize", "--name-IDs="],
        check=True,
    )
    try:
        subprocess.run(["woff2_compress", tmp_ttf], check=True)  # writes silos.subset.woff2
        os.replace(os.path.join(args.out, "silos.subset.woff2"), woff2)
    except FileNotFoundError:
        sys.exit("error: woff2_compress not found — install the 'woff2' package (or `pip install brotli` and use fontTools)")
    finally:
        if os.path.exists(tmp_ttf):
            os.remove(tmp_ttf)
    with open(os.path.join(args.out, "silos.json"), "w") as fh:
        json.dump({"family": "Colophon Silos", "glyphs": manifest}, fh, indent=2)
        fh.write("\n")

    size = os.path.getsize(woff2)
    print(f"wrote {woff2} ({size} bytes) + silos.json")
    print("glyphs:", ", ".join(f"{k}=U+{v.upper()}" for k, v in sorted(manifest.items())))
    if missing:
        print(f"\nMISSING from this font (no glyph): {', '.join(missing)}", file=sys.stderr)
        print("→ use a newer Nerd Font (Bluesky/X need a recent release) or accept those silos", file=sys.stderr)
        print("  showing no icon. Then update the CSS @font-face + the host→silo map from silos.json.", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
