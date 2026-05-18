package grpc

import (
	"bytes"
	"context"
	"io"
	"strings"
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

// AnalyzeVideo accepts a streamed, pre-extracted image frame and returns analysis.
//
// Raw video segment decoding is intentionally not performed here. Full video
// extraction must happen in a bounded background worker that samples frames by
// interval or explicit event trigger.
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

		// Capture metadata from the first data chunk.
		if sessionID == "" && chunk.SessionId != "" {
			sessionID = chunk.SessionId
			orgID = chunk.OrgId
			contentType = chunk.ContentType
			if !isSupportedStreamedFrameContentType(contentType) {
				return status.Errorf(
					codes.Unimplemented,
					"AnalyzeVideo accepts pre-extracted image frames only; got content_type %q",
					contentType,
				)
			}
		}

		if len(chunk.Data) > 0 {
			if sessionID == "" {
				return status.Error(codes.InvalidArgument, "session_id is required on the first data chunk")
			}
			if buf.Len()+len(chunk.Data) > s.maxFrameBytes {
				return status.Errorf(codes.InvalidArgument, "streamed frame data exceeds maximum size (%d bytes)", s.maxFrameBytes)
			}
			buf.Write(chunk.Data)
		}

		if chunk.IsLast {
			break
		}
	}

	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "session_id is required (set on first chunk)")
	}
	if buf.Len() == 0 {
		return status.Error(codes.InvalidArgument, "no frame data received")
	}

	s.logger.Info("inference: analyzing streamed image frame",
		zap.String("session_id", sessionID),
		zap.String("org_id", orgID),
		zap.String("content_type", contentType),
		zap.Int("total_bytes", buf.Len()),
	)

	frames, summary, err := s.analyzeStreamedImageFrame(stream.Context(), buf.Bytes(), contentType)
	if err != nil {
		return err
	}

	totalTime := float64(time.Since(start).Microseconds()) / 1000.0

	s.logger.Info("inference: streamed image analysis complete",
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

func (s *InferenceServer) analyzeStreamedImageFrame(ctx context.Context, data []byte, contentType string) ([]*inferencepb.FrameAnalysis, *analysisSummary, error) {
	release, ok := s.tryAcquireFrameSlot()
	if !ok {
		s.logger.Warn("inference: streamed frame dropped because worker pool is full")
		return nil, nil, status.Error(codes.ResourceExhausted, "inference worker pool is full; frame dropped")
	}
	defer release()

	result, err := s.engine.AnalyzeFrame(ctx, data, contentType)
	if err != nil {
		s.logger.Error("inference: streamed frame analysis failed", zap.Error(err))
		return nil, nil, status.Errorf(codes.Internal, "streamed frame analysis failed: %v", err)
	}

	summary := &analysisSummary{TotalFrames: 1}
	summary.updateFromResult(result)
	summary.finalize()

	return []*inferencepb.FrameAnalysis{
		{
			TimestampSec: 0,
			Faces:        mapFaceDetections(result.Faces),
			Objects:      mapObjectDetections(result.Objects),
			Liveness:     mapLivenessResult(result.Liveness),
		},
	}, summary, nil
}

func isSupportedStreamedFrameContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch mediaType {
	case "image/jpeg", "image/jpg", "image/png", "image/webp":
		return true
	default:
		return false
	}
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

func (s *analysisSummary) finalize() {
	if s.TotalFrames == 0 {
		s.Verdict = "clean"
		return
	}

	fraudRate := float64(s.FraudFrames) / float64(s.TotalFrames)
	switch {
	case fraudRate > 0.15 || s.MaxFraudConf > 0.9:
		s.Verdict = "fraud"
	case fraudRate > 0.05 || s.MaxFraudConf > 0.7:
		s.Verdict = "suspicious"
	default:
		s.Verdict = "clean"
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
