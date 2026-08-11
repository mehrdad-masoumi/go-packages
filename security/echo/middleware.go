package echoauth

import (
	"strings"

	"github.com/labstack/echo/v4"
	apperr "github.com/mehrdad-masoumi/go-packages/errors"
	securityjwt "github.com/mehrdad-masoumi/go-packages/security/jwt"
	"github.com/mehrdad-masoumi/go-packages/security/s2s"
)

const (
	ClaimsKey          = "claims"
	ServiceIdentityKey = "service_identity"
)

func JWT(verifier securityjwt.TokenVerifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if verifier == nil {
				return apperr.New("security.echo.JWT").WithKind(apperr.KindUnauthenticated).WithMessage("unauthenticated")
			}
			raw := accessToken(c)
			claims, err := verifier.Verify(c.Request().Context(), raw)
			if err != nil {
				return err
			}
			c.Set(ClaimsKey, claims)
			return next(c)
		}
	}
}

func GetClaims(c echo.Context) (*securityjwt.Claims, error) {
	claims, ok := c.Get(ClaimsKey).(*securityjwt.Claims)
	if !ok || claims == nil || strings.TrimSpace(claims.UserID) == "" {
		return nil, apperr.New("security.echo.GetClaims").WithKind(apperr.KindUnauthenticated).WithMessage("unauthenticated")
	}
	return claims, nil
}

func RequireAdmin(adminRoles []string) echo.MiddlewareFunc {
	roleSet := make(map[string]struct{}, len(adminRoles))
	for _, role := range adminRoles {
		roleSet[strings.ToLower(strings.TrimSpace(role))] = struct{}{}
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
			for _, role := range claims.Roles {
				if _, ok := roleSet[strings.ToLower(role)]; ok {
					return next(c)
				}
			}
			return forbidden("security.echo.RequireAdmin")
		}
	}
}

func RequirePermission(keys ...string) echo.MiddlewareFunc {
	required := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key = strings.TrimSpace(key); key != "" {
			required[key] = struct{}{}
		}
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
			for _, permission := range claims.Permissions {
				if _, ok := required[permission]; ok {
					return next(c)
				}
			}
			return forbidden("security.echo.RequirePermission")
		}
	}
}

func HasPermission(claims *securityjwt.Claims, keys ...string) bool {
	if claims == nil {
		return false
	}
	if claims.FullAccess {
		return true
	}
	set := make(map[string]struct{}, len(claims.Permissions))
	for _, p := range claims.Permissions {
		set[p] = struct{}{}
	}
	for _, key := range keys {
		if _, ok := set[key]; ok {
			return true
		}
	}
	return false
}

func S2S(verifier s2s.TokenVerifier) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if verifier == nil {
				return apperr.New("security.echo.S2S").WithKind(apperr.KindUnauthenticated).WithMessage("unauthenticated service")
			}
			identity, err := verifier.Verify(c.Request().Context(), bearer(c.Request().Header.Get(echo.HeaderAuthorization)))
			if err != nil {
				return err
			}
			c.Set(ServiceIdentityKey, identity)
			ctx := s2s.ContextWithIdentity(c.Request().Context(), *identity)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}

func GetServiceIdentity(c echo.Context) (*s2s.Identity, error) {
	if identity, ok := c.Get(ServiceIdentityKey).(*s2s.Identity); ok && identity != nil {
		return identity, nil
	}
	if identity, ok := s2s.IdentityFromContext(c.Request().Context()); ok {
		return &identity, nil
	}
	return nil, apperr.New("security.echo.GetServiceIdentity").WithKind(apperr.KindUnauthenticated).WithMessage("unauthenticated service")
}

func RequireServiceScopes(scopes ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			identity, err := GetServiceIdentity(c)
			if err != nil {
				return err
			}
			// Empty scopes means any authenticated service identity is enough.
			if len(scopes) > 0 && !identity.HasAnyScope(scopes...) {
				return forbidden("security.echo.RequireServiceScopes")
			}
			return next(c)
		}
	}
}

func accessToken(c echo.Context) string {
	if raw := bearer(c.Request().Header.Get(echo.HeaderAuthorization)); raw != "" {
		return raw
	}
	for _, name := range []string{"access_token", "admin_access_token"} {
		if cookie, err := c.Cookie(name); err == nil && cookie.Value != "" {
			return cookie.Value
		}
	}
	return ""
}

func bearer(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 7 && strings.EqualFold(value[:7], "Bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}

func forbidden(op string) error {
	return apperr.New(op).WithKind(apperr.KindForbidden).WithMessage("forbidden")
}
