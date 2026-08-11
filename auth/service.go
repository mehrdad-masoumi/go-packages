package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mehrdad-masoumi/go-packages/apperr"
)

const (
	// TokenUseService marks a JWT as a service-to-service identity token.
	TokenUseService = "service"

	// ServiceClaimsKey is the Echo context key for verified ServiceClaims.
	ServiceClaimsKey = "service_claims"

	// ServiceNameHeader is the bootstrap credential header (client credentials).
	ServiceNameHeader = "X-Service-Name"
	// ServiceSecretHeader is the bootstrap credential secret header.
	ServiceSecretHeader = "X-Service-Secret"
	// InternalSecretHeader is the legacy shared-secret S2S header.
	InternalSecretHeader = "X-Internal-Secret"
)

// ServiceClaims is the JWT payload for service identity tokens.
type ServiceClaims struct {
	TokenUse string   `json:"token_use"`
	Scopes   []string `json:"scopes,omitempty"`
	jwt.RegisteredClaims
}

// ServiceName returns the calling service (JWT subject).
func (c *ServiceClaims) ServiceName() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.Subject)
}

// HasScope reports whether the token grants any of the required scopes.
// An empty required list means any authenticated service token is enough.
func (c *ServiceClaims) HasScope(required ...string) bool {
	if c == nil {
		return false
	}
	if len(required) == 0 {
		return true
	}
	have := make(map[string]struct{}, len(c.Scopes))
	for _, s := range c.Scopes {
		have[s] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; ok {
			return true
		}
	}
	return false
}

// ServiceCredential is a trusted caller registered with a destination.
type ServiceCredential struct {
	Name   string
	Secret string
	// Audiences this credential may request when minting service tokens.
	Audiences []string
	// Scopes granted to this service (server-derived; never client-chosen).
	Scopes []string
}

// ServiceRegistry looks up trusted service credentials by name.
type ServiceRegistry struct {
	byName map[string]ServiceCredential
}

// NewServiceRegistry builds a registry. Duplicate names keep the last entry.
func NewServiceRegistry(creds []ServiceCredential) *ServiceRegistry {
	m := make(map[string]ServiceCredential, len(creds))
	for _, c := range creds {
		name := strings.TrimSpace(c.Name)
		secret := strings.TrimSpace(c.Secret)
		if name == "" || secret == "" {
			continue
		}
		c.Name = name
		c.Secret = secret
		m[name] = c
	}
	return &ServiceRegistry{byName: m}
}

// Len returns the number of registered services.
func (r *ServiceRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byName)
}

// Authenticate validates client credentials with constant-time secret compare.
func (r *ServiceRegistry) Authenticate(name, secret string) (ServiceCredential, bool) {
	if r == nil {
		return ServiceCredential{}, false
	}
	name = strings.TrimSpace(name)
	secret = strings.TrimSpace(secret)
	if name == "" || secret == "" {
		return ServiceCredential{}, false
	}
	cred, ok := r.byName[name]
	if !ok {
		return ServiceCredential{}, false
	}
	if subtle.ConstantTimeCompare([]byte(secret), []byte(cred.Secret)) != 1 {
		return ServiceCredential{}, false
	}
	return cred, true
}

// Lookup returns a credential without validating the secret.
func (r *ServiceRegistry) Lookup(name string) (ServiceCredential, bool) {
	if r == nil {
		return ServiceCredential{}, false
	}
	cred, ok := r.byName[strings.TrimSpace(name)]
	return cred, ok
}

// MayRequestAudience reports whether the credential may mint tokens for aud.
func (c ServiceCredential) MayRequestAudience(aud string) bool {
	aud = strings.TrimSpace(aud)
	if aud == "" {
		return false
	}
	if len(c.Audiences) == 0 {
		return true
	}
	for _, a := range c.Audiences {
		if strings.TrimSpace(a) == aud {
			return true
		}
	}
	return false
}

// ServiceTokenIssuer signs short-lived service JWTs (EdDSA).
type ServiceTokenIssuer struct {
	Issuer string
	KeyID  string
	Key    ed25519.PrivateKey
	TTL    time.Duration
}

