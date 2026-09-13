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
)

func TestSyncSingleAccount(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"-o", "table", "sync", "11111111-1111-4111-8111-111111111111"}); err != nil {
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
	if err := app.Run([]string{"-o", "table", "sync", "--all"}); err != nil {
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

func failingSyncServer(t *testing.T) *httptest.Server {
	t.Helper()
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
	return srv
}

func TestSyncAllAsJSON_CarriesTheFailures_AndStillExitsNonZero(t *testing.T) {
	for _, format := range []string{"json", "csv"} {
		t.Run(format, func(t *testing.T) { syncAllAsDocument(t, format) })
	}
}

func syncAllAsDocument(t *testing.T, format string) {
	bundle := fixtureBundle(t)
	cfg := writeConfig(t, bundle, failingSyncServer(t).URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	err := app.Run([]string{"-o", format, "sync", "--all"})

	if err == nil {
		t.Fatal("expected a non-zero exit when accounts could not be synced")
	}
	var view struct {
		Accounts int64 `json:"accounts"`
		Failures []struct {
			AccountID     string `json:"accountId"`
			Reason        string `json:"reason"`
			BankErrorCode string `json:"bankErrorCode"`
			Hint          string `json:"hint"`
		} `json:"failures"`
	}
	if jerr := json.Unmarshal(out.Bytes(), &view); jerr != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", jerr, out.String())
	}
	if view.Accounts != 1 || len(view.Failures) != 3 {
		t.Fatalf("view = %+v", view)
	}
	f := view.Failures[1]
	if f.AccountID != "44444444-4444-4444-8444-444444444444" || f.Reason != "psu_present_required" ||
		f.BankErrorCode != "PSU_HEADER_NOT_PROVIDED" || !strings.Contains(f.Hint, "web app") {
		t.Errorf("failure = %+v", f)
	}
	if strings.Contains(errOut.String(), "psu_present_required") {
		t.Errorf("JSON mode also wrote prose failures to stderr:\n%s", errOut.String())
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
	err := app.Run([]string{"-o", "table", "sync", "--all"})

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

func TestSyncSingleAccountThatNeedsReconnect_SaysSoOnce(t *testing.T) {
	bundle := fixtureBundle(t)
	var accounts []map[string]any
	raw, _ := os.ReadFile(filepath.Join("testdata", "api", "accounts.json"))
	_ = json.Unmarshal(raw, &accounts)
	accounts[0]["needsReconnect"] = true
	accounts[0]["uidEnc"] = nil
	body, _ := json.Marshal(accounts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	err := app.Run([]string{"sync", "11111111-1111-4111-8111-111111111111"})

	if err == nil || strings.Count(err.Error(), "reconnect") != 1 {
		t.Fatalf("err = %v, want the reconnect advice exactly once", err)
	}
}

func TestSyncSingleAccountPiped_IsJSON(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	for _, format := range []string{"json", "csv"} {
		var out, errOut bytes.Buffer
		app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
		if err := app.Run([]string{"-o", format, "sync", "11111111-1111-4111-8111-111111111111"}); err != nil {
			t.Fatalf("%s: sync: %v\n%s", format, err, errOut.String())
		}
		var view struct {
			NewTransactions int64 `json:"newTransactions"`
			TotalFetched    int64 `json:"totalFetched"`
		}
		if err := json.Unmarshal(out.Bytes(), &view); err != nil || view.TotalFetched != 1 {
			t.Errorf("%s: stdout = %q (%v)", format, out.String(), err)
		}
	}
}

func TestSyncSingleAccountThrottled_SaysWhenToTryAgain(t *testing.T) {
	bundle := fixtureBundle(t)
	accounts, _ := os.ReadFile(filepath.Join("testdata", "api", "accounts.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/accounts" {
			_, _ = w.Write(accounts)
			return
		}
		w.Header().Set("Retry-After", "3599")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"status":429,"reason":"rate_limited"}`))
	}))
	t.Cleanup(srv.Close)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	err := app.Run([]string{"sync", "11111111-1111-4111-8111-111111111111"})

	if err == nil || !strings.Contains(err.Error(), "try again in about 60 minute(s)") {
		t.Fatalf("err = %v, want when to try again", err)
	}
}
