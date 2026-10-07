// Package ratelimiter provides token-bucket rate limiting for protecting the
// event-collector ingestion endpoint from overload and abuse.
//
// Two levels of rate limiting are provided:
//
//   - Global rate limiter (Limiter): Enforces a service-wide request rate ceiling.
//     This protects the overall system capacity regardless of the source. Used at
//     the gRPC interceptor or HTTP middleware level.
//
//   - Per-session rate limiter (PerSessionLimiter): Enforces per-session request
//     rate limits. This prevents any single proctoring session from monopolizing
//     system resources. Each unique session_id gets its own independent token bucket.
//
// Both use golang.org/x/time/rate which implements a token bucket algorithm:
//   - Tokens are added at a steady rate (RequestsPerSecond).
//   - The bucket can hold at most BurstSize tokens.
//   - Each request consumes one token.
//   - If no tokens are available, the request is either rejected (Allow) or
//     blocks until a token becomes available (Wait).
//
// Design decisions:
//   - Token bucket is preferred over sliding window for its natural burst handling.
//   - Per-session limiters are stored in sync.Map for lock-free concurrent reads
//     (the common path), with atomic creation via LoadOrStore.
//   - Stale session limiters are cleaned up periodically to prevent memory leaks
//     during long-running exam sessions.
//   - Rate and burst can be adjusted dynamically at runtime for operational flexibility.
package ratelimiter

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// Config holds the configuration for a rate limiter.
type Config struct {
	// RequestsPerSecond is the steady-state rate at which tokens are added to the
	// bucket. For example, 1000.0 means the limiter allows 1000 requests per second
	// on average over time. This is the long-term sustainable throughput.
	RequestsPerSecond float64 `yaml:"requests_per_second"`

	// BurstSize is the maximum number of tokens the bucket can hold. This controls
	// the peak instantaneous throughput. For example, if BurstSize is 2000 and the
	// bucket is full, 2000 requests can be served immediately before the limiter
	// begins throttling. After the burst, requests are limited to RequestsPerSecond.
	BurstSize int `yaml:"burst_size"`
}

// ---------------------------------------------------------------------------
// Global Rate Limiter
// ---------------------------------------------------------------------------

// Limiter provides a global token-bucket rate limiter for the ingestion endpoint.
// All requests share a single bucket, making this the first line of defense against
// traffic spikes. It wraps golang.org/x/time/rate.Limiter with structured logging
// and a simplified API.
type Limiter struct {
	limiter *rate.Limiter
	logger  *zap.Logger
}

// New creates a new global rate limiter with the given configuration.
// The limiter starts with a full token bucket (BurstSize tokens available).
func New(cfg Config, logger *zap.Logger) *Limiter {
	namedLogger := logger.Named("rate_limiter")

	l := &Limiter{
		limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.BurstSize),
		logger:  namedLogger,
	}

	namedLogger.Info("global rate limiter initialized",
		zap.Float64("requests_per_second", cfg.RequestsPerSecond),
		zap.Int("burst_size", cfg.BurstSize),
	)

	return l
}

// Allow reports whether a single request can proceed immediately without waiting.
// Returns true if a token is available (and consumes it), false otherwise.
//
// This is the non-blocking check suitable for middleware that should reject
// excess traffic with an immediate "rate limit exceeded" response.
func (l *Limiter) Allow() bool {
	return l.limiter.Allow()
}

// Wait blocks until a token is available or the context is cancelled.
// Returns nil if a token was acquired, or the context error if cancelled.
//
// This is the blocking variant suitable for internal service-to-service calls
// where it is acceptable to queue and wait rather than reject.
//
// The context deadline is respected — if the caller's deadline expires before
// a token becomes available, ctx.Err() is returned.
func (l *Limiter) Wait(ctx context.Context) error {
	return l.limiter.Wait(ctx)
}

// Limit returns the current steady-state rate limit in requests per second.
func (l *Limiter) Limit() float64 {
	return float64(l.limiter.Limit())
}

// SetLimit dynamically adjusts the steady-state rate. This takes effect
// immediately and does not discard any existing tokens in the bucket.
//
// This is useful for runtime tuning via admin endpoints or configuration
// hot-reloading without service restarts.
func (l *Limiter) SetLimit(newRate float64) {
	l.limiter.SetLimit(rate.Limit(newRate))
	l.logger.Info("rate limit updated",
		zap.Float64("new_requests_per_second", newRate),
	)
}

// SetBurst dynamically adjusts the maximum burst size. This takes effect
// immediately but does not add or remove tokens from the current bucket.
//
// Increasing the burst allows larger traffic spikes to be absorbed.
// Decreasing it provides tighter throttling at the cost of rejecting
// legitimate bursts.
func (l *Limiter) SetBurst(newBurst int) {
	l.limiter.SetBurst(newBurst)
	l.logger.Info("burst size updated",
		zap.Int("new_burst_size", newBurst),
	)
}

