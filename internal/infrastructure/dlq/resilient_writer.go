package dlq

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/pkg/circuitbreaker"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// ResilientWriterConfig controls the DLQ-backed resilient writer behaviour.
type ResilientWriterConfig struct {
	// ReclamationInterval is how often the background goroutine attempts
	// to drain the DLQ back to the primary pipeline.
	// Default: 30 seconds.
	ReclamationInterval time.Duration

	// ReclamationBatchSize is the maximum number of events drained per cycle.
	// Default: 100.
	ReclamationBatchSize int

	// HeartbeatInterval is how often a degraded-mode heartbeat is sent to
	// Telegram while Kafka is unavailable. Keeps admins aware that the system
	// is still ingesting events into the DLQ.
	// Default: 5 minutes.
	HeartbeatInterval time.Duration

	// CircuitBreaker configures the circuit breaker wrapping the primary writer.
	// This is separate from the ClickHouse circuit breaker.
	CircuitBreaker circuitbreaker.Config
}

// applyResilientDefaults fills zero-valued fields with production-safe defaults.
func applyResilientDefaults(cfg *ResilientWriterConfig) {
	if cfg.ReclamationInterval == 0 {
		cfg.ReclamationInterval = 30 * time.Second
	}
	if cfg.ReclamationBatchSize == 0 {
		cfg.ReclamationBatchSize = 100
	}
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 5 * time.Minute
	}
}

// ---------------------------------------------------------------------------
// ResilientWriter — port.EventWriter decorator with DLQ fallback
// ---------------------------------------------------------------------------

// ResilientWriterMetrics holds atomic counter snapshots.
type ResilientWriterMetrics struct {
	TotalPrimaryOK   int64
	TotalDLQFallback int64
	TotalReclaimed   int64
	ReclaimErrors    int64
}

// ResilientWriter wraps a port.EventWriter with circuit breaker protection and
// BadgerDB DLQ fallback. It implements port.EventWriter itself, acting as a
// transparent proxy that the IngestUseCase is unaware of.
//
// Write path:
//  1. Try primary writer through circuit breaker → success → done.
//  2. Kafka fails or breaker OPEN → save to BadgerDB → return nil (deferred delivery).
//  3. BadgerDB fails → return error (event lost, CRITICAL alert already fired by Store).
//
// Background goroutines:
//   - Reclamation: polls BadgerDB every ReclamationInterval. When the circuit
//     breaker is CLOSED, drains events back to Kafka idempotently.
//   - Heartbeat: while breaker is not CLOSED, sends periodic Telegram alerts
//     so admins know the system is in Buffered Mode and still accepting events.
type ResilientWriter struct {
	primary  port.EventWriter
	dlqStore *Store
	breaker  *circuitbreaker.Breaker
	alerter  alerting.Provider
	logger   *zap.Logger
	cfg      ResilientWriterConfig

	done chan struct{}
	wg   sync.WaitGroup

	// Lock-free metrics.
	totalPrimaryOK   atomic.Int64
	totalDLQFallback atomic.Int64
	totalReclaimed   atomic.Int64
	reclaimErrors    atomic.Int64

	// Degraded-mode tracking.
	degradedSince atomic.Int64 // Unix timestamp when breaker first opened (0 = healthy)
	lastHeartbeat atomic.Int64 // Unix timestamp of last heartbeat sent
}

// NewResilientWriter constructs the decorator. Call Start() to begin the
// background reclamation goroutine.
func NewResilientWriter(
	primary port.EventWriter,
	dlqStore *Store,
	cfg ResilientWriterConfig,
	logger *zap.Logger,
) *ResilientWriter {
	applyResilientDefaults(&cfg)

	rw := &ResilientWriter{
		primary:  primary,
		dlqStore: dlqStore,
		breaker:  circuitbreaker.New(cfg.CircuitBreaker, logger),
		logger:   logger.Named("resilient_writer"),
		cfg:      cfg,
		done:     make(chan struct{}),
	}

	return rw
}

