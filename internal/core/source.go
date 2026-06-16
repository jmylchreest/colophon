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
}
