package core

import "testing"

func TestGravatarURL(t *testing.T) {
	// Documented Gravatar example: md5("myemailaddress@example.com").
	const hash = "0bc83cb571cd1c50ba6f3e8a78ef1346"

	cases := []struct {
		name   string
		value  string
		email  string
		want   string
		wantOK bool
	}{
		{"inline email", "gravatar:myemailaddress@example.com", "", "https://www.gravatar.com/avatar/" + hash + "?s=200&d=mp", true},
		{"bare uses author email", "gravatar", "myemailaddress@example.com", "https://www.gravatar.com/avatar/" + hash + "?s=200&d=mp", true},
		{"case and space normalised", "gravatar:  MyEmailAddress@Example.com ", "", "https://www.gravatar.com/avatar/" + hash + "?s=200&d=mp", true},
		{"query passthrough", "gravatar:myemailaddress@example.com?d=identicon&s=256", "", "https://www.gravatar.com/avatar/" + hash + "?d=identicon&s=256", true},
		{"not a gravatar ref", "assets/me.png", "x@y.com", "", false},
		{"http passthrough", "https://example.com/a.png", "x@y.com", "", false},
		{"bare with no email", "gravatar", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := GravatarURL(c.value, c.email)
			if ok != c.wantOK || got != c.want {
				t.Errorf("GravatarURL(%q, %q) = (%q, %v), want (%q, %v)", c.value, c.email, got, ok, c.want, c.wantOK)
			}
		})
	}
}
