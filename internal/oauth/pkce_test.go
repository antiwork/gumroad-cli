package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGenerateVerifier_Length(t *testing.T) {
	v, err := GenerateVerifier()
	if err != nil {
		t.Fatalf("GenerateVerifier: %v", err)
	}
	if len(v) != 43 {
		t.Fatalf("verifier length = %d, want 43", len(v))
	}
}

func TestGenerateVerifier_Unique(t *testing.T) {
	v1, _ := GenerateVerifier()
	v2, _ := GenerateVerifier()
	if v1 == v2 {
		t.Fatal("two verifiers should not be identical")
	}
}

func TestChallengeFromVerifier_KnownVector(t *testing.T) {
	// RFC 7636 Appendix B test vector
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	h := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(h[:])

	got := ChallengeFromVerifier(verifier)
	if got != want {
		t.Fatalf("challenge = %q, want %q", got, want)
	}
}

func TestChallengeFromVerifier_Deterministic(t *testing.T) {
	v, _ := GenerateVerifier()
	c1 := ChallengeFromVerifier(v)
	c2 := ChallengeFromVerifier(v)
	if c1 != c2 {
		t.Fatal("same verifier should produce same challenge")
	}
}
