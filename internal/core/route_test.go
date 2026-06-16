package core

import "testing"

func assetsRoute() []RouteRule {
	return []RouteRule{{Match: "**/assets/**", Publisher: "r2", BaseURL: "https://cdn.example.com/"}}
}

func TestRouterAssetURL(t *testing.T) {
	r := NewRouter(assetsRoute(), []string{"cf", "r2"})
	if got := r.AssetURL("posts/hi/assets/x.png"); got != "https://cdn.example.com/posts/hi/assets/x.png" {
		t.Errorf("AssetURL = %q", got)
	}
	if got := r.AssetURL("posts/hi/index.html"); got != "" {
		t.Errorf("non-asset should not route, got %q", got)
	}
}

func TestRouterKeepPartition(t *testing.T) {
	r := NewRouter(assetsRoute(), []string{"cf", "r2"})
	cases := []struct {
		pub, path string
		want      bool
	}{
		{"cf", "posts/hi/index.html", true},    // default publisher gets unrouted
		{"cf", "posts/hi/assets/x.png", false}, // ...but not routed files
		{"r2", "posts/hi/assets/x.png", true},  // route target gets its files
		{"r2", "posts/hi/index.html", false},   // ...but not the rest
	}
	for _, c := range cases {
		if got := r.Keep(c.pub, c.path); got != c.want {
			t.Errorf("Keep(%q, %q) = %v, want %v", c.pub, c.path, got, c.want)
		}
	}
}

func TestRouterInactiveWithoutBaseURL(t *testing.T) {
	r := NewRouter([]RouteRule{{Match: "**/assets/**", Publisher: "r2"}}, []string{"r2"}) // no base_url
	if r.Active() {
		t.Error("route without base_url should be inactive")
	}
	if !r.Keep("cf", "posts/assets/x.png") {
		t.Error("inactive routing should keep every file for every publisher")
	}
	if r.AssetURL("posts/assets/x.png") != "" {
		t.Error("inactive routing should not rewrite URLs")
	}
}

func TestRouterInactiveWhenTargetNotDeploying(t *testing.T) {
	// r2 owns the route but only cf is deploying — routing must not strip the assets.
	r := NewRouter(assetsRoute(), []string{"cf"})
	if r.Active() {
		t.Error("route should be inert when its target publisher is not deploying")
	}
	if !r.Keep("cf", "posts/assets/x.png") {
		t.Error("cf should keep the asset (co-located) when r2 is not deploying")
	}
	if r.AssetURL("posts/assets/x.png") != "" {
		t.Error("no rewrite when the route target is not deploying")
	}
}

func TestGlobMatching(t *testing.T) {
	r := NewRouter([]RouteRule{{Match: "img/*.png", Publisher: "r2", BaseURL: "https://c/"}}, []string{"r2"})
	if r.AssetURL("img/a.png") == "" {
		t.Error("img/a.png should match img/*.png")
	}
	if r.AssetURL("img/sub/a.png") != "" {
		t.Error("single * must not cross a slash")
	}
}
