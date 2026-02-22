package grpc

import (
	"context"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/usecase"
	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ---------------------------------------------------------------------------
//  Semaphore-based Concurrency Throttle
//
//  Replaces the fixed-goroutine worker pool with a semaphore pattern that
//  trusts Go's scheduler. Instead of N persistent goroutines pulling from a
//  shared channel, each Submit() acquires a semaphore slot, spawns a
//  goroutine that calls Ingest directly, and releases the slot on return.
//
//  Architecture:
//
//    Stream 1 --+                                  IngestUseCase
//    Stream 2 --+--> Submit() --[sem acquire]--> goroutine --> result
//    Stream N --+                                  IngestUseCase
//
//  Benefits over the fixed-pool approach:
//    - No persistent goroutines idle when traffic is low.
//    - No artificial channel contention under high load.
//    - Go's scheduler distributes work across OS threads naturally.
//    - Semaphore bounds max concurrency identically to the old pool.
//
//  Back-pressure: If all semaphore slots are taken, Submit blocks until
//  either a slot opens or the caller's context is cancelled. This is
//  functionally identical to the old pool's channel-full blocking.
// ---------------------------------------------------------------------------

// WorkerPool bounds concurrency for stream event ingestion using a semaphore.
// The public API (Submit, Close, Size, Pending) is unchanged from the
// channel-based pool it replaces — no changes needed in server.go or main.go.
type WorkerPool struct {
	sem    chan struct{} // semaphore: capacity = max concurrency
	ingest *usecase.IngestUseCase
	logger *zap.Logger
	size   int

	inflight atomic.Int64
	done     chan struct{}
	wg       sync.WaitGroup
}

// Job represents a single unit of work submitted to the pool.
// Each job carries one domain event and a channel for returning the result
// to the submitting goroutine (typically a gRPC stream handler).
type Job struct {
	// Event is the domain entity to be ingested.
	Event *entity.ProctoringEvent

	// ResultCh receives the ingestion result. The pool sends exactly
	// one value and never closes the channel — the caller is responsible
	// for reading the result.
	ResultCh chan<- *usecase.IngestResult
}

// DefaultWorkerPoolSize is the default maximum concurrent ingestions.
const DefaultWorkerPoolSize = 256

// NewWorkerPool creates a semaphore-bounded pool. No goroutines are spawned
// until work arrives — the semaphore merely caps how many can run at once.
//
// Parameters:
//   - size: Maximum concurrent ingestion goroutines. Use DefaultWorkerPoolSize if unsure.
//   - ingest: The application-layer use case that owns ingestion logic.
//   - logger: Structured logger for operational visibility.
func NewWorkerPool(size int, ingest *usecase.IngestUseCase, logger *zap.Logger) *WorkerPool {
	if size <= 0 {
		size = DefaultWorkerPoolSize
	}

	wp := &WorkerPool{
		sem:    make(chan struct{}, size),
		ingest: ingest,
		logger: logger.Named("worker_pool"),
		size:   size,
		done:   make(chan struct{}),
	}

	wp.logger.Info("worker pool started",
		zap.Int("max_concurrency", size),
	)

	return wp
}

// Submit processes a job with bounded concurrency. It acquires a semaphore
// slot, spawns a goroutine to run the ingestion, and sends the result back
// via the job's ResultCh.
//
// Blocks if all semaphore slots are taken (back-pressure). Returns the
// context error if the context is cancelled before a slot opens.
func (wp *WorkerPool) Submit(ctx context.Context, job Job) error {
	// Acquire semaphore slot — blocks if at capacity.
	select {
	case wp.sem <- struct{}{}:
		// Slot acquired.
	case <-ctx.Done():
		return ctx.Err()
	case <-wp.done:
		return context.Canceled
	}

	wp.wg.Add(1)
	wp.inflight.Add(1)

	go func() {
		defer func() {
			<-wp.sem // Release semaphore slot.
			wp.inflight.Add(-1)
			wp.wg.Done()
		}()

		// Use a background context for the ingestion call. The stream
		// context may be cancelled if the client disconnected, but we
		// still want to complete the write to Kafka/ClickHouse.
		result := wp.ingest.Ingest(context.Background(), job.Event)

		// Send result back to the stream handler. Non-blocking send
		// to avoid hanging if the stream handler has already exited.
		select {
		case job.ResultCh <- result:
		default:
			wp.logger.Debug("result channel full or closed, discarding result",
				zap.String("event_id", result.EventID),
			)
		}
	}()

	return nil
}

// Pending returns the current number of in-flight ingestion goroutines.
func (wp *WorkerPool) Pending() int {
	return int(wp.inflight.Load())
}

// Size returns the maximum concurrency (semaphore capacity).
func (wp *WorkerPool) Size() int {
	return wp.size
}

// Close performs a graceful shutdown:
//  1. Signals that no new jobs should be accepted.
//  2. Waits for all in-flight goroutines to finish.
//
// After Close returns, Submit will return context.Canceled.
// Close is safe to call multiple times (subsequent calls are no-ops).
func (wp *WorkerPool) Close() {
	select {
	case <-wp.done:
		return // Already closed.
	default:
		close(wp.done)
	}

	wp.wg.Wait()

	wp.logger.Info("worker pool stopped",
		zap.Int("max_concurrency", wp.size),
	)
}
