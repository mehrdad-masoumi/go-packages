# go-packages

Shared Go libraries for broker microservices.

**Module:** `github.com/mehrdad-masoumi/go-packages`

## Packages

| Package | Description |
|---------|-------------|
| `apperr` | Application errors: rich errors, validation (422), Echo HTTP handler |
| `httpserver` | Shared Echo setup + `/health-check` `/ready` + Prometheus `/metrics` |
| `auth` | JWT middleware, claims helpers, internal API key |
| `db` | Postgres connect + sql-migrate helpers |
| `notificationclient` | HTTP client for the internal notification-service API (`Send`/`SendDirect`) |

Domain sentinel errors (e.g. `ErrNotFound`) live in each service, not here.

## Install

```bash
go get github.com/mehrdad-masoumi/go-packages@latest
```

```go
import "github.com/mehrdad-masoumi/go-packages/apperr"
```

## Local development

Until the module is published, services can use a `replace` directive:

```
replace github.com/mehrdad-masoumi/go-packages => ../go-packages
```

## notificationclient

`notificationclient` is a thin HTTP client for the internal
notification-service API. It never talks to a queue or database itself —
it just makes a single HTTP POST per call and maps the response to a typed
result.

```go
import "github.com/mehrdad-masoumi/go-packages/notificationclient"

type Client interface {
	Send(ctx context.Context, command Command) (AcceptedResponse, error)
	SendDirect(ctx context.Context, command DirectCommand) (AcceptedResponse, error)
}
```

- `Send` → `POST {BaseURL}/internal/v1/notifications` (known user, rendered from a template).
- `SendDirect` → `POST {BaseURL}/internal/v1/direct-notifications` (explicit recipient, bypasses user lookup).
- Every request carries `X-Internal-Api-Key: {Config.APIKey}`.
- Request timeout comes from `Config.Timeout` (default 10s); the caller's `context.Context` is always respected (cancellation/deadline aborts the in-flight request).
- **This client performs NO automatic retry of the whole request.** A single HTTP call is made per `Send`/`SendDirect` invocation. Callers that need retries must implement their own backoff using a stable `IdempotencyKey`.
- The client never logs request/response bodies, template variables, or the API key. Errors only ever surface the server's `message` field, never the raw body.

### Errors

Non-2xx responses and transport failures are mapped to typed sentinel
errors usable with `errors.Is`, and to `*notificationclient.StatusError`
(carrying the raw HTTP status code and message) usable with `errors.As`:

| Condition | Sentinel |
|---|---|
| HTTP 409 | `ErrConflict` (likely an idempotency key collision — the original call may already have been accepted) |
| HTTP 422 | `ErrValidation` (bad payload — retrying won't help) |
| HTTP 429 | `ErrRateLimited` (back off before retrying) |
| HTTP 5xx, network error, timeout, context cancellation | `ErrUnavailable` (outcome unknown) |
| HTTP 401 / 403 | `ErrUnauthorized` (bad/missing internal API key) |

```go
resp, err := client.Send(ctx, command)
switch {
case errors.Is(err, notificationclient.ErrValidation):
	// caller bug — fix the payload, don't retry
case errors.Is(err, notificationclient.ErrRateLimited), errors.Is(err, notificationclient.ErrUnavailable):
	// transient — safe to retry later with the same IdempotencyKey
case err != nil:
	// ErrConflict, ErrUnauthorized, or unexpected — handle/log
}
```

### ⚠️ Do not fire-and-forget

**Callers MUST NOT do `go client.Send(ctx, command)` and ignore the
result.** Errors — including validation errors that indicate a caller
bug — would be silently dropped, and an unawaited goroutine can outlive
(or race with) the caller's own request context. Always call
`Send`/`SendDirect` synchronously and handle the returned error, even if
that just means logging it and moving on.

### ⚠️ Financial / security-sensitive notifications

Callers triggering notifications for financial or security-sensitive
operations (e.g. a withdrawal approval, a password change, a 2FA reset)
**should not** call this client synchronously in the middle of that
operation's transaction. A notification-service outage or slow response
must never block, fail, or partially roll back the underlying operation.

Instead, use your own **transactional outbox**: write the intended
notification command to an outbox table in the same DB transaction as the
business change, commit, and have a separate worker read the outbox and
call `Send`/`SendDirect` (with retries/backoff on `ErrRateLimited` /
`ErrUnavailable`) after the transaction has durably committed.

### Sample usage

```go
package withdrawal

import (
	"context"
	"time"

	"github.com/mehrdad-masoumi/go-packages/notificationclient"
)

// Called by an outbox worker after a withdrawal-approved event has been
// durably persisted — not inline with the approval transaction itself.
func notifyWithdrawalApproved(ctx context.Context, client notificationclient.Client, userID, amount, currency string) error {
	_, err := client.Send(ctx, notificationclient.Command{
		IdempotencyKey: "withdrawal_approved:" + userID + ":" + amount, // stable per event
		UserID:         userID,
		TemplateCode:   "withdrawal_approved",
		Locale:         "en",
		Channels:       []string{"sms", "email"},
		Priority:       "high",
		Variables: map[string]any{
			"amount":   amount,
			"currency": currency,
		},
	})
	return err
}
```

```go
client := notificationclient.New(notificationclient.Config{
	BaseURL: "http://notification-service:8080",
	APIKey:  cfg.InternalAPIKey,
	Timeout: 5 * time.Second,
})
```

### Testing callers

Use `notificationclient.Fake` in caller unit tests instead of spinning up
an HTTP server:

```go
fake := notificationclient.NewFake()
fake.SendResponse = notificationclient.AcceptedResponse{ID: "n1", Status: "queued"}

var client notificationclient.Client = fake
// ... exercise code that calls client.Send / client.SendDirect ...

commands := fake.Commands() // assert on IdempotencyKey, TemplateCode, Variables, etc.
```
