package config

import "testing"

func TestInferenceDefaultEngineIsPythonBridge(t *testing.T) {
	cfg := &Config{}

	applyDefaults(cfg)

	if cfg.Inference.EngineType != "python_bridge" {
		t.Fatalf("expected production-safe python_bridge default, got %q", cfg.Inference.EngineType)
	}
	if cfg.Inference.PythonBridgeURL == "" {
		t.Fatal("expected python bridge URL default")
	}
	if cfg.Inference.AllowStub {
		t.Fatal("allow_stub must default to false")
	}
}

func TestDeploymentConfigLoadsPythonBridgeInference(t *testing.T) {
	cfg, err := Load("../../../deployments/config.yaml")
	if err != nil {
		t.Fatalf("load deployment config: %v", err)
	}

	if cfg.Inference.EngineType != "python_bridge" {
		t.Fatalf("expected python_bridge inference engine, got %q", cfg.Inference.EngineType)
	}
	if cfg.Inference.PythonBridgeURL == "" {
		t.Fatal("expected python bridge URL to be configured")
	}
	if cfg.Inference.Concurrency < 1 {
		t.Fatalf("expected positive inference concurrency, got %d", cfg.Inference.Concurrency)
	}
}

func TestDeploymentConfigEnablesAudioBridge(t *testing.T) {
	cfg, err := Load("../../../deployments/config.yaml")
	if err != nil {
		t.Fatalf("load deployment config: %v", err)
	}

	if !cfg.AudioBridge.Enabled {
		t.Fatal("expected deployment audio bridge to be enabled")
	}
	if cfg.AudioBridge.NoiseThresholdDb >= 0 {
		t.Fatalf("expected negative audio noise dB threshold, got %f", cfg.AudioBridge.NoiseThresholdDb)
	}
	if cfg.AudioBridge.VADConfidenceThreshold <= 0 || cfg.AudioBridge.VADConfidenceThreshold > 1 {
		t.Fatalf("expected VAD threshold in (0,1], got %f", cfg.AudioBridge.VADConfidenceThreshold)
	}
	if cfg.AudioBridge.ConsecutiveEvents < 1 {
		t.Fatalf("expected positive consecutive event limit, got %d", cfg.AudioBridge.ConsecutiveEvents)
	}
}

func TestAudioBridgeEnvOverrides(t *testing.T) {
	t.Setenv("EVENT_COLLECTOR_AUDIO_BRIDGE_ENABLED", "true")
	t.Setenv("EVENT_COLLECTOR_AUDIO_NOISE_THRESHOLD_DB", "-32.5")
	t.Setenv("EVENT_COLLECTOR_AUDIO_VAD_CONFIDENCE_THRESHOLD", "0.81")
	t.Setenv("EVENT_COLLECTOR_AUDIO_CONSECUTIVE_EVENTS", "4")
	t.Setenv("EVENT_COLLECTOR_AUDIO_COOLDOWN_EVENTS", "9")

	cfg := &Config{}
	applyDefaults(cfg)
	applyEnvOverrides(cfg)

	if !cfg.AudioBridge.Enabled {
		t.Fatal("expected audio bridge env override to enable bridge")
	}
	if cfg.AudioBridge.NoiseThresholdDb != -32.5 {
		t.Fatalf("noise threshold = %f, want -32.5", cfg.AudioBridge.NoiseThresholdDb)
	}
	if cfg.AudioBridge.VADConfidenceThreshold != 0.81 {
		t.Fatalf("vad threshold = %f, want 0.81", cfg.AudioBridge.VADConfidenceThreshold)
	}
	if cfg.AudioBridge.ConsecutiveEvents != 4 {
		t.Fatalf("consecutive events = %d, want 4", cfg.AudioBridge.ConsecutiveEvents)
	}
	if cfg.AudioBridge.CooldownEvents != 9 {
		t.Fatalf("cooldown events = %d, want 9", cfg.AudioBridge.CooldownEvents)
	}
}

func TestInferenceFrameBudgetEnvOverrides(t *testing.T) {
	t.Setenv("EVENT_COLLECTOR_INFERENCE_MAX_FRAME_BYTES", "2097152")
	t.Setenv("EVENT_COLLECTOR_INFERENCE_MAX_VIDEO_DUR_SEC", "45")

	cfg := &Config{}
	applyDefaults(cfg)
	applyEnvOverrides(cfg)

	if cfg.Inference.MaxFrameBytes != 2097152 {
		t.Fatalf("max frame bytes = %d, want 2097152", cfg.Inference.MaxFrameBytes)
	}
	if cfg.Inference.MaxVideoDurSec != 45 {
		t.Fatalf("max video duration = %d, want 45", cfg.Inference.MaxVideoDurSec)
	}
}
