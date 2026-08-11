package jwks

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

type Document struct {
	Keys []Key `json:"keys"`
}

type Key struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
	Kid string `json:"kid"`
	X   string `json:"x"`
}

type PublicKeyEntry struct {
	KID       string
	PublicKey ed25519.PublicKey
}

type Cache struct {
	url        string
	ttl        time.Duration
	client     *http.Client
	mu         sync.RWMutex
	keys       map[string]ed25519.PublicKey
	fetchedAt  time.Time
	lastETag   string
	refreshing sync.Mutex
}

func NewCache(url string, ttl time.Duration, client *http.Client) *Cache {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Cache{url: strings.TrimSpace(url), ttl: ttl, client: client, keys: make(map[string]ed25519.PublicKey)}
}

func (c *Cache) Seed(entries []PublicKeyEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range entries {
		if e.KID != "" && len(e.PublicKey) == ed25519.PublicKeySize {
			c.keys[e.KID] = append(ed25519.PublicKey(nil), e.PublicKey...)
		}
	}
	if len(c.keys) > 0 {
		c.fetchedAt = time.Now()
	}
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.keys)
}

func (c *Cache) Get(kid string) (ed25519.PublicKey, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	pk, ok := c.keys[kid]
	if !ok {
		return nil, false
	}
	return append(ed25519.PublicKey(nil), pk...), true
}

func (c *Cache) Refresh(ctx context.Context) error {
	if c == nil || c.url == "" {
		return errors.New("jwks url is empty")
	}
	c.refreshing.Lock()
	defer c.refreshing.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return fmt.Errorf("jwks request: %w", err)
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
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}
	parsed, err := Parse(doc)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.keys = parsed
	c.fetchedAt = time.Now()
	if value := resp.Header.Get("ETag"); value != "" {
		c.lastETag = value
	}
	c.mu.Unlock()
	return nil
}

func (c *Cache) EnsureFresh(ctx context.Context) error {
	c.mu.RLock()
	stale := c.fetchedAt.IsZero() || time.Since(c.fetchedAt) > c.ttl
	hasKeys := len(c.keys) > 0
	hasRemote := c.url != ""
	c.mu.RUnlock()
	if hasKeys && (!stale || !hasRemote) {
		return nil
	}
	return c.Refresh(ctx)
}

func (c *Cache) Lookup(ctx context.Context, kid string) (ed25519.PublicKey, error) {
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

func Parse(doc Document) (map[string]ed25519.PublicKey, error) {
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

func Ed25519(kid string, pub ed25519.PublicKey) Key {
	return Key{Kty: "OKP", Crv: "Ed25519", Use: "sig", Alg: "EdDSA", Kid: kid, X: base64.RawURLEncoding.EncodeToString(pub)}
}
