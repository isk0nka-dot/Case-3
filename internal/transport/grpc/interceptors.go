package grpc

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/pkg/auth"
)

// ---------------------------------------------------------------------------
//  gRPC Interceptors — Cross-Cutting Concerns
//
//  These interceptors implement the standard grpc.UnaryServerInterceptor
//  and grpc.StreamServerInterceptor signatures. They are chained in the
//  gRPC server options via grpc.ChainUnaryInterceptor / grpc.ChainStreamInterceptor.
//
//  Chain order (outermost to innermost):
//    1. Recovery    — catches panics so one bad request never kills the server.
//    2. RateLimit   — sheds load before it reaches business logic.
//    3. Auth (JWT)  — verifies identity and injects claims into context.
//    4. Session     — validates session is active via Eduser cache.
//    5. Logging     — records method, duration, and status for observability.
//
//  All interceptors are stateless factories that return the actual interceptor
//  function, following the idiomatic Go pattern for parameterized middleware.
// ---------------------------------------------------------------------------

// ============================= LOGGING =====================================

// LoggingUnaryInterceptor returns a unary server interceptor that logs
// every RPC call with its method name, duration, and resulting gRPC status code.
//
// Log levels:
//   - INFO for successful calls (codes.OK).
//   - WARN for client errors (InvalidArgument, NotFound, etc.).
//   - ERROR for server errors (Internal, Unavailable, etc.).
func LoggingUnaryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	log := logger.Named("grpc_unary")

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Invoke the actual RPC handler.
		resp, err := handler(ctx, req)

		duration := time.Since(start)
		code := status.Code(err)

		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
			zap.String("code", code.String()),
		}

		// Add identity fields if available.
		if claims := auth.ClaimsFromContext(ctx); claims != nil {
			fields = append(fields,
				zap.String("session_id", claims.SessionID),
				zap.String("org_id", claims.OrgID),
			)
		}

		switch {
		case code == codes.OK:
			log.Info("unary call completed", fields...)
		case isClientError(code):
			log.Warn("unary call client error", append(fields, zap.Error(err))...)
		default:
			log.Error("unary call server error", append(fields, zap.Error(err))...)
		}

		return resp, err
	}
}

// LoggingStreamInterceptor returns a stream server interceptor that logs
// the lifecycle of streaming RPCs: open, close, duration, and status.
//
// For long-lived streams (StreamEvents), this provides visibility into
// connection lifetimes and abnormal terminations.
func LoggingStreamInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	log := logger.Named("grpc_stream")

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		log.Info("stream opened",
			zap.String("method", info.FullMethod),
			zap.Bool("client_stream", info.IsClientStream),
			zap.Bool("server_stream", info.IsServerStream),
		)

		// Invoke the stream handler (blocks until stream closes).
		err := handler(srv, ss)

		duration := time.Since(start)
		code := status.Code(err)

		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", duration),
			zap.String("code", code.String()),
		}

		switch {
		case code == codes.OK:
			log.Info("stream closed", fields...)
		case isClientError(code):
			log.Warn("stream closed with client error", append(fields, zap.Error(err))...)
		default:
			log.Error("stream closed with server error", append(fields, zap.Error(err))...)
		}

		return err
	}
}

// ============================= RECOVERY ====================================

// RecoveryUnaryInterceptor returns a unary server interceptor that recovers
// from panics in RPC handlers. Without this, a single panic would crash the
// entire gRPC server process.
//
// On panic:
//   - Logs the panic value and stack trace at ERROR level.
//   - Returns codes.Internal to the client.
//   - The server continues serving other requests normally.
func RecoveryUnaryInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	log := logger.Named("grpc_recovery")

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				// Capture the stack trace before it unwinds further.
				stack := debug.Stack()
				log.Error("panic recovered in unary handler",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
					zap.ByteString("stack", stack),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

// RecoveryStreamInterceptor returns a stream server interceptor that recovers
// from panics in streaming RPC handlers. This is critical for the StreamEvents
// RPC where a panic in one stream must not affect the 10,000+ other active streams.
func RecoveryStreamInterceptor(logger *zap.Logger) grpc.StreamServerInterceptor {
	log := logger.Named("grpc_recovery")

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error("panic recovered in stream handler",
					zap.String("method", info.FullMethod),
					zap.Any("panic", r),
					zap.ByteString("stack", stack),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()

		return handler(srv, ss)
	}
}

// ============================= RATE LIMITING ================================

// RateLimitUnaryInterceptor returns a unary server interceptor that enforces
// a global request rate limit using a token bucket algorithm.
//
// The limiter is shared across all unary RPCs. When the limit is exceeded,
// the interceptor returns codes.ResourceExhausted without invoking the handler,
// shedding load before it reaches the business logic layer.
//
// Typical configuration:
//   - rate.NewLimiter(rate.Limit(50000), 1000) — 50K events/sec sustained, burst of 1000.
func RateLimitUnaryInterceptor(limiter *rate.Limiter) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if !limiter.Allow() {
			return nil, status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded: %s (limit: %.0f/sec)",
				info.FullMethod,
				float64(limiter.Limit()),
			)
		}
		return handler(ctx, req)
	}
}

