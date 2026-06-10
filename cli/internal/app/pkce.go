package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// pkceVerifier returns a fresh PKCE code verifier: 32 random bytes, base64url-encoded (RFC 7636).
func pkceVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// pkceChallenge derives the S256 code challenge for a verifier: BASE64URL(SHA256(ASCII(verifier))).
// This must match the server's verification exactly, so the CLI and API agree on the binding.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
