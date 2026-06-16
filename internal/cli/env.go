package cli

import (
	"fmt"
	"sort"

	"github.com/jmylchreest/colophon/internal/publish"
)

// EnvCmd lists every environment variable the project depends on: the {env:VAR}
// placeholders the config interpolates, plus the deploy secrets read by its configured
// publishers. Tooling (e.g. fixtures/mixed/gen-dotenv.sh) uses this instead of a hard-coded
// list, so adding a publisher or a new {env:VAR} is picked up automatically.
type EnvCmd struct{}

func (c *EnvCmd) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	set := map[string]struct{}{}
	for _, v := range cfg.EnvRefs {
		set[v] = struct{}{}
	}
	drivers := make([]string, 0, len(cfg.Publishers))
	for _, p := range cfg.Publishers {
		drivers = append(drivers, p.Driver)
	}
	for _, v := range publish.DriverEnvVars(drivers) {
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	for _, v := range out {
		fmt.Println(v)
	}
	return nil
}
