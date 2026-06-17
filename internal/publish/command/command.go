// Package command implements the "command" publisher: it materialises the built tree to a
// directory and runs a user-configured CLI command against it, so any deploy tool that takes a
// directory — surge, netlify, vercel, wrangler, rsync, scp, aws s3 sync, a bespoke script — can
// be a colophon publisher without a driver of its own.
//
// The command is an argv list (executed directly, never through a shell, so there is no shell
// injection surface — for pipes/&& the user writes an explicit `sh -c`). Every argument is
// interpolated with {placeholders}: the materialised directory ({dir}), the public URL, the
// publisher id, and every setting the user declares on the publisher — so the command can be
// shaped to whatever the target tool expects.
//
// Secrets never pass through colophon: the child inherits the environment, so a tool reads its
// own token var ($SURGE_TOKEN, $VERCEL_TOKEN, …) directly. This matches colophon's env-only
// secrets rule and the deploy-CLI best practice of keeping tokens out of argv (where they would
// leak into process listings and shell history).
package command

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

func init() { publish.Register("command", New) }

// New builds a command publisher. Required: command (a non-empty argv list).
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	argv, err := toArgv(cfg.Settings["command"])
	if err != nil {
		return nil, fmt.Errorf("command publisher %q: %w", cfg.ID, err)
	}
	get := func(k string) string { s, _ := cfg.Settings[k].(string); return strings.TrimSpace(s) }
	return &publisher{
		id:        cfg.ID,
		argv:      argv,
		publicURL: strings.TrimRight(get("public_url"), "/"),
		settings:  cfg.Settings,
	}, nil
}

type publisher struct {
	id        string
	argv      []string
	publicURL string
	settings  map[string]any
	log       *clog.Logger
}

func (p *publisher) SetLogger(l *clog.Logger) { p.log = l }
func (p *publisher) ID() string               { return p.id }
func (p *publisher) Driver() string           { return "command" }

// --- core.Publisher methods that don't apply to an external deploy command ---

var errUsePush = errors.New("command: this driver deploys via TreePublisher.Push, not Publisher.Commit")

func (p *publisher) Hash(string, []byte) string { return "" }
func (p *publisher) Protected(string) bool      { return false }
func (p *publisher) Deployed(context.Context) (core.State, bool, error) {
	return nil, false, nil // an external command owns its own state; colophon can't enumerate it
}
func (p *publisher) Commit(context.Context, fs.FS, *core.Plan) (core.Result, error) {
	return core.Result{}, errUsePush
}
func (p *publisher) Invalidate(context.Context, []string) error { return nil }

// CanonicalURL is the configured public_url (or ""): colophon can't learn it from an opaque
// command, so set it for routing / canonical links.
func (p *publisher) CanonicalURL(context.Context) (string, error) { return p.publicURL, nil }

// Push materialises the tree to a temp directory, writes a classification manifest beside it,
// then runs the interpolated command with that directory as its working dir. Implements
// core.TreePublisher.
func (p *publisher) Push(ctx context.Context, tree fs.FS) (core.Result, error) {
	work, err := os.MkdirTemp("", "colophon-cmd-*")
	if err != nil {
		return core.Result{}, err
	}
	defer func() { _ = os.RemoveAll(work) }()

	// The served tree goes in site/; the manifest sits beside it (not inside), so a command that
	// uploads the whole directory doesn't publish the manifest unless it opts in via $COLOPHON_MANIFEST.
	dir := filepath.Join(work, "site")
	total, bytesN, entries, err := writeTree(tree, dir)
	if err != nil {
		return core.Result{}, err
	}
	manifestPath := filepath.Join(work, "manifest.json")
	if err := writeManifest(manifestPath, entries); err != nil {
		return core.Result{}, err
	}

	vars := p.vars(dir, manifestPath, total)
	argv, err := interpolate(p.argv, vars)
	if err != nil {
		return core.Result{}, err
	}

	p.detail("run", strings.Join(argv, " "), "files", total)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = p.childEnv(dir, manifestPath)
	cmd.Stdin = nil // non-interactive: no TTY for a deploy CLI to prompt on
	var out lineLogger
	out.log, out.id = p.log, p.id
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return core.Result{}, fmt.Errorf("command publisher %q failed: %w\n%s", p.id, err, out.tail())
	}

	url, _ := p.CanonicalURL(ctx)
	return core.Result{Total: total, Uploaded: total, Bytes: bytesN, URL: url}, nil
}

// vars is the {placeholder} namespace: the user's own settings first, then the colophon-owned
// runtime values (which win on a name clash). Settings let a user reference anything they
// declared on the publisher — {domain}, {project}, … — so the command fits any target tool.
func (p *publisher) vars(dir, manifest string, total int) map[string]string {
	v := map[string]string{}
	for k, val := range p.settings {
		if k == "command" {
			continue
		}
		v[k] = scalar(val)
	}
	v["dir"] = dir
	v["output_dir"] = dir
	v["manifest"] = manifest
	v["public_url"] = p.publicURL
	v["id"] = p.id
	v["driver"] = "command"
	v["file_count"] = fmt.Sprintf("%d", total)
	return v
}