// SetAlerter wires the Telegram alerter to both the resilient writer and the
// underlying DLQ store.
func (rw *ResilientWriter) SetAlerter(a alerting.Provider) {
	rw.alerter = a
	rw.dlqStore.SetAlerter(a)
}

// Start begins the background reclamation and heartbeat goroutines. Must be
// called after construction and before first Write.
func (rw *ResilientWriter) Start() {
	rw.wg.Add(2)
	go rw.reclamationLoop()
	go rw.heartbeatLoop()

	rw.logger.Info("resilient writer started",
		zap.Duration("reclamation_interval", rw.cfg.ReclamationInterval),
		zap.Int("reclamation_batch_size", rw.cfg.ReclamationBatchSize),
		zap.Duration("heartbeat_interval", rw.cfg.HeartbeatInterval),
		zap.String("breaker", rw.breaker.Name()),
	)
}

// ---------------------------------------------------------------------------
// port.EventWriter implementation
// ---------------------------------------------------------------------------

// Write sends a single event through the circuit breaker to the primary writer.
// If the primary write fails (or the breaker is open), the event is persisted
// to BadgerDB for later reclamation. Returns nil if the event is safely stored
// in either Kafka or the DLQ.
func (rw *ResilientWriter) Write(ctx context.Context, event *entity.ProctoringEvent) error {
	// Attempt primary write through circuit breaker.
	_, err := rw.breaker.Execute(func() (interface{}, error) {
		return nil, rw.primary.Write(ctx, event)
	})
	if err == nil {
		rw.totalPrimaryOK.Add(1)
		rw.markHealthy()
		return nil
	}

	// Primary failed or circuit is open — fall back to DLQ.
	rw.logger.Warn("primary write failed, falling back to DLQ",
		zap.String("event_id", event.EventID),
		zap.String("breaker_state", rw.breaker.State()),
		zap.Error(err),
	)
	rw.totalDLQFallback.Add(1)
	rw.markDegraded()

	if dlqErr := rw.dlqStore.Save(event); dlqErr != nil {
		// CRITICAL: even DLQ failed — event is lost.
		// The Store.Save() already fired a Telegram alert.
		return fmt.Errorf("both primary and DLQ failed: primary=%w, dlq=%v", err, dlqErr)
	}

	// Event is safely in DLQ — return nil (accepted, deferred delivery).
	return nil
}

// WriteBatch sends a batch of events through the circuit breaker to the primary
// writer. If the primary write fails, the entire batch is persisted to BadgerDB.
func (rw *ResilientWriter) WriteBatch(ctx context.Context, events []*entity.ProctoringEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Attempt primary write through circuit breaker.
	_, err := rw.breaker.Execute(func() (interface{}, error) {
		return nil, rw.primary.WriteBatch(ctx, events)
	})
	if err == nil {
		rw.totalPrimaryOK.Add(int64(len(events)))
		rw.markHealthy()
		return nil
	}

	// Primary failed — fall back to DLQ for entire batch.
	rw.logger.Warn("primary batch write failed, falling back to DLQ",
		zap.Int("batch_size", len(events)),
		zap.String("breaker_state", rw.breaker.State()),
		zap.Error(err),
	)
	rw.totalDLQFallback.Add(int64(len(events)))
	rw.markDegraded()

	if dlqErr := rw.dlqStore.SaveBatch(events); dlqErr != nil {
		return fmt.Errorf("both primary and DLQ failed: primary=%w, dlq=%v", err, dlqErr)
	}

	return nil
}

