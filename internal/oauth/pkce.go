package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateVerifier creates a PKCE code verifier: 32 random bytes, base64url-encoded (43 chars).
func GenerateVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// ChallengeFromVerifier computes the S256 PKCE code challenge for a given verifier.
func ChallengeFromVerifier(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
