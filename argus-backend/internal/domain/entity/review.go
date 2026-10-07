// Package entity defines the core domain entities for the Argus AI platform.
//
// ReviewDecision represents a mandatory human review of a proctoring session.
// Every session with violations requires a proctor or org_admin to explicitly
// confirm, dismiss, or escalate the findings. No automated verdicts are issued.
package entity

import "time"

// ReviewDecision is the result of a mandatory human review of a proctoring session.
// This is the final arbiter of session status — the integrity score is advisory,
// but the review decision determines the official outcome.
//
// Decisions:
//   - confirmed: violations are confirmed, academic consequences apply.
//   - dismissed: violations are false positives, session is cleared.
//   - escalated: requires senior review (org_admin or ethics committee).
type ReviewDecision struct {
	// Unique session identifier (one review per session).
	SessionID string `json:"sessionId"`

	// Who performed the review.
	ReviewerID   string `json:"reviewerId"`
	ReviewerName string `json:"reviewerName"`

	// Which organization this review belongs to.
	OrgID string `json:"orgId"`

	// The review outcome.
	Decision string `json:"decision"` // confirmed | dismissed | escalated

	// Free-text notes from the reviewer explaining their decision.
	Notes string `json:"notes"`

	// Evidence fragment IDs that were reviewed as part of this decision.
	EvidenceIDs []string `json:"evidenceIds"`

	// The integrity score at the time of review (for historical reference).
	IntegrityScore float64 `json:"integrityScore"`

	// When the review was performed.
	ReviewedAt time.Time `json:"reviewedAt"`
}
