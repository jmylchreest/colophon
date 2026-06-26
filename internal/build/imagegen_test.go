package build

import "testing"

func TestParseGenRef(t *testing.T) {
	cases := []struct {
		in         string
		wantOK     bool
		wantPrompt string
		wantAspect string
	}{
		{"gen:a fox in snow", true, "a fox in snow", ""},
		{"gen:a fox?aspect=16:9", true, "a fox", "16:9"},
		{"gen: a fox ?aspect=1:1&seed=3", true, "a fox", "1:1"},
		{"gen:why is the sky blue?", true, "why is the sky blue?", ""}, // trailing ? with no k=v stays in prompt
		{"posts/hero.png", false, "", ""},
		{"https://x/y.png", false, "", ""},
		{"gen:", false, "", ""},
		{"gen:   ", false, "", ""},
	}
	for _, c := range cases {
		g, ok := parseGenRef(c.in)
		if ok != c.wantOK {
			t.Errorf("parseGenRef(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if g.Prompt != c.wantPrompt {
			t.Errorf("parseGenRef(%q) prompt = %q, want %q", c.in, g.Prompt, c.wantPrompt)
		}
		if g.Params["aspect"] != c.wantAspect {
			t.Errorf("parseGenRef(%q) aspect = %q, want %q", c.in, g.Params["aspect"], c.wantAspect)
		}
	}
}

func TestEffectiveSystem(t *testing.T) {
	const def = "house style"
	cases := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"inherits default", map[string]string{"aspect": "16:9"}, def},
		{"override", map[string]string{"systemprompt": "woodcut, two-tone"}, "woodcut, two-tone"},
		{"suppress none", map[string]string{"systemprompt": "none"}, ""},
		{"suppress nil", map[string]string{"systemprompt": "nil"}, ""},
		{"suppress empty", map[string]string{"systemprompt": ""}, ""},
	}
	for _, c := range cases {
		got, clean := effectiveSystem(c.params, def)
		if got != c.want {
			t.Errorf("%s: system = %q, want %q", c.name, got, c.want)
		}
		if _, leaked := clean[systemPromptParam]; leaked {
			t.Errorf("%s: systemprompt must be stripped from provider params", c.name)
		}
	}
}

func TestGenResolverInertWhenNil(t *testing.T) {
	var gr *genResolver
	if gr.active() {
		t.Error("nil resolver must be inert")
	}
	if _, _, ok := gr.resolveURL("gen:a fox", "", "/", "", nil); ok {
		t.Error("nil resolver must not resolve gen refs")
	}
	if got := rewriteGenRefs("![x](<gen:a fox>)", "", "/", "", nil, gr); got != "![x](<gen:a fox>)" {
		t.Errorf("nil resolver must leave body unchanged, got %q", got)
	}
}
