// Package skills embeds colophon's authoring SKILL.md files so the `colophon skills` command can
// install them into a detected agent harness without the repo present. These are the same files
// the Claude plugin marketplace ships (see .claude-plugin/), kept as the single source of truth.
// The embed directive can only reach files beside it, so this package lives under contrib/.
package skills

import "embed"

// FS holds the skills/ tree: skills/<name>/SKILL.md (plus any bundled resources).
//
//go:embed skills
var FS embed.FS