// Issue creates a service JWT for the given subject, audience and scopes.
func (i *ServiceTokenIssuer) Issue(subject, audience string, scopes []string) (string, time.Time, error) {
	if i == nil || len(i.Key) == 0 {
		return "", time.Time{}, errors.New("service token issuer not configured")
	}
	subject = strings.TrimSpace(subject)
	audience = strings.TrimSpace(audience)
	if subject == "" || audience == "" {
		return "", time.Time{}, errors.New("subject and audience are required")
	}
	ttl := i.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	now := time.Now()
	exp := now.Add(ttl)
	if scopes == nil {
		scopes = []string{}
	}
	claims := &ServiceClaims{
		TokenUse: TokenUseService,
		Scopes:   scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.Issuer,
			Subject:   subject,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	if i.KeyID != "" {
		tok.Header["kid"] = i.KeyID
	}
	signed, err := tok.SignedString(ed25519.PrivateKey(i.Key))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// ServiceVerifier verifies service JWTs against JWKS/static keys.
type ServiceVerifier struct {
	Issuer  string
	jwksURL string
	cache   *JWKSCache
	leeway  time.Duration
}

// NewServiceVerifier builds a verifier for service tokens.
func NewServiceVerifier(cfg VerifierConfig) (*ServiceVerifier, error) {
	cfg.Issuer = strings.TrimSpace(cfg.Issuer)
	cfg.JWKSURL = strings.TrimSpace(cfg.JWKSURL)
	if cfg.Issuer == "" {
		return nil, errors.New("service verifier: issuer is required")
	}
	if cfg.JWKSURL == "" && len(cfg.StaticKeys) == 0 {
		return nil, errors.New("service verifier: jwks_url or static keys required")
	}
	if cfg.Leeway <= 0 {
		cfg.Leeway = 30 * time.Second
	}
	cache := NewJWKSCache(cfg.JWKSURL, cfg.JWKSCacheTTL, cfg.HTTPClient)
	if len(cfg.StaticKeys) > 0 {
		cache.Seed(cfg.StaticKeys)
	}
	return &ServiceVerifier{
		Issuer:  cfg.Issuer,
		jwksURL: cfg.JWKSURL,
		cache:   cache,
		leeway:  cfg.Leeway,
	}, nil
}

// Ready reports whether at least one key is available.
func (v *ServiceVerifier) Ready() bool {
	return v != nil && v.cache != nil && v.cache.Len() > 0
}

// Warm refreshes JWKS.
func (v *ServiceVerifier) Warm(ctx context.Context) error {
	if v == nil || v.cache == nil {
		return errors.New("service verifier: not configured")
	}
	if v.jwksURL == "" {
		if v.Ready() {
			return nil
		}
		return errors.New("service verifier: no keys available")
	}
	err := v.cache.Refresh(ctx)
	if err != nil && !v.Ready() {
		return err
	}
	return nil
}

// Verify validates a service JWT for the expected destination audience.
func (v *ServiceVerifier) Verify(ctx context.Context, rawToken, audience string) (*ServiceClaims, error) {
	if v == nil {
		return nil, unauthenticated("service verifier not configured")
	}
	rawToken = strings.TrimSpace(strings.TrimPrefix(rawToken, "Bearer "))
	audience = strings.TrimSpace(audience)
	if rawToken == "" {
		return nil, unauthenticated("empty token")
	}
	if audience == "" {
		return nil, unauthenticated("audience required")
	}
	_ = v.cache.EnsureFresh(ctx)

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(v.Issuer),
		jwt.WithAudience(audience),
		jwt.WithLeeway(v.leeway),
		jwt.WithExpirationRequired(),
	)
	token, err := parser.ParseWithClaims(rawToken, &ServiceClaims{}, func(t *jwt.Token) (interface{}, error) {
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
	claims, ok := token.Claims.(*ServiceClaims)
	if !ok || !token.Valid {
		return nil, unauthenticated("invalid token")
	}
	if claims.TokenUse != TokenUseService {
		return nil, unauthenticated("not a service token")
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return nil, unauthenticated("missing subject")
	}
	return claims, nil
}

// GetServiceClaims returns verified service claims from Echo context.
func GetServiceClaims(c echo.Context) (*ServiceClaims, error) {
	val := c.Get(ServiceClaimsKey)
	if val == nil {
		return nil, apperr.New("auth.GetServiceClaims").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("unauthenticated")
	}
	claims, ok := val.(*ServiceClaims)
	if !ok || claims == nil || claims.ServiceName() == "" {
		return nil, apperr.New("auth.GetServiceClaims").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("unauthenticated")
	}
	return claims, nil
}

// RequireServiceJWT authenticates callers with a service JWT for the given audience.
// Optional scopes: caller must hold at least one when provided.
func RequireServiceJWT(v *ServiceVerifier, audience string, scopes ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if v == nil {
				return apperr.New("auth.RequireServiceJWT").
					WithKind(apperr.KindUnauthenticated).
					WithMessage("service authentication not configured")
			}
			raw := bearerFromRequest(c)
			claims, err := v.Verify(c.Request().Context(), raw, audience)
			if err != nil {
				return apperr.New("auth.RequireServiceJWT").
					WithKind(apperr.KindUnauthenticated).
					WithMessage("invalid service token")
			}
			if !claims.HasScope(scopes...) {
				return apperr.New("auth.RequireServiceJWT").
					WithKind(apperr.KindForbidden).
					WithMessage("insufficient service scope")
			}
			c.Set(ServiceClaimsKey, claims)
			return next(c)
		}
	}
}

// RequireInternalSecret gates routes with X-Internal-Secret (legacy S2S).
// Empty expected secret rejects all requests (fail closed).
func RequireInternalSecret(expected string) echo.MiddlewareFunc {
	expected = strings.TrimSpace(expected)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if expected == "" {
				return apperr.New("auth.RequireInternalSecret").
					WithKind(apperr.KindUnauthenticated).
					WithMessage("internal s2s secret not configured")
			}
			got := strings.TrimSpace(c.Request().Header.Get(InternalSecretHeader))
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				return apperr.New("auth.RequireInternalSecret").
					WithKind(apperr.KindUnauthenticated).
					WithMessage("unauthorized")
			}
			return next(c)
		}
	}
}

