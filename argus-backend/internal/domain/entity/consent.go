// Package entity defines core domain entities for the Argus AI platform.
//
// ConsentRecord captures a student's explicit consent to proctoring terms
// before a session begins. Required for GDPR/privacy compliance.
//
// Consent covers:
//   - Video/audio capture during exam
//   - AI-based engagement risk signal analysis (non-diagnostic)
//   - Device attestation token collection
//   - Evidence storage with retention policy
package entity

import "time"

// ConsentRecord is an immutable record of a student's consent to proctoring.
// Once created, consent records are never modified (append-only audit trail).
type ConsentRecord struct {
	// ID is a unique identifier for this consent record (UUIDv7).
	ID string `json:"id"`

	// SessionID links this consent to the proctoring session.
	SessionID string `json:"sessionId"`

	// StudentID identifies the student who gave consent.
	StudentID string `json:"studentId"`

	// OrgID is the organization running the exam.
	OrgID string `json:"orgId"`

	// ExamID is the exam being proctored.
	ExamID string `json:"examId"`

	// ConsentVersion is the version of the consent terms shown to the student.
	// Format: "1.0", "2.0", etc. Allows tracking which terms were accepted.
	ConsentVersion string `json:"consentVersion"`

	// ConsentText is the full text of the consent terms shown to the student.
	// Stored for auditability — proves exactly what the student agreed to.
	ConsentText string `json:"consentText"`

	// Accepted indicates whether the student accepted (true) or declined (false).
	Accepted bool `json:"accepted"`

	// IPAddress is the client IP at the time of consent (set server-side).
	IPAddress string `json:"ipAddress"`

	// UserAgent is the browser user-agent at the time of consent.
	UserAgent string `json:"userAgent"`

	// CreatedAt is when the consent was recorded.
	CreatedAt time.Time `json:"createdAt"`
}
