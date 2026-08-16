package jwt

import (
	"crypto/ed25519"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/mehrdad-masoumi/go-packages/security/jwks"
)

func signedUserToken(t *testing.T, priv ed25519.PrivateKey, kid, iss, aud, tokenUse string, exp time.Time) string {
	t.Helper()
	claims := flexibleClaims{
		UserID:   "42",
		TokenUse: tokenUse,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    iss,
			Audience:  jwtlib.ClaimStrings{aud},
			ExpiresAt: jwtlib.NewNumericDate(exp),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(priv)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVerifierAudienceAndExpiry(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	v, err := NewVerifier(VerifierConfig{
		Issuer:     "auth",
		Audience:   "broker",
		StaticKeys: []jwks.PublicKeyEntry{{KID: "k1", PublicKey: pub}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(t.Context(), signedUserToken(t, priv, "k1", "auth", "broker", TokenUseAccess, time.Now().Add(time.Minute))); err != nil {
		t.Fatalf("valid token: %v", err)
	}
	if _, err := v.Verify(t.Context(), signedUserToken(t, priv, "k1", "auth", "other", TokenUseAccess, time.Now().Add(time.Minute))); err == nil {
		t.Fatal("expected invalid audience")
	}
	if _, err := v.Verify(t.Context(), signedUserToken(t, priv, "k1", "auth", "broker", TokenUseAccess, time.Now().Add(-time.Minute))); err == nil {
		t.Fatal("expected expired token")
	}
}

func TestVerifierRejectsRefreshTokenUse(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	v, err := NewVerifier(VerifierConfig{
		Issuer:     "auth",
		Audience:   "broker",
		StaticKeys: []jwks.PublicKeyEntry{{KID: "k1", PublicKey: pub}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw := signedUserToken(t, priv, "k1", "auth", "broker", TokenUseRefresh, time.Now().Add(time.Minute))
	if _, err := v.Verify(t.Context(), raw); err == nil {
		t.Fatal("resource verifier must reject refresh tokens")
	}
	if _, err := v.Verify(t.Context(), signedUserToken(t, priv, "k1", "auth", "broker", "", time.Now().Add(time.Minute))); err == nil {
		t.Fatal("resource verifier must reject tokens without token_use")
	}
}