// Close stops the reclamation goroutine, performs a final reclamation attempt,
// closes the DLQ store, and then closes the underlying primary writer.
func (rw *ResilientWriter) Close() error {
	// 1. Stop reclamation goroutine.
	close(rw.done)
	rw.wg.Wait()

	// 2. Final best-effort reclamation (primary is still alive at this point).
	rw.reclaim()

	remaining := rw.dlqStore.Size()
	if remaining > 0 {
		rw.logger.Warn("dlq has unreclaimed entries after final drain",
			zap.Int64("remaining", remaining),
		)
	}

	// 3. Close DLQ store (runs final GC, closes BadgerDB).
	if err := rw.dlqStore.Close(); err != nil {
		rw.logger.Error("dlq store close failed", zap.Error(err))
	}

	// 4. Close underlying primary writer (Kafka).
	return rw.primary.Close()
}

// Metrics returns a snapshot of resilient writer statistics.
func (rw *ResilientWriter) Metrics() ResilientWriterMetrics {
	return ResilientWriterMetrics{
		TotalPrimaryOK:   rw.totalPrimaryOK.Load(),
		TotalDLQFallback: rw.totalDLQFallback.Load(),
		TotalReclaimed:   rw.totalReclaimed.Load(),
		ReclaimErrors:    rw.reclaimErrors.Load(),
	}
}

// DLQSize returns the number of events currently in the DLQ.
func (rw *ResilientWriter) DLQSize() int64 {
	return rw.dlqStore.Size()
}

// BreakerState returns the current circuit breaker state: "closed", "open", or "half-open".
func (rw *ResilientWriter) BreakerState() string {
	return rw.breaker.State()
}

// IsDegraded returns true if the system is operating in buffered/degraded mode.
func (rw *ResilientWriter) IsDegraded() bool {
	return rw.degradedSince.Load() > 0
}

// DegradedDuration returns how long the system has been in degraded mode.
// Returns 0 if the system is healthy.
func (rw *ResilientWriter) DegradedDuration() time.Duration {
	since := rw.degradedSince.Load()
	if since == 0 {
		return 0
	}
	return time.Since(time.Unix(since, 0))
}

// ---------------------------------------------------------------------------
// Degraded mode tracking
// ---------------------------------------------------------------------------

// markDegraded records the start of degraded mode (only on first failure).
func (rw *ResilientWriter) markDegraded() {
	rw.degradedSince.CompareAndSwap(0, time.Now().Unix())
}

// markHealthy clears degraded mode and sends a recovery notification.
func (rw *ResilientWriter) markHealthy() {
	prev := rw.degradedSince.Swap(0)
	if prev > 0 {
		// Just recovered from degraded mode.
		degradedFor := time.Since(time.Unix(prev, 0))
		rw.logger.Info("kafka connectivity restored, exiting buffered mode",
			zap.Duration("degraded_for", degradedFor),
			zap.Int64("dlq_remaining", rw.dlqStore.Size()),
		)

		if rw.alerter != nil {
			msg := alerting.MsgKafkaRecovered(
				degradedFor.Round(time.Second).String(),
				rw.dlqStore.Size(),
				time.Now().Format("2006-01-02 15:04:05 MST"),
			)
			_ = rw.alerter.SendDirect(msg)
		}
	}
}

// ---------------------------------------------------------------------------
// Heartbeat — periodic Telegram pings during degraded mode
// ---------------------------------------------------------------------------

// heartbeatLoop sends periodic "BUFFERED MODE" alerts to Telegram while the
// system is operating in degraded mode. This ensures admins know the system
// is still accepting events even when Kafka is unreachable.
func (rw *ResilientWriter) heartbeatLoop() {
	defer rw.wg.Done()

	ticker := time.NewTicker(rw.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rw.sendHeartbeatIfDegraded()
		case <-rw.done:
			return
		}
	}
}

