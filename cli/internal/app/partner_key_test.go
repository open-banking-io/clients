package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return data
}

func fixtureKeypair(t *testing.T) (private, public string) {
	t.Helper()
	var kp struct {
		Private string `json:"privateKeyPkcs8B64"`
		Public  string `json:"publicKeyRawB64"`
	}
	if err := json.Unmarshal(repoFixture(t, "keypair.json"), &kp); err != nil {
		t.Fatal(err)
	}
	return kp.Private, kp.Public
}

func TestPartnerKeyGenerate_WritesA0600File_PrintsThePublicHalf_AndKeepsThePrivateOneOffStdout(t *testing.T) {
	out := filepath.Join(t.TempDir(), "recipient-key.json")
	var stdout, stderr bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &stderr}
	if err := app.Run([]string{"partner", "key", "generate", "--out", out}); err != nil {
		t.Fatalf("generate: %v\nstderr: %s", err, stderr.String())
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 0600", info.Mode().Perm())
	}
	var file recipientKeyFile
	data, _ := os.ReadFile(out)
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.Kind != "partner-recipient-key" || file.EncryptionKey.PrivateKey == "" || file.EncryptionKey.PublicKey == "" {
		t.Errorf("file = %+v, want a partner-recipient-key with both halves", file)
	}
	if !strings.Contains(stdout.String(), file.EncryptionKey.PublicKey) || !strings.Contains(stdout.String(), file.Fingerprint) {
		t.Errorf("stdout should show the public half and the fingerprint:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), file.EncryptionKey.PrivateKey) {
		t.Errorf("stdout must not show the private half without --print-private:\n%s", stdout.String())
	}
	if len(file.Fingerprint) != 16 {
		t.Errorf("fingerprint %q is not 16 hex characters", file.Fingerprint)
	}

	// The file is never overwritten: a second run must refuse rather than replace a key in use.
	stdout.Reset()
	if err := app.Run([]string{"partner", "key", "generate", "--out", out}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second generate: err = %v, want a refusal to overwrite", err)
	}
	after, _ := os.ReadFile(out)
	if !bytes.Equal(after, data) {
		t.Error("the existing key file changed")
	}

	// --print-private shows it, once, for the person who is about to paste it into a secret store.
	stdout.Reset()
	other := filepath.Join(t.TempDir(), "k.json")
	if err := app.Run([]string{"partner", "key", "generate", "--out", other, "--print-private"}); err != nil {
		t.Fatal(err)
	}
	var second recipientKeyFile
	d2, _ := os.ReadFile(other)
	_ = json.Unmarshal(d2, &second)
	if !strings.Contains(stdout.String(), second.EncryptionKey.PrivateKey) {
		t.Errorf("--print-private should print the private half:\n%s", stdout.String())
	}
}

func TestPartnerKeyFingerprint_MatchesThePartnerPage_ForEveryInputShape(t *testing.T) {
	private, public := fixtureKeypair(t)
	// Written out by hand for the committed fixtures/keypair.json.
	const want = "91fa2aea473dbab6\n"

	for name, input := range map[string]string{
		"raw public point": public,
		"bare pkcs8":       private,
		"key file":         `{"encryptionKey":{"privateKey":"` + private + `"}}`,
	} {
		var stdout, stderr bytes.Buffer
		app := &App{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if err := app.Run([]string{"partner", "key", "fingerprint", "-"}); err != nil {
			t.Fatalf("%s: %v\nstderr: %s", name, err, stderr.String())
		}
		if stdout.String() != want {
			t.Errorf("%s: fingerprint = %q, want %q", name, stdout.String(), want)
		}
	}

	var stdout bytes.Buffer
	app := &App{Stdin: strings.NewReader("aGVsbG8="), Stdout: &stdout, Stderr: &stdout}
	if err := app.Run([]string{"partner", "key", "fingerprint"}); err == nil {
		t.Error("a non-key should be refused")
	}
}

func TestPartnerKeyAnswer_PrintsOnlyTheNonce_AndOnlyForTheRightKey(t *testing.T) {
	private, _ := fixtureKeypair(t)
	var ch struct {
		Nonce    string `json:"nonce"`
		Envelope string `json:"envelope"`
	}
	if err := json.Unmarshal(repoFixture(t, "recipient-key-challenge.json"), &ch); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "recipient-key.json")
	if err := os.WriteFile(keyFile, []byte(`{"encryptionKey":{"privateKey":"`+private+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	app := &App{Stdout: &stdout, Stderr: &stderr}
	if err := app.Run([]string{"partner", "key", "answer", "--key", keyFile, ch.Envelope}); err != nil {
		t.Fatalf("answer: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.String() != ch.Nonce+"\n" {
		t.Errorf("stdout = %q, want the nonce alone", stdout.String())
	}

	// From stdin too, with the whitespace a terminal paste adds.
	stdout.Reset()
	app = &App{Stdin: strings.NewReader("  " + ch.Envelope + "\n"), Stdout: &stdout, Stderr: &stderr}
	if err := app.Run([]string{"partner", "key", "answer", "--key", keyFile}); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != ch.Nonce+"\n" {
		t.Errorf("stdin: stdout = %q", stdout.String())
	}

	// A stranger's key opens nothing.
	strangerOut := filepath.Join(t.TempDir(), "stranger.json")
	if err := (&App{Stdout: &bytes.Buffer{}, Stderr: &stderr}).Run([]string{"partner", "key", "generate", "--out", strangerOut}); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	app = &App{Stdout: &stdout, Stderr: &stderr}
	if err := app.Run([]string{"partner", "key", "answer", "--key", strangerOut, ch.Envelope}); err == nil {
		t.Error("a different key answered the challenge")
	}
	if stdout.Len() != 0 {
		t.Errorf("nothing must be printed on failure, got %q", stdout.String())
	}

	// A data envelope is not a challenge.
	var env struct {
		UID string `json:"uid"`
	}
	_ = json.Unmarshal(repoFixture(t, "envelopes.json"), &env)
	app = &App{Stdout: &stdout, Stderr: &stderr}
	if err := app.Run([]string{"partner", "key", "answer", "--key", keyFile, env.UID}); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Errorf("a data envelope should be refused as carrying no nonce, got %v", err)
	}
}
