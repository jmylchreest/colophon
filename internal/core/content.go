package core

import "github.com/jmylchreest/colophon/markdown"

// Content is a single article as read from a source, before it is bound to personas
// and sites via Publications. The byline is never taken from here; it comes from the
// persona a Publication resolves to. The document (frontmatter + body) is the portable
// markdown.Document; Content adds source location and provenance.
type Content struct {
	markdown.Document

	// SourcePath is the path within the source (used for slugs, backlinks, assets).
	SourcePath string
	// AuthoredVia identifies the operator (human or agent) that wrote it — provenance
	// for the audit trail, not the public byline.
	AuthoredVia string
}
