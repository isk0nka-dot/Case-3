// Package middleware provides cross-cutting HTTP and gRPC middleware for
// the Argus AI event collector.
//
// RequestID middleware generates a UUID v4 request identifier for every
// inbound request. This ID is:
//   - Set as the X-Request-Id response header (HTTP)
//   - Injected into the context for downstream structured logging
//   - Propagated through gRPC metadata for distributed tracing
//
// If the client already provides X-Request-Id, it is reused (trusted proxy chain).
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ---------------------------------------------------------------------------
// Context key for request ID propagation
// ---------------------------------------------------------------------------

type requestIDKey struct{}

const (
	// RequestIDHeader is the HTTP header name for the request identifier.
	RequestIDHeader = "X-Request-Id"

	// requestIDMetadataKey is the gRPC metadata key for the request identifier.
	requestIDMetadataKey = "x-request-id"
)

// RequestIDFromContext extracts the request ID from the context.
// Returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// contextWithRequestID returns a new context with the request ID attached.
func contextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// ---------------------------------------------------------------------------
// HTTP Middleware
// ---------------------------------------------------------------------------

// RequestIDHTTPMiddleware injects a unique request ID into every HTTP request.
// If the incoming request already has an X-Request-Id header, it is reused.
// The request ID is set on the response header and injected into the context.
func RequestIDHTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Set response header for client correlation.
		w.Header().Set(RequestIDHeader, requestID)

		// Inject into context for downstream logging.
		ctx := contextWithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------------------------------------------------------------------------
// gRPC Interceptors
// ---------------------------------------------------------------------------

// RequestIDUnaryInterceptor extracts or generates a request ID for unary RPCs.
// The ID is propagated via gRPC metadata and injected into the context.
func RequestIDUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		ctx = ensureRequestIDInContext(ctx)
		return handler(ctx, req)
	}
}

// RequestIDStreamInterceptor extracts or generates a request ID for streaming RPCs.
func RequestIDStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ensureRequestIDInContext(ss.Context())
		wrapped := &requestIDServerStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// ensureRequestIDInContext extracts request ID from gRPC metadata or generates one.
func ensureRequestIDInContext(ctx context.Context) context.Context {
	requestID := ""

	// Try to extract from incoming metadata.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(requestIDMetadataKey); len(vals) > 0 {
			requestID = vals[0]
		}
	}

	// Generate if not present.
	if requestID == "" {
		requestID = uuid.New().String()
	}

	// Set outgoing metadata for downstream services.
	ctx = metadata.AppendToOutgoingContext(ctx, requestIDMetadataKey, requestID)

	return contextWithRequestID(ctx, requestID)
}

// requestIDServerStream wraps grpc.ServerStream to override Context().
type requestIDServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *requestIDServerStream) Context() context.Context {
	return s.ctx
}
