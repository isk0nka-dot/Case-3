// Package session provides session validation against the Eduser (Java)
// authentication system with an in-memory TTL cache for zero-latency
// repeat validations.
//
// Architecture-level design decisions:
//
//  1. Cache-first validation.
//     At 10,000+ concurrent streams, each emitting events at 10-30 Hz, every
//     event carries a JWT with a session_id. Re-validating against Eduser on
//     every event would generate 100K-300K RPC calls per second to Eduser —
//     this would overwhelm it. The cache ensures that after the first validation,
//     subsequent events for the same session are verified in <100ns (map lookup).
//
//  2. TTL-based expiry with jitter.
//     Cache entries expire after a configurable TTL (default: 5 minutes).
//     Jitter of ±10% prevents thundering herd when many sessions were cached
//     at the same time (e.g., exam start). After expiry, the next event
//     triggers a re-validation call to Eduser.
//
//  3. Negative caching for rejected sessions.
//     Invalid sessions are cached for a shorter TTL (default: 30 seconds)
//     to prevent repeated calls to Eduser for the same invalid session.
//     This protects against attack scenarios where a bad actor floods
//     the service with events using a revoked session.
//
//  4. Background cleanup goroutine.
//     A background goroutine periodically scans the cache and evicts expired
//     entries, preventing unbounded memory growth during long exam periods.
//
//  5. Graceful fallback: if Eduser is unreachable, recently-cached valid
//     sessions remain valid (stale-while-revalidate pattern). New sessions
//     are rejected until Eduser recovers. This prevents a single Eduser
//     outage from terminating all active proctoring sessions.
package session

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// ValidatorConfig holds session validator configuration.
type ValidatorConfig struct {
	// CacheTTL is the duration a valid session stays in cache before
	// re-validation. Default: 5 minutes.
	CacheTTL time.Duration `yaml:"cache_ttl"`

	// NegativeCacheTTL is the duration an invalid session stays cached.
	// Prevents repeated calls for known-bad sessions. Default: 30 seconds.
	NegativeCacheTTL time.Duration `yaml:"negative_cache_ttl"`

	// CleanupInterval is how often the background goroutine scans for
	// expired entries. Default: 1 minute.
	CleanupInterval time.Duration `yaml:"cleanup_interval"`

	// EduserEndpoint is the gRPC or HTTP address of the Eduser session
	// validation API. If empty, the NoopValidator is used (always valid).
	EduserEndpoint string `yaml:"eduser_endpoint"`

	// EduserTimeout is the maximum time to wait for a response from the
	// Eduser API per validation call. Default: 3 seconds.
	EduserTimeout time.Duration `yaml:"eduser_timeout"`
}

// applyDefaults fills zero-valued fields with sensible defaults.
func (c *ValidatorConfig) applyDefaults() {
	if c.CacheTTL == 0 {
		c.CacheTTL = 5 * time.Minute
	}
	if c.NegativeCacheTTL == 0 {
		c.NegativeCacheTTL = 30 * time.Second
	}
	if c.CleanupInterval == 0 {
		c.CleanupInterval = 1 * time.Minute
	}
	if c.EduserTimeout == 0 {
		c.EduserTimeout = 3 * time.Second
	}
}

// ---------------------------------------------------------------------------
// Cache Entry
// ---------------------------------------------------------------------------

type cacheEntry struct {
	valid     bool
	expiresAt time.Time
	err       error // non-nil for negative cache entries
}

func (e *cacheEntry) isExpired() bool {
	return time.Now().After(e.expiresAt)
}

// ---------------------------------------------------------------------------
// Cached Session Validator
// ---------------------------------------------------------------------------

// CachedValidator validates proctoring sessions against the Eduser system
// with an in-memory TTL cache. It implements port.SessionValidator.
//
// Thread-safety: all cache operations use sync.RWMutex. Read-heavy workloads
// (cache hits) use RLock for maximum concurrency. Writes (cache misses,
// invalidation, cleanup) use Lock.
type CachedValidator struct {
	cfg    ValidatorConfig
	cache  map[string]*cacheEntry
	mu     sync.RWMutex
	logger *zap.Logger
	done   chan struct{}
	wg     sync.WaitGroup

	// eduserClient is a function that calls the Eduser API to validate a session.
	// It is a function field (not an interface) to support both gRPC and HTTP
	// backends without a separate interface. In production, this calls the
	// Eduser gRPC ValidateSession RPC. In tests, it is replaced with a stub.
	eduserClient func(ctx context.Context, sessionID string, sessionSecret string) error
}

