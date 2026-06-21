// Package skills installs colophon's embedded authoring SKILL.md files into whichever agent
// harness is present on the machine (Claude Code, Codex, opencode, Cursor, Copilot, Gemini CLI).
// Installed files carry a comment marker recording the colophon version + content hash, so a later
// run can tell whether each copy is up to date, superseded, or locally edited.
package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	embedded "github.com/jmylchreest/colophon/contrib/skills"
)

// Skill is one embedded skill: its directory name, the SKILL.md metadata, the canonical file
// content (no marker), and a short content hash used for staleness detection.
type Skill struct {
	Name        string // directory name, e.g. colophon-write
	Title       string // frontmatter name
	Description string
	Content     string // canonical SKILL.md bytes (as embedded, no marker)
	Hash        string // sha256(Content)[:12]
}

// Embedded returns every skill baked into the binary, sorted by name.
func Embedded() ([]Skill, error) {
	const root = "skills"
	entries, err := embedded.FS.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := embedded.FS.ReadFile(path.Join(root, e.Name(), "SKILL.md"))
		if err != nil {
			continue // a skill dir without a SKILL.md is not a skill
		}
		content := string(b)
		title, desc := frontmatterMeta(content)
		out = append(out, Skill{
			Name: e.Name(), Title: title, Description: desc,
			Content: content, Hash: hashContent(content),
		})
	}
	return out, nil
}

var fmNameRE = regexp.MustCompile(`(?m)^name:[ \t]*(.+?)[ \t]*$`)
var fmDescRE = regexp.MustCompile(`(?m)^description:[ \t]*(.+?)[ \t]*$`)

// frontmatterMeta pulls name/description out of a SKILL.md's leading --- block. It reads the
// lines directly rather than YAML-parsing, because authored SKILL.md descriptions routinely
// contain "key: value" fragments (e.g. "the seo: block") that aren't valid YAML plain scalars
// but which every harness accepts.
func frontmatterMeta(content string) (name, desc string) {
	block := content
	if strings.HasPrefix(content, "---") {
		if i := strings.Index(content[3:], "\n---"); i >= 0 {
			block = content[3 : 3+i]
		}
	}
	if m := fmNameRE.FindStringSubmatch(block); m != nil {
		name = strings.Trim(m[1], `"'`)
	}
	if m := fmDescRE.FindStringSubmatch(block); m != nil {
		desc = strings.Trim(m[1], `"'`)
	}
	return name, desc
}

func hashContent(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}

// --- harnesses ---------------------------------------------------------------

// Harness is a supported agent tool: how to detect it, and where its user-level skills live.
type Harness struct {
	ID, Name  string
	configRel string // config dir relative to $HOME (presence ⇒ the tool is used); "" if none
	bin       string // binary name on PATH; "" if none
	skillsRel string // user-level skills dir relative to $HOME
	Note      string // extra guidance (e.g. the Claude plugin-marketplace alternative)
}

// Harnesses is the static table of supported tools. Codex, opencode, Cursor and Copilot all read
// the tool-neutral ~/.agents/skills, so they share one target; Claude Code and Gemini CLI use
// their own. Aider has no skill discovery and is intentionally absent.
func Harnesses() []Harness {
	return []Harness{
		{ID: "claude", Name: "Claude Code", configRel: ".claude", bin: "claude", skillsRel: ".claude/skills",
			Note: "alternatively: /plugin marketplace add jmylchreest/colophon && /plugin install colophon-skills@colophon (self-updating)"},
		{ID: "codex", Name: "Codex CLI", configRel: ".codex", bin: "codex", skillsRel: ".agents/skills"},
		{ID: "opencode", Name: "opencode", configRel: ".config/opencode", bin: "opencode", skillsRel: ".agents/skills"},
		{ID: "cursor", Name: "Cursor", configRel: ".cursor", skillsRel: ".agents/skills"},
		{ID: "copilot", Name: "GitHub Copilot", configRel: ".copilot", bin: "copilot", skillsRel: ".agents/skills"},
		{ID: "gemini", Name: "Gemini CLI", configRel: ".gemini", bin: "gemini", skillsRel: ".gemini/skills"},
	}
}

// Detected reports whether the harness is present: its config dir exists, or its binary is on PATH.
func (h Harness) Detected(home string) bool {
	if h.configRel != "" {
		if fi, err := os.Stat(filepath.Join(home, h.configRel)); err == nil && fi.IsDir() {
			return true
		}
	}
	if h.bin != "" {
		if _, err := exec.LookPath(h.bin); err == nil {
			return true
		}
	}
	return false
}

// Dir is the harness's absolute user-level skills directory.
func (h Harness) Dir(home string) string { return filepath.Join(home, h.skillsRel) }

// Target is one install directory plus the detected harnesses that read it (several tools share
// ~/.agents/skills, so one Target can serve more than one harness).
type Target struct {
	Dir       string
	Harnesses []Harness
}

// Targets groups the given harnesses by their install directory, preserving first-seen order.
func Targets(home string, harnesses []Harness) []Target {
	var out []Target
	idx := map[string]int{}
	for _, h := range harnesses {
		dir := h.Dir(home)
		if i, ok := idx[dir]; ok {
			out[i].Harnesses = append(out[i].Harnesses, h)
			continue
		}
		idx[dir] = len(out)
		out = append(out, Target{Dir: dir, Harnesses: []Harness{h}})
	}
	return out
}

