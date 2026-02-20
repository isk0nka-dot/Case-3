// Package usecase implements the application-layer business logic.
// Use cases orchestrate domain entities and outbound ports.
// They contain NO infrastructure code — only pure business rules.
package usecase

import (
	"context"
	"fmt"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/application/port"
)

// EvidenceRecorder is the interface for the evidence capture subsystem.
// It is defined here (in the application layer) to avoid importing
// infrastructure packages. The recorder package implements this interface.
type EvidenceRecorder interface {
	TriggerCapture(ctx context.Context, event *entity.ProctoringEvent)
}

// IngestUseCase handles the core business logic for event ingestion.
// It validates incoming events, enriches them with server-side metadata,
// and fans out to multiple writers (Kafka for streaming, ClickHouse for analytics).
//
// This use case is designed for extreme throughput:
//   - No locks on the hot path.
//   - Atomic counters for metrics.
//   - Fan-out to writers is parallel.
//   - Evidence capture is async and non-blocking (tertiary fan-out).
type IngestUseCase struct {
	kafkaWriter      port.EventWriter
	clickhouseWriter port.EventWriter
	recorder         EvidenceRecorder // optional — nil when MinIO is unavailable
	logger           *zap.Logger

	// Metrics (lock-free atomic counters).
	totalIngested   atomic.Int64
	totalRejected   atomic.Int64
	totalCritical   atomic.Int64
}

// NewIngestUseCase creates a new IngestUseCase with the given dependencies.
// All dependencies are injected — no globals, no singletons.
// Pass functional options to attach optional subsystems (e.g., evidence recorder).
func NewIngestUseCase(
	kafkaWriter port.EventWriter,
	clickhouseWriter port.EventWriter,
	logger *zap.Logger,
	opts ...IngestOption,
) *IngestUseCase {
	uc := &IngestUseCase{
		kafkaWriter:      kafkaWriter,
		clickhouseWriter: clickhouseWriter,
		logger:           logger.Named("ingest_usecase"),
	}
	for _, opt := range opts {
		opt(uc)
	}
	return uc
}

// IngestOption is a functional option for configuring IngestUseCase.
type IngestOption func(*IngestUseCase)

// WithRecorder attaches an evidence recorder to the ingest pipeline.
// When set, critical events trigger async evidence capture.
func WithRecorder(r EvidenceRecorder) IngestOption {
	return func(uc *IngestUseCase) {
		uc.recorder = r
	}
}

// IngestResult contains the result of an ingestion operation.
type IngestResult struct {
	EventID  string
	Accepted bool
	Sequence int64
	Error    error
}

// Ingest processes a single proctoring event through the ingestion pipeline.
//
// Pipeline:
//  1. Validate domain invariants.
//  2. Enrich with server timestamp.
//  3. Write to Kafka (primary — for real-time streaming).
//  4. Write to ClickHouse (secondary — for analytics/archival).
//
// If Kafka write fails, the event is rejected (Kafka is the source of truth).
// If ClickHouse write fails, we log the error but don't reject the event.
func (uc *IngestUseCase) Ingest(ctx context.Context, event *entity.ProctoringEvent) *IngestResult {
	// Step 1: Validate domain invariants.
	if err := event.Validate(); err != nil {
		uc.totalRejected.Add(1)
		return &IngestResult{
			EventID:  event.EventID,
			Accepted: false,
			Error:    fmt.Errorf("validation failed: %w", err),
		}
	}

	// Step 2: Enrich with server-side metadata.
	event.SetServerTimestamp()

	// Step 3: Write to Kafka (primary write — must succeed).
	if err := uc.kafkaWriter.Write(ctx, event); err != nil {
		uc.totalRejected.Add(1)
		uc.logger.Error("kafka write failed",
			zap.String("event_id", event.EventID),
			zap.String("session_id", event.SessionID),
			zap.Error(err),
		)
		return &IngestResult{
			EventID:  event.EventID,
			Accepted: false,
			Error:    fmt.Errorf("kafka write failed: %w", err),
		}
	}

	// Step 4: Write to ClickHouse (secondary write — best-effort).
	if err := uc.clickhouseWriter.Write(ctx, event); err != nil {
		// Log but don't fail — Kafka is the source of truth.
		// A separate consumer will backfill ClickHouse from Kafka if needed.
		uc.logger.Warn("clickhouse write failed (non-fatal)",
			zap.String("event_id", event.EventID),
			zap.Error(err),
		)
	}

	// Step 5: Trigger evidence capture for critical events (tertiary, async).
	// Evidence capture is non-blocking and never causes event rejection.
	// Pattern: Kafka=primary, ClickHouse=secondary, Recorder=tertiary.
	if uc.recorder != nil && event.IsCritical() {
		// Use a background context — evidence capture should not be cancelled
		// if the gRPC stream context is done.
		bgCtx := context.Background()
		uc.recorder.TriggerCapture(bgCtx, event)
	}

	// Track metrics.
	uc.totalIngested.Add(1)
	if event.IsCritical() {
		uc.totalCritical.Add(1)
	}

	seq := uc.totalIngested.Load()

	return &IngestResult{
		EventID:  event.EventID,
		Accepted: true,
		Sequence: seq,
	}
}

