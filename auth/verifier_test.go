package auth_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	"github.com/mehrdad-masoumi/go-packages/auth"
)

func TestVerifier_EdDSAHappyPath(t *testing.T) {
	priv, pub := mustKey(t)
	kid := "auth-test-1"
	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:   "broker-auth",
		Audience: "broker-api",
		StaticKeys: []auth.PublicKeyEntry{
			{KID: kid, PublicKey: pub},
		},
	})
	require.NoError(t, err)

	tok := signEdDSA(t, priv, kid, "broker-auth", "broker-api", "42", time.Now().Add(time.Minute))
	claims, err := v.Verify(context.Background(), tok)
	require.NoError(t, err)
	require.Equal(t, "42", claims.UserID)
}

func TestVerifier_RejectsExpiredWrongIssAudAlg(t *testing.T) {
	priv, pub := mustKey(t)
	kid := "auth-test-1"
	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:   "broker-auth",
		Audience: "broker-api",
		StaticKeys: []auth.PublicKeyEntry{
			{KID: kid, PublicKey: pub},
		},
	})
	require.NoError(t, err)

	_, err = v.Verify(context.Background(), signEdDSA(t, priv, kid, "broker-auth", "broker-api", "1", time.Now().Add(-time.Minute)))
	require.Error(t, err)

	_, err = v.Verify(context.Background(), signEdDSA(t, priv, kid, "other", "broker-api", "1", time.Now().Add(time.Minute)))
	require.Error(t, err)

	_, err = v.Verify(context.Background(), signEdDSA(t, priv, kid, "broker-auth", "other", "1", time.Now().Add(time.Minute)))
	require.Error(t, err)

	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "1", "iss": "broker-auth", "aud": "broker-api",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	raw, err := hs.SignedString([]byte("secret"))
	require.NoError(t, err)
	_, err = v.Verify(context.Background(), raw)
	require.Error(t, err)
}

func TestVerifier_UnknownKidRefresh(t *testing.T) {
	privA, pubA := mustKey(t)
	privB, pubB := mustKey(t)

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		doc := auth.JWKS{Keys: []auth.JWK{
			auth.Ed25519JWK("kid-a", pubA),
			auth.Ed25519JWK("kid-b", pubB),
		}}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(srv.Close)

	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:   "broker-auth",
		Audience: "broker-api",
		JWKSURL:  srv.URL,
		StaticKeys: []auth.PublicKeyEntry{
			{KID: "kid-a", PublicKey: pubA},
		},
	})
	require.NoError(t, err)

	// Known kid — no JWKS call required.
	tokA := signEdDSA(t, privA, "kid-a", "broker-auth", "broker-api", "1", time.Now().Add(time.Minute))
	_, err = v.Verify(context.Background(), tokA)
	require.NoError(t, err)
	require.Equal(t, int32(0), hits.Load())

	tokB := signEdDSA(t, privB, "kid-b", "broker-auth", "broker-api", "2", time.Now().Add(time.Minute))
	_, err = v.Verify(context.Background(), tokB)
	require.NoError(t, err)
	require.Equal(t, int32(1), hits.Load())

	// Second time uses cache.
	_, err = v.Verify(context.Background(), tokB)
	require.NoError(t, err)
	require.Equal(t, int32(1), hits.Load())

	_, err = v.Verify(context.Background(), signEdDSA(t, privB, "kid-missing", "broker-auth", "broker-api", "3", time.Now().Add(time.Minute)))
	require.Error(t, err)
	require.Equal(t, int32(2), hits.Load())
}

func TestVerifier_CachedKeySurvivesJWKSOutage(t *testing.T) {
	priv, pub := mustKey(t)
	kid := "kid-1"
	var up atomic.Bool
	up.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			http.Error(w, "down", 503)
			return
		}
		_ = json.NewEncoder(w).Encode(auth.JWKS{Keys: []auth.JWK{auth.Ed25519JWK(kid, pub)}})
	}))
	t.Cleanup(srv.Close)

	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:       "broker-auth",
		Audience:     "broker-api",
		JWKSURL:      srv.URL,
		JWKSCacheTTL: time.Millisecond,
	})
	require.NoError(t, err)
	require.NoError(t, v.Warm(context.Background()))

	tok := signEdDSA(t, priv, kid, "broker-auth", "broker-api", "9", time.Now().Add(time.Minute))
	up.Store(false)
	time.Sleep(2 * time.Millisecond)
	_, err = v.Verify(context.Background(), tok)
	require.NoError(t, err)
}

func TestVerifier_LegacyHS256(t *testing.T) {
	_, pub := mustKey(t)
	v, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:            "broker-auth",
		Audience:          "broker-api",
		AllowLegacyHS256:  true,
		LegacyHS256Secret: "legacy-secret",
		StaticKeys: []auth.PublicKeyEntry{
			{KID: "x", PublicKey: pub},
		},
	})
	require.NoError(t, err)

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": "7",
		"exp":     time.Now().Add(time.Minute).Unix(),
		"iat":     time.Now().Unix(),
	})
	raw, err := tok.SignedString([]byte("legacy-secret"))
	require.NoError(t, err)
	claims, err := v.Verify(context.Background(), raw)
	require.NoError(t, err)
	require.Equal(t, "7", claims.UserID)

	v2, err := auth.NewVerifier(auth.VerifierConfig{
		Issuer:   "broker-auth",
		Audience: "broker-api",
		StaticKeys: []auth.PublicKeyEntry{
			{KID: "x", PublicKey: pub},
		},
	})
	require.NoError(t, err)
	_, err = v2.Verify(context.Background(), raw)
	require.Error(t, err)
}

func TestJWKS_NeverContainsPrivateMaterial(t *testing.T) {
	priv, pub := mustKey(t)
	jwk := auth.Ed25519JWK("k1", pub)
	raw, err := json.Marshal(jwk)
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"d"`)
	require.Equal(t, "OKP", jwk.Kty)
	require.Equal(t, "Ed25519", jwk.Crv)
	_ = priv
}

func mustKey(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv, pub
}

func signEdDSA(t *testing.T, priv ed25519.PrivateKey, kid, iss, aud, userID string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id": userID,
		"roles":   []string{"user"},
		"iss":     iss,
		"aud":     aud,
		"exp":     exp.Unix(),
		"iat":     time.Now().Unix(),
		"nbf":     time.Now().Add(-time.Second).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(priv)
	require.NoError(t, err)
	return raw
}
