package auth

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Verify parses and validates an access token (EdDSA via JWKS, optional legacy HS256).
func (v *Verifier) Verify(ctx context.Context, rawToken string) (*Claims, error) {
	rawToken = strings.TrimSpace(strings.TrimPrefix(rawToken, "Bearer "))
	if rawToken == "" {
		return nil, unauthenticated("empty token")
	}

	// Best-effort TTL refresh; known kids keep working if auth-service is down.
	_ = v.cache.EnsureFresh(ctx)

	unverified, _, err := jwt.NewParser().ParseUnverified(rawToken, &flexibleClaims{})
	if err != nil {
		return nil, unauthenticated("invalid token")
	}
	alg := unverified.Method.Alg()

	switch alg {
	case jwt.SigningMethodEdDSA.Alg():
		return v.verifyEdDSA(ctx, rawToken)
	case jwt.SigningMethodHS256.Alg():
		return v.verifyLegacyHS256(rawToken)
	default:
		return nil, unauthenticated("invalid token")
	}
}

func (v *Verifier) verifyEdDSA(ctx context.Context, rawToken string) (*Claims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithLeeway(v.cfg.Leeway),
		jwt.WithExpirationRequired(),
	)

	token, err := parser.ParseWithClaims(rawToken, &flexibleClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodEdDSA.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid")
		}
		pk, err := v.cache.Lookup(ctx, kid)
		if err != nil {
			return nil, err
		}
		return ed25519.PublicKey(pk), nil
	})
	if err != nil {
		return nil, mapJWTError(err)
	}
	return claimsFromToken(token)
}

func (v *Verifier) verifyLegacyHS256(rawToken string) (*Claims, error) {
	if !v.cfg.AllowLegacyHS256 || v.cfg.LegacyHS256Secret == "" {
		return nil, unauthenticated("invalid token")
	}
	// Legacy tokens predate iss/aud; do not require them. Still enforce alg + exp + signature.
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithLeeway(v.cfg.Leeway),
		jwt.WithExpirationRequired(),
	)
	token, err := parser.ParseWithClaims(rawToken, &flexibleClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(v.cfg.LegacyHS256Secret), nil
	})
	if err != nil {
		return nil, mapJWTError(err)
	}
	return claimsFromToken(token)
}

func claimsFromToken(token *jwt.Token) (*Claims, error) {
	fc, ok := token.Claims.(*flexibleClaims)
	if !ok || !token.Valid {
		return nil, unauthenticated("invalid token")
	}
	claims := &Claims{
		Roles:            fc.Roles,
		Permissions:      fc.Permissions,
		FullAccess:       fc.FullAccess,
		RegisteredClaims: fc.RegisteredClaims,
	}
	switch val := fc.UserID.(type) {
	case string:
		claims.UserID = val
	case float64:
		claims.UserID = fmt.Sprintf("%.0f", val)
	default:
		return nil, unauthenticated("invalid token")
	}
	if claims.UserID == "" {
		return nil, unauthenticated("invalid token")
	}
	return claims, nil
}
