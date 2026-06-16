package cli

// Commands planned for later milestones. They parse and report their target
// milestone so the CLI surface is complete and discoverable today.

type PersonaCmd struct {
	List    PersonaListCmd    `cmd:"" help:"List personas"`
	Context PersonaContextCmd `cmd:"" help:"Emit style guide + top-K exemplars for AI-assisted writing"`
}

type PersonaListCmd struct {
	JSON bool `help:"Output JSON"`
}

func (c *PersonaListCmd) Run() error { return notImplemented("M2") }

type PersonaContextCmd struct {
	Persona string `arg:"" optional:"" help:"Persona id"`
	Topic   string `help:"Topic/outline to retrieve exemplars for"`
}

func (c *PersonaContextCmd) Run() error { return notImplemented("M2") }

type SearchCmd struct {
	Query string `arg:"" optional:"" help:"Search query"`
	JSON  bool   `help:"Output JSON"`
}

func (c *SearchCmd) Run() error { return notImplemented("M4") }

type SyncCmd struct {
	Source string `arg:"" optional:"" help:"Source to pull (notion|hackmd)"`
}

func (c *SyncCmd) Run() error { return notImplemented("M6") }
