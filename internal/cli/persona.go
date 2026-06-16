package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/persona"
)

// PersonaCmd groups persona discovery and the AI write-as context command.
type PersonaCmd struct {
	List    PersonaListCmd    `cmd:"" help:"List personas"`
	Context PersonaContextCmd `cmd:"" help:"Emit style guide + top-K exemplars for AI-assisted writing"`
}

// PersonaListCmd enumerates the configured personas.
type PersonaListCmd struct {
	JSON bool `help:"Output JSON"`
}

func (c *PersonaListCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if c.JSON {
		return writeJSON(cfg.Personas)
	}
	if len(cfg.Personas) == 0 {
		fmt.Println("No personas. Add personas/<id>.yaml.")
		return nil
	}
	for _, p := range cfg.Personas {
		styled := ""
		if p.Style.Guide != "" || len(p.Style.References) > 0 {
			styled = " (styled)"
		}
		fmt.Printf("%-16s %s%s\n", p.ID, p.Name, styled)
	}
	return nil
}

// PersonaContextCmd emits write-as context for a persona: its style guide and references
// plus the top-K most relevant exemplars from its own content. The calling agent is the
// intelligence; colophon only supplies the context.
type PersonaContextCmd struct {
	Persona string   `arg:"" optional:"" help:"Persona id (defaults to the only persona, or 'default')"`
	Topic   string   `help:"Topic/outline to retrieve exemplars for (ranked by relevance)"`
	Tag     []string `help:"Only draw exemplars tagged with this tag; repeatable"`
	TopK    int      `name:"top-k" default:"3" help:"Number of exemplars to emit"`
	JSON    bool     `help:"Output JSON"`
}

func (c *PersonaContextCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	id := c.Persona
	if id == "" {
		switch {
		case len(cfg.Personas) == 1:
			id = cfg.Personas[0].ID
		default:
			id = "default"
		}
	}
	ctx, err := persona.BuildContext(cfg, id, c.Topic, c.TopK, c.Tag...)
	if err != nil {
		return err
	}
	if c.JSON {
		return writeJSON(ctx)
	}
	printContext(ctx)
	return nil
}

func printContext(ctx *persona.Context) {
	p := ctx.Persona
	label := p.Name
	if label == "" {
		label = p.ID
	}
	fmt.Printf("# Write in the %q voice (%s)\n", label, p.ID)
	fmt.Println()
	fmt.Println("## Style guide")
	if ctx.Guide != "" {
		fmt.Println(ctx.Guide)
	} else {
		fmt.Println("(none set — match the voice of the exemplars below)")
	}
	if len(ctx.References) > 0 {
		fmt.Println("\n## References")
		for _, r := range ctx.References {
			fmt.Printf("- %s\n", r)
		}
	}
	heading := "## Exemplars (most recent)"
	if ctx.Topic != "" {
		heading = fmt.Sprintf("## Exemplars relevant to %q", ctx.Topic)
	}
	fmt.Printf("\n%s\n", heading)
	if len(ctx.Exemplars) == 0 {
		fmt.Println("(no published content yet for this persona)")
		return
	}
	for _, e := range ctx.Exemplars {
		title := e.Title
		if title == "" {
			title = e.Path
		}
		when := ""
		if !e.Date.IsZero() {
			when = " · " + e.Date.Format("2006-01-02")
		}
		fmt.Printf("\n### %s%s\n%s\n", title, when, e.Excerpt)
	}
}

// loadConfig finds the project root and loads its config.
func loadConfig() (*config.Config, error) {
	root, err := findRoot()
	if err != nil {
		return nil, err
	}
	return config.Load(root)
}

// writeJSON prints v as indented JSON to stdout.
func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
