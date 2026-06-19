// Package clog is colophon's human-facing progress log: a thin, nil-safe adapter over
// log/slog (via github.com/jmylchreest/slog-logfilter). Events are structured key=value
// (logfmt text by default, JSON on request) so they never wrap into fixed-width columns,
// and real levels drive verbosity:
//
//	Step  → slog Info  (always visible at the default level)
//	Detail→ slog Debug (visible under --verbose / debug)
//
// The category is the event name (the slog message) and stays a filterable "category"
// attribute — shown in JSON, but elided from text output where the message already carries
// it. The optional label becomes a "label" attribute. Source file:line is attached under
// --verbose only, attributed to the Step/Detail call site.
package clog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	logfilter "github.com/jmylchreest/slog-logfilter"
)

// moduleRoot is trimmed from --verbose source paths to leave a short, module-relative file.
const moduleRoot = "github.com/jmylchreest/colophon/"

// Logger wraps a *slog.Logger with colophon's Step/Detail vocabulary. A nil *Logger is a
// no-op, so callers without one (e.g. the serve path) can pass nil freely.
type Logger struct {
	slog    *slog.Logger
	verbose bool
}

// Options configure a Logger. New fills any zero fields with sane defaults (Info level,
// logfmt text to stderr).
type Options struct {
	// Writer receives log lines. Defaults to os.Stderr so --json data on stdout stays clean.
	Writer io.Writer
	// Verbose drops the level to Debug (so Detail lines show) and attaches source file:line.
	Verbose bool
	// JSON selects the JSON formatter instead of logfmt text.
	JSON bool
	// Filter is a RUST_LOG-style spec for per-module/attribute verbosity, applied via
	// slog-logfilter. Empty means no filters. See ParseFilters.
	Filter string
}

// New builds a Logger from Options. It wraps a Text/JSON handler in slog-logfilter's filter
// handler; the filter sees every record attribute (so category-based filters work) while the
// inner handler decides rendering — which is where the redundant category is dropped in text.
func New(opts Options) *Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	level := slog.LevelInfo
	if opts.Verbose {
		level = slog.LevelDebug
	}
	lv := new(slog.LevelVar)
	lv.Set(level)

	json := opts.JSON
	ho := &slog.HandlerOptions{
		Level:     lv,
		AddSource: opts.Verbose,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch {
			case a.Key == "category" && !json && len(groups) == 0:
				return slog.Attr{} // the text message already is the category
			case a.Key == slog.SourceKey:
				return trimSource(a)
			}
			return a
		},
	}
	var inner slog.Handler
	if json {
		inner = slog.NewJSONHandler(w, ho)
	} else {
		inner = slog.NewTextHandler(w, ho)
	}
	h := logfilter.NewHandler(inner, lv)
	h.SetFilters(ParseFilters(opts.Filter))
	return &Logger{slog: slog.New(h), verbose: opts.Verbose}
}

// Discard is a logger that drops everything — for query paths (e.g. search indexing) whose
// progress must never reach the user or pollute --json stdout.
func Discard() *Logger {
	return &Logger{slog: slog.New(slog.DiscardHandler)}
}

func (l *Logger) Verbose() bool { return l != nil && l.verbose }

// Step records an always-visible event (slog Info). category becomes the message; label and
// kv (alternating key, value) become attributes.
func (l *Logger) Step(category, label string, kv ...any) {
	l.emit(slog.LevelInfo, category, label, kv)
}

// Detail records a verbose-only event (slog Debug), same shape as Step.
func (l *Logger) Detail(category, label string, kv ...any) {
	l.emit(slog.LevelDebug, category, label, kv)
}

// emit builds the record attributed to the Step/Detail caller (so --verbose source points at
// the real call site, not clog) and hands it to the filter handler.
func (l *Logger) emit(level slog.Level, category, label string, kv []any) {
	if l == nil || l.slog == nil {
		return
	}
	ctx := context.Background()
	h := l.slog.Handler()
	if !h.Enabled(ctx, level) {
		return
	}
	var pc uintptr
	if l.verbose { // a PC is only worth capturing when source will be rendered
		var pcs [1]uintptr
		runtime.Callers(3, pcs[:]) // skip Callers, emit, Step/Detail
		pc = pcs[0]
	}
	r := slog.NewRecord(time.Now(), level, category, pc)
	r.Add(attrs(category, label, kv)...)
	_ = h.Handle(ctx, r)
}

// attrs builds the attribute list. category is re-emitted as an attribute so JSON consumers
// and attribute filters can match on it (text rendering drops it, see New).
func attrs(category, label string, kv []any) []any {
	out := make([]any, 0, len(kv)+4)
	out = append(out, slog.String("category", category))
	if label != "" {
		out = append(out, slog.String("label", label))
	}
	return append(out, kv...)
}

// trimSource shortens a source path to module-relative (falling back to the base name).
func trimSource(a slog.Attr) slog.Attr {
	s, ok := a.Value.Any().(*slog.Source)
	if !ok || s == nil {
		return a
	}
	if i := strings.LastIndex(s.File, moduleRoot); i >= 0 {
		s.File = s.File[i+len(moduleRoot):]
	} else {
		s.File = filepath.Base(s.File)
	}
	return a
}

// ParseFilters turns a RUST_LOG-style spec into slog-logfilter filters. The spec is a
// comma-separated list of directives; first match wins. Each directive is one of:
//
//	level                  bare level → all events (attribute category=*) at that level
//	attr=level             elevate/suppress events carrying that attribute, e.g. category=debug
//	source:file:glob=level a source filter, e.g. source:file:internal/publish/*=debug
//
// The level is one of debug, info, warn, error. Malformed directives are skipped.
func ParseFilters(spec string) []logfilter.LogFilter {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	var filters []logfilter.LogFilter
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if f, ok := parseFilter(part); ok {
			filters = append(filters, f)
		}
	}
	return filters
}

func parseFilter(part string) (logfilter.LogFilter, bool) {
	// A bare token is a level applied to the universal "category" attribute.
	target, level := "category", part
	if i := strings.LastIndex(part, "="); i >= 0 {
		target, level = strings.TrimSpace(part[:i]), strings.TrimSpace(part[i+1:])
	}
	level = strings.ToLower(level)
	switch level {
	case "debug", "info", "warn", "error":
	default:
		return logfilter.LogFilter{}, false
	}
	pattern := "*"
	// source:file:<glob> / source:function:<glob> carry the glob inline; split it off.
	if strings.HasPrefix(target, "source:") {
		if k := strings.LastIndex(target, ":"); k > len("source") {
			pattern = target[k+1:]
			target = target[:k]
		}
	}
	if target == "" {
		return logfilter.LogFilter{}, false
	}
	return logfilter.LogFilter{Type: target, Pattern: pattern, Level: level, Enabled: true}, true
}

// Aware is an optional capability for drivers (publishers, sources) that want to emit their
// own progress: the CLI calls SetLogger after constructing them.
type Aware interface {
	SetLogger(*Logger)
}
