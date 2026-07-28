package notificationclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mehrdad-masoumi/go-packages/notificationclient"
)

func TestSend_Success202(t *testing.T) {
	var gotPath, gotAPIKey, gotContentType string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("X-Internal-Api-Key")
		gotContentType = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = buf

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(notificationclient.AcceptedResponse{
			ID:     "notif-1",
			Status: "queued",
		})
	}))
	defer srv.Close()

	client := notificationclient.New(notificationclient.Config{
		BaseURL: srv.URL,
		APIKey:  "secret-key",
		Timeout: 5 * time.Second,
	})

	resp, err := client.Send(context.Background(), notificationclient.Command{
		IdempotencyKey: "idem-1",
		UserID:         "user-1",
		TemplateCode:   "withdrawal_approved",
		Locale:         "en",
		Channels:       []string{"sms", "email"},
		Priority:       "high",
		Variables:      map[string]any{"amount": "100"},
	})
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}
	if resp.ID != "notif-1" || resp.Status != "queued" {
		t.Fatalf("Send() unexpected response: %+v", resp)
	}

	if gotPath != "/internal/v1/notifications" {
		t.Errorf("path = %q, want /internal/v1/notifications", gotPath)
	}
	if gotAPIKey != "secret-key" {
		t.Errorf("X-Internal-Api-Key = %q, want secret-key", gotAPIKey)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	var sent notificationclient.Command
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("failed to decode sent body: %v", err)
	}
	if sent.IdempotencyKey != "idem-1" || sent.TemplateCode != "withdrawal_approved" {
		t.Errorf("sent body mismatch: %+v", sent)
	}
}

func TestSendDirect_Success200(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(notificationclient.AcceptedResponse{Status: "queued"})
	}))
	defer srv.Close()

	client := notificationclient.New(notificationclient.Config{BaseURL: srv.URL, APIKey: "k"})

	resp, err := client.SendDirect(context.Background(), notificationclient.DirectCommand{
		IdempotencyKey: "idem-2",
		TemplateCode:   "otp_code",
		Channel:        "sms",
		Recipient:      "+10000000000",
	})
	if err != nil {
		t.Fatalf("SendDirect() unexpected error: %v", err)
	}
	if resp.Status != "queued" {
		t.Fatalf("SendDirect() unexpected response: %+v", resp)
	}
	if gotPath != "/internal/v1/direct-notifications" {
		t.Errorf("path = %q, want /internal/v1/direct-notifications", gotPath)
	}
}

