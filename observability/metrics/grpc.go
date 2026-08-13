package metrics

import (
	"context"
	"time"

	"github.com/mehrdad-masoumi/go-packages/observability/tracing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryClientInterceptor records unified RED metrics for outbound gRPC.
// Place it outermost so S2S token-fetch failures are still counted.
func UnaryClientInterceptor(source, destination string) grpc.UnaryClientInterceptor {
	ensure()
	src := SanitizeService(source)
	dst := SanitizeService(destination)
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		op := GRPCOperation(method)
		ctx = WithOperation(ctx, op)
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		st, errType := classifyGRPC(err)
		clientRequests.WithLabelValues(src, dst, ProtocolGRPC, op, st, errType).Inc()
		clientDuration.WithLabelValues(src, dst, ProtocolGRPC, op).Observe(time.Since(start).Seconds())
		return err
	}
}

// GRPCClientDialOptions returns OTEL + metrics (+ optional S2S) dial options.
// Interceptor order: metrics (outer) -> s2s (inner) -> invoker.
func GRPCClientDialOptions(source, destination string, s2sInterceptor grpc.UnaryClientInterceptor) []grpc.DialOption {
	chain := []grpc.UnaryClientInterceptor{UnaryClientInterceptor(source, destination)}
	if s2sInterceptor != nil {
		chain = append(chain, s2sInterceptor)
	}
	return []grpc.DialOption{
		tracing.GRPCClientDialOption(),
		grpc.WithChainUnaryInterceptor(chain...),
	}
}

func classifyGRPC(err error) (statusLabel, errorType string) {
	if err == nil {
		return StatusSuccess, ErrorNone
	}
	st, ok := status.FromError(err)
	if !ok {
		return StatusError, errorTypeFromErr(err)
	}
	switch st.Code() {
	case codes.OK:
		return StatusSuccess, ErrorNone
	case codes.Unauthenticated:
		return StatusError, ErrorUnauthenticated
	case codes.PermissionDenied:
		return StatusError, ErrorForbidden
	case codes.NotFound:
		return StatusError, ErrorNotFound
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return StatusError, ErrorInvalid
	case codes.AlreadyExists, codes.Aborted:
		return StatusError, ErrorConflict
	case codes.ResourceExhausted:
		return StatusError, ErrorRateLimited
	case codes.DeadlineExceeded:
		return StatusError, ErrorTimeout
	case codes.Canceled:
		return StatusError, ErrorCanceled
	case codes.Unavailable:
		return StatusError, ErrorUnavailable
	default:
		return StatusError, ErrorServer
	}
}
