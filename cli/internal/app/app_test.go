package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	openbanking "github.com/open-banking-io/clients/go"
)

var transactionsPath = regexp.MustCompile(`^/api/accounts/[^/]+/transactions$`)
var accountSyncPath = regexp.MustCompile(`^/api/accounts/[^/]+/sync$`)

// fixtureBundle reads the shared test credentials bundle.
func fixtureBundle(t *testing.T) openbanking.CredentialsBundle {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "credentials.json"))
	if err != nil {
		t.Fatalf("read fixture credentials: %v", err)
	}
	var b openbanking.CredentialsBundle
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("parse fixture credentials: %v", err)
	}
	return b
}

// startAPIServer serves the encrypted fixtures (accounts, transactions, connections) for a
// matching API key, mirroring the real API's read endpoints.
func startAPIServer(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	fixture := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", "api", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		return data
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != apiKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/accounts":
			_, _ = w.Write(fixture("accounts.json"))
		case r.Method == http.MethodGet && transactionsPath.MatchString(r.URL.Path):
			_, _ = w.Write(fixture("transactions.json"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/connections":
			_, _ = w.Write(fixture("connections.json"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/aspsps":
			_, _ = w.Write([]byte(`[{"name":"Lunar","country":"` + r.URL.Query().Get("country") + `","bic":"LUNADK22","beta":false,"psuTypes":["business"]}]`))
		case r.Method == http.MethodPost && accountSyncPath.MatchString(r.URL.Path):
			_, _ = w.Write(fixture("sync.json"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/sync":
			_, _ = w.Write(fixture("sync-all.json"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeConfig writes the fixture bundle (with the API base url pointed at srv) to a temp file.
func writeConfig(t *testing.T, b openbanking.CredentialsBundle, baseURL string) string {
	t.Helper()
	b.APIBaseURL = baseURL
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestAccountsCommandRendersDecryptedTable(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	// Pin the table format: a captured buffer is not a terminal, so the default would be json.
	if err := app.Run([]string{"-o", "table", "accounts"}); err != nil {
		t.Fatalf("Run accounts: %v\nstderr: %s", err, errOut.String())
	}

	got := out.String()
	for _, want := range []string{"Drift", "DK6466952001724927", "828.13", "Lunar"} {
		if !strings.Contains(got, want) {
			t.Errorf("accounts output missing %q\n--- output ---\n%s", want, got)
		}
	}
}

func TestTransactionsCommandRendersSignedAmountAndCounterparty(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"-o", "table", "transactions", "11111111-1111-4111-8111-111111111111"}); err != nil {
		t.Fatalf("Run transactions: %v\nstderr: %s", err, errOut.String())
	}

	got := out.String()
	// The fixture transaction is a DBIT of 194.23 to One.com: the sign is applied at the
	// presentation edge (DBIT negative) and the creditor is the counterparty.
	if !strings.Contains(got, "-194.23") {
		t.Errorf("expected DBIT amount rendered as -194.23\n--- output ---\n%s", got)
	}
	if !strings.Contains(got, "One.com") {
		t.Errorf("expected counterparty One.com\n--- output ---\n%s", got)
	}
	if !strings.Contains(got, "2026-06-08") {
		t.Errorf("expected booking date 2026-06-08\n--- output ---\n%s", got)
	}
}

func TestTransactionsCommandRequiresAccountID(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"transactions"}); err == nil {
		t.Fatal("expected an error when no account id is given")
	}
}

func TestConnectionsCommandRendersTable(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"-o", "table", "connections"}); err != nil {
		t.Fatalf("Run connections: %v\nstderr: %s", err, errOut.String())
	}

	got := out.String()
	for _, want := range []string{"Lunar", "Active", "business"} {
		if !strings.Contains(got, want) {
			t.Errorf("connections output missing %q\n--- output ---\n%s", want, got)
		}
	}
}

func TestAccountsCommandFailsWithoutConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "missing.json")}
	if err := app.Run([]string{"accounts"}); err == nil {
		t.Fatal("expected an error when the credentials file is missing")
	}
}

func TestUnknownCommandErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "x.json")}
	if err := app.Run([]string{"frobnicate"}); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
}

// Connecting a bank needs an interactive consent redirect, so the CLI has no `connect` command.
func TestConnectCommandIsRemoved(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "x.json")}
	err := app.Run([]string{"connect", "some-bank"})
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("connect should be an unknown command, got: %v", err)
	}
}
