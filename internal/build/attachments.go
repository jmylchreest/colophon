package build

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

// pageAttachment is one resolved downloadable file for a post: the link to use on the page,
// its absolute URL for feeds, display metadata, and whether it should appear as a feed
// enclosure/attachment.
type pageAttachment struct {
	URL   string // page link (root-relative when co-located, absolute when routed/external)
	Abs   string // absolute URL, for feed enclosures
	Label string // link text
	Name  string // file base name (e.g. dataset.zip)
	Type  string // MIME type
	Bytes int64  // file size, 0 if unknown (external link)
	Size  string // human-readable size, "" if unknown
	Feed  bool   // also list as a feed enclosure/attachment
}

// resolveAttachments turns a post's frontmatter attachments into copied+routed download links.
// Local refs mirror image resolution exactly — co-located beside the page, rewritten to the
// object store when a route binds them — so they ship through the existing asset copy (their
// paths are also returned by docRefs). An absolute/external ref is linked as-is, not copied.
// File sizes are read here (before feeds are written) so the enclosure length is available.
func resolveAttachments(ctx context.Context, it included, basePath, baseURL string, router *core.Router) []pageAttachment {
	var out []pageAttachment
	for _, a := range it.c.Frontmatter.Attachments {
		ref := a.Path
		if ref == "" {
			continue
		}
		if !localRef(ref) {
			out = append(out, pageAttachment{
				URL: ref, Abs: ref, Label: attachmentLabel(a, ref),
				Name: path.Base(ref), Type: mimeForName(ref), Feed: a.Feed,
			})
			continue
		}
		outPath := path.Clean(path.Join(it.slug, ref))
		url := router.AssetURL(outPath)
		abs := url
		if url == "" {
			url = basePath + outPath
			abs = absURL(baseURL, outPath)
		}
		sz := attachmentSize(ctx, it.src, path.Clean(path.Join(path.Dir(it.c.SourcePath), ref)))
		out = append(out, pageAttachment{
			URL: url, Abs: abs, Label: attachmentLabel(a, ref), Name: path.Base(ref),
			Type: mimeForName(ref), Bytes: sz, Size: humanSize(sz), Feed: a.Feed,
		})
	}
	return out
}

// attachmentVars projects resolved attachments into the template context for the themes'
// Downloads block.
func attachmentVars(as []pageAttachment) []map[string]any {
	out := make([]map[string]any, len(as))
	for i, a := range as {
		out[i] = map[string]any{
			"url": a.URL, "label": a.Label, "name": a.Name,
			"type": a.Type, "size": a.Size, "bytes": a.Bytes,
		}
	}
	return out
}

func attachmentLabel(a markdown.Attachment, ref string) string {
	if strings.TrimSpace(a.Label) != "" {
		return a.Label
	}
	return path.Base(ref)
}

// mimeForName maps a file name to a MIME type by extension, defaulting to a generic binary
// type so an enclosure always has a usable type.
func mimeForName(name string) string {
	if t := mime.TypeByExtension(path.Ext(name)); t != "" {
		if i := strings.IndexByte(t, ';'); i >= 0 {
			t = t[:i]
		}
		return strings.TrimSpace(t)
	}
	return "application/octet-stream"
}

// attachmentSize returns the byte length of an asset, or 0 if it cannot be read (a missing
// file is reported separately by the asset copy as a broken link).
func attachmentSize(ctx context.Context, src core.Source, srcPath string) int64 {
	rc, err := src.Open(ctx, srcPath)
	if err != nil {
		return 0
	}
	defer func() { _ = rc.Close() }()
	n, err := io.Copy(io.Discard, rc)
	if err != nil {
		return 0
	}
	return n
}

// humanSize formats a byte count as a short human-readable string (e.g. "3.4 MB"). Empty for 0.
func humanSize(n int64) string {
	if n <= 0 {
		return ""
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
