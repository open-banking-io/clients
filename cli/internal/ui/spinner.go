package ui

import (
	"fmt"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner starts an animated spinner on Stderr with the given label and returns a stop function.
// When Stderr is not a terminal (tests, pipes) the spinner is a no-op and stop does nothing, so
// stdout stays clean and captured test output is undisturbed. stop is safe to call more than once;
// it waits for the animation goroutine to exit before clearing the line, so no stray frame can be
// written afterwards.
func (e *Env) Spinner(label string) (stop func()) {
	if !e.errTTY {
		return func() {}
	}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(90 * time.Millisecond)
		defer ticker.Stop()
		paint := func(i int) {
			frame := e.Color(spinnerFrames[i%len(spinnerFrames)], StyleStatusOK)
			fmt.Fprintf(e.err(), "\r%s %s", frame, label)
		}
		paint(0)
		for i := 1; ; i++ {
			select {
			case <-done:
				return
			case <-ticker.C:
				paint(i)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-finished
			fmt.Fprint(e.err(), "\r\x1b[K")
		})
	}
}