// Detect returns the harnesses present on this machine.
func Detect(home string) []Harness {
	var out []Harness
	for _, h := range Harnesses() {
		if h.Detected(home) {
			out = append(out, h)
		}
	}
	return out
}

// --- marker + status ---------------------------------------------------------

var markerRE = regexp.MustCompile(`(?m)^# colophon-skill: (\S+) sha:([0-9a-f]+).*\n`)

// markerLine is the YAML comment injected into an installed SKILL.md. It is invisible to every
// harness (a comment inside the frontmatter) but lets us recognise and version our own files.
func markerLine(version, hash string) string {
	return fmt.Sprintf("# colophon-skill: %s sha:%s — managed by `colophon skills`; local edits are overwritten on update\n", version, hash)
}

// withMarker injects the marker as the first line after the opening frontmatter fence.
func withMarker(content, version, hash string) string {
	if i := strings.Index(content, "\n"); strings.HasPrefix(content, "---") && i >= 0 {
		return content[:i+1] + markerLine(version, hash) + content[i+1:]
	}
	return markerLine(version, hash) + content
}

// Status classifies an installed skill relative to its embedded counterpart.
type Status string

const (
	StatusMissing   Status = "not installed"
	StatusUpToDate  Status = "up to date"
	StatusOutdated  Status = "update available"
	StatusModified  Status = "locally modified"
	StatusUnmanaged Status = "unmanaged" // a same-named skill we didn't install
)

// SkillStatus is the per-skill result for one directory.
type SkillStatus struct {
	Skill   Skill
	Status  Status
	Version string // version recorded in the installed marker, when present
}

// statusOf inspects the on-disk SKILL.md for skill s in dir.
func statusOf(dir string, s Skill) SkillStatus {
	p := filepath.Join(dir, s.Name, "SKILL.md")
	b, err := os.ReadFile(p)
	if err != nil {
		return SkillStatus{Skill: s, Status: StatusMissing}
	}
	content := string(b)
	m := markerRE.FindStringSubmatch(content)
	if m == nil {
		return SkillStatus{Skill: s, Status: StatusUnmanaged}
	}
	markerVer, markerHash := m[1], m[2]
	canonical := markerRE.ReplaceAllString(content, "")
	switch {
	case hashContent(canonical) != markerHash:
		return SkillStatus{Skill: s, Status: StatusModified, Version: markerVer}
	case markerHash != s.Hash:
		return SkillStatus{Skill: s, Status: StatusOutdated, Version: markerVer}
	default:
		return SkillStatus{Skill: s, Status: StatusUpToDate, Version: markerVer}
	}
}

// StatusFor returns the status of every skill in dir.
func StatusFor(dir string, skills []Skill) []SkillStatus {
	out := make([]SkillStatus, len(skills))
	for i, s := range skills {
		out[i] = statusOf(dir, s)
	}
	return out
}

// --- install / uninstall -----------------------------------------------------

// Action records what install/uninstall did (or would do) for one skill.
type Action struct {
	Skill  string
	Result string // installed | updated | unchanged | skipped (reason) | removed
}

// Install writes (or updates) the embedded skills into dir, injecting the version marker. An
// up-to-date skill is left untouched; a locally-modified or unmanaged file is skipped unless force.
// dryRun reports the actions without touching disk.
func Install(dir, version string, skills []Skill, force, dryRun bool) ([]Action, error) {
	var actions []Action
	for _, st := range StatusFor(dir, skills) {
		s := st.Skill
		switch st.Status {
		case StatusUpToDate:
			actions = append(actions, Action{s.Name, "unchanged"})
			continue
		case StatusModified, StatusUnmanaged:
			if !force {
				actions = append(actions, Action{s.Name, "skipped (" + string(st.Status) + "; use --force)"})
				continue
			}
		}
		result := "installed"
		if st.Status == StatusOutdated || st.Status == StatusModified || st.Status == StatusUnmanaged {
			result = "updated"
		}
		if !dryRun {
			d := filepath.Join(dir, s.Name)
			if err := os.MkdirAll(d, 0o755); err != nil {
				return actions, err
			}
			if err := os.WriteFile(filepath.Join(d, "SKILL.md"), []byte(withMarker(s.Content, version, s.Hash)), 0o644); err != nil {
				return actions, err
			}
		}
		actions = append(actions, Action{s.Name, result})
	}
	return actions, nil
}

// Uninstall removes colophon-managed skills from dir. A locally-modified or unmanaged file is left
// in place unless force. dryRun reports without deleting.
func Uninstall(dir string, skills []Skill, force, dryRun bool) ([]Action, error) {
	var actions []Action
	for _, st := range StatusFor(dir, skills) {
		s := st.Skill
		switch st.Status {
		case StatusMissing:
			continue
		case StatusModified, StatusUnmanaged:
			if !force {
				actions = append(actions, Action{s.Name, "skipped (" + string(st.Status) + "; use --force)"})
				continue
			}
		}
		if !dryRun {
			if err := os.RemoveAll(filepath.Join(dir, s.Name)); err != nil {
				return actions, err
			}
		}
		actions = append(actions, Action{s.Name, "removed"})
	}
	return actions, nil
}

// HomeDir returns the user's home directory.
func HomeDir() (string, error) { return os.UserHomeDir() }
