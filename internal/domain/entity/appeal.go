// Package entity defines core domain entities for the Argus AI platform.
//
// Appeal represents a student's appeal of a proctoring violation decision.
// Implements a simple state machine: submitted -> under_review -> upheld|overturned|withdrawn.
//
// State transitions:
//   submitted     -> under_review  (admin starts review)
//   under_review  -> upheld        (admin confirms original decision)
//   under_review  -> overturned    (admin reverses original decision)
//   submitted     -> withdrawn     (student withdraws appeal)
//   under_review  -> withdrawn     (student withdraws appeal)
package entity

import (
	"fmt"
	"time"
)

// AppealStatus represents the lifecycle state of an appeal.
type AppealStatus string

const (
	AppealStatusSubmitted   AppealStatus = "submitted"
	AppealStatusUnderReview AppealStatus = "under_review"
	AppealStatusUpheld      AppealStatus = "upheld"
	AppealStatusOverturned  AppealStatus = "overturned"
	AppealStatusWithdrawn   AppealStatus = "withdrawn"
)

// Appeal is a student's challenge to a proctoring violation decision.
type Appeal struct {
	// ID is a unique identifier for this appeal (UUIDv7).
	ID string `json:"id"`

	// SessionID links this appeal to the proctoring session.
	SessionID string `json:"sessionId"`

	// StudentID identifies the student who filed the appeal.
	StudentID string `json:"studentId"`

	// OrgID is the organization that owns the exam.
	OrgID string `json:"orgId"`

	// ExamID is the exam being appealed.
	ExamID string `json:"examId"`

	// Reason is the student's stated reason for the appeal.
	Reason string `json:"reason"`

	// Status tracks the appeal lifecycle.
	Status AppealStatus `json:"status"`

	// ReviewedBy is the admin user ID who reviewed the appeal.
	ReviewedBy string `json:"reviewedBy,omitempty"`

	// ReviewNotes contains the admin's review comments.
	ReviewNotes string `json:"reviewNotes,omitempty"`

	// ReviewedAt is when the appeal was reviewed.
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`

	// CreatedAt is when the appeal was submitted.
	CreatedAt time.Time `json:"createdAt"`

	// UpdatedAt is when the appeal was last modified.
	UpdatedAt time.Time `json:"updatedAt"`
}

// ValidateTransition checks if a status transition is allowed.
func (a *Appeal) ValidateTransition(newStatus AppealStatus) error {
	switch a.Status {
	case AppealStatusSubmitted:
		if newStatus == AppealStatusUnderReview || newStatus == AppealStatusWithdrawn {
			return nil
		}
	case AppealStatusUnderReview:
		if newStatus == AppealStatusUpheld || newStatus == AppealStatusOverturned || newStatus == AppealStatusWithdrawn {
			return nil
		}
	default:
		// Terminal states — no transitions allowed.
	}
	return fmt.Errorf("appeal: invalid transition from %s to %s", a.Status, newStatus)
}
