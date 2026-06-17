# local publisher

Copies the built tree to a **directory on disk**. Used for offline preview, diffing a build, or
staging output a later step (a `command` publisher, an external sync) picks up. It's incremental:
it diffs the destination by content hash, writes only changed files, and (by default) removes
files no longer in the build.

## Settings

| Setting | Required | Purpose |
| ------- | -------- | ------- |
| `path` | yes | Destination directory. Relative paths resolve against the project root; absolute paths are used as-is. |
| `delete_orphaned` | no | Remove destination files no longer in the build (default `true`). Set `false` to only add/update. |

All settings support `{env:VAR}` / `{env:VAR:-default}`
[config interpolation](../../../docs/publishing.md#configuration-and-interpolation), e.g.
`path: "{env:OUT_DIR:-./dist}"`.

There are no credentials — it writes to the local filesystem with your privileges.

## Config

```yaml
publishers:
  - id: local
    driver: local
    path: ./dist
```

A single `local` publisher can write a **different directory per environment** via
[per-environment overrides](../../../docs/publishing.md#per-environment-overrides) — handy for
previewing themes side by side:

```yaml
environments:
  - name: dist
    publish: [local]            # → ./dist
  - name: text
    publish: [local]
    theme: minimal
    overrides:
      local:
        path: ./dist-text       # same publisher, a different dir
```
