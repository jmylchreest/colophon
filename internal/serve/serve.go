// Package serve runs a local preview server. It builds each environment of the first
// site into its own tree and hosts them all under /<site>/<env>/, with an index at the
// root. Page requests rebuild their environment first, a file watcher pushes a live-
// reload over SSE, and editing colophon.yaml reloads the config (rebuilding the target
// set and removing the serve trees of any environment that was deleted).
package serve

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
)

// rebuildTTL bounds how stale a served tree can be: an HTML request reuses the last build
// unless a watched file changed since (see markAllDirty) or the tree is older than this.
const rebuildTTL = 2 * time.Second

// reloadPath is the SSE endpoint; reloadScript (injected into pages) listens on it.
const reloadPath = "/_colophon/reload"

// reloadScript live-reloads the page on an SSE message, preserving scroll position
// across the reload (saved per-path in sessionStorage, restored once on the way back).
var reloadScript = []byte(`<script>(function(){` +
	`var K="__colophon_scroll:"+location.pathname;` +
	`try{if("scrollRestoration"in history)history.scrollRestoration="manual";` +
	`var y=sessionStorage.getItem(K);if(y!==null){sessionStorage.removeItem(K);window.scrollTo(0,parseInt(y,10)||0)}}catch(e){}` +
	`var s=new EventSource("` + reloadPath + `");` +
	`s.onmessage=function(){try{sessionStorage.setItem(K,String(window.pageYOffset))}catch(e){}location.reload()}` +
	`})();</script>`)

type target struct {
	name   string
	prefix string // "/main/preview/"
	drafts bool
	opts   build.Options
}

// buildState is one target's cached-build bookkeeping: its own lock so targets rebuild
// independently (a slow env doesn't block another), the time of the last build for the TTL
// check, and a dirty flag the watcher sets so the next request rebuilds promptly.
type buildState struct {
	mu        sync.Mutex  // serialises this target's rebuilds; guards lastBuilt
	lastBuilt time.Time   // when build() last completed for this target
	dirty     atomic.Bool // a watched file changed since lastBuilt → rebuild on next request
}

