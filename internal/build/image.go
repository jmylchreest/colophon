package build

import (
	"regexp"
	"strings"
)

// objectFits is the allowed set of CSS object-fit values, exposed to authors as the `*_fit`
// frontmatter so they can choose how an image fills its box: cover (crop to fill, the theme
// default), contain (fit whole, letterbox), fill (stretch), scale-down, or none.
var objectFits = map[string]bool{
	"cover": true, "contain": true, "fill": true, "scale-down": true, "none": true,
}

// objectPositionRE bounds the `*_position` value to safe CSS object-position syntax: keywords
// (top/left/…), percentages and lengths ("50% 20%"), nothing that could break out of the
// style attribute. Empty/invalid → no position emitted (theme default applies).
var objectPositionRE = regexp.MustCompile(`^[a-z0-9 %.\-]{1,40}$`)

// imageStyle builds a sanitized inline style for an <img> from an author-supplied fit and
// position. It returns "" when neither is set or valid, so the theme's CSS default (typically
// object-fit: cover) stays in force — authors opt in per image without every image needing a
// style. The values are validated against allow-lists, so the result is safe to place in a
// (pongo2-escaped) style="" attribute.
func imageStyle(fit, position string) string {
	var parts []string
	if f := strings.ToLower(strings.TrimSpace(fit)); objectFits[f] {
		parts = append(parts, "object-fit:"+f)
	}
	if p := strings.ToLower(strings.TrimSpace(position)); p != "" && objectPositionRE.MatchString(p) {
		parts = append(parts, "object-position:"+p)
	}
	return strings.Join(parts, ";")
}
