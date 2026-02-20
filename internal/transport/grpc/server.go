package grpc

import (
	"context"
	"io"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/argus-ai/event-collector/api/proto/v1"
	"github.com/argus-ai/event-collector/internal/application/usecase"
	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// ---------------------------------------------------------------------------
//  gRPC Server — EventCollectorServiceServer Implementation
//
//  This is the primary inbound adapter in the Clean Architecture. It sits
//  at the outermost ring and translates gRPC wire protocol into domain
//  operations via the IngestUseCase.
//
//  Concurrency model:
//    - IngestEvent / IngestBatch: One goroutine per RPC (standard gRPC model).
//    - StreamEvents: One goroutine per stream, but actual ingestion work is
//      delegated to the WorkerPool to bound total concurrency.
//    - Heartbeat: Stateless, no concurrency concerns.
// ---------------------------------------------------------------------------

// Server implements the pb.EventCollectorServiceServer interface.
// It delegates all business logic to the application-layer IngestUseCase
// and uses a WorkerPool for bounded-concurrency stream processing.
type Server struct {
	// Embed the unimplemented server to ensure forward compatibility.
	// When new RPCs are added to the proto, the service will return
	// "Unimplemented" rather than failing to compile.
	pb.UnimplementedEventCollectorServiceServer

	ingest     *usecase.IngestUseCase
	workerPool *WorkerPool
	logger     *zap.Logger
}

// NewServer creates a new gRPC server with injected dependencies.
// All dependencies are provided explicitly — no globals, no init().
func NewServer(
	ingest *usecase.IngestUseCase,
	pool *WorkerPool,
	logger *zap.Logger,
) *Server {
	return &Server{
		ingest:     ingest,
		workerPool: pool,
		logger:     logger.Named("grpc_server"),
	}
}

// ---------------------------------------------------------------------------
//  IngestEvent — Unary RPC
//
//  Accepts a single proctoring event from a browser client. This is the
//  preferred path for critical, low-latency events (face mismatch, phone
//  detected) where the client needs an immediate acknowledgement.
// ---------------------------------------------------------------------------

// IngestEvent handles a single event ingestion request.
func (s *Server) IngestEvent(
	ctx context.Context,
	req *pb.IngestEventRequest,
) (*pb.IngestEventResponse, error) {
	if req.GetEvent() == nil {
		return nil, status.Error(codes.InvalidArgument, "event is required")
	}

	// Extract the client IP from the gRPC peer metadata.
	peerAddr := extractPeerAddress(ctx)

	// Map from proto to domain entity.
	domainEvent := ProtoEventToDomain(req.GetEvent(), peerAddr)

	// Execute the domain use case.
	result := s.ingest.Ingest(ctx, domainEvent)

	if result.Error != nil {
		s.logger.Warn("event rejected",
			zap.String("event_id", result.EventID),
			zap.Error(result.Error),
		)
		return &pb.IngestEventResponse{
			Accepted: false,
			EventId:  result.EventID,
			Sequence: 0,
		}, nil
	}

	return &pb.IngestEventResponse{
		Accepted: true,
		EventId:  result.EventID,
		Sequence: result.Sequence,
	}, nil
}

// ---------------------------------------------------------------------------
//  IngestBatch — Unary RPC
//
//  Accepts a batch of events from a browser client. This is the preferred
//  path for high-frequency telemetry (gaze tracking, mouse movements) where
//  amortizing gRPC overhead across N events significantly improves throughput.
// ---------------------------------------------------------------------------

// IngestBatch handles a batch event ingestion request.
func (s *Server) IngestBatch(
	ctx context.Context,
	req *pb.IngestBatchRequest,
) (*pb.IngestBatchResponse, error) {
	events := req.GetEvents()
	if len(events) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one event is required")
	}

	peerAddr := extractPeerAddress(ctx)

	// Map all proto events to domain entities.
	domainEvents := make([]*entity.ProctoringEvent, 0, len(events))
	for _, pbEvent := range events {
		domainEvents = append(domainEvents, ProtoEventToDomain(pbEvent, peerAddr))
	}

	// Execute the batch use case.
	result, err := s.ingest.IngestBatch(ctx, domainEvents)
	if err != nil {
		s.logger.Error("batch ingestion failed",
			zap.String("batch_id", req.GetBatchId()),
			zap.Int("event_count", len(events)),
			zap.Error(err),
		)
		return nil, status.Errorf(codes.Internal, "batch ingestion failed: %v", err)
	}

	return &pb.IngestBatchResponse{
		AcceptedCount:    result.AcceptedCount,
		RejectedCount:    result.RejectedCount,
		RejectedEventIds: result.RejectedEventIDs,
		BatchSequence:    result.BatchSequence,
	}, nil
}

