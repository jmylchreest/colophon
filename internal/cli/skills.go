package cli

import (
	"fmt"
	"strings"

	"github.com/jmylchreest/colophon/internal/skills"
)

// SkillsCmd groups installation of colophon's authoring skills into agent harnesses.
type SkillsCmd struct {
	Detect    SkillsDetectCmd    `cmd:"" help:"Show which agent harnesses are present and the install status of each skill"`
	Install   SkillsInstallCmd   `cmd:"" help:"Install/update the skills into detected harnesses (or --harness/--dir)"`
	List      SkillsListCmd      `cmd:"" help:"List the skills embedded in this binary"`
	Uninstall SkillsUninstallCmd `cmd:"" help:"Remove colophon-managed skills from detected harnesses (or --harness/--dir)"`
}

// SkillsListCmd prints the embedded skills.
type SkillsListCmd struct{}

func (c *SkillsListCmd) Run() error {
	emb, err := skills.Embedded()
	if err != nil {
		return err
	}
	fmt.Printf("colophon %s — %d embedded skills:\n", resolveVersion(), len(emb))
	for _, s := range emb {
		fmt.Printf("  %-20s %s\n", s.Name, truncate(s.Description, 88))
	}
	return nil
}

// SkillsDetectCmd reports detected harnesses and per-skill status in each install dir.
type SkillsDetectCmd struct {
	Dir string `help:"Inspect a specific skills directory instead of detecting harnesses"`
}

func (c *SkillsDetectCmd) Run() error {
	emb, err := skills.Embedded()
	if err != nil {
		return err
	}
	home, err := skills.HomeDir()
	if err != nil {
		return err
	}
	fmt.Printf("colophon %s — %d skills available to install\n\n", resolveVersion(), len(emb))

	if c.Dir != "" {
		printStatus(skills.Target{Dir: c.Dir}, emb)
		return nil
	}
	detected := skills.Detect(home)
	if len(detected) == 0 {
		fmt.Println("No supported agent harness detected (Claude Code, Codex, opencode, Cursor, Copilot, Gemini CLI).")
		fmt.Println("Run `colophon skills install --dir <path>` or `--all` to install anyway.")
		return nil
	}
	for _, t := range skills.Targets(home, detected) {
		printStatus(t, emb)
	}
	return nil
}

// SkillsInstallCmd installs/updates the embedded skills.
type SkillsInstallCmd struct {
	Harness []string `help:"Install only for these harness ids (claude,codex,opencode,cursor,copilot,gemini)"`
	Dir     string   `help:"Install into a specific directory instead of detected harnesses"`
	All     bool     `help:"Install for every supported harness, detected or not"`
	Force   bool     `help:"Overwrite locally-modified or unmanaged skills"`
	DryRun  bool     `help:"Show what would change without writing"`
}

func (c *SkillsInstallCmd) Run() error {
	emb, err := skills.Embedded()
	if err != nil {
		return err
	}
	targets, err := resolveTargets(c.Dir, c.Harness, c.All)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Println("No supported agent harness detected. Install one, or pass --dir <path> or --all.")
		return nil
	}
	version := resolveVersion()
	for _, t := range targets {
		actions, err := skills.Install(t.Dir, version, emb, c.Force, c.DryRun)
		if err != nil {
			return err
		}
		printActions(t, actions, c.DryRun)
	}
	return nil
}

// SkillsUninstallCmd removes colophon-managed skills.
type SkillsUninstallCmd struct {
	Harness []string `help:"Uninstall only for these harness ids"`
	Dir     string   `help:"Uninstall from a specific directory"`
	All     bool     `help:"Consider every supported harness, detected or not"`
	Force   bool     `help:"Also remove locally-modified or unmanaged skills"`
	DryRun  bool     `help:"Show what would change without deleting"`
}

func (c *SkillsUninstallCmd) Run() error {
	emb, err := skills.Embedded()
	if err != nil {
		return err
	}
	targets, err := resolveTargets(c.Dir, c.Harness, c.All)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Println("No supported agent harness detected. Pass --dir <path> or --all.")
		return nil
	}
	for _, t := range targets {
		actions, err := skills.Uninstall(t.Dir, emb, c.Force, c.DryRun)
		if err != nil {
			return err
		}
		printActions(t, actions, c.DryRun)
	}
	return nil
}

// resolveTargets turns the --dir/--harness/--all flags (else detection) into install directories.
func resolveTargets(dir string, ids []string, all bool) ([]skills.Target, error) {
	if dir != "" {
		return []skills.Target{{Dir: dir}}, nil
	}
	home, err := skills.HomeDir()
	if err != nil {
		return nil, err
	}
	var hs []skills.Harness
	switch {
	case all:
		hs = skills.Harnesses()
	case len(ids) > 0:
		byID := map[string]skills.Harness{}
		for _, h := range skills.Harnesses() {
			byID[h.ID] = h
		}
		for _, id := range ids {
			h, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unknown harness %q", id)
			}
			hs = append(hs, h)
		}
	default:
		hs = skills.Detect(home)
	}
	return skills.Targets(home, hs), nil
}

func printStatus(t skills.Target, emb []skills.Skill) {
	fmt.Printf("%s%s\n", t.Dir, harnessSuffix(t))
	for _, st := range skills.StatusFor(t.Dir, emb) {
		ver := ""
		if st.Version != "" && st.Status != skills.StatusUpToDate {
			ver = " (" + st.Version + ")"
		}
		fmt.Printf("    %-20s %s%s\n", st.Skill.Name, st.Status, ver)
	}
	printNote(t)
	fmt.Println()
}

func printActions(t skills.Target, actions []skills.Action, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "[dry-run] "
	}
	fmt.Printf("%s%s%s\n", prefix, t.Dir, harnessSuffix(t))
	for _, a := range actions {
		fmt.Printf("    %-20s %s\n", a.Skill, a.Result)
	}
	printNote(t)
	fmt.Println()
}

func harnessSuffix(t skills.Target) string {
	if len(t.Harnesses) == 0 {
		return ""
	}
	names := make([]string, len(t.Harnesses))
	for i, h := range t.Harnesses {
		names[i] = h.Name
	}
	return "  (" + strings.Join(names, ", ") + ")"
}

func printNote(t skills.Target) {
	for _, h := range t.Harnesses {
		if h.Note != "" {
			fmt.Printf("    note: %s\n", h.Note)
		}
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
