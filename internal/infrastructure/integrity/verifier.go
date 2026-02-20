// Package integrity implements the port.IntegrityVerifier interface.
// It cross-references S3 evidence objects against the ClickHouse forensic
// ledger and validates the cryptographic hash chain for tamper detection.
//
// Verification flow:
//  1. Query ClickHouse for all evidence fragments in a session, ordered by sequence_num.
//  2. For each fragment: call EvidenceStore.VerifyIntegrity() to re-download and verify SHA-256.
//  3. Recompute the hash chain: SHA-256(seq|prev_hash|fragment_id|sha256|uploaded_at).
//  4. Compare computed chain hashes against stored record_hash values.
//  5. Return a comprehensive IntegrityReport.
package integrity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
)

// evidenceRow represents a single evidence fragment record from ClickHouse,
// including the hash chain fields added in v2.1.
type evidenceRow struct {
	FragmentID   string
	SessionID    string
	SHA256Hash   string
	URI          string
	SequenceNum  int64
	PreviousHash string
	RecordHash   string
	UploadedAt   time.Time
}

// Verifier implements port.IntegrityVerifier using ClickHouse for ledger
// queries and EvidenceStore for S3 object verification.
type Verifier struct {
	chConn driver.Conn
	store  port.EvidenceStore
	logger *zap.Logger
}

// NewVerifier creates a new integrity verifier.
func NewVerifier(chConn driver.Conn, store port.EvidenceStore, logger *zap.Logger) *Verifier {
	return &Verifier{
		chConn: chConn,
		store:  store,
		logger: logger.Named("integrity_verifier"),
	}
}

