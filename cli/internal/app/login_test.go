package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/open-banking-io/clients/cli/internal/config"
)

// The canonical PKCE worked example from RFC 7636, Appendix B. The Go CLI must derive the exact same
// S256 challenge the .NET backend verifies, or login would never succeed.
func TestPkceChallengeMatchesRFC7636Vector(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := pkceChallenge(verifier); got != want {
		t.Errorf("pkceChallenge = %q, want %q", got, want)
	}
}

func TestPkceVerifierIsFreshAndUrlSafe(t *testing.T) {
	a, b := pkceVerifier(), pkceVerifier()
	if a == b {
		t.Error("verifiers should be random and distinct")
	}
	for _, v := range []string{a, b} {
		for _, c := range v {
			if c == '+' || c == '/' || c == '=' {
				t.Errorf("verifier %q is not url-safe", v)
			}
		}
	}
}

func TestExchangeTokenSendsCodeAndVerifier(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/cli/token" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"ebk_minted","apiBaseUrl":"https://api.example","user":"a@b.c","prefix":"ebk_minted12"}`))
	}))
	defer srv.Close()

	app := &App{HTTPClient: srv.Client()}
	token, err := app.exchangeToken(context.Background(), srv.URL, "one-time-code", "the-verifier")
	if err != nil {
		t.Fatalf("exchangeToken: %v", err)
	}
	if token.APIKey != "ebk_minted" || token.User != "a@b.c" {
		t.Errorf("unexpected token: %+v", token)
	}
	if gotBody["code"] != "one-time-code" || gotBody["codeVerifier"] != "the-verifier" {
		t.Errorf("server received wrong body: %v", gotBody)
	}
}

func TestExchangeTokenSurfacesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid or expired login code"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	app := &App{HTTPClient: srv.Client()}
	if _, err := app.exchangeToken(context.Background(), srv.URL, "bad", "v"); err == nil {
		t.Fatal("expected an error for a 400 token exchange")
	}
}

// TestLoginPreservesEncryptionKey drives the full login command against a stub server, with the
// browser-open stubbed to immediately hit the loopback /callback, and asserts a previously imported
// encryption key survives the login.
func TestLoginPreservesEncryptionKey(t *testing.T) {
	// Seed a config that already has an encryption key (as if `key import` ran first).
	cfgPath := filepath.Join(t.TempDir(), "credentials.json")
	seeded := config.Bundle{}
	seeded.EncryptionKey.PrivateKey = "existing-pkcs8-key"
	if err := config.Save(cfgPath, seeded); err != nil {
		t.Fatal(err)
	}

	// Stub API: /auth/cli/start redirects the "browser" to the loopback callback; /auth/cli/token mints.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/cli/start":
			port := r.URL.Query().Get("port")
			http.Redirect(w, r, "http://127.0.0.1:"+port+"/callback?code=test-code", http.StatusFound)
		case "/auth/cli/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apiKey":"ebk_minted","apiBaseUrl":"` + "" + `","user":"me@example.com","prefix":"ebk_minted12"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Stub the browser opener to follow the start URL like a real browser (following redirects).
	prev := openBrowser
	openBrowser = func(target string) error {
		go func() { _, _ = srv.Client().Get(target) }()
		return nil
	}
	defer func() { openBrowser = prev }()

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfgPath, HTTPClient: srv.Client()}
	if err := app.login([]string{"--api", srv.URL, "--timeout", "10s"}); err != nil {
		t.Fatalf("login: %v\nstderr: %s", err, errOut.String())
	}

	got, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got.APIKey != "ebk_minted" {
		t.Errorf("apiKey = %q, want ebk_minted", got.APIKey)
	}
	if got.EncryptionKey.PrivateKey != "existing-pkcs8-key" {
		t.Errorf("encryption key was clobbered: %q", got.EncryptionKey.PrivateKey)
	}
	if got.APIBaseURL != srv.URL {
		t.Errorf("apiBaseUrl = %q, want %q (empty server value falls back to --api)", got.APIBaseURL, srv.URL)
	}
}