// Server hosts the preview of every environment for the first configured site.
type Server struct {
	root string
	addr string // set at ListenAndServe; used to derive each env's local base_url

	mu      sync.RWMutex // guards cfg, site, targets across config reloads
	cfg     *config.Config
	site    string
	targets []target

	buildsMu sync.Mutex             // guards the builds map
	builds   map[string]*buildState // per-target build state, keyed by target name

	clientMu sync.Mutex
	clients  map[chan struct{}]bool

	// shutdown is closed once when the server begins draining. SSE streams (handleReload)
	// select on it so they return promptly instead of blocking Shutdown until its deadline —
	// http.Server.Shutdown waits for in-flight handlers to return but never cancels their
	// request context, so a long-lived stream would otherwise hold shutdown open.
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// New builds a Server from the loaded config.
func New(cfg *config.Config) (*Server, error) {
	site, targets, err := targetsFor(cfg)
	if err != nil {
		return nil, err
	}
	return &Server{
		root:     cfg.Root,
		cfg:      cfg,
		site:     site,
		targets:  targets,
		builds:   map[string]*buildState{},
		clients:  map[chan struct{}]bool{},
		shutdown: make(chan struct{}),
	}, nil
}

// targetsFor derives the serve targets from a config. With no environments configured
// it yields a single draft-including "preview" so the command always works.
func targetsFor(cfg *config.Config) (string, []target, error) {
	if len(cfg.Sites) == 0 {
		return "", nil, fmt.Errorf("no sites configured")
	}
	site := cfg.Sites[0]
	envs := cfg.Environments
	if len(envs) == 0 {
		envs = []config.Environment{{Name: "preview", IncludeDrafts: true}}
	}
	var targets []target
	for _, e := range envs {
		prefix := "/" + site.ID + "/" + e.Name + "/"
		targets = append(targets, target{
			name:   e.Name,
			prefix: prefix,
			drafts: e.IncludeDrafts,
			opts: build.Options{
				OutDir:        filepath.Join(cfg.Root, ".colophon", "serve", e.Name),
				IncludeDrafts: e.IncludeDrafts,
				Title:         e.Title,
				BaseURL:       e.BaseURL,
				Theme:         e.Theme,
				BasePath:      prefix,
			},
		})
	}
	return site.ID, targets, nil
}

// ListenAndServe builds every environment once, starts the file watcher, then serves. When
// openTarget is non-empty it opens that well-known location (latest|home|sitemap|feeds|…) of
// the first environment in the browser once the server is up.
func (s *Server) ListenAndServe(addr, openTarget string) error {
	s.addr = addr // set before any build so each env gets its local base_url
	s.mu.RLock()
	targets := s.targets
	s.mu.RUnlock()
	for _, t := range targets {
		if err := s.forceBuild(t); err != nil {
			return fmt.Errorf("build %s: %w", t.name, err)
		}
	}

	// One context drives shutdown: Ctrl-C / SIGTERM cancels it, which stops the watcher,
	// the pending browser-open, and the HTTP server (so an in-flight build isn't killed
	// mid-write).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	watchDone := make(chan struct{})
	go func() { defer close(watchDone); s.watch(ctx) }()

	fmt.Printf("colophon serve → http://localhost%s/\n", port(addr))
	// One aligned key=value line per site/env, so a person or an agent can copy any URL directly.
	site := s.site
	siteW, envW := len(site), 0
	for _, t := range targets {
		if len(t.name) > envW {
			envW = len(t.name)
		}
	}
	slug, hasLatest := s.latestSlug()
	for _, t := range targets {
		base := "http://localhost" + port(addr) + t.prefix
		var kv strings.Builder
		fmt.Fprintf(&kv, "url=%s", base)
		if hasLatest {
			fmt.Fprintf(&kv, " latest=%s%s/", base, slug)
		}
		fmt.Fprintf(&kv, " sitemap=%ssitemap.xml atom=%satom.xml rss=%srss.xml json=%sfeed.json", base, base, base, base)
		if t.drafts {
			kv.WriteString(" draft=true")
		}
		fmt.Printf("  %-*s  %-*s  %s\n", siteW, site, envW, t.name, kv.String())
	}
	if len(targets) > 0 && openTarget != "" {
		if u, ok := s.resolveURL(addr, targets[0], openTarget); ok {
			fmt.Printf("opening %s\n", u)
			go openBrowserAfter(ctx, 300*time.Millisecond, u)
		} else {
			fmt.Printf("  (unknown --open target %q)\n", openTarget)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc(reloadPath, s.handleReload)
	mux.HandleFunc("/", s.route)
	srv := &http.Server{Addr: addr, Handler: mux}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case err := <-errCh:
		// ListenAndServe failed before any signal (e.g. the port is taken). Cancel ctx so the
		// watcher goroutine unwinds and closes its fsnotify handle before we return.
		stop()
		<-watchDone
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		stop() // restore default signal handling: a second Ctrl-C aborts the drain
		fmt.Fprintln(os.Stderr, "\ncolophon: shutting down…")
		s.beginShutdown() // release live-reload streams so they don't hold Shutdown to its deadline
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := srv.Shutdown(shutCtx)
		<-watchDone // the watcher saw ctx cancel; wait for it to close the fsnotify watcher
		return err
	}
}

// resolveURL maps an --open target name to a full URL under the given environment.
func (s *Server) resolveURL(addr string, t target, openName string) (string, bool) {
	base := "http://localhost" + port(addr) + t.prefix
	var path string
	switch openName {
	case "home":
		path = ""
	case "latest":
		slug, ok := s.latestSlug()
		if !ok {
			return "", false
		}
		path = slug + "/"
	case "sitemap":
		path = "sitemap.xml"
	case "atom":
		path = "atom.xml"
	case "rss":
		path = "rss.xml"
	case "json":
		path = "feed.json"
	case "robots":
		path = "robots.txt"
	default:
		path = strings.Trim(openName, "/")
		if path != "" && !strings.Contains(path, ".") {
			path += "/" // looks like a slug, not a file
		}
	}
	return base + path, true
}

// latestSlug is the slug of the newest dated post, for the `latest` target.
func (s *Server) latestSlug() (string, bool) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	entries, err := build.Entries(cfg)
	if err != nil {
		return "", false
	}
	best := ""
	var bestDate time.Time
	for _, e := range entries {
		if e.Date.IsZero() {
			continue
		}
		if best == "" || e.Date.After(bestDate) {
			best, bestDate = e.Slug, e.Date
		}
	}
	return best, best != ""
}

// openBrowserAfter opens url once delay elapses, unless ctx is cancelled first (the server
// is shutting down). The delay gives the listener a moment to come up; tying it to ctx keeps
// the open inside the server's lifecycle instead of a detached sleep.
func openBrowserAfter(ctx context.Context, delay time.Duration, url string) {
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
		openBrowser(url)
	}
}

// openBrowser best-effort launches the OS browser at url; failure is ignored (the URL was
// already printed).
func openBrowser(url string) {
	name := "xdg-open"
	args := make([]string, 0, 2)
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name = "rundll32"
		args = append(args, "url.dll,FileProtocolHandler")
	}
	args = append(args, url)
	_ = exec.Command(name, args...).Start()
}

