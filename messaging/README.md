# messaging

Reusable messaging infrastructure.

- `rabbitmq`: context-aware reconnecting AMQP client, confirms, graceful shutdown and W3C trace propagation.
- `outbox`: generic relay lifecycle/retry/locking interfaces.

Service-specific topology, SQL schemas, event payloads and business consumers stay inside each service.
