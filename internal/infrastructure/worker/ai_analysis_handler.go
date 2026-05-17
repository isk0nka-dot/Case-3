package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/hibiken/asynq"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	inferencepb "github.com/argus-ai/event-collector/api/proto/v1/inferencepb"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/domain/valueobject"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/forensic"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"github.com/argus-ai/event-collector/pkg/randutil"
)

// AIAnalysisHandler processes TypeAIAnalysis asynq tasks.
// It streams session evidence to the inference gateway for GPU-accelerated
// analysis, writes detected anomalies back to ClickHouse as source=BACKEND_AI
// events, and fires CRITICAL Telegram alerts for fraud.
type AIAnalysisHandler struct {
	inferenceAddr string
	chConn        driver.Conn
	chWriter      *clickhouse.Writer
	scorer        *forensic.Scorer
	pgRepo        *postgres.Repository
	minioClient   *minio.Client
	minioBucket   string
	alerter       alerting.Provider
	logger        *zap.Logger
	thresholds    AIAnalysisThresholds
}

type AIAnalysisThresholds struct {
	FaceMismatch     float32
	Liveness         float32
	ObjectConfidence float32
	SpoofConfidence  float32
}

// NewAIAnalysisHandler creates a new asynq handler for AI deep scan jobs.
func NewAIAnalysisHandler(
	inferenceAddr string,
	chConn driver.Conn,
	chWriter *clickhouse.Writer,
	pgRepo *postgres.Repository,
	minioClient *minio.Client,
	minioBucket string,
	logger *zap.Logger,
	alerter alerting.Provider,
	thresholds ...AIAnalysisThresholds,
) *AIAnalysisHandler {
	return &AIAnalysisHandler{
		inferenceAddr: inferenceAddr,
		chConn:        chConn,
		chWriter:      chWriter,
		scorer:        forensic.NewScorer(chConn, logger),
		pgRepo:        pgRepo,
		minioClient:   minioClient,
		minioBucket:   minioBucket,
		alerter:       alerter,
		logger:        logger.Named("asynq_ai_analysis"),
		thresholds:    normalizeAIAnalysisThresholds(firstAIAnalysisThresholds(thresholds)),
	}
}

