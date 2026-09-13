package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncSingleAccount(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"sync", "11111111-1111-4111-8111-111111111111"}); err != nil {
		t.Fatalf("sync: %v\nstderr: %s", err, errOut.String())
	}
	// The fixture reports 0 new, 1 fetched.
	if !strings.Contains(out.String(), "1 fetched") {
		t.Errorf("expected fetched count in output\n%s", out.String())
	}
}

func TestSyncAll(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"sync", "--all"}); err != nil {
		t.Fatalf("sync --all: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Synced 1 account") {
		t.Errorf("expected account count in output\n%s", out.String())
	}
}

func TestSyncRequiresAccountOrAll(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"sync"}); err == nil {
		t.Fatal("expected an error when neither an account id nor --all is given")
	}
}

func TestSyncAllReportsEveryAccountItCouldNotRefresh(t *testing.T) {
	bundle := fixtureBundle(t)
	accounts, _ := os.ReadFile(filepath.Join("testdata", "api", "accounts.json"))
	failures, _ := os.ReadFile(filepath.Join("testdata", "api", "sync-all-failures.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/accounts":
			_, _ = w.Write(accounts)
		case "/api/sync":
			_, _ = w.Write(failures)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	err := app.Run([]string{"sync", "--all"})

	if err == nil || !strings.Contains(err.Error(), "3 account(s) could not be synced") {
		t.Fatalf("err = %v, want the failure count", err)
	}
	for _, want := range []string{
		"33333333-3333-4333-8333-333333333333  reconnect_needed (ASPSP_ACCOUNT_NOT_ACCESSIBLE) — reconnect",
		"44444444-4444-4444-8444-444444444444  psu_present_required (PSU_HEADER_NOT_PROVIDED) — this bank only shares data",
		"55555555-5555-4555-8555-555555555555  rate_limited — the bank is throttling",
	} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr missing %q\n%s", want, errOut.String())
		}
	}
}

func TestSyncSingleAccountRefusalCarriesAHint(t *testing.T) {
	bundle := fixtureBundle(t)
	accounts, _ := os.ReadFile(filepath.Join("testdata", "api", "accounts.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/accounts" {
			_, _ = w.Write(accounts)
			return
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"status":409,"reason":"psu_present_required"}`))
	}))
	t.Cleanup(srv.Close)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	err := app.Run([]string{"sync", "11111111-1111-4111-8111-111111111111"})

	if err == nil || !strings.Contains(err.Error(), "psu_present_required — this bank only shares data") {
		t.Fatalf("err = %v, want the reason and its hint", err)
	}
}
