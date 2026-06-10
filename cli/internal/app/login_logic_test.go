package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrimTrailingSlash(t *testing.T) {
	cases := map[string]string{
		"https://x":     "https://x",
		"https://x/":    "https://x",
		"https://x///":  "https://x",
		"":              "",
		"/":             "",
		"no-slash-here": "no-slash-here",
	}
	for in, want := range cases {
		if got := trimTrailingSlash(in); got != want {
			t.Errorf("trimTrailingSlash(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCallbackHandlerRejectsMissingCode(t *testing.T) {
	codeCh := make(chan string, 1)
	srv := httptest.NewServer(callbackHandler(codeCh))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/callback") // no ?code
	if err != nil {
		t.Fatalf("GET /callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for a missing code", resp.StatusCode)
	}
}

func TestCallbackHandlerCapturesCode(t *testing.T) {
	codeCh := make(chan string, 1)
	srv := httptest.NewServer(callbackHandler(codeCh))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/callback?code=abc123")
	if err != nil {
		t.Fatalf("GET /callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := <-codeCh; got != "abc123" {
		t.Errorf("captured code = %q, want abc123", got)
	}
}

func TestExchangeTokenRejectsResponseWithoutAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":"a@b.c"}`)) // no apiKey
	}))
	defer srv.Close()

	app := &App{HTTPClient: srv.Client()}
	if _, err := app.exchangeToken(context.Background(), srv.URL, "code", "verifier"); err == nil {
		t.Fatal("expected an error when the token response has no apiKey")
	}
}

func TestExchangeTokenTransportError(t *testing.T) {
	app := &App{HTTPClient: http.DefaultClient}
	// Closed server: the POST connection is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if _, err := app.exchangeToken(context.Background(), url, "code", "verifier"); err == nil {
		t.Fatal("expected a transport error against a closed server")
	}
}
