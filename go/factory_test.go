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

// TestNewWithOptions exhaustively exercises every option path and ordering branch of
// NewWithOptions. Each case builds the client, then asserts on the actual constructed
// *http.Client (c.httpClient, observable in-package) — pointer identity, Transport, and
// Timeout — encoding the "options apply in order, last-wins across all types" contract.
func TestNewWithOptions(t *testing.T) {
	m := startMock(t)

	// Reusable clients/transports. Each case picks fresh ones where identity matters.
	newHC := func(timeout time.Duration, rt http.RoundTripper) *http.Client {
		return &http.Client{Timeout: timeout, Transport: rt}
	}

	cases := []struct {
		name string
		// build is called fresh per case so each gets its own option/client instances; it
		// returns the opts plus the specific objects the case asserts identity against.
		// wantClient is nil when a default client is expected (only its fields are checked).
		build func() (opts []Option, wantClient *http.Client, wantTransport http.RoundTripper, wantTimeout time.Duration)
	}{
		{
			// 1. No options → default client, 30s timeout, nil transport.
			name: "no options -> default",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				return nil, nil, nil, 30 * time.Second
			},
		},
		{
			// 2. WithHTTPClient(hc) alone → that exact client, untouched.
			name: "WithHTTPClient only",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				hc := newHC(11*time.Second, nil)
				return []Option{WithHTTPClient(hc)}, hc, nil, 11 * time.Second
			},
		},
		{
			// 3. WithHTTPClient(nil) → ignored, falls back to default client.
			name: "WithHTTPClient nil ignored",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				return []Option{WithHTTPClient(nil)}, nil, nil, 30 * time.Second
			},
		},
		{
			// 5. WithTimeout(d) alone → default client with Timeout=d.
			name: "WithTimeout only",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				return []Option{WithTimeout(7 * time.Second)}, nil, nil, 7 * time.Second
			},
		},
		{
			// 6. WithTimeout(0) → explicit zero honored.
			name: "WithTimeout zero honored",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				return []Option{WithTimeout(0)}, nil, nil, 0
			},
		},
		{
			// 7. Order: WithHTTPClient(hc), WithTransport(rt) → hc.Transport == rt.
			name: "client then transport",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				hc := newHC(5*time.Second, nil)
				rt := &recordingTransport{}
				return []Option{WithHTTPClient(hc), WithTransport(rt)}, hc, rt, 5 * time.Second
			},
		},
		{
			// 8. Order: WithTransport(rt), WithHTTPClient(hc) → final client == hc, rt discarded.
			name: "transport then client discards transport",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				rt := &recordingTransport{}
				hc := newHC(5*time.Second, nil) // hc's own (nil) transport must win
				return []Option{WithTransport(rt), WithHTTPClient(hc)}, hc, nil, 5 * time.Second
			},
		},
		{
			// 9. Order: WithTimeout(d1), WithHTTPClient(hc) → hc with hc's own timeout.
			name: "timeout then client uses client timeout",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				hc := newHC(13*time.Second, nil)
				return []Option{WithTimeout(2 * time.Second), WithHTTPClient(hc)}, hc, nil, 13 * time.Second
			},
		},
		{
			// 10. Order: WithHTTPClient(hc), WithTimeout(d2) → hc.Timeout == d2.
			name: "client then timeout overrides",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				hc := newHC(13*time.Second, nil)
				return []Option{WithHTTPClient(hc), WithTimeout(4 * time.Second)}, hc, nil, 4 * time.Second
			},
		},
		{
			// 11a. Last-wins within type: two WithTimeout.
			name: "two WithTimeout last wins",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				return []Option{WithTimeout(1 * time.Second), WithTimeout(9 * time.Second)}, nil, nil, 9 * time.Second
			},
		},
		{
			// 11b. Last-wins within type: two WithTransport.
			name: "two WithTransport last wins",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				rt1 := &recordingTransport{}
				rt2 := &recordingTransport{}
				return []Option{WithTransport(rt1), WithTransport(rt2)}, nil, rt2, 30 * time.Second
			},
		},
		{
			// 11c. Last-wins within type: two WithHTTPClient.
			name: "two WithHTTPClient last wins",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				hc1 := newHC(1*time.Second, nil)
				hc2 := newHC(8*time.Second, nil)
				return []Option{WithHTTPClient(hc1), WithHTTPClient(hc2)}, hc2, nil, 8 * time.Second
			},
		},
		{
			// 12. nil option in the variadic slice is skipped safely.
			name: "nil option skipped",
			build: func() ([]Option, *http.Client, http.RoundTripper, time.Duration) {
				return []Option{nil, WithTimeout(6 * time.Second), nil}, nil, nil, 6 * time.Second
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts, wantClient, wantTransport, wantTimeout := tc.build()
			c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey, opts...)
			if err != nil {
				t.Fatalf("NewWithOptions: %v", err)
			}
			if c.httpClient == nil {
				t.Fatal("client has no http client")
			}
			if wantClient != nil && c.httpClient != wantClient {
				t.Errorf("expected the supplied *http.Client (%p), got %p", wantClient, c.httpClient)
			}
			if c.httpClient.Transport != wantTransport {
				t.Errorf("Transport = %v, want %v", c.httpClient.Transport, wantTransport)
			}
			if c.httpClient.Timeout != wantTimeout {
				t.Errorf("Timeout = %v, want %v", c.httpClient.Timeout, wantTimeout)
			}
		})
	}
}

// TestNewWithOptionsTransportExercised confirms an injected transport actually handles the
// request (cases 4 & 7) and that the User-Agent is still emitted through it.
func TestNewWithOptionsTransportExercised(t *testing.T) {
	m := startMock(t)

	t.Run("transport only on default client", func(t *testing.T) {
		rt := &recordingTransport{}
		c, err := NewWithOptions(m.server.URL, m.apiKey, m.privateKey, WithTransport(rt))
		if err != nil {
			t.Fatalf("NewWithOptions: %v", err)
		}
		if c.httpClient.Transport != rt {
			t.Fatal("WithTransport did not set the RoundTripper on the default client")
		}
		if c.httpClient.Timeout != 30*time.Second {
			t.Errorf("expected default 30s timeout, got %v", c.httpClient.Timeout)
		}
		if _, err := c.GetAccounts(); err != nil {
			t.Fatalf("GetAccounts: %v", err)
		}
		if rt.calls == 0 || rt.last == nil {
			t.Fatal("injected transport was not exercised")
		}
		if got := rt.last.Header.Get("User-Agent"); got != "open-banking-io/go/"+Version {
			t.Errorf("unexpected User-Agent: %q", got)
		}
	})

	t.Run("transport on provided client", func(t *testing.T) {
		hc := &http.Client{Timeout: 99 * time.Second}
		rt := &recordingTransport{}
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
	})
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
