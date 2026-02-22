package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/hibiken/asynq"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/alerting"
	"github.com/argus-ai/event-collector/internal/infrastructure/config"
	exportPkg "github.com/argus-ai/event-collector/internal/infrastructure/export"
)

// VideoExportHandler processes TypeVideoExport asynq tasks.
// It wraps the shared ProcessExportJob logic with asynq lifecycle management
// and Telegram notifications.
type VideoExportHandler struct {
	db          *sql.DB
	chConn      driver.Conn
	minioClient *minio.Client
	cfg         config.ExportConfig
	evidenceBkt string
	logger      *zap.Logger
	alerter     alerting.Provider
}

// NewVideoExportHandler creates a new asynq handler for video export jobs.
func NewVideoExportHandler(
	db *sql.DB,
	chConn driver.Conn,
	minioClient *minio.Client,
	cfg config.ExportConfig,
	evidenceBucket string,
	logger *zap.Logger,
	alerter alerting.Provider,
) *VideoExportHandler {
	return &VideoExportHandler{
		db:          db,
		chConn:      chConn,
		minioClient: minioClient,
		cfg:         cfg,
		evidenceBkt: evidenceBucket,
		logger:      logger.Named("asynq_video_export"),
		alerter:     alerter,
	}
}

// ProcessTask implements the asynq.Handler interface.
func (h *VideoExportHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	var payload VideoExportPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal VideoExportPayload: %w", err)
	}

	h.logger.Info("processing video export job",
		zap.String("export_id", payload.ExportID),
		zap.String("org_id", payload.OrgID),
		zap.Int("sessions", len(payload.SessionIDs)),
	)

	// Mark job as processing in PostgreSQL.
	_, err := h.db.ExecContext(ctx, `
		UPDATE export_jobs SET status = 'processing' WHERE id = $1 AND status = 'pending'`,
		payload.ExportID,
	)
	if err != nil {
		return fmt.Errorf("claim job %s: %w", payload.ExportID, err)
	}

	// Build the ExportJob entity from the payload.
	job := &entity.ExportJob{
		ID:            payload.ExportID,
		OrgID:         payload.OrgID,
		SessionIDs:    payload.SessionIDs,
		PresignTTLSec: payload.PresignTTLSec,
	}

	// Delegate to the shared processing logic.
	if err := exportPkg.ProcessExportJob(
		ctx, h.db, h.chConn, h.minioClient,
		h.cfg, h.evidenceBkt, h.logger, job,
	); err != nil {
		// Mark as failed in PostgreSQL.
		exportPkg.MarkExportFailed(ctx, h.db, h.logger, payload.ExportID, err.Error())

		// Telegram notification for failure.
		NotifyJobFailed(h.alerter, TypeVideoExport, payload.ExportID, err)

		return fmt.Errorf("process export job %s: %w", payload.ExportID, err)
	}

	duration := time.Since(start)
	h.logger.Info("video export job completed",
		zap.String("export_id", payload.ExportID),
		zap.Duration("duration", duration),
	)

	// Telegram notification for success.
	NotifyJobCompleted(h.alerter, TypeVideoExport, payload.ExportID, duration)

	return nil
}
