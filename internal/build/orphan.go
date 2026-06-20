package build

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/generate"
)

// OrphanedGenerated returns files in the generated-asset cache (AI images, TTS audio, and
// their sidecars) that no current content references — left behind by an edited prompt, a
// changed style/model/voice, or a deleted post. It computes the live set by running the
// real collection (a throwaway build, no API calls), so the match is exact and a prune
// never deletes an asset a build would still use. Returns nil when generation is off.
func OrphanedGenerated(cfg *config.Config) ([]string, error) {
	dirs := generatedCacheDirs(cfg)
	var present []string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			present = append(present, d)
		}
	}
	if len(present) == 0 {
		return nil, nil
	}

	tmp, err := os.MkdirTemp("", "colophon-orphan-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	// Drafts/embargoed included so their (still-referenced) assets aren't seen as orphans.
	res, err := Run(cfg, Options{OutDir: tmp, IncludeDrafts: true, GenerateAI: false, Log: clog.Discard()})
	if err != nil {
		return nil, err
	}
	live := make(map[string]bool, len(res.Generated))
	for _, stem := range res.Generated {
		live[stem] = true
	}

	var orphans []string
	for _, dir := range present {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), ".json") {
				continue // sidecars are pruned alongside their asset
			}
			stem := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
			if !live[stem] {
				orphans = append(orphans, filepath.Join(dir, e.Name()))
			}
		}
	}
	return orphans, nil
}

// PruneGenerated deletes each orphan file and its sidecar. Missing files are ignored.
func PruneGenerated(orphans []string) error {
	for _, p := range orphans {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Remove(p + ".json"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// generatedCacheDirs is the set of cache directories generation writes to (image and/or
// speech output_dir, each defaulting to the shared default). Empty when generation is off,
// so a prune never touches the cache for a project that has simply disabled generation.
func generatedCacheDirs(cfg *config.Config) []string {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		if strings.TrimSpace(d) == "" {
			d = generate.DefaultOutputDir
		}
		if !filepath.IsAbs(d) {
			d = filepath.Join(cfg.Root, filepath.FromSlash(d))
		}
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	if cfg.Generation.Image.Configured() {
		add(cfg.Generation.Image.OutputDir)
	}
	if cfg.Generation.Speech.Configured() {
		add(cfg.Generation.Speech.OutputDir)
	}
	return out
}
