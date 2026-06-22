package build

import (
	"sort"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/render"
)

// navLinks lists the dateless (static) pages as {title, url} entries for the nav menu,
// sorted by title so the menu order is stable across builds. url already includes basePath.
func navLinks(pages []page, basePath string) []map[string]any {
	out := make([]map[string]any, 0)
	for _, p := range pages {
		if !p.Static {
			continue
		}
		out = append(out, map[string]any{"title": p.Title, "url": basePath + p.URL})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["title"].(string) < out[j]["title"].(string) })
	return out
}

// authorGroup is one author that wrote at least one published page, together with the
// index-item maps for its posts. Groups are returned most-recent-first (the order an author
// first appears while scanning the newest-first pages slice).
type authorGroup struct {
	id          string
	name        string
	avatar      string
	avatarStyle string
	initials    string
	url         string
	relMe       string // <link rel="me"> tags for this author's urls, for the author feed page <head>
	items       []map[string]any
}

// collectAuthors groups pages by their resolved byline author, preserving the newest-first
// order of pages so the result is ordered by each author's most recent post. list[i] is the
// index-item map for pages[i]. Authors are resolved through resolveAuthor, so a page with no
// explicit author falls under the default (first configured) author, matching the byline.
func collectAuthors(cfg *config.Config, pages []page, list []map[string]any, basePath string) []authorGroup {
	byID := map[string]*authorGroup{}
	var order []*authorGroup
	for i, p := range pages {
		author := resolveAuthor(cfg, p.Author)
		id := normalizeSlug(author.ID)
		if id == "" {
			continue
		}
		g := byID[id]
		if g == nil {
			g = &authorGroup{
				id:          id,
				name:        author.Name,
				avatar:      author.Avatar,
				avatarStyle: imageStyle(author.AvatarFit, author.AvatarPosition),
				initials:    initials(author.Name),
				url:         basePath + "authors/" + id + "/",
				relMe:       relMeLinks(author.URLs),
			}
			byID[id] = g
			order = append(order, g)
		}
		g.items = append(g.items, list[i])
	}
	out := make([]authorGroup, len(order))
	for i, g := range order {
		out[i] = *g
	}
	return out
}

// authorStrip renders the author groups as the template `authors` variable: one entry per
// known persona, most-recent-first, for the avatar widget in the topbar.
func authorStrip(groups []authorGroup) []map[string]any {
	out := make([]map[string]any, len(groups))
	for i, g := range groups {
		out[i] = map[string]any{
			"name":         g.name,
			"url":          g.url,
			"avatar":       g.avatar,
			"avatar_style": g.avatarStyle,
			"initials":     g.initials,
			"count":        len(g.items),
		}
	}
	return out
}

// writeAuthorPages renders a listing page per persona at authors/<id>/, reusing the index
// template (with a heading and that author's posts). Avatar links in the topbar point here,
// so personas become cross-entry navigation, mirroring writeTagPages.
func writeAuthorPages(write func(string, []byte) error, eng render.Engine, chrome listingChrome, groups []authorGroup) error {
	for _, g := range groups {
		heading := "By " + g.name
		html, err := chrome.render(eng, heading, g.items, map[string]any{
			"seo_head":  chrome.seoHead("authors/"+g.id+"/", heading, false),
			"feed_head": chrome.feedHead + g.relMe, // + this author's rel="me" identity links
		})
		if err != nil {
			return err
		}
		if err := write("authors/"+g.id+"/index.html", html); err != nil {
			return err
		}
	}
	return nil
}
