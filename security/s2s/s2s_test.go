package s2s

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/mehrdad-masoumi/go-packages/security/jwks"
)

func TestSignerVerifier(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	signer, err := NewSigner(SignerConfig{Issuer: "auth", KeyID: "k1", PrivateKey: priv, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signer.Mint("kyc-service", "user-service", []string{"identity:apply"})
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := NewVerifier(VerifierConfig{
		Issuer:          "auth",
		Audience:        "user-service",
		StaticKeys:      []jwks.PublicKeyEntry{{KID: "k1", PublicKey: pub}},
		AllowedSubjects: []string{"kyc-service"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := verifier.Verify(t.Context(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if id.Subject != "kyc-service" || !id.HasScope("identity:apply") {
		t.Fatalf("unexpected identity: %#v", id)
	}
}

func TestWrongAudienceRejected(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	signer, _ := NewSigner(SignerConfig{Issuer: "auth", KeyID: "k1", PrivateKey: priv, TTL: time.Minute})
	raw, _ := signer.Mint("kyc-service", "wallet-service", nil)
	verifier, _ := NewVerifier(VerifierConfig{Issuer: "auth", Audience: "user-service", StaticKeys: []jwks.PublicKeyEntry{{KID: "k1", PublicKey: pub}}})
	if _, err := verifier.Verify(t.Context(), raw); err == nil {
		t.Fatal("expected audience rejection")
	}
}
