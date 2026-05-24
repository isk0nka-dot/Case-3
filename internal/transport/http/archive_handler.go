// =============================================================================
// Argus AI — Archive Sessions REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for the Archive Sessions dashboard. Queries ClickHouse
// materialized views and raw event tables to retrieve completed/historical
// session data, event timelines, and per-exam summaries.
//
// Endpoints:
//   GET  /api/v1/archive/sessions            — List archived sessions with filters
//   GET  /api/v1/archive/sessions/{id}/events — All events for a specific session
//   GET  /api/v1/archive/exams                — Exam summaries for archive filters
//   GET  /api/v1/archive/sessions/{id}/export — Export session report (JSON)
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	minioInfra "github.com/argus-ai/event-collector/internal/infrastructure/minio"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
)

// ArchiveHandler serves the archive REST API for historical session review.
type ArchiveHandler struct {
	chConn driver.Conn
	chMeta *clickhouse.Writer
	pgRepo *postgres.Repository
	minio  *minioInfra.Store
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo
}

// NewArchiveHandler creates a new archive API handler.
func NewArchiveHandler(
	chWriter *clickhouse.Writer,
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
	minio *minioInfra.Store,
) *ArchiveHandler {
	return &ArchiveHandler{
		chConn:        chWriter.Conn(),
		chMeta:        chWriter,
		pgRepo:        pgRepo,
		minio:         minio,
		logger:        logger.Named("archive_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          pgRepo,
	}
}

// RegisterRoutes registers archive API endpoints on the given mux.
func (h *ArchiveHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/archive/sessions", h.requireAuth(h.handleListSessions))
	mux.HandleFunc("GET /api/v1/archive/sessions/{sessionId}/events", h.requireAuth(h.handleSessionEvents))
	mux.HandleFunc("GET /api/v1/archive/exams", h.requireAuth(h.handleExamSummaries))
	mux.HandleFunc("GET /api/v1/archive/sessions/{sessionId}/export", h.requireAuth(h.handleExportSession))
	mux.HandleFunc("GET /api/v1/archive/sessions/{sessionId}/video", h.requireAuth(h.handleStreamVideo))

	// Review workflow endpoints — mandatory human review for all sessions.
	mux.HandleFunc("POST /api/v1/archive/sessions/{sessionId}/review", h.requireAuth(h.handleSubmitReview))
	mux.HandleFunc("GET /api/v1/archive/sessions/{sessionId}/review", h.requireAuth(h.handleGetReview))
	mux.HandleFunc("GET /api/v1/archive/review-stats", h.requireAuth(h.handleReviewStats))
}

// ==========================================================================
// Response Types
// ==========================================================================

// ArchiveSessionItem represents one completed session in the archive list.
type ArchiveSessionItem struct {
	SessionID      string  `json:"sessionId"`
	StudentID      string  `json:"studentId"`
	ExamID         string  `json:"examId"`
	OrgID          string  `json:"orgId"`
	TotalEvents    uint64  `json:"totalEvents"`
	CriticalCount  uint64  `json:"criticalCount"`
	WarningCount   uint64  `json:"warningCount"`
	InfoCount      uint64  `json:"infoCount"`
	IntegrityScore float64 `json:"integrityScore"` // 100 - (critical_ratio * 100)
	ViolationCount uint64  `json:"violationCount"` // critical + warning
	FirstEventTime string  `json:"firstEventTime"`
	LastEventTime  string  `json:"lastEventTime"`
	DurationSec    int64   `json:"durationSec"`
	Duration       string  `json:"duration"` // formatted "1ч 32м"
	Status         string  `json:"status"`   // "reviewed" | "pending" | "voided"
}

// ArchiveSessionsResponse wraps the list of archived sessions.
type ArchiveSessionsResponse struct {
	Sessions      []ArchiveSessionItem `json:"sessions"`
	Total         int                  `json:"total"`
	TotalReviewed int                  `json:"totalReviewed"`
	TotalPending  int                  `json:"totalPending"`
	TotalVoided   int                  `json:"totalVoided"`
	GeneratedAt   string               `json:"generatedAt"`
}

// ArchiveEvent represents a single event in a session's timeline.
type ArchiveEvent struct {
	EventID        string  `json:"eventId"`
	EventType      string  `json:"eventType"`
	Severity       string  `json:"severity"`
	Source         string  `json:"source"`
	Label          string  `json:"label"`
	Confidence     float64 `json:"confidence"`
	Timestamp      string  `json:"timestamp"`
	VideoTimestamp int64   `json:"videoTimestamp"` // seconds from session start
}

// ArchiveSessionEventsResponse wraps the event timeline for a session.
type ArchiveSessionEventsResponse struct {
	SessionID      string         `json:"sessionId"`
	Events         []ArchiveEvent `json:"events"`
	TotalEvents    int            `json:"totalEvents"`
	CriticalCount  int            `json:"criticalCount"`
	WarningCount   int            `json:"warningCount"`
	DurationSec    int64          `json:"durationSec"`
	IntegrityScore float64        `json:"integrityScore"`
	GeneratedAt    string         `json:"generatedAt"`
}

// ArchiveExamSummary represents an exam with aggregated session stats.
type ArchiveExamSummary struct {
	ExamID          string  `json:"examId"`
	OrgID           string  `json:"orgId"`
	SessionCount    uint64  `json:"sessionCount"`
	TotalEvents     uint64  `json:"totalEvents"`
	TotalCritical   uint64  `json:"totalCritical"`
	TotalWarning    uint64  `json:"totalWarning"`
	AvgIntegrity    float64 `json:"avgIntegrity"`
	FirstEventTime  string  `json:"firstEventTime"`
	LastEventTime   string  `json:"lastEventTime"`
}

// ArchiveExamsResponse wraps exam summaries.
type ArchiveExamsResponse struct {
	Exams       []ArchiveExamSummary `json:"exams"`
	Total       int                  `json:"total"`
	GeneratedAt string               `json:"generatedAt"`
}

// ExportSessionResponse is the full JSON export of a session.
type ExportSessionResponse struct {
	Session    ArchiveSessionItem `json:"session"`
	Events     []ArchiveEvent     `json:"events"`
	ExportedAt string             `json:"exportedAt"`
	ExportedBy string             `json:"exportedBy"`
	Format     string             `json:"format"`
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleListSessions returns completed sessions from ClickHouse.
//
//	GET /api/v1/archive/sessions?org_id=...&exam_id=...&status=...&limit=...&offset=...
func (h *ArchiveHandler) handleListSessions(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	examID := r.URL.Query().Get("exam_id")
	statusFilter := r.URL.Query().Get("status") // "reviewed", "pending", "voided", ""
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 100
	offset := 0
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 500 {
		limit = l
	}
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	sessions, err := h.queryArchivedSessions(ctx, orgID, examID, limit, offset)
	if err != nil {
		h.logger.Error("archive sessions query failed", zap.Error(err))
		h.jsonError(w, "Failed to query archived sessions", http.StatusInternalServerError)
		return
	}

	// Apply status filter (status is derived from integrity score, not stored)
	if statusFilter != "" {
		filtered := make([]ArchiveSessionItem, 0)
		for _, s := range sessions {
			if s.Status == statusFilter {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}

	// Compute summary stats
	totalReviewed, totalPending, totalVoided := 0, 0, 0
	for _, s := range sessions {
		switch s.Status {
		case "reviewed":
			totalReviewed++
		case "pending":
			totalPending++
		case "voided":
			totalVoided++
		}
	}

	resp := ArchiveSessionsResponse{
		Sessions:      sessions,
		Total:         len(sessions),
		TotalReviewed: totalReviewed,
		TotalPending:  totalPending,
		TotalVoided:   totalVoided,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	h.jsonResponse(w, resp, http.StatusOK)
}

// handleSessionEvents returns all events for a specific session.
//
//	GET /api/v1/archive/sessions/{sessionId}/events
func (h *ArchiveHandler) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	events, firstTime, lastTime, err := h.querySessionEvents(ctx, sessionID)
	if err != nil {
		h.logger.Error("session events query failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		h.jsonError(w, "Failed to query session events", http.StatusInternalServerError)
		return
	}

	durationSec := int64(0)
	if !firstTime.IsZero() && !lastTime.IsZero() {
		durationSec = int64(lastTime.Sub(firstTime).Seconds())
	}

	criticalCount, warningCount := 0, 0
	for _, e := range events {
		switch e.Severity {
		case "critical":
			criticalCount++
		case "warning":
			warningCount++
		}
	}

	// Integrity score: 100 - (critical_ratio * 100)
	integrity := 100.0
	if len(events) > 0 {
		integrity = 100.0 - (float64(criticalCount) / float64(len(events)) * 100.0)
	}

	resp := ArchiveSessionEventsResponse{
		SessionID:      sessionID,
		Events:         events,
		TotalEvents:    len(events),
		CriticalCount:  criticalCount,
		WarningCount:   warningCount,
		DurationSec:    durationSec,
		IntegrityScore: math.Round(integrity*10) / 10,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	h.jsonResponse(w, resp, http.StatusOK)
}

// handleExamSummaries returns exam-level aggregates for the archive filter.
//
//	GET /api/v1/archive/exams?org_id=...
func (h *ArchiveHandler) handleExamSummaries(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	exams, err := h.queryExamSummaries(ctx, orgID)
	if err != nil {
		h.logger.Error("exam summaries query failed", zap.Error(err))
		h.jsonError(w, "Failed to query exam summaries", http.StatusInternalServerError)
		return
	}

	resp := ArchiveExamsResponse{
		Exams:       exams,
		Total:       len(exams),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	h.jsonResponse(w, resp, http.StatusOK)
}

// handleExportSession exports a full session report as JSON.
//
//	GET /api/v1/archive/sessions/{sessionId}/export
func (h *ArchiveHandler) handleExportSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Get session summary by ID
	sessions, err := h.queryArchivedSessionByID(ctx, sessionID)
	if err != nil || len(sessions) == 0 {
		h.logger.Error("export session query failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		h.jsonError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Get full event timeline
	events, _, _, err := h.querySessionEvents(ctx, sessionID)
	if err != nil {
		h.logger.Error("export events query failed",
			zap.String("session_id", sessionID),
			zap.Error(err))
		h.jsonError(w, "Failed to export session events", http.StatusInternalServerError)
		return
	}

	caller := getUserFromContext(r.Context())
	exportedBy := "unknown"
	if caller != nil {
		exportedBy = caller.Phone

		// Audit log the export
		entry := &entity.AuditEntry{
			UserID:       caller.ID,
			UserPhone:    caller.Phone,
			UserRole:     string(caller.Role),
			OrgID:        caller.OrgID,
			Action:       "session_export",
			ResourceType: "session",
			ResourceID:   sessionID,
			Details:      map[string]string{"format": "json", "session_id": sessionID},
			IPAddress:    extractClientIP(r),
			UserAgent:    r.UserAgent(),
		}
		go func() {
			bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer bgCancel()
			if err := h.pgRepo.CreateAuditEntry(bgCtx, entry); err != nil {
				h.logger.Error("failed to create export audit entry", zap.Error(err))
			}
		}()
	}

	resp := ExportSessionResponse{
		Session:    sessions[0],
		Events:     events,
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		ExportedBy: exportedBy,
		Format:     "json",
	}

	// Set download headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="argus-session-%s.json"`, sessionID))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// ==========================================================================
// ClickHouse Queries
// ==========================================================================

// queryArchivedSessions fetches completed sessions from student_session_summary.
// Unlike monitoring which only shows sessions from the last 30s, archive queries
// ALL sessions regardless of recency.
func (h *ArchiveHandler) queryArchivedSessionByID(ctx context.Context, sessionID string) ([]ArchiveSessionItem, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		SELECT
			s.student_id, s.session_id, s.exam_id, s.org_id,
			sum(s.total_events) AS total_events,
			sum(s.critical_count) AS critical_count,
			sum(s.warning_count) AS warning_count,
			sum(s.info_count) AS info_count,
			min(s.first_event_time) AS first_event_time,
			max(s.last_event_time) AS last_event_time
		FROM student_session_summary s
		WHERE s.session_id = ?
		GROUP BY s.student_id, s.session_id, s.exam_id, s.org_id
		LIMIT 1`

	rows, err := h.chConn.Query(qctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	terminatedSet := make(map[string]bool)
	terminatedSessions, err := h.pgRepo.GetTerminatedSessions(ctx, "*")
	if err == nil {
		for _, sid := range terminatedSessions {
			terminatedSet[sid] = true
		}
	}

	// Fetch review decisions to determine true session status.
	reviewedSet := make(map[string]string)
	if reviews, err := h.pgRepo.ListReviewedSessionIDs(ctx, ""); err == nil {
		reviewedSet = reviews
	}

	var sessions []ArchiveSessionItem
	for rows.Next() {
		var s ArchiveSessionItem
		var totalEvents, criticalCount, warningCount, infoCount uint64
		var firstTime, lastTime time.Time

		if err := rows.Scan(&s.StudentID, &s.SessionID, &s.ExamID, &s.OrgID,
			&totalEvents, &criticalCount, &warningCount, &infoCount,
			&firstTime, &lastTime); err != nil {
			return nil, err
		}

		s.TotalEvents = totalEvents
		s.CriticalCount = criticalCount
		s.WarningCount = warningCount
		s.InfoCount = infoCount
		s.ViolationCount = criticalCount + warningCount
		s.FirstEventTime = firstTime.Format(time.RFC3339)
		s.LastEventTime = lastTime.Format(time.RFC3339)

		dur := lastTime.Sub(firstTime)
		s.DurationSec = int64(dur.Seconds())
		hours := int(dur.Hours())
		minutes := int(dur.Minutes()) % 60
		if hours > 0 {
			s.Duration = fmt.Sprintf("%dч %02dм", hours, minutes)
		} else {
			s.Duration = fmt.Sprintf("%dм", minutes)
		}

		if totalEvents > 0 {
			s.IntegrityScore = math.Round((100.0-float64(criticalCount)/float64(totalEvents)*100.0)*10) / 10
		} else {
			s.IntegrityScore = 100.0
		}

		// Status derivation (updated for mandatory human review):
		// - "voided"   = terminated by a proctor (overrides everything)
		// - "reviewed" = a human review decision exists (confirmed/dismissed/escalated)
		// - "pending"  = no review decision yet (regardless of integrity score)
		if terminatedSet[s.SessionID] {
			s.Status = "voided"
		} else if _, hasReview := reviewedSet[s.SessionID]; hasReview {
			s.Status = "reviewed"
		} else {
			s.Status = "pending"
		}

		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []ArchiveSessionItem{}
	}
	return sessions, nil
}

func (h *ArchiveHandler) queryArchivedSessions(ctx context.Context, orgID, examID string, limit, offset int) ([]ArchiveSessionItem, error) {
	qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	query := `
		SELECT
			s.student_id,
			s.session_id,
			s.exam_id,
			s.org_id,
			sum(s.total_events) AS total_events,
			sum(s.critical_count) AS critical_count,
			sum(s.warning_count) AS warning_count,
			sum(s.info_count) AS info_count,
			min(s.first_event_time) AS first_event_time,
			max(s.last_event_time) AS last_event_time
		FROM student_session_summary s
		WHERE 1=1`

	var args []interface{}

	if orgID != "*" && orgID != "" {
		query += ` AND s.org_id = ?`
		args = append(args, orgID)
	}

	// When examID is provided as a specific session_id for export, filter by it
	if examID != "" {
		// Check if it's a session_id (contains "session") or exam_id
		query += ` AND s.exam_id = ?`
		args = append(args, examID)
	}

	query += `
		GROUP BY s.student_id, s.session_id, s.exam_id, s.org_id
		ORDER BY last_event_time DESC
		LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := h.chConn.Query(qctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get terminated session IDs from PostgreSQL (scoped to org).
	terminatedSet := make(map[string]bool)
	terminatedSessions, err := h.pgRepo.GetTerminatedSessions(ctx, orgID)
	if err == nil {
		for _, sid := range terminatedSessions {
			terminatedSet[sid] = true
		}
	}

	// Fetch review decisions to determine true session status.
	reviewedSet := make(map[string]string)
	if orgID == "*" || orgID == "" {
		if reviews, rErr := h.pgRepo.ListReviewedSessionIDs(ctx, "*"); rErr == nil {
			reviewedSet = reviews
		}
	} else {
		if reviews, rErr := h.pgRepo.ListReviewedSessionIDs(ctx, orgID); rErr == nil {
			reviewedSet = reviews
		}
	}

	var sessions []ArchiveSessionItem
	for rows.Next() {
		var s ArchiveSessionItem
		var totalEvents, criticalCount, warningCount, infoCount uint64
		var firstTime, lastTime time.Time

		if err := rows.Scan(
			&s.StudentID,
			&s.SessionID,
			&s.ExamID,
			&s.OrgID,
			&totalEvents,
			&criticalCount,
			&warningCount,
			&infoCount,
			&firstTime,
			&lastTime,
		); err != nil {
			return nil, err
		}

		s.TotalEvents = totalEvents
		s.CriticalCount = criticalCount
		s.WarningCount = warningCount
		s.InfoCount = infoCount
		s.ViolationCount = criticalCount + warningCount
		s.FirstEventTime = firstTime.Format(time.RFC3339)
		s.LastEventTime = lastTime.Format(time.RFC3339)

		// Duration
		dur := lastTime.Sub(firstTime)
		s.DurationSec = int64(dur.Seconds())
		hours := int(dur.Hours())
		minutes := int(dur.Minutes()) % 60
		if hours > 0 {
			s.Duration = fmt.Sprintf("%dч %02dм", hours, minutes)
		} else {
			s.Duration = fmt.Sprintf("%dм", minutes)
		}

		// Integrity score: 100 - (critical_events / total_events * 100)
		if totalEvents > 0 {
			s.IntegrityScore = math.Round((100.0-float64(criticalCount)/float64(totalEvents)*100.0)*10) / 10
		} else {
			s.IntegrityScore = 100.0
		}

		// Status derivation (updated for mandatory human review):
		// - "voided"   = terminated by a proctor (overrides everything)
		// - "reviewed" = a human review decision exists (confirmed/dismissed/escalated)
		// - "pending"  = no review decision yet (regardless of integrity score)
		if terminatedSet[s.SessionID] {
			s.Status = "voided"
		} else if _, hasReview := reviewedSet[s.SessionID]; hasReview {
			s.Status = "reviewed"
		} else {
			s.Status = "pending"
		}

		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []ArchiveSessionItem{}
	}

	return sessions, nil
}

// querySessionEvents returns ALL events for a given session, ordered by timestamp.
// Returns events plus the first/last event times for duration calculation.
func (h *ArchiveHandler) querySessionEvents(ctx context.Context, sessionID string) ([]ArchiveEvent, time.Time, time.Time, error) {
	qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	rows, err := h.chConn.Query(qctx,
		`SELECT
			event_id,
			event_type,
			severity,
			source,
			label,
			confidence,
			server_timestamp,
			video_timestamp_sec
		FROM proctoring_events
		WHERE session_id = ?
		ORDER BY server_timestamp ASC
		LIMIT 10000`, sessionID)
	if err != nil {
		return nil, time.Time{}, time.Time{}, err
	}
	defer rows.Close()

	var events []ArchiveEvent
	var firstTime, lastTime time.Time

	for rows.Next() {
		var e ArchiveEvent
		var ts time.Time
		var conf float32
		var videoTs float64

		if err := rows.Scan(&e.EventID, &e.EventType, &e.Severity, &e.Source, &e.Label, &conf, &ts, &videoTs); err != nil {
			return nil, time.Time{}, time.Time{}, err
		}

		e.Confidence = float64(conf)
		e.Timestamp = ts.Format(time.RFC3339)
		e.VideoTimestamp = int64(videoTs)
		events = append(events, e)

		// Track first/last
		if firstTime.IsZero() || ts.Before(firstTime) {
			firstTime = ts
		}
		if lastTime.IsZero() || ts.After(lastTime) {
			lastTime = ts
		}
	}

	if events == nil {
		events = []ArchiveEvent{}
	}

	return events, firstTime, lastTime, nil
}

// queryExamSummaries returns per-exam aggregates for the archive filter dropdown.
func (h *ArchiveHandler) queryExamSummaries(ctx context.Context, orgID string) ([]ArchiveExamSummary, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	query := `
		SELECT
			exam_id,
			org_id,
			uniq(session_id) AS session_count,
			sum(total_events) AS total_events,
			sum(critical_count) AS total_critical,
			sum(warning_count) AS total_warning,
			min(first_event_time) AS first_time,
			max(last_event_time) AS last_time
		FROM student_session_summary
		WHERE 1=1`

	var args []interface{}

	if orgID != "*" && orgID != "" {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}

	query += `
		GROUP BY exam_id, org_id
		ORDER BY last_time DESC
		LIMIT 50`

	rows, err := h.chConn.Query(qctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exams []ArchiveExamSummary
	for rows.Next() {
		var e ArchiveExamSummary
		var sessionCount, totalEvents, totalCritical, totalWarning uint64
		var firstTime, lastTime time.Time

		if err := rows.Scan(
			&e.ExamID,
			&e.OrgID,
			&sessionCount,
			&totalEvents,
			&totalCritical,
			&totalWarning,
			&firstTime,
			&lastTime,
		); err != nil {
			return nil, err
		}

		e.SessionCount = sessionCount
		e.TotalEvents = totalEvents
		e.TotalCritical = totalCritical
		e.TotalWarning = totalWarning
		e.FirstEventTime = firstTime.Format(time.RFC3339)
		e.LastEventTime = lastTime.Format(time.RFC3339)

		// Average integrity across sessions: 100 - (critical_ratio * 100)
		if totalEvents > 0 {
			e.AvgIntegrity = math.Round((100.0-float64(totalCritical)/float64(totalEvents)*100.0)*10) / 10
		} else {
			e.AvgIntegrity = 100.0
		}

		exams = append(exams, e)
	}

	if exams == nil {
		exams = []ArchiveExamSummary{}
	}

	return exams, nil
}

// ==========================================================================
// Review Workflow Handlers
// ==========================================================================

// ReviewRequest is the request body for submitting a review decision.
type ReviewRequest struct {
	Decision       string   `json:"decision"`       // confirmed | dismissed | escalated
	Notes          string   `json:"notes"`           // Free-text reviewer notes
	EvidenceIDs    []string `json:"evidenceIds"`     // Reviewed evidence fragment IDs
	IntegrityScore float64  `json:"integrityScore"`  // Integrity score at review time
}

// ReviewResponse is the response for review operations.
type ReviewResponse struct {
	SessionID    string   `json:"sessionId"`
	ReviewerID   string   `json:"reviewerId"`
	ReviewerName string   `json:"reviewerName"`
	Decision     string   `json:"decision"`
	Notes        string   `json:"notes"`
	EvidenceIDs  []string `json:"evidenceIds"`
	IntegrityScore float64 `json:"integrityScore"`
	ReviewedAt   string   `json:"reviewedAt"`
}

// ReviewStatsResponse contains aggregate review statistics.
type ReviewStatsResponse struct {
	Confirmed int    `json:"confirmed"`
	Dismissed int    `json:"dismissed"`
	Escalated int    `json:"escalated"`
	OrgID     string `json:"orgId"`
}

// handleSubmitReview creates or updates a review decision for a session.
// POST /api/v1/archive/sessions/{sessionId}/review
func (h *ArchiveHandler) handleSubmitReview(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Only proctors, org_admins, and super_admins can submit reviews.
	if caller.Role != entity.RoleProctor && caller.Role != entity.RoleOrgAdmin && caller.Role != entity.RoleSuperAdmin {
		h.jsonError(w, "Insufficient permissions — proctor or admin role required", http.StatusForbidden)
		return
	}

	var req ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate decision.
	validDecisions := map[string]bool{"confirmed": true, "dismissed": true, "escalated": true}
	if !validDecisions[req.Decision] {
		h.jsonError(w, "Invalid decision — must be confirmed, dismissed, or escalated", http.StatusBadRequest)
		return
	}

	review := &entity.ReviewDecision{
		SessionID:      sessionID,
		ReviewerID:     caller.ID,
		ReviewerName:   caller.FullName,
		OrgID:          caller.OrgID,
		Decision:       req.Decision,
		Notes:          req.Notes,
		EvidenceIDs:    req.EvidenceIDs,
		IntegrityScore: req.IntegrityScore,
	}

	if err := h.pgRepo.CreateReviewDecision(r.Context(), review); err != nil {
		h.logger.Error("failed to create review decision",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		h.jsonError(w, "Failed to save review decision", http.StatusInternalServerError)
		return
	}

	// Audit log the review.
	if h.pgRepo != nil {
		_ = h.pgRepo.CreateAuditEntry(r.Context(), &entity.AuditEntry{
			UserID:       caller.ID,
			UserPhone:    caller.Phone,
			UserRole:     string(caller.Role),
			OrgID:        caller.OrgID,
			Action:       "review_" + req.Decision,
			ResourceType: "session",
			ResourceID:   sessionID,
			Details:      map[string]interface{}{"decision": req.Decision, "notes": req.Notes},
			IPAddress:    r.RemoteAddr,
			UserAgent:    r.Header.Get("User-Agent"),
		})
	}

	h.jsonResponse(w, ReviewResponse{
		SessionID:      sessionID,
		ReviewerID:     caller.ID,
		ReviewerName:   caller.FullName,
		Decision:       req.Decision,
		Notes:          req.Notes,
		EvidenceIDs:    req.EvidenceIDs,
		IntegrityScore: req.IntegrityScore,
		ReviewedAt:     time.Now().UTC().Format(time.RFC3339),
	}, http.StatusCreated)
}

// handleGetReview retrieves the review decision for a session.
// GET /api/v1/archive/sessions/{sessionId}/review
func (h *ArchiveHandler) handleGetReview(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	review, err := h.pgRepo.GetReviewDecision(r.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to get review decision", zap.Error(err))
		h.jsonError(w, "Failed to retrieve review", http.StatusInternalServerError)
		return
	}

	if review == nil {
		h.jsonResponse(w, map[string]interface{}{
			"sessionId": sessionID,
			"status":    "pending",
			"message":   "No review decision has been submitted for this session",
		}, http.StatusOK)
		return
	}

	h.jsonResponse(w, ReviewResponse{
		SessionID:      review.SessionID,
		ReviewerID:     review.ReviewerID,
		ReviewerName:   review.ReviewerName,
		Decision:       review.Decision,
		Notes:          review.Notes,
		EvidenceIDs:    review.EvidenceIDs,
		IntegrityScore: review.IntegrityScore,
		ReviewedAt:     review.ReviewedAt.Format(time.RFC3339),
	}, http.StatusOK)
}

// handleReviewStats returns aggregate review statistics for the org.
// GET /api/v1/archive/review-stats
func (h *ArchiveHandler) handleReviewStats(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	if orgID == "" {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	confirmed, dismissed, escalated, err := h.pgRepo.GetReviewStats(r.Context(), orgID)
	if err != nil {
		h.logger.Error("failed to get review stats", zap.Error(err))
		h.jsonError(w, "Failed to retrieve review statistics", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, ReviewStatsResponse{
		Confirmed: confirmed,
		Dismissed: dismissed,
		Escalated: escalated,
		OrgID:     orgID,
	}, http.StatusOK)
}

// ==========================================================================
// Auth & Helpers (same pattern as MonitoringHandler)
// ==========================================================================

func (h *ArchiveHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}
		// Allow token via query param for media streaming (video element can't set headers).
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" {
			h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		user, err := h.verifyToken(r.Context(), token)
		if err != nil {
			h.jsonError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *ArchiveHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
	parts := splitToken(tokenStr)
	if parts == nil {
		return nil, errInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := hmacSHA256([]byte(signingInput), h.jwtSigningKey)
	actualSig, err := base64URLDecode(parts[2])
	if err != nil {
		return nil, errInvalidToken
	}
	if !hmacEqual(expectedSig, actualSig) {
		return nil, errInvalidToken
	}

	payloadBytes, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errInvalidToken
	}
	if time.Now().Unix() > claims.Exp {
		return nil, errInvalidToken
	}

	user, err := h.repo.GetUserByID(ctx, claims.Sub)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// resolveOrgID extracts org_id from query params, or from the authenticated user.
func (h *ArchiveHandler) resolveOrgID(r *http.Request) string {
	caller := getUserFromContext(r.Context())
	if caller == nil {
		return ""
	}

	// Super admins can filter by any org.
	if caller.OrgID == "*" {
		qOrg := r.URL.Query().Get("org_id")
		if qOrg != "" {
			return qOrg
		}
		return "*"
	}

	// Non-super admins are scoped to their own org.
	return caller.OrgID
}

func (h *ArchiveHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *ArchiveHandler) jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleStreamVideo streams a recorded MP4 from MinIO for the given session.
//
//	GET /api/v1/archive/sessions/{sessionId}/video
func (h *ArchiveHandler) handleStreamVideo(w http.ResponseWriter, r *http.Request) {
	if h.minio == nil {
		h.jsonError(w, "recording storage not configured", http.StatusServiceUnavailable)
		return
	}

	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "sessionId is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// First try: use stored file_url from livekit_recordings (populated via webhook).
	var objectKey string
	var fileURL string
	_ = h.pgRepo.DB().QueryRowContext(ctx, `
		SELECT file_url FROM livekit_recordings
		WHERE session_id = $1 AND file_url != ''
		ORDER BY created_at DESC LIMIT 1`, sessionID,
	).Scan(&fileURL)
	if fileURL != "" {
		objectKey = fileURL
	}

	// Second try: construct key from external_sessions student_id (webhook not needed).
	if objectKey == "" {
		var studentID string
		_ = h.pgRepo.DB().QueryRowContext(ctx,
			`SELECT student_id FROM external_sessions WHERE session_id = $1 LIMIT 1`,
			sessionID,
		).Scan(&studentID)
		if studentID != "" {
			objectKey = fmt.Sprintf(
				"content/recordings/argus-session-%s/%s-%s.mp4",
				sessionID, sessionID, studentID,
			)
		}
	}

	if objectKey == "" {
		h.jsonError(w, "recording not found", http.StatusNotFound)
		return
	}

	if err := h.minio.StreamKey(ctx, objectKey, w, r); err != nil {
		h.logger.Warn("failed to stream recording",
			zap.String("session_id", sessionID),
			zap.String("key", objectKey),
			zap.Error(err),
		)
		if w.Header().Get("Content-Type") == "" {
			h.jsonError(w, "recording not found or not yet ready", http.StatusNotFound)
		}
	}
}
