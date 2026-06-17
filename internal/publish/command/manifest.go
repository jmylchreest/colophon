package command

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"

	"github.com/jmylchreest/colophon/internal/clog"
)

// fileInfo is one entry of the classification manifest the command can read via $COLOPHON_MANIFEST.
type fileInfo struct {
	Kind        string `json:"kind"`         // page | asset | feed | sitemap | meta
	ContentType string `json:"content_type"` // best-effort MIME type
	Bytes       int64  `json:"bytes"`
}

// writeManifest serialises path → {kind, content_type, bytes} to a stable, sorted JSON document,
// so a deploy command can act on the asset-vs-content distinction (cache headers, etc.).
func writeManifest(filePath string, entries map[string]int64) error {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)

	doc := make(map[string]fileInfo, len(entries))
	for _, name := range names {
		doc[name] = fileInfo{Kind: classify(name), ContentType: contentType(name), Bytes: entries[name]}
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, b, 0o644)
}

// lineLogger forwards a child command's stdout/stderr to the logger one line at a time (under
// --verbose) while retaining a bounded tail for the error message on failure.
type lineLogger struct {
	log *clog.Logger
	id  string
	buf bytes.Buffer // carry for a partial final line
	all bytes.Buffer // full capture, used for the failure tail
}

func (w *lineLogger) Write(b []byte) (int, error) {
	w.all.Write(b)
	w.buf.Write(b)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := string(w.buf.Next(i + 1))
		if w.log != nil {
			w.log.Detail("PUBLISH", w.id, "out", line[:len(line)-1])
		}
	}
	return len(b), nil
}

// tail returns the last ~20 lines of captured output for an error message.
func (w *lineLogger) tail() string {
	lines := bytes.Split(bytes.TrimRight(w.all.Bytes(), "\n"), []byte("\n"))
	const max = 20
	if len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	return string(bytes.Join(lines, []byte("\n")))
}