// sendHeartbeatIfDegraded sends a Telegram heartbeat only when:
//  1. The system is in degraded mode (breaker not closed).
//  2. Enough time has passed since the last heartbeat (prevent spam).
func (rw *ResilientWriter) sendHeartbeatIfDegraded() {
	if !rw.IsDegraded() {
		return
	}

	if rw.alerter == nil {
		return
	}

	now := time.Now().Unix()
	last := rw.lastHeartbeat.Load()
	if last > 0 && now-last < int64(rw.cfg.HeartbeatInterval.Seconds()) {
		return // Too soon since last heartbeat.
	}
	rw.lastHeartbeat.Store(now)

	msg := alerting.MsgBufferedModeHeartbeat(
		rw.breaker.State(),
		rw.dlqStore.Size(),
		rw.DegradedDuration().Round(time.Second).String(),
		rw.totalDLQFallback.Load(),
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)
	_ = rw.alerter.SendDirect(msg)

	rw.logger.Warn("buffered mode heartbeat sent",
		zap.String("breaker_state", rw.breaker.State()),
		zap.Int64("dlq_size", rw.dlqStore.Size()),
		zap.Duration("degraded_for", rw.DegradedDuration()),
	)
}

// ---------------------------------------------------------------------------
// Reclamation — background drain of DLQ back to primary pipeline
// ---------------------------------------------------------------------------

// reclamationLoop is the background goroutine that periodically drains
// BadgerDB back to the primary writer when healthy.
func (rw *ResilientWriter) reclamationLoop() {
	defer rw.wg.Done()

	ticker := time.NewTicker(rw.cfg.ReclamationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rw.reclaim()
		case <-rw.done:
			return
		}
	}
}

// reclaim performs a single drain cycle: read a batch from DLQ, write to
// primary through the circuit breaker, delete from DLQ on success.
//
// Idempotency guarantee: Events are keyed by EventID. The ClickHouse
// ReplacingMergeTree deduplicates on (org_id, exam_id, session_id, event_id),
// so re-delivered events are collapsed. Kafka consumers use EventID as the
// dedup key. This means reclamation is safe to retry without side effects.
func (rw *ResilientWriter) reclaim() {
	// Only attempt reclamation if the circuit breaker is CLOSED (healthy).
	state := rw.breaker.State()
	if state != "closed" {
		rw.logger.Debug("skipping reclamation, breaker not closed",
			zap.String("state", state),
			zap.Int64("dlq_size", rw.dlqStore.Size()),
		)
		return
	}

	// Check if there's anything to reclaim.
	if rw.dlqStore.Size() == 0 {
		return
	}

	events, keys, err := rw.dlqStore.DrainBatch(rw.cfg.ReclamationBatchSize)
	if err != nil {
		rw.logger.Error("reclamation: failed to read DLQ", zap.Error(err))
		rw.reclaimErrors.Add(1)
		return
	}
	if len(events) == 0 {
		return
	}

	// Write batch to primary through circuit breaker.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, writeErr := rw.breaker.Execute(func() (interface{}, error) {
		return nil, rw.primary.WriteBatch(ctx, events)
	})
	if writeErr != nil {
		rw.reclaimErrors.Add(1)
		rw.logger.Warn("reclamation: primary write failed, events remain in DLQ",
			zap.Int("count", len(events)),
			zap.String("breaker_state", rw.breaker.State()),
			zap.Error(writeErr),
		)
		return // Events stay in BadgerDB for next cycle.
	}

	// Success — delete reclaimed entries from DLQ atomically.
	if err := rw.dlqStore.DeleteKeys(keys); err != nil {
		rw.logger.Error("reclamation: delete failed (possible duplicates on next cycle)",
			zap.Int("count", len(keys)),
			zap.Error(err),
		)
		// Not critical — worst case is duplicate delivery, which is acceptable
		// because ClickHouse ReplacingMergeTree deduplicates on event_id and
		// Kafka consumers are idempotent (keyed by EventID).
	}

	rw.totalReclaimed.Add(int64(len(events)))
	rw.logger.Info("reclamation: events recovered from DLQ",
		zap.Int("count", len(events)),
		zap.Int64("remaining", rw.dlqStore.Size()),
	)

	// If DLQ is fully drained, clear degraded state.
	if rw.dlqStore.Size() == 0 {
		rw.markHealthy()
	}
}
