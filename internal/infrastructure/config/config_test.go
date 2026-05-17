package config

import "testing"

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
