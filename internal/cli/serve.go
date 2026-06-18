package cli

import (
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/profiling"
	"github.com/jmylchreest/colophon/internal/serve"
)

// ServeCmd builds every environment of the first site and serves them locally under
// /<site>/<env>/, with an index at the root. Includes drafts where the environment does.
type ServeCmd struct {
	Addr    string `help:"Address to listen on" default:":8080"`
	Open    string `help:"Open a target in the browser: latest | home | sitemap | atom | rss | json | robots | <slug>"`
	Verbose bool   `short:"v" help:"Log each rebuild and attach source locations"`
	Pprof   string `help:"Enable net/http/pprof (addr, or 1 for localhost:6060)" hidden:""`
}

func (c *ServeCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	srv, err := serve.New(cfg, newLogger(c.Verbose))
	if err != nil {
		return err
	}
	defer profiling.Serve(c.Pprof)()
	return srv.ListenAndServe(c.Addr, c.Open)
}
