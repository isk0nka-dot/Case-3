// Package valueobject defines immutable value objects for the Event Collector domain.
// Value objects have no identity — they are defined entirely by their attributes.
package valueobject

// EventType classifies proctoring events. Each type maps directly to
// a specific AI detector or browser/system probe from the frontend settings.
type EventType int32

const (
	EventTypeUnspecified EventType = 0

	// Video & Face events (Видео-правила)
	GazeDeviation            EventType = 1
	FaceMismatch             EventType = 2
	FaceNotDetected          EventType = 3
	FaceSpoofDetected        EventType = 4
	MultiplePersons          EventType = 5
	DynamicFaceRecheckFail   EventType = 6

	// Object detection events
	PhoneDetected            EventType = 10
	BookDetected             EventType = 11
	EarbudsDetected          EventType = 12
	UnknownObjectDetected    EventType = 13

	// Audio events (Чувствительность ИИ)
	VoiceActivity            EventType = 20
	AudioAnomaly             EventType = 21
	AudioPeripheryDetected   EventType = 22
	SmartNoiseClassified     EventType = 23

	// Browser events (Ограничения браузера)
	TabSwitch                EventType = 30
	CopyPasteAttempt         EventType = 31
	PrintScreenAttempt       EventType = 32
	ContextMenuAttempt       EventType = 33
	FullscreenExit           EventType = 34
	ExternalDisplayDetected  EventType = 35

	// Network events (Сетевой контроль)
	VPNProxyDetected         EventType = 40
	SuspiciousNetworkDevice  EventType = 41

	// Psychometry events (Психометрия и AI-аналитика)
	EmotionStressSpike       EventType = 50
	FocusLossDetected        EventType = 51
	BlinkPatternAnomaly      EventType = 52

	// Behavioral analysis events (Поведенческий анализ)
	// NOTE: These use kernel-level data (SourceKernelAgent) for 100% integrity,
	// but conceptually belong to behavioral biometrics, not system control.
	TypingDynamicsAnomaly    EventType = 60
	HandCursorDesync         EventType = 61

	// Kernel-level events (Системный контроль)
	ForbiddenProcessDetected EventType = 62
	HardwareDeviceAnomaly    EventType = 63
	RemoteAccessDetected     EventType = 64
	HardwareIDMismatch       EventType = 65
	VirtualMonitorDetected   EventType = 66
	VirtualMachineDetected   EventType = 67

	// AI Vision events (biometric inference results)
	HeadPoseAnomaly          EventType = 7  // Yaw/Pitch/Roll outside threshold
	LivenessCheckFailed      EventType = 8  // Eye-blink + texture liveness failure
	FaceOccluded             EventType = 9  // Partial face occlusion detected

	// AI Audio events (sound activity detection)
	WhisperDetected          EventType = 24 // Low-amplitude speech (<40dB)
	SecondSpeakerDetected    EventType = 25 // Voice embedding mismatch
	AudioPlaybackDetected    EventType = 26 // TTS/recorded audio fingerprint

	// Secondary Camera (Side Camera) events (Fix 7)
	SidecamDeviceDisplaced    EventType = 70 // Accelerometer displacement > 2.0
	SidecamHandsOffDesk       EventType = 71 // Hands left desk area
	SidecamBatteryCritical    EventType = 72 // Battery < 10%
	SidecamStreamDisconnected EventType = 73 // Heartbeat timeout
	SidecamCalibrationFailed  EventType = 74 // Golden angle calibration retry
	SidecamThermalThrottle    EventType = 75 // FPS throttled due to thermal

	// Backend AI deep scan events (post-session GPU re-analysis)
	BackendAIFaceMismatch     EventType = 80 // Backend confirmed face doesn't match enrolled
	BackendAIScreenReflection EventType = 81 // Another person's reflection detected on screen
	BackendAIMicroExpression  EventType = 82 // Micro-expression indicating deception
	BackendAIHiddenObject     EventType = 83 // Object detected that frontend missed
	BackendAIDeepfakeDetected EventType = 84 // Advanced deepfake analysis (backend-only model)
	BackendAIVoiceSynth       EventType = 85 // Voice synthesis / TTS detected

	// Telemetry (high-frequency, low-severity)
	GazeTelemetry            EventType = 100
	MouseTelemetry           EventType = 101
	KeyboardTelemetry        EventType = 102
	FocusScoreUpdate         EventType = 103

	// AI Telemetry (continuous inference streams)
	HeadPoseTelemetry        EventType = 104 // Continuous yaw/pitch/roll
	FaceEmbeddingTelemetry   EventType = 105 // Periodic face embedding snapshot
	AudioLevelTelemetry      EventType = 106 // Continuous dB + VAD stream
)

// IsTelemetry returns true for high-frequency telemetry event types.
// These are routed to a separate Kafka topic for batch analytics.
func (et EventType) IsTelemetry() bool {
	return et >= 100
}

// IsViolation returns true if this event type represents a rule violation.
func (et EventType) IsViolation() bool {
	return et > 0 && et < 100
}

