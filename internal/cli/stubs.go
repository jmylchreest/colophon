package cli

// Commands planned for later milestones. They parse and report their target
// milestone so the CLI surface is complete and discoverable today.

type SyncCmd struct {
	Source string `arg:"" optional:"" help:"Source to pull (notion|hackmd)"`
}

func (c *SyncCmd) Run() error { return notImplemented("M6") }
