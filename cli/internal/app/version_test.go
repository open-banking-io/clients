package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut, Version: "1.2.3"}
	if err := app.Run([]string{"version"}); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "openbanking 1.2.3") {
		t.Errorf("version output = %q, want to contain 'openbanking 1.2.3'", out.String())
	}
}

func TestVersionDefaultsToDev(t *testing.T) {
	var out, errOut bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errOut}
	if err := app.Run([]string{"--version"}); err != nil {
		t.Fatalf("--version: %v", err)
	}
	if !strings.Contains(out.String(), "openbanking dev") {
		t.Errorf("version output = %q, want 'openbanking dev'", out.String())
	}
}
