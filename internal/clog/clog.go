// Package clog is colophon's human-facing progress log. Each line is:
//
//	CATEGORY  label      key=value key=value ...
//
// The CATEGORY and label are space-padded columns for easy scanning; the rest are
// logfmt key=value fields (values with spaces are quoted) for easy machine parsing.
// Step lines always show; Detail lines show only with --verbose.
package clog

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Logger writes aligned progress lines. A nil *Logger is a no-op, so callers without one
// can pass nil freely. labelWidth is the label column width — the caller sizes it to the
// longest label it will use (e.g. its source/publisher/env names), so columns line up.
type Logger struct {
	w          io.Writer
	verbose    bool
	labelWidth int
}

func New(w io.Writer, verbose bool, labelWidth int) *Logger {
	if labelWidth < 1 {
		labelWidth = 10
	}
	return &Logger{w: w, verbose: verbose, labelWidth: labelWidth}
}

func (l *Logger) Verbose() bool { return l != nil && l.verbose }

// Step prints an always-visible line. kv is alternating key, value pairs.
func (l *Logger) Step(category, label string, kv ...any) {
	if l == nil {
		return
	}
	_, _ = fmt.Fprintf(l.w, "%-8s %-*s %s\n", category, l.labelWidth, label, fields(kv))
}

// Detail prints a line only under --verbose (same format as Step).
func (l *Logger) Detail(category, label string, kv ...any) {
	if l == nil || !l.verbose {
		return
	}
	l.Step(category, label, kv...)
}

func fields(kv []any) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%v=%s", kv[i], logfmtValue(kv[i+1]))
	}
	return b.String()
}

func logfmtValue(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\"=") {
		return strconv.Quote(s)
	}
	return s
}

// Aware is an optional capability for drivers (publishers, sources) that want to emit
// their own progress: the CLI calls SetLogger after constructing them.
type Aware interface {
	SetLogger(*Logger)
}