// ProcessTask implements the asynq.Handler interface.
func (h *AIAnalysisHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	var payload AIAnalysisPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal AIAnalysisPayload: %w", err)
	}

	h.logger.Info("processing AI analysis job",
		zap.String("session_id", payload.SessionID),
		zap.String("org_id", payload.OrgID),
		zap.String("exam_id", payload.ExamID),
		zap.String("analysis_type", payload.AnalysisType),
	)

	// ── Step 1: Connect to inference gateway ──────────────────────────────
	conn, err := grpc.NewClient(
		h.inferenceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect to inference gateway at %s: %w", h.inferenceAddr, err)
	}
	defer conn.Close()

	client := inferencepb.NewInferenceServiceClient(conn)

	h.logger.Info("connected to inference gateway",
		zap.String("addr", h.inferenceAddr),
	)

	// ── Step 2: Query evidence fragments from ClickHouse ──────────────────
	fragments, err := h.queryEvidenceFragments(ctx, payload.SessionID)
	if err != nil {
		return fmt.Errorf("query evidence fragments for %s: %w", payload.SessionID, err)
	}

	if len(fragments) == 0 {
		h.logger.Warn("no evidence fragments found for session, skipping analysis",
			zap.String("session_id", payload.SessionID),
		)
		NotifyJobCompleted(h.alerter, TypeAIAnalysis, payload.SessionID, time.Since(start))
		return nil
	}

	h.logger.Info("found evidence fragments",
		zap.String("session_id", payload.SessionID),
		zap.Int("count", len(fragments)),
	)

	// ── Step 3: Stream each fragment to inference gateway ──────────────────
	var allFrames []*inferencepb.FrameAnalysis
	var totalSummary aggregateSummary

	for _, frag := range fragments {
		resp, err := h.analyzeFragment(ctx, client, payload, frag)
		if err != nil {
			h.logger.Warn("failed to analyze fragment, skipping",
				zap.String("session_id", payload.SessionID),
				zap.String("object_key", frag.ObjectKey),
				zap.Error(err),
			)
			continue
		}

		allFrames = append(allFrames, resp.Frames...)
		totalSummary.merge(resp.Summary)
	}

	h.logger.Info("inference analysis complete",
		zap.String("session_id", payload.SessionID),
		zap.Int("total_frames", len(allFrames)),
		zap.String("verdict", totalSummary.verdict()),
	)

	// ── Step 4: Write detected anomalies back to ClickHouse ───────────────
	eventsWritten, err := h.writeBackAnomalies(ctx, payload, allFrames)
	if err != nil {
		h.logger.Error("failed to write anomalies to clickhouse",
			zap.String("session_id", payload.SessionID),
			zap.Error(err),
		)
	}

	h.logger.Info("anomalies written to clickhouse",
		zap.String("session_id", payload.SessionID),
		zap.Int("events_written", eventsWritten),
	)

	// ── Step 5: Critical fraud → Telegram alert + recompute score ─────────
	verdict := totalSummary.verdict()
	if verdict == "fraud" || totalSummary.maxFraudConf > 0.9 {
		h.fireFraudAlert(payload, totalSummary)

		// Recompute integrity score — the write-back events are now in ClickHouse.
		if h.pgRepo != nil {
			settings, settingsErr := h.pgRepo.GetExamProctoringSettings(ctx, payload.OrgID, payload.ExamID)
			if settingsErr == nil && settings != nil {
				_, scoreErr := h.scorer.ComputeScoreWithConfig(ctx, payload.SessionID, settings)
				if scoreErr != nil {
					h.logger.Warn("failed to recompute integrity score",
						zap.String("session_id", payload.SessionID),
						zap.Error(scoreErr),
					)
				}
			}
		}
	}

	duration := time.Since(start)

	h.logger.Info("AI analysis job completed",
		zap.String("session_id", payload.SessionID),
		zap.String("verdict", verdict),
		zap.Int("events_written", eventsWritten),
		zap.Duration("duration", duration),
	)

	NotifyJobCompleted(h.alerter, TypeAIAnalysis, payload.SessionID, duration)
	return nil
}

// ---------------------------------------------------------------------------
// Evidence fragment query
// ---------------------------------------------------------------------------

type evidenceFragment struct {
	ObjectKey   string
	ContentType string
	SizeBytes   int64
}

