package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNoop_MissingWhenEnabled(t *testing.T) {
	v := NoopVerifier{Enabled: true}
	err := v.Verify(context.Background(), "", VerifyOptions{})
	if !IsCaptchaError(err) {
		t.Fatalf("expected captcha error, got %v", err)
	}
	code, ok := CodeOf(err)
	if !ok || code != CodeRequired {
		t.Fatalf("expected captcha_required, got %v ok=%v", code, ok)
	}
}

func TestNoop_DisabledAllowsEmpty(t *testing.T) {
	v := NoopVerifier{Enabled: false}
	if err := v.Verify(context.Background(), "", VerifyOptions{}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestEnsureProductionSafe_RejectsNoop(t *testing.T) {
	err := EnsureProductionSafe(true, true, NoopVerifier{Enabled: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if err := EnsureProductionSafe(false, true, NoopVerifier{Enabled: true}); err != nil {
		t.Fatalf("non-prod should allow noop: %v", err)
	}
	if err := EnsureProductionSafe(true, false, NoopVerifier{}); err != nil {
		t.Fatalf("disabled captcha should allow noop: %v", err)
	}
}

func TestRecaptchaV3_MissingToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call provider")
	}))
	t.Cleanup(srv.Close)

	v, err := NewRecaptchaV3(Config{
		Secret:        "test-secret",
		Enabled:       true,
		SiteVerifyURL: srv.URL,
		HTTPClient:    srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = v.Verify(context.Background(), "  ", VerifyOptions{})
	code, ok := CodeOf(err)
	if !ok || code != CodeRequired {
		t.Fatalf("expected required, got %v %v", code, err)
	}
}

func TestRecaptchaV3_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(siteVerifyResponse{
			Success:  true,
			Score:    0.9,
			Action:   "login",
			Hostname: "broker.localhost",
		})
	}))
	t.Cleanup(srv.Close)

	v, err := NewRecaptchaV3(Config{
		Secret:        "test-secret",
		Enabled:       true,
		MinScore:      0.5,
		ExpectedHost:  "broker.localhost",
		SiteVerifyURL: srv.URL,
		HTTPClient:    srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify(context.Background(), "tok", VerifyOptions{Action: "login", RemoteIP: "1.2.3.4"}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestRecaptchaV3_InvalidScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(siteVerifyResponse{Success: true, Score: 0.1, Action: "login"})
	}))
	t.Cleanup(srv.Close)

	v, err := NewRecaptchaV3(Config{
		Secret: "s", Enabled: true, MinScore: 0.5,
		SiteVerifyURL: srv.URL, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = v.Verify(context.Background(), "tok", VerifyOptions{Action: "login"})
	code, ok := CodeOf(err)
	if !ok || code != CodeInvalid {
		t.Fatalf("expected invalid, got %v %v", code, err)
	}
}

func TestRecaptchaV3_ProviderFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	v, err := NewRecaptchaV3(Config{
		Secret: "s", Enabled: true,
		SiteVerifyURL: srv.URL, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = v.Verify(context.Background(), "tok", VerifyOptions{})
	code, ok := CodeOf(err)
	if !ok || code != CodeVerificationFailed {
		t.Fatalf("expected verification_failed, got %v %v", code, err)
	}
}

func TestRecaptchaV3_ActionMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(siteVerifyResponse{Success: true, Score: 0.9, Action: "register"})
	}))
	t.Cleanup(srv.Close)

	v, err := NewRecaptchaV3(Config{
		Secret: "s", Enabled: true,
		SiteVerifyURL: srv.URL, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = v.Verify(context.Background(), "tok", VerifyOptions{Action: "login"})
	code, ok := CodeOf(err)
	if !ok || code != CodeInvalid {
		t.Fatalf("expected invalid, got %v %v", code, err)
	}
}

func TestNewFromConfig_Disabled(t *testing.T) {
	v, err := NewFromConfig(true, false, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify(context.Background(), "", VerifyOptions{}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestNewFromConfig_ProductionRequiresSecret(t *testing.T) {
	_, err := NewFromConfig(true, true, "", 0.5, "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewFromConfig_DevNoopWithoutSecret(t *testing.T) {
	v, err := NewFromConfig(false, true, "", 0.5, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify(context.Background(), "", VerifyOptions{}); err == nil {
		t.Fatal("enabled noop should require token")
	}
}

func TestNewRecaptchaV3_RequiresSecret(t *testing.T) {
	_, err := NewRecaptchaV3(Config{Enabled: true})
	if err == nil {
		t.Fatal("expected error")
	}
}
