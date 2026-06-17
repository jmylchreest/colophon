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
		{".env.defaults", envDefaults},
		{filepath.Join(".github", "workflows", "deploy.yml"), deployWorkflow},
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
    # Reader analytics for THIS site (page views, engagement) — your data, your instance.
    # One block per provider; inert until configured. The statsfactory beacon is cookieless
    # and honours Do-Not-Track; its ingest key is a public "sf_live_" key, safe to embed in
    # pages. Values flow from .env.defaults / .env / the real environment. See docs/analytics.md.
    analytics:
      statsfactory:
        server_url: "{env:STATSFACTORY_SERVER_URL:-}"
        app_key: "{env:STATSFACTORY_APP_KEY:-}"
      # google_analytics:            # GA4 — note: sets cookies, brings its own consent duties
      #   measurement_id: "{env:GA_MEASUREMENT_ID:-}"

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

# Telemetry is colophon's own anonymous usage reporting (builds, source types, publisher
# types — never your content), sent to the colophon maintainer. "enabled" is the MASTER
# switch over ALL telemetry: set it false to disable this AND every site's analytics above.
# You can also disable just this with the COLOPHON_TELEMETRY=off environment variable.
telemetry:
  enabled: true
  # statsfactory:                # override the maintainer's default to point at your own
  #   server_url: "{env:COLOPHON_TELEMETRY_URL:-}"
  #   app_key: "{env:COLOPHON_TELEMETRY_KEY:-}"
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

# Local env overrides / secrets. Shared, non-secret defaults live in .env.defaults
# (committed); this file overrides them and is where deploy secrets go.
/.env
`

// deployWorkflow builds (and optionally publishes) the site in GitHub Actions, sourcing the
// analytics config and any deploy credentials from the repository's Actions secrets/variables.
// The statsfactory ingest key is public, so it is a Variable; deploy credentials are Secrets
// and are only read by colophon, never written into the output tree.
const deployWorkflow = `name: Deploy

on:
  push:
    branches: [main]
  workflow_dispatch:

# Configure under Settings → Secrets and variables → Actions:
#   Variables (public):  STATSFACTORY_SERVER_URL, STATSFACTORY_APP_KEY
#   Secrets (private):   CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID,
#                        R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY
# These override .env.defaults at build time (real env wins). The analytics key is a
# public sf_live_ key embedded in pages; the deploy credentials never leave colophon.

permissions:
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    env:
      # Public analytics config — baked into the built site.
      STATSFACTORY_SERVER_URL: ${{ vars.STATSFACTORY_SERVER_URL }}
      STATSFACTORY_APP_KEY: ${{ vars.STATSFACTORY_APP_KEY }}
      # Deploy credentials — used by colophon to publish, never embedded in output.
      CLOUDFLARE_API_TOKEN: ${{ secrets.CLOUDFLARE_API_TOKEN }}
      CLOUDFLARE_ACCOUNT_ID: ${{ secrets.CLOUDFLARE_ACCOUNT_ID }}
      R2_ACCESS_KEY_ID: ${{ secrets.R2_ACCESS_KEY_ID }}
      R2_SECRET_ACCESS_KEY: ${{ secrets.R2_SECRET_ACCESS_KEY }}
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: stable
      - name: Install colophon
        run: go install github.com/jmylchreest/colophon/cmd/colophon@latest
      - name: Build
        run: colophon build --env production
      # Enable once the production environment targets a cloud publisher (e.g.
      # cloudflare-pages + cloudflare-r2) and the secrets above are set:
      # - name: Publish
      #   run: colophon publish --env production --allow-publish
`

const envDefaults = `# Shared, non-secret defaults loaded by colophon before {env:VAR} interpolation.
# This file IS committed. Precedence: real environment (e.g. CI secrets) > .env (local,
# gitignored) > .env.defaults. Put only PUBLIC values here — never deploy secrets.
#
# The statsfactory ingest key is a public "sf_live_" key, safe to embed in pages. Set these
# to your statsfactory instance to turn analytics on; leave unset and it stays inert.
# STATSFACTORY_SERVER_URL=https://stats.example.com
# STATSFACTORY_APP_KEY=sf_live_xxxxxxxxxxxxxxxx
`
