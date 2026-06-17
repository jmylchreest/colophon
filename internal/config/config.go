// Package config loads colophon.yaml (sites + publishers) and the personas/*.yaml
// files, applying {env:VAR} interpolation and basic validation. Deploy secrets are
// resolved from the environment at load time and never round-tripped to an agent.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"

	"github.com/jmylchreest/colophon/internal/core"
)

// unmarshalConf decodes using the existing `yaml` struct tags.
var unmarshalConf = koanf.UnmarshalConf{Tag: "yaml"}

// ConfigFile is the canonical config filename at a project root.
const ConfigFile = "colophon.yaml"

// SourceConfig is a named content origin. Driver selects the implementation (md-dir,
// obsidian, webdav, ...); Settings carries driver-specific fields (path, publish flag).
type SourceConfig struct {
	ID       string         `yaml:"id"`
	Driver   string         `yaml:"driver"`
	Settings map[string]any `yaml:",remain"`
}

// PublisherConfig is a named deploy backend: pure mechanism. Driver selects the
// implementation; Settings carries the driver-specific fields (bucket, project,
// path, ...). What/where/whether to deploy is decided by an Environment.
type PublisherConfig struct {
	ID       string         `yaml:"id"`
	Driver   string         `yaml:"driver"`
	Settings map[string]any `yaml:",remain"`
}

// Merged returns a copy of the publisher with overrides layered over its Settings,
// for per-environment publisher tweaks (e.g. a different Cloudflare branch). The id
// and driver are typed fields and cannot be overridden this way.
func (p PublisherConfig) Merged(overrides map[string]any) PublisherConfig {
	if len(overrides) == 0 {
		return p
	}
	s := make(map[string]any, len(p.Settings)+len(overrides))
	for k, v := range p.Settings {
		s[k] = v
	}
	for k, v := range overrides {
		s[k] = v
	}
	p.Settings = s
	return p
}

// Environment is a named build+deploy profile. No name is privileged; an environment
// chooses which publishers to deploy to, whether drafts are included, optional site
// overrides, and per-publisher setting overrides.
type Environment struct {
	Name          string   `yaml:"name"`
	Publish       []string `yaml:"publish"`
	IncludeDrafts bool     `yaml:"include_drafts"`
	// AllowPublish gates deploys for this environment. Nil/true: deploy normally.
	// False: require the --allow-publish flag (a safety latch for production).
	AllowPublish *bool `yaml:"allow_publish"`
	// Title, BaseURL and Theme override the site's values for this environment when set
	// (Theme lets you preview a theme in one env before promoting it to production).
	Title   string `yaml:"title,omitempty"`
	BaseURL string `yaml:"base_url,omitempty"`
	Theme   string `yaml:"theme,omitempty"`
	// Overrides layers per-publisher Settings, keyed by publisher id.
	Overrides map[string]map[string]any `yaml:"overrides,omitempty"`
}

// Gated reports whether deploying this environment requires the --allow-publish flag.
func (e Environment) Gated() bool { return e.AllowPublish != nil && !*e.AllowPublish }

// Config is the parsed colophon.yaml plus personas loaded from personas/*.yaml.
type Config struct {
	Sites        []core.Site       `yaml:"sites"`
	Sources      []SourceConfig    `yaml:"sources"`
	Publishers   []PublisherConfig `yaml:"publishers"`
	Environments []Environment     `yaml:"environments"`

	// Telemetry is colophon's own usage reporting plus the master switch over all telemetry
	// (the tool's events and every site's reader analytics).
	Telemetry core.Telemetry `yaml:"telemetry,omitempty"`

	// Personas (the hidden writing voices) and Authors (the shown bylines) are populated
	// from personas/*.yaml and authors/*.yaml, not from colophon.yaml.
	Personas []core.Persona `yaml:"-"`
	Authors  []core.Author  `yaml:"-"`

	// Root is the project directory the config was loaded from.
	Root string `yaml:"-"`

	// EnvRefs are the {env:VAR} variable names the config references (set or not), for
	// `colophon env`. Deploy-secret env vars (read by publishers, not in the config) are
	// added on top of these by the command.
	EnvRefs []string `yaml:"-"`
}

// Environment returns the named environment, or nil if not configured.
func (c *Config) Environment(name string) *Environment {
	for i := range c.Environments {
		if c.Environments[i].Name == name {
			return &c.Environments[i]
		}
	}
	return nil
}

// Publisher returns the publisher with the given id, or nil.
func (c *Config) Publisher(id string) *PublisherConfig {
	for i := range c.Publishers {
		if c.Publishers[i].ID == id {
			return &c.Publishers[i]
		}
	}
	return nil
}

// EnvironmentNames lists the configured environment names, in declaration order.
func (c *Config) EnvironmentNames() []string {
	names := make([]string, len(c.Environments))
	for i, e := range c.Environments {
		names[i] = e.Name
	}
	return names
}

