package grpc

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/pkg/auth"
	"github.com/argus-ai/event-collector/pkg/middleware"
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

// ============================= IDEMPOTENCY / DEDUPLICATION =================

// idempotencyEntry holds the expiration time for a cached idempotency key.
type idempotencyEntry struct {
	expiresAt time.Time
}

// IdempotencyCache provides a TTL-based cache of idempotency keys using
// sync.Map for lock-free reads. A background goroutine evicts expired
// entries every 60 seconds. This prevents duplicate event ingestion when
// the frontend transport retries a failed request with the same
// X-Idempotency-Key header.
type IdempotencyCache struct {
	entries sync.Map
	ttl     time.Duration
	logger  *zap.Logger
	done    chan struct{}
}

// NewIdempotencyCache creates an idempotency cache with the given TTL.
// The cache starts a background eviction goroutine that runs every 60 seconds.
// Call Close() to stop the eviction goroutine.
func NewIdempotencyCache(ttl time.Duration, logger *zap.Logger) *IdempotencyCache {
	c := &IdempotencyCache{
		ttl:    ttl,
		logger: logger.Named("idemp_cache"),
		done:   make(chan struct{}),
	}
	go c.evictionLoop()
	return c
}

// Check returns true if the key has already been seen (duplicate).
// If the key is new, it is stored with the configured TTL and false is returned.
func (c *IdempotencyCache) Check(key string) bool {
	entry := idempotencyEntry{expiresAt: time.Now().Add(c.ttl)}
	_, loaded := c.entries.LoadOrStore(key, entry)
	return loaded
}

// evictionLoop periodically removes expired entries from the cache.
func (c *IdempotencyCache) evictionLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			evicted := 0
			c.entries.Range(func(key, value any) bool {
				entry := value.(idempotencyEntry)
				if now.After(entry.expiresAt) {
					c.entries.Delete(key)
					evicted++
				}
				return true
			})
			if evicted > 0 {
				c.logger.Debug("evicted expired idempotency keys",
					zap.Int("count", evicted),
				)
			}
		case <-c.done:
			return
		}
	}
}

// Close stops the background eviction goroutine.
func (c *IdempotencyCache) Close() {
	close(c.done)
}

// DeduplicationUnaryInterceptor returns a unary server interceptor that
// rejects duplicate requests based on the X-Idempotency-Key gRPC metadata
// header. When a duplicate is detected, the interceptor returns an empty
// successful response (codes.OK) without invoking the handler.
//
// This prevents duplicate event ingestion during frontend transport retries
// (exponential backoff). The frontend generates a unique key per logical
// request and preserves it across retries.
//
// If cache is nil or the request has no idempotency key, the interceptor
// passes through to the handler.
func DeduplicationUnaryInterceptor(cache *IdempotencyCache) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Skip if cache is nil (dedup disabled).
		if cache == nil {
			return handler(ctx, req)
		}

		// Skip for health checks and reflection.
		if shouldSkipAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract idempotency key from gRPC metadata.
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return handler(ctx, req)
		}

		keys := md.Get("x-idempotency-key")
		if len(keys) == 0 || keys[0] == "" {
			return handler(ctx, req)
		}

		idempKey := keys[0]

		// Check if this key has been seen before.
		if cache.Check(idempKey) {
			// Duplicate request — return success without processing.
			// The client already has the data; it just needs confirmation.
			cache.logger.Debug("duplicate request deduplicated",
				zap.String("key", idempKey),
				zap.String("method", info.FullMethod),
			)
			// Return nil response with OK status. gRPC serializes this as
			// an empty protobuf message, which is a valid "success" response.
			return nil, nil
		}

		// First time seeing this key — proceed with handler.
		return handler(ctx, req)
	}
}

// NewInterceptorChain is a convenience function that constructs the recommended
// interceptor chain for the Event Collector gRPC server.
//
// Chain order (applied outermost-first):
//  1. Recovery     — ensures panics never crash the server.
//  2. RequestID    — assigns a unique ID to every request for tracing.
//  3. Metrics      — records request duration and count.
//  4. Dedup        — rejects duplicate requests via X-Idempotency-Key.
//  5. Rate Limit   — sheds excess load before reaching business logic.
//  6. Auth (JWT)   — verifies identity and injects claims into context.
//  7. Logging      — records every call for observability.
//
// Session validation is embedded within the Auth interceptor. The Auth
// interceptor calls sessionValidator.ValidateSession() after JWT verification.
//
// The idempCache parameter may be nil to disable deduplication.
//
// Usage:
//
//	unaryInterceptors, streamInterceptors := grpc.NewInterceptorChain(
//	    logger, unaryLimiter, streamLimiter, jwtVerifier, sessionValidator, idempCache,
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
	idempCache *IdempotencyCache,
) ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	unary := []grpc.UnaryServerInterceptor{
		RecoveryUnaryInterceptor(logger),
		middleware.RequestIDUnaryInterceptor(),
		MetricsUnaryInterceptor(),
		DeduplicationUnaryInterceptor(idempCache),
		RateLimitUnaryInterceptor(unaryLimiter),
		AuthUnaryInterceptor(jwtVerifier, sessionValidator, logger),
		LoggingUnaryInterceptor(logger),
	}

	stream := []grpc.StreamServerInterceptor{
		RecoveryStreamInterceptor(logger),
		middleware.RequestIDStreamInterceptor(),
		MetricsStreamInterceptor(),
		RateLimitStreamInterceptor(streamLimiter),
		AuthStreamInterceptor(jwtVerifier, sessionValidator, logger),
		LoggingStreamInterceptor(logger),
	}

	return unary, stream
}

// ============================= METRICS =====================================

// Prometheus metrics for gRPC request observability.
var (
	grpcRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "argus_grpc_request_duration_seconds",
			Help:    "Duration of gRPC requests in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "code"},
	)

	grpcRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "argus_grpc_requests_total",
			Help: "Total number of gRPC requests.",
		},
		[]string{"method", "code"},
	)
)

// MetricsUnaryInterceptor records request duration and count for unary RPCs.
func MetricsUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		duration := time.Since(start).Seconds()

		code := status.Code(err).String()
		grpcRequestDuration.WithLabelValues(info.FullMethod, code).Observe(duration)
		grpcRequestsTotal.WithLabelValues(info.FullMethod, code).Inc()

		return resp, err
	}
}

// MetricsStreamInterceptor records request duration and count for streaming RPCs.
func MetricsStreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		duration := time.Since(start).Seconds()

		code := status.Code(err).String()
		grpcRequestDuration.WithLabelValues(info.FullMethod, code).Observe(duration)
		grpcRequestsTotal.WithLabelValues(info.FullMethod, code).Inc()

		return err
	}
}

// FormatRateLimit returns a human-readable string for a rate limiter configuration.
// Useful for startup logging.
func FormatRateLimit(limiter *rate.Limiter) string {
	return fmt.Sprintf("%.0f/sec (burst: %d)", float64(limiter.Limit()), limiter.Burst())
}
