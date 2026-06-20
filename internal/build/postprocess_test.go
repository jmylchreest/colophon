package build

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// barredImage builds an RGBA image with solid black top/bottom bars of the given thickness
// and a mid-grey content band between them, PNG-encoded.
func barredImage(t *testing.T, w, h, bar int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		c := color.RGBA{0, 0, 0, 255}
		if y >= bar && y < h-bar {
			c = color.RGBA{128, 128, 128, 255}
		}
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestTrimLetterboxRemovesBars(t *testing.T) {
	raw := barredImage(t, 320, 200, 20) // 20px black bars top+bottom → 160px content
	out, ok := trimLetterbox(raw, 0)    // no re-frame, just trim
	if !ok {
		t.Fatal("expected trimming to occur")
	}
	got := decode(t, out)
	if h := got.Bounds().Dy(); h != 160 {
		t.Errorf("height after trim = %d, want 160", h)
	}
	// The top row of the trimmed image must no longer be black.
	if pixelDark(got, got.Bounds().Min.X+1, got.Bounds().Min.Y+1) {
		t.Error("top of trimmed image is still black")
	}
}

func TestTrimLetterboxReframesToAspect(t *testing.T) {
	raw := barredImage(t, 320, 200, 20) // trims to 320x160 (2.0); re-frame to 16:9 (~1.778)
	out, ok := trimLetterbox(raw, 16.0/9.0)
	if !ok {
		t.Fatal("expected trim + reframe")
	}
	got := decode(t, out)
	ar := float64(got.Bounds().Dx()) / float64(got.Bounds().Dy())
	if ar < 1.7 || ar > 1.85 {
		t.Errorf("aspect after reframe = %.3f, want ~1.778", ar)
	}
}

func TestTrimLetterboxNoBarsNoChange(t *testing.T) {
	raw := barredImage(t, 320, 180, 0) // no bars, already 16:9
	if _, ok := trimLetterbox(raw, 16.0/9.0); ok {
		t.Error("a clean, correctly-framed image should not be re-encoded")
	}
}

func TestTrimLetterboxGuardsOverCrop(t *testing.T) {
	// An all-black image must not be trimmed to nothing: each edge is capped at 40%.
	raw := barredImage(t, 100, 100, 50) // "bars" cover the whole image
	out, ok := trimLetterbox(raw, 0)
	if ok {
		got := decode(t, out)
		if got.Bounds().Dy() < 20 {
			t.Errorf("over-crop guard failed: height %d", got.Bounds().Dy())
		}
	}
}

func TestAspectValue(t *testing.T) {
	cases := map[string]float64{"16:9": 16.0 / 9.0, "1:1": 1, "": 0, "garbage": 0, "2": 2}
	for in, want := range cases {
		if got := aspectValue(map[string]string{"aspect": in}); got != want {
			t.Errorf("aspectValue(%q) = %v, want %v", in, got, want)
		}
	}
}
