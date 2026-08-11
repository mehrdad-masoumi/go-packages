# Implementation report

## Final architecture

The previous single Go module was refactored into six independent modules in one Git repository:

- `errors`
- `security`
- `observability`
- `http`
- `postgres`
- `messaging`

A root `go.work` composes them for local development.

## Major changes

- Removed transport coupling from application errors.
- Removed `sharederrors`; domain-specific sentinels must remain in owning services.
- Split JWT/JWKS concerns from Echo middleware.
- Added production-oriented EdDSA S2S identities with issuer/audience/expiry validation.
- Added Echo and gRPC S2S server middleware/interceptors.
- Added outbound HTTP and gRPC service-token injection primitives.
- Kept private service-token signing isolated behind `s2s.Signer`; ordinary services only need verification keys.
- Moved Echo-specific logging/error/tracing concerns into the `http` module.
- Kept structured logging and OTEL core in `observability`.
- Moved RabbitMQ-specific trace carrier code into `messaging/rabbitmq`, removing the AMQP dependency from observability.
- Reworked RabbitMQ lifecycle with cancellation, bounded dial timeout, reconnect, publisher confirms and graceful shutdown.
- Reworked generic outbox relay configuration/error reporting and fixed zero-duration ticker hazards.
- Split PostgreSQL connection/pool/instrumentation from migrations.
- Added root CI and module check script.
- Added migration/versioning documentation.

## Validation performed

- `gofmt` completed across all Go files.
- All Go source files were parsed successfully using Go's parser.
- Internal `go-packages/...` imports were checked against the resulting filesystem and all resolve to real package paths.
- `errors` tests passed in an isolated Go 1.23 compile-check copy.
- `messaging/outbox` tests passed in an isolated Go 1.23 compile-check copy.

## Full build/test limitation

The final repository intentionally keeps `go 1.24.0`.

The execution environment has:

`go version go1.23.2 linux/amd64`

Running the full check attempted to download Go 1.24.0 but outbound DNS/network access is disabled, so full `go vet ./...`, `go test ./...` and `go build ./...` could not be executed for modules requiring external dependencies.

The project Go version was not downgraded to hide this environment limitation.

## Migration note

This package refactor intentionally changes import paths. Existing services should be migrated using `MIGRATION.md` after publishing/tagging the nested modules.
