// Package openbanking is a server-to-server client for open-banking.io. It authenticates with an
// API key and decrypts the zero-knowledge data envelopes locally with your exported private key —
// the service only ever returns ciphertext it cannot read.
package openbanking

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// Envelope scheme: ephemeral ECDH on NIST P-256 -> HKDF-SHA256 -> AES-256-GCM.
// Wire: version(1)=0x01 | ephemeralPublicKeyRaw(65) | nonce(12) | tag(16) | ciphertext.
// Only the user's private key can decrypt — the service stores ciphertext it cannot read.
const (
	envVersion = 0x01
	pointLen   = 65
	nonceLen   = 12
	tagLen     = 16
	headerLen  = 1 + pointLen + nonceLen + tagLen
)

var (
	hkdfSalt = make([]byte, 32) // 32 zero bytes (must be exactly this).
	hkdfInfo = "bank.core.ci/zk/v1"
)

// loadPrivateKey loads a base64 PKCS#8 EC (P-256) private key for ECDH.
func loadPrivateKey(privateKeyPKCS8B64 string) (*ecdh.PrivateKey, error) {
	der, err := base64.StdEncoding.DecodeString(privateKeyPKCS8B64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 private key: %w", err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("invalid PKCS#8 key: %w", err)
	}
	ecdhKey, ok := key.(interface {
		ECDH() (*ecdh.PrivateKey, error)
	})
	if !ok {
		return nil, errors.New("private key is not an EC key")
	}
	priv, err := ecdhKey.ECDH()
	if err != nil {
		return nil, fmt.Errorf("private key is not usable for ECDH: %w", err)
	}
	return priv, nil
}

// decryptEnvelope decrypts the raw bytes of a zero-knowledge envelope.
func decryptEnvelope(privateKey *ecdh.PrivateKey, envelope []byte) ([]byte, error) {
	if len(envelope) < headerLen || envelope[0] != envVersion {
		return nil, errors.New("invalid or unsupported envelope")
	}

	ephPubBytes := envelope[1 : 1+pointLen]
	nonce := envelope[1+pointLen : 1+pointLen+nonceLen]
	tag := envelope[1+pointLen+nonceLen : headerLen]
	ciphertext := envelope[headerLen:]

	ephPub, err := ecdh.P256().NewPublicKey(ephPubBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid ephemeral public key: %w", err)
	}
	shared, err := privateKey.ECDH(ephPub)
	if err != nil {
		return nil, fmt.Errorf("ECDH failed: %w", err)
	}

	key, err := hkdf.Key(sha256.New, shared, hkdfSalt, hkdfInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("hkdf failed: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("invalid aes key: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm init failed: %w", err)
	}

	// Go's GCM Open expects ciphertext||tag.
	ctWithTag := make([]byte, 0, len(ciphertext)+len(tag))
	ctWithTag = append(ctWithTag, ciphertext...)
	ctWithTag = append(ctWithTag, tag...)

	plaintext, err := gcm.Open(nil, nonce, ctWithTag, nil)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm decryption failed: %w", err)
	}
	return plaintext, nil
}

// decryptTo decrypts a base64 envelope and unmarshals its JSON payload into out.
// An empty/absent envelope leaves out untouched and reports decrypted=false.
func decryptTo(privateKey *ecdh.PrivateKey, envelopeB64 string, out any) (bool, error) {
	if envelopeB64 == "" {
		return false, nil
	}
	raw, err := base64.StdEncoding.DecodeString(envelopeB64)
	if err != nil {
		return false, fmt.Errorf("invalid base64 envelope: %w", err)
	}
	plaintext, err := decryptEnvelope(privateKey, raw)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(plaintext, out); err != nil {
		return false, fmt.Errorf("invalid envelope payload: %w", err)
	}
	return true, nil
}
