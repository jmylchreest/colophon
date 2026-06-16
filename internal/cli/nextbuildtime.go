package cli

import (
	"fmt"
	"time"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
)

// NextBuildTimeCmd prints the next pending publish_after timestamp — the soonest
// instant a production build would reveal a new post — so CI/scheduling can fire
// exactly then instead of polling. Empty when nothing is scheduled.
type NextBuildTimeCmd struct {
	JSON bool `help:"Output JSON"`
}

func (c *NextBuildTimeCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	next, err := build.NextEmbargo(cfg, time.Now())
	if err != nil {
		return err
	}

	if c.JSON {
		if next == nil {
			fmt.Println(`{"next_build_time":null}`)
		} else {
			fmt.Printf("{\"next_build_time\":%q}\n", next.Format(time.RFC3339))
		}
		return nil
	}
	if next == nil {
		fmt.Println("no scheduled posts pending")
		return nil
	}
	fmt.Println(next.Format(time.RFC3339))
	return nil
}
