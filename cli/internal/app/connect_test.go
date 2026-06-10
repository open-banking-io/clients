package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-banking-io/clients/cli/internal/config"
)

func TestBanksCommandRendersTable(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"banks", "--country", "DK"}); err != nil {
		t.Fatalf("banks: %v\nstderr: %s", err, errOut.String())
	}
	for _, want := range []string{"Lunar", "LUNADK22", "business"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("banks output missing %q\n%s", want, out.String())
		}
	}
}

func TestConnectStartsAuthorizationAndDetectsNewConnection(t *testing.T) {
	apiKey := fixtureBundle(t).APIKey

	// The connection appears only after the user "completes consent" (the browser stub flips this).
	var consented atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/connections":
			if consented.Load() {
				_, _ = w.Write([]byte(`[{"sessionId":"new-1","aspspName":"Lunar","aspspCountry":"DK","status":"Active","accountCount":2}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/authorizations":
			_, _ = w.Write([]byte(`{"url":"https://bank.example/consent?state=abc"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "credentials.json")
	if err := config.Save(cfg, config.Bundle{APIKey: apiKey, APIBaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}

	// Stub the browser to simulate the user completing consent.
	prev := openBrowser
	openBrowser = func(target string) error {
		if target != "https://bank.example/consent?state=abc" {
			t.Errorf("opened wrong consent URL: %s", target)
		}
		consented.Store(true)
		return nil
	}
	defer func() { openBrowser = prev }()

	prevInterval := connectPollInterval
	connectPollInterval = 5 * time.Millisecond
	defer func() { connectPollInterval = prevInterval }()

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg, HTTPClient: srv.Client()}
	if err := app.Run([]string{"connect", "Lunar", "--psu-type", "business", "--timeout", "5s"}); err != nil {
		t.Fatalf("connect: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "Connected Lunar") {
		t.Errorf("expected connected message\n%s", out.String())
	}
}

func TestConnectRequiresBankName(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "credentials.json")
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"connect"}); err == nil {
		t.Fatal("expected an error when no bank name is given")
	}
}
