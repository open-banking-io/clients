package app

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/open-banking-io/clients/cli/internal/config"
)

const defaultAPIBaseURL = "https://open-banking.io"

// cliTokenResponse is the credential returned by POST /auth/cli/token.
type cliTokenResponse struct {
	APIKey     string `json:"apiKey"`
	APIBaseURL string `json:"apiBaseUrl"`
	User       string `json:"user"`
	Prefix     string `json:"prefix"`
}

// relayPayload is what the browser POSTs to the loopback after the user unlocks their key in the UI.
// The encryption private key travels browser -> localhost only; it never touches the server.
type relayPayload struct {
	Code          string `json:"code"`
	State         string `json:"state"`
	EncryptionKey *struct {
		PrivateKey string `json:"privateKey"`
		PublicKey  string `json:"publicKey"`
	} `json:"encryptionKey"`
}

// callbackResult is delivered from the loopback to login(): the one-time code, and (when the browser
// relayed it) the unlocked encryption key material.
type callbackResult struct {
	code       string
	privKeyB64 string // empty if the browser didn't relay a key (key-less fallback)
	pubKeyB64  string
}

// openBrowser is a package var so tests can stub it. It best-effort opens a URL in the user's browser.
var openBrowser = func(target string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

// loginFlags are the values `login` binds. Registration is split out of login() so the defaults can
// be asserted on their own — the browser wait in particular is a real UX budget, not an arbitrary
// number, and it is otherwise unreachable from a test without running a whole login.
type loginFlags struct {
	apiBaseURL *string
	timeout    *time.Duration
	method     *string
}

func registerLoginFlags(fs *flag.FlagSet) loginFlags {
	return loginFlags{
		apiBaseURL: fs.String("api", defaultAPIBaseURL, "API base URL to log in to"),
		// Sized for the emailed-code path: delivery, then finding the mail, then typing six digits.
		// GitHub used to be the only option and took seconds, which is what the old 3 minutes fit.
		timeout: fs.Duration("timeout", 10*time.Minute, "how long to wait for the browser login"),
		method:  fs.String("method", "", "sign-in method to go straight to: pin or github (default: pick in the browser)"),
	}
}

func (a *App) login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	flags := registerLoginFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	apiBaseURL, timeout := flags.apiBaseURL, flags.timeout

	// The server treats an unrecognised method as absent — it is a UX hint, not a security control —
	// so a typo would quietly give the default rather than failing. Catch it here, before a browser
	// opens and a listener is left waiting on a login that isn't the one that was asked for.
	method := *flags.method
	if method != "" && method != "pin" && method != "github" {
		return fmt.Errorf("unknown --method %q (use pin or github, or omit it to pick in the browser)", method)
	}

	base := trimTrailingSlash(*apiBaseURL)

	// PKCE binds this login to this process: the verifier never leaves the CLI; only its hash
	// (the challenge) goes to the server, and only the verifier can later redeem the code.
	verifier := pkceVerifier()
	challenge := pkceChallenge(verifier)
	// state binds the browser's relay POST to this exact CLI session.
	state := pkceVerifier()

	// A loopback listener the browser (or the server redirect) reaches with the code + relayed key.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("could not start local login listener: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	resultCh := make(chan callbackResult, 1)
	server := &http.Server{Handler: callbackHandler(resultCh, state, base)}
	go server.Serve(listener)
	defer server.Close()

	startURL := fmt.Sprintf("%s/auth/cli/start?port=%d&code_challenge=%s&state=%s",
		base, port, url.QueryEscape(challenge), url.QueryEscape(state))
	// Omitted unless asked for: with no method the server sends you to the login page, which offers
	// the emailed code and GitHub side by side. Older servers ignore the parameter outright.
	if method != "" {
		startURL += "&method=" + url.QueryEscape(method)
	}
	fmt.Fprintf(a.Stderr, "Opening your browser to sign in...\nIf it doesn't open, visit:\n  %s\n\n", startURL)
	if err := openBrowser(startURL); err != nil {
		fmt.Fprintf(a.Stderr, "(could not open a browser automatically: %v)\n", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	stopSpinner := a.ui().Spinner("Waiting for you to finish signing in…")
	var result callbackResult
	select {
	case result = <-resultCh:
	case <-ctx.Done():
		stopSpinner()
		return fmt.Errorf("timed out waiting for the browser login after %s", *timeout)
	}
	stopSpinner()

	token, err := a.exchangeToken(ctx, base, result.code, verifier)
	if err != nil {
		return err
	}

	// Preserve any encryption key already imported; we overwrite it only if the browser relayed one.
	bundle, err := config.Load(a.ConfigPath)
	if err != nil {
		bundle = config.Bundle{} // first login: start fresh
	}
	bundle.Service = "open-banking.io"
	if token.APIBaseURL != "" {
		bundle.APIBaseURL = token.APIBaseURL
	} else {
		bundle.APIBaseURL = base
	}
	bundle.User = token.User
	bundle.APIKey = token.APIKey

	if result.privKeyB64 != "" {
		if err := validateP256Pkcs8(result.privKeyB64); err != nil {
			return fmt.Errorf("the browser relayed an invalid encryption key: %w", err)
		}
		bundle.EncryptionKey = encryptionKeyFor(result.privKeyB64)
		bundle.EncryptionKey.PublicKey = result.pubKeyB64
	}

	if err := config.Save(a.ConfigPath, bundle); err != nil {
		return err
	}

	if bundle.EncryptionKey.PrivateKey != "" {
		fmt.Fprintf(a.Stdout, "Logged in as %s — encryption key set up. You're ready to go.\nCredentials saved to %s\n", token.User, a.ConfigPath)
	} else {
		fmt.Fprintf(a.Stdout, "Logged in as %s. Credentials saved to %s\n", token.User, a.ConfigPath)
		fmt.Fprintln(a.Stdout, "Next: run `openbanking key import` to add your encryption key so accounts can be decrypted.")
	}
	return nil
}

// exchangeToken redeems the one-time code (proving the PKCE verifier) for an API key.
func (a *App) exchangeToken(ctx context.Context, apiBaseURL, code, verifier string) (cliTokenResponse, error) {
	payload, _ := json.Marshal(map[string]string{"code": code, "codeVerifier": verifier})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBaseURL+"/auth/cli/token", bytes.NewReader(payload))
	if err != nil {
		return cliTokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Identify the CLI (and its version) to the server; without this the raw request would go out as
	// Go's default Go-http-client/1.1 with no product/version. versionString defaults to "dev".
	req.Header.Set("User-Agent", "open-banking-io/cli/"+a.versionString())

	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return cliTokenResponse{}, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return cliTokenResponse{}, fmt.Errorf("token exchange failed: %s", resp.Status)
	}
	var token cliTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return cliTokenResponse{}, fmt.Errorf("could not decode token response: %w", err)
	}
	if token.APIKey == "" {
		return cliTokenResponse{}, fmt.Errorf("token exchange returned no API key")
	}
	return token, nil
}

