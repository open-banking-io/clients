package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

// deadURL returns the URL of a server that has already been closed: connecting to it fails.
func deadURL(t *testing.T) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := s.URL
	s.Close()
	return url
}

func TestRunVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, Version: "1.2.3"}
	if err := app.Run([]string{"version"}); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "openbanking 1.2.3") {
		t.Errorf("version output = %q", out.String())
	}
}

func TestRunVersionDefaultsToDev(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut} // no Version set
	if err := app.Run([]string{"--version"}); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "openbanking dev") {
		t.Errorf("version output = %q, want 'openbanking dev'", out.String())
	}
}

func TestRunHelpPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut}
	if err := app.Run([]string{"help"}); err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(errOut.String(), "openbanking <command>") {
		t.Errorf("usage not printed; stderr = %q", errOut.String())
	}
}

func TestRunNoCommandErrorsAndPrintsUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut}
	if err := app.Run(nil); err == nil {
		t.Fatal("expected an error when no command is given")
	}
	if !strings.Contains(errOut.String(), "Usage:") {
		t.Errorf("usage not printed on empty args; stderr = %q", errOut.String())
	}
}

// TestKeyImportDefaultStdin exercises the nil-Stdin path (stdin() returns an empty reader): the
// command reads no key and fails cleanly rather than panicking.
func TestKeyImportDefaultStdin(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "credentials.json")
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg} // Stdin nil
	if err := app.Run([]string{"key", "import"}); err == nil {
		t.Fatal("expected an error when no key is provided on stdin")
	}
}

func TestPublicClientCommandsRequireAPIKey(t *testing.T) {
	// A config that exists but carries no API key.
	cfg := filepath.Join(t.TempDir(), "credentials.json")
	if err := config.Save(cfg, config.Bundle{APIBaseURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"banks"}); err == nil {
		t.Fatal("expected an error when the config has no API key")
	}
}

func TestPublicClientCommandsRequireConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "missing.json")}
	if err := app.Run([]string{"banks"}); err == nil {
		t.Fatal("expected an error when the config is missing")
	}
}

func TestBanksReportsListError(t *testing.T) {
	bundle := fixtureBundle(t)
	cfg := writeConfig(t, bundle, deadURL(t))
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"banks"}); err == nil {
		t.Fatal("expected an error when the banks request fails")
	}
}

func TestConnectionsReportsListError(t *testing.T) {
	bundle := fixtureBundle(t)
	cfg := writeConfig(t, bundle, deadURL(t))
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"connections"}); err == nil {
		t.Fatal("expected an error when the connections request fails")
	}
}

func TestSyncReportsError(t *testing.T) {
	bundle := fixtureBundle(t)
	cfg := writeConfig(t, bundle, deadURL(t))
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"sync", "--all"}); err == nil {
		t.Fatal("expected an error when the sync request fails")
	}
}

func TestSyncAllRejectsAccountID(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"sync", "--all", "some-account"}); err == nil {
		t.Fatal("expected an error: `sync --all` takes no account id")
	}
}

func TestRenderBanksShowsBetaFlag(t *testing.T) {
	var out bytes.Buffer
	banks := []openbanking.Bank{
		{Name: "Lunar", Country: "DK", Bic: "LUNADK22", PsuTypes: []string{"business"}, Beta: false},
		{Name: "NewBank", Country: "DK", Beta: true},
	}
	e := ui.Custom(nil, &out, &out, ui.FormatTable, false, false)
	if err := e.Render(banksTable(banks), banksView(banks)); err != nil {
		t.Fatalf("render banks: %v", err)
	}
	if !strings.Contains(out.String(), "beta") {
		t.Errorf("expected a beta marker in output\n%s", out.String())
	}
}
