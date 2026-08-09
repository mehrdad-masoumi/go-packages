package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mehrdad-masoumi/go-packages/apperr"
)

// TokenVerifier verifies access tokens.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*Claims, error)
	Ready() bool
	Warm(ctx context.Context) error
}

// VerifierConfig configures EdDSA JWKS verification with optional legacy HS256.
type VerifierConfig struct {
	Issuer   string
	Audience string

	JWKSURL      string
	JWKSCacheTTL time.Duration
	HTTPClient   *http.Client

	// StaticKeys seeds the JWKS cache (auth-service local keys; optional for others).
	StaticKeys []PublicKeyEntry

	Leeway time.Duration

	// AllowLegacyHS256 enables temporary HS256 verification during migration.
	// Must be paired with LegacyHS256Secret. Deprecated — disable after access TTL window.
	AllowLegacyHS256  bool
	LegacyHS256Secret string
}

// Verifier validates JWTs using a local JWKS cache (EdDSA) and optional legacy HS256.
type Verifier struct {
	cfg   VerifierConfig
	cache *JWKSCache
}

// NewVerifier builds a verifier. Fails fast on invalid security configuration.
func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.Audience = strings.TrimSpace(cfg.Audience)
	cfg.JWKSURL = strings.TrimSpace(cfg.JWKSURL)
	cfg.LegacyHS256Secret = strings.TrimSpace(cfg.LegacyHS256Secret)

	if cfg.Issuer == "" {
		return nil, errors.New("auth verifier: issuer is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("auth verifier: audience is required")
	}
	if cfg.JWKSURL == "" && len(cfg.StaticKeys) == 0 {
		return nil, errors.New("auth verifier: jwks_url or static keys required")
	}
	if cfg.AllowLegacyHS256 && cfg.LegacyHS256Secret == "" {
		return nil, errors.New("auth verifier: legacy HS256 enabled but secret empty")
	}
	if !cfg.AllowLegacyHS256 {
		cfg.LegacyHS256Secret = ""
	}
	if cfg.Leeway <= 0 {
		cfg.Leeway = 30 * time.Second
	}

	cache := NewJWKSCache(cfg.JWKSURL, cfg.JWKSCacheTTL, cfg.HTTPClient)
	if len(cfg.StaticKeys) > 0 {
		cache.Seed(cfg.StaticKeys)
	}

	return &Verifier{cfg: cfg, cache: cache}, nil
}

// Ready reports whether at least one verification key is cached.
func (v *Verifier) Ready() bool {
	return v != nil && v.cache != nil && v.cache.Len() > 0
}

// Warm loads JWKS. Safe to call at startup; leaves existing keys on failure.
func (v *Verifier) Warm(ctx context.Context) error {
	if v.cfg.JWKSURL == "" {
		if v.Ready() {
			return nil
		}
		return errors.New("auth verifier: no keys available")
	}
	err := v.cache.Refresh(ctx)
	if err != nil && !v.Ready() {
		return err
	}
	return nil
}

func mapJWTError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, jwt.ErrTokenExpired) {
		return apperr.New("auth.ParseToken").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("token expired")
	}
	return unauthenticated("invalid token")
}

func unauthenticated(msg string) error {
	return apperr.New("auth.ParseToken").
		WithKind(apperr.KindUnauthenticated).
		WithMessage(msg)
}
