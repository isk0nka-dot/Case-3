// Package port defines the interfaces (ports) that the application layer
// requires from infrastructure. This is the core of Clean Architecture —
// the application layer depends only on abstractions, never on concrete implementations.
package port

import (
	"context"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// EventWriter is the primary outbound port for persisting proctoring events.
// Implementations include Kafka producers and ClickHouse batch writers.
//
// Contract:
//   - Write must be non-blocking for the caller (async buffered write).
//   - WriteBatch must guarantee atomicity — all events are written or none.
//   - Implementations must handle retries internally.
type EventWriter interface {
	// Write sends a single event to the downstream system.
	// Returns an error only if the event cannot be buffered (backpressure).
	Write(ctx context.Context, event *entity.ProctoringEvent) error

	// WriteBatch sends a batch of events atomically.
	WriteBatch(ctx context.Context, events []*entity.ProctoringEvent) error

	// Close flushes pending writes and releases resources.
	// Must be called during graceful shutdown.
	Close() error
}

// EventValidator is the inbound port for validating events against
// the current exam proctoring settings. The application layer uses this
// to check whether an incoming event type is actually enabled for the exam.
type EventValidator interface {
	// IsEventEnabled checks whether the given event type is enabled
	// for the specified exam configuration.
	IsEventEnabled(ctx context.Context, examID string, eventType string) (bool, error)
}

// HealthChecker provides health status for infrastructure dependencies.
type HealthChecker interface {
	// Check returns nil if the dependency is healthy, error otherwise.
	Check(ctx context.Context) error
	// Name returns the human-readable name of the dependency.
	Name() string
}
