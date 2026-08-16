// Package captcha provides server-side captcha verification for abuse-sensitive endpoints.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultSiteVerifyURL = "https://www.google.com/recaptcha/api/siteverify"

// Verifier validates captcha tokens.
type Verifier interface {
	Verify(ctx context.Context, token string, opts VerifyOptions) error
}

// VerifyOptions carries request-scoped verification inputs.
type VerifyOptions struct {
	Action   string
	RemoteIP string
	Hostname string
}

// Code is a bounded machine-readable captcha failure.
type Code string

const (
	CodeRequired           Code = "captcha_required"
	CodeInvalid            Code = "captcha_invalid"
	CodeVerificationFailed Code = "captcha_verification_failed"
)

// Error is returned for captcha failures (never wrap as 500 for bad tokens).
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return "captcha error"
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.Cause }

func ErrRequired() *Error {
	return &Error{Code: CodeRequired, Message: "captcha required"}
}

func ErrInvalid(msg string) *Error {
	if msg == "" {
		msg = "captcha invalid"
	}
	return &Error{Code: CodeInvalid, Message: msg}
}

func ErrVerificationFailed(err error) *Error {
	return &Error{Code: CodeVerificationFailed, Message: "captcha verification failed", Cause: err}
}

// IsCaptchaError reports whether err is a captcha.Error.
func IsCaptchaError(err error) bool {
	var ce *Error
	return errors.As(err, &ce)
}

// CodeOf returns the captcha code if err is a captcha.Error.
func CodeOf(err error) (Code, bool) {
	var ce *Error
	if errors.As(err, &ce) {
		return ce.Code, true
	}
	return "", false
}

// Config configures RecaptchaV3.
type Config struct {
	Secret          string
	Enabled         bool
	MinScore        float64
	ExpectedHost    string // optional hostname allowlist (exact match)
	AllowedActions  []string
	SiteVerifyURL   string
	HTTPClient      *http.Client
	AllowInProdNoop bool // unused by RecaptchaV3; used by Noop construction helpers
}

// RecaptchaV3 verifies Google reCAPTCHA v3 tokens via siteverify.
type RecaptchaV3 struct {
	secret         string
	minScore       float64
	expectedHost   string
	allowedActions map[string]struct{}
	siteVerifyURL  string
	client         *http.Client
}

// NewRecaptchaV3 builds a verifier. Secret is required when Enabled is true.
func NewRecaptchaV3(cfg Config) (*RecaptchaV3, error) {
	if !cfg.Enabled {
		return nil, errors.New("captcha: RecaptchaV3 requires Enabled=true; use NoopVerifier when disabled")
	}
	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, errors.New("captcha: secret is required when enabled")
	}
	minScore := cfg.MinScore
	if minScore <= 0 {
		minScore = 0.5
	}
	urlStr := cfg.SiteVerifyURL
	if urlStr == "" {
		urlStr = defaultSiteVerifyURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	actions := make(map[string]struct{}, len(cfg.AllowedActions))
	for _, a := range cfg.AllowedActions {
		a = strings.TrimSpace(a)
		if a != "" {
			actions[a] = struct{}{}
		}
	}
	return &RecaptchaV3{
		secret:         cfg.Secret,
		minScore:       minScore,
		expectedHost:   strings.TrimSpace(cfg.ExpectedHost),
		allowedActions: actions,
		siteVerifyURL:  urlStr,
		client:         client,
	}, nil
}

type siteVerifyResponse struct {
	Success     bool     `json:"success"`
	Score       float64  `json:"score"`
	Action      string   `json:"action"`
	Hostname    string   `json:"hostname"`
	ChallengeTS string   `json:"challenge_ts"`
	ErrorCodes  []string `json:"error-codes"`
}

func (r *RecaptchaV3) Verify(ctx context.Context, token string, opts VerifyOptions) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrRequired()
	}

	form := url.Values{}
	form.Set("secret", r.secret)
	form.Set("response", token)
	if ip := strings.TrimSpace(opts.RemoteIP); ip != "" {
		form.Set("remoteip", ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.siteVerifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return ErrVerificationFailed(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.client.Do(req)
	if err != nil {
		return ErrVerificationFailed(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ErrVerificationFailed(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrVerificationFailed(fmt.Errorf("provider status %d", resp.StatusCode))
	}

	var parsed siteVerifyResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ErrVerificationFailed(err)
	}
	if !parsed.Success {
		return ErrInvalid(fmt.Sprintf("provider rejected token: %s", strings.Join(parsed.ErrorCodes, ",")))
	}
	if parsed.Score < r.minScore {
		return ErrInvalid("captcha score too low")
	}
	expectedAction := strings.TrimSpace(opts.Action)
	if expectedAction != "" && parsed.Action != "" && expectedAction != parsed.Action {
		return ErrInvalid("captcha action mismatch")
	}
	if len(r.allowedActions) > 0 {
		check := expectedAction
		if check == "" {
			check = parsed.Action
		}
		if _, ok := r.allowedActions[check]; !ok {
			return ErrInvalid("captcha action mismatch")
		}
	}
	if r.expectedHost != "" && parsed.Hostname != "" && !strings.EqualFold(parsed.Hostname, r.expectedHost) {
		return ErrInvalid("captcha hostname mismatch")
	}
	if host := strings.TrimSpace(opts.Hostname); host != "" && parsed.Hostname != "" && !strings.EqualFold(parsed.Hostname, host) {
		return ErrInvalid("captcha hostname mismatch")
	}
	return nil
}

// NoopVerifier is for local/test only. It never calls a real provider.
// Production must not start with NoopVerifier when captcha is required.
type NoopVerifier struct {
	Enabled bool
}

func (n NoopVerifier) Verify(_ context.Context, token string, _ VerifyOptions) error {
	if n.Enabled && strings.TrimSpace(token) == "" {
		return ErrRequired()
	}
	return nil
}

// ErrNoopCaptchaInProduction is returned at startup when production enables captcha with NoopVerifier.
var ErrNoopCaptchaInProduction = errors.New("captcha: NoopVerifier cannot be used in production when captcha is enabled; configure a real provider or disable captcha")

// EnsureProductionSafe refuses Noop when captcha is enabled in production.
func EnsureProductionSafe(isProduction, captchaEnabled bool, v Verifier) error {
	if !isProduction || !captchaEnabled {
		return nil
	}
	if _, ok := v.(NoopVerifier); ok {
		return ErrNoopCaptchaInProduction
	}
	if _, ok := v.(*NoopVerifier); ok {
		return ErrNoopCaptchaInProduction
	}
	return nil
}
