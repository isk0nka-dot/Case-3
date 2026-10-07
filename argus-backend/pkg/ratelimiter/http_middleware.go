// Package ratelimiter — HTTP rate limiting middleware for the admin REST API.
//
// This file provides HTTP-specific rate limiting using the same token-bucket
// approach as the gRPC interceptors. It enforces two layers of protection:
//
//   - Global rate limit: Caps the total requests/second across all HTTP clients.
//     This protects the admin API from being overwhelmed regardless of source.
//
//   - Per-IP rate limit: Caps the requests/second from any single client IP.
//     This prevents a single user or attacker from monopolising the admin API
//     while leaving room for other legitimate clients.
//
// When a limit is exceeded, the middleware responds with HTTP 429 (Too Many
// Requests) and includes standard rate-limit headers for client-side handling.
//
// Design decisions:
//   - In-memory sync.Map for per-IP tracking (same pattern as PerSessionLimiter).
//     For a single-instance deployment, this is sufficient. For horizontal scaling,
//     swap with a Redis-backed store.
//   - Stale IP entries are cleaned up periodically to prevent memory leaks
//     from short-lived clients.
//   - The global limiter is separate from the gRPC global limiter because the
//     admin API and event ingestion API have different throughput requirements.
package ratelimiter

import (
	"encoding/json"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ipEntry holds a per-IP rate limiter along with metadata for staleness tracking.
type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// HTTPRateLimiter provides per-IP and global rate limiting for HTTP endpoints.
type HTTPRateLimiter struct {
	globalLimiter *rate.Limiter
	ipLimiters    sync.Map // map[string]*ipEntry
	perIPRPS      float64
	perIPBurst    int
	logger        *zap.Logger
}

// NewHTTPRateLimiter creates a new HTTP rate limiter with global and per-IP limits.
func NewHTTPRateLimiter(globalRPS float64, globalBurst int, perIPRPS float64, perIPBurst int, logger *zap.Logger) *HTTPRateLimiter {
	namedLogger := logger.Named("http_rate_limiter")

	namedLogger.Info("http rate limiter initialized",
		zap.Float64("global_rps", globalRPS),
		zap.Int("global_burst", globalBurst),
		zap.Float64("per_ip_rps", perIPRPS),
		zap.Int("per_ip_burst", perIPBurst),
	)

	return &HTTPRateLimiter{
		globalLimiter: rate.NewLimiter(rate.Limit(globalRPS), globalBurst),
		perIPRPS:      perIPRPS,
		perIPBurst:    perIPBurst,
		logger:        namedLogger,
	}
}

// Middleware returns an http.Handler middleware that enforces rate limits.
// It should be wrapped around the admin API mux.
func (rl *HTTPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Layer 1: Global rate limit — protects overall system capacity.
		if !rl.globalLimiter.Allow() {
			retryAfter := rl.computeRetryAfter(rl.globalLimiter)
			rl.logger.Warn("global HTTP rate limit exceeded",
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("path", r.URL.Path),
			)
			rl.reject(w, retryAfter)
			return
		}

		// Layer 2: Per-IP rate limit — prevents single-client abuse.
		clientIP := extractIP(r)
		ipLimiter := rl.getOrCreateIPLimiter(clientIP)

		if !ipLimiter.Allow() {
			retryAfter := rl.computeRetryAfter(ipLimiter)
			rl.logger.Warn("per-IP HTTP rate limit exceeded",
				zap.String("client_ip", clientIP),
				zap.String("path", r.URL.Path),
			)
			rl.reject(w, retryAfter)
			return
		}

		// Request allowed — proceed to the handler.
		next.ServeHTTP(w, r)
	})
}

// getOrCreateIPLimiter retrieves or creates a rate limiter for the given IP.
func (rl *HTTPRateLimiter) getOrCreateIPLimiter(ip string) *rate.Limiter {
	now := time.Now()

	// Fast path: IP already has a limiter.
	if val, ok := rl.ipLimiters.Load(ip); ok {
		entry := val.(*ipEntry)
		entry.lastSeen = now
		return entry.limiter
	}

	// Slow path: create a new limiter for this IP.
	newEntry := &ipEntry{
		limiter:  rate.NewLimiter(rate.Limit(rl.perIPRPS), rl.perIPBurst),
		lastSeen: now,
	}

	actual, loaded := rl.ipLimiters.LoadOrStore(ip, newEntry)
	if loaded {
		entry := actual.(*ipEntry)
		entry.lastSeen = now
		return entry.limiter
	}

	return newEntry.limiter
}

// Cleanup removes stale IP entries that have not been seen for longer than maxAge.
// Call this periodically (e.g., every 5 minutes) to prevent unbounded memory growth.
// Returns the number of entries removed.
func (rl *HTTPRateLimiter) Cleanup(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	removed := 0

	rl.ipLimiters.Range(func(key, value interface{}) bool {
		entry := value.(*ipEntry)
		if entry.lastSeen.Before(cutoff) {
			rl.ipLimiters.Delete(key)
			removed++
		}
		return true
	})

	if removed > 0 {
		rl.logger.Info("cleaned up stale IP rate limiters",
			zap.Int("removed", removed),
			zap.Duration("max_age", maxAge),
		)
	}

	return removed
}

// reject writes a 429 Too Many Requests response with standard headers.
func (rl *HTTPRateLimiter) reject(w http.ResponseWriter, retryAfterMs int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", formatRetryAfterSeconds(retryAfterMs))
	w.Header().Set("X-RateLimit-Retry-After-Ms", formatRetryAfterMs(retryAfterMs))
	w.WriteHeader(http.StatusTooManyRequests)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":          "Rate limit exceeded",
		"retry_after_ms": retryAfterMs,
	})
}

// computeRetryAfter estimates milliseconds until the next token is available.
func (rl *HTTPRateLimiter) computeRetryAfter(limiter *rate.Limiter) int {
	// The reservation approach gives us a precise wait time.
	res := limiter.Reserve()
	delay := res.Delay()
	res.Cancel()

	ms := int(math.Ceil(float64(delay) / float64(time.Millisecond)))
	if ms < 100 {
		ms = 100 // Minimum retry-after to avoid spin loops.
	}
	return ms
}

// extractIP returns the client's real IP address, considering reverse proxies.
func extractIP(r *http.Request) string {
	// Check X-Forwarded-For (set by reverse proxies/load balancers).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if parts := strings.SplitN(xff, ",", 2); len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP (set by Nginx).
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr (strip port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// formatRetryAfterSeconds converts milliseconds to seconds (rounded up) for the
// standard Retry-After header (which uses seconds).
func formatRetryAfterSeconds(ms int) string {
	seconds := (ms + 999) / 1000
	if seconds < 1 {
		seconds = 1
	}
	return strings.TrimRight(strings.TrimRight(
		// Using integer seconds for the Retry-After header.
		formatInt(seconds), "0"), ".")
}

func formatRetryAfterMs(ms int) string {
	return formatInt(ms)
}

func formatInt(n int) string {
	// Simple int-to-string without importing strconv (we only need positive ints).
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	// Reverse.
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
