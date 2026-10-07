// Package inference provides the pluggable AI inference engine abstraction.
//
// The Engine interface decouples the inference gateway and worker from the
// underlying AI framework. Implementations:
//
//   - StubEngine    — returns synthetic detections for development and testing
//   - ONNXEngine    — ONNX Runtime Go bindings for GPU-accelerated inference (future)
//   - PythonBridge  — calls the Python ONNX HTTP sidecar
package inference

import "context"

// Engine is the pluggable AI inference backend. All inference operations flow
// through this interface, allowing the gateway and worker to remain agnostic
// to the underlying model framework.
type Engine interface {
	// AnalyzeFrame runs face detection, object detection, and liveness
	// verification on a single image frame. The contentType indicates the
	// image format ("image/jpeg" or "image/png").
	AnalyzeFrame(ctx context.Context, frame []byte, contentType string, options ...FrameAnalysisOptions) (*FrameResult, error)

	// Name returns the engine identifier for logging and metrics.
	Name() string

	// Close releases resources (GPU memory, model handles, connections).
	Close() error
}

// FrameAnalysisOptions carries optional per-frame context for real model
// inference. Empty options preserve the legacy behavior.
type FrameAnalysisOptions struct {
	// ReferenceEmbedding is the enrolled 512-dimensional ArcFace vector used
	// by the sidecar to compute identity similarity.
	ReferenceEmbedding []float32
}

func firstFrameAnalysisOptions(options []FrameAnalysisOptions) FrameAnalysisOptions {
	if len(options) == 0 {
		return FrameAnalysisOptions{}
	}
	return options[0]
}

// FrameResult contains all detections from a single frame analysis.
type FrameResult struct {
	Faces     []FaceResult
	Objects   []ObjectResult
	Liveness  LivenessResult
	LatencyMs float64
}

// FaceResult represents a single detected face with identity and spoof analysis.
type FaceResult struct {
	Confidence float32
	BBox       BoundingBox
	Embedding  []float32 // 512-dim ArcFace vector
	Similarity float32   // cosine similarity to enrolled face (-1 = not computed)
	IsSpoof    bool
	SpoofType  string // "photo" | "video" | "mask" | "deepfake" | ""
	HeadYaw    float32
	HeadPitch  float32
	HeadRoll   float32
}

// ObjectResult represents a single detected object.
type ObjectResult struct {
	ObjectType string // "phone" | "book" | "earbuds" | "screen_reflection" | "person"
	Confidence float32
	BBox       BoundingBox
}

// BoundingBox defines a rectangular region in normalized coordinates (0-1).
type BoundingBox struct {
	X float32
	Y float32
	W float32
	H float32
}

// LivenessResult contains the liveness verification outcome.
type LivenessResult struct {
	Score  float32 // 0.0 = definite spoof, 1.0 = definitely live
	IsLive bool
	Method string // "texture" | "blink" | "depth"
}
