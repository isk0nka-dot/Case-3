package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"go.uber.org/zap"
)

type fakeEgressController struct {
	startRoom    string
	startVideoID string
	startAudioID string
	startUserID  string
	startSession string
	stopEgressID string
	startErr     error
	stopErr      error
	returnEgress string
}

func (f *fakeEgressController) StartRecording(ctx context.Context, roomName, videoTrackID, audioTrackID, userID, sessionID string) (string, error) {
	f.startRoom = roomName
	f.startVideoID = videoTrackID
	f.startAudioID = audioTrackID
	f.startUserID = userID
	f.startSession = sessionID
	if f.startErr != nil {
		return "", f.startErr
	}
	return f.returnEgress, nil
}

func (f *fakeEgressController) StopRecording(ctx context.Context, egressID string) error {
	f.stopEgressID = egressID
	return f.stopErr
}

type fakeRecordingStore struct {
	created *entity.Recording
	err     error
}

func (f *fakeRecordingStore) CreateRecording(ctx context.Context, rec *entity.Recording) error {
	copy := *rec
	f.created = &copy
	return f.err
}

func (f *fakeRecordingStore) UpdateRecordingStopped(ctx context.Context, egressID string) error {
	return nil
}

func TestRecordingHandlerStartCreatesRecordingWithEgressID(t *testing.T) {
	egress := &fakeEgressController{returnEgress: "egress-123"}
	store := &fakeRecordingStore{}
	handler := NewRecordingHandler(egress, store, zap.NewNop(), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/recordings/start", strings.NewReader(`{
		"sessionId": "session-1",
		"studentId": "student-1",
		"videoTrackId": "video-track",
		"audioTrackId": "audio-track"
	}`))
	rec := httptest.NewRecorder()

	handler.handleStartRecording(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if egress.startRoom != "argus-session-session-1" {
		t.Fatalf("room = %q", egress.startRoom)
	}
	if store.created == nil {
		t.Fatal("recording was not stored")
	}
	if store.created.EgressID != "egress-123" || store.created.SessionID != "session-1" || store.created.UserID != "student-1" {
		t.Fatalf("stored recording = %+v", store.created)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["egressId"] != "egress-123" {
		t.Fatalf("egressId response = %q", body["egressId"])
	}
}

func TestRecordingHandlerStartStopsEgressWhenStoreFails(t *testing.T) {
	egress := &fakeEgressController{returnEgress: "egress-123"}
	store := &fakeRecordingStore{err: errors.New("db down")}
	handler := NewRecordingHandler(egress, store, zap.NewNop(), nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/media/recordings/start", strings.NewReader(`{
		"sessionId": "session-1",
		"studentId": "student-1",
		"videoTrackId": "video-track"
	}`))
	rec := httptest.NewRecorder()

	handler.handleStartRecording(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if egress.stopEgressID != "egress-123" {
		t.Fatalf("StopRecording called with %q, want egress-123", egress.stopEgressID)
	}
}