// VerifySession verifies all evidence fragments and the hash chain for a session.
func (v *Verifier) VerifySession(ctx context.Context, sessionID string) (*port.IntegrityReport, error) {
	startTime := time.Now()

	// ── Step 1: Query all fragments ordered by sequence_num ─────────────
	rows, err := v.querySessionFragments(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("integrity: failed to query session fragments: %w", err)
	}

	report := &port.IntegrityReport{
		SessionID:      sessionID,
		TotalFragments: len(rows),
		ChainValid:     true,
		VerifiedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if len(rows) == 0 {
		report.DurationMs = time.Since(startTime).Milliseconds()
		return report, nil
	}

	// ── Step 2: Verify each fragment ────────────────────────────────────
	expectedPrevHash := "GENESIS"

	for i, row := range rows {
		result := port.FragmentIntegrityResult{
			FragmentID:   row.FragmentID,
			SequenceNum:  row.SequenceNum,
			PreviousHash: row.PreviousHash,
			RecordHash:   row.RecordHash,
			SHA256Match:  false,
			ChainValid:   true,
		}

		// ── 2a: Verify S3 object integrity ──────────────────────────────
		if row.URI != "" && row.SHA256Hash != "" {
			match, err := v.store.VerifyIntegrity(ctx, row.URI, row.SHA256Hash)
			if err != nil {
				result.ErrorMessage = fmt.Sprintf("S3 verification error: %v", err)
				report.S3Errors++
				v.logger.Warn("integrity: S3 verification failed",
					zap.String("fragment_id", row.FragmentID),
					zap.Error(err),
				)
			} else if match {
				result.SHA256Match = true
				report.S3Verified++
			} else {
				result.SHA256Match = false
				result.ErrorMessage = "S3 hash mismatch"
				report.S3Mismatches++
				report.Mismatches = append(report.Mismatches, result)
			}
		}

		// ── 2b: Verify hash chain linkage ───────────────────────────────
		// Check sequence_num continuity (must be i+1 for 1-based indexing).
		expectedSeq := int64(i + 1)
		if row.SequenceNum != expectedSeq && row.SequenceNum != 0 {
			// Allow sequence_num=0 for legacy pre-chain records.
			result.ChainValid = false
			result.ErrorMessage += fmt.Sprintf("; sequence gap: expected %d, got %d", expectedSeq, row.SequenceNum)
		}

		// Check previous_hash linkage.
		if row.PreviousHash != "" && row.PreviousHash != expectedPrevHash {
			result.ChainValid = false
			result.ErrorMessage += fmt.Sprintf("; previous_hash mismatch")
		}

		// Recompute record_hash and compare.
		if row.SequenceNum > 0 && row.RecordHash != "" {
			uploadedAtStr := row.UploadedAt.UTC().Format(time.RFC3339Nano)
			computed := computeRecordHash(row.SequenceNum, row.PreviousHash, row.FragmentID, row.SHA256Hash, uploadedAtStr)
			if computed != row.RecordHash {
				result.ChainValid = false
				result.ErrorMessage += fmt.Sprintf("; record_hash mismatch (computed=%s...)", computed[:16])
			}
		}

		// Update chain state for next iteration.
		if !result.ChainValid {
			report.ChainValid = false
			report.BrokenLinks = append(report.BrokenLinks, result)
		}

		if result.SHA256Match && result.ChainValid {
			report.VerifiedOK++
		}

		report.Fragments = append(report.Fragments, result)

		// Advance expected previous hash.
		if row.RecordHash != "" {
			expectedPrevHash = row.RecordHash
		}
	}

	report.DurationMs = time.Since(startTime).Milliseconds()

	v.logger.Info("integrity: session verification complete",
		zap.String("session_id", sessionID),
		zap.Int("total_fragments", report.TotalFragments),
		zap.Int("verified_ok", report.VerifiedOK),
		zap.Bool("chain_valid", report.ChainValid),
		zap.Int64("duration_ms", report.DurationMs),
	)

	return report, nil
}

// VerifyFragment verifies a single evidence fragment against S3.
func (v *Verifier) VerifyFragment(ctx context.Context, fragmentID string) (*port.FragmentIntegrityResult, error) {
	row, err := v.queryFragment(ctx, fragmentID)
	if err != nil {
		return nil, fmt.Errorf("integrity: failed to query fragment: %w", err)
	}

	result := &port.FragmentIntegrityResult{
		FragmentID:   row.FragmentID,
		SequenceNum:  row.SequenceNum,
		PreviousHash: row.PreviousHash,
		RecordHash:   row.RecordHash,
	}

	// Verify S3 object integrity.
	if row.URI != "" && row.SHA256Hash != "" {
		match, err := v.store.VerifyIntegrity(ctx, row.URI, row.SHA256Hash)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("S3 verification error: %v", err)
		} else {
			result.SHA256Match = match
			if !match {
				result.ErrorMessage = "S3 hash mismatch"
			}
		}
	}

	// Verify record hash computation.
	if row.SequenceNum > 0 && row.RecordHash != "" {
		uploadedAtStr := row.UploadedAt.UTC().Format(time.RFC3339Nano)
		computed := computeRecordHash(row.SequenceNum, row.PreviousHash, row.FragmentID, row.SHA256Hash, uploadedAtStr)
		result.ChainValid = (computed == row.RecordHash)
		if !result.ChainValid {
			result.ErrorMessage += fmt.Sprintf("; record_hash mismatch")
		}
	} else {
		// Legacy record without chain — mark as valid (no chain to verify).
		result.ChainValid = true
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// ClickHouse Queries
// ---------------------------------------------------------------------------

func (v *Verifier) querySessionFragments(ctx context.Context, sessionID string) ([]evidenceRow, error) {
	chCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	rows, err := v.chConn.Query(chCtx, `
		SELECT fragment_id, session_id, sha256_hash, uri,
		       sequence_num, previous_hash, record_hash, uploaded_at
		FROM evidence_fragments
		WHERE session_id = ?
		ORDER BY sequence_num ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []evidenceRow
	for rows.Next() {
		var r evidenceRow
		if err := rows.Scan(
			&r.FragmentID, &r.SessionID, &r.SHA256Hash, &r.URI,
			&r.SequenceNum, &r.PreviousHash, &r.RecordHash, &r.UploadedAt,
		); err != nil {
			return nil, fmt.Errorf("scan error: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (v *Verifier) queryFragment(ctx context.Context, fragmentID string) (*evidenceRow, error) {
	chCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var r evidenceRow
	row := v.chConn.QueryRow(chCtx, `
		SELECT fragment_id, session_id, sha256_hash, uri,
		       sequence_num, previous_hash, record_hash, uploaded_at
		FROM evidence_fragments
		WHERE fragment_id = ?
		LIMIT 1`,
		fragmentID,
	)

	if err := row.Scan(
		&r.FragmentID, &r.SessionID, &r.SHA256Hash, &r.URI,
		&r.SequenceNum, &r.PreviousHash, &r.RecordHash, &r.UploadedAt,
	); err != nil {
		return nil, fmt.Errorf("fragment not found: %w", err)
	}
	return &r, nil
}

// ---------------------------------------------------------------------------
// Hash Chain Computation
// ---------------------------------------------------------------------------

// computeRecordHash recomputes the hash chain record hash.
// Must match the formula in chain_writer.go exactly.
func computeRecordHash(seqNum int64, prevHash, fragmentID, sha256Hash, uploadedAt string) string {
	input := fmt.Sprintf("%d|%s|%s|%s|%s", seqNum, prevHash, fragmentID, sha256Hash, uploadedAt)
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}
