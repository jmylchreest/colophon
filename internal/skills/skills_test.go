package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedSkills(t *testing.T) {
	got, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 5 {
		t.Fatalf("expected the bundled skills, got %d", len(got))
	}
	for _, s := range got {
		if s.Name == "" || s.Content == "" || len(s.Hash) != 12 {
			t.Errorf("incomplete skill: %+v", Skill{Name: s.Name, Hash: s.Hash})
		}
		// Descriptions can contain "key: value" fragments; they must still be extracted.
		if s.Name == "colophon-metadata" && !strings.Contains(s.Description, "seo:") {
			t.Errorf("metadata description not extracted: %q", s.Description)
		}
	}
}

func TestInstallStatusLifecycle(t *testing.T) {
	dir := t.TempDir()
	emb, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := Install(dir, "v1.2.3", emb, false, false); err != nil {
		t.Fatal(err)
	}
	for _, st := range StatusFor(dir, emb) {
		if st.Status != StatusUpToDate {
			t.Errorf("%s: want up-to-date, got %s", st.Skill.Name, st.Status)
		}
	}

	// Marker is injected as a frontmatter comment, invisible to harnesses.
	first := emb[0]
	p := filepath.Join(dir, first.Name, "SKILL.md")
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# colophon-skill: v1.2.3 sha:"+first.Hash) {
		t.Errorf("marker missing/incorrect in %s:\n%s", first.Name, b)
	}

	// Editing the body → locally modified; install without force skips it.
	if err := os.WriteFile(p, append(b, []byte("\nedited\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(dir, first).Status; got != StatusModified {
		t.Errorf("after edit: want modified, got %s", got)
	}
	acts, _ := Install(dir, "v1.2.3", emb, false, false)
	if !strings.Contains(acts[0].Result, "skipped") {
		t.Errorf("modified skill should be skipped without force, got %q", acts[0].Result)
	}
	// With force it is restored.
	if _, err := Install(dir, "v1.2.3", emb, true, false); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(dir, first).Status; got != StatusUpToDate {
		t.Errorf("after force install: want up-to-date, got %s", got)
	}

	// A marker-less file is unmanaged and left alone by uninstall without force.
	if err := os.WriteFile(p, []byte("---\nname: x\n---\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(dir, first).Status; got != StatusUnmanaged {
		t.Errorf("want unmanaged, got %s", got)
	}
	uacts, _ := Uninstall(dir, emb, false, false)
	if !strings.Contains(uacts[0].Result, "skipped") {
		t.Errorf("unmanaged skill should be skipped on uninstall, got %q", uacts[0].Result)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("unmanaged file should remain: %v", err)
	}
}

func TestTargetsDedupe(t *testing.T) {
	// codex + cursor both map to ~/.agents/skills → one target serving both.
	var codex, cursor Harness
	for _, h := range Harnesses() {
		switch h.ID {
		case "codex":
			codex = h
		case "cursor":
			cursor = h
		}
	}
	ts := Targets("/home/u", []Harness{codex, cursor})
	if len(ts) != 1 {
		t.Fatalf("want 1 deduped target, got %d", len(ts))
	}
	if len(ts[0].Harnesses) != 2 {
		t.Errorf("want both harnesses on the shared dir, got %d", len(ts[0].Harnesses))
	}
	if !strings.HasSuffix(ts[0].Dir, "/.agents/skills") {
		t.Errorf("shared dir should be ~/.agents/skills, got %s", ts[0].Dir)
	}
}
