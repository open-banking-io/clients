package app

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"time"

	"github.com/open-banking-io/clients/cli/internal/config"
)

const defaultAPIBaseURL = "https://bank.core.ci"

// cliTokenResponse is the credential returned by POST /auth/cli/token.
type cliTokenResponse struct {
	APIKey     string `json:"apiKey"`
	APIBaseURL string `json:"apiBaseUrl"`
	User       string `json:"user"`
	Prefix     string `json:"prefix"`
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

func (a *App) login(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	apiBaseURL := fs.String("api", defaultAPIBaseURL, "API base URL to log in to")
	timeout := fs.Duration("timeout", 3*time.Minute, "how long to wait for the browser login")
	if err := fs.Parse(args); err != nil {
		return err
	}
	base := trimTrailingSlash(*apiBaseURL)

	// PKCE binds this login to this process: the verifier never leaves the CLI; only its hash
	// (the challenge) goes to the server, and only the verifier can later redeem the code.
	verifier := pkceVerifier()
	challenge := pkceChallenge(verifier)

	// A loopback listener the server will redirect the browser back to with the one-time code.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("could not start local login listener: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	codeCh := make(chan string, 1)
	server := &http.Server{Handler: callbackHandler(codeCh)}
	go server.Serve(listener)
	defer server.Close()

	startURL := fmt.Sprintf("%s/auth/cli/start?port=%d&code_challenge=%s",
		base, port, url.QueryEscape(challenge))
	fmt.Fprintf(a.Stderr, "Opening your browser to sign in...\nIf it doesn't open, visit:\n  %s\n\n", startURL)
	if err := openBrowser(startURL); err != nil {
		fmt.Fprintf(a.Stderr, "(could not open a browser automatically: %v)\n", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for the browser login after %s", *timeout)
	}

	token, err := a.exchangeToken(ctx, base, code, verifier)
	if err != nil {
		return err
	}

	// Preserve any encryption key already imported; login only owns the API credential.
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
	if err := config.Save(a.ConfigPath, bundle); err != nil {
		return err
	}

	fmt.Fprintf(a.Stdout, "Logged in as %s. Credentials saved to %s\n", token.User, a.ConfigPath)
	if bundle.EncryptionKey.PrivateKey == "" {
		fmt.Fprintln(a.Stdout, "Next: run `obank key import` to add your encryption key so accounts can be decrypted.")
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

// callbackHandler serves the loopback /callback the server redirects to, capturing the one-time code.
func callbackHandler(codeCh chan<- string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><meta charset=utf-8>
<title>obank</title><body style="font-family:system-ui;padding:3rem">
<h2>Login complete</h2><p>You can close this tab and return to your terminal.</p></body>`)
		select {
		case codeCh <- code:
		default:
		}
	})
	return mux
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