// RateLimitStreamInterceptor returns a stream server interceptor that enforces
// a global rate limit on new stream creation.
//
// This limits the number of new StreamEvents connections per second, not the
// per-message rate within an existing stream. Per-message rate limiting is
// handled by the WorkerPool back-pressure mechanism.
//
// Typical configuration:
//   - rate.NewLimiter(rate.Limit(10000), 500) — 10K new streams/sec, burst of 500.
func RateLimitStreamInterceptor(limiter *rate.Limiter) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if !limiter.Allow() {
			return status.Errorf(
				codes.ResourceExhausted,
				"rate limit exceeded: %s (limit: %.0f/sec)",
				info.FullMethod,
				float64(limiter.Limit()),
			)
		}
		return handler(srv, ss)
	}
}

// ============================= JWT AUTHENTICATION ============================

// skipAuthMethods contains gRPC methods that do not require authentication.
// Health checks and reflection must be accessible without a token.
var skipAuthMethods = map[string]bool{
	"/grpc.health.v1.Health/Check":          true,
	"/grpc.health.v1.Health/Watch":          true,
	"/grpc.reflection.v1.ServerReflection/": true,
	"/grpc.reflection.v1alpha.ServerReflection/": true,
}

// shouldSkipAuth returns true if the method is in the skip list.
func shouldSkipAuth(fullMethod string) bool {
	if skipAuthMethods[fullMethod] {
		return true
	}
	// Prefix match for reflection.
	for prefix := range skipAuthMethods {
		if strings.HasPrefix(fullMethod, prefix) {
			return true
		}
	}
	return false
}

// AuthUnaryInterceptor returns a unary server interceptor that validates
// JWT tokens from the Authorization metadata header.
//
// Flow:
//  1. Extract "authorization" from gRPC metadata.
//  2. Verify token signature, expiration, issuer, audience.
//  3. Validate required proctoring claims (session_id, student_id, org_id, exam_id).
//  4. Optionally validate session via SessionValidator (Eduser cache).
//  5. Inject verified claims into the context.
//
// If verification fails, returns codes.Unauthenticated before the handler runs.
func AuthUnaryInterceptor(
	verifier *auth.Verifier,
	sessionValidator port.SessionValidator,
	logger *zap.Logger,
) grpc.UnaryServerInterceptor {
	log := logger.Named("grpc_auth")

	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip auth for health checks and reflection.
		if shouldSkipAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract and verify token.
		claims, err := extractAndVerify(ctx, verifier)
		if err != nil {
			log.Debug("authentication failed",
				zap.String("method", info.FullMethod),
				zap.Error(err),
			)
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Validate session (cache-first, calls Eduser on miss).
		if sessionValidator != nil {
			if err := sessionValidator.ValidateSession(ctx, claims.SessionID, claims.SessionSecret); err != nil {
				log.Warn("session validation failed",
					zap.String("method", info.FullMethod),
					zap.String("session_id", claims.SessionID),
					zap.Error(err),
				)
				return nil, status.Errorf(codes.Unauthenticated, "session validation failed: %v", err)
			}
		}

		// Inject claims into context for downstream handlers.
		ctx = auth.ContextWithClaims(ctx, claims)

		return handler(ctx, req)
	}
}

