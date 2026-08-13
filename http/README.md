# http

Echo platform primitives:
- `server`: explicit server bootstrap, including inbound RED middleware (`http_server_requests_total`).
- `middleware`: error mapping, request IDs, access logs, tracing.
- `health`: liveness/readiness registration.
- `metrics`: Prometheus endpoint registration and HTTP server RED metrics.

Probe paths (`/metrics`, `/health`, `/ready`, `/live`) are excluded from traffic RED.

The module maps transport errors but business/application errors remain transport-agnostic in `go-packages/errors`.
