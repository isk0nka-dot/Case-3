package grpc

import (
	"bytes"
	"context"
	"io"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
	"github.com/argus-ai/event-collector/internal/infrastructure/inference"
)

// InferenceServer implements the InferenceServiceServer gRPC interface.
// It delegates all AI work to the pluggable Engine and maps results to proto.
type InferenceServer struct {
	inferencepb.UnimplementedInferenceServiceServer
	engine        inference.Engine
	logger        *zap.Logger
	maxFrameBytes int
	frameSlots    chan struct{}
}

type InferenceServerOptions struct {
	MaxFrameBytes       int
	MaxConcurrentFrames int
}

// NewInferenceServer creates a new inference gRPC server.
func NewInferenceServer(engine inference.Engine, logger *zap.Logger, maxFrameBytes int) *InferenceServer {
	return NewInferenceServerWithOptions(engine, logger, InferenceServerOptions{
		MaxFrameBytes: maxFrameBytes,
	})
}

func NewInferenceServerWithOptions(engine inference.Engine, logger *zap.Logger, options InferenceServerOptions) *InferenceServer {
	var frameSlots chan struct{}
	if options.MaxConcurrentFrames > 0 {
		frameSlots = make(chan struct{}, options.MaxConcurrentFrames)
	}

	return &InferenceServer{
		engine:        engine,
		logger:        logger.Named("inference_grpc"),
		maxFrameBytes: options.MaxFrameBytes,
		frameSlots:    frameSlots,
	}
}

// AnalyzeFrame processes a single image frame and returns structured detections.
func (s *InferenceServer) AnalyzeFrame(ctx context.Context, req *inferencepb.AnalyzeFrameRequest) (*inferencepb.AnalyzeFrameResponse, error) {
	if len(req.FrameData) == 0 {
		return nil, status.Error(codes.InvalidArgument, "frame_data is required")
	}
	if len(req.FrameData) > s.maxFrameBytes {
		return nil, status.Errorf(codes.InvalidArgument, "frame_data exceeds maximum size (%d bytes)", s.maxFrameBytes)
	}
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	release, ok := s.tryAcquireFrameSlot()
	if !ok {
		s.logger.Warn("inference: frame dropped because worker pool is full",
			zap.String("session_id", req.SessionId),
		)
		return nil, status.Error(codes.ResourceExhausted, "inference worker pool is full; frame dropped")
	}
	defer release()

	result, err := s.engine.AnalyzeFrame(ctx, req.FrameData, req.ContentType)
	if err != nil {
		s.logger.Error("inference: frame analysis failed",
			zap.String("session_id", req.SessionId),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "frame analysis failed: %v", err)
	}

	return mapFrameResult(result), nil
}

// AnalyzeVideo receives a client-streamed video and returns frame-by-frame analysis.
func (s *InferenceServer) AnalyzeVideo(stream inferencepb.InferenceService_AnalyzeVideoServer) error {
	start := time.Now()

	var (
		sessionID   string
		orgID       string
		contentType string
		buf         bytes.Buffer
	)

	// ── Receive all chunks ────────────────────────────────────────────────
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status.Errorf(codes.Internal, "recv error: %v", err)
		}

		// Capture metadata from the first chunk.
		if sessionID == "" {
			sessionID = chunk.SessionId
			orgID = chunk.OrgId
			contentType = chunk.ContentType
		}

		buf.Write(chunk.Data)

		if chunk.IsLast {
			break
		}
	}

	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "session_id is required (set on first chunk)")
	}
	if buf.Len() == 0 {
		return status.Error(codes.InvalidArgument, "no video data received")
	}

	s.logger.Info("inference: analyzing video segment",
		zap.String("session_id", sessionID),
		zap.String("org_id", orgID),
		zap.String("content_type", contentType),
		zap.Int("total_bytes", buf.Len()),
	)

	// ── Extract frames and analyze ────────────────────────────────────────
	// For MVP, treat the entire buffer as a single "frame" for the stub engine.
	// A real implementation would use ffmpeg to extract frames at the configured
	// sample rate. This placeholder simulates multi-frame analysis by splitting
	// the buffer into fixed-size chunks and analyzing each.
	frames, summary := s.analyzeVideoBuffer(stream.Context(), buf.Bytes())

	totalTime := float64(time.Since(start).Microseconds()) / 1000.0

	s.logger.Info("inference: video analysis complete",
		zap.String("session_id", sessionID),
		zap.Int("frames_analyzed", len(frames)),
		zap.String("verdict", summary.Verdict),
		zap.Float64("total_ms", totalTime),
	)

	return stream.SendAndClose(&inferencepb.AnalyzeVideoResponse{
		Frames:                frames,
		Summary:               mapAnalysisSummary(summary),
		TotalProcessingTimeMs: totalTime,
	})
}

