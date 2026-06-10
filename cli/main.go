// Command obank is the open-banking.io command line: authenticate once, then read and sync your
// accounts and transactions locally, decrypting the zero-knowledge data with your own key.
package main

import (
	"fmt"
	"os"

	"github.com/open-banking-io/clients/cli/internal/app"
	"github.com/open-banking-io/clients/cli/internal/config"
)

func main() {
	path, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "obank:", err)
		os.Exit(1)
	}
	a := &app.App{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		ConfigPath: path,
	}
	if err := a.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "obank:", err)
		os.Exit(1)
	}
}
