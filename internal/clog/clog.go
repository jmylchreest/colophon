// Package clog is colophon's human-facing progress log: a thin, nil-safe adapter over
// log/slog (via github.com/jmylchreest/slog-logfilter). Events are structured key=value
// (logfmt text by default, JSON on request) so they never wrap into fixed-width columns,
// and real levels drive verbosity:
//
//	Step  → slog Info  (always visible at the default level)
//	Detail→ slog Debug (visible under --verbose / debug)
//
// The old CATEGORY/label columns survive as the "category" and "label" attributes, so
// existing call sites keep their shape while output stays grep-friendly and terminal-safe.
package clog

import (
	"io"
	"log/slog"
	"os"
	"strings"

	logfilter "github.com/jmylchreest/slog-logfilter"
)

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
	// Verbose drops the level to Debug (so Detail lines show).
	Verbose bool
	// JSON selects the JSON formatter instead of logfmt text.
	JSON bool
	// Filter is a RUST_LOG-style spec for per-module/attribute verbosity, applied via
	// slog-logfilter. Empty means no filters. See ParseFilters.
	Filter string
}

// New builds a Logger from Options. The level and filters live on slog-logfilter's global
// handler, so the most recent New wins for any runtime SetLevel/SetFilters.
func New(opts Options) *Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}
	level := slog.LevelInfo
	if opts.Verbose {
		level = slog.LevelDebug
	}
	format := "text"
	if opts.JSON {
		format = "json"
	}
	l := logfilter.New(
		logfilter.WithLevel(level),
		logfilter.WithFormat(format),
		logfilter.WithOutput(w),
		// Source file:line is noise in a build tool's progress log; attribute filters still
		// work without it, so we favour clean, narrow output.
		logfilter.WithSource(false),
		logfilter.WithFilters(ParseFilters(opts.Filter)),
	)
	return &Logger{slog: l, verbose: opts.Verbose}
}

// Discard is a logger that drops everything — for query paths (e.g. search indexing) whose
// progress must never reach the user or pollute --json stdout.
func Discard() *Logger {
	return &Logger{slog: slog.New(slog.DiscardHandler), verbose: false}
}

// Slog exposes the underlying *slog.Logger for code that wants to log directly. Nil-safe.
func (l *Logger) Slog() *slog.Logger {
	if l == nil {
		return slog.New(slog.DiscardHandler)
	}
	return l.slog
}

func (l *Logger) Verbose() bool { return l != nil && l.verbose }

// Step records an always-visible event (slog Info). category and label become attributes;
// kv is alternating key, value pairs appended after them.
func (l *Logger) Step(category, label string, kv ...any) {
	if l == nil {
		return
	}
	l.slog.Info(category, attrs(category, label, kv)...)
}

// Detail records a verbose-only event (slog Debug), same shape as Step.
func (l *Logger) Detail(category, label string, kv ...any) {
	if l == nil {
		return
	}
	l.slog.Debug(category, attrs(category, label, kv)...)
}

// attrs builds the slog argument list. category is the message and also re-emitted as an
// attribute so JSON consumers and attribute filters can match on it uniformly.
func attrs(category, label string, kv []any) []any {
	out := make([]any, 0, len(kv)+4)
	out = append(out, slog.String("category", category))
	if label != "" {
		out = append(out, slog.String("label", label))
	}
	return append(out, kv...)
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
		f, ok := parseFilter(part)
		if ok {
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
