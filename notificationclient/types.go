// Package notificationclient is a thin HTTP client for the internal
// notification-service API. It lets other services enqueue notifications
// (Send) or send a one-off message straight to a channel/recipient
// (SendDirect) without depending on notification-service's internal types.
package notificationclient

import (
	"net/http"
	"time"
)

// Command asks notification-service to render a template and deliver it to
// a known user across one or more channels. It maps to
// POST {BaseURL}/internal/v1/notifications.
type Command struct {
	// IdempotencyKey deduplicates retried/duplicate calls server-side.
	// Callers MUST set a stable, unique key per logical notification.
	IdempotencyKey string `json:"idempotency_key"`
	// UserID is the recipient's internal user id.
	UserID string `json:"user_id"`
	// TemplateCode identifies the message template to render (e.g.
	// "withdrawal_approved").
	TemplateCode string `json:"template_code"`
	// Locale selects the template translation (e.g. "en", "fa").
	Locale string `json:"locale,omitempty"`
	// Channels restricts delivery to the given channels (e.g. "sms",
	// "email", "push"). Empty means "use the template/user defaults".
	Channels []string `json:"channels,omitempty"`
	// Priority is a free-form hint to notification-service ("low",
	// "normal", "high", "critical").
	Priority string `json:"priority,omitempty"`
	// Variables are the template placeholders (e.g. amount, currency).
	// Do not put secrets in here; they may be persisted/logged by
	// notification-service.
	Variables map[string]any `json:"variables,omitempty"`
	// ActionURL is an optional deep link/CTA URL included in the message.
	ActionURL string `json:"action_url,omitempty"`
	// ScheduledAt defers delivery until the given time. Nil means "now".
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
}

// DirectCommand asks notification-service to render a template and deliver
// it straight to an explicit recipient on a single channel, bypassing user
// lookup/preferences. It maps to
// POST {BaseURL}/internal/v1/direct-notifications.
type DirectCommand struct {
	// IdempotencyKey deduplicates retried/duplicate calls server-side.
	IdempotencyKey string `json:"idempotency_key"`
	// TemplateCode identifies the message template to render.
	TemplateCode string `json:"template_code"`
	// Locale selects the template translation.
	Locale string `json:"locale,omitempty"`
	// Channel is the single delivery channel (e.g. "sms", "email").
	Channel string `json:"channel"`
	// Recipient is the raw address for Channel (phone number, email, etc).
	Recipient string `json:"recipient"`
	// Variables are the template placeholders.
	Variables map[string]any `json:"variables,omitempty"`
	// Priority is a free-form hint to notification-service.
	Priority string `json:"priority,omitempty"`
}

// AcceptedResponse is returned by notification-service once a command has
// been accepted for asynchronous processing. It does not indicate the
// notification has been delivered, only that it was queued.
type AcceptedResponse struct {
	ID      string `json:"id,omitempty"`
	BatchID string `json:"batch_id,omitempty"`
	Status  string `json:"status,omitempty"`
}

// Config configures an HTTP-backed Client.
type Config struct {
	// BaseURL is the notification-service base URL, e.g.
	// "http://notification-service:8080" (no trailing slash required).
	BaseURL string
	// APIKey is sent as the X-Internal-Api-Key header on every request.
	// Never logged by this package.
	APIKey string
	// Timeout bounds each individual HTTP request. Defaults to 10s when
	// zero or negative.
	Timeout time.Duration
	// HTTPClient optionally overrides the underlying *http.Client (e.g. to
	// customize transport, tracing, or connection pooling). When nil, a
	// client is constructed using Timeout.
	HTTPClient *http.Client
}
