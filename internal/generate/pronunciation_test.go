package generate

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPronunciationDict_YAML(t *testing.T) {
	p := writeTemp(t, "d.yaml", "pronunciations:\n  - word: schedule\n    ipa: ʃˈɛdjuːl\n  - word: nginx\n    say: engine x\n")
	got, err := LoadPronunciationDict(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []Pronunciation{{Word: "schedule", IPA: "ʃˈɛdjuːl"}, {Word: "nginx", Say: "engine x"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadPronunciationDict_LegacyJSONTone(t *testing.T) {
	p := writeTemp(t, "d.json", `{"tone":["schedule/(ʃˈɛdjuːl)","omg/oh my god"]}`)
	got, err := LoadPronunciationDict(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []Pronunciation{{Word: "schedule", IPA: "ʃˈɛdjuːl"}, {Word: "omg", Say: "oh my god"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestBuiltinPronunciationDict_enGB(t *testing.T) {
	entries, ok, err := BuiltinPronunciationDict("en_GB")
	if err != nil || !ok {
		t.Fatalf("en_GB: ok=%v err=%v", ok, err)
	}
	if len(entries) == 0 {
		t.Fatal("en_GB dict is empty")
	}
	var zebra bool
	for _, p := range entries {
		if p.Word == "zebra" && p.Say == "zebbra" {
			zebra = true
		}
	}
	if !zebra {
		t.Error("expected zebra → say: zebbra in en_GB")
	}
	if _, ok, _ := BuiltinPronunciationDict("nope_XX"); ok {
		t.Error("unknown name should report ok=false")
	}
	if got := BuiltinPronunciationDicts(); len(got) == 0 || got[0] == "" {
		t.Errorf("BuiltinPronunciationDicts() = %v", got)
	}
}

func TestResolvePronunciationDict(t *testing.T) {
	// Bare name → built-in.
	if entries, err := ResolvePronunciationDict("en_GB", "/nonexistent"); err != nil || len(entries) == 0 {
		t.Fatalf("en_GB builtin: %d entries, err=%v", len(entries), err)
	}
	// Bare unknown name → error listing available built-ins (not a file lookup).
	if _, err := ResolvePronunciationDict("nope_XX", t.TempDir()); err == nil {
		t.Error("unknown bare name should error")
	}
	// Path-looking ref → file, relative to root.
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "custom.yaml"), []byte("pronunciations:\n  - word: foo\n    say: bar\n"), 0o644)
	got, err := ResolvePronunciationDict("custom.yaml", root)
	if err != nil || len(got) != 1 || got[0].Word != "foo" {
		t.Fatalf("custom.yaml: %+v err=%v", got, err)
	}
}

func TestFilterPronunciation_WordBoundary(t *testing.T) {
	dict := []Pronunciation{{Word: "route", IPA: "rˈuːt"}, {Word: "router", IPA: "ˈruːtə"}}
	if got := FilterPronunciation(dict, "the router restarted"); !reflect.DeepEqual(got, []Pronunciation{{Word: "router", IPA: "ˈruːtə"}}) {
		t.Errorf("router-only text matched %+v", got)
	}
	if got := FilterPronunciation(dict, "take the scenic route, please"); !reflect.DeepEqual(got, []Pronunciation{{Word: "route", IPA: "rˈuːt"}}) {
		t.Errorf("route-only text matched %+v", got)
	}
}

func TestMinimaxTone(t *testing.T) {
	got := minimaxTone([]Pronunciation{{Word: "schedule", IPA: "ʃˈɛdjuːl"}, {Word: "nginx", Say: "engine x"}})
	want := []string{"schedule/(ʃˈɛdjuːl)", "nginx/engine x"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestApplySayAliases(t *testing.T) {
	ps := []Pronunciation{{Word: "nginx", Say: "engine x"}, {Word: "schedule", IPA: "ʃˈɛdjuːl"}}
	got := applySayAliases("Configure NGINX on a schedule.", ps)
	if got != "Configure engine x on a schedule." { // say applied, ipa-only left alone
		t.Errorf("got %q", got)
	}
}

// Pronunciation must affect the cache key so a changed override regenerates the audio.
func TestSpeechStem_PronunciationInvalidatesCache(t *testing.T) {
	base := SpeechStem("minimax", "m1", "v1", "post", "hello garage", nil)
	a := SpeechStem("minimax", "m1", "v1", "post", "hello garage", []Pronunciation{{Word: "garage", IPA: "ɡˈærɪdʒ"}})
	b := SpeechStem("minimax", "m1", "v1", "post", "hello garage", []Pronunciation{{Word: "garage", IPA: "ɡəˈrɑːʒ"}})
	if base == a || a == b {
		t.Error("pronunciation changes must change the stem")
	}
}
