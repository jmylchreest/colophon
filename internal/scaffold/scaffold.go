// Package scaffold writes the initial file tree for a new colophon project.
package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

type file struct {
	path string
	body string
}

// Project writes a starter project rooted at dir, creating directories as needed.
func Project(dir string) error {
	files := []file{
		{"colophon.yaml", configYAML},
		{"authors/me.yaml", authorYAML},
		{"personas/default.yaml", personaYAML},
		{filepath.Join("content", "posts", "hello-world.md"), helloPost},
		{filepath.Join("content", "pages", "about.md"), aboutPage},
		{"themes/default/.keep", ""},
		{"assets/.keep", ""},
		{".gitignore", gitignore},
	}
	for _, f := range files {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", f.path, err)
		}
		if err := os.WriteFile(full, []byte(f.body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", f.path, err)
		}
	}
	return nil
}

const configYAML = `# colophon project configuration.
# Any value may reference the environment with {env:VAR} or {env:VAR:-default}.
# Deploy secrets are resolved from the environment, never stored here.

sites:
  - id: main
    title: "My Blog"
    base_url: "{env:SITE_URL:-http://localhost:8080}"
    theme: default
    federation:
      feeds: [rss, atom, json]       # always emit at least one feed
    search: lexical            # on-site visitor search: lexical | semantic | off

# Publishers are pure mechanism (how to deploy). Environments decide what/where.
publishers:
  - id: local                  # writes the built tree to a local directory
    driver: local
    path: ./dist
  # - id: cf
  #   driver: cloudflare-pages
  #   project: "{env:CF_PAGES_PROJECT}"
  #   account_id: "{env:CLOUDFLARE_ACCOUNT_ID}"

# Environments are named build+deploy profiles. No name is special; each chooses its
# publishers, whether drafts are included, and any per-environment overrides.
environments:
  - name: production           # publish with: colophon publish --env production
    publish: [local]
    # allow_publish: false     # gate: require --allow-publish to deploy this env
  # - name: preview
  #   publish: [cf]
  #   include_drafts: true     # build & deploy draft posts in this environment
  #   title: "My Blog (preview)"
  #   overrides:
  #     cf: { branch: preview }  # Cloudflare: non-production branch → Preview env
`

const authorYAML = `# An author is the byline shown to readers. A post's "author:" names one of these (by file
# stem or "id"); with none set, the first author is the default, else "Anonymous".
id: me
name: "Me"
bio: "Writes things."
# avatar: avatar.png
# urls: ["https://example.com"]
`

const personaYAML = `# A persona is a hidden writing VOICE the agent writes in (never shown — the byline is the
# author). Personas are shareable across authors; a persona's corpus is every post in this voice.
id: default
name: "House voice"
style:
  guide: "Conversational, concise, technical when needed."
  # references: ["https://example.com/glossary"]
`

const helloPost = `---
title: Hello World
date: 2026-06-14
author: me        # the byline (authors/me.yaml)
persona: default  # the writing voice (optional; used by the agent)
tags: [meta]
draft: false
---

Welcome to **colophon**. This is your first post.
`

const aboutPage = `---
title: About
author: me
---

About this blog.
`

const gitignore = `# colophon build output and derived state
/public/
/dist/
/.colophon/
`
