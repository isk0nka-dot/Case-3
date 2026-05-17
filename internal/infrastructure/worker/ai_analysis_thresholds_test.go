package worker

import (
	"testing"

	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
)

func TestClassifyFrameAnomaliesUsesConfigurableThresholds(t *testing.T) {
	frame := &inferencepb.FrameAnalysis{
		Faces: []*inferencepb.FaceDetection{
			{Similarity: 0.59, Confidence: 0.8},
			{IsSpoof: true, SpoofType: "deepfake", Confidence: 0.65},
		},
		Objects: []*inferencepb.ObjectDetection{
			{ObjectType: "phone", Confidence: 0.69},
			{ObjectType: "book", Confidence: 0.91},
		},
		Liveness: &inferencepb.LivenessResult{IsLive: false, Score: 0.31, Method: "blink"},
	}

	anomalies := classifyFrameAnomalies(frame, AIAnalysisThresholds{
		FaceMismatch:     0.6,
		Liveness:         0.3,
		ObjectConfidence: 0.7,
		SpoofConfidence:  0.7,
	})

	if len(anomalies) != 2 {
		t.Fatalf("expected face mismatch and high-confidence object only, got %d anomalies: %#v", len(anomalies), anomalies)
	}
	if anomalies[0].eventType != valueobject.BackendAIFaceMismatch {
		t.Fatalf("expected face mismatch event, got %s", anomalies[0].eventType)
	}
	if anomalies[1].eventType != valueobject.BackendAIHiddenObject {
		t.Fatalf("expected hidden object event, got %s", anomalies[1].eventType)
	}
}

func TestClassifyFrameAnomaliesAllowsThresholdTuning(t *testing.T) {
	frame := &inferencepb.FrameAnalysis{
		Faces: []*inferencepb.FaceDetection{
			{Similarity: 0.59, Confidence: 0.8},
			{IsSpoof: true, SpoofType: "deepfake", Confidence: 0.65},
		},
		Objects: []*inferencepb.ObjectDetection{
			{ObjectType: "phone", Confidence: 0.69},
		},
		Liveness: &inferencepb.LivenessResult{IsLive: false, Score: 0.31, Method: "blink"},
	}

	anomalies := classifyFrameAnomalies(frame, AIAnalysisThresholds{
		FaceMismatch:     0.55,
		Liveness:         0.4,
		ObjectConfidence: 0.6,
		SpoofConfidence:  0.6,
	})

	if len(anomalies) != 3 {
		t.Fatalf("expected tuned thresholds to emit spoof, object, and liveness, got %d anomalies: %#v", len(anomalies), anomalies)
	}
}
