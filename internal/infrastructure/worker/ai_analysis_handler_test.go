package worker

import (
	"context"
	"errors"
	"testing"
)

func TestAIAnalysisEvidenceContentTypeClassification(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		supported   bool
	}{
		{name: "jpeg", contentType: "image/jpeg", supported: true},
		{name: "jpeg with parameters", contentType: "image/jpeg; charset=binary", supported: true},
		{name: "png", contentType: "image/png", supported: true},
		{name: "webp", contentType: "image/webp", supported: true},
		{name: "mp4", contentType: "video/mp4", supported: false},
		{name: "webm", contentType: "video/webm", supported: false},
		{name: "empty", contentType: "", supported: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedEvidenceFrameContentType(tt.contentType); got != tt.supported {
				t.Fatalf("isSupportedEvidenceFrameContentType(%q)=%v, want %v", tt.contentType, got, tt.supported)
			}
		})
	}
}

func TestAIAnalysisRejectsRawVideoBeforeStorageDownload(t *testing.T) {
	handler := &AIAnalysisHandler{}

	_, err := handler.analyzeFragment(
		context.Background(),
		nil,
		AIAnalysisPayload{SessionID: "session-1", OrgID: "org-1"},
		evidenceFragment{ObjectKey: "recordings/session-1.webm", ContentType: "video/webm"},
	)

	if !errors.Is(err, errUnsupportedEvidenceContentType) {
		t.Fatalf("expected unsupported content type error, got %v", err)
	}
}
