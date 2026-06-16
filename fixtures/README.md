# Fixtures

Self-contained colophon projects for end-to-end testing — one per scenario. Each is a
real project (`colophon.yaml`, `content/`, `personas/`, a theme) that the CLI can build
and publish against.

| Fixture    | Sources                       | Purpose                                              |
| ---------- | ----------------------------- | --------------------------------------------------- |
| `local`    | `content/` (md-dir)           | Offline build + copy-to-dir; no credentials.        |
| `cf`       | `content/` (md-dir)           | Cloudflare Pages direct-upload deploy.              |
| `mixed`    | md-dir **+** Obsidian vault   | Same CF setup, publishing the **merged** content of multiple sources. |
| `obsidian` | Obsidian vault(s) only        | A site built entirely from one or more Obsidian vaults. |

`mixed` and `obsidian` read an Obsidian vault folder, pointed at by an env var so no
path is hard-coded:

```sh
export OBSIDIAN_BLOG="$HOME/OBSIDIAN/Blog/blog.i0.pm"   # your vault's blog folder
# obsidian fixture can take a second: export OBSIDIAN_BLOG2="$HOME/OBSIDIAN/Blog/notes"
```

An unset vault path simply contributes nothing (so `mixed` still builds from `content/`
alone). The Obsidian source publishes every note in the folder (`publish_required:
false`); set it `true` to require a `publish: true` flag per note. Vault folder structure
maps onto the site, and deletes/renames flow through the build reconciliation.

## Nothing sensitive lives here

Secrets are never written to these files:

- The Cloudflare **API token** is read only from `CLOUDFLARE_API_TOKEN`.
- Account id / project name resolve via `{env:...}` (`CLOUDFLARE_ACCOUNT_ID`,
  `CF_PAGES_PROJECT`), with a non-secret default project name.

Build output (`public/`, `dist/`) and derived state (`.colophon/`) are gitignored
per fixture.

## Running

Build a fixture (no credentials needed):

```sh
go build -o /tmp/colophon ./cmd/colophon
( cd fixtures/local && /tmp/colophon build )                # → fixtures/local/public/
( cd fixtures/local && /tmp/colophon publish --env dist )   # local publisher → fixtures/local/dist/
```

Deploy the Cloudflare fixture (see `internal/publish/cloudflare/README.md` for the
token permission — Account › Cloudflare Pages › Edit):

```sh
export CLOUDFLARE_API_TOKEN=...
export CLOUDFLARE_ACCOUNT_ID=...
export CF_PAGES_PROJECT=my-pages-project   # created automatically with --create

# production (gated → needs --allow-publish); --create makes the project if missing
( cd fixtures/cf && /tmp/colophon publish --env production --create --allow-publish )

# preview env: includes drafts, deploys to a Cloudflare Preview branch URL
( cd fixtures/cf && /tmp/colophon publish --env preview --create --allow-publish )
```

The `production` environment sets `allow_publish: false`, so it needs `--allow-publish`;
the `dist` environment is ungated. Pass several `--env` to publish multiple in one run.

## Adding a fixture

Scaffold a new project and wire an environment:

```sh
go run ./cmd/colophon init fixtures/<name>
# edit fixtures/<name>/colophon.yaml — publishers (mechanism) + environments (what/where);
# keep secrets in env vars
```
