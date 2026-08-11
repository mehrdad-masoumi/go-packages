package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

func TestServiceRegistryAuthenticate(t *testing.T) {
	reg := NewServiceRegistry([]ServiceCredential{
		{Name: "user-service", Secret: "s3cret", Audiences: []string{"auth-service"}, Scopes: []string{"auth.tokens.issue"}},
	})
	_, ok := reg.Authenticate("user-service", "wrong")
	require.False(t, ok)
	cred, ok := reg.Authenticate("user-service", "s3cret")
	require.True(t, ok)
	require.True(t, cred.MayRequestAudience("auth-service"))
	require.False(t, cred.MayRequestAudience("wallet-service"))
	require.Equal(t, []string{"auth.tokens.issue"}, cred.Scopes)
}

func TestServiceTokenIssueAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	issuer := &ServiceTokenIssuer{
		Issuer: "broker-auth",
		KeyID:  "test-key",
		Key:    priv,
		TTL:    time.Minute,
	}
	tok, exp, err := issuer.Issue("user-service", "auth-service", []string{"auth.tokens.issue"})
	require.NoError(t, err)
	require.False(t, exp.IsZero())

	v, err := NewServiceVerifier(VerifierConfig{
		Issuer:     "broker-auth",
		StaticKeys: []PublicKeyEntry{{KID: "test-key", PublicKey: pub}},
	})
	require.NoError(t, err)

	claims, err := v.Verify(context.Background(), tok, "auth-service")
	require.NoError(t, err)
	require.Equal(t, "user-service", claims.ServiceName())
	require.True(t, claims.HasScope("auth.tokens.issue"))
	require.False(t, claims.HasScope("other"))

	_, err = v.Verify(context.Background(), tok, "wallet-service")
	require.Error(t, err)
}

func TestRequireInternalSecretFailClosed(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	h := RequireInternalSecret("")(func(echo.Context) error {
		called = true
		return nil
	})
	err := h(c)
	require.Error(t, err)
	require.False(t, called)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set(InternalSecretHeader, "secret")
	c2 := e.NewContext(req2, httptest.NewRecorder())
	ok := false
	h2 := RequireInternalSecret("secret")(func(echo.Context) error {
		ok = true
		return nil
	})
	require.NoError(t, h2(c2))
	require.True(t, ok)
}

func TestRequireServiceJWTRejectsMissingToken(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_ = priv
	v, err := NewServiceVerifier(VerifierConfig{
		Issuer:     "broker-auth",
		StaticKeys: []PublicKeyEntry{{KID: "k", PublicKey: pub}},
	})
	require.NoError(t, err)

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/tokens", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	called := false
	h := RequireServiceJWT(v, "auth-service", "auth.tokens.issue")(func(echo.Context) error {
		called = true
		return nil
	})
	require.Error(t, h(c))
	require.False(t, called)
}
