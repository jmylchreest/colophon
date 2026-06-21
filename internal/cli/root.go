// Package cli defines colophon's command tree using kong. Commands stay thin: their
// struct fields are flags/args and their Run methods delegate to the core library
// and other internal packages.
package cli

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

// version is overridden at build time via -ldflags "-X ...cli.version=...".
var version = "dev"

// CLI is the root command tree.
type CLI struct {
	Init          InitCmd          `cmd:"" help:"Scaffold a new colophon project"`
	New           NewCmd           `cmd:"" help:"Scaffold a new post or page (validated author/persona, unique slug)"`
	Build         BuildCmd         `cmd:"" help:"Build the site into public/ (prints next pending embargo)"`
	NextBuildTime NextBuildTimeCmd `cmd:"" help:"Print the next pending publish_after timestamp (for CI scheduling)"`
	Serve         ServeCmd         `cmd:"" help:"Serve every environment locally with live reload"`
	Publish       PublishCmd       `cmd:"" help:"Build and deploy/mirror to publishers (gated)"`
	Themes        ThemesCmd        `cmd:"" help:"List built-in themes or eject one to customise"`
	Authors       AuthorsCmd       `cmd:"" aliases:"author" help:"List authors (the bylines) or show one"`
	Persona       PersonaCmd       `cmd:"" aliases:"personas" help:"List writing voices or emit write-as context"`
	Sources       SourcesCmd       `cmd:"" aliases:"source" help:"Show where content lives and how posts are marked publishable"`
	Posts         PostsCmd         `cmd:"" aliases:"post" help:"List content entries (for editing and cross-referencing)"`
	Search        SearchCmd        `cmd:"" help:"Search content (lexical or semantic)"`
	Sync          SyncCmd          `cmd:"" hidden:"" help:"Pull API sources (notion/hackmd) into content/ (planned)"`
	Doctor        DoctorCmd        `cmd:"" help:"Validate the project config and report problems"`
	Env           EnvCmd           `cmd:"" help:"List the environment variables this project uses"`

	Version kong.VersionFlag `help:"Print version and exit"`
}

// Execute parses arguments, runs the selected command, and returns an exit code.
func Execute() int {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("colophon"),
		kong.Description("A themed Markdown blog generator with pluggable publishers"),
		kong.UsageOnError(),
		kong.Vars{"version": version},
	)
	if err := ctx.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "colophon:", err)
		return 1
	}
	return 0
}