// buildStateFor returns the per-target build state, creating it on first use.
func (s *Server) buildStateFor(name string) *buildState {
	s.buildsMu.Lock()
	defer s.buildsMu.Unlock()
	bs := s.builds[name]
	if bs == nil {
		bs = &buildState{}
		s.builds[name] = bs
	}
	return bs
}

// build runs the actual build for t. The caller must hold the target's buildState.mu so two
// requests for the same env don't race on its output tree.
func (s *Server) build(t target) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	// In a preview the canonical URL is the local serve URL, so override base_url to it.
	// This keeps feeds, sitemap, robots.txt, autodiscovery and on-page links all
	// resolvable within the preview, instead of pointing at the configured base_url.
	opts := t.opts
	opts.BaseURL = "http://localhost" + port(s.addr) + strings.TrimSuffix(t.prefix, "/")
	_, err := build.Run(cfg, opts)
	return err
}

// forceBuild rebuilds t unconditionally (startup and config reload) and records the time.
func (s *Server) forceBuild(t target) error {
	bs := s.buildStateFor(t.name)
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if err := s.build(t); err != nil {
		return err
	}
	bs.lastBuilt = time.Now()
	bs.dirty.Store(false)
	return nil
}

// ensureBuilt rebuilds t only when needed: a watched file changed since the last build, the
// cached tree is older than rebuildTTL, or it was never built. Otherwise it reuses the tree
// on disk — so browsing N pages no longer recompiles the whole site N times.
func (s *Server) ensureBuilt(t target) error {
	bs := s.buildStateFor(t.name)
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if !bs.lastBuilt.IsZero() && !bs.dirty.Load() && time.Since(bs.lastBuilt) < rebuildTTL {
		return nil
	}
	if err := s.build(t); err != nil {
		return err
	}
	bs.lastBuilt = time.Now()
	bs.dirty.Store(false)
	return nil
}

// markAllDirty flags every known target so its next request rebuilds (a watched file changed).
func (s *Server) markAllDirty() {
	s.buildsMu.Lock()
	defer s.buildsMu.Unlock()
	for _, bs := range s.builds {
		bs.dirty.Store(true)
	}
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.writeIndex(w)
		return
	}
	s.mu.RLock()
	targets := s.targets
	s.mu.RUnlock()
	for _, t := range targets {
		base := strings.TrimSuffix(t.prefix, "/")
		if r.URL.Path == base {
			http.Redirect(w, r, t.prefix, http.StatusMovedPermanently)
			return
		}
		if !strings.HasPrefix(r.URL.Path, t.prefix) {
			continue
		}
		if isPage(r.URL.Path) {
			if err := s.ensureBuilt(t); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.servePage(w, r, t)
			return
		}
		http.StripPrefix(base, http.FileServer(http.Dir(t.opts.OutDir))).ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

// servePage serves an HTML document with the live-reload script injected.
func (s *Server) servePage(w http.ResponseWriter, r *http.Request, t target) {
	rel := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, t.prefix), "/")
	fp := filepath.Join(t.opts.OutDir, filepath.FromSlash(rel))
	if rel == "" || strings.HasSuffix(r.URL.Path, "/") {
		fp = filepath.Join(fp, "index.html")
	}
	b, err := os.ReadFile(fp)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(injectReload(b))
}

func (s *Server) writeIndex(w http.ResponseWriter) {
	s.mu.RLock()
	site := s.site
	targets := s.targets
	s.mu.RUnlock()

	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><title>colophon serve</title>")
	b.WriteString(`<style>body{font:16px/1.6 system-ui,sans-serif;max-width:40rem;margin:3rem auto;padding:0 1rem}a{color:#0a58ca}li{margin:.4rem 0}.d{color:#888;font-size:.85em}</style>`)
	fmt.Fprintf(&b, "<h1>colophon · local preview</h1><p>site: <strong>%s</strong></p><ul>", html.EscapeString(site))
	for _, t := range targets {
		drafts := ""
		if t.drafts {
			drafts = ` <span class="d">drafts</span>`
		}
		fmt.Fprintf(&b, `<li><a href="%s">%s</a>%s</li>`, t.prefix, html.EscapeString(t.name), drafts)
	}
	b.WriteString("</ul>")
	b.Write(reloadScript)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// handleReload is the SSE stream: it sends "reload" whenever a watched file changes.
func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan struct{}, 1)
	s.addClient(ch)
	defer s.removeClient(ch)

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.shutdown: // server draining: release the stream so Shutdown doesn't block on it
			return
		case <-ch:
			_, _ = fmt.Fprint(w, "data: reload\n\n")
			flusher.Flush()
		}
	}
}

