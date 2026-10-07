// Package circuitbreaker provides a resilience wrapper around sony/gobreaker for
// protecting downstream service calls (Kafka, ClickHouse) from cascading failures.
//
// The circuit breaker implements the standard three-state model:
//
//   - Closed: Requests flow normally. Failures are counted. When the failure
//     threshold is reached (by count or ratio), the breaker transitions to Open.
//   - Open: All requests are immediately rejected with ErrCircuitOpen. After the
//     configured timeout, the breaker transitions to Half-Open.
//   - Half-Open: A limited number of probe requests are allowed through. If they
//     succeed, the breaker returns to Closed. If any fail, it returns to Open.
//
// Design decisions:
//   - Wraps sony/gobreaker rather than reimplementing — battle-tested library
//     with correct concurrency semantics.
//   - Supports two tripping strategies: absolute failure count and failure ratio.
//     If both are configured, the breaker trips when either condition is met.
//   - Structured logging on every state transition for operational visibility.
//   - State() returns a human-readable string for health endpoints and metrics.
package circuitbreaker

import (
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

// ErrCircuitOpen is returned when a call is attempted while the circuit breaker
// is in the Open state. Callers should handle this as a fast-fail signal and
// avoid retrying immediately.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Config holds the configuration for a circuit breaker instance.
// At least one tripping strategy (FailureThreshold or FailureRatio) must be set,
// otherwise the breaker will never trip.
type Config struct {
	// Name identifies this circuit breaker in logs and metrics.
	// Should describe the downstream service (e.g., "kafka-producer", "clickhouse-writer").
	Name string `yaml:"name"`

	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is in the Half-Open state. Once MaxRequests
	// succeed consecutively, the breaker transitions back to Closed.
	// Default: 1.
	MaxRequests uint32 `yaml:"max_requests"`

	// Interval is the cyclic time period of the Closed state. If Interval is 0,
	// the circuit breaker does not clear internal failure counts during the Closed state.
	// If Interval > 0, the failure counters are reset at the start of each interval.
	// This prevents old failures from accumulating indefinitely.
	Interval time.Duration `yaml:"interval"`

	// Timeout is the duration the circuit breaker remains in the Open state before
	// transitioning to Half-Open. During this period, all calls are rejected.
	// Default: 60s.
	Timeout time.Duration `yaml:"timeout"`

	// FailureThreshold is the absolute number of consecutive failures required
	// to trip the circuit breaker from Closed to Open. If set to 0, only
	// FailureRatio is used for tripping.
	FailureThreshold uint32 `yaml:"failure_threshold"`

	// FailureRatio is the ratio of failures to total requests (0.0 to 1.0) that
	// will trip the circuit breaker. For example, 0.6 means the breaker trips
	// when 60% or more of requests in the current interval have failed.
	// A minimum of 5 requests is required before the ratio is evaluated to
	// avoid tripping on small sample sizes.
	// If set to 0, only FailureThreshold is used.
	FailureRatio float64 `yaml:"failure_ratio"`

	// OnOpenCallback is an optional function invoked when the breaker transitions
	// to the Open state. It runs in a goroutine to avoid blocking the calling path.
	// Use this to fire critical alerts (e.g., Telegram) when a downstream service
	// is confirmed unhealthy.
	OnOpenCallback func(name string)
}

// minRequestsForRatio is the minimum number of total requests required before
// the failure ratio is evaluated. This prevents the breaker from tripping on
// statistically insignificant samples (e.g., 1 failure out of 1 request = 100%).
const minRequestsForRatio = 5

// Breaker wraps a sony/gobreaker CircuitBreaker with structured logging
// and a simplified API for the event-collector service.
type Breaker struct {
	cb       *gobreaker.CircuitBreaker
	settings gobreaker.Settings // retained for Reset()
	logger   *zap.Logger
}

// New creates a new circuit breaker with the given configuration. The breaker
// starts in the Closed state, allowing all requests through.
//
// The logger is used for state transition events. It is named with the breaker
// name for easy filtering in aggregated logs.
func New(cfg Config, logger *zap.Logger) *Breaker {
	namedLogger := logger.Named("circuit_breaker").With(zap.String("breaker", cfg.Name))

	// Apply defaults for optional fields.
	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = 1
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}

	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,

		// ReadyToTrip determines whether the circuit breaker should trip based
		// on the current failure counts. It is called on every failure while
		// the breaker is in the Closed state.
		ReadyToTrip: buildReadyToTrip(cfg, namedLogger),

		// OnStateChange logs every state transition for operational visibility.
		// State changes are critical events that should always be visible in logs.
		// If the breaker transitions to Open and an OnOpenCallback is configured,
		// the callback fires in a goroutine to avoid blocking the calling path.
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			namedLogger.Warn("circuit breaker state changed",
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
			if to == gobreaker.StateOpen && cfg.OnOpenCallback != nil {
				go cfg.OnOpenCallback(name)
			}
		},
	}

	return &Breaker{
		cb:       gobreaker.NewCircuitBreaker(settings),
		settings: settings,
		logger:   namedLogger,
	}
}

