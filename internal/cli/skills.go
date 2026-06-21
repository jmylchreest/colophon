package cli

import (
	"bufio"
	"fmt"
	"os"
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
		printStatus(home, skills.Target{Dir: c.Dir}, emb)
		return nil
	}
	detected := skills.Detect(home)
	if len(detected) == 0 {
		fmt.Println("No supported agent harness detected (Claude Code, Codex, opencode, Cursor, Copilot, Gemini CLI).")
		fmt.Println("Run `colophon skills install --dir <path>` or `--all` to install anyway.")
		return nil
	}
	for _, t := range skills.Targets(home, detected) {
		printStatus(home, t, emb)
	}
	return nil
}

// SkillsInstallCmd installs/updates the embedded skills.
type SkillsInstallCmd struct {
	Harness []string `help:"Install only for these harness ids (claude,codex,opencode,cursor,copilot,gemini)"`
	Dir     string   `help:"Install into a specific directory instead of detected harnesses"`
	All     bool     `help:"Install for every supported harness, detected or not"`
	Claude  string   `help:"How to install for Claude Code: ask|marketplace|files|skip" default:"ask" enum:"ask,marketplace,files,skip"`
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
	home, _ := skills.HomeDir()
	version := resolveVersion()
	for _, t := range targets {
		// Claude Code can take the skills as files or via the self-updating plugin marketplace,
		// so ask (or honour --claude) before writing into ~/.claude/skills.
		if targetIsClaude(t) && c.handleClaude(home, t) {
			continue // marketplace/skip/already-installed — nothing to copy
		}
		actions, err := skills.Install(t.Dir, version, emb, c.Force, c.DryRun)
		if err != nil {
			return err
		}
		printActions(t, actions, c.DryRun)
	}
	return nil
}

// handleClaude resolves how to handle the Claude target. It returns true when nothing should be
// copied (marketplace / skip / already-installed), or false to fall through to a file install. An
// explicit --claude value wins; "ask" first reports an existing marketplace install, then prompts
// when interactive (else defaults to marketplace, never silently writing files).
func (c *SkillsInstallCmd) handleClaude(home string, t skills.Target) bool {
	mode := c.Claude
	if mode == "" || mode == "ask" {
		if ok, ver := skills.ClaudePlugin(home); ok {
			fmt.Printf("%s%s\n    already installed via the colophon plugin marketplace (colophon-skills %s, self-updating)\n\n", t.Dir, harnessSuffix(t), ver)
			return true
		}
		if isInteractive() {
			mode = promptClaude()
		} else {
			mode = "marketplace"
		}
	}
	switch mode {
	case "marketplace":
		printClaudeMarketplace(t)
		return true
	case "skip":
		fmt.Printf("%s%s\n    skipped (--claude=skip)\n\n", t.Dir, harnessSuffix(t))
		return true
	default: // files
		return false
	}
}

func targetIsClaude(t skills.Target) bool {
	for _, h := range t.Harnesses {
		if h.ID == "claude" {
			return true
		}
	}
	return false
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func promptClaude() string {
	fmt.Print("Claude Code detected. Install skills via the self-updating [m]arketplace plugin, " +
		"as [f]iles in ~/.claude/skills, or [s]kip? [M/f/s] ")
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return "marketplace"
	}
	switch strings.ToLower(strings.TrimSpace(sc.Text())) {
	case "f", "files":
		return "files"
	case "s", "skip":
		return "skip"
	default:
		return "marketplace"
	}
}

func printClaudeMarketplace(t skills.Target) {
	fmt.Printf("%s%s\n", t.Dir, harnessSuffix(t))
	fmt.Println("    install via the colophon plugin marketplace (self-updating):")
	fmt.Println("      /plugin marketplace add jmylchreest/colophon")
	fmt.Println("      /plugin install colophon-skills@colophon")
	fmt.Println("    (or re-run with --claude=files to copy them into ~/.claude/skills)")
	fmt.Println()
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

func printStatus(home string, t skills.Target, emb []skills.Skill) {
	fmt.Printf("%s%s\n", t.Dir, harnessSuffix(t))
	statuses := skills.StatusFor(t.Dir, emb)
	if targetIsClaude(t) {
		if ok, ver := skills.ClaudePlugin(home); ok {
			fmt.Printf("    ✓ installed via the colophon plugin marketplace (colophon-skills %s, self-updating)\n", ver)
			if allMissing(statuses) {
				fmt.Println() // nothing on disk to report; the plugin provides the skills
				return
			}
		}
	}
	for _, st := range statuses {
		ver := ""
		if st.Version != "" && st.Status != skills.StatusUpToDate {
			ver = " (" + st.Version + ")"
		}
		fmt.Printf("    %-20s %s%s\n", st.Skill.Name, st.Status, ver)
	}
	printNote(t)
	fmt.Println()
}

func allMissing(ss []skills.SkillStatus) bool {
	for _, s := range ss {
		if s.Status != skills.StatusMissing {
			return false
		}
	}
	return true
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
