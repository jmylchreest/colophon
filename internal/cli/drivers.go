package cli

// Source and publisher drivers self-register via init(). Blank-importing them here
// registers every driver for the whole binary, so any command (build, serve, publish,
// next-build-time) can resolve them. Adding a driver = add its package + a line here.
import (
	_ "github.com/jmylchreest/colophon/internal/publish/cloudflare"
	_ "github.com/jmylchreest/colophon/internal/publish/git"
	_ "github.com/jmylchreest/colophon/internal/publish/local"
	_ "github.com/jmylchreest/colophon/internal/publish/r2"
	_ "github.com/jmylchreest/colophon/internal/publish/s3"
	_ "github.com/jmylchreest/colophon/internal/source/mddir"
	_ "github.com/jmylchreest/colophon/internal/source/obsidian"
)
