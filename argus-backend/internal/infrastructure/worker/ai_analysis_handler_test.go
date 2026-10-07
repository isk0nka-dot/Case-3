package worker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"google.golang.org/grpc"

	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
)

func TestAIAnalysisEvidenceContentTypeClassification(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		supported   bool
	}{
		{name: "jpeg", contentType: "image/jpeg", supported: true},
		{name: "jpeg with parameters", contentType: "image/jpeg; charset=binary", supported: true},
		{name: "png", contentType: "image/png", supported: true},
		{name: "webp", contentType: "image/webp", supported: true},
		{name: "mp4", contentType: "video/mp4", supported: false},
		{name: "webm", contentType: "video/webm", supported: false},
		{name: "empty", contentType: "", supported: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSupportedEvidenceFrameContentType(tt.contentType); got != tt.supported {
				t.Fatalf("isSupportedEvidenceFrameContentType(%q)=%v, want %v", tt.contentType, got, tt.supported)
			}
		})
	}
}

func TestFFmpegFrameExtractorBuildsBoundedCommand(t *testing.T) {
	extractor := NewFFmpegFrameExtractor(FrameExtractionConfig{
		FFmpegPath:     "ffmpeg",
		IntervalSec:    4,
		MaxVideoDurSec: 20,
	})

	args := strings.Join(extractor.ffmpegArgs("input.webm", "frames"), " ")
	for _, expected := range []string{
		"-nostdin",
		"-vf fps=1/4",
		"-frames:v 5",
		"-q:v 3",
	} {
		if !strings.Contains(args, expected) {
			t.Fatalf("expected ffmpeg args to contain %q, got %q", expected, args)
		}
	}
}

func TestAIAnalysisRejectsUnknownEvidenceBeforeStorageDownload(t *testing.T) {
	handler := &AIAnalysisHandler{}

	_, err := handler.analyzeFragment(
		context.Background(),
		nil,
		AIAnalysisPayload{SessionID: "session-1", OrgID: "org-1"},
		evidenceFragment{ObjectKey: "recordings/session-1.bin", ContentType: "application/octet-stream"},
	)

	if !errors.Is(err, errUnsupportedEvidenceContentType) {
		t.Fatalf("expected unsupported content type error, got %v", err)
	}
}

type fakeFrameExtractor struct {
	contentType string
	frames      []evidenceFrame
}

func (f *fakeFrameExtractor) ExtractFrames(_ context.Context, _ io.Reader, contentType string) ([]evidenceFrame, error) {
	f.contentType = contentType
	return f.frames, nil
}

type fakeInferenceClient struct {
	requests []*inferencepb.AnalyzeFrameRequest
}

func (c *fakeInferenceClient) AnalyzeFrame(_ context.Context, req *inferencepb.AnalyzeFrameRequest, _ ...grpc.CallOption) (*inferencepb.AnalyzeFrameResponse, error) {
	c.requests = append(c.requests, req)
	return &inferencepb.AnalyzeFrameResponse{
		Objects: []*inferencepb.ObjectDetection{
			{ObjectType: "phone", Confidence: 0.91},
		},
		ProcessingTimeMs: 3,
	}, nil
}

func (c *fakeInferenceClient) AnalyzeVideo(context.Context, ...grpc.CallOption) (grpc.ClientStreamingClient[inferencepb.AnalyzeVideoChunk, inferencepb.AnalyzeVideoResponse], error) {
	return nil, errors.New("AnalyzeVideo should not be used by AIAnalysisHandler")
}

func TestAIAnalysisExtractsVideoFramesAndSendsAnalyzeFrameRequests(t *testing.T) {
	extractor := &fakeFrameExtractor{
		frames: []evidenceFrame{
			{Data: []byte("jpeg-1"), ContentType: "image/jpeg", TimestampSec: 0},
			{Data: []byte("jpeg-2"), ContentType: "image/jpeg", TimestampSec: 4},
		},
	}
	client := &fakeInferenceClient{}
	handler := &AIAnalysisHandler{
		frameExtractor: extractor,
		thresholds: AIAnalysisThresholds{
			ObjectConfidence: 0.35,
		},
	}

	resp, err := handler.analyzeVideoEvidence(
		context.Background(),
		client,
		AIAnalysisPayload{SessionID: "session-1", OrgID: "org-1", ExamID: "exam-1", StudentID: "student-1"},
		evidenceFragment{ObjectKey: "recordings/session-1.webm", ContentType: "video/webm"},
		strings.NewReader("video-bytes"),
	)
	if err != nil {
		t.Fatalf("analyzeVideoEvidence returned error: %v", err)
	}

	if extractor.contentType != "video/webm" {
		t.Fatalf("expected extractor to receive video/webm, got %q", extractor.contentType)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected two AnalyzeFrame requests, got %d", len(client.requests))
	}
	if client.requests[1].VideoTimestampSec != 4 {
		t.Fatalf("expected second frame timestamp 4s, got %.2f", client.requests[1].VideoTimestampSec)
	}
	if len(resp.Frames) != 2 {
		t.Fatalf("expected two frame responses, got %d", len(resp.Frames))
	}
	if resp.Summary.GetTotalFramesAnalyzed() != 2 {
		t.Fatalf("expected summary to count two frames, got %d", resp.Summary.GetTotalFramesAnalyzed())
	}
	if resp.Summary.GetObjectDetectionFrames() != 2 {
		t.Fatalf("expected two object frames, got %d", resp.Summary.GetObjectDetectionFrames())
	}
}