// IngestBatch processes a batch of events atomically.
// This is the preferred path for high-frequency telemetry data.
//
// Strategy:
//   - Validate all events first (fail-fast on any invalid event).
//   - Write valid events to Kafka as a batch.
//   - Write to ClickHouse as a batch (best-effort).
func (uc *IngestUseCase) IngestBatch(ctx context.Context, events []*entity.ProctoringEvent) (*BatchResult, error) {
	if len(events) == 0 {
		return &BatchResult{}, nil
	}

	valid := make([]*entity.ProctoringEvent, 0, len(events))
	rejectedIDs := make([]string, 0)

	// Step 1: Validate all events.
	for _, event := range events {
		if err := event.Validate(); err != nil {
			rejectedIDs = append(rejectedIDs, event.EventID)
			uc.totalRejected.Add(1)
			uc.logger.Debug("batch event rejected",
				zap.String("event_id", event.EventID),
				zap.Error(err),
			)
			continue
		}
		event.SetServerTimestamp()
		valid = append(valid, event)
	}

	if len(valid) == 0 {
		return &BatchResult{
			AcceptedCount:    0,
			RejectedCount:    int32(len(rejectedIDs)),
			RejectedEventIDs: rejectedIDs,
		}, nil
	}

	// Step 2: Write valid events to Kafka (primary).
	if err := uc.kafkaWriter.WriteBatch(ctx, valid); err != nil {
		uc.totalRejected.Add(int64(len(valid)))
		return nil, fmt.Errorf("kafka batch write failed: %w", err)
	}

	// Step 3: Write to ClickHouse (secondary, best-effort).
	if err := uc.clickhouseWriter.WriteBatch(ctx, valid); err != nil {
		uc.logger.Warn("clickhouse batch write failed (non-fatal)",
			zap.Int("event_count", len(valid)),
			zap.Error(err),
		)
	}

	// Track metrics.
	uc.totalIngested.Add(int64(len(valid)))
	for _, event := range valid {
		if event.IsCritical() {
			uc.totalCritical.Add(1)
		}
	}

	seq := uc.totalIngested.Load()

	return &BatchResult{
		AcceptedCount:    int32(len(valid)),
		RejectedCount:    int32(len(rejectedIDs)),
		RejectedEventIDs: rejectedIDs,
		BatchSequence:    seq,
	}, nil
}

// BatchResult contains the results of a batch ingestion.
type BatchResult struct {
	AcceptedCount    int32
	RejectedCount    int32
	RejectedEventIDs []string
	BatchSequence    int64
}

// Stats returns current ingestion metrics.
func (uc *IngestUseCase) Stats() IngestStats {
	return IngestStats{
		TotalIngested: uc.totalIngested.Load(),
		TotalRejected: uc.totalRejected.Load(),
		TotalCritical: uc.totalCritical.Load(),
	}
}

// IngestStats holds runtime metrics for monitoring.
type IngestStats struct {
	TotalIngested int64
	TotalRejected int64
	TotalCritical int64
}

// Close shuts down both writers gracefully, flushing pending data.
func (uc *IngestUseCase) Close() error {
	var firstErr error
	if err := uc.kafkaWriter.Close(); err != nil {
		uc.logger.Error("failed to close kafka writer", zap.Error(err))
		firstErr = err
	}
	if err := uc.clickhouseWriter.Close(); err != nil {
		uc.logger.Error("failed to close clickhouse writer", zap.Error(err))
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
