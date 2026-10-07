package entity

import "time"

// Recording tracks a LiveKit Egress recording and its persisted S3 output.
type Recording struct {
	EgressID     string
	SessionID    string
	UserID       string
	RoomName     string
	VideoTrackID string
	AudioTrackID string
	Status       string
	FileURL      string
	ErrorMessage string
	StartedAt    time.Time
	EndedAt      *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
