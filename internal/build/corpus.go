package build

import (
	"time"

	"github.com/jmylchreest/colophon/internal/config"
)

// CorpusDoc is one persona-attributed content document, exposed for retrieval/exemplar
// selection (e.g. `colophon persona context`). Body is the raw markdown.
type CorpusDoc struct {
	PersonaID  string
	Title      string
	Date       time.Time
	SourcePath string
	Tags       []string
	Body       string
}

// Corpus gathers every content document from the configured sources and attributes each to
// a persona id (frontmatter `persona`, else the first `publications` entry, else ""). It
// includes drafts and embargoed posts: this is authoring context, not a publication. It
// reuses the build's source loading so it sees exactly what a build would.
func Corpus(cfg *config.Config) ([]CorpusDoc, error) {
	docs, err := gatherDocuments(cfg, nil)
	if err != nil {
		return nil, err
	}
	out := make([]CorpusDoc, 0, len(docs))
	for _, d := range docs {
		fm := d.doc.Frontmatter
		pid := fm.Persona
		if pid == "" && len(fm.Publications) > 0 {
			pid = fm.Publications[0].Persona
		}
		out = append(out, CorpusDoc{
			PersonaID:  pid,
			Title:      fm.Title,
			Date:       fm.Date,
			SourcePath: d.doc.SourcePath,
			Tags:       fm.Tags,
			Body:       d.doc.Body,
		})
	}
	return out, nil
}
