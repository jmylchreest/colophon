// Package profiling is colophon's hidden, opt-in pprof hook. It is deliberately isolated from
// the rest of the codebase: commands wire it in with a single call (Capture for one-shot runs
// like build/publish, Serve for the long-running serve command) and everything else lives here.
//
// It is off unless the --pprof flag or COLOPHON_PPROF environment variable is set, so a normal
// run carries no profiling cost and the feature stays out of the way.
//
//	COLOPHON_PPROF=1                 # build/publish: write profiles to the cwd
//	COLOPHON_PPROF=/tmp/prof         # build/publish: write profiles under /tmp/prof
//	COLOPHON_PPROF=1 colophon serve  # serve: net/http/pprof on localhost:6060
//	COLOPHON_PPROF=:7000 colophon serve
package profiling

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"
	rpprof "runtime/pprof"
	"strings"
	"time"
)

// defaultServeAddr is the listen address used when serve profiling is enabled without an
// explicit address (a bare truthy token like "1").
const defaultServeAddr = "localhost:6060"

// resolve returns the active profiling spec: the flag value if set, else COLOPHON_PPROF.
func resolve(spec string) string {
	if spec != "" {
		return spec
	}
	return os.Getenv("COLOPHON_PPROF")
}

// truthy reports whether v is a bare on switch (rather than a path/address argument).
func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// Capture starts CPU profiling when profiling is enabled and returns a stop function that
// stops it and writes the CPU and heap profiles. When disabled it returns a no-op stop, so
// callers can unconditionally `defer stop()`. spec is the command's --pprof flag; an empty
// flag falls back to COLOPHON_PPROF. The spec value is the output directory ("." for a bare
// truthy token). A setup failure is reported but never fatal — profiling must not break a run.
func Capture(spec string) (stop func()) {
	v := resolve(spec)
	if v == "" {
		return func() {}
	}
	dir := "."
	if !truthy(v) {
		dir = v
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "colophon: pprof disabled: %v\n", err)
		return func() {}
	}
	cpuPath := filepath.Join(dir, "colophon-cpu.pprof")
	f, err := os.Create(cpuPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "colophon: pprof disabled: %v\n", err)
		return func() {}
	}
	if err := rpprof.StartCPUProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "colophon: pprof disabled: %v\n", err)
		_ = f.Close()
		return func() {}
	}
	fmt.Fprintf(os.Stderr, "colophon: pprof capturing CPU → %s\n", cpuPath)

	return func() {
		rpprof.StopCPUProfile()
		_ = f.Close()
		writeHeap(filepath.Join(dir, "colophon-heap.pprof"))
		fmt.Fprintf(os.Stderr, "colophon: pprof wrote %s and colophon-heap.pprof (go tool pprof %s)\n", cpuPath, cpuPath)
	}
}

// writeHeap snapshots the heap profile after a GC so freed objects don't inflate it.
func writeHeap(path string) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "colophon: pprof heap: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()
	runtime.GC()
	if err := rpprof.WriteHeapProfile(f); err != nil {
		fmt.Fprintf(os.Stderr, "colophon: pprof heap: %v\n", err)
	}
}

// Serve starts net/http/pprof on a background listener when profiling is enabled and returns a
// shutdown function (a no-op when disabled). spec is the command's --pprof flag; an empty flag
// falls back to COLOPHON_PPROF. The spec value is the listen address (defaultServeAddr for a
// bare truthy token). Endpoints live under http://<addr>/debug/pprof/. Handlers are registered
// on a private mux, never the global DefaultServeMux.
func Serve(spec string) (shutdown func()) {
	v := resolve(spec)
	if v == "" {
		return func() {}
	}
	addr := defaultServeAddr
	if !truthy(v) {
		addr = v
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "colophon: pprof disabled: %v\n", err)
		return func() {}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Handler: mux}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "colophon: pprof listener: %v\n", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "colophon: pprof at http://%s/debug/pprof/ (go tool pprof http://%s/debug/pprof/profile)\n", ln.Addr(), ln.Addr())

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}
