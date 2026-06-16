package core

import "time"

// Publication is a resolved binding of one Content to one Persona on one Site. It
// is what the render stage actually emits a page for. The byline, style, theme and
// publishers all derive from Persona and Site, not from Content.
type Publication struct {
	Content *Content
	Persona *Persona
	Site    *Site

	Slug         string
	Date         time.Time
	Tags         []string
	Draft        bool
	PublishAfter *time.Time
}

// VisibleAt reports whether the publication should appear in a production build run
// at instant now: not a draft, and past any embargo.
func (p Publication) VisibleAt(now time.Time) bool {
	if p.Draft {
		return false
	}
	if p.PublishAfter != nil && now.Before(*p.PublishAfter) {
		return false
	}
	return true
}
