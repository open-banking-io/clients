package app

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	openbanking "github.com/open-banking-io/clients/go"
)

const partnerKeyUsage = "usage: openbanking partner key generate [--out FILE] [--print-private] | fingerprint [<file>|-] | answer --key FILE [<envelope>|-]"

// partner is the tooling a partner (not an end user) needs: its own decryption key. The install
// itself stays on the partner page — the endpoints take a signed-in owner session, not an API key.
func (a *App) partner(args []string) error {
	if len(args) < 2 || args[0] != "key" {
		return errors.New(partnerKeyUsage)
	}
	switch args[1] {
	case "generate":
		return a.partnerKeyGenerate(args[2:])
	case "fingerprint":
		return a.partnerKeyFingerprint(args[2:])
	case "answer":
		return a.partnerKeyAnswer(args[2:])
	default:
		return fmt.Errorf("unknown partner key subcommand %q (%s)", args[1], partnerKeyUsage)
	}
}

// recipientKeyFile is the bundle the partner page downloads and `partner key generate` writes:
// the same shape as a credentials bundle's encryptionKey, so the other key readers accept it.
type recipientKeyFile struct {
	Service       string                    `json:"service"`
	Kind          string                    `json:"kind"`
	Fingerprint   string                    `json:"fingerprint"`
	EncryptionKey openbanking.EncryptionKey `json:"encryptionKey"`
	CreatedAt     string                    `json:"createdAt"`
}

func (a *App) partnerKeyGenerate(args []string) error {
	fs := flag.NewFlagSet("partner key generate", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	out := fs.String("out", "recipient-key.json", "file to write the key pair to (created 0600, never overwritten)")
	printPrivate := fs.Bool("print-private", false, "also print the private half to stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}

	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("could not generate a key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return fmt.Errorf("could not encode the key: %w", err)
	}
	privateB64 := base64.StdEncoding.EncodeToString(der)
	publicRaw := priv.PublicKey().Bytes()
	publicB64 := base64.StdEncoding.EncodeToString(publicRaw)
	fp := recipientKeyFingerprint(publicRaw)

	enc := encryptionKeyFor(privateB64)
	enc.PublicKey = publicB64
	file := recipientKeyFile{Service: "open-banking.io", Kind: "partner-recipient-key", Fingerprint: fp, EncryptionKey: enc, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists — a key file is never overwritten; pass --out to write elsewhere", *out)
		}
		return fmt.Errorf("could not create %s: %w", *out, err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return fmt.Errorf("could not write %s: %w", *out, err)
	}
	if err := f.Close(); err != nil {
		return err
	}

	fmt.Fprintf(a.stdout(), "Private key written to %s (keep it where your deployment can read it; it cannot be recovered)\n", *out)
	fmt.Fprintf(a.stdout(), "Public key (install this on your partner page):\n%s\n", publicB64)
	fmt.Fprintf(a.stdout(), "Fingerprint: %s\n", fp)
	if *printPrivate {
		fmt.Fprintf(a.stdout(), "Private key (PKCS#8, base64):\n%s\n", privateB64)
	}
	return nil
}

func (a *App) partnerKeyFingerprint(args []string) error {
	fs := flag.NewFlagSet("partner key fingerprint", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := a.readInput(fs.Arg(0))
	if err != nil {
		return err
	}
	publicRaw, err := publicPointFrom(raw)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.stdout(), recipientKeyFingerprint(publicRaw))
	return nil
}

func (a *App) partnerKeyAnswer(args []string) error {
	fs := flag.NewFlagSet("partner key answer", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	keyPath := fs.String("key", "", "the key file (from `partner key generate` or the partner page download), or a bare PKCS#8 key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyPath == "" {
		return errors.New("--key is required: the file holding the private half of the key being installed")
	}
	keyRaw, err := os.ReadFile(*keyPath)
	if err != nil {
		return fmt.Errorf("could not read the key file: %w", err)
	}
	enc, err := parseEncryptionKey(keyRaw)
	if err != nil {
		return err
	}
	envelopeText := fs.Arg(0)
	if envelopeText == "" || envelopeText == "-" {
		piped, err := io.ReadAll(a.stdin())
		if err != nil {
			return fmt.Errorf("could not read stdin: %w", err)
		}
		envelopeText = string(piped)
	}
	envelope := strings.Join(strings.Fields(envelopeText), "")
	if envelope == "" {
		return errors.New("no envelope given: pass it as the argument, or pipe it on stdin")
	}
	var payload struct {
		Nonce string `json:"nonce"`
	}
	ok, err := openbanking.DecryptTo(enc.PrivateKey, envelope, &payload)
	if err != nil {
		return fmt.Errorf("could not open the challenge with this key — is it the private half of the key you are installing? (%w)", err)
	}
	if !ok || payload.Nonce == "" {
		return errors.New("the envelope opened but carries no nonce; it is not a recipient-key challenge")
	}
	fmt.Fprintln(a.stdout(), payload.Nonce)
	return nil
}

// readInput reads a file argument, or stdin when the argument is empty or "-".
func (a *App) readInput(arg string) ([]byte, error) {
	if arg == "" || arg == "-" {
		data, err := io.ReadAll(a.stdin())
		if err != nil {
			return nil, fmt.Errorf("could not read stdin: %w", err)
		}
		return data, nil
	}
	data, err := os.ReadFile(arg)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", arg, err)
	}
	return data, nil
}

// publicPointFrom accepts a key file, a bare PKCS#8 private key, or the raw base64 public point
// itself, and returns the 65-byte uncompressed point the fingerprint is computed over.
func publicPointFrom(raw []byte) ([]byte, error) {
	trimmed := strings.Join(strings.Fields(string(raw)), "")
	if !strings.HasPrefix(trimmed, "{") {
		if b, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(b) == 65 && b[0] == 0x04 {
			return b, nil
		}
	}
	enc, err := parseEncryptionKey(raw)
	if err != nil {
		return nil, err
	}
	der, err := base64.StdEncoding.DecodeString(enc.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 private key: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("invalid PKCS#8 key: %w", err)
	}
	ecKey, ok := parsed.(interface {
		ECDH() (*ecdh.PrivateKey, error)
	})
	if !ok {
		return nil, errors.New("key is not an EC key")
	}
	priv, err := ecKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("key is not usable for ECDH: %w", err)
	}
	if priv.Curve() != ecdh.P256() {
		return nil, errors.New("key is not on the P-256 curve")
	}
	return priv.PublicKey().Bytes(), nil
}

// recipientKeyFingerprint is what the partner page and the admin show: SHA-256 of the raw
// 65-byte point, lowercase hex, first 16 characters.
func recipientKeyFingerprint(publicRaw []byte) string {
	sum := sha256.Sum256(publicRaw)
	return hex.EncodeToString(sum[:])[:16]
}
