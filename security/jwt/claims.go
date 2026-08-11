package jwt

import jwtlib "github.com/golang-jwt/jwt/v5"

type Claims struct {
	UserID      string   `json:"user_id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions,omitempty"`
	FullAccess  bool     `json:"full_access"`
	jwtlib.RegisteredClaims
}

type flexibleClaims struct {
	UserID      any      `json:"user_id"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	FullAccess  bool     `json:"full_access"`
	jwtlib.RegisteredClaims
}
