package build

import (
	"context"
	"fmt"
	"html"
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
	URL       string // page link (root-relative when co-located, absolute when routed/external)
	Abs       string // absolute URL, for feed enclosures
	Label     string // link text
	Desc      string // optional one-line description
	Name      string // file base name (e.g. dataset.zip)
	Type      string // MIME type
	TypeLabel string // short human filetype badge (e.g. ZIP, PDF, MP4)
	Bytes     int64  // file size, 0 if unknown (external link)
	Size      string // human-readable size, "" if unknown
	Feed      bool   // also list as a feed enclosure/attachment
	View      bool   // open in the browser (no download attr) — e.g. the slide deck page
}

// slidesAttachment is the synthetic Downloads-box entry for a post's derived slide deck. It opens
// in the browser (View) rather than downloading, and is never a feed enclosure.
func slidesAttachment(url string, bytes int) pageAttachment {
	return pageAttachment{
		URL: url, Abs: url, Label: "Slides", Desc: "Presentation — opens in your browser",
		Name: "slides", Type: "text/html", TypeLabel: "DECK",
		Bytes: int64(bytes), Size: humanSize(int64(bytes)), View: true,
	}
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
				URL: ref, Abs: ref, Label: attachmentLabel(a, ref), Desc: a.Description,
				Name: path.Base(ref), Type: mimeForName(ref), TypeLabel: typeLabel(ref),
				Feed: a.Feed,
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
			URL: url, Abs: abs, Label: attachmentLabel(a, ref), Desc: a.Description,
			Name: path.Base(ref), Type: mimeForName(ref), TypeLabel: typeLabel(ref),
			Bytes: sz, Size: humanSize(sz), Feed: a.Feed,
		})
	}
	return out
}

// attachmentVars projects resolved attachments into the template context as a structured list,
// so a theme can build its own download UI (the `attachments` variable). Themes that just want
// the batteries-included markup use `attachments_html` instead.
func attachmentVars(as []pageAttachment) []map[string]any {
	out := make([]map[string]any, len(as))
	for i, a := range as {
		out[i] = map[string]any{
			"url": a.URL, "label": a.Label, "description": a.Desc, "name": a.Name,
			"type": a.Type, "type_label": a.TypeLabel, "size": a.Size, "bytes": a.Bytes,
			"view": a.View,
		}
	}
	return out
}

// attachmentsHTML renders the engine's ready-made Downloads block — a no-JS, semantic fragment
// with stable classes a theme styles via CSS (or ignores in favour of the `attachments` list).
// Returns "" when there are none, so a theme can `{{ attachments_html|safe }}` unconditionally.
func attachmentsHTML(as []pageAttachment) string {
	if len(as) == 0 {
		return ""
	}
	const clip = `<svg class="dl-ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>`
	var b strings.Builder
	b.WriteString(`<aside class="post-downloads" aria-label="Downloads">`)
	b.WriteString(`<div class="downloads-title">Downloads</div>`)
	b.WriteString(`<ul class="downloads-list">`)
	for _, a := range as {
		b.WriteString(`<li class="dl-item"><a class="dl" href="`)
		b.WriteString(html.EscapeString(a.URL))
		if a.View {
			// A viewable artifact (the deck) opens in a new tab — a full-viewport presentation,
			// separate from the post, that Esc/close returns from cleanly.
			b.WriteString(`" target="_blank" rel="noopener">`)
		} else {
			b.WriteString(`" download>`)
		}
		b.WriteString(clip)
		b.WriteString(`<span class="dl-main"><span class="dl-label">`)
		b.WriteString(html.EscapeString(a.Label))
		b.WriteString(`</span>`)
		if a.Desc != "" {
			b.WriteString(`<span class="dl-desc">`)
			b.WriteString(html.EscapeString(a.Desc))
			b.WriteString(`</span>`)
		}
		b.WriteString(`</span><span class="dl-meta">`)
		if a.TypeLabel != "" {
			b.WriteString(`<span class="dl-type">`)
			b.WriteString(html.EscapeString(a.TypeLabel))
			b.WriteString(`</span>`)
		}
		if a.Size != "" {
			b.WriteString(`<span class="dl-size">`)
			b.WriteString(html.EscapeString(a.Size))
			b.WriteString(`</span>`)
		}
		b.WriteString(`</span></a></li>`)
	}
	b.WriteString(`</ul></aside>`)
	return b.String()
}

// typeLabel is a short uppercase filetype badge derived from the file extension (e.g. "ZIP",
// "PDF", "MP4"), or "" when there is no extension.
func typeLabel(name string) string {
	return strings.ToUpper(strings.TrimPrefix(path.Ext(name), "."))
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
