package core

import "testing"

func TestLangMatches(t *testing.T) {
	cases := []struct {
		want, got string
		ok        bool
	}{
		{"en", "en", true},
		{"EN", "en", true},
		{"es", "es-MX", true},
		{"es-MX", "es", true},
		{"es-MX", "es-MX", true},
		{"es-MX", "es-ES", false},
		{"es", "en", false},
		{"", "en", false},
		{"en", "", false},
	}
	for _, c := range cases {
		if got := LangMatches(c.want, c.got); got != c.ok {
			t.Errorf("LangMatches(%q, %q) = %v, want %v", c.want, c.got, got, c.ok)
		}
	}
}

func TestSyndicatorAcceptsLang(t *testing.T) {
	cases := []struct {
		name string
		conf []string
		lang string
		ok   bool
	}{
		{"unset takes the default language", nil, "en", true},
		{"unset rejects a translation", nil, "es", false},
		{"unset defaults a blank post language", nil, "", true},
		{"explicit language", []string{"es"}, "es", true},
		{"explicit language rejects others", []string{"es"}, "fr", false},
		{"list", []string{"es", "fr"}, "fr", true},
		{"wildcard takes everything", []string{"*"}, "ja", true},
		{"region matches its base", []string{"es"}, "es-MX", true},
	}
	for _, c := range cases {
		conf := SyndicatorConf{ID: "x", Lang: c.conf}
		if got := conf.AcceptsLang(c.lang, "en"); got != c.ok {
			t.Errorf("%s: AcceptsLang(%q) = %v, want %v", c.name, c.lang, got, c.ok)
		}
	}
}
