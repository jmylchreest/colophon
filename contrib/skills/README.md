# colophon agent skills

Authoring skills that teach an AI coding agent to drive colophon. Each is a portable
[`SKILL.md`](https://docs.claude.com/en/docs/claude-code/skills) (YAML frontmatter +
instructions) — the de-facto cross-tool format that **Claude Code and opencode both read
natively**, so one folder installs into either.

The throughline: **colophon is the context provider, the agent is the writer.** Skills use the
colophon CLI to discover voices, scaffold files, preview and publish; the agent writes the prose.
colophon never calls an LLM and never handles deploy secrets.

## The skills

| Skill | Does |
|-------|------|
| [`colophon-write`](skills/colophon-write/SKILL.md) | Draft a new post/page in a chosen author + persona voice. |
| [`colophon-edit`](skills/colophon-edit/SKILL.md) | Revise an existing post, preserving voice and frontmatter. |
| [`colophon-crosslink`](skills/colophon-crosslink/SKILL.md) | Add `[[wikilink]]` cross-references / related posts. |
| [`colophon-metadata`](skills/colophon-metadata/SKILL.md) | Fill the `seo:` block, description, and tags (reusing vocabulary). |
| [`colophon-publish`](skills/colophon-publish/SKILL.md) | Validate, preview, and deploy (gated; secrets stay in the env). |

## Requirements

These skills drive the `colophon` CLI — they don't bundle it. You need the `colophon` binary on
`PATH` and a project with a `colophon.yaml`. Install the binary with:

```sh
go install github.com/jmylchreest/colophon/cmd/colophon@latest
```

(or grab a release binary). Each skill preflight-checks for `colophon` and, if it's missing,
asks before installing — it never installs software silently.

## Installing

### Claude Code — one command (plugin marketplace)

This repo is a Claude Code plugin marketplace, so you can install all the skills at once:

```text
/plugin marketplace add jmylchreest/colophon
/plugin install colophon-skills
```

### Any tool — copy the folders (the portable path)

`SKILL.md` is the lingua franca, so a skill folder drops straight into a tool's skills dir:

```sh
# Claude Code (personal, all projects)        ~/.claude/skills/
# Claude Code (one project)                   <project>/.claude/skills/
# opencode (see opencode.ai/docs/skills)       ~/.config/opencode/skill/  (or <project>/.opencode/skill/)
cp -r contrib/skills/skills/colophon-write ~/.claude/skills/
```

Copy whichever skills you want (or all five). Cross-tool sync utilities (e.g. community
`skills-supply` / `agent-skills-lint`) can install one skill repo across Claude Code, opencode
and others at once.

> Not Claude/opencode? `SKILL.md` is still just markdown — point any agent that supports skills
> at these folders, or paste a skill's body in as instructions.
