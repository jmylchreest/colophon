#!/bin/sh
# Symlink the community themes (contrib/themes/) into themes/ so `colophon serve` can preview the
# flux/signal/obsidian envs. themes/ is gitignored — run this after checkout to populate it.
set -e
cd "$(dirname "$0")"
mkdir -p themes
for t in signal flux obsidian; do
  ln -sfn "../../../contrib/themes/$t" "themes/$t"
done
echo "linked: $(ls themes)"
