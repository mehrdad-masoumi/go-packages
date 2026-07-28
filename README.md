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
