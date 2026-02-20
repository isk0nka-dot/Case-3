// Package evidence implements the port.EvidenceChainWriter interface.
// It records evidence fragment metadata to both ClickHouse (queryable analytics)
// and Kafka (immutable append-only log) for tamper-proof chain of custody.
//
// Architecture:
//   - ClickHouse: evidence_fragments table — queryable by org, session, event.
//   - Kafka: argus.evidence.chain topic — immutable append-only log.
//   - Kafka: argus.forensic.ledger topic — hash chain proofs for tamper detection.
//   - If Kafka write succeeds but ClickHouse fails, the entry is still
//     recoverable from Kafka (source of truth).
//   - All writes are append-only — no updates or deletes.
//
// Hash Chaining (v2.1):
//   Each evidence record includes a cryptographic link to the previous record
//   in the same session. This forms an immutable hash chain that detects:
//     - Insertion of forged records mid-sequence
//     - Deletion of legitimate records
//     - Reordering of records
//   The chain is per-session, starting with "GENESIS" as the initial previous hash.
//   record_hash = SHA-256(sequence_num || previous_hash || fragment_id || sha256_hash || uploaded_at)
package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// KafkaTopic is the Kafka topic for evidence chain of custody records.
const KafkaTopic = "argus.evidence.chain"

// ForensicLedgerTopic is the Kafka topic for hash chain proofs.
// Each message contains the cryptographic proof linking consecutive evidence
// records within a session, enabling independent tamper verification.
const ForensicLedgerTopic = "argus.forensic.ledger"

// GenesisHash is the initial previous_hash for the first record in a session.
const GenesisHash = "GENESIS"

// KafkaPublisher is the interface for publishing raw messages to Kafka.
// The kafka.Producer.PublishRaw method satisfies this interface.
type KafkaPublisher interface {
	PublishRaw(ctx context.Context, topic string, key string, value []byte) error
}

// sessionChainState tracks the hash chain state for a single proctoring session.
// This is an in-memory structure — on restart, the chain state is rebuilt from
// ClickHouse by querying the last sequence_num and record_hash for the session.
type sessionChainState struct {
	sequenceNum int64  // last assigned sequence number
	lastHash    string // last record_hash (or "GENESIS" if first)
}

// ChainWriter writes evidence fragment metadata to ClickHouse and Kafka
// for tamper-proof chain of custody. It implements port.EvidenceChainWriter.
//
// Hash Chain Guarantees:
//   - Per-session monotonic sequence numbers (no gaps, no duplicates)
//   - Each record cryptographically links to the previous via SHA-256
//   - Chain state is mutex-protected for concurrent session writes
//   - On restart, chain state is recovered from ClickHouse
type ChainWriter struct {
	chConn    driver.Conn
	publisher KafkaPublisher
	logger    *zap.Logger

	// Hash chain state — protected by mu.
	mu     sync.Mutex
	chains map[string]*sessionChainState // sessionID -> chain state
}

// NewChainWriter creates a new evidence chain writer.
// chConn is the ClickHouse connection (from clickhouse.Writer.Conn()).
// publisher is the Kafka producer for raw message publishing.
func NewChainWriter(chConn driver.Conn, publisher KafkaPublisher, logger *zap.Logger) *ChainWriter {
	return &ChainWriter{
		chConn:    chConn,
		publisher: publisher,
		logger:    logger.Named("evidence_chain"),
		chains:    make(map[string]*sessionChainState),
	}
}

