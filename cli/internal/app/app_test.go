package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	openbanking "github.com/open-banking-io/clients/go"
)

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

// startAccountsServer serves the encrypted accounts fixture for a matching API key.
func startAccountsServer(t *testing.T, apiKey string) *httptest.Server {
	t.Helper()
	accounts, err := os.ReadFile(filepath.Join("testdata", "api", "accounts.json"))
	if err != nil {
		t.Fatalf("read accounts fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != apiKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/accounts" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(accounts)
			return
		}
		http.NotFound(w, r)
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
	srv := startAccountsServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"accounts"}); err != nil {
		t.Fatalf("Run accounts: %v\nstderr: %s", err, errOut.String())
	}

	got := out.String()
	for _, want := range []string{"Drift", "DK6466952001724927", "828.13", "Lunar"} {
		if !strings.Contains(got, want) {
			t.Errorf("accounts output missing %q\n--- output ---\n%s", want, got)
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
