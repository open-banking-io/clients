package config

import (
	"os"
	"path/filepath"
	"testing"

	openbanking "github.com/open-banking-io/clients/go"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "credentials.json")

	want := openbanking.CredentialsBundle{
		Service:    "open-banking.io",
		APIBaseURL: "https://open-banking.io",
		User:       "demo@open-banking.io",
		APIKey:     "ebk_secret",
		EncryptionKey: openbanking.EncryptionKey{
			PrivateKey: "pkcs8-base64-here",
		},
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIBaseURL != want.APIBaseURL || got.APIKey != want.APIKey ||
		got.EncryptionKey.PrivateKey != want.EncryptionKey.PrivateKey {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestSaveWritesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := Save(path, openbanking.CredentialsBundle{APIKey: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("perm = %o, want 600 (the file holds an API key + private key)", perm)
	}
}

func TestLoadMissingFileIsHelpful(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected an error loading a missing credentials file")
	}
}

func TestDefaultPathHonorsXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	t.Setenv("OPENBANKING_CONFIG", "")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if want := "/tmp/xdg/open-banking/credentials.json"; got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPathHonorsExplicitOverride(t *testing.T) {
	t.Setenv("OPENBANKING_CONFIG", "/custom/creds.json")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if got != "/custom/creds.json" {
		t.Errorf("DefaultPath = %q, want the explicit override", got)
	}
}
