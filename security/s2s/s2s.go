package s2s

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	apperr "github.com/mehrdad-masoumi/go-packages/errors"
	"github.com/mehrdad-masoumi/go-packages/security/jwks"
)

type Identity struct {
	Subject  string
	Audience []string
	Scopes   []string
}

func (i Identity) HasScope(scope string) bool {
	return slices.Contains(i.Scopes, scope)
}

func (i Identity) HasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if i.HasScope(scope) {
			return true
		}
	}
	return false
}

type Claims struct {
	Scopes []string `json:"scopes,omitempty"`
	jwtlib.RegisteredClaims
}

type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (*Identity, error)
	Ready() bool
	Warm(ctx context.Context) error
}

type VerifierConfig struct {
	Issuer          string
	Audience        string
	JWKSURL         string
	JWKSCacheTTL    time.Duration
	HTTPClient      *http.Client
	StaticKeys      []jwks.PublicKeyEntry
	Leeway          time.Duration
	AllowedSubjects []string
}

type Verifier struct {
	cfg     VerifierConfig
	cache   *jwks.Cache
	allowed map[string]struct{}
}

func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.Audience = strings.TrimSpace(cfg.Audience)
	cfg.JWKSURL = strings.TrimSpace(cfg.JWKSURL)
	if cfg.Issuer == "" {
		return nil, errors.New("s2s verifier: issuer is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("s2s verifier: audience is required")
	}
	if cfg.JWKSURL == "" && len(cfg.StaticKeys) == 0 {
		return nil, errors.New("s2s verifier: jwks URL or static keys required")
	}
	if cfg.Leeway <= 0 {
		cfg.Leeway = 30 * time.Second
	}
	cache := jwks.NewCache(cfg.JWKSURL, cfg.JWKSCacheTTL, cfg.HTTPClient)
	cache.Seed(cfg.StaticKeys)
	allowed := make(map[string]struct{}, len(cfg.AllowedSubjects))
	for _, subject := range cfg.AllowedSubjects {
		subject = strings.TrimSpace(subject)
		if subject != "" {
			allowed[subject] = struct{}{}
		}
	}
	return &Verifier{cfg: cfg, cache: cache, allowed: allowed}, nil
}

func (v *Verifier) Ready() bool {
	return v != nil && v.cache != nil && v.cache.Len() > 0
}

func (v *Verifier) Warm(ctx context.Context) error {
	if v == nil || v.cache == nil {
		return errors.New("s2s verifier is nil")
	}
	if v.cfg.JWKSURL == "" {
		if v.Ready() {
			return nil
		}
		return errors.New("s2s verifier: no keys available")
	}
	if err := v.cache.Refresh(ctx); err != nil && !v.Ready() {
		return err
	}
	return nil
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (*Identity, error) {
	if v == nil {
		return nil, unauthenticated()
	}
	rawToken = trimBearer(rawToken)
	if rawToken == "" {
		return nil, unauthenticated()
	}
	_ = v.cache.EnsureFresh(ctx)
	parser := jwtlib.NewParser(
		jwtlib.WithValidMethods([]string{jwtlib.SigningMethodEdDSA.Alg()}),
		jwtlib.WithIssuer(v.cfg.Issuer),
		jwtlib.WithAudience(v.cfg.Audience),
		jwtlib.WithLeeway(v.cfg.Leeway),
		jwtlib.WithExpirationRequired(),
	)
	token, err := parser.ParseWithClaims(rawToken, &Claims{}, func(t *jwtlib.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("missing kid")
		}
		return v.cache.Lookup(ctx, kid)
	})
	if err != nil {
		return nil, unauthenticated()
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || strings.TrimSpace(claims.Subject) == "" {
		return nil, unauthenticated()
	}
	if len(v.allowed) > 0 {
		if _, ok := v.allowed[claims.Subject]; !ok {
			return nil, apperr.New("security.s2s.Verify").WithKind(apperr.KindForbidden).WithMessage("service not allowed")
		}
	}
	return &Identity{Subject: claims.Subject, Audience: []string(claims.Audience), Scopes: append([]string(nil), claims.Scopes...)}, nil
}

type SignerConfig struct {
	Issuer     string
	KeyID      string
	PrivateKey ed25519.PrivateKey
	TTL        time.Duration
}

type Signer struct {
	cfg SignerConfig
}

func NewSigner(cfg SignerConfig) (*Signer, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.KeyID = strings.TrimSpace(cfg.KeyID)
	if cfg.Issuer == "" || cfg.KeyID == "" {
		return nil, errors.New("s2s signer: issuer and key ID are required")
	}
	if len(cfg.PrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("s2s signer: valid Ed25519 private key required")
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 2 * time.Minute
	}
	return &Signer{cfg: cfg}, nil
}

func (s *Signer) Mint(subject, audience string, scopes []string) (string, error) {
	tok, _, err := s.Issue(subject, audience, scopes)
	return tok, err
}

// Issue creates a service JWT and returns its absolute expiry (for expires_in responses).
func (s *Signer) Issue(subject, audience string, scopes []string) (string, time.Time, error) {
	if s == nil {
		return "", time.Time{}, errors.New("s2s signer: not configured")
	}
	subject = strings.TrimSpace(subject)
	audience = strings.TrimSpace(audience)
	if subject == "" || audience == "" {
		return "", time.Time{}, errors.New("s2s signer: subject and audience are required")
	}
	now := time.Now().UTC()
	exp := now.Add(s.cfg.TTL)
	claims := Claims{
		Scopes: append([]string(nil), scopes...),
		RegisteredClaims: jwtlib.RegisteredClaims{
			Issuer:    s.cfg.Issuer,
			Subject:   subject,
			Audience:  jwtlib.ClaimStrings{audience},
			IssuedAt:  jwtlib.NewNumericDate(now),
			NotBefore: jwtlib.NewNumericDate(now.Add(-5 * time.Second)),
			ExpiresAt: jwtlib.NewNumericDate(exp),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.cfg.KeyID
	signed, err := token.SignedString(s.cfg.PrivateKey)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

type identityKey struct{}

func ContextWithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}

func AuthorizationHeader(token string) string {
	token = trimBearer(token)
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

func trimBearer(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 7 && strings.EqualFold(raw[:7], "Bearer ") {
		return strings.TrimSpace(raw[7:])
	}
	return raw
}

func unauthenticated() error {
	return apperr.New("security.s2s.Verify").WithKind(apperr.KindUnauthenticated).WithMessage("unauthenticated service")
}
