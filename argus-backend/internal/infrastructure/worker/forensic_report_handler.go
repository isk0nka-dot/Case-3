package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/hibiken/asynq"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/application/port"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/internal/infrastructure/forensic"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
)

// ForensicReportHandler processes TypeForensicReport asynq tasks.
// It generates a forensic PDF report, uploads it to MinIO, and sends
// a Telegram notification upon completion.
type ForensicReportHandler struct {
	scorer      *forensic.Scorer
	verifier    port.IntegrityVerifier
	pgRepo      *postgres.Repository
	minioClient *minio.Client
	logger      *zap.Logger
	alerter     alerting.Provider

	// reportBucket is the MinIO bucket for storing generated PDF reports.
	reportBucket string
}

// NewForensicReportHandler creates a new asynq handler for forensic report jobs.
func NewForensicReportHandler(
	chConn driver.Conn,
	verifier port.IntegrityVerifier,
	pgRepo *postgres.Repository,
	minioClient *minio.Client,
	logger *zap.Logger,
	alerter alerting.Provider,
) *ForensicReportHandler {
	return &ForensicReportHandler{
		scorer:       forensic.NewScorer(chConn, logger),
		verifier:     verifier,
		pgRepo:       pgRepo,
		minioClient:  minioClient,
		logger:       logger.Named("asynq_forensic_report"),
		alerter:      alerter,
		reportBucket: "argus-reports",
	}
}

// ProcessTask implements the asynq.Handler interface.
func (h *ForensicReportHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	var payload ForensicReportPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal ForensicReportPayload: %w", err)
	}

	h.logger.Info("processing forensic report job",
		zap.String("session_id", payload.SessionID),
		zap.String("org_id", payload.OrgID),
		zap.String("requested_by", payload.RequestedBy),
	)

	// ── Step 1: Build forensic report ───────────────────────────────────
	report, err := h.buildReport(ctx, payload.SessionID)
	if err != nil {
		NotifyJobFailed(h.alerter, TypeForensicReport, payload.SessionID, err)
		return fmt.Errorf("build forensic report for %s: %w", payload.SessionID, err)
	}

	// ── Step 2: Generate PDF ────────────────────────────────────────────
	pdfBytes, pdfHash, err := forensic.GenerateForensicPDF(report)
	if err != nil {
		NotifyJobFailed(h.alerter, TypeForensicReport, payload.SessionID, err)
		return fmt.Errorf("generate forensic PDF for %s: %w", payload.SessionID, err)
	}

	h.logger.Info("forensic PDF generated",
		zap.String("session_id", payload.SessionID),
		zap.Int("size_bytes", len(pdfBytes)),
		zap.String("pdf_hash", pdfHash),
	)

	// ── Step 3: Upload PDF to MinIO ─────────────────────────────────────
	objectKey := fmt.Sprintf("%s/%s/%s.pdf", payload.OrgID, payload.SessionID, report.ReportID)

	_, err = h.minioClient.PutObject(ctx, h.reportBucket, objectKey, bytes.NewReader(pdfBytes), int64(len(pdfBytes)),
		minio.PutObjectOptions{
			ContentType: "application/pdf",
			UserMetadata: map[string]string{
				"session-id":   payload.SessionID,
				"report-id":    report.ReportID,
				"report-hash":  pdfHash,
				"requested-by": payload.RequestedBy,
			},
		})
	if err != nil {
		NotifyJobFailed(h.alerter, TypeForensicReport, payload.SessionID, err)
		return fmt.Errorf("upload forensic PDF to MinIO for %s: %w", payload.SessionID, err)
	}

	// ── Step 4: Generate presigned URL ──────────────────────────────────
	presignedURL, err := h.minioClient.PresignedGetObject(ctx, h.reportBucket, objectKey, 24*time.Hour, nil)
	if err != nil {
		h.logger.Warn("forensic: failed to generate presigned URL, continuing",
			zap.String("session_id", payload.SessionID), zap.Error(err))
	}

	duration := time.Since(start)

	h.logger.Info("forensic report job completed",
		zap.String("session_id", payload.SessionID),
		zap.String("report_id", report.ReportID),
		zap.Duration("duration", duration),
		zap.String("pdf_hash", pdfHash),
	)

	if presignedURL != nil {
		h.logger.Info("forensic report download URL",
			zap.String("session_id", payload.SessionID),
			zap.String("url", presignedURL.String()),
		)
	}

	// ── Step 5: Telegram notification ───────────────────────────────────
	NotifyJobCompleted(h.alerter, TypeForensicReport, payload.SessionID, duration)

	return nil
}

