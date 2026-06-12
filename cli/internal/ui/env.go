// Package ui holds the CLI's presentation primitives: terminal detection, color theming, loading
// spinners, interactive selectors, and multi-format (table/json/csv) rendering. Everything degrades
// gracefully when the streams are not terminals (pipes, tests), so output stays clean and scriptable.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Format is an output rendering format chosen by the global --output/-o flag.
type Format int

const (
	// FormatTable is the human-friendly aligned table (the default on a terminal).
	FormatTable Format = iota
	// FormatJSON emits a stable, CLI-shaped JSON document (the default when stdout is piped).
	FormatJSON
	// FormatCSV emits comma-separated rows for spreadsheets.
	FormatCSV
)

// Env carries the resolved output environment for one CLI invocation: where to write, which format
// to render, and whether color and interactivity are available. It is built once (by Resolve) and
// threaded onto App, so commands never reach for process globals.
type Env struct {
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader

	Format Format
	color  bool // styled output is both wanted and possible
	outTTY bool // Stdout is a terminal
	errTTY bool // Stderr is a terminal (gates spinners and interactive prompts)
	inTTY  bool // Stdin is a terminal (gates the arrow-key selector)
}

// Resolve computes the output environment from the streams and the global flags. outFmt is the raw
// --output value: "" means auto (table on a terminal, json when piped). An unknown value is an error.
func Resolve(stdout, stderr io.Writer, stdin io.Reader, outFmt string, noColor bool) (*Env, error) {
	e := &Env{
		Stdout: stdout,
		Stderr: stderr,
		Stdin:  stdin,
		outTTY: isTerminal(stdout),
		errTTY: isTerminal(stderr),
		inTTY:  isTerminal(stdin),
	}

	switch strings.ToLower(strings.TrimSpace(outFmt)) {
	case "":
		if e.outTTY {
			e.Format = FormatTable
		} else {
			e.Format = FormatJSON
		}
	case "table":
		e.Format = FormatTable
	case "json":
		e.Format = FormatJSON
	case "csv":
		e.Format = FormatCSV
	default:
		return nil, fmt.Errorf("unknown output format %q (use table, json, or csv)", outFmt)
	}

	e.color = !noColor && os.Getenv("NO_COLOR") == "" && e.outTTY
	return e, nil
}

// Custom builds an Env with explicit settings, bypassing terminal detection. It exists for callers
// that already know their environment — chiefly tests, which need deterministic color and an
// interactive flag they can drive against in-memory buffers.
func Custom(stdin io.Reader, stdout, stderr io.Writer, f Format, color, interactive bool) *Env {
	return &Env{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Format: f,
		color:  color,
		outTTY: interactive,
		errTTY: interactive,
		inTTY:  interactive,
	}
}

// Interactive reports whether the user can be prompted for input (stdin is a terminal). Callers use
// it to decide between opening a picker and returning a "pass the value explicitly" error.
func (e *Env) Interactive() bool { return e.inTTY }

// Colored reports whether styled output is enabled.
func (e *Env) Colored() bool { return e.color }

func (e *Env) out() io.Writer {
	if e.Stdout == nil {
		return io.Discard
	}
	return e.Stdout
}

func (e *Env) err() io.Writer {
	if e.Stderr == nil {
		return io.Discard
	}
	return e.Stderr
}

func (e *Env) in() io.Reader {
	if e.Stdin == nil {
		return strings.NewReader("")
	}
	return e.Stdin
}

// isTerminal reports whether v is backed by a terminal file descriptor. A *bytes.Buffer (tests) or
// any non-file stream has no Fd, so it is reported non-interactive — keeping tests deterministic.
func isTerminal(v any) bool {
	f, ok := v.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
