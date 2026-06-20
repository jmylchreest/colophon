package core

import (
	"crypto/md5" //nolint:gosec // Gravatar's documented hash is MD5 of the email; not a security use.
	"encoding/hex"
	"strings"
)

// GravatarURL resolves a `gravatar` avatar reference to a Gravatar image URL, or returns
// ok=false for any other value (a path, a data:/http(s) URL, or empty — left as-is). Two forms
// are accepted: `gravatar:<email>` carries the address inline, and a bare `gravatar` uses the
// author's own `email:`. Optional Gravatar query options pass through after a `?`
// (e.g. `gravatar:me@x.com?d=identicon&s=256`); when none are given a crisp retina size and a
// neutral "mystery person" fallback are used, so a missing Gravatar degrades to a silhouette
// rather than the Gravatar logo. The email is hashed per the spec: trimmed, lower-cased, MD5 hex.
func GravatarURL(value, fallbackEmail string) (string, bool) {
	var rest string
	switch {
	case value == "gravatar":
		rest = ""
	case strings.HasPrefix(value, "gravatar:"):
		rest = value[len("gravatar:"):]
	default:
		return "", false
	}
	email, query := rest, ""
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		email, query = rest[:i], rest[i+1:]
	}
	email = strings.TrimSpace(email)
	if email == "" {
		email = strings.TrimSpace(fallbackEmail)
	}
	if email == "" {
		return "", false // nothing to hash — leave the reference untouched
	}
	sum := md5.Sum([]byte(strings.ToLower(email))) //nolint:gosec // see import note
	if query == "" {
		query = "s=200&d=mp"
	}
	return "https://www.gravatar.com/avatar/" + hex.EncodeToString(sum[:]) + "?" + query, true
}

// Author is the byline shown to readers — who wrote a post. A post names one via `author:`
// (defaulting to the first configured author, else "Anonymous"). Authors live in
// authors/*.yaml; the fields are h-card properties used for the byline, the author page,
// feeds and JSON-LD. An author is an identity (a person, or a brand name) and is distinct
// from a persona, which is the hidden writing voice and is never shown.
type Author struct {
	ID     string `yaml:"id" json:"id"`
	Name   string `yaml:"name" json:"name"`
	Bio    string `yaml:"bio,omitempty" json:"bio,omitempty"`
	Avatar string `yaml:"avatar,omitempty" json:"avatar,omitempty"`
	// AvatarFit/AvatarPosition choose how the avatar fills its (usually circular) frame —
	// CSS object-fit / object-position. Empty → the theme default (cover, centred).
	AvatarFit      string   `yaml:"avatar_fit,omitempty" json:"avatar_fit,omitempty"`
	AvatarPosition string   `yaml:"avatar_position,omitempty" json:"avatar_position,omitempty"`
	Email          string   `yaml:"email,omitempty" json:"email,omitempty"`
	URLs           []string `yaml:"urls,omitempty" json:"urls,omitempty"`
	// Voice is the text-to-speech voice id used when a post by this author opts into audio
	// (a provider system voice, or a cloned voice id). A post's audio_voice overrides it.
	Voice string `yaml:"voice,omitempty" json:"voice,omitempty"`
}

// AnonymousAuthor is the fallback byline when a post names no author and none are configured.
func AnonymousAuthor() Author { return Author{ID: "anonymous", Name: "Anonymous"} }

// Validate checks an author has the fields a byline needs.
func (a Author) Validate() error {
	if a.ID == "" {
		return &ValidationError{Field: "id", Msg: "author id is required"}
	}
	if strings.TrimSpace(a.Name) == "" {
		return &ValidationError{Field: "name", Msg: "author name is required"}
	}
	return nil
}
