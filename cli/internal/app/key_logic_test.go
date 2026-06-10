package app

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestKeyRequiresSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "c.json")}
	if err := app.Run([]string{"key"}); err == nil {
		t.Fatal("expected an error for `key` with no subcommand")
	}
}

func TestKeyUnknownSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: filepath.Join(t.TempDir(), "c.json")}
	if err := app.Run([]string{"key", "frobnicate"}); err == nil {
		t.Fatal("expected an error for an unknown key subcommand")
	}
}

func TestParseEncryptionKeyEmptyInput(t *testing.T) {
	if _, err := parseEncryptionKey([]byte("   \n")); err == nil {
		t.Error("expected an error for empty key input")
	}
}

func TestParseEncryptionKeyBundleWithoutPrivateKey(t *testing.T) {
	if _, err := parseEncryptionKey([]byte(`{"encryptionKey":{}}`)); err == nil {
		t.Error("expected an error for a bundle with no private key")
	}
}

func TestParseEncryptionKeyMalformedBundle(t *testing.T) {
	if _, err := parseEncryptionKey([]byte(`{ not json`)); err == nil {
		t.Error("expected an error for a malformed bundle JSON")
	}
}

func TestParseEncryptionKeyCollapsesWrappedBareKey(t *testing.T) {
	// A bare key pasted with whitespace/newlines must be collapsed to a single base64 string.
	enc, err := parseEncryptionKey([]byte("AAAA\n  BBBB \tCCCC\n"))
	if err != nil {
		t.Fatalf("parseEncryptionKey: %v", err)
	}
	if enc.PrivateKey != "AAAABBBBCCCC" {
		t.Errorf("collapsed key = %q, want AAAABBBBCCCC", enc.PrivateKey)
	}
}

func TestValidateP256Pkcs8(t *testing.T) {
	// A real P-256 key passes.
	p256, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p256DER, err := x509.MarshalPKCS8PrivateKey(p256)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateP256Pkcs8(base64.StdEncoding.EncodeToString(p256DER)); err != nil {
		t.Errorf("valid P-256 key rejected: %v", err)
	}

	// Invalid base64.
	if err := validateP256Pkcs8("not!base64!!"); err == nil {
		t.Error("expected error for invalid base64")
	}

	// Valid base64 but not a PKCS#8 structure.
	if err := validateP256Pkcs8(base64.StdEncoding.EncodeToString([]byte("nope"))); err == nil {
		t.Error("expected error for non-PKCS#8 DER")
	}

	// A non-EC key (Ed25519): parses, but has no ECDH.
	_, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edDER, err := x509.MarshalPKCS8PrivateKey(edPriv)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateP256Pkcs8(base64.StdEncoding.EncodeToString(edDER)); err == nil {
		t.Error("expected error for a non-EC key")
	}

	// A valid EC key on the wrong curve (P-384).
	p384, err := ecdh.P384().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p384DER, err := x509.MarshalPKCS8PrivateKey(p384)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateP256Pkcs8(base64.StdEncoding.EncodeToString(p384DER)); err == nil {
		t.Error("expected error for a P-384 key (wrong curve)")
	}
}
