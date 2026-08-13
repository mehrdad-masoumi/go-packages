# runtime

Small application lifecycle helper for microservices.

This is not a DI framework. Construct dependencies explicitly, then register:

- HTTP servers (`Start` + `Shutdown`)
- gRPC servers (`Serve` + `GracefulStop` with `Stop` fallback)
- background runners (`Run(ctx)`)
- closable resources (reverse registration order)

Default shutdown timeout is 25s, overridable with `Config.ShutdownTimeout` or `SHUTDOWN_TIMEOUT`.
