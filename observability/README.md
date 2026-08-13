# observability

Shared structured logging, OpenTelemetry primitives, and the unified Prometheus metric contract.

Packages:
- `logger`: JSON `slog` logger enriched with request/trace/span IDs and optional `operation`.
- `tracing`: OTLP initialization plus HTTP/gRPC/W3C propagation helpers.
- `metrics`: shared RED metrics for HTTP/gRPC clients, S2S auth, RabbitMQ, outbox, and a small business-flow helper.

Transport-specific Echo and RabbitMQ middleware live in the `http` and `messaging` modules respectively.

Do not invent per-service names for service-to-service calls. Use `metrics.HTTPClient` and `metrics.GRPCClientDialOptions`.
