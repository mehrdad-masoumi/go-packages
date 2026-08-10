package tracing

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// GRPCServerHandler returns a stats.Handler for gRPC servers (W3C propagation + SERVER spans).
func GRPCServerHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

// GRPCClientHandler returns a stats.Handler for gRPC clients (W3C propagation + CLIENT spans).
func GRPCClientHandler() stats.Handler {
	return otelgrpc.NewClientHandler()
}

// GRPCServerOption returns grpc.ServerOption for OpenTelemetry instrumentation.
func GRPCServerOption() grpc.ServerOption {
	return grpc.StatsHandler(GRPCServerHandler())
}

// GRPCClientDialOption returns a dial option for OpenTelemetry instrumentation.
func GRPCClientDialOption() grpc.DialOption {
	return grpc.WithStatsHandler(GRPCClientHandler())
}
