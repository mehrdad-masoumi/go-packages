package s2s

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	obsmetrics "github.com/mehrdad-masoumi/go-packages/observability/metrics"
)

// ServiceTokenClient exchanges client credentials for short-lived service JWTs
// from auth-service and caches them until near expiry.
type ServiceTokenClient struct {
	baseURL    string
	service    string
	secret     string
	audience   string
	httpClient *http.Client

	mu            sync.Mutex
	token         string
	tokenAudience string
	expiresAt     time.Time
}

// ServiceTokenClientConfig configures ServiceTokenClient.
type ServiceTokenClientConfig struct {
	BaseURL       string
	ServiceName   string
	ServiceSecret string
	Audience      string
	Timeout       time.Duration
	HTTPClient    *http.Client
}

// NewServiceTokenClient builds a caching TokenSource for S2S calls.
func NewServiceTokenClient(cfg ServiceTokenClientConfig) (*ServiceTokenClient, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	name := strings.TrimSpace(cfg.ServiceName)
	secret := strings.TrimSpace(cfg.ServiceSecret)
	aud := strings.TrimSpace(cfg.Audience)
	if base == "" || name == "" || secret == "" || aud == "" {
		return nil, fmt.Errorf("service token client: base_url, service_name, service_secret and audience are required")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		hc = &http.Client{Timeout: timeout}
	}
	return &ServiceTokenClient{
		baseURL:    base,
		service:    name,
		secret:     secret,
		audience:   aud,
		httpClient: hc,
	}, nil
}

// Token implements TokenSource. When audience is empty, the configured default is used.
func (c *ServiceTokenClient) Token(ctx context.Context, audience string) (string, error) {
	aud := strings.TrimSpace(audience)
	if aud == "" {
		aud = c.audience
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.tokenAudience == aud && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		obsmetrics.RecordTokenCacheHit(c.service, aud)
		return c.token, nil
	}
	obsmetrics.RecordTokenRequest(c.service, aud)
	tok, exp, err := c.fetch(ctx, aud)
	if err != nil {
		obsmetrics.RecordTokenRequestFailure(c.service, aud, ReasonTokenFetchFailed)
		return "", err
	}
	c.token = tok
	c.tokenAudience = aud
	c.expiresAt = exp
	return tok, nil
}

type serviceTokenRequest struct {
	Audience string `json:"audience"`
}

type serviceTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (c *ServiceTokenClient) fetch(ctx context.Context, audience string) (string, time.Time, error) {
	body, err := json.Marshal(serviceTokenRequest{Audience: audience})
	if err != nil {
		return "", time.Time{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/service-tokens", bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ServiceNameHeader, c.service)
	req.Header.Set(ServiceSecretHeader, c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("service token: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("service token: auth-service returned %d: %s", resp.StatusCode, string(raw))
	}
	var out serviceTokenResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", time.Time{}, fmt.Errorf("service token: decode: %w", err)
	}
	if out.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("service token: empty access_token")
	}
	exp := time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	if out.ExpiresIn <= 0 {
		exp = time.Now().Add(4 * time.Minute)
	}
	return out.AccessToken, exp, nil
}

// SetBearer sets Authorization: Bearer <service-token> on an HTTP request.
func SetBearer(req *http.Request, token string) {
	if req == nil || token == "" {
		return
	}
	req.Header.Set("Authorization", AuthorizationHeader(token))
}
