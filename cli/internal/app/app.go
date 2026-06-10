// Package app implements the obank CLI command dispatch. Commands are methods on App so tests can
// drive them with injected writers, an explicit config path, and a custom HTTP client.
package app

import (
	"fmt"
	"io"
	"net/http"

	openbanking "github.com/open-banking-io/clients/go"
	"github.com/open-banking-io/clients/cli/internal/config"
)

// App carries the I/O and configuration a command needs. The zero value is not usable; main wires
// real os streams and the default config path, while tests inject buffers and a temp path.
type App struct {
	Stdout     io.Writer
	Stderr     io.Writer
	ConfigPath string
	HTTPClient *http.Client // nil uses the SDK default
}

// Run dispatches a single command invocation. args is everything after the program name.
func (a *App) Run(args []string) error {
	if len(args) == 0 {
		a.usage()
		return fmt.Errorf("no command given")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "accounts":
		return a.accounts(rest)
	case "transactions":
		return a.transactions(rest)
	case "connections":
		return a.connections(rest)
	case "help", "-h", "--help":
		a.usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (run `obank help`)", cmd)
	}
}

// client builds a decrypting SDK client from the saved credentials bundle.
func (a *App) client() (*openbanking.Client, error) {
	bundle, err := config.Load(a.ConfigPath)
	if err != nil {
		return nil, err
	}
	return openbanking.FromBundle(bundle, a.HTTPClient)
}

func (a *App) usage() {
	fmt.Fprint(a.Stderr, `obank — open-banking.io command line

Usage:
  obank <command> [flags]

Commands:
  accounts                       List your accounts with balances
  transactions <account-id>      Show an account's statement (--from --to --limit --offset)
  connections                    List your bank connections
  help                           Show this help
`)
}