func TestSend_StatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       any
		wantErr    error
	}{
		{
			name:       "409 conflict",
			statusCode: http.StatusConflict,
			body:       map[string]string{"message": "duplicate idempotency key"},
			wantErr:    notificationclient.ErrConflict,
		},
		{
			name:       "422 validation",
			statusCode: http.StatusUnprocessableEntity,
			body:       map[string]string{"message": "template_code is required"},
			wantErr:    notificationclient.ErrValidation,
		},
		{
			name:       "429 rate limited",
			statusCode: http.StatusTooManyRequests,
			body:       map[string]string{"message": "too many requests"},
			wantErr:    notificationclient.ErrRateLimited,
		},
		{
			name:       "500 unavailable",
			statusCode: http.StatusInternalServerError,
			body:       map[string]string{"message": "boom"},
			wantErr:    notificationclient.ErrUnavailable,
		},
		{
			name:       "503 unavailable",
			statusCode: http.StatusServiceUnavailable,
			body:       nil,
			wantErr:    notificationclient.ErrUnavailable,
		},
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       map[string]string{"message": "invalid api key"},
			wantErr:    notificationclient.ErrUnauthorized,
		},
		{
			name:       "403 unauthorized",
			statusCode: http.StatusForbidden,
			body:       map[string]string{"message": "forbidden"},
			wantErr:    notificationclient.ErrUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				if tt.body != nil {
					_ = json.NewEncoder(w).Encode(tt.body)
				}
			}))
			defer srv.Close()

			client := notificationclient.New(notificationclient.Config{BaseURL: srv.URL, APIKey: "k"})

			_, err := client.Send(context.Background(), notificationclient.Command{
				IdempotencyKey: "idem",
				UserID:         "user-1",
				TemplateCode:   "withdrawal_approved",
			})
			if err == nil {
				t.Fatalf("Send() expected error, got nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Send() error = %v, want errors.Is match for %v", err, tt.wantErr)
			}

			var statusErr *notificationclient.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("Send() error = %v, want *StatusError via errors.As", err)
			}
			if statusErr.StatusCode != tt.statusCode {
				t.Errorf("StatusError.StatusCode = %d, want %d", statusErr.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestSend_NetworkErrorMapsToUnavailable(t *testing.T) {
	client := notificationclient.New(notificationclient.Config{
		BaseURL: "http://127.0.0.1:1", // nothing listening
		APIKey:  "k",
		Timeout: 500 * time.Millisecond,
	})

	_, err := client.Send(context.Background(), notificationclient.Command{
		IdempotencyKey: "idem",
		UserID:         "user-1",
		TemplateCode:   "withdrawal_approved",
	})
	if err == nil {
		t.Fatal("Send() expected error, got nil")
	}
	if !errors.Is(err, notificationclient.ErrUnavailable) {
		t.Fatalf("Send() error = %v, want errors.Is(ErrUnavailable)", err)
	}
}

func TestSend_RespectsContextCancellation(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer func() {
		close(block)
		srv.Close()
	}()

	client := notificationclient.New(notificationclient.Config{BaseURL: srv.URL, APIKey: "k"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Send(ctx, notificationclient.Command{
		IdempotencyKey: "idem",
		UserID:         "user-1",
		TemplateCode:   "withdrawal_approved",
	})
	if err == nil {
		t.Fatal("Send() expected error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Send() error = %v, want errors.Is(context.DeadlineExceeded)", err)
	}
	if !errors.Is(err, notificationclient.ErrUnavailable) {
		t.Fatalf("Send() error = %v, want errors.Is(ErrUnavailable)", err)
	}
}

func TestSend_UnexpectedStatusHasNoSentinelMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := notificationclient.New(notificationclient.Config{BaseURL: srv.URL, APIKey: "k"})

	_, err := client.Send(context.Background(), notificationclient.Command{
		IdempotencyKey: "idem",
		UserID:         "user-1",
		TemplateCode:   "withdrawal_approved",
	})
	if err == nil {
		t.Fatal("Send() expected error, got nil")
	}
	for _, sentinel := range []error{
		notificationclient.ErrConflict,
		notificationclient.ErrValidation,
		notificationclient.ErrRateLimited,
		notificationclient.ErrUnavailable,
		notificationclient.ErrUnauthorized,
	} {
		if errors.Is(err, sentinel) {
			t.Errorf("Send() error unexpectedly matches sentinel %v", sentinel)
		}
	}

	var statusErr *notificationclient.StatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("Send() error = %v, want *StatusError via errors.As", err)
	}
	if statusErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusError.StatusCode = %d, want 404", statusErr.StatusCode)
	}
}

func TestFake_RecordsSentCommands(t *testing.T) {
	fake := notificationclient.NewFake()
	fake.SendResponse = notificationclient.AcceptedResponse{ID: "fake-1", Status: "queued"}

	var client notificationclient.Client = fake

	resp, err := client.Send(context.Background(), notificationclient.Command{
		IdempotencyKey: "idem-1",
		UserID:         "user-1",
		TemplateCode:   "withdrawal_approved",
	})
	if err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}
	if resp.ID != "fake-1" {
		t.Fatalf("Send() response = %+v, want ID fake-1", resp)
	}

	_, err = client.SendDirect(context.Background(), notificationclient.DirectCommand{
		IdempotencyKey: "idem-2",
		TemplateCode:   "otp_code",
		Channel:        "sms",
		Recipient:      "+10000000000",
	})
	if err != nil {
		t.Fatalf("SendDirect() unexpected error: %v", err)
	}

	if got := fake.Commands(); len(got) != 1 || got[0].IdempotencyKey != "idem-1" {
		t.Errorf("Commands() = %+v, want one command with idem-1", got)
	}
	if got := fake.DirectCommands(); len(got) != 1 || got[0].IdempotencyKey != "idem-2" {
		t.Errorf("DirectCommands() = %+v, want one command with idem-2", got)
	}
}

func TestFake_SendFuncOverridesStaticResponse(t *testing.T) {
	fake := notificationclient.NewFake()
	fake.SendErr = errors.New("should not be used")
	calls := 0
	fake.SendFunc = func(ctx context.Context, command notificationclient.Command) (notificationclient.AcceptedResponse, error) {
		calls++
		if calls == 1 {
			return notificationclient.AcceptedResponse{}, notificationclient.ErrRateLimited
		}
		return notificationclient.AcceptedResponse{ID: "ok"}, nil
	}

	_, err := fake.Send(context.Background(), notificationclient.Command{TemplateCode: "t"})
	if !errors.Is(err, notificationclient.ErrRateLimited) {
		t.Fatalf("first Send() error = %v, want ErrRateLimited", err)
	}

	resp, err := fake.Send(context.Background(), notificationclient.Command{TemplateCode: "t"})
	if err != nil {
		t.Fatalf("second Send() unexpected error: %v", err)
	}
	if resp.ID != "ok" {
		t.Fatalf("second Send() response = %+v, want ID ok", resp)
	}

	if len(fake.Commands()) != 2 {
		t.Fatalf("Commands() len = %d, want 2", len(fake.Commands()))
	}
}
