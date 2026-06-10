package openbanking

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

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
