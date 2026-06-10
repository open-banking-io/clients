package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	ch := make(chan callbackResult, 1)
	srv := httptest.NewServer(callbackHandler(ch, "st", "https://app.example"))
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
	ch := make(chan callbackResult, 1)
	srv := httptest.NewServer(callbackHandler(ch, "st", "https://app.example"))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/callback?code=abc123")
	if err != nil {
		t.Fatalf("GET /callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := <-ch; got.code != "abc123" || got.privKeyB64 != "" {
		t.Errorf("captured = %+v, want code abc123 and no key", got)
	}
}

func TestCallbackHandlerRelaysEncryptionKey(t *testing.T) {
	ch := make(chan callbackResult, 1)
	srv := httptest.NewServer(callbackHandler(ch, "secret-state", "https://app.example"))
	defer srv.Close()

	body := `{"code":"c1","state":"secret-state","encryptionKey":{"privateKey":"PK","publicKey":"PUB"}}`
	resp, err := srv.Client().Post(srv.URL+"/callback", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Errorf("CORS origin = %q, want the app origin", got)
	}
	res := <-ch
	if res.code != "c1" || res.privKeyB64 != "PK" || res.pubKeyB64 != "PUB" {
		t.Errorf("relayed result = %+v, want code c1 + key PK/PUB", res)
	}
}

func TestCallbackHandlerRejectsWrongState(t *testing.T) {
	ch := make(chan callbackResult, 1)
	srv := httptest.NewServer(callbackHandler(ch, "right-state", "https://app.example"))
	defer srv.Close()

	body := `{"code":"c1","state":"WRONG","encryptionKey":{"privateKey":"PK"}}`
	resp, err := srv.Client().Post(srv.URL+"/callback", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for a state mismatch", resp.StatusCode)
	}
}

func TestCallbackHandlerCORSPreflight(t *testing.T) {
	ch := make(chan callbackResult, 1)
	srv := httptest.NewServer(callbackHandler(ch, "st", "https://app.example"))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/callback", nil)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /callback: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("preflight is missing Access-Control-Allow-Methods")
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
