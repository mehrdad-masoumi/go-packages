module github.com/mehrdad-masoumi/go-packages/security

go 1.24.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/labstack/echo/v4 v4.13.4
	github.com/mehrdad-masoumi/go-packages/errors v0.1.0
	github.com/mehrdad-masoumi/go-packages/observability v0.2.0
	google.golang.org/grpc v1.75.0
)

replace github.com/mehrdad-masoumi/go-packages/observability => ../observability

require (
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250825161204-c5933d9347a5 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)