// String returns a human-readable label for the event type.
func (et EventType) String() string {
	names := map[EventType]string{
		GazeDeviation:            "gaze_deviation",
		FaceMismatch:             "face_mismatch",
		FaceNotDetected:          "face_not_detected",
		FaceSpoofDetected:        "face_spoof_detected",
		MultiplePersons:          "multiple_persons",
		DynamicFaceRecheckFail:   "dynamic_face_recheck_fail",
		PhoneDetected:            "phone_detected",
		BookDetected:             "book_detected",
		EarbudsDetected:          "earbuds_detected",
		UnknownObjectDetected:    "unknown_object_detected",
		VoiceActivity:            "voice_activity",
		AudioAnomaly:             "audio_anomaly",
		AudioPeripheryDetected:   "audio_periphery_detected",
		SmartNoiseClassified:     "smart_noise_classified",
		TabSwitch:                "tab_switch",
		CopyPasteAttempt:         "copy_paste_attempt",
		PrintScreenAttempt:       "print_screen_attempt",
		ContextMenuAttempt:       "context_menu_attempt",
		FullscreenExit:           "fullscreen_exit",
		ExternalDisplayDetected:  "external_display_detected",
		VPNProxyDetected:         "vpn_proxy_detected",
		SuspiciousNetworkDevice:  "suspicious_network_device",
		EmotionStressSpike:       "emotion_stress_spike",
		FocusLossDetected:        "focus_loss_detected",
		BlinkPatternAnomaly:      "blink_pattern_anomaly",
		TypingDynamicsAnomaly:    "typing_dynamics_anomaly",
		HandCursorDesync:         "hand_cursor_desync",
		ForbiddenProcessDetected: "forbidden_process_detected",
		HardwareDeviceAnomaly:    "hardware_device_anomaly",
		RemoteAccessDetected:     "remote_access_detected",
		HardwareIDMismatch:       "hardware_id_mismatch",
		VirtualMonitorDetected:   "virtual_monitor_detected",
		VirtualMachineDetected:   "virtual_machine_detected",
		HeadPoseAnomaly:          "head_pose_anomaly",
		LivenessCheckFailed:      "liveness_check_failed",
		FaceOccluded:             "face_occluded",
		WhisperDetected:          "whisper_detected",
		SecondSpeakerDetected:    "second_speaker_detected",
		AudioPlaybackDetected:    "audio_playback_detected",
		// Secondary Camera events (Fix 7) — UPPERCASE to match penalty rules
		SidecamDeviceDisplaced:    "SIDECAM_DEVICE_DISPLACED",
		SidecamHandsOffDesk:       "SIDECAM_HANDS_OFF_DESK",
		SidecamBatteryCritical:    "SIDECAM_BATTERY_CRITICAL",
		SidecamStreamDisconnected: "SIDECAM_STREAM_DISCONNECTED",
		SidecamCalibrationFailed:  "SIDECAM_CALIBRATION_FAILED",
		SidecamThermalThrottle:    "SIDECAM_THERMAL_THROTTLE",
		// Backend AI deep scan events — UPPERCASE to match penalty rules
		BackendAIFaceMismatch:     "BACKEND_AI_FACE_MISMATCH",
		BackendAIScreenReflection: "BACKEND_AI_SCREEN_REFLECTION",
		BackendAIMicroExpression:  "BACKEND_AI_MICRO_EXPRESSION",
		BackendAIHiddenObject:     "BACKEND_AI_HIDDEN_OBJECT",
		BackendAIDeepfakeDetected: "BACKEND_AI_DEEPFAKE_DETECTED",
		BackendAIVoiceSynth:       "BACKEND_AI_VOICE_SYNTH",
		GazeTelemetry:            "gaze_telemetry",
		MouseTelemetry:           "mouse_telemetry",
		KeyboardTelemetry:        "keyboard_telemetry",
		FocusScoreUpdate:         "focus_score_update",
		HeadPoseTelemetry:        "head_pose_telemetry",
		FaceEmbeddingTelemetry:   "face_embedding_telemetry",
		AudioLevelTelemetry:      "audio_level_telemetry",
	}
	if name, ok := names[et]; ok {
		return name
	}
	return "unspecified"
}

// Severity classifies event urgency.
type Severity int32

const (
	SeverityUnspecified Severity = 0
	SeverityInfo        Severity = 1
	SeverityWarning     Severity = 2
	SeverityCritical    Severity = 3
)

// String returns a human-readable severity label.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	default:
		return "unspecified"
	}
}

// EventSource identifies where the event originated.
type EventSource int32

const (
	SourceUnspecified  EventSource = 0
	SourceWebcam       EventSource = 1
	SourceSideCamera   EventSource = 2
	SourceSystem       EventSource = 3
	SourceBrowser      EventSource = 4
	SourceKernelAgent  EventSource = 5
	SourceNetworkProbe EventSource = 6
	SourceBackendAI    EventSource = 7 // Backend GPU-accelerated inference service
)

// String returns a human-readable source label.
func (es EventSource) String() string {
	switch es {
	case SourceWebcam:
		return "webcam"
	case SourceSideCamera:
		return "side_camera"
	case SourceSystem:
		return "system"
	case SourceBrowser:
		return "browser"
	case SourceKernelAgent:
		return "kernel_agent"
	case SourceNetworkProbe:
		return "network_probe"
	case SourceBackendAI:
		return "backend_ai"
	default:
		return "unspecified"
	}
}
