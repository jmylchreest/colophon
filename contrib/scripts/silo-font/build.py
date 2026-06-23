#!/usr/bin/env python3
"""Build colophon's silo icon font by merging curated glyphs from multiple font packs.

Reads silos.toml, locates/fetches each source font, extracts the named glyph for each silo,
reassigns it a colophon PUA codepoint, merges everything into one subset, and writes
`silos.woff2` + `silos.json` (the {silo: codepoint} manifest) into the configured output dir.

No single pack has every mark we want (e.g. Bluesky/X are only in current Font Awesome Brands),
so we curate from several and ship one tiny font. Run rarely — only when a silo changes.

Requires: fonttools (pyftsubset/subset, merge), and woff2 (woff2_compress).

    ./build.py                 # uses silos.toml beside this script
    ./build.py --config x.toml # alternate config (for testing)
"""

import argparse
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import tomllib
import urllib.request

from fontTools.merge import Merger
from fontTools.subset import Options, Subsetter
from fontTools.ttLib import TTFont
from fontTools.ttLib.scaleUpem import scale_upem

HERE = os.path.dirname(os.path.abspath(__file__))


def resolve_source(src: dict, cache: str) -> str | None:
    """Return a local path to a source font, fetching+caching a url: source if needed."""
    if src.get("path"):
        p = os.path.expanduser(src["path"])
        return p if os.path.exists(p) else None
    url = src.get("url")
    if not url:
        return None
    os.makedirs(cache, exist_ok=True)
    dest = os.path.join(cache, hashlib.sha1(url.encode()).hexdigest()[:12] + os.path.splitext(url)[1])
    if not os.path.exists(dest):
        print("fetching", url)
        urllib.request.urlretrieve(url, dest)
    return dest


def glyph_name(font: TTFont, ref: str) -> str | None:
    """Resolve a config glyph reference (a glyph name or 'U+XXXX') to a glyph name in font."""
    cmap = font.getBestCmap()
    if ref.upper().startswith("U+"):
        return cmap.get(int(ref[2:], 16))
    if ref in font.getGlyphOrder():
        return ref
    for cp, gn in cmap.items():
        if gn == ref:
            return gn
    return None


def extract(path: str, ref: str, target_cp: int, upem: int, outpath: str) -> bool:
    """Subset path to one glyph, scale to upem, remap it to target_cp, save to outpath."""
    f = TTFont(path, fontNumber=0)
    if f["head"].unitsPerEm != upem:
        scale_upem(f, upem)
    gn = glyph_name(f, ref)
    if not gn:
        return False
    opt = Options()
    opt.glyph_names = True
    opt.name_IDs = []
    opt.layout_features = []
    opt.drop_tables += ["PfEd", "TeX", "BDF", "FFTM"]
    ss = Subsetter(options=opt)
    ss.populate(glyphs=[gn])
    ss.subset(f)
    for st in f["cmap"].tables:
        st.cmap = {target_cp: gn}
    f.save(outpath)
    return True


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--config", default=os.path.join(HERE, "silos.toml"))
    args = ap.parse_args()

    with open(args.config, "rb") as fh:
        cfg = tomllib.load(fh)
    cfg_dir = os.path.dirname(os.path.abspath(args.config))
    out_dir = os.path.normpath(os.path.join(cfg_dir, cfg["output"]))
    upem = int(cfg.get("upem", 1000))
    next_cp = int(cfg.get("codepoint_start", "F300"), 16)
    cache = os.path.join(HERE, ".cache")
    sources = cfg["sources"]

    tmp = tempfile.mkdtemp()
    parts, manifest, missing = [], {}, []
    for silo in cfg["silo"]:
        sid = silo["id"]
        src = sources.get(silo["source"])
        if src is None:
            sys.exit(f"silo {sid}: unknown source {silo['source']!r}")
        path = resolve_source(src, cache)
        if not path:
            missing.append(f"{sid} (source {silo['source']!r} has no usable path/url)")
            continue
        target = int(silo["codepoint"], 16) if silo.get("codepoint") else next_cp
        part = os.path.join(tmp, f"{sid}.ttf")
        if extract(path, silo["glyph"], target, upem, part):
            parts.append(part)
            manifest[sid] = f"{target:04x}"
            if not silo.get("codepoint"):
                next_cp += 1
        else:
            missing.append(f"{sid} (glyph {silo['glyph']!r} not found in {silo['source']!r})")

    if not parts:
        sys.exit("error: no glyphs extracted — check the source paths/urls and glyph names")

    os.makedirs(out_dir, exist_ok=True)
    merged_ttf = os.path.join(tmp, "silos.ttf")
    if len(parts) == 1:
        os.replace(parts[0], merged_ttf)
    else:
        Merger().merge(parts).save(merged_ttf)
    subprocess.run(["woff2_compress", merged_ttf], check=True)  # writes silos.woff2 beside it
    os.replace(os.path.join(tmp, "silos.woff2"), os.path.join(out_dir, "silos.woff2"))
    with open(os.path.join(out_dir, "silos.json"), "w") as fh:
        json.dump({"family": cfg.get("family", "Colophon Silos"), "glyphs": manifest}, fh, indent=2)
        fh.write("\n")

    size = os.path.getsize(os.path.join(out_dir, "silos.woff2"))
    print(f"wrote {out_dir}/silos.woff2 ({size} bytes) + silos.json")
    print("glyphs:", ", ".join(f"{k}=U+{v.upper()}" for k, v in manifest.items()))
    if missing:
        print("\nMISSING (no icon shipped for these silos):", file=sys.stderr)
        for m in missing:
            print("  -", m, file=sys.stderr)
        print("→ point the source at a current pack (Bluesky/X need Font Awesome 6 Brands or a recent Nerd Font).", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
