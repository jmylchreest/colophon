# Design: publishing from Obsidian

> Status: **thin design** · relates to PLAN §7 (Sources)

Goal: a "publish / preview" flow from an Obsidian vault with the **smallest integration
footprint**, keeping colophon a plain CLI that reads markdown and publishes.

## Key constraint: CI cannot see the live vault

A vault lives on your machine (and Obsidian Sync). A CI/CD runner has no access to it.
So any CI-based publish requires the publishable content to **reach a git repo the
runner can clone**. The important realization:

> colophon's `obsidian` source reads `.md` files from a directory — it does **not**
> require the Obsidian app. A checked-out repo folder is just as readable as a live
> vault.

So "give CI access to the vault" = "commit the publishable markdown to a repo." The
Obsidian → colophon normalization (publish-flag filter, wikilink resolution) then runs
in CI, at build time, by colophon — on the committed `.md` files.

## Two deployment models (same source driver)

| | Local | Git + CI |
|---|---|---|
| Where colophon runs | your machine | CI runner |
| Reads | the live vault path | the committed repo snapshot |
| Secrets (`CLOUDFLARE_API_TOKEN`) | on the device | CI secret |
| Devices | desktop only | any (mobile Obsidian Git can push) |
| History | none implicit | every publish is a commit |
| Trigger | a button / `colophon publish` | `git push` |

Both use the **same `obsidian` source** (reads a folder of `.md`). The only difference
is where colophon runs and where the files are. `serve` (local, hot-reload) is the
instant-preview path in both models.

## The chain (git + CI)

```
Obsidian (write, publish: true)
   │  commit + push the publishable subset   ← Obsidian Git plugin (existing)
   ▼
Git repo  ──on push──▶  CI: colophon publish --env <production|preview>
                              (secrets in CI; obsidian source reads committed md)
   ▼
Cloudflare Pages   (PR/branch → preview env; main → production)
```

This composes with existing colophon pieces: environments→branches→CF environments,
`publish_after` + `next-build-time` (a scheduled CI run publishes embargoed posts when
due), and per-deploy `prune`.

## The "button"

1. **No custom plugin (recommended start):** use the existing **Obsidian Git** plugin's
   commit+push. colophon needs nothing.
2. **Thin status plugin (polish, later):** a small plugin that triggers commit+push then
   shows the resulting deploy URL/status. It never touches secrets or runs colophon.

## What colophon provides

- `obsidian` source (done): folder read + `publish: true` whitelist; folder structure →
  slug; deletes/renames flow through build reconciliation.
- A documented CI workflow (~15 lines) — to add.
- `next-build-time` (done) for scheduled embargo publishing.

## Open / dependencies

- **Vault privacy:** commit only a `Blog/` subfolder or rely on the publish-flag; private
  vaults → private repo.
- **Wikilinks** (`[[note]]`, `[[note|alias]]`): resolved at build via a cross-document
  link map. *In progress.*
- **Embeds** (`![[image.png]]`, `![[note]]`): image embeds depend on the **asset
  pipeline** (PLAN §6a) to copy/host the file; note transclusion is a later step. Until
  then, note links resolve and embeds are left untouched.