// callbackHandler serves the loopback /callback. The browser authorize page POSTs {code,state,
// encryptionKey} after unlocking the key (CORS-scoped to the app origin); the legacy GET ?code= path
// remains as a key-less fallback.
func callbackHandler(resultCh chan<- callbackResult, state, allowOrigin string) http.Handler {
	deliver := func(res callbackResult) {
		select {
		case resultCh <- res:
		default:
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Set("Vary", "Origin")
		switch r.Method {
		case http.MethodOptions:
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "content-type")
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			code := r.URL.Query().Get("code")
			if code == "" {
				http.Error(w, "missing code", http.StatusBadRequest)
				return
			}
			writeDoneHTML(w)
			deliver(callbackResult{code: code})
		case http.MethodPost:
			code, gotState, privKey, pubKey := parseRelayPost(r)
			if code == "" {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if subtle.ConstantTimeCompare([]byte(gotState), []byte(state)) != 1 {
				http.Error(w, "state mismatch", http.StatusForbidden)
				return
			}
			// The browser reaches us via a cross-origin form POST (no CORS preflight, not blocked by
			// mixed-content rules) and navigates here, so reply with the closeable success page.
			writeDoneHTML(w)
			deliver(callbackResult{code: code, privKeyB64: privKey, pubKeyB64: pubKey})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	return mux
}

// parseRelayPost reads the relayed credentials from either a browser form POST (the default — it needs
// no CORS preflight and isn't blocked by mixed-content rules) or a JSON body.
func parseRelayPost(r *http.Request) (code, state, privKey, pubKey string) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var p relayPayload
		if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&p) == nil {
			code, state = p.Code, p.State
			if p.EncryptionKey != nil {
				privKey, pubKey = p.EncryptionKey.PrivateKey, p.EncryptionKey.PublicKey
			}
		}
		return
	}
	_ = r.ParseForm()
	return r.PostFormValue("code"), r.PostFormValue("state"),
		r.PostFormValue("privateKey"), r.PostFormValue("publicKey")
}

func writeDoneHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, `<!doctype html><meta charset=utf-8>
<title>open-banking.io CLI</title><body style="font-family:system-ui;padding:3rem">
<h2>Login complete</h2><p>You can close this tab and return to your terminal.</p></body>`)
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
