package config

import (
	"path/filepath"
	"testing"
)

func TestStatePathIsSiblingOfCredentials(t *testing.T) {
	creds := filepath.Join("/tmp", "open-banking", "credentials.json")
	want := filepath.Join("/tmp", "open-banking", "state.json")
	if got := StatePath(creds); got != want {
		t.Errorf("StatePath = %q, want %q", got, want)
	}
}

func TestLoadStateMissingIsZero(t *testing.T) {
	creds := filepath.Join(t.TempDir(), "credentials.json")
	s, err := LoadState(creds)
	if err != nil {
		t.Fatalf("LoadState on missing file: %v", err)
	}
	if s.CurrentAccountID != "" {
		t.Errorf("expected zero State, got %+v", s)
	}
}

func TestSaveLoadStateRoundTrip(t *testing.T) {
	creds := filepath.Join(t.TempDir(), "credentials.json")
	if err := SaveState(creds, State{CurrentAccountID: "acct-123"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	s, err := LoadState(creds)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.CurrentAccountID != "acct-123" {
		t.Errorf("CurrentAccountID = %q, want acct-123", s.CurrentAccountID)
	}
}