// ---------------------------------------------------------------------------
//  StreamEvents — Bidirectional Streaming RPC
//
//  Opens a persistent bidirectional stream for continuous event ingestion.
//  Used by the browser extension for always-on telemetry with minimal
//  connection overhead.
//
//  Flow:
//    Client --> [ProctoringEvent] --> Server
//    Client <-- [StreamAck]       <-- Server
//
//  The server reads events from the client stream, submits them to the
//  WorkerPool for bounded-concurrency processing, and sends acknowledgements
//  back to the client as results become available.
//
//  Back-pressure: If the worker pool is saturated, the stream handler blocks
//  on Submit(), which naturally slows down the Recv() loop and applies
//  TCP-level flow control back to the client.
// ---------------------------------------------------------------------------

// StreamEvents handles a bidirectional streaming ingestion session.
func (s *Server) StreamEvents(
	stream pb.EventCollectorService_StreamEventsServer,
) error {
	ctx := stream.Context()
	peerAddr := extractPeerAddress(ctx)

	s.logger.Debug("stream opened",
		zap.String("peer", peerAddr),
	)

	for {
		// Read the next event from the client stream.
		req, err := stream.Recv()
		if err == io.EOF {
			// Client closed its send side. Normal stream termination.
			s.logger.Debug("stream closed by client",
				zap.String("peer", peerAddr),
			)
			return nil
		}
		if err != nil {
			// Transport error or client disconnect.
			s.logger.Warn("stream recv error",
				zap.String("peer", peerAddr),
				zap.Error(err),
			)
			return status.Errorf(codes.Internal, "stream recv failed: %v", err)
		}

		pbEvent := req.GetEvent()
		if pbEvent == nil {
			// Skip malformed requests rather than terminating the stream.
			s.logger.Debug("received empty event on stream, skipping",
				zap.String("peer", peerAddr),
			)
			continue
		}

		// Map proto to domain.
		domainEvent := ProtoEventToDomain(pbEvent, peerAddr)

		// Create a buffered result channel so the worker never blocks on send.
		resultCh := make(chan *usecase.IngestResult, 1)

		// Submit to the worker pool. This blocks if the pool is saturated,
		// which naturally back-pressures the client stream.
		if err := s.workerPool.Submit(ctx, Job{
			Event:    domainEvent,
			ResultCh: resultCh,
		}); err != nil {
			// Context cancelled — client disconnected or server shutting down.
			return status.Errorf(codes.Unavailable, "worker pool submit failed: %v", err)
		}

		// Wait for the ingestion result from the worker.
		select {
		case result := <-resultCh:
			ack := &pb.StreamAck{
				EventId:  result.EventID,
				Sequence: result.Sequence,
				Accepted: result.Accepted,
			}
			if err := stream.Send(ack); err != nil {
				s.logger.Warn("stream send error",
					zap.String("event_id", result.EventID),
					zap.Error(err),
				)
				return status.Errorf(codes.Internal, "stream send failed: %v", err)
			}
		case <-ctx.Done():
			return status.Errorf(codes.Canceled, "stream context cancelled: %v", ctx.Err())
		}
	}
}

// ---------------------------------------------------------------------------
//  Heartbeat — Unary RPC
//
//  Keeps a proctoring session alive and allows the server to issue dynamic
//  directives (increase telemetry frequency, terminate session, etc.).
//  The browser client calls this every 5-10 seconds.
// ---------------------------------------------------------------------------

// Heartbeat responds to a session keepalive request.
func (s *Server) Heartbeat(
	ctx context.Context,
	req *pb.HeartbeatRequest,
) (*pb.HeartbeatResponse, error) {
	if req.GetSessionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	// For now, return a simple "session active" response with the server
	// timestamp. In production, this would query a session store to check
	// whether the exam is still running and whether any server-side
	// directives need to be pushed to the client.
	//
	// TODO: Integrate with session management service for real-time
	// directive computation (escalate telemetry, force terminate, etc.).
	resp := &pb.HeartbeatResponse{
		SessionActive:   true,
		ServerTimestamp: timestamppb.New(time.Now().UTC()),
		Directive: &pb.SessionDirective{
			TelemetryMode:   pb.TelemetryMode_TELEMETRY_NORMAL,
			Terminate:       false,
			TerminateReason: "",
		},
	}

	s.logger.Debug("heartbeat",
		zap.String("session_id", req.GetSessionId()),
		zap.String("student_id", req.GetStudentId()),
		zap.Float32("focus_score", req.GetCurrentFocusScore()),
		zap.Int32("violations", req.GetViolationCount()),
	)

	return resp, nil
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

// extractPeerAddress retrieves the client IP address from the gRPC peer
// context. This is set by the gRPC transport layer and represents the
// actual TCP connection source address (or the X-Forwarded-For header
// if behind a load balancer, depending on server configuration).
//
// Returns an empty string if peer information is not available.
func extractPeerAddress(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}
	return p.Addr.String()
}