// ---------------------------------------------------------------------------
// Per-Session Rate Limiter
// ---------------------------------------------------------------------------

// sessionEntry holds a per-session rate limiter along with metadata for
// staleness tracking. The lastSeen timestamp is updated on every access
// and used by Cleanup to evict idle sessions.
type sessionEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// PerSessionLimiter maintains an independent rate limiter for each proctoring
// session. This prevents a single misbehaving or compromised session from
// exhausting the global rate limit and starving other sessions.
//
// Internally, limiters are stored in a sync.Map keyed by session ID. This
// provides O(1) lock-free lookups for the hot path (existing sessions) and
// uses LoadOrStore for atomic creation of new session limiters.
type PerSessionLimiter struct {
	limiters sync.Map // map[string]*sessionEntry
	cfg      Config
	logger   *zap.Logger
}

// NewPerSession creates a new per-session rate limiter factory.
// Each session will be allocated a limiter with the given Config when first seen.
//
// The caller is responsible for calling Cleanup periodically (e.g., every 5 minutes)
// to remove limiters for sessions that have been idle longer than the specified maxAge.
func NewPerSession(cfg Config, logger *zap.Logger) *PerSessionLimiter {
	namedLogger := logger.Named("per_session_rate_limiter")

	namedLogger.Info("per-session rate limiter initialized",
		zap.Float64("requests_per_second_per_session", cfg.RequestsPerSecond),
		zap.Int("burst_size_per_session", cfg.BurstSize),
	)

	return &PerSessionLimiter{
		cfg:    cfg,
		logger: namedLogger,
	}
}

// Allow checks whether the given session is permitted to make a request.
// If the session has not been seen before, a new limiter is created atomically.
// Returns true if a token was available, false if the session's rate limit is exceeded.
//
// This method is safe for concurrent use from multiple goroutines.
func (p *PerSessionLimiter) Allow(sessionID string) bool {
	entry := p.getOrCreate(sessionID)
	return entry.limiter.Allow()
}

// Wait blocks until a token is available for the given session or the context
// is cancelled. This is the blocking variant of Allow.
func (p *PerSessionLimiter) Wait(ctx context.Context, sessionID string) error {
	entry := p.getOrCreate(sessionID)
	return entry.limiter.Wait(ctx)
}

// getOrCreate retrieves the limiter for a session, creating one if it does not exist.
// Uses sync.Map.LoadOrStore for atomic, race-free creation.
func (p *PerSessionLimiter) getOrCreate(sessionID string) *sessionEntry {
	now := time.Now()

	// Fast path: session already has a limiter.
	if val, ok := p.limiters.Load(sessionID); ok {
		entry := val.(*sessionEntry)
		entry.lastSeen = now
		return entry
	}

	// Slow path: create a new limiter for this session.
	newEntry := &sessionEntry{
		limiter:  rate.NewLimiter(rate.Limit(p.cfg.RequestsPerSecond), p.cfg.BurstSize),
		lastSeen: now,
	}

	// LoadOrStore ensures that if another goroutine created the entry between
	// our Load and this call, we use theirs instead of ours.
	actual, loaded := p.limiters.LoadOrStore(sessionID, newEntry)
	if loaded {
		// Another goroutine beat us — use the existing entry.
		entry := actual.(*sessionEntry)
		entry.lastSeen = now
		return entry
	}

	p.logger.Debug("created rate limiter for new session",
		zap.String("session_id", sessionID),
	)

	return newEntry
}

// Cleanup removes limiters for sessions that have been idle longer than maxAge.
// This should be called periodically (e.g., via a time.Ticker in a background
// goroutine) to prevent unbounded memory growth from accumulated session limiters.
//
// A typical maxAge for exam proctoring is 4-6 hours, matching the maximum
// expected exam duration plus a buffer for late-arriving events.
//
// Returns the number of sessions removed for operational visibility.
func (p *PerSessionLimiter) Cleanup(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	removed := 0

	p.limiters.Range(func(key, value interface{}) bool {
		entry := value.(*sessionEntry)
		if entry.lastSeen.Before(cutoff) {
			p.limiters.Delete(key)
			removed++
		}
		return true // continue iteration
	})

	if removed > 0 {
		p.logger.Info("cleaned up stale session rate limiters",
			zap.Int("removed", removed),
			zap.Duration("max_age", maxAge),
		)
	}

	return removed
}

// Size returns the approximate number of active session limiters.
// This is useful for metrics and capacity monitoring. The count is approximate
// because concurrent modifications may occur during iteration.
func (p *PerSessionLimiter) Size() int {
	count := 0
	p.limiters.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
