package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
)

type captureWriter struct {
	mu     sync.Mutex
	events []*entity.ProctoringEvent
	err    error
}

func (w *captureWriter) Write(_ context.Context, event *entity.ProctoringEvent) error {
	if w.err != nil {
		return w.err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, event)
	return nil
}

func (w *captureWriter) WriteBatch(_ context.Context, events []*entity.ProctoringEvent) error {
	if w.err != nil {
		return w.err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, events...)
	return nil
}

func (w *captureWriter) Close() error { return nil }

func (w *captureWriter) snapshot() []*entity.ProctoringEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*entity.ProctoringEvent, len(w.events))
	copy(out, w.events)
	return out
}

func TestIngestDerivesSustainedAudioTelemetryAnomalyForClickHouseOnly(t *testing.T) {
	kafka := &captureWriter{}
	clickhouse := &captureWriter{}
	uc := NewIngestUseCase(
		kafka,
		clickhouse,
		zap.NewNop(),
		WithAudioBridge(AudioBridgeConfig{
			Enabled:                true,
			NoiseThresholdDb:       -35,
			VADConfidenceThreshold: 0.7,
			ConsecutiveEvents:      2,
			CooldownEvents:         8,
			DerivedEventConfidence: 0.85,
		}),
	)

	first := audioTelemetryEvent("evt-audio-1", -30, true, 0.91)
	second := audioTelemetryEvent("evt-audio-2", -29, true, 0.92)

	if result := uc.Ingest(context.Background(), first); !result.Accepted {
		t.Fatalf("first event rejected: %v", result.Error)
	}
	if result := uc.Ingest(context.Background(), second); !result.Accepted {
		t.Fatalf("second event rejected: %v", result.Error)
	}

	kafkaEvents := kafka.snapshot()
	if len(kafkaEvents) != 2 {
		t.Fatalf("kafka events = %d, want only original 2 telemetry events", len(kafkaEvents))
	}
	for _, event := range kafkaEvents {
		if event.EventType != valueobject.AudioLevelTelemetry {
			t.Fatalf("unexpected kafka event type %s; derived anomalies must stay ClickHouse-only", event.EventType)
		}
	}

	clickhouseEvents := clickhouse.snapshot()
	if len(clickhouseEvents) != 3 {
		t.Fatalf("clickhouse events = %d, want original 2 plus 1 derived anomaly", len(clickhouseEvents))
	}

	derived := clickhouseEvents[2]
	if derived.EventType != valueobject.AudioAnomaly {
		t.Fatalf("derived event type = %s, want audio_anomaly", derived.EventType)
	}
	if derived.Source != valueobject.SourceBackendAI {
		t.Fatalf("derived source = %s, want backend_ai", derived.Source)
	}
	if derived.SessionID != second.SessionID || derived.StudentID != second.StudentID || derived.ExamID != second.ExamID {
		t.Fatalf("derived event did not preserve session identity: %+v", derived)
	}
	if derived.AudioRmsDb != second.AudioRmsDb || !derived.VADActive || derived.AudioClassification != second.AudioClassification {
		t.Fatalf("derived event did not preserve denormalized audio fields: %+v", derived)
	}
	if derived.PayloadType != "audio_analysis" {
		t.Fatalf("derived payload type = %q, want audio_analysis", derived.PayloadType)
	}
}

func TestIngestAudioBridgeDoesNotDuplicateExplicitAudioAnomalies(t *testing.T) {
	kafka := &captureWriter{}
	clickhouse := &captureWriter{}
	uc := NewIngestUseCase(
		kafka,
		clickhouse,
		zap.NewNop(),
		WithAudioBridge(AudioBridgeConfig{
			Enabled:                true,
			NoiseThresholdDb:       -35,
			VADConfidenceThreshold: 0.7,
			ConsecutiveEvents:      1,
			CooldownEvents:         8,
		}),
	)

	event := audioTelemetryEvent("evt-explicit-audio-anomaly", -20, true, 0.95)
	event.EventType = valueobject.AudioAnomaly
	event.Severity = valueobject.SeverityWarning

	if result := uc.Ingest(context.Background(), event); !result.Accepted {
		t.Fatalf("event rejected: %v", result.Error)
	}

	if got := len(kafka.snapshot()); got != 1 {
		t.Fatalf("kafka events = %d, want 1", got)
	}
	if got := len(clickhouse.snapshot()); got != 1 {
		t.Fatalf("clickhouse events = %d, want 1 explicit event without derived duplicate", got)
	}
}

func TestIngestAcceptsAudioTelemetryWhenDerivedClickHouseWriteFails(t *testing.T) {
	kafka := &captureWriter{}
	clickhouse := &captureWriter{err: errors.New("clickhouse unavailable")}
	uc := NewIngestUseCase(
		kafka,
		clickhouse,
		zap.NewNop(),
		WithAudioBridge(AudioBridgeConfig{
			Enabled:                true,
			NoiseThresholdDb:       -35,
			VADConfidenceThreshold: 0.7,
			ConsecutiveEvents:      1,
			CooldownEvents:         8,
		}),
	)

	result := uc.Ingest(context.Background(), audioTelemetryEvent("evt-ch-down", -20, true, 0.95))
	if !result.Accepted {
		t.Fatalf("audio telemetry should remain accepted when ClickHouse is down: %v", result.Error)
	}
	if got := len(kafka.snapshot()); got != 1 {
		t.Fatalf("kafka events = %d, want 1", got)
	}
}

func audioTelemetryEvent(id string, rmsDb float32, vadActive bool, vadConfidence float32) *entity.ProctoringEvent {
	payload := fmt.Sprintf(
		`{"rmsDb":%.2f,"vadActive":%t,"vadConfidence":%.2f,"classification":"speech","classificationConfidence":0.9,"speakerCount":1,"speakerMatch":true,"speakerSimilarity":1,"segmentDurationMs":250}`,
		rmsDb,
		vadActive,
		vadConfidence,
	)
	return &entity.ProctoringEvent{
		EventID:             id,
		SessionID:           "session-1",
		StudentID:           "student-1",
		ExamID:              "exam-1",
		OrgID:               "org-1",
		EventType:           valueobject.AudioLevelTelemetry,
		Severity:            valueobject.SeverityInfo,
		Source:              valueobject.SourceBrowser,
		ClientTimestamp:     time.Unix(1_700_000_000, 0),
		Label:               "",
		Confidence:          1,
		PayloadType:         "audio_analysis",
		Payload:             []byte(payload),
		AudioRmsDb:          rmsDb,
		VADActive:           vadActive,
		AudioClassification: "speech",
		SpeakerCount:        1,
		SpeakerMatch:        true,
	}
}
