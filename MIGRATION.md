# Migration from the old single module

Old imports map to the new modules as follows:

| Old | New |
|---|---|
| `go-packages/apperr` | `go-packages/errors` (package name `apperr`) |
| `go-packages/auth` verifier/claims | `go-packages/security/jwt` |
| `go-packages/auth` Echo middleware | `go-packages/security/echo` |
| `go-packages/auth` JWKS/key helpers | `go-packages/security/jwks` |
| ad-hoc S2S auth | `go-packages/security/s2s` + `security/echo` / `security/grpc` |
| `go-packages/httpserver` | `go-packages/http/server`, `health`, `metrics`, `middleware` |
| `go-packages/db` | `go-packages/postgres/connection`, `postgres/migration` |
| `go-packages/observability/*` | same package paths, now under the independent `observability` module |
| `go-packages/rabbitmq` | `go-packages/messaging/rabbitmq` |
| `go-packages/outbox` | `go-packages/messaging/outbox` |
| `go-packages/sharederrors` | removed; keep domain-specific sentinels in the owning service |

## Important API changes

### HTTP server
The old server read configuration from environment variables inside the library. The new API receives explicit `server.Config`.

### PostgreSQL
Use `connection.Open(ctx, connection.Config{...})`; connection establishment is context-aware and verifies the database with a bounded ping.

### RabbitMQ
Use `rabbitmq.New(ctx, rabbitmq.Config{URL: ...}, topology)`. The reconnect goroutine is cancellable and stops on `Close`.

### Outbox
`outbox.New` validates configuration and returns `(*Relay, error)`. `Relay.Run(ctx)` supports graceful cancellation.

### S2S
Do not copy `X-Internal-Secret` implementations. Verify short-lived EdDSA service tokens with destination audience checks and inject the authenticated service identity into request context.

## Removed
`sharederrors.ErrInsufficient` and similar domain sentinels were intentionally removed from shared infrastructure.
