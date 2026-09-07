package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
)

const fixtureAccountID = "11111111-1111-4111-8111-111111111111"

// newApp wires an App against the mock API server with the fixture credentials.
func newApp(t *testing.T, out, errOut *bytes.Buffer) *App {
	t.Helper()
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)
	return &App{Stdout: out, Stderr: errOut, ConfigPath: cfg}
}

func TestCommandAliasesDispatch(t *testing.T) {
	for _, alias := range []string{"acc", "ls"} {
		var out, errOut bytes.Buffer
		app := newApp(t, &out, &errOut)
		if err := app.Run([]string{"-o", "json", alias}); err != nil {
			t.Fatalf("alias %q: %v\nstderr: %s", alias, err, errOut.String())
		}
		if !strings.Contains(out.String(), fixtureAccountID) {
			t.Errorf("alias %q did not behave like `accounts`:\n%s", alias, out.String())
		}
	}

	// tx is the alias for transactions and takes the account id positionally.
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := app.Run([]string{"-o", "json", "tx", fixtureAccountID}); err != nil {
		t.Fatalf("alias tx: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "-194.23") {
		t.Errorf("alias tx did not behave like `transactions`:\n%s", out.String())
	}
}

func TestAccountsJSONOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := app.Run([]string{"-o", "json", "accounts"}); err != nil {
		t.Fatalf("accounts -o json: %v\nstderr: %s", err, errOut.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output is not a json array: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0]["id"] != fixtureAccountID {
		t.Errorf("unexpected json: %+v", got)
	}
	if got[0]["bank"] != "Lunar" {
		t.Errorf("expected lowerCamel CLI keys (bank=Lunar), got: %+v", got[0])
	}
}

func TestAccountsCSVOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := app.Run([]string{"-o", "csv", "accounts"}); err != nil {
		t.Fatalf("accounts -o csv: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "NAME,IBAN,TYPE,BALANCE,CUR,BANK,ID") {
		t.Errorf("csv header missing:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Drift") {
		t.Errorf("csv data row missing:\n%s", out.String())
	}
}

func TestPipedDefaultsToJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	// No -o flag and a captured (non-terminal) stdout -> json by default.
	if err := app.Run([]string{"accounts"}); err != nil {
		t.Fatalf("accounts: %v\nstderr: %s", err, errOut.String())
	}
	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("piped output should default to json, but did not parse: %v\n%s", err, out.String())
	}
}

func TestUseSetsCurrentAccountExplicit(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := app.Run([]string{"use", fixtureAccountID}); err != nil {
		t.Fatalf("use: %v\nstderr: %s", err, errOut.String())
	}
	state, err := config.LoadState(app.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentAccountID != fixtureAccountID {
		t.Errorf("current account = %q, want %q", state.CurrentAccountID, fixtureAccountID)
	}
}

func TestUseUnknownAccountErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := app.Run([]string{"use", "does-not-exist"}); err == nil {
		t.Fatal("expected an error for an unknown account id")
	}
}

func TestUseNoIDNonInteractiveErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	// Non-terminal streams: there's no id and no way to prompt, so it must error clearly.
	if err := app.Run([]string{"use"}); err == nil {
		t.Fatal("expected an error: no id and not a terminal")
	}
}

func TestUseInteractivePicker(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	// Force an interactive env reading a choice from a buffer; the arrow-key path can't run on a
	// buffer (no Fd), so Select falls back to the numbered prompt and reads "1".
	app.env = ui.Custom(strings.NewReader("1\n"), &out, &errOut, ui.FormatTable, false, true)
	if err := app.Run([]string{"use"}); err != nil {
		t.Fatalf("use (picker): %v\nstderr: %s", err, errOut.String())
	}
	state, _ := config.LoadState(app.ConfigPath)
	if state.CurrentAccountID != fixtureAccountID {
		t.Errorf("picker did not save the chosen account, got %q", state.CurrentAccountID)
	}
}

func TestTransactionsUsesCurrentAccount(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := config.SaveState(app.ConfigPath, config.State{CurrentAccountID: fixtureAccountID}); err != nil {
		t.Fatal(err)
	}
	// No account id on the command line -> the current account is used.
	if err := app.Run([]string{"-o", "table", "transactions"}); err != nil {
		t.Fatalf("transactions (current account): %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "-194.23") {
		t.Errorf("expected the current account's statement:\n%s", out.String())
	}
}

func TestSyncUsesCurrentAccount(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	if err := config.SaveState(app.ConfigPath, config.State{CurrentAccountID: fixtureAccountID}); err != nil {
		t.Fatal(err)
	}
	if err := app.Run([]string{"sync"}); err != nil {
		t.Fatalf("sync (current account): %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "fetched") {
		t.Errorf("expected a sync summary:\n%s", out.String())
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var out, errOut bytes.Buffer
		app := &App{Stdout: &out, Stderr: &errOut}
		if err := app.Run([]string{"completion", shell}); err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
		for _, want := range []string{"openbanking", "accounts", "tx"} {
			if !strings.Contains(out.String(), want) {
				t.Errorf("completion %s missing %q:\n%s", shell, want, out.String())
			}
		}
	}

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut}
	if err := app.Run([]string{"completion", "powershell"}); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}

func TestGlobalOutputFlagNeedsValue(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut}
	if err := app.Run([]string{"-o"}); err == nil {
		t.Fatal("expected an error: -o needs a value")
	}
}

func TestGlobalOutputFlagRejectsUnknownFormat(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut}
	if err := app.Run([]string{"-o", "yaml", "accounts"}); err == nil {
		t.Fatal("expected an error for an unknown output format")
	}
}

func TestInteractiveMenuQuit(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	// Option 8 is Quit; the numbered fallback reads it and the menu exits cleanly.
	app.env = ui.Custom(strings.NewReader("8\n"), &out, &errOut, ui.FormatTable, false, true)
	if err := app.Run(nil); err != nil {
		t.Fatalf("interactive menu quit: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(errOut.String(), "openbanking") {
		t.Errorf("expected the menu header on stderr:\n%s", errOut.String())
	}
}

func TestInteractiveMenuRunsChosenCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(t, &out, &errOut)
	// Option 1 is Accounts; selecting it dispatches the accounts command.
	app.env = ui.Custom(strings.NewReader("1\n"), &out, &errOut, ui.FormatTable, false, true)
	if err := app.Run(nil); err != nil {
		t.Fatalf("interactive menu run: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Lunar") {
		t.Errorf("expected accounts output after choosing it from the menu:\n%s", out.String())
	}
}

func TestParseGlobals(t *testing.T) {
	rest, opt, err := parseGlobals([]string{"-o", "json", "transactions", "acct-1", "--no-color", "--from", "2026-01-01"})
	if err != nil {
		t.Fatal(err)
	}
	if opt.output != "json" || !opt.noColor {
		t.Errorf("globals not parsed: %+v", opt)
	}
	want := []string{"transactions", "acct-1", "--from", "2026-01-01"}
	if strings.Join(rest, " ") != strings.Join(want, " ") {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}
