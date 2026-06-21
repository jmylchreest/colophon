# Arch User Repository (AUR) packages

PKGBUILDs for colophon. Two flavours, mutually exclusive:

| Package | Builds | Use when |
|---------|--------|----------|
| [`colophon`](colophon/) | from source (`go`, the tagged source tarball) | you want a from-source build |
| [`colophon-bin`](colophon-bin/) | from the prebuilt release binary | you want a fast install, no Go toolchain |

Both `provides=('colophon')` and conflict with each other, so only one is installed at a time.

## Status

These are **not yet wired into the release pipeline** — they're maintained here and published to
the AUR by hand. Each carries the version + checksums for the matching colophon release.

## Updating for a new release

From the package directory, after a release is published:

```sh
cd contrib/aur/colophon-bin       # or contrib/aur/colophon
# bump pkgver, then refresh the sha256sums from the release:
updpkgsums
makepkg --printsrcinfo > .SRCINFO
makepkg -f                        # local build test (validates checksums + packaging)
```

## Publishing to the AUR

One-time per package (requires an AUR account with your SSH key registered):

```sh
git clone ssh://aur@aur.archlinux.org/colophon-bin.git aur-colophon-bin
cp contrib/aur/colophon-bin/{PKGBUILD,.SRCINFO} aur-colophon-bin/
cd aur-colophon-bin && git commit -am "colophon-bin 0.0.4-1" && git push
```
