package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
)

func TestKeyImportFromRawPkcs8(t *testing.T) {
	rawKey := fixtureBundle(t).EncryptionKey.PrivateKey
	cfg := filepath.Join(t.TempDir(), "credentials.json")

	var out, errOut bytes.Buffer
	app := &App{Stdin: strings.NewReader(rawKey), Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"key", "import"}); err != nil {
		t.Fatalf("key import: %v\nstderr: %s", err, errOut.String())
	}

	got, err := config.Load(cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.EncryptionKey.PrivateKey != rawKey {
		t.Errorf("stored key = %q, want the imported key", got.EncryptionKey.PrivateKey)
	}
}

func TestKeyImportFromBundleFilePreservesApiKey(t *testing.T) {
	bundle := fixtureBundle(t)

	// A login already saved an API key into the local config.
	cfg := filepath.Join(t.TempDir(), "credentials.json")
	existing := config.Bundle{APIKey: "ebk_from_login", APIBaseURL: "https://open-banking.io"}
	if err := config.Save(cfg, existing); err != nil {
		t.Fatal(err)
	}

	// The user imports the exported credentials bundle (which carries the encryption key).
	bundleFile := filepath.Join(t.TempDir(), "export.json")
	data, _ := json.Marshal(bundle)
	if err := os.WriteFile(bundleFile, data, 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"key", "import", bundleFile}); err != nil {
		t.Fatalf("key import: %v\nstderr: %s", err, errOut.String())
	}

	got, err := config.Load(cfg)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.EncryptionKey.PrivateKey != bundle.EncryptionKey.PrivateKey {
		t.Error("encryption key not imported from bundle")
	}
	if got.APIKey != "ebk_from_login" {
		t.Errorf("login's API key was clobbered: %q", got.APIKey)
	}
}

func TestKeyImportRejectsInvalidKey(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "credentials.json")
	var out, errOut bytes.Buffer
	app := &App{Stdin: strings.NewReader("not-a-real-key"), Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := app.Run([]string{"key", "import"}); err == nil {
		t.Fatal("expected an error importing a non-PKCS#8 string")
	}
	if _, statErr := os.Stat(cfg); statErr == nil {
		t.Error("config should not be written when the key is invalid")
	}
}

func TestKeyImportPromptsWhenStdinIsATerminal(t *testing.T) {
	rawKey := fixtureBundle(t).EncryptionKey.PrivateKey

	for _, args := range [][]string{{"key", "import"}, {"key", "import", "-"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "credentials.json")
			var out, errOut bytes.Buffer
			stdin := strings.NewReader(rawKey)
			app := &App{Stdin: stdin, Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
			app.env = ui.Custom(stdin, &out, &errOut, ui.FormatTable, false, true)

			if err := app.Run(args); err != nil {
				t.Fatalf("key import: %v\nstderr: %s", err, errOut.String())
			}
			if !strings.Contains(errOut.String(), "Ctrl-D") {
				t.Errorf("prompt does not say how to end the input; stderr = %q", errOut.String())
			}
			got, err := config.Load(cfg)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if got.EncryptionKey.PrivateKey != rawKey {
				t.Error("prompting broke the import itself")
			}
		})
	}
}

func TestKeyImportDoesNotPromptWhenPiped(t *testing.T) {
	rawKey := fixtureBundle(t).EncryptionKey.PrivateKey

	for _, args := range [][]string{{"key", "import"}, {"key", "import", "-"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "credentials.json")
			var out, errOut bytes.Buffer
			stdin := strings.NewReader(rawKey)
			app := &App{Stdin: stdin, Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
			app.env = ui.Custom(stdin, &out, &errOut, ui.FormatTable, false, false)

			if err := app.Run(args); err != nil {
				t.Fatalf("key import: %v\nstderr: %s", err, errOut.String())
			}
			if errOut.Len() != 0 {
				t.Errorf("piped import wrote to stderr: %q", errOut.String())
			}
		})
	}
}

func TestKeyImportFromFileDoesNotPrompt(t *testing.T) {
	bundle := fixtureBundle(t)
	keyFile := filepath.Join(t.TempDir(), "key.txt")
	if err := os.WriteFile(keyFile, []byte(bundle.EncryptionKey.PrivateKey), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(t.TempDir(), "credentials.json")

	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	app.env = ui.Custom(nil, &out, &errOut, ui.FormatTable, false, true)

	if err := app.Run([]string{"key", "import", keyFile}); err != nil {
		t.Fatalf("key import: %v\nstderr: %s", err, errOut.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("prompted despite reading a file: %q", errOut.String())
	}
}

// TestKeyImportThenDecrypt is the end-to-end proof: after importing the key and having an API key,
// `accounts` decrypts the fixtures.
func TestKeyImportThenDecrypt(t *testing.T) {
	bundle := fixtureBundle(t)
	srv := startAPIServer(t, bundle.APIKey)

	cfg := filepath.Join(t.TempDir(), "credentials.json")
	// Start from a login-only config (API key + base url, no encryption key yet).
	if err := config.Save(cfg, config.Bundle{APIKey: bundle.APIKey, APIBaseURL: srv.URL}); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	importApp := &App{Stdin: strings.NewReader(bundle.EncryptionKey.PrivateKey), Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := importApp.Run([]string{"key", "import"}); err != nil {
		t.Fatalf("key import: %v", err)
	}

	out.Reset()
	acctApp := &App{Stdout: &out, Stderr: &errOut, ConfigPath: cfg}
	if err := acctApp.Run([]string{"accounts"}); err != nil {
		t.Fatalf("accounts after import: %v\nstderr: %s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "DK6466952001724927") {
		t.Errorf("expected decrypted IBAN after key import\n%s", out.String())
	}
}
