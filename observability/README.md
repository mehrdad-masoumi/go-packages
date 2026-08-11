# observability

Shared structured logging and OpenTelemetry primitives.

Packages:
- `logger`: JSON `slog` logger enriched with request/trace/span IDs.
- `tracing`: OTLP initialization plus HTTP/gRPC/W3C propagation helpers.

Transport-specific Echo and RabbitMQ middleware live in the `http` and `messaging` modules respectively.
