package app

import (
	"crypto/ecdh"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/open-banking-io/clients/cli/internal/config"
	"github.com/open-banking-io/clients/cli/internal/ui"
	openbanking "github.com/open-banking-io/clients/go"
)

func (a *App) key(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: openbanking key import [<file>]")
	}
	switch args[0] {
	case "import":
		return a.keyImport(args[1:])
	default:
		return fmt.Errorf("unknown key subcommand %q (try `openbanking key import`)", args[0])
	}
}

func (a *App) keyImport(args []string) error {
	fs := flag.NewFlagSet("key import", flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	var raw []byte
	var err error
	if fs.NArg() == 0 || fs.Arg(0) == "-" {
		if env := a.ui(); env.Interactive() {
			fmt.Fprintln(a.stderr(), env.Color("Paste your encryption key or credentials bundle, then press Ctrl-D:", ui.StyleHeader))
			fmt.Fprintln(a.stderr(), env.Color("(Ctrl-C to cancel, or re-run as `openbanking key import <file>`)", ui.StyleMuted))
		}
		raw, err = io.ReadAll(a.stdin())
	} else {
		raw, err = os.ReadFile(fs.Arg(0))
	}
	if err != nil {
		return fmt.Errorf("could not read key input: %w", err)
	}

	enc, err := parseEncryptionKey(raw)
	if err != nil {
		return err
	}
	if err := validateP256Pkcs8(enc.PrivateKey); err != nil {
		return fmt.Errorf("not a valid P-256 encryption key: %w", err)
	}

	// Preserve any API credential a prior `openbanking login` already wrote.
	bundle, err := config.Load(a.ConfigPath)
	if err != nil {
		bundle = config.Bundle{}
	}
	bundle.EncryptionKey = enc
	if bundle.Service == "" {
		bundle.Service = "open-banking.io"
	}
	if err := config.Save(a.ConfigPath, bundle); err != nil {
		return err
	}

	fmt.Fprintf(a.Stdout, "Encryption key imported to %s\n", a.ConfigPath)
	if bundle.APIKey == "" {
		fmt.Fprintln(a.Stdout, "Next: run `openbanking login` to add your API key.")
	}
	return nil
}

// parseEncryptionKey accepts either a credentials bundle JSON exported from the app (taking its
// encryptionKey) or a bare base64 PKCS#8 private key.
func parseEncryptionKey(raw []byte) (openbanking.EncryptionKey, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return openbanking.EncryptionKey{}, fmt.Errorf("no key input provided")
	}

	if strings.HasPrefix(trimmed, "{") {
		var bundle openbanking.CredentialsBundle
		if err := json.Unmarshal([]byte(trimmed), &bundle); err != nil {
			return openbanking.EncryptionKey{}, fmt.Errorf("could not parse credentials bundle: %w", err)
		}
		if strings.TrimSpace(bundle.EncryptionKey.PrivateKey) == "" {
			return openbanking.EncryptionKey{}, fmt.Errorf("the credentials bundle has no encryption private key")
		}
		// Only the private key is load-bearing; rebuild the metadata from known-good constants so a
		// crafted bundle can't seed misleading scheme/curve/publicKey values into the config at rest.
		return encryptionKeyFor(bundle.EncryptionKey.PrivateKey), nil
	}

	// A bare key may be pasted with line wrapping; collapse all whitespace.
	return encryptionKeyFor(strings.Join(strings.Fields(trimmed), "")), nil
}

// encryptionKeyFor wraps a base64 PKCS#8 private key with the fixed scheme metadata.
func encryptionKeyFor(privateKeyB64 string) openbanking.EncryptionKey {
	return openbanking.EncryptionKey{
		Scheme:           "ECDH-P256 + HKDF-SHA256 + AES-256-GCM",
		Curve:            "P-256",
		PrivateKeyFormat: "pkcs8-base64",
		PrivateKey:       privateKeyB64,
	}
}

// validateP256Pkcs8 checks that the base64 PKCS#8 string is a P-256 EC key usable for ECDH —
// exactly what the SDK requires to decrypt — so a bad import fails here rather than at first use.
func validateP256Pkcs8(privateKeyB64 string) error {
	der, err := base64.StdEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return fmt.Errorf("invalid base64: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return fmt.Errorf("invalid PKCS#8 key: %w", err)
	}
	ecKey, ok := parsed.(interface {
		ECDH() (*ecdh.PrivateKey, error)
	})
	if !ok {
		return fmt.Errorf("key is not an EC key")
	}
	priv, err := ecKey.ECDH()
	if err != nil {
		return fmt.Errorf("key is not usable for ECDH: %w", err)
	}
	if priv.Curve() != ecdh.P256() {
		return fmt.Errorf("key is not on the P-256 curve")
	}
	return nil
}
