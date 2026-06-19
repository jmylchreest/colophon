package core

import (
	"context"
	"io"
)

// Source yields the content documents to build, abstracting over origin: a local
// markdown directory, an Obsidian vault, a WebDAV share, another blog's API, and so on.
// Each driver lives in its own package under internal/source and self-registers; core
// knows only this interface. The build merges the documents from every configured
// source, so deletions/renames in a source flow through the normal build reconciliation.
type Source interface {
	// ID is the configured source id (e.g. "notes").
	ID() string
	// Driver is the implementation key (e.g. "obsidian").
	Driver() string
	// Documents returns every publishable document the source provides. Each is a
	// Content whose SourcePath is source-relative (slash-separated) and drives its
	// slug/URL, so the source's folder structure maps onto the site structure.
	Documents(ctx context.Context) ([]Content, error)
	// Open returns the bytes of an asset (image, etc.) referenced by a document, by its
	// source-relative path. A filesystem source reads a file; an API source (Notion,
	// WebDAV) fetches it. The caller closes the reader; a missing asset is an error.
	Open(ctx context.Context, ref string) (io.ReadCloser, error)
	// Resolve reports whether a source-relative ref can be sourced and, if so, its qualified
	// (canonical) location — the same place Open would read from — without reading it. It is
	// the cheap "does this reference resolve?" check (used by doctor and by avatar/asset
	// publishing): a filesystem source stats; an API source would do a metadata lookup. The
	// resolution semantics mirror the driver's own embed handling (e.g. obsidian searches its
	// scan roots + vault; md-dir is plain dir-relative).
	Resolve(ctx context.Context, ref string) (qualified string, ok bool)
}

// Warner is an optional Source capability: after Documents, the build collects and logs any
// non-fatal warnings the source recorded (e.g. vault notes that matched the publish tag but
// failed the structure checks and were skipped). Sources that never warn need not implement it.
type Warner interface {
	Warnings() []string
}
