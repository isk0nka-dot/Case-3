// Package port defines the interfaces (ports) that the application layer
// requires from infrastructure. This file defines the IntegrityVerifier port
// for cross-referencing S3 evidence against the ClickHouse forensic ledger.
package port

import "context"

// FragmentIntegrityResult represents the verification result for a single
// evidence fragment. It contains the S3 hash match status and chain linkage.
type FragmentIntegrityResult struct {
	FragmentID   string `json:"fragmentId"`
	SequenceNum  int64  `json:"sequenceNum"`
	SHA256Match  bool   `json:"sha256Match"`  // S3 object hash matches ClickHouse record
	ChainValid   bool   `json:"chainValid"`   // record_hash is correctly computed from inputs
	PreviousHash string `json:"previousHash"` // expected previous_hash value
	RecordHash   string `json:"recordHash"`   // stored record_hash
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// IntegrityReport is the full verification report for a proctoring session.
type IntegrityReport struct {
	SessionID       string                    `json:"sessionId"`
	TotalFragments  int                       `json:"totalFragments"`
	VerifiedOK      int                       `json:"verifiedOk"`
	ChainValid      bool                      `json:"chainValid"`      // entire hash chain is unbroken
	S3Verified      int                       `json:"s3Verified"`      // fragments with matching S3 hash
	S3Mismatches    int                       `json:"s3Mismatches"`    // fragments with S3 hash mismatch
	S3Errors        int                       `json:"s3Errors"`        // fragments where S3 verification failed
	BrokenLinks     []FragmentIntegrityResult `json:"brokenLinks"`     // chain linkage failures
	Mismatches      []FragmentIntegrityResult `json:"mismatches"`      // S3 hash mismatches
	Fragments       []FragmentIntegrityResult `json:"fragments"`       // all fragment results
	VerifiedAt      string                    `json:"verifiedAt"`
	DurationMs      int64                     `json:"durationMs"`
}

// IntegrityVerifier is the outbound port for verifying the integrity of
// evidence stored in S3 against the forensic ledger in ClickHouse.
//
// Contract:
//   - VerifySession must check ALL fragments for a session:
//     1. S3 object integrity (re-download + SHA-256 comparison)
//     2. Hash chain continuity (recompute record_hash, verify linkage)
//   - VerifyFragment must check a single fragment's S3 integrity.
//   - All verification is read-only — no data is modified.
type IntegrityVerifier interface {
	// VerifySession verifies all evidence fragments and the hash chain
	// for the given session. Returns a comprehensive integrity report.
	VerifySession(ctx context.Context, sessionID string) (*IntegrityReport, error)

	// VerifyFragment verifies a single evidence fragment against S3.
	// Returns the fragment-level integrity result.
	VerifyFragment(ctx context.Context, fragmentID string) (*FragmentIntegrityResult, error)
}
