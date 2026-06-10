package app

import (
	"bytes"
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
