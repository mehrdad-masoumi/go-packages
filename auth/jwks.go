package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// JWKS is a JSON Web Key Set document (RFC 7517).
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK is a single JSON Web Key. Only Ed25519 OKP keys are supported for verification.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
	Kid string `json:"kid"`
	X   string `json:"x"`
}

// PublicKeyEntry is a verified Ed25519 public key keyed by kid.
type PublicKeyEntry struct {
	KID       string
	PublicKey ed25519.PublicKey
}

// JWKSCache is a concurrency-safe cache of JWKS public keys.
type JWKSCache struct {
	url        string
	ttl        time.Duration
	client     *http.Client
	mu         sync.RWMutex
	keys       map[string]ed25519.PublicKey
	fetchedAt  time.Time
	lastETag   string
	refreshing sync.Mutex
}

// NewJWKSCache creates a cache. ttl defaults to 15m when <= 0.
func NewJWKSCache(url string, ttl time.Duration, client *http.Client) *JWKSCache {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &JWKSCache{
		url:    strings.TrimSpace(url),
		ttl:    ttl,
		client: client,
		keys:   make(map[string]ed25519.PublicKey),
	}
}

// Seed installs keys without a network call (e.g. auth-service seeding its own public key).
func (c *JWKSCache) Seed(entries []PublicKeyEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range entries {
		if e.KID == "" || len(e.PublicKey) != ed25519.PublicKeySize {
			continue
		}
		c.keys[e.KID] = e.PublicKey
	}
	c.fetchedAt = time.Now()
}

// Len returns the number of cached keys.
func (c *JWKSCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.keys)
}

// Get returns a public key for kid if present.
func (c *JWKSCache) Get(kid string) (ed25519.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pk, ok := c.keys[kid]
	return pk, ok
}

// Refresh fetches JWKS from the configured URL and replaces the cache.
func (c *JWKSCache) Refresh(ctx context.Context) error {
	if c.url == "" {
		return errors.New("jwks url is empty")
	}
	c.refreshing.Lock()
	defer c.refreshing.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return err
	}
	c.mu.RLock()
	etag := c.lastETag
	c.mu.RUnlock()
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		c.mu.Lock()
		c.fetchedAt = time.Now()
		c.mu.Unlock()
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("jwks fetch: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("jwks read: %w", err)
	}
	var doc JWKS
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}
	parsed, err := ParseJWKS(doc)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.keys = parsed
	c.fetchedAt = time.Now()
	if v := resp.Header.Get("ETag"); v != "" {
		c.lastETag = v
	}
	c.mu.Unlock()
	return nil
}

// EnsureFresh refreshes when the TTL has elapsed. Failures leave existing keys intact.
func (c *JWKSCache) EnsureFresh(ctx context.Context) error {
	c.mu.RLock()
	stale := c.fetchedAt.IsZero() || time.Since(c.fetchedAt) > c.ttl
	hasKeys := len(c.keys) > 0
	c.mu.RUnlock()
	if !stale && hasKeys {
		return nil
	}
	return c.Refresh(ctx)
}

// Lookup returns the key for kid. On miss, refreshes JWKS once and retries.
// Known kids never require a network call.
func (c *JWKSCache) Lookup(ctx context.Context, kid string) (ed25519.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("missing kid")
	}
	if pk, ok := c.Get(kid); ok {
		return pk, nil
	}
	if err := c.Refresh(ctx); err != nil {
		if pk, ok := c.Get(kid); ok {
			return pk, nil
		}
		return nil, err
	}
	if pk, ok := c.Get(kid); ok {
		return pk, nil
	}
	return nil, fmt.Errorf("unknown kid")
}

// ParseJWKS converts a JWKS document into a kid→public-key map. Unsupported keys are skipped.
func ParseJWKS(doc JWKS) (map[string]ed25519.PublicKey, error) {
	out := make(map[string]ed25519.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if !strings.EqualFold(k.Kty, "OKP") || !strings.EqualFold(k.Crv, "Ed25519") {
			continue
		}
		if k.Alg != "" && !strings.EqualFold(k.Alg, "EdDSA") {
			continue
		}
		if k.Kid == "" || k.X == "" {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, fmt.Errorf("jwk %s: invalid x: %w", k.Kid, err)
		}
		if len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("jwk %s: invalid public key length", k.Kid)
		}
		out[k.Kid] = ed25519.PublicKey(raw)
	}
	if len(out) == 0 {
		return nil, errors.New("jwks contains no usable Ed25519 keys")
	}
	return out, nil
}

// Ed25519JWK builds a public JWK for an Ed25519 key.
func Ed25519JWK(kid string, pub ed25519.PublicKey) JWK {
	return JWK{
		Kty: "OKP",
		Crv: "Ed25519",
		Use: "sig",
		Alg: "EdDSA",
		Kid: kid,
		X:   base64.RawURLEncoding.EncodeToString(pub),
	}
}
