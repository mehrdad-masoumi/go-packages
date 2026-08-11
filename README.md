# go-packages

Shared **technical/platform** libraries for broker microservices.

This repository is intentionally a **single Git repository with multiple Go modules**. Services depend only on the modules they actually use.

## Modules

| Module | Responsibility |
|---|---|
| `errors` | Transport-agnostic application error primitives |
| `security` | User JWT/JWKS + service-to-service identity for Echo/gRPC |
| `observability` | Structured logs and OpenTelemetry |
| `http` | Echo server/middleware/health/metrics |
| `postgres` | PostgreSQL connection/pool/instrumentation/migrations |
| `messaging` | RabbitMQ lifecycle/tracing + generic outbox relay |

`broker-contract` remains a separate repository/module: it contains **business integration contracts**, while this repository contains reusable technical infrastructure.

## Dependency direction

```text
errors

observability

security  -> errors
http      -> errors + observability
postgres  -> OTEL primitives only
messaging -> observability
```

No module may import broker service `internal` packages or broker domain models.

## Local development

The root `go.work` wires all modules together:

```bash
go work sync
./scripts/check.sh
```

Do not downgrade the `go` directive if a workstation is older; install/use the required Go toolchain instead.

## Versioning

Nested modules are independently versioned from the same Git repository. Example tags:

```text
errors/v0.1.0
security/v0.1.0
observability/v0.1.0
http/v0.1.0
postgres/v0.1.0
messaging/v0.1.0
```

A service that only upgrades `security` does not need to upgrade `postgres` or `messaging`.

## Placement rule

Before adding code here, ask:

> Is this reusable technical infrastructure across multiple services?

If no, it belongs in the owning service or in `broker-contract` when it is a cross-service business contract.