// AuthStreamInterceptor returns a stream server interceptor that validates
// JWT tokens on stream establishment. The token is verified ONCE when the
// stream opens — individual messages within the stream are not re-verified
// (the session is trusted for the stream's lifetime).
//
// For long-lived streams (1-4 hour proctoring sessions), the Heartbeat RPC
// handles periodic session revalidation. If a session is terminated by an
// admin, the Heartbeat response includes a terminate directive, and the
// client closes the stream.
func AuthStreamInterceptor(
	verifier *auth.Verifier,
	sessionValidator port.SessionValidator,
	logger *zap.Logger,
) grpc.StreamServerInterceptor {
	log := logger.Named("grpc_auth")

	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Skip auth for health checks and reflection.
		if shouldSkipAuth(info.FullMethod) {
			return handler(srv, ss)
		}

		ctx := ss.Context()

		// Extract and verify token.
		claims, err := extractAndVerify(ctx, verifier)
		if err != nil {
			log.Debug("stream authentication failed",
				zap.String("method", info.FullMethod),
				zap.Error(err),
			)
			return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Validate session.
		if sessionValidator != nil {
			if err := sessionValidator.ValidateSession(ctx, claims.SessionID, claims.SessionSecret); err != nil {
				log.Warn("stream session validation failed",
					zap.String("method", info.FullMethod),
					zap.String("session_id", claims.SessionID),
					zap.Error(err),
				)
				return status.Errorf(codes.Unauthenticated, "session validation failed: %v", err)
			}
		}

		// Wrap the stream with claims in context.
		wrappedStream := &authenticatedServerStream{
			ServerStream: ss,
			ctx:          auth.ContextWithClaims(ctx, claims),
		}

		return handler(srv, wrappedStream)
	}
}

// authenticatedServerStream wraps a grpc.ServerStream to override the
// Context() method with a context containing verified claims.
type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context with verified claims.
func (s *authenticatedServerStream) Context() context.Context {
	return s.ctx
}

// extractAndVerify extracts the Authorization token from gRPC metadata and
// verifies it using the JWT verifier.
func extractAndVerify(ctx context.Context, verifier *auth.Verifier) (*auth.ProctoringClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, auth.ErrNoToken
	}

	// gRPC-Web and Connect-ES send the token in the "authorization" header.
	// Standard gRPC clients may use "authorization" or custom headers.
	tokenValues := md.Get("authorization")
	if len(tokenValues) == 0 {
		// Fallback: check for a custom "x-auth-token" header (used by some
		// browser-based gRPC-Web implementations).
		tokenValues = md.Get("x-auth-token")
	}
	if len(tokenValues) == 0 {
		return nil, auth.ErrNoToken
	}

	return verifier.VerifyToken(tokenValues[0])
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

// isClientError returns true if the gRPC status code represents a client-side
// error. These are logged at WARN level rather than ERROR because they
// indicate issues with the request, not with the server.
func isClientError(code codes.Code) bool {
	switch code {
	case codes.InvalidArgument,
		codes.NotFound,
		codes.AlreadyExists,
		codes.PermissionDenied,
		codes.Unauthenticated,
		codes.FailedPrecondition,
		codes.OutOfRange,
		codes.Canceled:
		return true
	default:
		return false
	}
}

// NewInterceptorChain is a convenience function that constructs the recommended
// interceptor chain for the Event Collector gRPC server.
//
// Chain order (applied outermost-first):
//  1. Recovery  — ensures panics never crash the server.
//  2. Rate Limit — sheds excess load before reaching business logic.
//  3. Auth (JWT) — verifies identity and injects claims into context.
//  4. Logging   — records every call for observability.
//
// Session validation is embedded within the Auth interceptor. The Auth
// interceptor calls sessionValidator.ValidateSession() after JWT verification.
//
// Usage:
//
//	unaryInterceptors, streamInterceptors := grpc.NewInterceptorChain(
//	    logger, unaryLimiter, streamLimiter, jwtVerifier, sessionValidator,
//	)
//	srv := grpc.NewServer(
//	    grpc.ChainUnaryInterceptor(unaryInterceptors...),
//	    grpc.ChainStreamInterceptor(streamInterceptors...),
//	)
func NewInterceptorChain(
	logger *zap.Logger,
	unaryLimiter *rate.Limiter,
	streamLimiter *rate.Limiter,
	jwtVerifier *auth.Verifier,
	sessionValidator port.SessionValidator,
) ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	unary := []grpc.UnaryServerInterceptor{
		RecoveryUnaryInterceptor(logger),
		RateLimitUnaryInterceptor(unaryLimiter),
		AuthUnaryInterceptor(jwtVerifier, sessionValidator, logger),
		LoggingUnaryInterceptor(logger),
	}

	stream := []grpc.StreamServerInterceptor{
		RecoveryStreamInterceptor(logger),
		RateLimitStreamInterceptor(streamLimiter),
		AuthStreamInterceptor(jwtVerifier, sessionValidator, logger),
		LoggingStreamInterceptor(logger),
	}

	return unary, stream
}

// FormatRateLimit returns a human-readable string for a rate limiter configuration.
// Useful for startup logging.
func FormatRateLimit(limiter *rate.Limiter) string {
	return fmt.Sprintf("%.0f/sec (burst: %d)", float64(limiter.Limit()), limiter.Burst())
}
