package cli

import (
	"path/filepath"
	"time"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
)

// BuildCmd renders content/ into the canonical static tree under public/. With --env
// it applies that environment's overrides (include_drafts, title, base_url).
type BuildCmd struct {
	Env     string `help:"Build a named environment (applies its overrides)"`
	Verbose bool   `short:"v" help:"Log each step (sources, files, feeds)"`
}

func (c *BuildCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}

	log := newLogger(c.Verbose)
	tel := telemetryFor(cfg, c.Env, root)
	defer tel.Flush()
	opts := build.Options{OutDir: filepath.Join(root, "public"), Log: log, Env: c.Env, Telemetry: tel}
	if c.Env != "" {
		env := cfg.Environment(c.Env)
		if env == nil {
			return unknownEnvErr(cfg, c.Env)
		}
		opts.IncludeDrafts = env.IncludeDrafts
		opts.Title = env.Title
		opts.BaseURL = env.BaseURL
		opts.Theme = env.Theme
		opts.Publishers = env.Publish
	}

	res, err := build.Run(cfg, opts)
	if err != nil {
		return err
	}
	log.Step("BUILD", "", "out", res.OutDir)
	if res.NextEmbargo != nil {
		log.Step("BUILD", "", "next_embargo", res.NextEmbargo.Format(time.RFC3339))
	}
	return nil
}
