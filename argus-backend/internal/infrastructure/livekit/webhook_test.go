package livekit

import (
	"context"
	"errors"
	"testing"

	livekitpb "github.com/livekit/protocol/livekit"
)

type recordingStoreSpy struct {
	egressID string
	status   string
	fileURL  string
	errText  string
	err      error
}

func (s *recordingStoreSpy) UpdateRecordingEnded(ctx context.Context, egressID, status, fileURL, errorMessage string) error {
	s.egressID = egressID
	s.status = status
	s.fileURL = fileURL
	s.errText = errorMessage
	return s.err
}

func TestHandleEgressEndedUpdatesRecordingByEgressID(t *testing.T) {
	store := &recordingStoreSpy{}
	info := &livekitpb.EgressInfo{
		EgressId: "egress-123",
		Status:   livekitpb.EgressStatus_EGRESS_COMPLETE,
		FileResults: []*livekitpb.FileInfo{
			{
				Filename: "content/recordings/room/session-student.mp4",
			},
		},
	}

	if err := handleEgressEnded(context.Background(), store, info); err != nil {
		t.Fatalf("handleEgressEnded returned error: %v", err)
	}

	if store.egressID != "egress-123" {
		t.Fatalf("egressID = %q, want egress-123", store.egressID)
	}
	if store.status != "EGRESS_COMPLETE" {
		t.Fatalf("status = %q, want EGRESS_COMPLETE", store.status)
	}
	if store.fileURL != "content/recordings/room/session-student.mp4" {
		t.Fatalf("fileURL = %q", store.fileURL)
	}
}

func TestHandleEgressEndedPropagatesStoreErrors(t *testing.T) {
	store := &recordingStoreSpy{err: errors.New("db down")}
	info := &livekitpb.EgressInfo{EgressId: "egress-123"}

	if err := handleEgressEnded(context.Background(), store, info); err == nil {
		t.Fatal("expected store error")
	}
}