// RequireServiceOrInternalSecret accepts a service JWT for audience OR a legacy
// X-Internal-Secret. Prefer service JWT; keep secret as defense-in-depth / migration.
func RequireServiceOrInternalSecret(v *ServiceVerifier, audience, internalSecret string, scopes ...string) echo.MiddlewareFunc {
	internalSecret = strings.TrimSpace(internalSecret)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := bearerFromRequest(c)
			if raw != "" && v != nil {
				claims, err := v.Verify(c.Request().Context(), raw, audience)
				if err == nil && claims.HasScope(scopes...) {
					c.Set(ServiceClaimsKey, claims)
					return next(c)
				}
			}
			if internalSecret != "" {
				got := strings.TrimSpace(c.Request().Header.Get(InternalSecretHeader))
				if got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(internalSecret)) == 1 {
					return next(c)
				}
			}
			return apperr.New("auth.RequireServiceOrInternalSecret").
				WithKind(apperr.KindUnauthenticated).
				WithMessage("unauthorized")
		}
	}
}

func bearerFromRequest(c echo.Context) string {
	h := strings.TrimSpace(c.Request().Header.Get("Authorization"))
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

type serviceCtxKey struct{}

// ContextWithServiceClaims attaches service identity to a context.
func ContextWithServiceClaims(ctx context.Context, claims *ServiceClaims) context.Context {
	return context.WithValue(ctx, serviceCtxKey{}, claims)
}

// ServiceClaimsFromContext extracts service identity from context.
func ServiceClaimsFromContext(ctx context.Context) (*ServiceClaims, bool) {
	claims, ok := ctx.Value(serviceCtxKey{}).(*ServiceClaims)
	return claims, ok && claims != nil
}

// GRPCMethodAuthorizer returns required scopes for a gRPC full method.
// Return nil/empty to allow any authenticated service. Return a non-nil error
// via a sentinel by using DenyAllGRPCMethod.
type GRPCMethodAuthorizer func(fullMethod string) (scopes []string, deny bool)

// GRPCUnaryServerInterceptor verifies service JWT from authorization metadata.
// Health and reflection methods are skipped. expectedAudience is required.
func GRPCUnaryServerInterceptor(v *ServiceVerifier, expectedAudience string, requiredScopes ...string) grpc.UnaryServerInterceptor {
	return GRPCUnaryServerInterceptorAuthz(v, expectedAudience, func(string) ([]string, bool) {
		return requiredScopes, false
	})
}

// GRPCUnaryServerInterceptorAuthz verifies service JWT and applies per-method scopes.
func GRPCUnaryServerInterceptorAuthz(v *ServiceVerifier, expectedAudience string, authz GRPCMethodAuthorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if isPublicGRPCMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		if v == nil {
			return nil, status.Error(codes.Unauthenticated, "service authentication not configured")
		}
		raw, err := bearerFromMD(ctx)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "missing service token")
		}
		claims, err := v.Verify(ctx, raw, expectedAudience)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid service token")
		}
		scopes := []string(nil)
		if authz != nil {
			var deny bool
			scopes, deny = authz(info.FullMethod)
			if deny {
				return nil, status.Error(codes.PermissionDenied, "method not allowed")
			}
		}
		if !claims.HasScope(scopes...) {
			return nil, status.Error(codes.PermissionDenied, "insufficient service scope")
		}
		return handler(ContextWithServiceClaims(ctx, claims), req)
	}
}

// GRPCUnaryClientInterceptor attaches a service token from TokenSource to outgoing RPCs.
func GRPCUnaryClientInterceptor(src TokenSource) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if src != nil {
			tok, err := src.Token(ctx)
			if err != nil {
				return status.Errorf(codes.Unauthenticated, "service token: %v", err)
			}
			if tok != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tok)
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// TokenSource supplies a service bearer token (typically cached).
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

func bearerFromMD(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		vals = md.Get("Authorization")
	}
	if len(vals) == 0 {
		return "", errors.New("missing authorization")
	}
	raw := strings.TrimSpace(vals[0])
	if len(raw) > 7 && strings.EqualFold(raw[:7], "bearer ") {
		return strings.TrimSpace(raw[7:]), nil
	}
	return "", errors.New("invalid authorization")
}

func isPublicGRPCMethod(fullMethod string) bool {
	switch {
	case strings.HasPrefix(fullMethod, "/grpc.health.v1.Health/"):
		return true
	case strings.HasPrefix(fullMethod, "/grpc.reflection."):
		return true
	default:
		return false
	}
}
