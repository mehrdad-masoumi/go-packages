package jwt

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	apperr "github.com/mehrdad-masoumi/go-packages/errors"
	"github.com/mehrdad-masoumi/go-packages/security/jwks"
)

type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*Claims, error)
	Ready() bool
	Warm(ctx context.Context) error
}

type VerifierConfig struct {
	Issuer   string
	Audience string

	JWKSURL      string
	JWKSCacheTTL time.Duration
	HTTPClient   *http.Client
	StaticKeys   []jwks.PublicKeyEntry

	Leeway time.Duration

	// RequiredTokenUse is the expected token_use claim. Resource services must
	// require TokenUseAccess so refresh JWTs cannot be presented as access tokens.
	// Empty defaults to TokenUseAccess.
	RequiredTokenUse string

	// Deprecated migration-only fallback. Production callers should leave this false.
	AllowLegacyHS256  bool
	LegacyHS256Secret string
}

type Verifier struct {
	cfg   VerifierConfig
	cache *jwks.Cache
}

func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.Audience = strings.TrimSpace(cfg.Audience)
	cfg.JWKSURL = strings.TrimSpace(cfg.JWKSURL)
	cfg.LegacyHS256Secret = strings.TrimSpace(cfg.LegacyHS256Secret)
	if cfg.Issuer == "" {
		return nil, errors.New("jwt verifier: issuer is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("jwt verifier: audience is required")
	}
	if cfg.JWKSURL == "" && len(cfg.StaticKeys) == 0 {
		return nil, errors.New("jwt verifier: jwks URL or static keys required")
	}
	if cfg.AllowLegacyHS256 && cfg.LegacyHS256Secret == "" {
		return nil, errors.New("jwt verifier: legacy HS256 enabled but secret empty")
	}
	if !cfg.AllowLegacyHS256 {
		cfg.LegacyHS256Secret = ""
	}
	if cfg.Leeway <= 0 {
		cfg.Leeway = 30 * time.Second
	}
	if strings.TrimSpace(cfg.RequiredTokenUse) == "" {
		cfg.RequiredTokenUse = TokenUseAccess
	}
	cache := jwks.NewCache(cfg.JWKSURL, cfg.JWKSCacheTTL, cfg.HTTPClient)
	cache.Seed(cfg.StaticKeys)
	return &Verifier{cfg: cfg, cache: cache}, nil
}

func (v *Verifier) Ready() bool {
	return v != nil && v.cache != nil && v.cache.Len() > 0
}

func (v *Verifier) Warm(ctx context.Context) error {
	if v == nil || v.cache == nil {
		return errors.New("jwt verifier is nil")
	}
	if v.cfg.JWKSURL == "" {
		if v.Ready() {
			return nil
		}
		return errors.New("jwt verifier: no keys available")
	}
	if err := v.cache.Refresh(ctx); err != nil && !v.Ready() {
		return err
	}
	return nil
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (*Claims, error) {
	if v == nil {
		return nil, unauthenticated("invalid token")
	}
	rawToken = trimBearer(rawToken)
	if rawToken == "" {
		return nil, unauthenticated("empty token")
	}
	// Refresh failure is tolerated only while a cached key can still validate the token.
	_ = v.cache.EnsureFresh(ctx)

	unverified, _, err := jwtlib.NewParser().ParseUnverified(rawToken, &flexibleClaims{})
	if err != nil {
		return nil, unauthenticated("invalid token")
	}
	switch unverified.Method.Alg() {
	case jwtlib.SigningMethodEdDSA.Alg():
		return v.verifyEdDSA(ctx, rawToken)
	case jwtlib.SigningMethodHS256.Alg():
		return v.verifyLegacyHS256(rawToken)
	default:
		return nil, unauthenticated("invalid token")
	}
}

func (v *Verifier) verifyEdDSA(ctx context.Context, rawToken string) (*Claims, error) {
	parser := jwtlib.NewParser(
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodEdDSA.Alg()}),
		jwtlib.WithIssuer(v.cfg.Issuer),
		jwtlib.WithAudience(v.cfg.Audience),
		jwtlib.WithLeeway(v.cfg.Leeway),
		jwtlib.WithExpirationRequired(),
	)
	token, err := parser.ParseWithClaims(rawToken, &flexibleClaims{}, func(t *jwtlib.Token) (any, error) {
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
	return v.enforceTokenUse(claimsFromToken(token))
}

func (v *Verifier) verifyLegacyHS256(rawToken string) (*Claims, error) {
	if !v.cfg.AllowLegacyHS256 || v.cfg.LegacyHS256Secret == "" {
		return nil, unauthenticated("invalid token")
	}
	parser := jwtlib.NewParser(
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}),
		jwtlib.WithLeeway(v.cfg.Leeway),
		jwtlib.WithExpirationRequired(),
	)
	token, err := parser.ParseWithClaims(rawToken, &flexibleClaims{}, func(t *jwtlib.Token) (any, error) {
		return []byte(v.cfg.LegacyHS256Secret), nil
	})
	if err != nil {
		return nil, mapJWTError(err)
	}
	return v.enforceTokenUse(claimsFromToken(token))
}

func (v *Verifier) enforceTokenUse(claims *Claims, err error) (*Claims, error) {
	if err != nil {
		return nil, err
	}
	required := TokenUseAccess
	if v != nil && strings.TrimSpace(v.cfg.RequiredTokenUse) != "" {
		required = strings.TrimSpace(v.cfg.RequiredTokenUse)
	}
	if claims.TokenUse != required {
		return nil, unauthenticated("invalid token use")
	}
	return claims, nil
}

func claimsFromToken(token *jwtlib.Token) (*Claims, error) {
	fc, ok := token.Claims.(*flexibleClaims)
	if !ok || !token.Valid {
		return nil, unauthenticated("invalid token")
	}
	out := &Claims{
		Roles:            fc.Roles,
		Permissions:      fc.Permissions,
		FullAccess:       fc.FullAccess,
		RegisteredClaims: fc.RegisteredClaims,
	}
	switch value := fc.UserID.(type) {
	case string:
		out.UserID = value
	case float64:
		out.UserID = fmt.Sprintf("%.0f", value)
	default:
		return nil, unauthenticated("invalid token")
	}
	if strings.TrimSpace(out.UserID) == "" {
		return nil, unauthenticated("invalid token")
	}
	out.TokenUse = strings.TrimSpace(fc.TokenUse)
	return out, nil
}

func mapJWTError(err error) error {
	if errors.Is(err, jwtlib.ErrTokenExpired) {
		return unauthenticated("token expired")
	}
	return unauthenticated("invalid token")
}

func unauthenticated(msg string) error {
	return apperr.New("security.jwt.Verify").WithKind(apperr.KindUnauthenticated).WithMessage(msg)
}

func trimBearer(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 7 && strings.EqualFold(raw[:7], "Bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return raw
}

// ParseAccessToken validates a legacy HS256 token. Migration/testing only.
func ParseAccessToken(rawToken, secret string) (*Claims, error) {
	v := &Verifier{cfg: VerifierConfig{AllowLegacyHS256: true, LegacyHS256Secret: secret, Leeway: 30 * time.Second}}
	return v.verifyLegacyHS256(trimBearer(rawToken))
}