// buildReadyToTrip constructs the tripping function based on the configured strategy.
// If both FailureThreshold and FailureRatio are set, the breaker trips when either
// condition is met (logical OR). This provides defense-in-depth: the absolute count
// catches sustained failures, while the ratio catches degraded-but-not-dead scenarios.
func buildReadyToTrip(cfg Config, logger *zap.Logger) func(counts gobreaker.Counts) bool {
	return func(counts gobreaker.Counts) bool {
		// Strategy 1: Absolute failure count.
		// Trip if the number of consecutive failures meets or exceeds the threshold.
		if cfg.FailureThreshold > 0 && counts.ConsecutiveFailures >= cfg.FailureThreshold {
			logger.Info("tripping circuit breaker: failure threshold reached",
				zap.Uint32("consecutive_failures", counts.ConsecutiveFailures),
				zap.Uint32("threshold", cfg.FailureThreshold),
			)
			return true
		}

		// Strategy 2: Failure ratio.
		// Only evaluate the ratio after a minimum number of requests to avoid
		// tripping on statistically insignificant samples.
		if cfg.FailureRatio > 0 && counts.Requests >= minRequestsForRatio {
			ratio := float64(counts.TotalFailures) / float64(counts.Requests)
			if ratio >= cfg.FailureRatio {
				logger.Info("tripping circuit breaker: failure ratio exceeded",
					zap.Float64("current_ratio", ratio),
					zap.Float64("threshold_ratio", cfg.FailureRatio),
					zap.Uint32("total_requests", counts.Requests),
					zap.Uint32("total_failures", counts.TotalFailures),
				)
				return true
			}
		}

		return false
	}
}

// Execute runs the given function within the circuit breaker's protection.
//
// If the breaker is Closed or Half-Open, fn is executed. If fn returns an error,
// it is counted as a failure. If fn succeeds, it is counted as a success.
//
// If the breaker is Open, fn is NOT executed and ErrCircuitOpen is returned
// immediately. This provides fast-fail behavior and prevents piling up requests
// against a known-unhealthy downstream service.
//
// Usage:
//
//	result, err := breaker.Execute(func() (interface{}, error) {
//	    return kafkaProducer.Send(ctx, msg)
//	})
func (b *Breaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
	result, err := b.cb.Execute(fn)
	if err != nil {
		// Wrap gobreaker's sentinel error with our own for cleaner API boundaries.
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("%w: %s", ErrCircuitOpen, b.cb.Name())
		}
		return nil, err
	}
	return result, nil
}

// State returns the current state of the circuit breaker as a human-readable string.
// Possible values: "closed", "open", "half-open".
//
// This is intended for health check endpoints and metrics exporters.
func (b *Breaker) State() string {
	return b.cb.State().String()
}

// Name returns the configured name of this circuit breaker.
func (b *Breaker) Name() string {
	return b.cb.Name()
}

// Counts returns the current internal counters of the circuit breaker.
// This is useful for metrics and debugging. The returned Counts struct contains
// fields for Requests, TotalSuccesses, TotalFailures, and ConsecutiveFailures.
func (b *Breaker) Counts() gobreaker.Counts {
	return b.cb.Counts()
}

// Reset manually forces the circuit breaker back to the Closed state,
// clearing all failure counters. This is intended for admin panic-button
// recovery when a breaker is stuck in the Open state after the downstream
// service has actually recovered but the probe window has not elapsed yet.
//
// Implementation: gobreaker v1 does not expose a Reset() method, so we
// create a fresh CircuitBreaker instance with the same settings.
// The old instance is garbage-collected.
//
// This method should only be called via the admin system-reset API.
func (b *Breaker) Reset() {
	previousState := b.cb.State().String()
	b.cb = gobreaker.NewCircuitBreaker(b.settings)
	b.logger.Info("circuit breaker manually reset by admin",
		zap.String("breaker", b.settings.Name),
		zap.String("previous_state", previousState),
		zap.String("new_state", "closed"),
	)
}
