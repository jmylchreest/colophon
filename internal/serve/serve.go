// Package serve runs a local preview server. It builds each environment of the first
// site into its own tree and hosts them all under /<site>/<env>/, with an index at the
// root. Page requests rebuild their environment first, a file watcher pushes a live-
// reload over SSE, and editing colophon.yaml reloads the config (rebuilding the target
// set and removing the serve trees of any environment that was deleted).
package serve

import (
	"bytes"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
)

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

// Server hosts the preview of every environment for the first configured site.
type Server struct {
	root string
	addr string // set at ListenAndServe; used to derive each env's local base_url

	mu      sync.RWMutex // guards cfg, site, targets across config reloads
	cfg     *config.Config
	site    string
	targets []target

	buildMu sync.Mutex // serialises rebuilds so concurrent requests don't race on a tree

	clientMu sync.Mutex
	clients  map[chan struct{}]bool
}

// New builds a Server from the loaded config.
func New(cfg *config.Config) (*Server, error) {
	site, targets, err := targetsFor(cfg)
	if err != nil {
		return nil, err
	}
	return &Server{
		root:    cfg.Root,
		cfg:     cfg,
		site:    site,
		targets: targets,
		clients: map[chan struct{}]bool{},
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
		if err := s.rebuild(t); err != nil {
			return fmt.Errorf("build %s: %w", t.name, err)
		}
	}
	go s.watch()

	fmt.Printf("colophon serve → http://localhost%s/\n", port(addr))
	for _, t := range targets {
		drafts := ""
		if t.drafts {
			drafts = "  (drafts)"
		}
		fmt.Printf("  http://localhost%s%s%s\n", port(addr), t.prefix, drafts)
	}
	if len(targets) > 0 {
		s.printWellKnown(addr, targets[0])
		if openTarget != "" {
			if u, ok := s.resolveURL(addr, targets[0], openTarget); ok {
				fmt.Printf("opening %s\n", u)
				go func() { time.Sleep(300 * time.Millisecond); openBrowser(u) }()
			} else {
				fmt.Printf("  (unknown --open target %q)\n", openTarget)
			}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc(reloadPath, s.handleReload)
	mux.HandleFunc("/", s.route)
	return http.ListenAndServe(addr, mux)
}

// printWellKnown lists the home/latest/sitemap/feed URLs for a target, so a person or an
// agent (which can't open a browser) can copy them straight from serve's output.
func (s *Server) printWellKnown(addr string, t target) {
	base := "http://localhost" + port(addr) + t.prefix
	fmt.Printf("  home     %s\n", base)
	if slug, ok := s.latestSlug(); ok {
		fmt.Printf("  latest   %s%s/\n", base, slug)
	}
	fmt.Printf("  sitemap  %ssitemap.xml\n", base)
	fmt.Printf("  feeds    atom %satom.xml · rss %srss.xml · json %sfeed.json\n", base, base, base)
}

// resolveURL maps an --open target name to a full URL under the given environment.
func (s *Server) resolveURL(addr string, t target, target_ string) (string, bool) {
	base := "http://localhost" + port(addr) + t.prefix
	var path string
	switch target_ {
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
		path = strings.Trim(target_, "/")
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

// openBrowser best-effort launches the OS browser at url; failure is ignored (the URL was
// already printed).
func openBrowser(url string) {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		name = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(name, args...).Start()
}

func (s *Server) rebuild(t target) error {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()
	// In a preview the canonical URL is the local serve URL, so override base_url to it.
	// This keeps feeds, sitemap, robots.txt, autodiscovery and on-page links all
	// resolvable within the preview, instead of pointing at the configured base_url.
	opts := t.opts
	opts.BaseURL = "http://localhost" + port(s.addr) + strings.TrimSuffix(t.prefix, "/")
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	_, err := build.Run(cfg, opts)
	return err
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
			if err := s.rebuild(t); err != nil {
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
		case <-ch:
			_, _ = fmt.Fprint(w, "data: reload\n\n")
			flusher.Flush()
		}
	}
}

// watch debounces filesystem changes and reacts: a content/theme edit broadcasts a
// reload; a colophon.yaml edit reloads the config first. A failure to start the watcher
// degrades to rebuild-on-refresh, not an error.
func (s *Server) watch() {
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
		if err := s.rebuild(t); err != nil {
			fmt.Fprintf(os.Stderr, "colophon: rebuild %s failed: %v\n", t.name, err)
		}
	}
	fmt.Fprintln(os.Stderr, "colophon: config reloaded")
	s.broadcast()
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

func addTree(w *fsnotify.Watcher, dir string) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			_ = w.Add(p)
		}
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

func port(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return addr
}
