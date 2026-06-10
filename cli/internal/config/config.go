// Package config loads and saves the open-banking.io credentials bundle the CLI uses to
// authenticate (API key) and decrypt (encryption private key). The on-disk format is exactly the
// SDK's CredentialsBundle, so a saved file is consumable by openbanking.FromCredentials too.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	openbanking "github.com/open-banking-io/clients/go"
)

// Bundle is the credentials bundle persisted by the CLI.
type Bundle = openbanking.CredentialsBundle

// DefaultPath resolves where the credentials bundle lives. An explicit OPENBANKING_CONFIG wins;
// otherwise it is <XDG_CONFIG_HOME or ~/.config>/open-banking/credentials.json.
func DefaultPath() (string, error) {
	if override := os.Getenv("OPENBANKING_CONFIG"); override != "" {
		return override, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not resolve home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "open-banking", "credentials.json"), nil
}

// Load reads and parses the credentials bundle at path.
func Load(path string) (Bundle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Bundle{}, fmt.Errorf("no credentials at %s — run `openbanking login` first: %w", path, err)
		}
		return Bundle{}, fmt.Errorf("could not read credentials at %s: %w", path, err)
	}
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return Bundle{}, fmt.Errorf("could not parse credentials at %s: %w", path, err)
	}
	return b, nil
}

// Save writes the credentials bundle to path with owner-only permissions, creating parent
// directories as needed. The file holds an API key and an encryption private key, so it is 0600.
func Save(path string, b Bundle) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("could not create config directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode credentials: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("could not write credentials to %s: %w", path, err)
	}
	return nil
}
