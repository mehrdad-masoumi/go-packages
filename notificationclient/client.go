package notificationclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	pathNotifications       = "/internal/v1/notifications"
	pathDirectNotifications = "/internal/v1/direct-notifications"

	headerInternalAPIKey = "X-Internal-Api-Key"

	defaultTimeout = 10 * time.Second

	// maxErrorBodyBytes bounds how much of an error response body we will
	// read, so a misbehaving upstream can't force us to buffer unbounded
	// data in memory.
	maxErrorBodyBytes = 64 * 1024
)

// Client sends notification commands to notification-service.
//
// IMPORTANT — this package performs NO automatic retry of the whole HTTP
// request. A single call is made per Send/SendDirect invocation and its
// outcome (success, typed error, or context error) is returned as-is.
// Callers that need retries must implement their own backoff/retry policy
// using a fresh IdempotencyKey-stable Command, and callers whose
// notification is part of a financial or security-sensitive operation
// (e.g. "withdrawal approved") should not call this client synchronously
// inline with that operation — use your own transactional outbox so the
// notification is only sent after the operation durably commits, and so a
// notification-service outage cannot roll back or block the operation.
//
// Callers MUST NOT fire-and-forget these calls (e.g. `go client.Send(...)`
// without awaiting the result): errors — including ErrValidation and
// ErrUnauthorized, which indicate a caller bug — would be silently
// dropped, and the passed-in ctx would race with the caller's own
// lifecycle. Always call Send/SendDirect synchronously (optionally from a
// supervised background worker that owns its own context and error
// handling, such as an outbox dispatcher) and handle the returned error.
type Client interface {
	// Send renders and delivers a template-based notification to a known
	// user. See Command for field semantics.
	Send(ctx context.Context, command Command) (AcceptedResponse, error)

	// SendDirect renders and delivers a template-based notification
	// straight to an explicit recipient on a single channel, bypassing
	// user lookup/preferences. See DirectCommand for field semantics.
	SendDirect(ctx context.Context, command DirectCommand) (AcceptedResponse, error)
}

// HTTPClient is the default HTTP-backed implementation of Client.
type HTTPClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

var _ Client = (*HTTPClient)(nil)

// New builds an HTTPClient from cfg. It does not perform any network I/O.
func New(cfg Config) *HTTPClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	return &HTTPClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		httpClient: httpClient,
	}
}

// Send implements Client.
func (c *HTTPClient) Send(ctx context.Context, command Command) (AcceptedResponse, error) {
	return c.do(ctx, pathNotifications, command)
}

// SendDirect implements Client.
func (c *HTTPClient) SendDirect(ctx context.Context, command DirectCommand) (AcceptedResponse, error) {
	return c.do(ctx, pathDirectNotifications, command)
}

func (c *HTTPClient) do(ctx context.Context, path string, payload any) (AcceptedResponse, error) {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return AcceptedResponse{}, fmt.Errorf("notificationclient: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return AcceptedResponse{}, fmt.Errorf("notificationclient: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(headerInternalAPIKey, c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network error, timeout, or context cancellation: the request may
		// or may not have reached notification-service. Treat it the same
		// as a server-side outage from the caller's perspective.
		return AcceptedResponse{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		return AcceptedResponse{}, fmt.Errorf("%w: read response: %w", ErrUnavailable, err)
	}

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusCreated {
		var out AcceptedResponse
		if len(respBody) > 0 {
			if err := json.Unmarshal(respBody, &out); err != nil {
				return AcceptedResponse{}, fmt.Errorf("notificationclient: decode response: %w", err)
			}
		}
		return out, nil
	}

	return AcceptedResponse{}, mapStatusError(resp.StatusCode, respBody)
}
