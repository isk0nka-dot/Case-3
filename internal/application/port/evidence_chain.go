// Package port defines the interfaces (ports) that the application layer
// requires from infrastructure. This file defines the EvidenceChainWriter port
// for recording tamper-proof chain of custody entries.
package port

import (
	"context"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// EvidenceChainWriter is the outbound port for recording evidence chain of
// custody entries. Implementations write to both ClickHouse (queryable) and
// Kafka (immutable append-only log) for tamper-evidence.
//
// Contract:
//   - RecordEvidence must write to BOTH ClickHouse and Kafka atomically.
//   - If Kafka write succeeds but ClickHouse fails, the entry is still
//     recoverable from Kafka (source of truth).
//   - All writes are append-only — no updates or deletes.
type EvidenceChainWriter interface {
	// RecordEvidence persists an evidence fragment metadata record to the
	// chain of custody (ClickHouse + Kafka topic 'argus.evidence.chain').
	// The fragment must have a valid SHA256Hash before calling this method.
	RecordEvidence(ctx context.Context, fragment *entity.EvidenceFragment) error

	// Close flushes pending writes and releases resources.
	Close() error
}
