package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	openbanking "github.com/open-banking-io/clients/go"
)

func TestDefaultPathFallsBackToHomeConfig(t *testing.T) {
	t.Setenv("OPENBANKING_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	suffix := filepath.Join(".config", "open-banking", "credentials.json")
	if !strings.HasSuffix(got, suffix) {
		t.Errorf("DefaultPath = %q, want it to end with %q", got, suffix)
	}
}

func TestLoadRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte("{ not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error parsing malformed credentials JSON")
	}
}

func TestSaveFailsWhenParentIsAFile(t *testing.T) {
	// Create a regular file, then try to save "underneath" it: MkdirAll cannot create a directory
	// where a file already exists, so Save must surface that error.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(file, "sub", "credentials.json")
	if err := Save(target, openbanking.CredentialsBundle{APIKey: "k"}); err == nil {
		t.Fatal("expected an error when the parent path is a file")
	}
}
