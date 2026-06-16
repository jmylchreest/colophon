package core

import "strings"

// Author is the byline shown to readers — who wrote a post. A post names one via `author:`
// (defaulting to the first configured author, else "Anonymous"). Authors live in
// authors/*.yaml; the fields are h-card properties used for the byline, the author page,
// feeds and JSON-LD. An author is an identity (a person, or a brand name) and is distinct
// from a persona, which is the hidden writing voice and is never shown.
type Author struct {
	ID     string   `yaml:"id" json:"id"`
	Name   string   `yaml:"name" json:"name"`
	Bio    string   `yaml:"bio,omitempty" json:"bio,omitempty"`
	Avatar string   `yaml:"avatar,omitempty" json:"avatar,omitempty"`
	Email  string   `yaml:"email,omitempty" json:"email,omitempty"`
	URLs   []string `yaml:"urls,omitempty" json:"urls,omitempty"`
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
