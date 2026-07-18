// Package app implements the openbanking CLI command dispatch. Commands are methods on App so tests can
// drive them with injected writers, an explicit config path, and a custom HTTP client.
package app

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

// apiBaseURLEnv overrides the saved bundle's API base URL when set, letting the CLI target a
// staging/local environment without re-running `login`. Empty/whitespace keeps the bundle's value.
// Mirrors the n8n node's "API Base URL Override" credential field.
const apiBaseURLEnv = "OPENBANKING_API_BASE_URL"

// App carries the I/O and configuration a command needs. The zero value is not usable; main wires
// real os streams and the default config path, while tests inject buffers and a temp path.
type App struct {
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	ConfigPath string
	Version    string       // set by main from the build-time version
	HTTPClient *http.Client // nil uses the SDK default

	env *ui.Env // resolved per run; tests may pre-set it to force color/format/interactivity
}

// stdin returns the configured input, defaulting to an empty reader when unset.
func (a *App) stdin() io.Reader {
	if a.Stdin == nil {
		return strings.NewReader("")
	}
	return a.Stdin
}

// stdout/stderr return the configured streams, defaulting to io.Discard so a partly-built App (or a
// command reached before Run resolves the environment) never nil-panics.
func (a *App) stdout() io.Writer {
	if a.Stdout == nil {
		return io.Discard
	}
	return a.Stdout
}

func (a *App) stderr() io.Writer {
	if a.Stderr == nil {
		return io.Discard
	}
	return a.Stderr
}

// ui returns the resolved output environment, lazily building a default one. Run sets a.env up front
// (honoring the global flags), but commands invoked directly in tests get a sensible default here so
// they never nil-panic on a.env.
func (a *App) ui() *ui.Env {
	if a.env == nil {
		a.env, _ = ui.Resolve(a.stdout(), a.stderr(), a.stdin(), "", false)
	}
	return a.env
}

// Run dispatches a single command invocation. args is everything after the program name. It first
// strips the global flags (--output, --no-color, --config), resolves the output environment once,
// then dispatches through the command table so names, aliases, help and completion share one source.
func (a *App) Run(args []string) error {
	rest, opt, err := parseGlobals(args)
	if err != nil {
		return err
	}
	if opt.config != "" {
		a.ConfigPath = opt.config
	}
	if a.env == nil {
		env, err := ui.Resolve(a.stdout(), a.stderr(), a.stdin(), opt.output, opt.noColor)
		if err != nil {
			return err
		}
		a.env = env
	}

	if len(rest) == 0 {
		// On a terminal, an empty invocation opens the interactive menu; otherwise (piped, scripts)
		// it prints usage and errors, exactly as before.
		if a.env.Interactive() {
			return a.interactiveMenu()
		}
		a.usage()
		return fmt.Errorf("no command given")
	}

	cmd, cargs := rest[0], rest[1:]
	c, ok := a.lookup(cmd)
	if !ok {
		return fmt.Errorf("unknown command %q (run `openbanking help`)", cmd)
	}
	return c.run(a, cargs)
}

// loadBundle reads the saved credentials bundle and applies the OPENBANKING_API_BASE_URL override
// when set, so every command can target staging/local without re-running `login`.
func (a *App) loadBundle() (config.Bundle, error) {
	bundle, err := config.Load(a.ConfigPath)
	if err != nil {
		return config.Bundle{}, err
	}
	if override := trimTrailingSlash(strings.TrimSpace(os.Getenv(apiBaseURLEnv))); override != "" {
		bundle.APIBaseURL = override
	}
	return bundle, nil
}

// client builds a decrypting SDK client from the saved credentials bundle (needs the encryption key).
func (a *App) client() (*openbanking.Client, error) {
	bundle, err := a.loadBundle()
	if err != nil {
		return nil, err
	}
	return openbanking.FromBundle(bundle, a.HTTPClient)
}

// publicClient builds a client for operations that don't decrypt data (e.g. banks), so they
// work right after `login`, before any encryption key has been imported.
func (a *App) publicClient() (*openbanking.Client, error) {
	bundle, err := a.loadBundle()
	if err != nil {
		return nil, err
	}
	if bundle.APIKey == "" {
		return nil, fmt.Errorf("no API key — run `openbanking login` first")
	}
	return openbanking.NewPublic(bundle.APIBaseURL, bundle.APIKey, a.HTTPClient)
}

func (a *App) versionString() string {
	if a.Version == "" {
		return "dev"
	}
	return a.Version
}

// usage prints the help text, generated from the command table so it can never drift from the
// commands that actually exist.
func (a *App) usage() {
	var b strings.Builder
	b.WriteString("openbanking — open-banking.io command line\n\n")
	b.WriteString("Usage:\n")
	b.WriteString("  openbanking <command> [flags]\n")
	b.WriteString("  openbanking [--output table|json|csv] [--no-color] [--config <path>] <command>\n\n")
	b.WriteString("Commands:\n")
	for _, c := range a.commands() {
		if c.hidden {
			continue
		}
		name := c.name
		if len(c.aliases) > 0 {
			name += " (" + strings.Join(c.aliases, ", ") + ")"
		}
		fmt.Fprintf(&b, "  %-22s %s\n", name, c.short)
	}
	b.WriteString("\nGlobal flags:\n")
	b.WriteString("  -o, --output   Output format: table (default on a terminal), json (default when piped), csv\n")
	b.WriteString("      --no-color Disable colored output (also respects NO_COLOR)\n")
	b.WriteString("      --config   Path to the credentials file\n")
	b.WriteString("      --timeout  HTTP request timeout as a Go duration, e.g. 30s (default: SDK default)\n")
	b.WriteString("\nHTTP: OPENBANKING_CA_CERT points at a PEM file of extra trusted root CAs; the standard\n")
	b.WriteString("HTTPS_PROXY / HTTP_PROXY / NO_PROXY variables are honored automatically.\n")
	b.WriteString("\nRun `openbanking <command> --help` for a command's own flags, or `openbanking` with no\n")
	b.WriteString("arguments for an interactive menu.\n")
	fmt.Fprint(a.stderr(), b.String())
}
