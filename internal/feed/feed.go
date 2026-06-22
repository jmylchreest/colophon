// Package feed renders syndication formats (RSS 2.0, Atom 1.0, JSON Feed 1.1) and a
// sitemap from a site's built pages, using only encoding/xml + encoding/json — no
// external dependency. It works purely off absolute URLs + rendered HTML, so it has no
// dependency on the asset pipeline or any source.
//
// Implemented against these specifications (consult them before changing a format):
//
//	RSS 2.0      https://www.rssboard.org/rss-specification   (dates: RFC 822 / RFC 1123)
//	Atom 1.0     https://www.rfc-editor.org/rfc/rfc4287       (dates: RFC 3339)
//	JSON Feed 1.1 https://www.jsonfeed.org/version/1.1/
//	Sitemap 0.9  https://www.sitemaps.org/protocol.html       (dates: W3C Datetime)
package feed

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// Site is the feed-level metadata.
type Site struct {
	Title   string
	BaseURL string // absolute site root, e.g. https://blog.example.com
	Author  string // optional byline
	// Self is this feed's own absolute URL (rel="self"); Hubs are WebSub hub URLs
	// (rel="hub"). Both are emitted for real-time push discovery when set.
	Self string
	Hubs []string
}

// Item is one syndicated entry. URL is absolute; Content is rendered HTML.
type Item struct {
	Title       string
	URL         string
	Description string
	Content     string
	Published   time.Time
	// Enclosure is the item's primary attached media file (e.g. a podcast audio reading). RSS
	// permits only one <enclosure> per item, so this is the only one RSS renders; it is also
	// rendered as an Atom <link rel="enclosure"> and a JSON Feed attachment.
	Enclosure *Enclosure
	// Attachments are additional downloadable files beyond the primary Enclosure. Atom and
	// JSON Feed allow multiple, so each is rendered there (as another enclosure link / feed
	// attachment); RSS cannot carry them and omits them.
	Attachments []Enclosure
}

// Enclosure is an attached media file. URL is absolute; Length is the byte size (0 omits it).
type Enclosure struct {
	URL    string
	Type   string
	Length int64
}

func updated(items []Item) time.Time {
	var t time.Time
	for _, it := range items {
		if it.Published.After(t) {
			t = it.Published
		}
	}
	return t
}

// --- RSS 2.0 ---

type rssRoot struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	AtomLinks     string    `xml:",innerxml"` // pre-rendered Atom <link rel=self/hub> for WebSub
	Items         []rssItem `xml:"item"`
}

// rssAtomLinks renders the WebSub self/hub discovery links for an RSS channel as Atom
// <link> elements. RSS has no native self/hub link, so the convention is to embed Atom's;
// the inline xmlns puts each in the Atom namespace without needing a prefix declaration.
func rssAtomLinks(s Site) string {
	const ns = `xmlns="http://www.w3.org/2005/Atom"`
	var b strings.Builder
	if s.Self != "" {
		fmt.Fprintf(&b, `<link %s rel="self" type="application/rss+xml" href="%s"/>`, ns, xmlAttr(s.Self))
	}
	for _, h := range s.Hubs {
		fmt.Fprintf(&b, `<link %s rel="hub" href="%s"/>`, ns, xmlAttr(h))
	}
	return b.String()
}

// xmlAttr escapes a string for use in a double-quoted XML attribute (feed URLs can carry
// & in query strings). Used only for the innerxml-rendered RSS self/hub links above.
func xmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

