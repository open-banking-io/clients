package openbanking

import "testing"

func TestNewTrimsWhitespaceAroundBaseURL(t *testing.T) {
	m := startMock(t)

	for _, padded := range []string{
		"  " + m.server.URL,
		m.server.URL + "  ",
		"\t" + m.server.URL + "\n",
		"  " + m.server.URL + "/  ",
	} {
		c, err := New(padded, m.apiKey, m.privateKey, nil)
		if err != nil {
			t.Fatalf("New(%q) = %v, want no error", padded, err)
		}
		if _, err := c.GetAccounts(); err != nil {
			t.Fatalf("GetAccounts() with base %q: %v", padded, err)
		}
	}
}

func TestNewRejectsBaseURLWithoutHTTPScheme(t *testing.T) {
	for _, bad := range []string{"open-banking.io", "//open-banking.io", "ftp://open-banking.io"} {
		if _, err := New(bad, "k", "x", nil); err == nil {
			t.Errorf("New(%q) = nil error, want a scheme error", bad)
		}
	}
}

func TestNewRejectsCleartextHTTPToRemoteHost(t *testing.T) {
	for _, bad := range []string{
		"http://open-banking.io",
		"http://192.168.1.10:8080",
		"http://localhost.evil.test",
	} {
		if _, err := New(bad, "k", "x", nil); err == nil {
			t.Errorf("New(%q) = nil error, want a cleartext error", bad)
		}
	}
}

func TestNewAllowsCleartextHTTPOnLoopback(t *testing.T) {
	m := startMock(t)
	// httptest binds 127.0.0.1, so the mock server itself is the loopback case.
	if _, err := New(m.server.URL, m.apiKey, m.privateKey, nil); err != nil {
		t.Fatalf("New(%q) = %v, want no error", m.server.URL, err)
	}
	for _, ok := range []string{"http://localhost:8080", "http://[::1]:8080", "HTTP://LOCALHOST:8080"} {
		if _, err := New(ok, m.apiKey, m.privateKey, nil); err != nil {
			t.Errorf("New(%q) = %v, want no error", ok, err)
		}
	}
}

func TestNewPublicAppliesTheSameBaseURLRules(t *testing.T) {
	if _, err := NewPublic("open-banking.io", "k", nil); err == nil {
		t.Error("NewPublic with no scheme = nil error, want a scheme error")
	}
	if _, err := NewPublic("http://open-banking.io", "k", nil); err == nil {
		t.Error("NewPublic with cleartext http = nil error, want a cleartext error")
	}
}
