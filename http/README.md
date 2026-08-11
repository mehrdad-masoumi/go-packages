# http

Echo platform primitives:
- `server`: explicit server bootstrap.
- `middleware`: error mapping, request IDs, access logs, tracing.
- `health`: liveness/readiness registration.
- `metrics`: Prometheus endpoint registration.

The module maps transport errors but business/application errors remain transport-agnostic in `go-packages/errors`.
