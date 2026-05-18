package grpc

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
	"github.com/argus-ai/event-collector/internal/infrastructure/inference"
)

type blockingInferenceEngine struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func newBlockingInferenceEngine() *blockingInferenceEngine {
	return &blockingInferenceEngine{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
}

func (e *blockingInferenceEngine) AnalyzeFrame(ctx context.Context, _ []byte, _ string) (*inference.FrameResult, error) {
	e.calls.Add(1)
	e.entered <- struct{}{}

	select {
	case <-e.release:
		return &inference.FrameResult{LatencyMs: 1}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *blockingInferenceEngine) Name() string { return "blocking" }
func (e *blockingInferenceEngine) Close() error { return nil }

type countingInferenceEngine struct {
	calls atomic.Int32
}

func (e *countingInferenceEngine) AnalyzeFrame(context.Context, []byte, string) (*inference.FrameResult, error) {
	e.calls.Add(1)
	return &inference.FrameResult{LatencyMs: 1}, nil
}

func (e *countingInferenceEngine) Name() string { return "counting" }
func (e *countingInferenceEngine) Close() error { return nil }

type fakeAnalyzeVideoStream struct {
	ctx      context.Context
	chunks   []*inferencepb.AnalyzeVideoChunk
	index    int
	response *inferencepb.AnalyzeVideoResponse
}

func (s *fakeAnalyzeVideoStream) Recv() (*inferencepb.AnalyzeVideoChunk, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := s.chunks[s.index]
	s.index++
	return chunk, nil
}

func (s *fakeAnalyzeVideoStream) SendAndClose(resp *inferencepb.AnalyzeVideoResponse) error {
	s.response = resp
	return nil
}

func (s *fakeAnalyzeVideoStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeAnalyzeVideoStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeAnalyzeVideoStream) SetTrailer(metadata.MD)       {}

func (s *fakeAnalyzeVideoStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

func (s *fakeAnalyzeVideoStream) SendMsg(any) error { return nil }
func (s *fakeAnalyzeVideoStream) RecvMsg(any) error { return nil }

func TestInferenceServerRejectsFramesWhenWorkerPoolIsFull(t *testing.T) {
	engine := newBlockingInferenceEngine()
	server := NewInferenceServerWithOptions(engine, zap.NewNop(), InferenceServerOptions{
		MaxFrameBytes:       1024,
		MaxConcurrentFrames: 1,
	})

	req := &inferencepb.AnalyzeFrameRequest{
		SessionId:   "session-1",
		FrameData:   []byte("jpeg"),
		ContentType: "image/jpeg",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := server.AnalyzeFrame(ctx, req)
		done <- err
	}()

	select {
	case <-engine.entered:
	case <-time.After(time.Second):
		t.Fatal("first analysis did not enter engine")
	}

	_, err := server.AnalyzeFrame(context.Background(), req)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("expected ResourceExhausted when inference pool is full, got %v (%v)", status.Code(err), err)
	}

	close(engine.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("first analysis returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("first analysis did not complete after release")
	}

	if got := engine.calls.Load(); got != 1 {
		t.Fatalf("expected saturated request to be dropped before engine call, calls=%d", got)
	}
}

func TestInferenceServerRejectsRawVideoSegments(t *testing.T) {
	engine := &countingInferenceEngine{}
	server := NewInferenceServerWithOptions(engine, zap.NewNop(), InferenceServerOptions{
		MaxFrameBytes:       1024 * 1024,
		MaxConcurrentFrames: 1,
	})

	stream := &fakeAnalyzeVideoStream{
		ctx: context.Background(),
		chunks: []*inferencepb.AnalyzeVideoChunk{
			{
				SessionId:   "session-1",
				OrgId:       "org-1",
				ContentType: "video/mp4",
				Data:        []byte("not-an-image-frame"),
				IsLast:      true,
			},
		},
	}

	err := server.AnalyzeVideo(stream)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected Unimplemented for raw video segments, got %v (%v)", status.Code(err), err)
	}
	if stream.response != nil {
		t.Fatalf("expected no response for rejected raw video segment, got %#v", stream.response)
	}
	if got := engine.calls.Load(); got != 0 {
		t.Fatalf("expected raw video rejection before engine call, calls=%d", got)
	}
}

func TestInferenceServerAnalyzesPreExtractedImageFrameStream(t *testing.T) {
	engine := &countingInferenceEngine{}
	server := NewInferenceServerWithOptions(engine, zap.NewNop(), InferenceServerOptions{
		MaxFrameBytes:       1024 * 1024,
		MaxConcurrentFrames: 1,
	})

	stream := &fakeAnalyzeVideoStream{
		ctx: context.Background(),
		chunks: []*inferencepb.AnalyzeVideoChunk{
			{
				SessionId:   "session-1",
				OrgId:       "org-1",
				ContentType: "image/jpeg",
				Data:        []byte("jpeg-part-1"),
			},
			{
				Data:   []byte("jpeg-part-2"),
				IsLast: true,
			},
		},
	}

	if err := server.AnalyzeVideo(stream); err != nil {
		t.Fatalf("AnalyzeVideo returned error for image frame stream: %v", err)
	}
	if stream.response == nil {
		t.Fatal("expected response for image frame stream")
	}
	if got := len(stream.response.Frames); got != 1 {
		t.Fatalf("expected one analyzed image frame, got %d", got)
	}
	if got := stream.response.Summary.GetTotalFramesAnalyzed(); got != 1 {
		t.Fatalf("expected summary to count one frame, got %d", got)
	}
	if got := engine.calls.Load(); got != 1 {
		t.Fatalf("expected one engine call for one streamed image frame, calls=%d", got)
	}
}
