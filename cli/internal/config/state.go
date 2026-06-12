package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// State is CLI-local, non-secret UI state — chiefly the "current account" that transactions and
// sync default to. It is deliberately kept out of the credentials Bundle: the Bundle is the SDK's
// CredentialsBundle struct (consumed by the SDK too), so adding fields there would be fragile.
type State struct {
	CurrentAccountID string `json:"currentAccountId,omitempty"`
}

// StatePath returns the state file path: state.json beside the credentials file. Deriving it from
// the credentials path (rather than re-resolving DefaultPath) means OPENBANKING_CONFIG overrides and
// test temp directories are honored automatically.
func StatePath(credentialsPath string) string {
	return filepath.Join(filepath.Dir(credentialsPath), "state.json")
}

// LoadState reads CLI state stored beside the credentials file. A missing file is not an error — it
// just means no current account has been chosen yet, so the zero State is returned.
func LoadState(credentialsPath string) (State, error) {
	path := StatePath(credentialsPath)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("could not read state at %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("could not parse state at %s: %w", path, err)
	}
	return s, nil
}

// SaveState writes CLI state beside the credentials file. It sits next to secrets, so it is written
// 0600 with its directory created 0700, matching Save.
func SaveState(credentialsPath string, s State) error {
	path := StatePath(credentialsPath)
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("could not create config directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("could not encode state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("could not write state to %s: %w", path, err)
	}
	return nil
}