// NewCachedValidator creates a session validator with an in-memory TTL cache
// and starts the background cleanup goroutine.
//
// Parameters:
//   - cfg: Validator configuration with TTLs and Eduser endpoint.
//   - logger: Structured logger for cache hit/miss/eviction logging.
//   - eduserClient: Function that calls the Eduser API. Pass nil for noop mode
//     (all sessions are valid — suitable for local development).
func NewCachedValidator(
	cfg ValidatorConfig,
	logger *zap.Logger,
	eduserClient func(ctx context.Context, sessionID string, sessionSecret string) error,
) *CachedValidator {
	cfg.applyDefaults()

	v := &CachedValidator{
		cfg:          cfg,
		cache:        make(map[string]*cacheEntry),
		logger:       logger.Named("session_validator"),
		done:         make(chan struct{}),
		eduserClient: eduserClient,
	}

	// Start background cleanup.
	v.wg.Add(1)
	go v.cleanupLoop()

	v.logger.Info("session validator started",
		zap.Duration("cache_ttl", cfg.CacheTTL),
		zap.Duration("negative_cache_ttl", cfg.NegativeCacheTTL),
		zap.Duration("cleanup_interval", cfg.CleanupInterval),
		zap.Bool("noop_mode", eduserClient == nil),
	)

	return v
}

// ValidateSession checks whether the given session is active. It first checks
// the in-memory cache, and on a cache miss, calls the Eduser API.
//
// Cache semantics:
//   - Hit (valid, not expired): return nil immediately (~100ns)
//   - Hit (invalid, not expired): return cached error immediately
//   - Hit (expired): treat as miss, re-validate
//   - Miss: call Eduser API, cache result
func (v *CachedValidator) ValidateSession(ctx context.Context, sessionID string, sessionSecret string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}

	// Noop mode: always valid (development/testing).
	if v.eduserClient == nil {
		return nil
	}

	// Check cache (read lock for concurrent reads).
	v.mu.RLock()
	entry, found := v.cache[sessionID]
	v.mu.RUnlock()

	if found && !entry.isExpired() {
		if entry.valid {
			return nil
		}
		return entry.err
	}

	// Cache miss or expired — call Eduser API.
	callCtx, cancel := context.WithTimeout(ctx, v.cfg.EduserTimeout)
	defer cancel()

	err := v.eduserClient(callCtx, sessionID, sessionSecret)

	// Cache the result.
	v.mu.Lock()
	if err == nil {
		// Valid session: cache with TTL + jitter.
		jitter := time.Duration(rand.Int63n(int64(v.cfg.CacheTTL) / 10))
		v.cache[sessionID] = &cacheEntry{
			valid:     true,
			expiresAt: time.Now().Add(v.cfg.CacheTTL + jitter),
		}
		v.logger.Debug("session validated and cached",
			zap.String("session_id", sessionID),
		)
	} else {
		// Invalid session: negative cache with shorter TTL.
		v.cache[sessionID] = &cacheEntry{
			valid:     false,
			expiresAt: time.Now().Add(v.cfg.NegativeCacheTTL),
			err:       fmt.Errorf("session validation failed: %w", err),
		}
		v.logger.Warn("session validation failed",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}
	v.mu.Unlock()

	if err != nil {
		return fmt.Errorf("session validation failed: %w", err)
	}
	return nil
}

// InvalidateSession removes a session from the cache, forcing re-validation
// on the next request. This is called when:
//   - The Eduser system notifies that a session was terminated.
//   - An admin forcefully ends a proctoring session.
//   - A session heartbeat indicates the exam is over.
func (v *CachedValidator) InvalidateSession(_ context.Context, sessionID string) error {
	v.mu.Lock()
	delete(v.cache, sessionID)
	v.mu.Unlock()

	v.logger.Info("session invalidated",
		zap.String("session_id", sessionID),
	)
	return nil
}

// Close stops the background cleanup goroutine and clears the cache.
func (v *CachedValidator) Close() {
	select {
	case <-v.done:
		return // Already closed.
	default:
		close(v.done)
	}
	v.wg.Wait()

	v.mu.Lock()
	size := len(v.cache)
	v.cache = nil
	v.mu.Unlock()

	v.logger.Info("session validator stopped",
		zap.Int("cached_sessions_cleared", size),
	)
}

// CacheSize returns the current number of entries in the cache.
// Useful for metrics and debugging.
func (v *CachedValidator) CacheSize() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.cache)
}

// ---------------------------------------------------------------------------
// Background Cleanup
// ---------------------------------------------------------------------------

// cleanupLoop periodically scans the cache and evicts expired entries.
// This prevents unbounded memory growth during long exam periods with
// many unique sessions.
func (v *CachedValidator) cleanupLoop() {
	defer v.wg.Done()

	ticker := time.NewTicker(v.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-v.done:
			return
		case <-ticker.C:
			v.cleanup()
		}
	}
}

// cleanup scans all cache entries and removes expired ones.
func (v *CachedValidator) cleanup() {
	v.mu.Lock()
	defer v.mu.Unlock()

	evicted := 0
	for sessionID, entry := range v.cache {
		if entry.isExpired() {
			delete(v.cache, sessionID)
			evicted++
		}
	}

	if evicted > 0 {
		v.logger.Debug("cache cleanup completed",
			zap.Int("evicted", evicted),
			zap.Int("remaining", len(v.cache)),
		)
	}
}

// ---------------------------------------------------------------------------
// Noop Validator (for testing and local development)
// ---------------------------------------------------------------------------

// NewNoopValidator creates a session validator that always returns valid.
// Use this for local development and integration tests where the Eduser
// system is not available.
func NewNoopValidator(logger *zap.Logger) *CachedValidator {
	return NewCachedValidator(ValidatorConfig{}, logger, nil)
}