func (h *AIAnalysisHandler) queryEvidenceFragments(ctx context.Context, sessionID string) ([]evidenceFragment, error) {
	rows, err := h.chConn.Query(ctx,
		`SELECT object_key, content_type, size_bytes
		 FROM evidence_fragments
		 WHERE session_id = ?
		 ORDER BY fragment_index ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query evidence_fragments: %w", err)
	}
	defer rows.Close()

	var fragments []evidenceFragment
	for rows.Next() {
		var f evidenceFragment
		if err := rows.Scan(&f.ObjectKey, &f.ContentType, &f.SizeBytes); err != nil {
			return nil, fmt.Errorf("scan evidence fragment: %w", err)
		}
		fragments = append(fragments, f)
	}
	return fragments, rows.Err()
}

// ---------------------------------------------------------------------------
// Fragment analysis via inference gateway
// ---------------------------------------------------------------------------

func (h *AIAnalysisHandler) analyzeFragment(
	ctx context.Context,
	client inferencepb.InferenceServiceClient,
	payload AIAnalysisPayload,
	frag evidenceFragment,
) (*inferencepb.AnalyzeVideoResponse, error) {
	// Download fragment from MinIO.
	obj, err := h.minioClient.GetObject(ctx, h.minioBucket, frag.ObjectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object %s: %w", frag.ObjectKey, err)
	}
	defer obj.Close()

	// Stream to inference gateway via AnalyzeVideo client-streaming RPC.
	stream, err := client.AnalyzeVideo(ctx)
	if err != nil {
		return nil, fmt.Errorf("open AnalyzeVideo stream: %w", err)
	}

	const chunkSize = 64 * 1024 // 64 KB chunks
	buf := make([]byte, chunkSize)
	isFirst := true

	for {
		n, readErr := obj.Read(buf)
		if n > 0 {
			chunk := &inferencepb.AnalyzeVideoChunk{
				Data:        buf[:n],
				ContentType: frag.ContentType,
			}
			if isFirst {
				chunk.SessionId = payload.SessionID
				chunk.OrgId = payload.OrgID
				isFirst = false
			}
			if err := stream.Send(chunk); err != nil {
				return nil, fmt.Errorf("send chunk: %w", err)
			}
		}
		if readErr != nil {
			// Send final chunk marker.
			if err := stream.Send(&inferencepb.AnalyzeVideoChunk{IsLast: true}); err != nil {
				return nil, fmt.Errorf("send last chunk: %w", err)
			}
			break
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return nil, fmt.Errorf("close stream: %w", err)
	}

	return resp, nil
}

// ---------------------------------------------------------------------------
// Write-back anomalies to ClickHouse
// ---------------------------------------------------------------------------

func (h *AIAnalysisHandler) writeBackAnomalies(
	ctx context.Context,
	payload AIAnalysisPayload,
	frames []*inferencepb.FrameAnalysis,
) (int, error) {
	count := 0
	now := time.Now().UTC()

	for _, frame := range frames {
		for _, anomaly := range classifyFrameAnomalies(frame, h.thresholds) {
			evt := h.buildEvent(payload, now, frame.TimestampSec,
				anomaly.eventType, anomaly.severity, anomaly.label, anomaly.confidence,
			)
			if err := h.chWriter.Write(ctx, evt); err != nil {
				h.logger.Warn("failed to write AI anomaly event", zap.Error(err))
				continue
			}
			count++
		}
	}

	return count, nil
}

type detectedAIAnomaly struct {
	eventType  valueobject.EventType
	severity   valueobject.Severity
	label      string
	confidence float32
}

func classifyFrameAnomalies(frame *inferencepb.FrameAnalysis, thresholds AIAnalysisThresholds) []detectedAIAnomaly {
	if frame == nil {
		return nil
	}

	thresholds = normalizeAIAnalysisThresholds(thresholds)
	anomalies := make([]detectedAIAnomaly, 0, len(frame.Faces)+len(frame.Objects)+1)

	for _, face := range frame.Faces {
		if face.Similarity > 0 && face.Similarity < thresholds.FaceMismatch {
			anomalies = append(anomalies, detectedAIAnomaly{
				eventType:  valueobject.BackendAIFaceMismatch,
				severity:   valueobject.SeverityCritical,
				label:      fmt.Sprintf("Backend AI: face mismatch (similarity=%.2f)", face.Similarity),
				confidence: face.Confidence,
			})
		}

		if face.IsSpoof && face.Confidence >= thresholds.SpoofConfidence {
			label := fmt.Sprintf("Backend AI: spoof detected (type=%s)", face.SpoofType)
			evtType := valueobject.BackendAIFaceMismatch
			if face.SpoofType == "deepfake" {
				evtType = valueobject.BackendAIDeepfakeDetected
			}
			anomalies = append(anomalies, detectedAIAnomaly{
				eventType:  evtType,
				severity:   valueobject.SeverityCritical,
				label:      label,
				confidence: face.Confidence,
			})
		}
	}

	for _, obj := range frame.Objects {
		if obj.Confidence < thresholds.ObjectConfidence {
			continue
		}

		var evtType valueobject.EventType
		switch obj.ObjectType {
		case "phone", "book", "earbuds":
			evtType = valueobject.BackendAIHiddenObject
		case "screen_reflection", "person":
			evtType = valueobject.BackendAIScreenReflection
		default:
			evtType = valueobject.BackendAIHiddenObject
		}

		anomalies = append(anomalies, detectedAIAnomaly{
			eventType:  evtType,
			severity:   valueobject.SeverityWarning,
			label:      fmt.Sprintf("Backend AI: %s detected (conf=%.2f)", obj.ObjectType, obj.Confidence),
			confidence: obj.Confidence,
		})
	}

	if frame.Liveness != nil && !frame.Liveness.IsLive && frame.Liveness.Score < thresholds.Liveness {
		anomalies = append(anomalies, detectedAIAnomaly{
			eventType:  valueobject.BackendAIFaceMismatch,
			severity:   valueobject.SeverityCritical,
			label:      fmt.Sprintf("Backend AI: liveness failed (score=%.2f, method=%s)", frame.Liveness.Score, frame.Liveness.Method),
			confidence: 1.0 - frame.Liveness.Score,
		})
	}

	return anomalies
}

func firstAIAnalysisThresholds(thresholds []AIAnalysisThresholds) AIAnalysisThresholds {
	if len(thresholds) == 0 {
		return AIAnalysisThresholds{}
	}
	return thresholds[0]
}

func normalizeAIAnalysisThresholds(thresholds AIAnalysisThresholds) AIAnalysisThresholds {
	if thresholds.FaceMismatch <= 0 || thresholds.FaceMismatch > 1 {
		thresholds.FaceMismatch = 0.6
	}
	if thresholds.Liveness <= 0 || thresholds.Liveness > 1 {
		thresholds.Liveness = 0.3
	}
	if thresholds.ObjectConfidence <= 0 || thresholds.ObjectConfidence > 1 {
		thresholds.ObjectConfidence = 0.7
	}
	if thresholds.SpoofConfidence <= 0 || thresholds.SpoofConfidence > 1 {
		thresholds.SpoofConfidence = 0.7
	}
	return thresholds
}

func (h *AIAnalysisHandler) buildEvent(
	payload AIAnalysisPayload,
	serverTime time.Time,
	videoTimestamp float64,
	eventType valueobject.EventType,
	severity valueobject.Severity,
	label string,
	confidence float32,
) *entity.ProctoringEvent {
	return &entity.ProctoringEvent{
		EventID:         randutil.HexToken(16),
		SessionID:       payload.SessionID,
		StudentID:       payload.StudentID,
		ExamID:          payload.ExamID,
		OrgID:           payload.OrgID,
		EventType:       eventType,
		Severity:        severity,
		Source:          valueobject.SourceBackendAI,
		ServerTimestamp: serverTime,
		ClientTimestamp: serverTime,
		VideoTimestamp:  videoTimestamp,
		Label:           label,
		Confidence:      confidence,
	}
}

// ---------------------------------------------------------------------------
// Telegram fraud alert
// ---------------------------------------------------------------------------

func (h *AIAnalysisHandler) fireFraudAlert(payload AIAnalysisPayload, summary aggregateSummary) {
	if h.alerter == nil {
		return
	}

	msg := alerting.MsgAIFraudDetected(
		payload.SessionID, payload.StudentID, payload.ExamID,
		summary.verdict(), summary.maxFraudConf,
		summary.fraudFrames, summary.totalFrames,
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)

	_ = h.alerter.SendDirect(msg)
}

// ---------------------------------------------------------------------------
// Aggregate summary across multiple fragments
// ---------------------------------------------------------------------------

type aggregateSummary struct {
	totalFrames  int
	fraudFrames  int
	maxFraudConf float32
}

func (s *aggregateSummary) merge(summary *inferencepb.AnalysisSummary) {
	if summary == nil {
		return
	}
	s.totalFrames += int(summary.TotalFramesAnalyzed)
	s.fraudFrames += int(summary.FraudFrames)
	if summary.MaxFraudConfidence > s.maxFraudConf {
		s.maxFraudConf = summary.MaxFraudConfidence
	}
}

func (s *aggregateSummary) verdict() string {
	if s.totalFrames == 0 {
		return "clean"
	}
	fraudRate := float64(s.fraudFrames) / float64(s.totalFrames)
	switch {
	case fraudRate > 0.15 || s.maxFraudConf > 0.9:
		return "fraud"
	case fraudRate > 0.05 || s.maxFraudConf > 0.7:
		return "suspicious"
	default:
		return "clean"
	}
}
