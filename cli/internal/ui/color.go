package ui

import (
	"fmt"
	"strings"
)

// Style is a semantic role for a piece of text. The concrete ANSI code is chosen by the theme, so
// commands describe meaning ("this is a negative amount") rather than a color.
type Style int

const (
	StyleNone Style = iota
	StyleHeader
	StylePositive
	StyleNegative
	StyleStatusOK
	StyleStatusWarn
	StyleError
	StyleSuccess
	StyleMuted
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

func styleCode(s Style) string {
	switch s {
	case StyleHeader:
		return ansiBold + ansiCyan
	case StylePositive, StyleStatusOK, StyleSuccess:
		return ansiGreen
	case StyleNegative, StyleError:
		return ansiRed
	case StyleStatusWarn:
		return ansiYellow
	case StyleMuted:
		return ansiDim
	default:
		return ""
	}
}

// Color wraps s in the ANSI codes for style when color is enabled; otherwise it returns s unchanged
// so the same call site works in both colored and plain (piped, NO_COLOR) output.
func (e *Env) Color(s string, style Style) string {
	if !e.color || style == StyleNone || s == "" {
		return s
	}
	code := styleCode(style)
	if code == "" {
		return s
	}
	return code + s + ansiReset
}

// AmountStyle colors a monetary amount by direction: a leading minus (money out) is red, anything
// else green. Matches the presentation-edge signing used by the transactions view.
func AmountStyle(amount string) Style {
	if strings.HasPrefix(strings.TrimSpace(amount), "-") {
		return StyleNegative
	}
	return StylePositive
}

// StatusStyle colors a connection/transaction status: healthy states green, problem states red,
// everything else neutral. The match is case-insensitive and tolerant of unknown values.
func StatusStyle(status string) Style {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ok", "active", "booked", "valid", "connected", "ready":
		return StyleStatusOK
	case "expired", "error", "failed", "revoked", "rejected", "suspended":
		return StyleNegative
	case "", "-":
		return StyleNone
	default:
		return StyleStatusWarn
	}
}

// Successf prints a success line to Stderr — green with a check mark when color is on, plain text
// otherwise. Status chatter goes to Stderr so Stdout stays a clean data channel for piping.
func (e *Env) Successf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if e.color {
		fmt.Fprintln(e.err(), e.Color("✓ "+msg, StyleSuccess))
		return
	}
	fmt.Fprintln(e.err(), msg)
}

// Infof prints a neutral informational line to Stderr (dim when color is on).
func (e *Env) Infof(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintln(e.err(), e.Color(msg, StyleMuted))
}

// Errorf prints an error line to Stderr — red with a cross when color is on, plain text otherwise.
func (e *Env) Errorf(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if e.color {
		fmt.Fprintln(e.err(), e.Color("✗ "+msg, StyleError))
		return
	}
	fmt.Fprintln(e.err(), msg)
}
