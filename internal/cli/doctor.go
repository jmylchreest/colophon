package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/publish"
	"github.com/jmylchreest/colophon/internal/render"
	"github.com/jmylchreest/colophon/internal/source"
)

// DoctorCmd validates the project: config structure (via config.Load), then drivers, themes,
// publisher readiness, credentials, env refs and content. Errors exit non-zero; warnings don't.
type DoctorCmd struct{}

// report accumulates findings. Errors are problems that will break a build/publish; warnings are
// things worth knowing (a fall-back kicked in, a credential isn't set yet) but not fatal.
type report struct {
	errs, warns []string
}

func (r *report) err(format string, a ...any)  { r.errs = append(r.errs, fmt.Sprintf(format, a...)) }
func (r *report) warn(format string, a ...any) { r.warns = append(r.warns, fmt.Sprintf(format, a...)) }

func (c *DoctorCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	// config.Load already validates structure + cross-references (unknown publisher/persona
	// refs, missing ids/drivers, malformed author/persona yaml). A failure here is a hard stop.
	cfg, err := config.Load(root)
	if err != nil {
		fmt.Printf("✗ %s\n  config: %v\n", root, err)
		return fmt.Errorf("config invalid")
	}

	r := &report{}
	checkPublishers(root, cfg, r)
	checkSources(root, cfg, r)
	checkThemes(root, cfg, r)
	checkCredentials(cfg, r)
	checkEnvRefs(cfg, r)
	checkContent(cfg, r)

	fmt.Println(root)
	fmt.Printf("  sites %d · publishers %d · environments %d (%s) · personas %d · authors %d\n",
		len(cfg.Sites), len(cfg.Publishers), len(cfg.Environments),
		strings.Join(cfg.EnvironmentNames(), ", "), len(cfg.Personas), len(cfg.Authors))
	for _, w := range r.warns {
		fmt.Printf("  ⚠ %s\n", w)
	}
	for _, e := range r.errs {
		fmt.Printf("  ✗ %s\n", e)
	}
	if len(r.errs) == 0 && len(r.warns) == 0 {
		fmt.Println("  ✓ all checks passed")
	}
	if len(r.errs) > 0 {
		return fmt.Errorf("%d problem(s) found", len(r.errs))
	}
	return nil
}

// checkPublishers verifies each publisher's driver is registered (error) and that it can be
// constructed from its config (warning — an incomplete field is often an unset {env:} that only
// matters at publish time, so it shouldn't fail a local checkup).
func checkPublishers(root string, cfg *config.Config, r *report) {
	known := toSet(publish.Drivers())
	for _, p := range cfg.Publishers {
		if !known[p.Driver] {
			r.err("publisher %q: unknown driver %q (have: %s)", p.ID, p.Driver, strings.Join(publish.Drivers(), ", "))
			continue
		}
		if _, err := publish.Open(root, p); err != nil {
			r.warn("publisher %q not deploy-ready: %v", p.ID, err)
		}
	}
}

// checkSources verifies each source's driver is registered (error) and that its content is
// actually reachable (warning — a missing path/vault just means the source contributes nothing).
func checkSources(root string, cfg *config.Config, r *report) {
	known := toSet(source.Drivers())
	for _, s := range cfg.Sources {
		if !known[s.Driver] {
			r.err("source %q: unknown driver %q (have: %s)", s.ID, s.Driver, strings.Join(source.Drivers(), ", "))
			continue
		}
		switch s.Driver {
		case "md-dir":
			if p := setting(s.Settings, "path"); p != "" && !pathExists(absUnder(root, p)) {
				r.warn("source %q (md-dir): path %q does not exist — nothing is read from it", s.ID, p)
			}
		case "obsidian":
			switch vault := setting(s.Settings, "vault"); {
			case vault == "":
				r.warn("source %q (obsidian): no vault set — source is inert", s.ID)
			case !pathExists(vault):
				r.warn("source %q (obsidian): vault %q does not exist — source is inert", s.ID, vault)
			}
		}
	}
}

