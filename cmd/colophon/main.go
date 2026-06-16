package main

import (
	"os"

	"github.com/jmylchreest/colophon/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
