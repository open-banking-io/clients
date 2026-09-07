package openbanking

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFixtureFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "fixtures", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return data
}

func TestDecryptEnvelope_OpensTheChallengeFixture_OnlyWithTheRightKey(t *testing.T) {
	var kp struct {
		Private string `json:"privateKeyPkcs8B64"`
	}
	if err := json.Unmarshal(readFixtureFile(t, "keypair.json"), &kp); err != nil {
		t.Fatal(err)
	}
	var ch struct {
		Nonce    string `json:"nonce"`
		Envelope string `json:"envelope"`
	}
	if err := json.Unmarshal(readFixtureFile(t, "recipient-key-challenge.json"), &ch); err != nil {
		t.Fatal(err)
	}

	plain, err := DecryptEnvelope(kp.Private, ch.Envelope)
	if err != nil {
		t.Fatalf("DecryptEnvelope: %v", err)
	}
	if !strings.Contains(string(plain), ch.Nonce) {
		t.Errorf("plaintext %q does not carry the nonce %q", plain, ch.Nonce)
	}

	var payload struct {
		Nonce string `json:"nonce"`
	}
	ok, err := DecryptTo(kp.Private, ch.Envelope, &payload)
	if err != nil || !ok {
		t.Fatalf("DecryptTo: ok=%v err=%v", ok, err)
	}
	if payload.Nonce != ch.Nonce {
		t.Errorf("nonce = %q, want %q", payload.Nonce, ch.Nonce)
	}
	if ok, err := DecryptTo(kp.Private, "", &payload); err != nil || ok {
		t.Errorf("empty envelope: ok=%v err=%v, want false and nil", ok, err)
	}

	// Another key opens nothing: the challenge proves possession of exactly this one.
	stranger, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	strangerDER, err := x509.MarshalPKCS8PrivateKey(stranger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptEnvelope(base64.StdEncoding.EncodeToString(strangerDER), ch.Envelope); err == nil {
		t.Error("a different key opened the challenge")
	}
	if _, err := DecryptEnvelope(kp.Private, "not base64!"); err == nil {
		t.Error("garbage envelope decrypted")
	}
}