// checkThemes verifies every site/environment theme resolves to a built-in or themes/<name>/ on
// disk; an unknown theme silently falls back to the default, so it's a warning.
func checkThemes(root string, cfg *config.Config, r *report) {
	builtin := toSet(render.BuiltinThemes())
	ok := func(theme string) bool {
		if theme == "" || theme == "default" { // default → the canonical default theme
			return true
		}
		if builtin[theme] {
			return true
		}
		fi, err := os.Stat(filepath.Join(root, "themes", theme))
		return err == nil && fi.IsDir()
	}
	for _, s := range cfg.Sites {
		if !ok(s.Theme) {
			r.warn("site %q: theme %q not found (built-in or themes/%s/) — falls back to the default", s.ID, s.Theme, s.Theme)
		}
	}
	for _, e := range cfg.Environments {
		if e.Theme != "" && !ok(e.Theme) {
			r.warn("environment %q: theme %q not found — falls back to the default", e.Name, e.Theme)
		}
	}
}

// checkCredentials warns when a publisher's deploy-secret env vars aren't set. Only publishers an
// environment actually deploys to are checked, and it never reads or validates the values — just
// reports which are absent, since they only matter at publish time.
func checkCredentials(cfg *config.Config, r *report) {
	used := map[string]bool{}
	for _, e := range cfg.Environments {
		for _, id := range e.Publish {
			if p := cfg.Publisher(id); p != nil {
				used[p.Driver] = true
			}
		}
	}
	drivers := make([]string, 0, len(used))
	for d := range used {
		drivers = append(drivers, d)
	}
	sort.Strings(drivers)
	for _, d := range drivers {
		var missing []string
		for _, v := range publish.DriverEnvVars([]string{d}) {
			if _, set := os.LookupEnv(v); !set {
				missing = append(missing, v)
			}
		}
		if len(missing) > 0 {
			r.warn("driver %q: deploy credentials not set: %s (needed only when you publish)", d, strings.Join(missing, ", "))
		}
	}
}

// checkEnvRefs warns about {env:VAR} references the config makes that aren't set. Being unset is
// not necessarily wrong (a default may apply, or it's a target you're not using), hence a warning.
func checkEnvRefs(cfg *config.Config, r *report) {
	var unset []string
	for _, v := range cfg.EnvRefs {
		if _, set := os.LookupEnv(v); !set {
			unset = append(unset, v)
		}
	}
	if len(unset) > 0 {
		sort.Strings(unset)
		r.warn("config references unset env vars: %s (a default applies where given; may be intentional)", strings.Join(unset, ", "))
	}
}

// checkContent loads the entries and flags slug collisions (error — two posts would overwrite the
// same output path) and posts naming an author/persona that isn't defined (warning — it falls back).
func checkContent(cfg *config.Config, r *report) {
	entries, err := build.Entries(cfg)
	if err != nil {
		r.warn("content checks skipped: %v", err)
		return
	}
	authors, personas := map[string]bool{}, map[string]bool{}
	for _, a := range cfg.Authors {
		authors[a.ID] = true
	}
	for _, p := range cfg.Personas {
		personas[p.ID] = true
	}
	seen := map[string]string{}
	for _, e := range entries {
		if prev, dup := seen[e.URL]; dup {
			r.err("slug collision: %q and %q both render to %q", prev, e.Title, e.URL)
		} else {
			seen[e.URL] = e.Title
		}
		if e.Author != "" && !authors[e.Author] {
			r.warn("post %q names unknown author %q (falls back to the default byline)", e.Title, e.Author)
		}
		if e.Persona != "" && !personas[e.Persona] {
			r.warn("post %q names unknown persona %q", e.Title, e.Persona)
		}
	}
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

func setting(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

func absUnder(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
