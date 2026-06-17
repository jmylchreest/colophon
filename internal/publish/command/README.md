# command publisher

Deploys by running a **user-configured CLI command** against the built tree. It's the escape
hatch: any tool that takes a directory — [surge](https://surge.sh), Netlify, Vercel, Wrangler,
[exe.dev](https://exe.dev), Azure SWA, `rsync`, `scp`, `aws s3 sync`, or your own script — can
be a colophon publisher with no driver of its own.

## How it works

On publish, colophon:

1. **Materialises** the (routed) build tree into a temp directory.
2. Writes a **classification manifest** beside it (not inside — so it isn't published unless you
   opt in), listing each path's `kind` (page/asset/feed/sitemap/meta), `content_type` and `bytes`.
3. Runs your **command** with that directory as its working dir, the parent environment inherited
   (so the tool's own token var flows through), `COLOPHON_*` context injected, and `CI=true`.
4. A **non-zero exit** fails the publish; stdout/stderr stream to the log (`--verbose`).

## Config

```yaml
publishers:
  - id: surge
    driver: command
    command: ["surge", "{dir}", "myblog.surge.sh"]   # argv list — no shell
    public_url: "https://myblog.surge.sh"            # SURGE_TOKEN comes from the env

  - id: rsync
    driver: command
    host: "deploy@example.com:/var/www/blog"          # a custom setting → {host}
    command: ["rsync", "-az", "--delete", "{dir}/", "{host}"]
    public_url: "https://www.example.com"
```

The `command` is an **argv list executed directly — never through a shell**, so there's no shell
injection surface. For a one-liner that needs pipes or `&&`, make the shell explicit:

```yaml
    command: ["sh", "-c", "aws s3 sync {dir} s3://bucket --cache-control max-age=3600"]
```

## Interpolation

Every argument is interpolated with `{placeholder}` tokens. The namespace is **your own
publisher settings** plus colophon-owned runtime values (which win on a name clash). Unknown
placeholders are an error, so a typo fails loudly. `{{` / `}}` emit literal braces.

| Placeholder | Value |
| ----------- | ----- |
| `{dir}`, `{output_dir}` | Absolute path to the materialised tree (also the command's CWD). |
| `{manifest}` | Absolute path to the classification manifest JSON. |
| `{public_url}` | The configured `public_url`. |
| `{id}` | The publisher id. |
| `{driver}` | `command`. |
| `{file_count}` | Number of files in the tree. |
| `{<any setting>}` | Any key you declare on the publisher (e.g. `{host}`, `{domain}`, `{project}`). |

Per-environment `overrides` can vary any setting (and thus any interpolated value) per
environment — so one `command` publisher can target staging vs production without `{env}` plumbing.

## Environment handed to the command

The child inherits the parent environment (so `$SURGE_TOKEN`, `$VERCEL_TOKEN`, … work) plus:

| Var | Value |
| --- | ----- |
| `COLOPHON_OUTPUT_DIR` | Same as `{dir}`. |
| `COLOPHON_MANIFEST` | Same as `{manifest}`. |
| `COLOPHON_PUBLIC_URL` | The `public_url`. |
| `COLOPHON_PUBLISHER_ID` | The publisher id. |
| `COLOPHON_DRIVER` | `command`. |
| `CI` | `true` — nudges deploy CLIs into non-interactive mode. |

## Security

The command runs with your privileges, from your own config — the same trust level as a Makefile
or a CI script, and gated behind `--allow-publish` like every deploy. **Secrets never pass
through colophon or the command line:** tokens stay in the environment, where the target tool
reads them — matching colophon's env-only secrets rule and the deploy-CLI best practice of
keeping credentials out of argv (where they'd leak into process listings and shell history).