// Load reads colophon.yaml and personas/*.yaml from root, interpolates environment
// placeholders, and validates the result.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, ConfigFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	// Populate the environment from the project's dot-env files before interpolation, so
	// {env:VAR} placeholders resolve against them. Real environment variables (e.g. CI
	// secrets) are never overridden — they win over both files.
	loadDotEnv(root)

	k := koanf.New(".")
	if err := k.Load(rawbytes.Provider(interpolateEnv(raw)), yaml.Parser()); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var cfg Config
	if err := k.UnmarshalWithConf("", &cfg, unmarshalConf); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	cfg.Root = root
	cfg.EnvRefs = envRefs(raw)

	personas, err := loadPersonas(filepath.Join(root, "personas"))
	if err != nil {
		return nil, err
	}
	cfg.Personas = personas

	authors, err := loadAuthors(filepath.Join(root, "authors"))
	if err != nil {
		return nil, err
	}
	cfg.Authors = authors

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadPersonas(dir string) ([]core.Persona, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read personas dir %s: %w", dir, err)
	}

	var personas []core.Persona
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read persona %s: %w", p, err)
		}
		k := koanf.New(".")
		if err := k.Load(rawbytes.Provider(interpolateEnv(raw)), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("parse persona %s: %w", p, err)
		}
		var persona core.Persona
		if err := k.UnmarshalWithConf("", &persona, unmarshalConf); err != nil {
			return nil, fmt.Errorf("decode persona %s: %w", p, err)
		}
		personas = append(personas, persona)
	}
	sort.Slice(personas, func(i, j int) bool { return personas[i].ID < personas[j].ID })
	return personas, nil
}

func loadAuthors(dir string) ([]core.Author, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read authors dir %s: %w", dir, err)
	}
	var authors []core.Author
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read author %s: %w", p, err)
		}
		k := koanf.New(".")
		if err := k.Load(rawbytes.Provider(interpolateEnv(raw)), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("parse author %s: %w", p, err)
		}
		var author core.Author
		if err := k.UnmarshalWithConf("", &author, unmarshalConf); err != nil {
			return nil, fmt.Errorf("decode author %s: %w", p, err)
		}
		// Default the id to the file stem (so authors/john.yaml → id "john").
		if author.ID == "" {
			author.ID = strings.TrimSuffix(e.Name(), ".yaml")
		}
		authors = append(authors, author)
	}
	sort.Slice(authors, func(i, j int) bool { return authors[i].ID < authors[j].ID })
	return authors, nil
}

// Author returns the author with the given id, or nil.
func (c *Config) Author(id string) *core.Author {
	for i := range c.Authors {
		if c.Authors[i].ID == id {
			return &c.Authors[i]
		}
	}
	return nil
}

// Validate checks cross-references between sites, publishers, and personas.
func (c *Config) Validate() error {
	pubIDs := make(map[string]bool, len(c.Publishers))
	for _, p := range c.Publishers {
		if p.ID == "" {
			return &core.ValidationError{Field: "publishers", Msg: "publisher missing id"}
		}
		if p.Driver == "" {
			return &core.ValidationError{Field: "publishers." + p.ID, Msg: "missing driver"}
		}
		pubIDs[p.ID] = true
	}

	srcIDs := make(map[string]bool, len(c.Sources))
	for _, s := range c.Sources {
		if s.ID == "" {
			return &core.ValidationError{Field: "sources", Msg: "source missing id"}
		}
		if s.Driver == "" {
			return &core.ValidationError{Field: "sources." + s.ID, Msg: "missing driver"}
		}
		if srcIDs[s.ID] {
			return &core.ValidationError{Field: "sources", Msg: "duplicate source: " + s.ID}
		}
		srcIDs[s.ID] = true
	}

	personaIDs := make(map[string]bool, len(c.Personas))
	for _, p := range c.Personas {
		if err := p.Validate(); err != nil {
			return err
		}
		personaIDs[p.ID] = true
	}

	authorIDs := make(map[string]bool, len(c.Authors))
	for _, a := range c.Authors {
		if err := a.Validate(); err != nil {
			return err
		}
		if authorIDs[a.ID] {
			return &core.ValidationError{Field: "authors", Msg: "duplicate author: " + a.ID}
		}
		authorIDs[a.ID] = true
	}

	for _, s := range c.Sites {
		if s.ID == "" {
			return &core.ValidationError{Field: "sites", Msg: "site missing id"}
		}
		for _, ref := range s.Publishers {
			if !pubIDs[ref] {
				return &core.ValidationError{Field: "sites." + s.ID + ".publishers", Msg: "unknown publisher: " + ref}
			}
		}
		for _, ref := range s.Personas {
			if !personaIDs[ref] {
				return &core.ValidationError{Field: "sites." + s.ID + ".personas", Msg: "unknown persona: " + ref}
			}
		}
		for _, r := range s.Routing {
			if !pubIDs[r.Publisher] {
				return &core.ValidationError{Field: "sites." + s.ID + ".routing", Msg: "unknown publisher: " + r.Publisher}
			}
		}
	}

	envNames := make(map[string]bool, len(c.Environments))
	for _, e := range c.Environments {
		if e.Name == "" {
			return &core.ValidationError{Field: "environments", Msg: "environment missing name"}
		}
		if envNames[e.Name] {
			return &core.ValidationError{Field: "environments", Msg: "duplicate environment: " + e.Name}
		}
		envNames[e.Name] = true
		for _, ref := range e.Publish {
			if !pubIDs[ref] {
				return &core.ValidationError{Field: "environments." + e.Name + ".publish", Msg: "unknown publisher: " + ref}
			}
		}
		for ref := range e.Overrides {
			if !pubIDs[ref] {
				return &core.ValidationError{Field: "environments." + e.Name + ".overrides", Msg: "unknown publisher: " + ref}
			}
		}
	}
	return nil
}
