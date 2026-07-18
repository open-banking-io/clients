// Command openbanking is the open-banking.io command line: authenticate once, then read and sync your
// accounts and transactions locally, decrypting the zero-knowledge data with your own key.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/open-banking-io/clients/cli/internal/app"
	"github.com/open-banking-io/clients/cli/internal/config"
)

// version is set at build time via -ldflags "-X main.version=...". GoReleaser fills it from the tag.
var version = "dev"

func main() {
	path, err := config.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "openbanking:", err)
		os.Exit(1)
	}

	// Pull the optional global --timeout out of the argument list before dispatch (it applies to
	// every command's HTTP calls, not to any single command's own FlagSet). Combined with the
	// optional OPENBANKING_CA_CERT env, it opts the CLI into a custom HTTP client; with neither set,
	// httpClient stays nil and behavior is exactly as before.
	args, timeout, err := parseTimeout(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "openbanking:", err)
		os.Exit(1)
	}
	httpClient, err := app.BuildHTTPClient(timeout, os.Getenv(app.CACertEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "openbanking:", err)
		os.Exit(1)
	}

	a := &app.App{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		ConfigPath: path,
		Version:    version,
		HTTPClient: httpClient,
	}
	if err := a.Run(args); err != nil {
		fmt.Fprintln(os.Stderr, "openbanking:", err)
		os.Exit(1)
	}
}

// parseTimeout pulls the global --timeout flag (a Go duration such as 30s) out of args wherever it
// appears, returning the remaining args untouched so they dispatch exactly as before. It mirrors the
// stdlib-style manual flag handling used for the other global flags. A zero duration means the flag
// was not given.
func parseTimeout(args []string) (rest []string, timeout time.Duration, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--timeout":
			if i+1 >= len(args) {
				return nil, 0, fmt.Errorf("flag --timeout needs a duration (e.g. 30s)")
			}
			timeout, err = time.ParseDuration(args[i+1])
			if err != nil {
				return nil, 0, fmt.Errorf("invalid --timeout %q: %w", args[i+1], err)
			}
			i++
		case strings.HasPrefix(arg, "--timeout="):
			v := strings.TrimPrefix(arg, "--timeout=")
			timeout, err = time.ParseDuration(v)
			if err != nil {
				return nil, 0, fmt.Errorf("invalid --timeout %q: %w", v, err)
			}
		default:
			rest = append(rest, arg)
		}
	}
	return rest, timeout, nil
}
