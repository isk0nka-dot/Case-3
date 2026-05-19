package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type PythonBridgeConfig struct {
	BaseURL string
	Timeout time.Duration
}

type PythonBridgeEngine struct {
	baseURL string
	client  *http.Client
}

func NewPythonBridgeEngine(ctx context.Context, cfg PythonBridgeConfig) (*PythonBridgeEngine, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("python bridge url is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	engine := &PythonBridgeEngine{
		baseURL: baseURL,
		client:  &http.Client{Timeout: cfg.Timeout},
	}
	if err := engine.checkHealth(ctx); err != nil {
		return nil, err
	}
	return engine, nil
}

func (e *PythonBridgeEngine) Name() string { return "python_bridge" }

func (e *PythonBridgeEngine) Close() error { return nil }

func (e *PythonBridgeEngine) AnalyzeFrame(ctx context.Context, frame []byte, contentType string, options ...FrameAnalysisOptions) (*FrameResult, error) {
	opts := firstFrameAnalysisOptions(options)
	reqBody := analyzeFrameRequest{
		ContentType: contentType,
		FrameBase64: base64.StdEncoding.EncodeToString(frame),
	}
	if len(opts.ReferenceEmbedding) > 0 {
		reqBody.ReferenceEmbedding = append([]float32(nil), opts.ReferenceEmbedding...)
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal python bridge frame request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/analyze-frame", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build python bridge frame request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("python bridge frame request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("python bridge frame request returned status %d", resp.StatusCode)
	}

	var out analyzeFrameResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode python bridge frame response: %w", err)
	}
	return out.toFrameResult(), nil
}

func (e *PythonBridgeEngine) checkHealth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("build python bridge health request: %w", err)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("python bridge health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("python bridge health check returned status %d", resp.StatusCode)
	}

	var health bridgeHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return fmt.Errorf("decode python bridge health response: %w", err)
	}
	if health.Status != "ok" {
		return fmt.Errorf("python bridge is not healthy: status=%s models=%v", health.Status, health.Models)
	}
	for _, name := range []string{"yolo", "arcface"} {
		model, ok := health.Models[name]
		if !ok || !model.Loaded {
			return fmt.Errorf("python bridge is not healthy: required model %q is not loaded", name)
		}
	}
	return nil
}

type bridgeHealthResponse struct {
	Status string                      `json:"status"`
	Models map[string]bridgeModelState `json:"models"`
}

type bridgeModelState struct {
	Loaded bool   `json:"loaded"`
	Error  string `json:"error,omitempty"`
}

type analyzeFrameRequest struct {
	ContentType        string    `json:"content_type"`
	FrameBase64        string    `json:"frame_base64"`
	ReferenceEmbedding []float32 `json:"reference_embedding,omitempty"`
}

type analyzeFrameResponse struct {
	Faces     []bridgeFace   `json:"faces"`
	Objects   []bridgeObject `json:"objects"`
	Liveness  bridgeLiveness `json:"liveness"`
	LatencyMs float64        `json:"latency_ms"`
}

type bridgeFace struct {
	Confidence float32    `json:"confidence"`
	BBox       bridgeBBox `json:"bbox"`
	Embedding  []float32  `json:"embedding"`
	Similarity float32    `json:"similarity"`
	IsSpoof    bool       `json:"is_spoof"`
	SpoofType  string     `json:"spoof_type"`
	HeadYaw    float32    `json:"head_yaw"`
	HeadPitch  float32    `json:"head_pitch"`
	HeadRoll   float32    `json:"head_roll"`
}

type bridgeObject struct {
	ObjectType string     `json:"object_type"`
	Confidence float32    `json:"confidence"`
	BBox       bridgeBBox `json:"bbox"`
}

type bridgeLiveness struct {
	Score  float32 `json:"score"`
	IsLive bool    `json:"is_live"`
	Method string  `json:"method"`
}

type bridgeBBox [4]float32

func (b *bridgeBBox) UnmarshalJSON(data []byte) error {
	var arr [4]float32
	if err := json.Unmarshal(data, &arr); err == nil {
		*b = bridgeBBox(arr)
		return nil
	}

	var obj struct {
		X float32 `json:"x"`
		Y float32 `json:"y"`
		W float32 `json:"w"`
		H float32 `json:"h"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	*b = bridgeBBox{obj.X, obj.Y, obj.W, obj.H}
	return nil
}

func (b bridgeBBox) boundingBox() BoundingBox {
	return BoundingBox{X: b[0], Y: b[1], W: b[2], H: b[3]}
}

func (r analyzeFrameResponse) toFrameResult() *FrameResult {
	result := &FrameResult{
		Faces:     make([]FaceResult, 0, len(r.Faces)),
		Objects:   make([]ObjectResult, 0, len(r.Objects)),
		Liveness:  LivenessResult{Score: r.Liveness.Score, IsLive: r.Liveness.IsLive, Method: r.Liveness.Method},
		LatencyMs: r.LatencyMs,
	}
	for _, face := range r.Faces {
		result.Faces = append(result.Faces, FaceResult{
			Confidence: face.Confidence,
			BBox:       face.BBox.boundingBox(),
			Embedding:  face.Embedding,
			Similarity: face.Similarity,
			IsSpoof:    face.IsSpoof,
			SpoofType:  face.SpoofType,
			HeadYaw:    face.HeadYaw,
			HeadPitch:  face.HeadPitch,
			HeadRoll:   face.HeadRoll,
		})
	}
	for _, obj := range r.Objects {
		result.Objects = append(result.Objects, ObjectResult{
			ObjectType: obj.ObjectType,
			Confidence: obj.Confidence,
			BBox:       obj.BBox.boundingBox(),
		})
	}
	return result
}