func TestAIAnalysisSendsReferenceEmbeddingWithExtractedFrames(t *testing.T) {
	extractor := &fakeFrameExtractor{
		frames: []evidenceFrame{
			{Data: []byte("jpeg-1"), ContentType: "image/jpeg", TimestampSec: 0},
		},
	}
	client := &fakeInferenceClient{}
	handler := &AIAnalysisHandler{
		frameExtractor: extractor,
		thresholds: AIAnalysisThresholds{
			ObjectConfidence: 0.35,
		},
	}

	_, err := handler.analyzeVideoEvidence(
		context.Background(),
		client,
		AIAnalysisPayload{
			SessionID:          "session-1",
			OrgID:              "org-1",
			ExamID:             "exam-1",
			StudentID:          "student-1",
			ReferenceEmbedding: []float32{0.11, 0.22, 0.33},
		},
		evidenceFragment{ObjectKey: "recordings/session-1.webm", ContentType: "video/webm"},
		strings.NewReader("video-bytes"),
	)
	if err != nil {
		t.Fatalf("analyzeVideoEvidence returned error: %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("expected one AnalyzeFrame request, got %d", len(client.requests))
	}
	if got, want := client.requests[0].ReferenceEmbedding, []float32{0.11, 0.22, 0.33}; len(got) != len(want) {
		t.Fatalf("reference embedding length=%d, want %d", len(got), len(want))
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("reference embedding[%d]=%v, want %v", i, got[i], want[i])
			}
		}
	}
}

func TestAIAnalysisSendsImageEvidenceViaAnalyzeFrame(t *testing.T) {
	client := &fakeInferenceClient{}
	handler := &AIAnalysisHandler{
		thresholds: AIAnalysisThresholds{
			ObjectConfidence: 0.35,
		},
	}

	resp, err := handler.analyzeImageEvidence(
		context.Background(),
		client,
		AIAnalysisPayload{
			SessionID:          "session-1",
			OrgID:              "org-1",
			ExamID:             "exam-1",
			StudentID:          "student-1",
			ReferenceEmbedding: []float32{0.11, 0.22},
		},
		evidenceFragment{ObjectKey: "recordings/session-1.jpg", ContentType: "image/jpeg"},
		strings.NewReader("jpeg-bytes"),
	)
	if err != nil {
		t.Fatalf("analyzeImageEvidence returned error: %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("expected one AnalyzeFrame request, got %d", len(client.requests))
	}
	req := client.requests[0]
	if string(req.FrameData) != "jpeg-bytes" {
		t.Fatalf("unexpected frame data %q", string(req.FrameData))
	}
	if req.ContentType != "image/jpeg" {
		t.Fatalf("expected image/jpeg content type, got %q", req.ContentType)
	}
	if req.SessionId != "session-1" || req.StudentId != "student-1" || req.ExamId != "exam-1" || req.OrgId != "org-1" {
		t.Fatalf("request metadata was not preserved: %#v", req)
	}
	if got, want := req.ReferenceEmbedding, []float32{0.11, 0.22}; len(got) != len(want) {
		t.Fatalf("reference embedding length=%d, want %d", len(got), len(want))
	}
	if len(resp.Frames) != 1 {
		t.Fatalf("expected one frame response, got %d", len(resp.Frames))
	}
	if resp.Summary.GetObjectDetectionFrames() != 1 {
		t.Fatalf("expected image object finding in summary, got %d", resp.Summary.GetObjectDetectionFrames())
	}
}

func TestAIAnalysisRejectsOversizedImageEvidenceBeforeAnalyzeFrame(t *testing.T) {
	client := &fakeInferenceClient{}
	handler := &AIAnalysisHandler{}
	handler.ConfigureMaxFrameBytes(4)

	_, err := handler.analyzeImageEvidence(
		context.Background(),
		client,
		AIAnalysisPayload{SessionID: "session-1", OrgID: "org-1"},
		evidenceFragment{ObjectKey: "recordings/session-1.jpg", ContentType: "image/jpeg"},
		strings.NewReader("12345"),
	)
	if err == nil {
		t.Fatal("expected oversized image evidence error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum frame size") {
		t.Fatalf("expected maximum frame size error, got %v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("expected no AnalyzeFrame requests for oversized image, got %d", len(client.requests))
	}
}