type rssItem struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	GUID        rssGUID       `xml:"guid"`
	PubDate     string        `xml:"pubDate,omitempty"`
	Description string        `xml:"description"`
	Enclosure   *rssEnclosure `xml:"enclosure,omitempty"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length int64  `xml:"length,attr"`
}

type rssGUID struct {
	IsPermaLink bool   `xml:"isPermaLink,attr"`
	Value       string `xml:",chardata"`
}

// RSS renders an RSS 2.0 document.
func RSS(s Site, items []Item) ([]byte, error) {
	ch := rssChannel{Title: s.Title, Link: s.BaseURL, Description: s.Title, AtomLinks: rssAtomLinks(s)}
	if t := updated(items); !t.IsZero() {
		ch.LastBuildDate = t.UTC().Format(time.RFC1123Z)
	}
	for _, it := range items {
		ri := rssItem{
			Title:       it.Title,
			Link:        it.URL,
			GUID:        rssGUID{IsPermaLink: true, Value: it.URL},
			Description: it.Content,
		}
		if !it.Published.IsZero() {
			ri.PubDate = it.Published.UTC().Format(time.RFC1123Z)
		}
		if it.Enclosure != nil {
			ri.Enclosure = &rssEnclosure{URL: it.Enclosure.URL, Type: it.Enclosure.Type, Length: it.Enclosure.Length}
		}
		ch.Items = append(ch.Items, ri)
	}
	return marshal(rssRoot{Version: "2.0", Channel: ch})
}

// --- Atom 1.0 ---

type atomRoot struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"` // rel="alternate" (site) + rel="self"/rel="hub" (WebSub)
	Author  *atomAuthor `xml:"author,omitempty"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href   string `xml:"href,attr"`
	Rel    string `xml:"rel,attr,omitempty"`
	Type   string `xml:"type,attr,omitempty"`
	Length int64  `xml:"length,attr,omitempty"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Links     []atomLink `xml:"link"` // the alternate link, plus a rel="enclosure" when present
	Updated   string     `xml:"updated"`
	Published string     `xml:"published,omitempty"`
	Summary   *atomText  `xml:"summary,omitempty"`
	Content   *atomText  `xml:"content,omitempty"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

// Atom renders an Atom 1.0 document.
func Atom(s Site, items []Item) ([]byte, error) {
	root := atomRoot{
		Title:   s.Title,
		ID:      s.BaseURL,
		Updated: stamp(updated(items)),
		Links:   []atomLink{{Href: s.BaseURL, Rel: "alternate"}},
	}
	if s.Self != "" {
		root.Links = append(root.Links, atomLink{Href: s.Self, Rel: "self", Type: "application/atom+xml"})
	}
	for _, h := range s.Hubs {
		root.Links = append(root.Links, atomLink{Href: h, Rel: "hub"})
	}
	if s.Author != "" {
		root.Author = &atomAuthor{Name: s.Author}
	}
	for _, it := range items {
		e := atomEntry{
			Title:   it.Title,
			ID:      it.URL,
			Links:   []atomLink{{Href: it.URL, Rel: "alternate"}},
			Updated: stamp(it.Published),
			Content: &atomText{Type: "html", Body: it.Content},
		}
		if !it.Published.IsZero() {
			e.Published = it.Published.UTC().Format(time.RFC3339)
		}
		if it.Description != "" {
			e.Summary = &atomText{Type: "text", Body: it.Description}
		}
		if it.Enclosure != nil {
			e.Links = append(e.Links, atomLink{Href: it.Enclosure.URL, Rel: "enclosure", Type: it.Enclosure.Type, Length: it.Enclosure.Length})
		}
		for _, a := range it.Attachments {
			e.Links = append(e.Links, atomLink{Href: a.URL, Rel: "enclosure", Type: a.Type, Length: a.Length})
		}
		root.Entries = append(root.Entries, e)
	}
	return marshal(root)
}

// --- JSON Feed 1.1 ---

type jsonRoot struct {
	Version     string       `json:"version"`
	Title       string       `json:"title"`
	HomePageURL string       `json:"home_page_url,omitempty"`
	FeedURL     string       `json:"feed_url,omitempty"`
	Authors     []jsonAuthor `json:"authors,omitempty"`
	Hubs        []jsonHub    `json:"hubs,omitempty"`
	Items       []jsonItem   `json:"items"`
}

type jsonHub struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type jsonAuthor struct {
	Name string `json:"name"`
}

type jsonItem struct {
	ID            string           `json:"id"`
	URL           string           `json:"url"`
	Title         string           `json:"title"`
	Summary       string           `json:"summary,omitempty"`
	ContentHTML   string           `json:"content_html"`
	DatePublished string           `json:"date_published,omitempty"`
	Attachments   []jsonAttachment `json:"attachments,omitempty"`
}

type jsonAttachment struct {
	URL         string `json:"url"`
	MIMEType    string `json:"mime_type"`
	SizeInBytes int64  `json:"size_in_bytes,omitempty"`
}

// JSON renders a JSON Feed 1.1 document.
func JSON(s Site, items []Item) ([]byte, error) {
	root := jsonRoot{Version: "https://jsonfeed.org/version/1.1", Title: s.Title, HomePageURL: s.BaseURL, FeedURL: s.Self}
	if s.Author != "" {
		root.Authors = []jsonAuthor{{Name: s.Author}}
	}
	for _, h := range s.Hubs {
		root.Hubs = append(root.Hubs, jsonHub{Type: "WebSub", URL: h})
	}
	for _, it := range items {
		ji := jsonItem{ID: it.URL, URL: it.URL, Title: it.Title, Summary: it.Description, ContentHTML: it.Content}
		if !it.Published.IsZero() {
			ji.DatePublished = it.Published.UTC().Format(time.RFC3339)
		}
		if it.Enclosure != nil {
			ji.Attachments = append(ji.Attachments, jsonAttachment{URL: it.Enclosure.URL, MIMEType: it.Enclosure.Type, SizeInBytes: it.Enclosure.Length})
		}
		for _, a := range it.Attachments {
			ji.Attachments = append(ji.Attachments, jsonAttachment{URL: a.URL, MIMEType: a.Type, SizeInBytes: a.Length})
		}
		root.Items = append(root.Items, ji)
	}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode json feed: %w", err)
	}
	return append(b, '\n'), nil
}

// --- sitemap ---

type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// SitemapEntry is one sitemap URL with optional last-modified time.
type SitemapEntry struct {
	URL     string
	LastMod time.Time
}

// Sitemap renders a sitemap.xml document.
func Sitemap(entries []SitemapEntry) ([]byte, error) {
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, e := range entries {
		u := sitemapURL{Loc: e.URL}
		if !e.LastMod.IsZero() {
			u.LastMod = e.LastMod.UTC().Format("2006-01-02")
		}
		set.URLs = append(set.URLs, u)
	}
	return marshal(set)
}

// stamp formats t as RFC3339, defaulting to the zero time's epoch when unset so Atom's
// required <updated> always has a value.
func stamp(t time.Time) string {
	if t.IsZero() {
		t = time.Unix(0, 0).UTC()
	}
	return t.UTC().Format(time.RFC3339)
}

func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode xml: %w", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}
