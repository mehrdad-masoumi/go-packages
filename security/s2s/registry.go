package s2s

import (
	"crypto/subtle"
	"strings"
)

const (
	// ServiceNameHeader is the bootstrap credential header (client credentials).
	ServiceNameHeader = "X-Service-Name"
	// ServiceSecretHeader is the bootstrap credential secret header.
	ServiceSecretHeader = "X-Service-Secret"
)

// ServiceCredential is a trusted caller registered for client-credentials minting.
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