// watch debounces filesystem changes and reacts: a content/theme edit broadcasts a
// reload; a colophon.yaml edit reloads the config first. A failure to start the watcher
// degrades to rebuild-on-refresh, not an error.
func (s *Server) watch(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "colophon: live reload disabled: %v\n", err)
		return
	}
	defer func() { _ = w.Close() }()
	for _, d := range []string{"content", "themes"} {
		addTree(w, filepath.Join(s.root, d))
	}
	configPath := filepath.Join(s.root, config.ConfigFile)
	_ = w.Add(configPath)

	debounce := time.NewTimer(time.Hour)
	debounce.Stop()
	configDirty := false
	for {
		select {
		case <-ctx.Done(): // server shutting down: closing w ends this loop
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			switch {
			case filepath.Clean(ev.Name) == configPath:
				configDirty = true
				_ = w.Add(configPath) // re-add: editors often rename-replace on save
			case ev.Op&fsnotify.Create != 0:
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					addTree(w, ev.Name)
				}
			}
			debounce.Reset(150 * time.Millisecond)
		case <-debounce.C:
			if configDirty {
				configDirty = false
				s.reconfigure()
			} else {
				s.markAllDirty() // the next page request rebuilds; until then serve the cache
				s.broadcast()
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}

// reconfigure reloads colophon.yaml, swaps in the new target set, removes the serve
// trees of environments that no longer exist, rebuilds, and triggers a reload. A bad
// config is logged and ignored so a typo mid-edit doesn't take the server down.
func (s *Server) reconfigure() {
	cfg, err := config.Load(s.root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "colophon: config reload failed, keeping previous: %v\n", err)
		return
	}
	site, targets, err := targetsFor(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "colophon: config reload failed, keeping previous: %v\n", err)
		return
	}

	keep := make(map[string]bool, len(targets))
	for _, t := range targets {
		keep[t.name] = true
	}
	_ = build.ReconcileDirs(filepath.Join(s.root, ".colophon", "serve"), keep)

	s.mu.Lock()
	s.cfg, s.site, s.targets = cfg, site, targets
	s.mu.Unlock()

	for _, t := range targets {
		if err := s.forceBuild(t); err != nil {
			fmt.Fprintf(os.Stderr, "colophon: rebuild %s failed: %v\n", t.name, err)
		}
	}
	fmt.Fprintln(os.Stderr, "colophon: config reloaded")
	s.broadcast()
}

// beginShutdown signals every live-reload stream to return. Closing the channel once is safe
// even if no streams are open and even if called more than once.
func (s *Server) beginShutdown() {
	s.shutdownOnce.Do(func() { close(s.shutdown) })
}

func (s *Server) addClient(ch chan struct{}) {
	s.clientMu.Lock()
	s.clients[ch] = true
	s.clientMu.Unlock()
}

func (s *Server) removeClient(ch chan struct{}) {
	s.clientMu.Lock()
	delete(s.clients, ch)
	s.clientMu.Unlock()
}

func (s *Server) broadcast() {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for ch := range s.clients {
		select {
		case ch <- struct{}{}:
		default: // a reload is already pending for this client
		}
	}
}

// addTree watches dir and its subdirectories (fsnotify is non-recursive, so each dir is added
// individually). Hidden directories (.git, .obsidian, …) are pruned: they hold no rendered
// content and can be huge, so watching them would burn the inotify watch budget for nothing.
func addTree(w *fsnotify.Watcher, dir string) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if name := d.Name(); name != "." && strings.HasPrefix(name, ".") && p != dir {
			return filepath.SkipDir
		}
		_ = w.Add(p)
		return nil
	})
}

func injectReload(b []byte) []byte {
	if i := bytes.LastIndex(b, []byte("</body>")); i >= 0 {
		out := make([]byte, 0, len(b)+len(reloadScript))
		out = append(out, b[:i]...)
		out = append(out, reloadScript...)
		out = append(out, b[i:]...)
		return out
	}
	return append(b, reloadScript...)
}

// isPage reports whether a request should rebuild + get the reload script: directory
// roots and HTML documents do; static assets (css, images) are served as-is.
func isPage(p string) bool {
	return strings.HasSuffix(p, "/") || strings.HasSuffix(p, ".html")
}

// port returns the ":port" suffix of a listen address for building localhost URLs. It accepts
// host:port (":8080", "localhost:8080", "[::1]:8080") or a bare port ("8080"); anything that
// doesn't yield a numeric port falls back to the default so the printed URLs stay valid.
func port(addr string) string {
	if _, p, err := net.SplitHostPort(addr); err == nil {
		if _, err := strconv.Atoi(p); err == nil {
			return ":" + p
		}
	}
	if p := strings.TrimPrefix(addr, ":"); p != "" {
		if _, err := strconv.Atoi(p); err == nil {
			return ":" + p
		}
	}
	return ":8080"
}
