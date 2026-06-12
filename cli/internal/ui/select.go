package ui

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// Option is one selectable entry. Label is shown to the user; Value is returned when it is chosen.
type Option struct {
	Label string
	Value string
}

var (
	// ErrCancelled is returned when the user aborts a selection (Ctrl-C, q, or empty input).
	ErrCancelled = errors.New("selection cancelled")
	// ErrNoInput is returned when there is no input available to read a selection from.
	ErrNoInput = errors.New("no input available to make a selection")
)

// Select asks the user to choose one of opts. With a real terminal on Stdin and Stderr it draws an
// interactive arrow-key list; otherwise (or if raw mode can't be enabled) it falls back to a
// numbered prompt read from Stdin. All prompting and echo go to Stderr, so Stdout stays a clean
// channel for piped data. Callers should gate the interactive path with Interactive() when they
// want a hard error in non-terminal contexts instead of consuming stdin.
func (e *Env) Select(title string, opts []Option) (Option, error) {
	if len(opts) == 0 {
		return Option{}, fmt.Errorf("nothing to choose from")
	}
	if e.inTTY && e.errTTY {
		if opt, ok, err := e.selectArrow(title, opts); ok {
			return opt, err
		}
		// Raw mode unavailable on these streams — fall through to the numbered prompt.
	}
	return e.selectNumbered(title, opts)
}

// selectArrow runs the raw-mode arrow-key picker. The bool result reports whether the picker ran;
// false means raw mode could not be enabled (e.g. Stdin is not a real file) and the caller should
// fall back. The terminal is always restored, including on SIGINT/SIGTERM.
func (e *Env) selectArrow(title string, opts []Option) (Option, bool, error) {
	f, ok := e.Stdin.(interface{ Fd() uintptr })
	if !ok {
		return Option{}, false, nil
	}
	fd := int(f.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return Option{}, false, nil
	}
	defer term.Restore(fd, oldState)

	// Safety net: if a signal arrives while the terminal is raw, restore it and exit cleanly rather
	// than leaving the user's shell in a broken state.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	guardDone := make(chan struct{})
	defer close(guardDone)
	go func() {
		select {
		case <-sigCh:
			term.Restore(fd, oldState)
			fmt.Fprint(e.err(), "\r\n")
			os.Exit(130)
		case <-guardDone:
		}
	}()

	reader := bufio.NewReader(e.Stdin)
	selected := 0
	lines := len(opts) + 1 // title row + one row per option (the hint shares the final cursor line)
	first := true
	repaint := func() {
		if !first {
			fmt.Fprintf(e.err(), "\r\x1b[%dA", lines)
		}
		first = false
		e.drawMenu(title, opts, selected)
	}
	clear := func() { fmt.Fprintf(e.err(), "\r\x1b[%dA\x1b[J", lines) }

	repaint()
	for {
		b, err := reader.ReadByte()
		if err != nil {
			clear()
			return Option{}, true, ErrCancelled
		}
		switch b {
		case '\r', '\n':
			clear()
			return opts[selected], true, nil
		case 3, 'q', 'Q': // Ctrl-C or quit
			clear()
			return Option{}, true, ErrCancelled
		case 'k': // vim up
			selected = (selected - 1 + len(opts)) % len(opts)
			repaint()
		case 'j': // vim down
			selected = (selected + 1) % len(opts)
			repaint()
		case 27: // ESC — start of an arrow-key sequence
			b2, err := reader.ReadByte()
			if err != nil || b2 != '[' {
				continue
			}
			b3, err := reader.ReadByte()
			if err != nil {
				continue
			}
			switch b3 {
			case 'A': // up
				selected = (selected - 1 + len(opts)) % len(opts)
				repaint()
			case 'B': // down
				selected = (selected + 1) % len(opts)
				repaint()
			}
		}
	}
}

// drawMenu paints the title, the options with the current one highlighted, and a key hint. In raw
// mode line breaks must be explicit \r\n, and each line is cleared to the right (\x1b[K) so a
// shorter repaint can't leave stale characters behind.
func (e *Env) drawMenu(title string, opts []Option, selected int) {
	w := e.err()
	fmt.Fprintf(w, "\r%s\x1b[K\r\n", e.Color(title, StyleHeader))
	for i, opt := range opts {
		if i == selected {
			fmt.Fprintf(w, "%s%s\x1b[K\r\n", e.Color("❯ ", StyleStatusOK), e.Color(opt.Label, StyleStatusOK))
		} else {
			fmt.Fprintf(w, "  %s\x1b[K\r\n", opt.Label)
		}
	}
	fmt.Fprintf(w, "%s\x1b[K", e.Color("  ↑/↓ move · enter select · q cancel", StyleMuted))
}

// selectNumbered prints a numbered list and reads a choice from Stdin. It is the non-terminal
// fallback (and what tests drive), so it never needs raw mode.
func (e *Env) selectNumbered(title string, opts []Option) (Option, error) {
	w := e.err()
	fmt.Fprintln(w, e.Color(title, StyleHeader))
	for i, opt := range opts {
		fmt.Fprintf(w, "  %s %s\n", e.Color(strconv.Itoa(i+1)+")", StyleMuted), opt.Label)
	}
	fmt.Fprint(w, "Enter a number: ")

	line, err := bufio.NewReader(e.in()).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		if err != nil {
			return Option{}, ErrNoInput
		}
		return Option{}, ErrCancelled
	}
	n, convErr := strconv.Atoi(line)
	if convErr != nil || n < 1 || n > len(opts) {
		return Option{}, fmt.Errorf("invalid selection %q (choose 1-%d)", line, len(opts))
	}
	return opts[n-1], nil
}
