// Package export implements the background export worker that processes
// bulk evidence export jobs. It downloads evidence from MinIO, queries
// violation events from ClickHouse, and produces TAR.GZ archives with
// forensic manifests.
//
// Architecture:
//  1. Worker polls PostgreSQL for 'pending' export jobs every 5s.
//  2. Claims a job by setting status = 'processing'.
//  3. For each session: downloads evidence fragments from MinIO.
//  4. Queries ClickHouse for violation events with video timestamps.
//  5. Builds forensic manifest.json with per-file SHA-256 + violations.
//  6. Creates TAR.GZ archive containing evidence files + manifest.
//  7. Uploads archive to MinIO 'argus-exports' bucket.
//  8. Generates presigned download URL with configurable TTL.
//  9. Updates PostgreSQL with completed status + URL.
package export

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/minio/minio-go/v7"
	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/config"
)

// Worker processes pending export jobs in the background.
type Worker struct {
	db           *sql.DB
	chConn       driver.Conn
	minioClient  *minio.Client
	cfg          config.ExportConfig
	evidenceBkt  string // evidence bucket name (argus-evidence)
	logger       *zap.Logger

	stopCh chan struct{}
}

// NewWorker creates a new export background worker.
func NewWorker(
	db *sql.DB,
	chConn driver.Conn,
	minioClient *minio.Client,
	cfg config.ExportConfig,
	evidenceBucket string,
	logger *zap.Logger,
) *Worker {
	return &Worker{
		db:          db,
		chConn:      chConn,
		minioClient: minioClient,
		cfg:         cfg,
		evidenceBkt: evidenceBucket,
		logger:      logger.Named("export_worker"),
		stopCh:      make(chan struct{}),
	}
}

// Start begins the background polling loop.
func (w *Worker) Start(ctx context.Context) {
	w.logger.Info("export worker started",
		zap.Duration("poll_interval", w.cfg.PollInterval),
		zap.Int("worker_count", w.cfg.WorkerCount),
	)

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("export worker stopping (context cancelled)")
			return
		case <-w.stopCh:
			w.logger.Info("export worker stopping (stop signal)")
			return
		case <-ticker.C:
			w.pollAndProcess(ctx)
		}
	}
}

// Stop signals the worker to shut down.
func (w *Worker) Stop() {
	close(w.stopCh)
}

// pollAndProcess finds a pending job and processes it.
func (w *Worker) pollAndProcess(ctx context.Context) {
	job, err := w.claimPendingJob(ctx)
	if err != nil {
		if err != sql.ErrNoRows {
			w.logger.Error("export worker: failed to claim job", zap.Error(err))
		}
		return
	}

	w.logger.Info("export worker: processing job",
		zap.String("export_id", job.ID),
		zap.String("org_id", job.OrgID),
		zap.Int("sessions", len(job.SessionIDs)),
	)

	if err := w.processJob(ctx, job); err != nil {
		w.logger.Error("export worker: job failed",
			zap.String("export_id", job.ID),
			zap.Error(err),
		)
		w.markFailed(ctx, job.ID, err.Error())
		return
	}
}

