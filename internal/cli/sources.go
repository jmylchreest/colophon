package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
)

// SourcesCmd reports where content lives and how a post is marked publishable in each source,
// so an agent (or a person) knows where to write a new post and how to make it appear.
type SourcesCmd struct {
	JSON bool `help:"Output JSON"`
}

type sourceInfo struct {
	ID      string `json:"id"`
	Driver  string `json:"driver"`
	Path    string `json:"path"`    // resolved writable location
	Publish string `json:"publish"` // how a note is marked publishable here
}

func (c *SourcesCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	infos := describeSources(cfg)
	if c.JSON {
		return writeJSON(infos)
	}
	for _, s := range infos {
		fmt.Printf("%-12s %-9s %-30s %s\n", s.ID, s.Driver, s.Path, s.Publish)
	}
	return nil
}

func describeSources(cfg *config.Config) []sourceInfo {
	srcs := cfg.Sources
	if len(srcs) == 0 {
		srcs = []config.SourceConfig{{ID: "content", Driver: "md-dir"}}
	}
	out := make([]sourceInfo, 0, len(srcs))
	for _, s := range srcs {
		get := func(k string) string { return setting(s.Settings, k) }
		info := sourceInfo{ID: s.ID, Driver: s.Driver}
		switch s.Driver {
		case "md-dir":
			p := get("path")
			if p == "" {
				p = "content"
			}
			info.Path = resolveUnder(cfg.Root, p)
			info.Publish = "all .md files"
		case "obsidian":
			info.Path = resolveUnder(cfg.Root, get("vault"))
			path, tag := get("path"), get("tag")
			switch {
			case tag != "":
				info.Publish = "tag #" + tag
				if path != "" {
					info.Path += " (in " + path + ")"
				}
			case path != "":
				info.Publish = "publish: true (in " + path + ")"
			default:
				info.Publish = "publish: true"
			}
		default:
			info.Path = get("path")
			info.Publish = "(driver-specific)"
		}
		out = append(out, info)
	}
	return out
}

// resolveUnder expands ~ and resolves p relative to root; empty stays empty.
func resolveUnder(root, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p
}