// childEnv is the parent environment (so the command's own token vars flow through) plus
// COLOPHON_* context and CI=true to nudge deploy CLIs into non-interactive mode.
func (p *publisher) childEnv(dir, manifest string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env,
		"COLOPHON_OUTPUT_DIR="+dir,
		"COLOPHON_MANIFEST="+manifest,
		"COLOPHON_PUBLIC_URL="+p.publicURL,
		"COLOPHON_PUBLISHER_ID="+p.id,
		"COLOPHON_DRIVER=command",
		"CI=true",
	)
	return env
}

func (p *publisher) detail(action, label string, kv ...any) {
	if p.log != nil {
		p.log.Detail("PUBLISH", p.id, append([]any{action, label}, kv...)...)
	}
}

// toArgv coerces the YAML `command` setting (a list of scalars) into a non-empty []string.
// A bare string is rejected on purpose: argv-only keeps the no-shell guarantee, and a one-line
// command is `["sh", "-c", "…"]` made explicit.
func toArgv(v any) ([]string, error) {
	switch t := v.(type) {
	case nil:
		return nil, errors.New("'command' is required (an argv list, e.g. [\"surge\", \"{dir}\", \"site\"])")
	case []string:
		return nonEmptyArgv(t)
	case []any:
		argv := make([]string, 0, len(t))
		for _, e := range t {
			argv = append(argv, scalar(e))
		}
		return nonEmptyArgv(argv)
	case string:
		return nil, fmt.Errorf("'command' must be a list, not a string; for a shell one-liner use [\"sh\", \"-c\", %q]", t)
	default:
		return nil, fmt.Errorf("'command' must be a list of strings, got %T", v)
	}
}

func nonEmptyArgv(argv []string) ([]string, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, errors.New("'command' must have at least one element (the program to run)")
	}
	return argv, nil
}

// interpolate replaces every {name} token in each arg from vars, erroring on an unknown name so
// a typo fails loudly rather than running a malformed command. `{{`/`}}` emit literal braces.
func interpolate(argv []string, vars map[string]string) ([]string, error) {
	out := make([]string, len(argv))
	for i, arg := range argv {
		var b strings.Builder
		for j := 0; j < len(arg); j++ {
			switch {
			case strings.HasPrefix(arg[j:], "{{"):
				b.WriteByte('{')
				j++
			case strings.HasPrefix(arg[j:], "}}"):
				b.WriteByte('}')
				j++
			case arg[j] == '{':
				end := strings.IndexByte(arg[j:], '}')
				if end < 0 {
					return nil, fmt.Errorf("unterminated {placeholder} in %q", arg)
				}
				name := arg[j+1 : j+end]
				val, ok := vars[name]
				if !ok {
					return nil, fmt.Errorf("unknown placeholder {%s} in %q (available: %s)", name, arg, available(vars))
				}
				b.WriteString(val)
				j += end
			default:
				b.WriteByte(arg[j])
			}
		}
		out[i] = b.String()
	}
	return out, nil
}

func available(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// scalar renders a YAML scalar setting as a string for interpolation; non-scalars (maps/lists)
// become their Go default formatting, which is good enough for an error-surfacing edge case.
func scalar(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// writeTree copies every file of the built tree into dir, returning the count, total bytes, and
// a path→bytes listing used to build the manifest.
func writeTree(tree fs.FS, dir string) (int, int64, map[string]int64, error) {
	entries := map[string]int64{}
	var total int
	var bytesN int64
	err := fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(tree, p)
		if err != nil {
			return err
		}
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			return err
		}
		entries[p] = int64(len(b))
		total++
		bytesN += int64(len(b))
		return nil
	})
	return total, bytesN, entries, err
}

// classify labels a path by role so a command can treat assets, pages and feeds differently
// (e.g. cache headers). It is a path/extension heuristic — colophon doesn't carry per-file
// roles to the publish boundary — so it errs toward "asset" for anything it doesn't recognise.
func classify(name string) string {
	base := strings.ToLower(path.Base(name))
	ext := strings.ToLower(path.Ext(name))
	switch {
	case ext == ".html" || ext == ".htm":
		return "page"
	case base == "sitemap.xml" || strings.HasPrefix(base, "sitemap"):
		return "sitemap"
	case base == "rss.xml" || base == "atom.xml" || base == "feed.xml" || base == "feed.json":
		return "feed"
	case base == "robots.txt" || base == "humans.txt" || base == "manifest.webmanifest":
		return "meta"
	default:
		return "asset"
	}
}

// contentType maps an extension to a MIME type, falling back to octet-stream.
func contentType(name string) string {
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		return strings.SplitN(ct, ";", 2)[0]
	}
	return "application/octet-stream"
}
