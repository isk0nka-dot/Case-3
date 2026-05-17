package inference

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPythonBridgeAnalyzeFrameMapsSidecarDetections(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"models": map[string]any{
					"yolo":    map[string]any{"loaded": true},
					"arcface": map[string]any{"loaded": true},
				},
			})
		case "/v1/analyze-frame":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"latency_ms": 12.5,
				"faces": []map[string]any{
					{
						"confidence": 0.98,
						"bbox":       []float32{0.1, 0.2, 0.3, 0.4},
						"embedding":  []float32{0.11, 0.22},
						"similarity": 0.57,
						"is_spoof":   false,
						"head_yaw":   12.0,
					},
				},
				"objects": []map[string]any{
					{
						"object_type": "phone",
						"confidence":  0.91,
						"bbox":        []float32{0.5, 0.6, 0.1, 0.2},
					},
				},
				"liveness": map[string]any{
					"score":   0.88,
					"is_live": true,
					"method":  "arcface_sidecar",
				},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer sidecar.Close()

	engine, err := NewPythonBridgeEngine(context.Background(), PythonBridgeConfig{
		BaseURL: sidecar.URL,
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("create bridge engine: %v", err)
	}

	result, err := engine.AnalyzeFrame(context.Background(), []byte("jpeg-data"), "image/jpeg")
	if err != nil {
		t.Fatalf("analyze frame: %v", err)
	}
	if len(result.Faces) != 1 || result.Faces[0].Similarity != 0.57 {
		t.Fatalf("unexpected face result: %#v", result.Faces)
	}
	if len(result.Objects) != 1 || result.Objects[0].ObjectType != "phone" || result.Objects[0].Confidence != 0.91 {
		t.Fatalf("unexpected object result: %#v", result.Objects)
	}
	if !result.Liveness.IsLive || result.Liveness.Method != "arcface_sidecar" {
		t.Fatalf("unexpected liveness result: %#v", result.Liveness)
	}
}
