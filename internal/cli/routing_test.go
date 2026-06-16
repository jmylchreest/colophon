package cli

import (
	"io/fs"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRouteDecision(t *testing.T) {
	cases := []struct {
		name                        string
		active, owns, isRouteTarget bool
		want                        deliverMode
	}{
		{"route target, routing active & owned", true, true, true, deliverRouted},
		{"route target, routing inactive", false, false, true, deliverSkip},
		{"route target, active but its route didn't resolve", true, false, true, deliverSkip},
		{"default publisher, routing active", true, false, false, deliverRouted},
		{"default publisher, routing inactive", false, false, false, deliverFull},
	}
	for _, c := range cases {
		if got := routeDecision(c.active, c.owns, c.isRouteTarget); got != c.want {
			t.Errorf("%s: routeDecision(%v,%v,%v) = %v, want %v", c.name, c.active, c.owns, c.isRouteTarget, got, c.want)
		}
	}
}

func TestSelectFSFilters(t *testing.T) {
	base := fstest.MapFS{
		"index.html":              {Data: []byte("x")},
		"posts/hi/index.html":     {Data: []byte("x")},
		"posts/hi/assets/cat.png": {Data: []byte("img")},
		"style.css":               {Data: []byte("x")},
	}
	// keep everything except files under an assets/ dir.
	sel := selectFS{base: base, keep: func(p string) bool { return !strings.Contains(p, "/assets/") }}

	var got []string
	if err := fs.WalkDir(sel, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			got = append(got, p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"index.html", "posts/hi/index.html", "style.css"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("walked %v, want %v", got, want)
	}

	// The filtered-out file is also unreadable directly.
	if _, err := fs.ReadFile(sel, "posts/hi/assets/cat.png"); err == nil {
		t.Error("expected filtered file to be hidden from Open")
	}
	if _, err := fs.ReadFile(sel, "index.html"); err != nil {
		t.Errorf("kept file should be readable: %v", err)
	}
}
