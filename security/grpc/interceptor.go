package grpcauth

import (
	"context"
	"strings"

	"github.com/mehrdad-masoumi/go-packages/observability/logger"
	obsmetrics "github.com/mehrdad-masoumi/go-packages/observability/metrics"
	"github.com/mehrdad-masoumi/go-packages/security/s2s"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Authorizer func(ctx context.Context, fullMethod string, identity s2s.Identity) error

func UnaryServerInterceptor(verifier s2s.TokenVerifier, authorize Authorizer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if isPublicGRPCMethod(info.FullMethod) {
			return handler(ctx, req)
		}
		identity, err := authenticate(ctx, verifier)
		if err != nil {
			return nil, err
		}
		ctx = s2s.ContextWithIdentity(ctx, identity)
		if authorize != nil {
			if err := authorize(ctx, info.FullMethod, identity); err != nil {
				obsmetrics.RecordS2SAuthFailure(identity.Subject, logger.Service(), s2s.ReasonMissingScope)
				return nil, status.Error(codes.PermissionDenied, "forbidden")
			}
		}
		return handler(ctx, req)
	}
}

func StreamServerInterceptor(verifier s2s.TokenVerifier, authorize Authorizer) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if isPublicGRPCMethod(info.FullMethod) {
			return handler(srv, stream)
		}
		identity, err := authenticate(stream.Context(), verifier)
		if err != nil {
			return err
		}
		ctx := s2s.ContextWithIdentity(stream.Context(), identity)
		if authorize != nil {
			if err := authorize(ctx, info.FullMethod, identity); err != nil {
				obsmetrics.RecordS2SAuthFailure(identity.Subject, logger.Service(), s2s.ReasonMissingScope)
				return status.Error(codes.PermissionDenied, "forbidden")
			}
		}
		return handler(srv, &wrappedServerStream{ServerStream: stream, ctx: ctx})
	}
}

func RequireAnyScope(methodScopes map[string][]string) Authorizer {
	return func(_ context.Context, fullMethod string, identity s2s.Identity) error {
		scopes, restricted := methodScopes[fullMethod]
		if !restricted {
			return nil
		}
		if len(scopes) == 0 || identity.HasAnyScope(scopes...) {
			return nil
		}
		return status.Error(codes.PermissionDenied, "forbidden")
	}
}

func UnaryClientInterceptor(source s2s.TokenSource, audience string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx, err := outgoingContext(ctx, source, audience)
		if err != nil {
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func StreamClientInterceptor(source s2s.TokenSource, audience string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		ctx, err := outgoingContext(ctx, source, audience)
		if err != nil {
			return nil, err
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}

func authenticate(ctx context.Context, verifier s2s.TokenVerifier) (s2s.Identity, error) {
	dest := logger.Service()
	if verifier == nil {
		obsmetrics.RecordS2SAuthFailure("unknown", dest, s2s.ReasonMissingToken)
		return s2s.Identity{}, status.Error(codes.Unauthenticated, "unauthenticated service")
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		obsmetrics.RecordS2SAuthFailure("unknown", dest, s2s.ReasonMissingToken)
		return s2s.Identity{}, status.Error(codes.Unauthenticated, "unauthenticated service")
	}
	raw := first(md.Get("authorization"))
	identity, err := verifier.Verify(ctx, raw)
	if err != nil {
		obsmetrics.RecordS2SAuthFailure("unknown", dest, s2s.ReasonOf(err))
		return s2s.Identity{}, status.Error(codes.Unauthenticated, "unauthenticated service")
	}
	obsmetrics.RecordS2SAuth(identity.Subject, dest)
	return *identity, nil
}

func outgoingContext(ctx context.Context, source s2s.TokenSource, audience string) (context.Context, error) {
	token, err := source.Token(ctx, audience)
	if err != nil {
		return ctx, err
	}
	return metadata.AppendToOutgoingContext(ctx, "authorization", s2s.AuthorizationHeader(token)), nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context { return w.ctx }

func isPublicGRPCMethod(fullMethod string) bool {
	switch {
	case strings.HasPrefix(fullMethod, "/grpc.health.v1.Health/"):
		return true
	case strings.HasPrefix(fullMethod, "/grpc.reflection."):
		return true
	default:
		return false
	}
}