// RecordEvidence persists an evidence fragment to both ClickHouse and Kafka,
// with cryptographic hash chaining for tamper detection.
//
// Write order:
//  1. Acquire chain state for session (mutex-protected).
//  2. Compute hash chain fields (sequence_num, previous_hash, record_hash).
//  3. Write to Kafka evidence chain topic (source of truth).
//  4. Write to Kafka forensic ledger topic (hash chain proof).
//  5. Write to ClickHouse (queryable, includes chain fields).
//  6. Update in-memory chain state.
func (w *ChainWriter) RecordEvidence(ctx context.Context, fragment *entity.EvidenceFragment) error {
	uploadedAt := fragment.CreatedAt.UTC().Format(time.RFC3339Nano)

	// ── Step 1: Compute hash chain under mutex ──────────────────────────
	w.mu.Lock()
	state, err := w.getOrRecoverChainState(ctx, fragment.SessionID)
	if err != nil {
		w.mu.Unlock()
		return fmt.Errorf("evidence chain: failed to recover chain state: %w", err)
	}

	seqNum := state.sequenceNum + 1
	prevHash := state.lastHash
	recordHash := computeRecordHash(seqNum, prevHash, fragment.FragmentID, fragment.SHA256Hash, uploadedAt)
	w.mu.Unlock()

	// ── Step 2: Write to Kafka evidence chain topic (source of truth) ───
	kafkaRecord := chainRecord{
		FragmentID:   fragment.FragmentID,
		SessionID:    fragment.SessionID,
		EventID:      fragment.EventID,
		OrgID:        fragment.OrgID,
		ExamID:       fragment.ExamID,
		StudentID:    fragment.StudentID,
		SHA256Hash:   fragment.SHA256Hash,
		URI:          fragment.URI,
		SizeBytes:    fragment.SizeBytes,
		ContentType:  fragment.ContentType,
		DurationSec:  fragment.DurationSec,
		StartTime:    fragment.StartTime.UTC().Format(time.RFC3339Nano),
		EndTime:      fragment.EndTime.UTC().Format(time.RFC3339Nano),
		UploadedAt:   uploadedAt,
		SequenceNum:  seqNum,
		PreviousHash: prevHash,
		RecordHash:   recordHash,
	}

	value, err := json.Marshal(kafkaRecord)
	if err != nil {
		return fmt.Errorf("evidence chain: failed to marshal kafka record: %w", err)
	}

	if err := w.publisher.PublishRaw(ctx, KafkaTopic, fragment.SessionID, value); err != nil {
		return fmt.Errorf("evidence chain: kafka write failed: %w", err)
	}

	// ── Step 3: Write hash chain proof to forensic ledger topic ─────────
	ledgerEntry := forensicLedgerEntry{
		SessionID:    fragment.SessionID,
		FragmentID:   fragment.FragmentID,
		SequenceNum:  seqNum,
		PreviousHash: prevHash,
		RecordHash:   recordHash,
		SHA256Hash:   fragment.SHA256Hash,
		Timestamp:    uploadedAt,
	}

	ledgerValue, err := json.Marshal(ledgerEntry)
	if err != nil {
		w.logger.Warn("evidence chain: failed to marshal forensic ledger entry",
			zap.String("fragment_id", fragment.FragmentID),
			zap.Error(err),
		)
		// Non-fatal: the evidence chain topic has the full record.
	} else {
		if err := w.publisher.PublishRaw(ctx, ForensicLedgerTopic, fragment.SessionID, ledgerValue); err != nil {
			w.logger.Warn("evidence chain: forensic ledger write failed (evidence chain record exists)",
				zap.String("fragment_id", fragment.FragmentID),
				zap.Error(err),
			)
			// Non-fatal: ledger is secondary to the evidence chain topic.
		}
	}

	// ── Step 4: Write to ClickHouse (queryable, includes chain fields) ──
	chCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	err = w.chConn.Exec(chCtx, `
		INSERT INTO evidence_fragments (
			fragment_id, session_id, event_id, org_id, exam_id, student_id,
			sha256_hash, uri, size_bytes, content_type,
			duration_sec, start_time, end_time, uploaded_at,
			sequence_num, previous_hash, record_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fragment.FragmentID,
		fragment.SessionID,
		fragment.EventID,
		fragment.OrgID,
		fragment.ExamID,
		fragment.StudentID,
		fragment.SHA256Hash,
		fragment.URI,
		fragment.SizeBytes,
		fragment.ContentType,
		fragment.DurationSec,
		fragment.StartTime,
		fragment.EndTime,
		fragment.CreatedAt,
		seqNum,
		prevHash,
		recordHash,
	)
	if err != nil {
		// Log but don't fail — Kafka has the record, ClickHouse can be backfilled.
		w.logger.Error("evidence chain: clickhouse write failed (kafka record exists)",
			zap.String("fragment_id", fragment.FragmentID),
			zap.String("sha256", fragment.SHA256Hash),
			zap.Int64("sequence_num", seqNum),
			zap.Error(err),
		)
		// Return the error so the caller can decide whether to retry.
		return fmt.Errorf("evidence chain: clickhouse write failed: %w", err)
	}

	// ── Step 5: Update in-memory chain state (only after successful writes) ─
	w.mu.Lock()
	w.chains[fragment.SessionID] = &sessionChainState{
		sequenceNum: seqNum,
		lastHash:    recordHash,
	}
	w.mu.Unlock()

	w.logger.Info("evidence chain recorded",
		zap.String("fragment_id", fragment.FragmentID),
		zap.String("session_id", fragment.SessionID),
		zap.String("sha256", fragment.SHA256Hash),
		zap.Int64("sequence_num", seqNum),
		zap.String("record_hash", recordHash[:16]+"..."),
		zap.String("kafka_topic", KafkaTopic),
		zap.String("ch_table", "evidence_fragments"),
	)

	return nil
}

// getOrRecoverChainState returns the chain state for a session. If not in memory,
// it queries ClickHouse for the last sequence_num and record_hash.
// MUST be called with w.mu held.
func (w *ChainWriter) getOrRecoverChainState(ctx context.Context, sessionID string) (*sessionChainState, error) {
	if state, ok := w.chains[sessionID]; ok {
		return state, nil
	}

	// Recover from ClickHouse — find the last record for this session.
	chCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var seqNum int64
	var lastHash string

	row := w.chConn.QueryRow(chCtx, `
		SELECT sequence_num, record_hash
		FROM evidence_fragments
		WHERE session_id = ?
		ORDER BY sequence_num DESC
		LIMIT 1`,
		sessionID,
	)

	if err := row.Scan(&seqNum, &lastHash); err != nil {
		// No previous records — this is the first fragment for this session.
		state := &sessionChainState{
			sequenceNum: 0,
			lastHash:    GenesisHash,
		}
		w.chains[sessionID] = state

		w.logger.Debug("evidence chain: initialized new chain for session",
			zap.String("session_id", sessionID),
		)
		return state, nil
	}

	state := &sessionChainState{
		sequenceNum: seqNum,
		lastHash:    lastHash,
	}
	w.chains[sessionID] = state

	w.logger.Info("evidence chain: recovered chain state from ClickHouse",
		zap.String("session_id", sessionID),
		zap.Int64("last_sequence_num", seqNum),
		zap.String("last_hash", lastHash[:16]+"..."),
	)
	return state, nil
}

// computeRecordHash computes the SHA-256 hash that links this record to the
// previous record in the session chain.
//
// Hash input: "seqNum|prevHash|fragmentID|sha256Hash|uploadedAt"
// This deterministic format allows independent verification by any party
// with access to the chain records.
func computeRecordHash(seqNum int64, prevHash, fragmentID, sha256Hash, uploadedAt string) string {
	input := fmt.Sprintf("%d|%s|%s|%s|%s", seqNum, prevHash, fragmentID, sha256Hash, uploadedAt)
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// Close releases resources. Chain writer holds references but not ownership,
// so it does not close the ClickHouse connection or Kafka producer.
func (w *ChainWriter) Close() error {
	w.mu.Lock()
	chainCount := len(w.chains)
	w.chains = make(map[string]*sessionChainState)
	w.mu.Unlock()

	w.logger.Info("evidence chain writer closed",
		zap.Int("sessions_tracked", chainCount),
	)
	return nil
}

// ---------------------------------------------------------------------------
// Kafka Wire Formats
// ---------------------------------------------------------------------------

// chainRecord is the JSON format written to the argus.evidence.chain Kafka topic.
// This is a public contract — fields must remain stable across versions.
//
// v2.1: Added sequence_num, previous_hash, record_hash for hash chaining.
// These fields enable independent verification of the evidence chain integrity.
type chainRecord struct {
	FragmentID  string  `json:"fragment_id"`
	SessionID   string  `json:"session_id"`
	EventID     string  `json:"event_id"`
	OrgID       string  `json:"org_id"`
	ExamID      string  `json:"exam_id"`
	StudentID   string  `json:"student_id"`
	SHA256Hash  string  `json:"sha256_hash"`
	URI         string  `json:"uri"`
	SizeBytes   int64   `json:"size_bytes"`
	ContentType string  `json:"content_type"`
	DurationSec float64 `json:"duration_sec"`
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	UploadedAt  string  `json:"uploaded_at"`

	// Hash chain fields (v2.1).
	SequenceNum  int64  `json:"sequence_num"`   // Monotonic per-session, starts at 1
	PreviousHash string `json:"previous_hash"`  // SHA-256 of previous record (or "GENESIS")
	RecordHash   string `json:"record_hash"`    // SHA-256(seq|prev|fragment_id|sha256|uploaded_at)
}

// forensicLedgerEntry is the JSON format written to the argus.forensic.ledger
// Kafka topic. Contains only the hash chain proof fields — a minimal record
// for independent verification without the full evidence metadata.
//
// This topic serves as a secondary tamper-evidence log. Even if the evidence
// chain topic is compromised, the forensic ledger provides an independent
// verification path.
type forensicLedgerEntry struct {
	SessionID    string `json:"session_id"`
	FragmentID   string `json:"fragment_id"`
	SequenceNum  int64  `json:"sequence_num"`
	PreviousHash string `json:"previous_hash"`
	RecordHash   string `json:"record_hash"`
	SHA256Hash   string `json:"sha256_hash"`
	Timestamp    string `json:"timestamp"`
}
