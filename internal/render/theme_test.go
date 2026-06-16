package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractTheme(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "themes", "minimal")
	written, err := ExtractTheme("minimal", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("ExtractTheme wrote nothing")
	}
	if _, err := os.Stat(filepath.Join(dest, "page.html")); err != nil {
		t.Errorf("page.html should be on disk: %v", err)
	}
	if _, err := ExtractTheme("nope", filepath.Join(dir, "x")); err == nil {
		t.Error("unknown theme should error")
	}
}

func TestThemeSelection(t *testing.T) {
	// The minimal theme ships no vendored JS; the default theme does.
	minimal, err := newThemeSource("", "minimal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := minimal.read("page.html"); err != nil {
		t.Fatalf("minimal page.html: %v", err)
	}
	assets, err := minimal.staticAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range assets {
		if strings.HasPrefix(a, "vendor/") {
			t.Errorf("minimal theme should ship no vendored assets, got %q", a)
		}
		if strings.HasSuffix(a, ".html") {
			t.Errorf("staticAssets must exclude templates, got %q", a)
		}
	}
}

func TestThemeDefaultHasVendor(t *testing.T) {
	def, err := newThemeSource("", "default")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := def.staticAssets()
	if err != nil {
		t.Fatal(err)
	}
	var hasVendor, hasCSS bool
	for _, a := range assets {
		hasVendor = hasVendor || strings.HasPrefix(a, "vendor/")
		hasCSS = hasCSS || a == "style.css"
	}
	if !hasVendor {
		t.Error("default theme should ship vendored assets")
	}
	if !hasCSS {
		t.Error("default theme should ship style.css")
	}
}

func TestBaseThemeInheritsAssets(t *testing.T) {
	// press is an overlay built-in: it declares `base: default`, so it must inherit the
	// default theme's vendored assets (fonts/JS) while serving its own templates + CSS.
	press, err := newThemeSource("", "press")
	if err != nil {
		t.Fatal(err)
	}
	// Its own page template renders, not a fallback.
	body, err := press.read("page.html")
	if err != nil {
		t.Fatalf("press page.html: %v", err)
	}
	if !strings.Contains(string(body), "topbar") {
		t.Error("press should serve its own page.html (expected its topbar markup)")
	}
	assets, err := press.staticAssets()
	if err != nil {
		t.Fatal(err)
	}
	var hasFonts, hasCSS, leakedBase bool
	for _, a := range assets {
		hasFonts = hasFonts || a == "vendor/fonts.css"
		hasCSS = hasCSS || a == "style.css"
		leakedBase = leakedBase || a == baseMarker
	}
	if !hasFonts {
		t.Error("press should inherit the default theme's vendored fonts")
	}
	if !hasCSS {
		t.Error("press should ship its own style.css")
	}
	if leakedBase {
		t.Errorf("the %q marker must not be emitted as a static asset", baseMarker)
	}
}

func TestUnknownThemeFallsBackToDefault(t *testing.T) {
	// A name with no built-in and no on-disk dir inherits the default's templates.
	ts, err := newThemeSource("", "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ts.read("page.html"); err != nil {
		t.Errorf("unknown theme should fall back to default's page.html: %v", err)
	}
}
