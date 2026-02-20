// =============================================================================
// Argus AI — Live Monitoring REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for the real-time Monitoring dashboard. Queries
// ClickHouse materialized views to discover active sessions, compute risk
// scores, and fetch recent violations. Also integrates with PostgreSQL for
// session actions (warn, terminate) and audit logging.
//
// Endpoints:
//   GET  /api/v1/monitoring/active-sessions   — Active sessions with risk scores
//   POST /api/v1/monitoring/sessions/:id/warn — Log a warning for a session
//   POST /api/v1/monitoring/sessions/:id/terminate — Terminate a session
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/postgres"
	"go.uber.org/zap"
)

// MonitoringHandler serves the monitoring REST API for the live proctoring dashboard.
type MonitoringHandler struct {
	chConn driver.Conn
	chMeta *clickhouse.Writer
	pgRepo *postgres.Repository
	logger *zap.Logger

	jwtSigningKey []byte
	repo          adminRepo // for JWT verification (GetUserByID)
}

// NewMonitoringHandler creates a new monitoring API handler.
func NewMonitoringHandler(
	chWriter *clickhouse.Writer,
	pgRepo *postgres.Repository,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *MonitoringHandler {
	return &MonitoringHandler{
		chConn:        chWriter.Conn(),
		chMeta:        chWriter,
		pgRepo:        pgRepo,
		logger:        logger.Named("monitoring_api"),
		jwtSigningKey: jwtSigningKey,
		repo:          pgRepo,
	}
}

// RegisterRoutes registers monitoring API endpoints on the given mux.
func (h *MonitoringHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/monitoring/active-sessions", h.requireAuth(h.handleActiveSessions))
	mux.HandleFunc("POST /api/v1/monitoring/sessions/{sessionId}/warn", h.requireAuth(h.handleWarnSession))
	mux.HandleFunc("POST /api/v1/monitoring/sessions/{sessionId}/terminate", h.requireAuth(h.handleTerminateSession))
}

// ==========================================================================
// Response Types
// ==========================================================================

// ActiveSession represents a single active proctoring session with risk data.
type ActiveSession struct {
	SessionID       string  `json:"sessionId"`
	StudentID       string  `json:"studentId"`
	ExamID          string  `json:"examId"`
	OrgID           string  `json:"orgId"`
	TotalEvents     uint64  `json:"totalEvents"`
	CriticalCount   uint64  `json:"criticalCount"`
	WarningCount    uint64  `json:"warningCount"`
	InfoCount       uint64  `json:"infoCount"`
	RiskScore       float64 `json:"riskScore"` // critical_events / total_events * 100
	ViolationLevel  string  `json:"violationLevel"` // "critical" | "warning" | "clean"
	LastEventTime   string  `json:"lastEventTime"`
	FirstEventTime  string  `json:"firstEventTime"`
	Status          string  `json:"status"` // "active" | "terminated"
	RecentViolations []RecentViolation `json:"recentViolations"`
}

// RecentViolation is a recent violation event for a session.
type RecentViolation struct {
	EventID    string  `json:"eventId"`
	EventType  string  `json:"eventType"`
	Severity   string  `json:"severity"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Timestamp  string  `json:"timestamp"`
}

// ActiveSessionsResponse wraps the list of active sessions with summary stats.
type ActiveSessionsResponse struct {
	Sessions     []ActiveSession `json:"sessions"`
	TotalActive  int             `json:"totalActive"`
	TotalCritical int            `json:"totalCritical"`
	TotalWarning  int            `json:"totalWarning"`
	TotalClean    int            `json:"totalClean"`
	AvgRiskScore float64         `json:"avgRiskScore"`
	GeneratedAt  string          `json:"generatedAt"`
}

// ==========================================================================
// Handlers
// ==========================================================================

// handleActiveSessions returns all sessions that had events in the last 30 seconds.
//
//	GET /api/v1/monitoring/active-sessions?org_id=...&exam_id=...
func (h *MonitoringHandler) handleActiveSessions(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	examID := r.URL.Query().Get("exam_id")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	sessions, err := h.queryActiveSessions(ctx, orgID, examID)
	if err != nil {
		h.logger.Error("active sessions query failed", zap.Error(err))
		h.jsonError(w, "Failed to query active sessions", http.StatusInternalServerError)
		return
	}

	// Fetch recent violations for each session (last 10 per session).
	for i := range sessions {
		violations, err := h.queryRecentViolations(ctx, sessions[i].SessionID, 10)
		if err != nil {
			h.logger.Warn("failed to fetch violations for session",
				zap.String("session_id", sessions[i].SessionID),
				zap.Error(err))
			sessions[i].RecentViolations = []RecentViolation{}
		} else {
			sessions[i].RecentViolations = violations
		}
	}

	// Compute summary statistics.
	totalCritical := 0
	totalWarning := 0
	totalClean := 0
	totalRiskScore := 0.0

	for _, s := range sessions {
		switch s.ViolationLevel {
		case "critical":
			totalCritical++
		case "warning":
			totalWarning++
		default:
			totalClean++
		}
		totalRiskScore += s.RiskScore
	}

	avgRisk := 0.0
	if len(sessions) > 0 {
		avgRisk = totalRiskScore / float64(len(sessions))
	}

	resp := ActiveSessionsResponse{
		Sessions:      sessions,
		TotalActive:   len(sessions),
		TotalCritical: totalCritical,
		TotalWarning:  totalWarning,
		TotalClean:    totalClean,
		AvgRiskScore:  avgRisk,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	h.jsonResponse(w, resp, http.StatusOK)
}

// handleWarnSession logs a proctor warning to the audit log.
//
//	POST /api/v1/monitoring/sessions/:sessionId/warn
func (h *MonitoringHandler) handleWarnSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	// Parse optional message body.
	var body struct {
		Message string `json:"message"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if body.Message == "" {
		body.Message = "Проктор отправил предупреждение студенту"
	}

	// Get the caller user from context.
	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create audit entry for the warning.
	entry := &entity.AuditEntry{
		UserID:       caller.ID,
		UserPhone:    caller.Phone,
		UserRole:     string(caller.Role),
		OrgID:        caller.OrgID,
		Action:       "session_warn",
		ResourceType: "session",
		ResourceID:   sessionID,
		Details:      map[string]string{"message": body.Message, "session_id": sessionID},
		IPAddress:    extractClientIP(r),
		UserAgent:    r.UserAgent(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.pgRepo.CreateAuditEntry(ctx, entry); err != nil {
		h.logger.Error("failed to create warn audit entry",
			zap.String("session_id", sessionID),
			zap.Error(err))
		h.jsonError(w, "Failed to log warning", http.StatusInternalServerError)
		return
	}

	h.logger.Info("session warning issued",
		zap.String("session_id", sessionID),
		zap.String("by_user", caller.Phone),
		zap.String("message", body.Message))

	h.jsonResponse(w, map[string]string{
		"status":    "warned",
		"sessionId": sessionID,
		"message":   body.Message,
	}, http.StatusOK)
}

// handleTerminateSession marks a session as terminated and logs to audit.
//
//	POST /api/v1/monitoring/sessions/:sessionId/terminate
func (h *MonitoringHandler) handleTerminateSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionId")
	if sessionID == "" {
		h.jsonError(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	// Parse optional reason.
	var body struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}

	if body.Reason == "" {
		body.Reason = "Сессия завершена проктором"
	}

	caller := getUserFromContext(r.Context())
	if caller == nil {
		h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Insert termination record into PostgreSQL.
	if err := h.pgRepo.TerminateSession(ctx, sessionID, caller.ID, body.Reason); err != nil {
		h.logger.Error("failed to terminate session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		h.jsonError(w, "Failed to terminate session", http.StatusInternalServerError)
		return
	}

	// Create audit entry.
	entry := &entity.AuditEntry{
		UserID:       caller.ID,
		UserPhone:    caller.Phone,
		UserRole:     string(caller.Role),
		OrgID:        caller.OrgID,
		Action:       "session_terminate",
		ResourceType: "session",
		ResourceID:   sessionID,
		Details:      map[string]string{"reason": body.Reason, "session_id": sessionID},
		IPAddress:    extractClientIP(r),
		UserAgent:    r.UserAgent(),
	}

	go func() {
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bgCancel()
		if err := h.pgRepo.CreateAuditEntry(bgCtx, entry); err != nil {
			h.logger.Error("failed to create terminate audit entry", zap.Error(err))
		}
	}()

	h.logger.Info("session terminated",
		zap.String("session_id", sessionID),
		zap.String("by_user", caller.Phone),
		zap.String("reason", body.Reason))

	h.jsonResponse(w, map[string]string{
		"status":    "terminated",
		"sessionId": sessionID,
		"reason":    body.Reason,
	}, http.StatusOK)
}

// ==========================================================================
// ClickHouse Queries
// ==========================================================================

// queryActiveSessions fetches sessions with events in the last 30 seconds
// from the student_session_summary materialized view.
func (h *MonitoringHandler) queryActiveSessions(ctx context.Context, orgID, examID string) ([]ActiveSession, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// For the loadtest data, "active" means sessions with recent events.
	// We query sessions that have had events in the last 30 seconds.
	// Since SummingMergeTree needs sum() for correctness, we aggregate.
	cutoff := time.Now().UTC().Add(-30 * time.Second)

	var query string
	var args []interface{}

	// Build the query based on filters.
	// We query the raw proctoring_events table for recency, then join with
	// student_session_summary for aggregated stats.
	query = `
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
		WHERE s.session_id IN (
			SELECT DISTINCT session_id
			FROM proctoring_events
			WHERE server_timestamp > ?`

	args = append(args, cutoff)

	if orgID != "*" && orgID != "" {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}
	if examID != "" {
		query += ` AND exam_id = ?`
		args = append(args, examID)
	}

	query += `
		)`

	// Also filter the summary by org/exam for efficiency.
	if orgID != "*" && orgID != "" {
		query += ` AND s.org_id = ?`
		args = append(args, orgID)
	}
	if examID != "" {
		query += ` AND s.exam_id = ?`
		args = append(args, examID)
	}

	query += `
		GROUP BY s.student_id, s.session_id, s.exam_id, s.org_id
		ORDER BY critical_count DESC, warning_count DESC, total_events DESC
		LIMIT 200`

	rows, err := h.chConn.Query(qctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Also get terminated session IDs from PostgreSQL.
	terminatedSet := make(map[string]bool)
	terminatedSessions, err := h.pgRepo.GetTerminatedSessions(ctx)
	if err == nil {
		for _, sid := range terminatedSessions {
			terminatedSet[sid] = true
		}
	}

	var sessions []ActiveSession
	for rows.Next() {
		var s ActiveSession
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
		s.FirstEventTime = firstTime.Format(time.RFC3339)
		s.LastEventTime = lastTime.Format(time.RFC3339)

		// Calculate risk score: (critical_events / total_events) * 100
		if totalEvents > 0 {
			s.RiskScore = float64(criticalCount) / float64(totalEvents) * 100
		}

		// Determine violation level.
		if criticalCount > 0 {
			s.ViolationLevel = "critical"
		} else if warningCount > 0 {
			s.ViolationLevel = "warning"
		} else {
			s.ViolationLevel = "clean"
		}

		// Check termination status.
		if terminatedSet[s.SessionID] {
			s.Status = "terminated"
		} else {
			s.Status = "active"
		}

		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []ActiveSession{}
	}

	return sessions, nil
}

// queryRecentViolations fetches recent warning/critical events for a session.
func (h *MonitoringHandler) queryRecentViolations(ctx context.Context, sessionID string, limit int) ([]RecentViolation, error) {
	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := h.chConn.Query(qctx,
		`SELECT event_id, event_type, severity, label, confidence, server_timestamp
		 FROM proctoring_events
		 WHERE session_id = ? AND severity IN ('warning', 'critical')
		 ORDER BY server_timestamp DESC
		 LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []RecentViolation
	for rows.Next() {
		var v RecentViolation
		var ts time.Time
		var conf float32
		if err := rows.Scan(&v.EventID, &v.EventType, &v.Severity, &v.Label, &conf, &ts); err != nil {
			return nil, err
		}
		v.Confidence = float64(conf)
		v.Timestamp = ts.Format(time.RFC3339)
		violations = append(violations, v)
	}

	if violations == nil {
		violations = []RecentViolation{}
	}
	return violations, nil
}

// ==========================================================================
// Auth & Helpers (same pattern as AnalyticsHandler)
// ==========================================================================

func (h *MonitoringHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := ""
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
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

func (h *MonitoringHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

	payloadJSON, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadJSON, &claims); err != nil {
		return nil, errInvalidToken
	}

	exp, ok := claims["exp"].(float64)
	if !ok || time.Unix(int64(exp), 0).Before(time.Now()) {
		return nil, errTokenExpired
	}

	userID, _ := claims["sub"].(string)
	if userID == "" {
		return nil, errInvalidToken
	}

	user, err := h.repo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, errUserNotFound
	}

	return user, nil
}

func (h *MonitoringHandler) resolveOrgID(r *http.Request) string {
	user := getUserFromContext(r.Context())
	if user == nil {
		return ""
	}

	if user.IsSuperAdmin() {
		if qOrgID := r.URL.Query().Get("org_id"); qOrgID != "" {
			return qOrgID
		}
		return "*"
	}

	return user.OrgID
}

func (h *MonitoringHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

func (h *MonitoringHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
