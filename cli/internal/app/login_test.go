package app

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
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

func TestExchangeTokenSendsVersionedUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"ebk_minted","apiBaseUrl":"https://api.example","user":"a@b.c","prefix":"ebk_minted12"}`))
	}))
	defer srv.Close()

	app := &App{HTTPClient: srv.Client(), Version: "1.2.3"}
	if _, err := app.exchangeToken(context.Background(), srv.URL, "code", "verifier"); err != nil {
		t.Fatalf("exchangeToken: %v", err)
	}
	if want := "open-banking-io/cli/1.2.3"; gotUA != want {
		t.Errorf("User-Agent = %q, want %q", gotUA, want)
	}
}

func TestExchangeTokenUserAgentDefaultsToDev(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"ebk_minted","apiBaseUrl":"https://api.example","user":"a@b.c","prefix":"ebk_minted12"}`))
	}))
	defer srv.Close()

	app := &App{HTTPClient: srv.Client()} // no Version set
	if _, err := app.exchangeToken(context.Background(), srv.URL, "code", "verifier"); err != nil {
		t.Fatalf("exchangeToken: %v", err)
	}
	if want := "open-banking-io/cli/dev"; gotUA != want {
		t.Errorf("User-Agent = %q, want %q", gotUA, want)
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

// TestLoginRelaysEncryptionKeyFromBrowser drives the one-step flow: the stubbed browser plays the
// authorize page — it reads the loopback port + state from the start URL and POSTs the unlocked key
// to the loopback. login() must end up with BOTH the API key and the relayed encryption key, with no
// manual `key import`.
func TestLoginRelaysEncryptionKeyFromBrowser(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "credentials.json")
	fb := fixtureBundle(t)
	validKey, validPub := fb.EncryptionKey.PrivateKey, fb.EncryptionKey.PublicKey

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/cli/token" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apiKey":"ebk_minted","user":"me@example.com","prefix":"ebk_minted12"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	prev := openBrowser
	openBrowser = func(target string) error {
		u, err := url.Parse(target)
		if err != nil {
			return err
		}
		port, state := u.Query().Get("port"), u.Query().Get("state")
		body, _ := json.Marshal(map[string]any{
			"code":          "relay-code",
			"state":         state,
			"encryptionKey": map[string]string{"privateKey": validKey, "publicKey": validPub},
		})
		go func() {
			_, _ = http.Post("http://127.0.0.1:"+port+"/callback", "application/json", bytes.NewReader(body))
		}()
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
	if got.EncryptionKey.PrivateKey != validKey {
		t.Error("the relayed encryption private key was not saved")
	}
	if got.EncryptionKey.PublicKey != validPub {
		t.Error("the relayed public key was not saved")
	}
	if !strings.Contains(out.String(), "encryption key set up") {
		t.Errorf("expected the one-step success message, got: %q", out.String())
	}
}

// captureStartURL runs `login` with args, stubbing the browser so it records the URL the CLI would
// open and then lets the flow lapse. Returns that URL's query. The login error is discarded: with
// nothing answering the loopback it always times out, and only the request matters here.
func captureStartURL(t *testing.T, args ...string) url.Values {
	t.Helper()
	var got string
	prev := openBrowser
	openBrowser = func(target string) error { got = target; return nil }
	defer func() { openBrowser = prev }()

	var out, errOut bytes.Buffer
	app := &App{
		Stdout:     &out,
		Stderr:     &errOut,
		ConfigPath: filepath.Join(t.TempDir(), "credentials.json"),
	}
	_ = app.login(append([]string{"--api", "https://example.invalid", "--timeout", "1ms"}, args...))

	if got == "" {
		t.Fatal("no start URL was opened")
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse start URL %q: %v", got, err)
	}
	return u.Query()
}

// An absent `method` means "let the login page offer both". Released binaries send nothing at all,
// so the default must stay exactly that — and the rest of the URL contract must not drift either.
func TestLoginOmitsMethodByDefault(t *testing.T) {
	q := captureStartURL(t)

	if q.Has("method") {
		t.Errorf("method should be absent by default, got %q", q.Get("method"))
	}
	for _, key := range []string{"port", "code_challenge", "state"} {
		if q.Get(key) == "" {
			t.Errorf("start URL is missing %s: %v", key, q)
		}
	}
}

func TestLoginThreadsMethodIntoTheStartURL(t *testing.T) {
	for _, method := range []string{"pin", "github"} {
		t.Run(method, func(t *testing.T) {
			if got := captureStartURL(t, "--method", method).Get("method"); got != method {
				t.Errorf("method = %q, want %q", got, method)
			}
		})
	}
}

// The server silently drops an unrecognised method — it is a UX hint, not a security control — so a
// typo would otherwise appear to work while quietly giving the default. Fail here instead, before
// a browser is opened and before a listener is left waiting.
func TestLoginRejectsUnknownMethodBeforeOpeningABrowser(t *testing.T) {
	opened := false
	prev := openBrowser
	openBrowser = func(string) error { opened = true; return nil }
	defer func() { openBrowser = prev }()

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "credentials.json")}
	err := app.login([]string{"--api", "https://example.invalid", "--method", "banana"})

	if err == nil {
		t.Fatal("expected an error for an unknown --method")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("the error should name the bad value, got: %v", err)
	}
	if opened {
		t.Error("no browser should be opened for an invalid --method")
	}
}

// Signing in now routinely means waiting on an emailed 6-digit code — delivery, finding it, typing
// it. The old 3-minute budget was sized for a GitHub round-trip that took seconds.
func TestLoginBrowserWaitDefaultsToTenMinutes(t *testing.T) {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerLoginFlags(fs)

	if got := fs.Lookup("timeout").DefValue; got != "10m0s" {
		t.Errorf("default browser wait = %s, want 10m0s", got)
	}
}
