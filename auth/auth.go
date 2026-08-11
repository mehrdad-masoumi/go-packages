package auth

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"

	"github.com/mehrdad-masoumi/go-packages/apperr"
)

const ClaimsKey = "claims"

type Claims struct {
	UserID      string   `json:"user_id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions,omitempty"`
	FullAccess  bool     `json:"full_access"`
	jwt.RegisteredClaims
}

type flexibleClaims struct {
	UserID      interface{} `json:"user_id"`
	Roles       []string    `json:"roles"`
	Permissions []string    `json:"permissions"`
	FullAccess  bool        `json:"full_access"`
	jwt.RegisteredClaims
}

// ParseAccessToken validates an HS256 access token with a shared secret.
// Deprecated: use Verifier.Verify with EdDSA/JWKS. Kept for tests and emergency fallbacks.
func ParseAccessToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &flexibleClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperr.New("auth.ParseToken").
				WithKind(apperr.KindUnauthenticated).
				WithMessage("token expired")
		}
		return nil, apperr.New("auth.ParseToken").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("invalid token")
	}

	fc, ok := token.Claims.(*flexibleClaims)
	if !ok || !token.Valid {
		return nil, apperr.New("auth.ParseToken").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("invalid token")
	}

	claims := &Claims{
		Roles:            fc.Roles,
		Permissions:      fc.Permissions,
		FullAccess:       fc.FullAccess,
		RegisteredClaims: fc.RegisteredClaims,
	}
	switch v := fc.UserID.(type) {
	case string:
		claims.UserID = v
	case float64:
		claims.UserID = fmt.Sprintf("%.0f", v)
	default:
		return nil, apperr.New("auth.ParseToken").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("invalid token")
	}
	return claims, nil
}

// JWTMiddlewareHS256 is the legacy shared-secret middleware.
// Deprecated: use JWTMiddleware with a Verifier.
func JWTMiddlewareHS256(secret string) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		ContextKey:    ClaimsKey,
		SigningKey:    []byte(secret),
		SigningMethod: "HS256",
		TokenLookup:   "header:Authorization:Bearer ,cookie:access_token,cookie:admin_access_token",
		ParseTokenFunc: func(_ echo.Context, authHeader string) (interface{}, error) {
			return ParseAccessToken(authHeader, secret)
		},
	})
}

// JWTMiddleware validates Bearer/cookie access tokens using the shared Verifier (EdDSA/JWKS).
func JWTMiddleware(v TokenVerifier) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		ContextKey:  ClaimsKey,
		TokenLookup: "header:Authorization:Bearer ,cookie:access_token,cookie:admin_access_token",
		ParseTokenFunc: func(c echo.Context, authHeader string) (interface{}, error) {
			return v.Verify(c.Request().Context(), authHeader)
		},
	})
}

func GetClaims(c echo.Context) (*Claims, error) {
	val := c.Get(ClaimsKey)
	if val == nil {
		return nil, apperr.New("auth.GetClaims").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("unauthenticated")
	}
	claims, ok := val.(*Claims)
	if !ok || claims == nil || claims.UserID == "" {
		return nil, apperr.New("auth.GetClaims").
			WithKind(apperr.KindUnauthenticated).
			WithMessage("unauthenticated")
	}
	return claims, nil
}

func RequireAdmin(adminRoles []string) echo.MiddlewareFunc {
	roleSet := map[string]struct{}{}
	for _, r := range adminRoles {
		roleSet[strings.ToLower(r)] = struct{}{}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims, err := GetClaims(c)
			if err != nil {
				return err
			}
			if claims.FullAccess {
				return next(c)
			}
			for _, r := range claims.Roles {
				if _, ok := roleSet[strings.ToLower(r)]; ok {
					return next(c)
				}
			}
			return apperr.New("auth.RequireAdmin").
				WithKind(apperr.KindForbidden).
				WithMessage("forbidden")
		}
	}
}

// RequirePermission allows the request when claims.FullAccess is set or the
// caller holds any of the required permission keys in claims.Permissions.
func RequirePermission(keys ...string) echo.MiddlewareFunc {
	required := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		required[k] = struct{}{}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims, err := GetClaims(c)
			if err != nil {
				return err
			}
			if claims.FullAccess {
				return next(c)
			}
			if len(required) == 0 {
				return apperr.New("auth.RequirePermission").
					WithKind(apperr.KindForbidden).
					WithMessage("forbidden")
			}
			for _, p := range claims.Permissions {
				if _, ok := required[p]; ok {
					return next(c)
				}
			}
			return apperr.New("auth.RequirePermission").
				WithKind(apperr.KindForbidden).
				WithMessage("forbidden")
		}
	}
}

// HasPermission reports whether claims grant full access or any of the keys.
func HasPermission(claims *Claims, keys ...string) bool {
	if claims == nil {
		return false
	}
	if claims.FullAccess {
		return true
	}
	if len(keys) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(claims.Permissions))
	for _, p := range claims.Permissions {
		set[p] = struct{}{}
	}
	for _, k := range keys {
		if _, ok := set[k]; ok {
			return true
		}
	}
	return false
}
