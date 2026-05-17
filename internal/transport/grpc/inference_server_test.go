package grpc

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
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