// buildReport generates the full forensic report (mirrors ForensicHandler.buildForensicReport).
func (h *ForensicReportHandler) buildReport(ctx context.Context, sessionID string) (*forensic.ForensicReport, error) {
	// Attempt config-aware scoring.
	var score *forensic.IntegrityScore
	var err error

	if h.pgRepo != nil {
		orgID, examID, _, metaErr := h.scorer.QuerySessionMeta(ctx, sessionID)
		if metaErr == nil && orgID != "" && examID != "" {
			settings, settingsErr := h.pgRepo.GetExamProctoringSettings(ctx, orgID, examID)
			if settingsErr == nil && settings != nil {
				score, err = h.scorer.ComputeScoreWithConfig(ctx, sessionID, settings)
				if err != nil {
					return nil, fmt.Errorf("dynamic score computation: %w", err)
				}
			}
		}
	}

	// Fallback to default scoring.
	if score == nil {
		score, err = h.scorer.ComputeScore(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("score computation: %w", err)
		}
	}

	timeline, err := h.scorer.QueryTimeline(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("timeline query: %w", err)
	}

	deviceInfo, err := h.scorer.QueryDeviceInfo(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("device info query: %w", err)
	}

	gazeData, err := h.scorer.QueryGazeData(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("gaze data query: %w", err)
	}

	voice, err := h.scorer.QueryVoiceBiometric(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("voice biometric query: %w", err)
	}

	sidecamSummary, err := h.scorer.QuerySidecamSummary(ctx, sessionID)
	if err != nil {
		h.logger.Warn("forensic: sidecam summary query failed, continuing",
			zap.String("session_id", sessionID), zap.Error(err))
		sidecamSummary = &forensic.SidecamSummary{}
	}

	// Integrity verification.
	var ledger forensic.LedgerSummary
	if h.verifier != nil {
		intReport, vErr := h.verifier.VerifySession(ctx, sessionID)
		if vErr != nil {
			h.logger.Warn("forensic: ledger verification failed, continuing",
				zap.String("session_id", sessionID), zap.Error(vErr))
		} else if intReport != nil {
			ledger = forensic.LedgerSummary{
				TotalFragments: intReport.TotalFragments,
				VerifiedOK:     intReport.VerifiedOK,
				ChainValid:     intReport.ChainValid,
				S3Verified:     intReport.S3Verified,
				S3Mismatches:   intReport.S3Mismatches,
			}
		}
	}

	reportID := fmt.Sprintf("FR-%s-%d", sessionID[:min(8, len(sessionID))], time.Now().UnixMilli())

	report := &forensic.ForensicReport{
		ReportID:      reportID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		SessionID:     sessionID,
		StudentID:     score.StudentID,
		ExamID:        score.ExamID,
		OrgID:         score.OrgID,
		Integrity:     *score,
		Voice:         *voice,
		Timeline:      timeline,
		DeviceInfo:    *deviceInfo,
		GazeData:      gazeData,
		LedgerSummary: ledger,
		Sidecam:       *sidecamSummary,
	}

	// Compute report hash.
	reportJSON, _ := json.Marshal(report)
	hash := sha256.Sum256(reportJSON)
	report.ReportHash = fmt.Sprintf("%x", hash[:])

	return report, nil
}