// claimPendingJob atomically picks and claims a pending export job.
func (w *Worker) claimPendingJob(ctx context.Context) (*entity.ExportJob, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var job entity.ExportJob
	var sessionIDsStr string

	err = tx.QueryRowContext(ctx, `
		SELECT id, org_id, requested_by, session_ids, presign_ttl_sec, created_at
		FROM export_jobs
		WHERE status = 'pending'
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`,
	).Scan(&job.ID, &job.OrgID, &job.RequestedBy, &sessionIDsStr, &job.PresignTTLSec, &job.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Parse session_ids from PostgreSQL array format.
	job.SessionIDs = parsePostgresArray(sessionIDsStr)

	_, err = tx.ExecContext(ctx, `
		UPDATE export_jobs SET status = 'processing' WHERE id = $1`, job.ID)
	if err != nil {
		return nil, err
	}

	return &job, tx.Commit()
}

// processJob downloads evidence and creates the archive.
func (w *Worker) processJob(ctx context.Context, job *entity.ExportJob) error {
	return ProcessExportJob(ctx, w.db, w.chConn, w.minioClient, w.cfg, w.evidenceBkt, w.logger, job)
}

// ProcessExportJob is the shared export logic used by both the legacy polling
// worker and the asynq-based worker service. It downloads evidence fragments
// from MinIO, queries violation events from ClickHouse, builds a TAR.GZ
// archive with a forensic manifest, uploads it, generates a presigned URL,
// and updates the PostgreSQL job record.
func ProcessExportJob(
	ctx context.Context,
	db *sql.DB,
	chConn driver.Conn,
	minioClient *minio.Client,
	cfg config.ExportConfig,
	evidenceBkt string,
	logger *zap.Logger,
	job *entity.ExportJob,
) error {
	// ── Step 1: Query evidence fragments for all sessions ────────────────
	type fragmentInfo struct {
		FragmentID  string
		SessionID   string
		SHA256Hash  string
		URI         string
		SizeBytes   int64
		ContentType string
		DurationSec float64
	}

	var fragments []fragmentInfo

	for _, sessionID := range job.SessionIDs {
		rows, err := chConn.Query(ctx, `
			SELECT fragment_id, session_id, sha256_hash, uri, size_bytes, content_type, duration_sec
			FROM evidence_fragments
			WHERE session_id = ? AND org_id = ?
			ORDER BY uploaded_at ASC`,
			sessionID, job.OrgID,
		)
		if err != nil {
			return fmt.Errorf("query fragments for session %s: %w", sessionID, err)
		}

		for rows.Next() {
			var f fragmentInfo
			if err := rows.Scan(&f.FragmentID, &f.SessionID, &f.SHA256Hash, &f.URI, &f.SizeBytes, &f.ContentType, &f.DurationSec); err != nil {
				rows.Close()
				return fmt.Errorf("scan fragment: %w", err)
			}
			fragments = append(fragments, f)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate fragments for session %s: %w", sessionID, err)
		}
	}

	if len(fragments) == 0 {
		return fmt.Errorf("no evidence fragments found for the requested sessions")
	}

	// ── Step 2: Query violation events for manifest ─────────────────────
	type violationInfo struct {
		EventType         string  `json:"eventType"`
		VideoTimestampSec float64 `json:"videoTimestampSec"`
		Confidence        float64 `json:"confidence"`
		Label             string  `json:"label"`
		Severity          string  `json:"severity"`
	}

	sessionViolations := make(map[string][]violationInfo)

	for _, sessionID := range job.SessionIDs {
		rows, err := chConn.Query(ctx, `
			SELECT event_type, video_timestamp_sec, confidence, label, severity
			FROM proctoring_events
			WHERE session_id = ? AND org_id = ? AND severity IN ('warning', 'critical')
			ORDER BY server_timestamp ASC`,
			sessionID, job.OrgID,
		)
		if err != nil {
			logger.Warn("export: failed to query violations, continuing",
				zap.String("session_id", sessionID), zap.Error(err))
			continue
		}

		var violations []violationInfo
		for rows.Next() {
			var v violationInfo
			if err := rows.Scan(&v.EventType, &v.VideoTimestampSec, &v.Confidence, &v.Label, &v.Severity); err != nil {
				rows.Close()
				continue
			}
			violations = append(violations, v)
		}
		rows.Close()
		if len(violations) > 0 {
			sessionViolations[sessionID] = violations
		}
	}

	// ── Step 3: Create TAR.GZ archive with pipe to MinIO ────────────────
	archiveKey := fmt.Sprintf("%s/%s.tar.gz", job.OrgID, job.ID)

	pr, pw := io.Pipe()

	// Archive hash computation.
	archiveHash := sha256.New()
	teeWriter := io.MultiWriter(pw, archiveHash)

	gzWriter := gzip.NewWriter(teeWriter)
	tarWriter := tar.NewWriter(gzWriter)

	// Build manifest data.
	type manifestFile struct {
		Filename  string `json:"filename"`
		SHA256    string `json:"sha256"`
		SizeBytes int64  `json:"sizeBytes"`
	}

	type manifestSession struct {
		SessionID      string           `json:"sessionId"`
		EvidenceFiles  []manifestFile   `json:"evidenceFiles"`
		Violations     []violationInfo  `json:"violations,omitempty"`
	}

	type manifest struct {
		Version        string            `json:"version"`
		ExportID       string            `json:"exportId"`
		OrgID          string            `json:"orgId"`
		Sessions       []manifestSession `json:"sessions"`
		ArchiveSHA256  string            `json:"archiveSha256"`
		ExportedAt     string            `json:"exportedAt"`
		TotalFragments int               `json:"totalFragments"`
		TotalSizeBytes int64             `json:"totalSizeBytes"`
	}

	manifestData := manifest{
		Version:    "1.0",
		ExportID:   job.ID,
		OrgID:      job.OrgID,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Upload concurrently while writing.
	uploadDone := make(chan error, 1)
	go func() {
		_, err := minioClient.PutObject(ctx, cfg.ExportBucket, archiveKey, pr, -1,
			minio.PutObjectOptions{ContentType: "application/gzip"})
		uploadDone <- err
	}()

	// Write evidence fragments to tar.
	var totalSize int64

	sessionFiles := make(map[string][]manifestFile)

	for _, frag := range fragments {
		// Download from evidence bucket.
		objectKey := extractObjectKey(frag.URI)
		if objectKey == "" {
			objectKey = fmt.Sprintf("%s/%s/%s.webm", job.OrgID, frag.SessionID, frag.FragmentID)
		}

		obj, err := minioClient.GetObject(ctx, evidenceBkt, objectKey, minio.GetObjectOptions{})
		if err != nil {
			logger.Warn("export: failed to download fragment, skipping",
				zap.String("fragment_id", frag.FragmentID), zap.Error(err))
			continue
		}

		// Read the entire object to get exact size.
		data, err := io.ReadAll(obj)
		obj.Close()
		if err != nil {
			logger.Warn("export: failed to read fragment, skipping",
				zap.String("fragment_id", frag.FragmentID), zap.Error(err))
			continue
		}

		filename := fmt.Sprintf("%s/%s.webm", frag.SessionID, frag.FragmentID)

		// Write tar header + data.
		header := &tar.Header{
			Name:    filename,
			Size:    int64(len(data)),
			Mode:    0444, // read-only
			ModTime: time.Now(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			pw.CloseWithError(err)
			<-uploadDone
			return fmt.Errorf("write tar header: %w", err)
		}

		if _, err := tarWriter.Write(data); err != nil {
			pw.CloseWithError(err)
			<-uploadDone
			return fmt.Errorf("write tar data: %w", err)
		}

		totalSize += int64(len(data))
		sessionFiles[frag.SessionID] = append(sessionFiles[frag.SessionID], manifestFile{
			Filename:  filename,
			SHA256:    frag.SHA256Hash,
			SizeBytes: int64(len(data)),
		})
	}

	// Build manifest sessions.
	for _, sessionID := range job.SessionIDs {
		ms := manifestSession{
			SessionID:     sessionID,
			EvidenceFiles: sessionFiles[sessionID],
			Violations:    sessionViolations[sessionID],
		}
		manifestData.Sessions = append(manifestData.Sessions, ms)
	}

	manifestData.TotalFragments = len(fragments)
	manifestData.TotalSizeBytes = totalSize

	// Write manifest.json to tar (SHA-256 placeholder — filled after).
	manifestBytes, err := json.MarshalIndent(manifestData, "", "  ")
	if err != nil {
		pw.CloseWithError(err)
		<-uploadDone
		return fmt.Errorf("marshal manifest: %w", err)
	}

	manifestHeader := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestBytes)),
		Mode:    0444,
		ModTime: time.Now(),
	}
	if err := tarWriter.WriteHeader(manifestHeader); err != nil {
		pw.CloseWithError(err)
		<-uploadDone
		return fmt.Errorf("write manifest header: %w", err)
	}
	if _, err := tarWriter.Write(manifestBytes); err != nil {
		pw.CloseWithError(err)
		<-uploadDone
		return fmt.Errorf("write manifest data: %w", err)
	}

	// Close tar -> gzip -> pipe.
	if err := tarWriter.Close(); err != nil {
		pw.CloseWithError(err)
		<-uploadDone
		return fmt.Errorf("close tar: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		pw.CloseWithError(err)
		<-uploadDone
		return fmt.Errorf("close gzip: %w", err)
	}
	pw.Close()

	// Wait for upload to complete.
	if err := <-uploadDone; err != nil {
		return fmt.Errorf("upload archive to MinIO: %w", err)
	}

	archiveHashHex := hex.EncodeToString(archiveHash.Sum(nil))
	archiveURI := fmt.Sprintf("s3://%s/%s", cfg.ExportBucket, archiveKey)

	// ── Step 4: Generate presigned download URL ─────────────────────────
	ttl := time.Duration(job.PresignTTLSec) * time.Second
	if ttl == 0 {
		ttl = cfg.PresignTTL
	}

	presignedURL, err := minioClient.PresignedGetObject(ctx, cfg.ExportBucket, archiveKey, ttl, nil)
	if err != nil {
		return fmt.Errorf("generate presigned URL: %w", err)
	}

	expiresAt := time.Now().Add(ttl)
	completedAt := time.Now()

	// ── Step 5: Update job as completed ─────────────────────────────────
	_, err = db.ExecContext(ctx, `
		UPDATE export_jobs SET
			status = 'completed',
			archive_uri = $1,
			sha256_archive = $2,
			download_url = $3,
			total_size_bytes = $4,
			fragment_count = $5,
			expires_at = $6,
			completed_at = $7
		WHERE id = $8`,
		archiveURI, archiveHashHex, presignedURL.String(),
		totalSize, len(fragments), expiresAt, completedAt, job.ID,
	)
	if err != nil {
		return fmt.Errorf("update job status: %w", err)
	}

	logger.Info("export worker: job completed",
		zap.String("export_id", job.ID),
		zap.Int("fragments", len(fragments)),
		zap.Int64("total_size_bytes", totalSize),
		zap.String("archive_hash", archiveHashHex[:16]+"..."),
	)

	return nil
}

// markFailed updates a job status to 'failed' with an error message.
func (w *Worker) markFailed(ctx context.Context, jobID, errMsg string) {
	MarkExportFailed(ctx, w.db, w.logger, jobID, errMsg)
}

// MarkExportFailed updates a job status to 'failed' with an error message.
// Exported for use by the asynq worker handler.
func MarkExportFailed(ctx context.Context, db *sql.DB, logger *zap.Logger, jobID, errMsg string) {
	_, err := db.ExecContext(ctx, `
		UPDATE export_jobs SET status = 'failed', error_message = $1 WHERE id = $2`,
		errMsg, jobID,
	)
	if err != nil {
		logger.Error("export worker: failed to mark job as failed",
			zap.String("export_id", jobID), zap.Error(err))
	}
}

// extractObjectKey extracts the MinIO object key from an S3 URI.
// Input: "s3://argus-evidence/org-id/session-id/fragment-id.webm"
// Output: "org-id/session-id/fragment-id.webm"
func extractObjectKey(uri string) string {
	const prefix = "s3://argus-evidence/"
	if len(uri) > len(prefix) && uri[:len(prefix)] == prefix {
		return uri[len(prefix):]
	}
	// Try generic s3:// prefix.
	if len(uri) > 5 && uri[:5] == "s3://" {
		rest := uri[5:]
		// Skip bucket name.
		for i := 0; i < len(rest); i++ {
			if rest[i] == '/' {
				return rest[i+1:]
			}
		}
	}
	return ""
}

// parsePostgresArray parses a PostgreSQL text array representation.
// Input: "{sess-1,sess-2,sess-3}" -> ["sess-1", "sess-2", "sess-3"]
func parsePostgresArray(s string) []string {
	if len(s) < 2 {
		return nil
	}
	// Strip braces.
	if s[0] == '{' && s[len(s)-1] == '}' {
		s = s[1 : len(s)-1]
	}
	if s == "" {
		return nil
	}
	parts := make([]string, 0)
	for _, p := range splitComma(s) {
		// Strip quotes if present.
		if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
			p = p[1 : len(p)-1]
		}
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// splitComma splits on commas but respects quoted strings.
func splitComma(s string) []string {
	var result []string
	var current []byte
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuotes = !inQuotes
			current = append(current, s[i])
		case s[i] == ',' && !inQuotes:
			result = append(result, string(current))
			current = current[:0]
		default:
			current = append(current, s[i])
		}
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	return result
}
