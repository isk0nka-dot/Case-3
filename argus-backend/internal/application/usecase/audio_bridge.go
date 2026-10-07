package usecase

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
)

// AudioBridgeConfig controls backend-side aggregation of browser-local audio
// telemetry into low-frequency anomaly events for ClickHouse analytics.
type AudioBridgeConfig struct {
	Enabled                bool
	NoiseThresholdDb       float32
	VADConfidenceThreshold float32
	ConsecutiveEvents      int
	CooldownEvents         int
	DerivedEventConfidence float32
}

// AudioBridge aggregates lightweight audio telemetry without blocking the
// student session path. It never transmits or stores raw microphone audio.
type AudioBridge struct {
	cfg    AudioBridgeConfig
	mu     sync.Mutex
	states map[string]audioBridgeState
}

type audioBridgeState struct {
	streak    int
	cooldown  int
	lastEvent string
}

type audioAnalysisPayloadFields struct {
	VADConfidence                 float32 `json:"vadConfidence"`
	VADConfidenceProtoJSON        float32 `json:"vad_confidence"`
	Classification                string  `json:"classification"`
	ClassificationConfidence      float32 `json:"classificationConfidence"`
	ClassificationConfidenceProto float32 `json:"classification_confidence"`
}

// NewAudioBridge creates an audio telemetry aggregator. A nil bridge is used
// when the feature is disabled so the ingest hot path stays branch-light.
func NewAudioBridge(cfg AudioBridgeConfig) *AudioBridge {
	applyAudioBridgeDefaults(&cfg)
	if !cfg.Enabled {
		return nil
	}
	return &AudioBridge{
		cfg:    cfg,
		states: make(map[string]audioBridgeState),
	}
}

func applyAudioBridgeDefaults(cfg *AudioBridgeConfig) {
	if cfg.NoiseThresholdDb == 0 {
		cfg.NoiseThresholdDb = -35
	}
	if cfg.VADConfidenceThreshold == 0 {
		cfg.VADConfidenceThreshold = 0.7
	}
	if cfg.ConsecutiveEvents <= 0 {
		cfg.ConsecutiveEvents = 3
	}
	if cfg.CooldownEvents < 0 {
		cfg.CooldownEvents = 0
	}
	if cfg.DerivedEventConfidence == 0 {
		cfg.DerivedEventConfidence = 0.85
	}
}

// Derive returns a ClickHouse-only audio anomaly when sustained telemetry
// exceeds configured noise/VAD thresholds. Explicit anomaly events are not
// duplicated; only AudioLevelTelemetry can produce a derived event.
func (b *AudioBridge) Derive(event *entity.ProctoringEvent) *entity.ProctoringEvent {
	if b == nil || event == nil || event.EventType != valueobject.AudioLevelTelemetry {
		return nil
	}

	payloadFields := parseAudioAnalysisPayload(event.Payload)
	vadConfidence := payloadFields.VADConfidence
	classification := event.AudioClassification
	if classification == "" {
		classification = payloadFields.Classification
	}

	exceedsNoise := event.AudioRmsDb >= b.cfg.NoiseThresholdDb
	exceedsVoice := event.VADActive && (vadConfidence == 0 || vadConfidence >= b.cfg.VADConfidenceThreshold)
	if !exceedsNoise && !exceedsVoice {
		b.resetStreak(event)
		return nil
	}

	b.mu.Lock()
	key := audioBridgeKey(event)
	state := b.states[key]
	if state.cooldown > 0 {
		state.cooldown--
		state.streak++
		state.lastEvent = event.EventID
		b.states[key] = state
		b.mu.Unlock()
		return nil
	}

	state.streak++
	state.lastEvent = event.EventID
	if state.streak < b.cfg.ConsecutiveEvents {
		b.states[key] = state
		b.mu.Unlock()
		return nil
	}

	state.streak = 0
	state.cooldown = b.cfg.CooldownEvents
	b.states[key] = state
	b.mu.Unlock()

	confidence := b.cfg.DerivedEventConfidence
	if vadConfidence > confidence {
		confidence = vadConfidence
	}

	labelReason := "noise"
	if exceedsVoice {
		labelReason = "voice"
	}
	if exceedsNoise && exceedsVoice {
		labelReason = "noise+voice"
	}

	payload := map[string]any{
		"rmsDb":                    event.AudioRmsDb,
		"vadActive":                event.VADActive,
		"vadConfidence":            vadConfidence,
		"classification":           classification,
		"classificationConfidence": payloadFields.ClassificationConfidence,
		"speakerCount":             event.SpeakerCount,
		"speakerMatch":             event.SpeakerMatch,
		"derivedFromEventId":       event.EventID,
		"derivation":               "sustained_audio_threshold",
	}
	payloadBytes, _ := json.Marshal(payload)

	return &entity.ProctoringEvent{
		EventID:             uuid.NewString(),
		SessionID:           event.SessionID,
		StudentID:           event.StudentID,
		ExamID:              event.ExamID,
		OrgID:               event.OrgID,
		EventType:           valueobject.AudioAnomaly,
		Severity:            valueobject.SeverityWarning,
		Source:              valueobject.SourceBackendAI,
		ClientTimestamp:     event.ClientTimestamp,
		VideoTimestamp:      event.VideoTimestamp,
		Label:               fmt.Sprintf("Backend audio anomaly: sustained %s", labelReason),
		Confidence:          confidence,
		Payload:             payloadBytes,
		PayloadType:         "audio_analysis",
		ClientMeta:          event.ClientMeta,
		AudioRmsDb:          event.AudioRmsDb,
		VADActive:           event.VADActive,
		AudioClassification: classification,
		SpeakerCount:        event.SpeakerCount,
		SpeakerMatch:        event.SpeakerMatch,
	}
}

func (b *AudioBridge) resetStreak(event *entity.ProctoringEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := audioBridgeKey(event)
	state := b.states[key]
	state.streak = 0
	state.lastEvent = event.EventID
	b.states[key] = state
}

func audioBridgeKey(event *entity.ProctoringEvent) string {
	return event.OrgID + ":" + event.SessionID
}

func parseAudioAnalysisPayload(payload []byte) audioAnalysisPayloadFields {
	if len(payload) == 0 {
		return audioAnalysisPayloadFields{}
	}
	var fields audioAnalysisPayloadFields
	_ = json.Unmarshal(payload, &fields)
	if fields.VADConfidence == 0 {
		fields.VADConfidence = fields.VADConfidenceProtoJSON
	}
	if fields.ClassificationConfidence == 0 {
		fields.ClassificationConfidence = fields.ClassificationConfidenceProto
	}
	return fields
}
