package openbanking

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fuzzFixtureValue reads fixtures/<file> and returns the string at the given top-level key.
// It accepts testing.TB so the fuzz seed phase (*testing.F) can share it with normal tests.
func fuzzFixtureValue(tb testing.TB, file, key string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join(fixturesDir, file))
	if err != nil {
		tb.Fatalf("read fixture %s: %v", file, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		tb.Fatalf("parse fixture %s: %v", file, err)
	}
	s, ok := m[key].(string)
	if !ok {
		tb.Fatalf("fixture %s has no string key %q", file, key)
	}
	return s
}

// FuzzDecryptEnvelope feeds arbitrary bytes to the wire-format envelope parser. An envelope
// arrives from the network as attacker-influenced ciphertext, and decryptEnvelope slices its
// fixed header (version|ephPub|nonce|tag) purely by length — so malformed input must always
// return an error, never panic or read out of range.
func FuzzDecryptEnvelope(f *testing.F) {
	priv, err := loadPrivateKey(fuzzFixtureValue(f, "keypair.json", "privateKeyPkcs8B64"))
	if err != nil {
		f.Fatalf("load key: %v", err)
	}

	// Seed corpus: a valid envelope plus structurally interesting malformations.
	if raw, err := base64.StdEncoding.DecodeString(fuzzFixtureValue(f, "envelopes.json", "account")); err == nil {
		f.Add(raw)
		if len(raw) >= headerLen {
			f.Add(raw[:headerLen])   // valid header, empty ciphertext
			f.Add(raw[:headerLen-1]) // one byte short of the header
		}
	}
	f.Add([]byte(nil))
	f.Add([]byte{envVersion})
	f.Add(make([]byte, headerLen))

	f.Fuzz(func(t *testing.T, data []byte) {
		// The contract is "no panic": an error is the expected outcome for virtually all inputs,
		// and a successful decrypt of random bytes is cryptographically impossible.
		_, _ = decryptEnvelope(priv, data)
	})
}
