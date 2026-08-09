# go-packages

Shared Go libraries for broker microservices.

**Module:** `github.com/mehrdad-masoumi/go-packages`

## Packages

| Package | Description |
|----------------|-------------|
| `apperr` | Application errors: rich errors, validation (422), Echo HTTP handler |
| `httpserver` | Shared Echo setup + `/health-check` `/ready` + Prometheus `/metrics` |
| `auth` | JWT middleware, claims helpers |
| `db` | Postgres connect + sql-migrate helpers |
| `outbox` | Transactional outbox relay loop (`Store` + confirmed `Publisher`) |
| `rabbitmq` | Reconnecting AMQP client + confirmed publish (topology via callback) |
| `sharederrors` | Common sentinel errors (`ErrNotFound`, `ErrAlreadyExists`, …) |

## Install

```bash
go get github.com/mehrdad-masoumi/go-packages@latest
```

```go
import "github.com/mehrdad-masoumi/go-packages/apperr"
```

## Local development

In the micro-service monorepo:

```
replace github.com/mehrdad-masoumi/go-packages => ../utils/go-packages
```

## Outbox

Services keep domain claim/mark SQL in their repository. The shared relay only needs a thin adapter:

```go
svc := outbox.New(storeAdapter{repo}, mq, cfg,
  outbox.WithDefaultExchange("broker.events"),
  outbox.WithLockedBy("wallet-outbox"),
)
go svc.Run(ctx)
```

## RabbitMQ

```go
client, err := rabbitmq.New(uri, func(ch *amqp.Channel) error {
  // declare service-specific topology
  return nil
}, rabbitmq.WithContentType("application/json"))
```
