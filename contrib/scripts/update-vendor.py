#!/usr/bin/env python3
"""Refresh bundled browser libraries from integrity-pinned npm release archives."""

import base64
import hashlib
import io
import json
from pathlib import Path
import tarfile
from urllib.request import urlopen


def main():
    here = Path(__file__).resolve().parent
    vendor = here.parent.parent / "internal/render/themes/default/vendor"
    libraries = json.loads((here / "vendor-libraries.json").read_text())
    for name, release in libraries.items():
        with urlopen(release["url"], timeout=60) as response:
            archive = response.read()
        digest = "sha512-" + base64.b64encode(hashlib.sha512(archive).digest()).decode()
        if digest != release["integrity"]:
            raise ValueError(f"Integrity mismatch for {name}")
        with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as package:
            if name == "mermaid":
                files = {"package/dist/mermaid.min.js": "mermaid.min.js"}
            elif name == "katex":
                files = {"package/dist/katex.min.js": "katex.min.js",
                         "package/dist/katex.min.css": "katex.min.css"}
                files.update({m.name: m.name.removeprefix("package/dist/")
                              for m in package.getmembers()
                              if m.isfile() and m.name.startswith("package/dist/fonts/")})
            else:
                files = {"package/highlight.min.js": "highlight.min.js",
                         "package/styles/github.min.css": "github.min.css",
                         "package/styles/github-dark.min.css": "github-dark.min.css"}
            license_files = [m.name for m in package.getmembers()
                             if m.name in ("package/LICENSE", "package/LICENSE.md")]
            for license_file in license_files:
                files[license_file] = "LICENSE"
            for source, relative in files.items():
                path = Path(relative)
                if path.is_absolute() or ".." in path.parts:
                    raise ValueError(f"Unsafe archive path: {source}")
                member = package.getmember(source)
                if not member.isfile():
                    raise ValueError(f"Expected a regular file: {source}")
                target = vendor / name / path
                target.parent.mkdir(parents=True, exist_ok=True)
                with package.extractfile(member) as content:
                    target.write_bytes(content.read())
        print(f"Updated {name} to {release['version']}")


if __name__ == "__main__":
    main()