// analyzeVideoBuffer simulates frame extraction and per-frame analysis.
// In a real implementation, this would use ffmpeg/libav to decode the video
// and extract frames at the configured sample rate.
func (s *InferenceServer) analyzeVideoBuffer(ctx context.Context, data []byte) ([]*inferencepb.FrameAnalysis, *analysisSummary) {
	// Simulate frame extraction by splitting into ~50KB pseudo-frames.
	const pseudoFrameSize = 50 * 1024
	var frames []*inferencepb.FrameAnalysis
	summary := &analysisSummary{}

	frameIdx := 0
	for offset := 0; offset < len(data); offset += pseudoFrameSize {
		end := offset + pseudoFrameSize
		if end > len(data) {
			end = len(data)
		}
		frameData := data[offset:end]

		release, ok := s.tryAcquireFrameSlot()
		if !ok {
			s.logger.Warn("inference: video frame dropped because worker pool is full",
				zap.Int("frame_idx", frameIdx),
			)
			frameIdx++
			continue
		}
		result, err := s.engine.AnalyzeFrame(ctx, frameData, "image/jpeg")
		release()
		if err != nil {
			s.logger.Warn("inference: frame analysis error, skipping",
				zap.Int("frame_idx", frameIdx),
				zap.Error(err),
			)
			frameIdx++
			continue
		}

		fa := &inferencepb.FrameAnalysis{
			TimestampSec: float64(frameIdx) * 0.5, // Assume 2 FPS sampling
			Faces:        mapFaceDetections(result.Faces),
			Objects:      mapObjectDetections(result.Objects),
			Liveness:     mapLivenessResult(result.Liveness),
		}
		frames = append(frames, fa)

		// Update summary counters.
		summary.TotalFrames++
		summary.updateFromResult(result)

		frameIdx++
	}

	// Determine verdict.
	if summary.TotalFrames > 0 {
		fraudRate := float64(summary.FraudFrames) / float64(summary.TotalFrames)
		switch {
		case fraudRate > 0.15 || summary.MaxFraudConf > 0.9:
			summary.Verdict = "fraud"
		case fraudRate > 0.05 || summary.MaxFraudConf > 0.7:
			summary.Verdict = "suspicious"
		default:
			summary.Verdict = "clean"
		}
	} else {
		summary.Verdict = "clean"
	}

	return frames, summary
}

func (s *InferenceServer) tryAcquireFrameSlot() (func(), bool) {
	if s.frameSlots == nil {
		return func() {}, true
	}

	select {
	case s.frameSlots <- struct{}{}:
		return func() { <-s.frameSlots }, true
	default:
		return nil, false
	}
}

// ---------------------------------------------------------------------------
// Summary accumulator
// ---------------------------------------------------------------------------

type analysisSummary struct {
	TotalFrames    int
	FraudFrames    int
	MismatchFrames int
	SpoofFrames    int
	ObjectFrames   int
	MaxFraudConf   float32
	Verdict        string
}

func (s *analysisSummary) updateFromResult(r *inference.FrameResult) {
	isFraud := false

	for _, face := range r.Faces {
		if face.Similarity > 0 && face.Similarity < 0.6 {
			s.MismatchFrames++
			isFraud = true
			if face.Confidence > s.MaxFraudConf {
				s.MaxFraudConf = face.Confidence
			}
		}
		if face.IsSpoof {
			s.SpoofFrames++
			isFraud = true
			if face.Confidence > s.MaxFraudConf {
				s.MaxFraudConf = face.Confidence
			}
		}
	}

	if len(r.Objects) > 0 {
		s.ObjectFrames++
		for _, obj := range r.Objects {
			if obj.ObjectType == "phone" || obj.ObjectType == "screen_reflection" {
				isFraud = true
				if obj.Confidence > s.MaxFraudConf {
					s.MaxFraudConf = obj.Confidence
				}
			}
		}
	}

	if isFraud {
		s.FraudFrames++
	}
}

// ---------------------------------------------------------------------------
// Proto mapping helpers
// ---------------------------------------------------------------------------

func mapFrameResult(r *inference.FrameResult) *inferencepb.AnalyzeFrameResponse {
	return &inferencepb.AnalyzeFrameResponse{
		Faces:            mapFaceDetections(r.Faces),
		Objects:          mapObjectDetections(r.Objects),
		Liveness:         mapLivenessResult(r.Liveness),
		ProcessingTimeMs: r.LatencyMs,
	}
}

func mapFaceDetections(faces []inference.FaceResult) []*inferencepb.FaceDetection {
	if len(faces) == 0 {
		return nil
	}
	out := make([]*inferencepb.FaceDetection, len(faces))
	for i, f := range faces {
		out[i] = &inferencepb.FaceDetection{
			Confidence: f.Confidence,
			BboxX:      f.BBox.X,
			BboxY:      f.BBox.Y,
			BboxW:      f.BBox.W,
			BboxH:      f.BBox.H,
			Embedding:  f.Embedding,
			Similarity: f.Similarity,
			IsSpoof:    f.IsSpoof,
			SpoofType:  f.SpoofType,
			HeadYaw:    f.HeadYaw,
			HeadPitch:  f.HeadPitch,
			HeadRoll:   f.HeadRoll,
		}
	}
	return out
}

func mapObjectDetections(objects []inference.ObjectResult) []*inferencepb.ObjectDetection {
	if len(objects) == 0 {
		return nil
	}
	out := make([]*inferencepb.ObjectDetection, len(objects))
	for i, o := range objects {
		out[i] = &inferencepb.ObjectDetection{
			ObjectType: o.ObjectType,
			Confidence: o.Confidence,
			BboxX:      o.BBox.X,
			BboxY:      o.BBox.Y,
			BboxW:      o.BBox.W,
			BboxH:      o.BBox.H,
		}
	}
	return out
}

func mapLivenessResult(l inference.LivenessResult) *inferencepb.LivenessResult {
	return &inferencepb.LivenessResult{
		Score:  l.Score,
		IsLive: l.IsLive,
		Method: l.Method,
	}
}

func mapAnalysisSummary(s *analysisSummary) *inferencepb.AnalysisSummary {
	if s == nil {
		return &inferencepb.AnalysisSummary{Verdict: "clean"}
	}
	return &inferencepb.AnalysisSummary{
		TotalFramesAnalyzed:   int32(s.TotalFrames),
		FraudFrames:           int32(s.FraudFrames),
		FaceMismatchFrames:    int32(s.MismatchFrames),
		SpoofFrames:           int32(s.SpoofFrames),
		ObjectDetectionFrames: int32(s.ObjectFrames),
		MaxFraudConfidence:    s.MaxFraudConf,
		Verdict:               s.Verdict,
	}
}
