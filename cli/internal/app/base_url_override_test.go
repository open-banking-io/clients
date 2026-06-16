package app

import (
	"bytes"
	"strings"
	"testing"
)

// TestAPIBaseURLEnvRedirectsClient verifies OPENBANKING_API_BASE_URL overrides the saved bundle's
// API base URL, so a command targets staging/local without re-running `login`. The bundle is pinned
// at a dead address; only the env override (with a trailing slash, to exercise trimming) can reach
// the live test server.
func TestAPIBaseURLEnvRedirectsClient(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, "http://127.0.0.1:1")

	t.Setenv(apiBaseURLEnv, srv.URL+"/")

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"-o", "table", "accounts"}); err != nil {
		t.Fatalf("Run accounts with base URL override: %v\nstderr: %s", err, errOut.String())
	}
	if got := out.String(); !strings.Contains(got, "DK6466952001724927") {
		t.Errorf("override server output missing expected account\n--- output ---\n%s", got)
	}
}

// TestAPIBaseURLEnvBlankKeepsBundle confirms a blank/whitespace override leaves the bundle's URL
// untouched, so the env var is opt-in and never silently breaks the default flow.
func TestAPIBaseURLEnvBlankKeepsBundle(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)
	cfg := writeConfig(t, bundle, srv.URL)

	t.Setenv(apiBaseURLEnv, "   ")

	app := &App{ConfigPath: cfg}
	got, err := app.loadBundle()
	if err != nil {
		t.Fatalf("loadBundle: %v", err)
	}
	if got.APIBaseURL != srv.URL {
		t.Errorf("blank override changed base URL: got %q, want %q", got.APIBaseURL, srv.URL)
	}
}
