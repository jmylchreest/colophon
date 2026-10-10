package webmention

import (
	"net/url"
	"strings"
)

// SafeURL permits only absolute HTTP(S) URLs in sender-supplied links and images.
// HTML escaping alone does not prevent javascript: links from executing on click.
func SafeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, "\\") || strings.ContainsFunc(raw, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return raw
}

// SafeMention validates URLs at render time, including previously cached mentions.
func SafeMention(m Mention) Mention {
	m.URL = SafeURL(m.URL)
	m.Author.URL = SafeURL(m.Author.URL)
	m.Author.Photo = SafeURL(m.Author.Photo)
	return m
}
