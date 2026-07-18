package app

import (
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildHTTPClientNoOptsReturnsNil(t *testing.T) {
	client, err := BuildHTTPClient(0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil {
		t.Fatalf("expected nil client (SDK default) when nothing is set, got %#v", client)
	}
	// Whitespace-only CA path is treated as unset.
	client, err = BuildHTTPClient(0, "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil {
		t.Fatalf("expected nil client for blank CA path, got %#v", client)
	}
}

func TestBuildHTTPClientTimeoutOnly(t *testing.T) {
	client, err := BuildHTTPClient(30*time.Second, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected a client when a timeout is set")
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %v, want 30s", client.Timeout)
	}
	if client.Transport != nil {
		t.Fatalf("expected nil Transport (default) when only a timeout is set, got %#v", client.Transport)
	}
}

func TestBuildHTTPClientBadCAPath(t *testing.T) {
	if _, err := BuildHTTPClient(0, filepath.Join(t.TempDir(), "does-not-exist.pem")); err == nil {
		t.Fatal("expected an error for a missing CA file")
	}
}

func TestBuildHTTPClientInvalidCAContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildHTTPClient(0, path); err == nil {
		t.Fatal("expected an error when the PEM file has no certificates")
	}
}

// TestBuildHTTPClientCustomCATrusts verifies the custom root CA is actually wired
// into the client's transport: a TLS server signed only by that CA is reachable
// through the built client (and, as a sanity check, not through the default one).
func TestBuildHTTPClientCustomCATrusts(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "ok")
	}))
	defer srv.Close()

	// Write the server's self-signed cert out as the PEM the CLI would be pointed at.
	certPath := filepath.Join(t.TempDir(), "ca.pem")
	block := &pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := BuildHTTPClient(5*time.Second, certPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil || client.Transport == nil {
		t.Fatal("expected a client with a custom transport")
	}
	if client.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v, want 5s", client.Timeout)
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("custom-CA client should trust the server: %v", err)
	}
	resp.Body.Close()

	// Sanity: the default client (system roots only) must not trust this cert.
	if _, err := http.DefaultClient.Get(srv.URL); err == nil {
		t.Fatal("expected the default client to reject the untrusted server cert")
	}
}
