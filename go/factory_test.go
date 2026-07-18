package openbanking

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// recordingTransport is an http.RoundTripper that records the last request it saw and delegates
// to a wrapped RoundTripper for the actual round trip.
type recordingTransport struct {
	inner http.RoundTripper
	last  *http.Request
	calls int
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.last = req
	rt.calls++
	inner := rt.inner
	if inner == nil {
		inner = http.DefaultTransport
	}
	return inner.RoundTrip(req)
}

func TestNewWithOptionsDefaults(t *testing.T) {
	m := startMock(t)
	c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey)
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	// With no options it mirrors New's default: a 30s-timeout http.Client.
	if c.httpClient == nil {
		t.Fatal("expected a default http client")
	}
	if c.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected default 30s timeout, got %v", c.httpClient.Timeout)
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestNewWithOptionsWithHTTPClient(t *testing.T) {
	m := startMock(t)
	hc := &http.Client{}
	c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey, WithHTTPClient(hc))
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if c.httpClient != hc {
		t.Error("WithHTTPClient client was not used")
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestNewWithOptionsWithTransport(t *testing.T) {
	m := startMock(t)
	rt := &recordingTransport{}
	c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey, WithTransport(rt))
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if c.httpClient.Transport != rt {
		t.Error("WithTransport did not set the RoundTripper on the client")
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if rt.calls == 0 || rt.last == nil {
		t.Fatal("injected transport was not exercised")
	}
	// User-Agent behavior is preserved through the injected transport.
	if got := rt.last.Header.Get("User-Agent"); got != "open-banking-io/go/"+Version {
		t.Errorf("unexpected User-Agent: %q", got)
	}
}

func TestNewWithOptionsWithTimeout(t *testing.T) {
	m := startMock(t)
	c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey, WithTimeout(7*time.Second))
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if c.httpClient.Timeout != 7*time.Second {
		t.Errorf("expected 7s timeout, got %v", c.httpClient.Timeout)
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestNewWithOptionsTransportAndTimeoutOnProvidedClient(t *testing.T) {
	m := startMock(t)
	hc := &http.Client{Timeout: 99 * time.Second}
	rt := &recordingTransport{}
	// WithTransport and WithTimeout mutate the caller-provided client (last-applied wins).
	c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey,
		WithHTTPClient(hc), WithTransport(rt), WithTimeout(3*time.Second))
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	if c.httpClient != hc {
		t.Fatal("expected provided client to be used")
	}
	if hc.Transport != rt {
		t.Error("WithTransport did not override the provided client's transport")
	}
	if hc.Timeout != 3*time.Second {
		t.Errorf("expected timeout override to 3s, got %v", hc.Timeout)
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if rt.calls == 0 {
		t.Error("injected transport was not exercised")
	}
}

// bundleJSON builds a minimal credentials-bundle JSON document pointing at baseURL.
func bundleJSON(baseURL, apiKey, privateKey string) string {
	return `{"apiBaseUrl":"` + baseURL + `","apiKey":"` + apiKey +
		`","encryptionKey":{"privateKey":"` + privateKey + `"}}`
}

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name, base, key, priv string
	}{
		{"blank base url", "  ", "k", "p"},
		{"blank api key", "https://x", "", "p"},
		{"blank private key", "https://x", "k", "  "},
		{"invalid private key", "https://x", "k", "not!base64!!"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := New(c.base, c.key, c.priv, nil); err == nil {
				t.Errorf("expected error for %s", c.name)
			}
		})
	}
}

func TestNewWithInjectedHTTPClient(t *testing.T) {
	m := startMock(t)
	// A non-nil client must be used as-is (rather than the 30s-timeout default).
	hc := &http.Client{}
	c, err := New(m.server.URL, m.apiKey, m.privateKey, hc)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.httpClient != hc {
		t.Error("injected http client was not used")
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestNewPublicValidation(t *testing.T) {
	if _, err := NewPublic("", "k", nil); err == nil {
		t.Error("expected error for blank base url")
	}
	if _, err := NewPublic("https://x", "  ", nil); err == nil {
		t.Error("expected error for blank api key")
	}
}

func TestFromBundleValidation(t *testing.T) {
	if _, err := FromBundle(CredentialsBundle{}, nil); err == nil {
		t.Error("expected error for bundle without apiKey")
	}
	noKey := CredentialsBundle{APIBaseURL: "https://x", APIKey: "k"}
	if _, err := FromBundle(noKey, nil); err == nil {
		t.Error("expected error for bundle without encryption private key")
	}
}

func TestFromBundleBuildsUsableClient(t *testing.T) {
	m := startMock(t)
	bundle := CredentialsBundle{
		APIBaseURL: m.server.URL,
		APIKey:     m.apiKey,
	}
	bundle.EncryptionKey.PrivateKey = m.privateKey
	c, err := FromBundle(bundle, nil)
	if err != nil {
		t.Fatalf("FromBundle: %v", err)
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestFromCredentialsAcceptsJSONString(t *testing.T) {
	m := startMock(t)
	c, err := FromCredentials(bundleJSON(m.server.URL, m.apiKey, m.privateKey), nil)
	if err != nil {
		t.Fatalf("FromCredentials: %v", err)
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestFromCredentialsReadsFromFile(t *testing.T) {
	m := startMock(t)
	path := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(path, []byte(bundleJSON(m.server.URL, m.apiKey, m.privateKey)), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	c, err := FromCredentials(path, nil)
	if err != nil {
		t.Fatalf("FromCredentials: %v", err)
	}
	if _, err := c.GetAccounts(); err != nil {
		t.Errorf("GetAccounts: %v", err)
	}
}

func TestFromCredentialsMissingFile(t *testing.T) {
	if _, err := FromCredentials(filepath.Join(t.TempDir(), "nope.json"), nil); err == nil {
		t.Error("expected error for missing credentials file")
	}
}

func TestFromCredentialsMalformedJSON(t *testing.T) {
	if _, err := FromCredentials("{ not valid json", nil); err == nil {
		t.Error("expected error for malformed bundle JSON")
	}
}
