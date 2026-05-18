package inference

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// StubEngine returns synthetic, deterministic inference results for development
// and testing. Results are seeded from a hash of the frame data so identical
// frames always produce identical detections — useful for reproducible tests.
//
// Detection rates:
//   - Face: 100% (always detects 1 face, similarity 0.92, liveness 0.95)
//   - Phone object: ~10% of frames
//   - Spoof: ~5% of frames
//   - Simulated latency: 15 ms
type StubEngine struct{}

// NewStubEngine creates a new stub inference engine.
func NewStubEngine() *StubEngine {
	return &StubEngine{}
}

// Name returns the engine identifier.
func (e *StubEngine) Name() string { return "stub" }

// Close is a no-op for the stub engine.
func (e *StubEngine) Close() error { return nil }

// AnalyzeFrame returns synthetic detections derived from a hash of the frame bytes.
func (e *StubEngine) AnalyzeFrame(_ context.Context, frame []byte, _ string, _ ...FrameAnalysisOptions) (*FrameResult, error) {
	start := time.Now()

	// Simulate GPU inference latency.
	time.Sleep(15 * time.Millisecond)

	// Seed deterministic results from frame content hash.
	hash := sha256.Sum256(frame)
	seed := binary.BigEndian.Uint32(hash[:4])

	result := &FrameResult{
		Faces: []FaceResult{
			{
				Confidence: 0.97,
				BBox:       BoundingBox{X: 0.25, Y: 0.15, W: 0.30, H: 0.40},
				Embedding:  stubEmbedding(hash),
				Similarity: 0.92,
				IsSpoof:    seed%20 == 0, // ~5% spoof rate
				SpoofType:  "",
				HeadYaw:    float32(int(seed%30) - 15), // -15 to +14 degrees
				HeadPitch:  float32(int(seed%20) - 10), // -10 to +9 degrees
				HeadRoll:   float32(int(seed%10) - 5),  // -5 to +4 degrees
			},
		},
		Liveness: LivenessResult{
			Score:  0.95,
			IsLive: seed%20 != 0,
			Method: "texture",
		},
	}

	// Mark spoof type when spoof is detected.
	if result.Faces[0].IsSpoof {
		result.Faces[0].SpoofType = "photo"
		result.Liveness.Score = 0.15
		result.Liveness.IsLive = false
	}

	// ~10% of frames: detect a phone object.
	if seed%10 == 0 {
		result.Objects = []ObjectResult{
			{
				ObjectType: "phone",
				Confidence: 0.85,
				BBox:       BoundingBox{X: 0.60, Y: 0.70, W: 0.10, H: 0.15},
			},
		}
	}

	// ~3% of frames: detect screen reflection (another person).
	if seed%33 == 0 {
		result.Objects = append(result.Objects, ObjectResult{
			ObjectType: "screen_reflection",
			Confidence: 0.72,
			BBox:       BoundingBox{X: 0.05, Y: 0.05, W: 0.15, H: 0.20},
		})
	}

	result.LatencyMs = float64(time.Since(start).Microseconds()) / 1000.0
	return result, nil
}

// stubEmbedding generates a deterministic 512-dim embedding from a SHA-256 hash.
func stubEmbedding(hash [32]byte) []float32 {
	emb := make([]float32, 512)
	for i := range emb {
		// Use bytes cyclically from the hash to produce pseudo-random floats.
		b := hash[i%32]
		emb[i] = float32(b)/255.0*2.0 - 1.0 // Range [-1, 1]
	}
	return emb
}
