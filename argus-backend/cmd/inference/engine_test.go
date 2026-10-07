package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/argus-ai/event-collector/internal/infrastructure/config"
)

func TestNewInferenceEngineRejectsStubUnlessExplicitlyAllowed(t *testing.T) {
	_, err := newInferenceEngine(config.InferenceConfig{
		EngineType: "stub",
		AllowStub:  false,
	})
	if err == nil {
		t.Fatal("expected stub engine to be rejected when allow_stub is false")
	}
	if !strings.Contains(err.Error(), "stub inference engine is disabled") {
		t.Fatalf("expected explicit disabled stub error, got %q", err.Error())
	}
}

func TestNewInferenceEngineAllowsStubForExplicitDevelopmentConfig(t *testing.T) {
	engine, err := newInferenceEngine(config.InferenceConfig{
		EngineType: "stub",
		AllowStub:  true,
	})
	if err != nil {
		t.Fatalf("expected stub engine to be allowed for explicit development config: %v", err)
	}
	defer engine.Close()

	if engine.Name() != "stub" {
		t.Fatalf("expected stub engine, got %q", engine.Name())
	}
}

func TestNewInferenceEngineInitializesPythonBridgeWhenModelsAreHealthy(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"models": map[string]any{
				"yolo":    map[string]any{"loaded": true},
				"arcface": map[string]any{"loaded": true},
			},
		})
	}))
	defer sidecar.Close()

	engine, err := newInferenceEngine(config.InferenceConfig{
		EngineType:       "python_bridge",
		PythonBridgeURL:  sidecar.URL,
		BridgeTimeoutSec: 2,
	})
	if err != nil {
		t.Fatalf("expected healthy python bridge engine: %v", err)
	}
	defer engine.Close()

	if engine.Name() != "python_bridge" {
		t.Fatalf("expected python_bridge engine, got %q", engine.Name())
	}
}

func TestNewInferenceEngineFailsFastWhenPythonBridgeModelMissing(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "degraded",
			"models": map[string]any{
				"yolo":    map[string]any{"loaded": true},
				"arcface": map[string]any{"loaded": false, "error": "models/arcface.onnx: no such file"},
			},
		})
	}))
	defer sidecar.Close()

	_, err := newInferenceEngine(config.InferenceConfig{
		EngineType:       "python_bridge",
		PythonBridgeURL:  sidecar.URL,
		BridgeTimeoutSec: 2,
	})
	if err == nil {
		t.Fatal("expected python bridge startup to fail when a required model is not loaded")
	}
	if !strings.Contains(err.Error(), "python bridge is not healthy") {
		t.Fatalf("expected model health error, got %q", err.Error())
	}
}
