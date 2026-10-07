// =============================================================================
// Argus AI — Analytics REST API Handler
// =============================================================================
//
// Provides HTTP endpoints for the Executive Analytics Dashboard. Queries
// ClickHouse materialized views to deliver real-time risk distribution,
// system health metrics, and per-organization event statistics.
//
// All endpoints require JWT authentication. Org-level isolation is enforced:
//   - super_admin: can query any org_id or all orgs
//   - org_admin/proctor/viewer: restricted to their own org_id
//
// These endpoints power the frontend Executive View dashboard:
//   - Risk Distribution Pie Chart (severity breakdown)
//   - System Health Monitor (throughput, latency, event rates)
//   - Top Violations & At-Risk Students tables
//
// URL structure:
//   GET /api/v1/analytics/risk-distribution     — Severity breakdown for pie chart
//   GET /api/v1/analytics/system-health          — Writer metrics + event throughput
//   GET /api/v1/analytics/top-violations         — Top violation types by count
//   GET /api/v1/analytics/student-risk           — Top at-risk students
//   GET /api/v1/analytics/hourly-stats           — Hourly event timeline
//   GET /api/v1/analytics/overview               — Combined executive summary
// =============================================================================
package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/argus-ai/event-collector/internal/domain/entity"
	"github.com/argus-ai/event-collector/internal/infrastructure/clickhouse"
	"github.com/argus-ai/event-collector/internal/infrastructure/monitoring"
	"go.uber.org/zap"
)

// AnalyticsHandler serves the analytics REST API for the Executive Dashboard.
type AnalyticsHandler struct {
	chConn driver.Conn
	chMeta *clickhouse.Writer
	logger *zap.Logger

	// JWT signing key and repo from AdminHandler — shared auth layer.
	jwtSigningKey []byte
	repo          adminRepo

	// Infrastructure monitoring collector (Docker + system metrics).
	infraCollector *monitoring.Collector
}

// adminRepo is the minimal interface needed from the PostgreSQL repository
// for authentication (verify JWT → look up user).
type adminRepo interface {
	GetUserByID(ctx context.Context, id string) (*entity.User, error)
}

// NewAnalyticsHandler creates a new analytics API handler.
func NewAnalyticsHandler(
	chWriter *clickhouse.Writer,
	repo adminRepo,
	logger *zap.Logger,
	jwtSigningKey []byte,
) *AnalyticsHandler {
	return &AnalyticsHandler{
		chConn:         chWriter.Conn(),
		chMeta:         chWriter,
		logger:         logger.Named("analytics_api"),
		jwtSigningKey:  jwtSigningKey,
		repo:           repo,
		infraCollector: monitoring.NewCollector(logger),
	}
}

// RegisterRoutes registers analytics API endpoints on the given mux.
func (h *AnalyticsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/analytics/risk-distribution", h.requireAuth(h.handleRiskDistribution))
	mux.HandleFunc("GET /api/v1/analytics/system-health", h.requireAuth(h.handleSystemHealth))
	mux.HandleFunc("GET /api/v1/analytics/top-violations", h.requireAuth(h.handleTopViolations))
	mux.HandleFunc("GET /api/v1/analytics/student-risk", h.requireAuth(h.handleStudentRisk))
	mux.HandleFunc("GET /api/v1/analytics/hourly-stats", h.requireAuth(h.handleHourlyStats))
	mux.HandleFunc("GET /api/v1/analytics/overview", h.requireAuth(h.handleOverview))
	mux.HandleFunc("GET /api/v1/analytics/infrastructure-stats", h.requireAuth(h.handleInfrastructureStats))

	// Enterprise analytics endpoints (Migration 006 MVs)
	mux.HandleFunc("GET /api/v1/analytics/exam-performance", h.requireAuth(h.handleExamPerformance))
	mux.HandleFunc("GET /api/v1/analytics/realtime-health", h.requireAuth(h.handleRealtimeHealth))
	mux.HandleFunc("GET /api/v1/analytics/daily-trends", h.requireAuth(h.handleDailyTrends))
}

// ==========================================================================
// Response Types
// ==========================================================================

// RiskDistribution holds severity counts for the pie chart.
type RiskDistribution struct {
	TotalEvents int64            `json:"totalEvents"`
	BySeverity  map[string]int64 `json:"bySeverity"`
	// Calculated percentages for the frontend.
	Honest     float64 `json:"honest"`     // INFO events %
	Suspicious float64 `json:"suspicious"` // WARNING events %
	Cheaters   float64 `json:"cheaters"`   // CRITICAL events %
}

// SystemHealth holds real-time system metrics.
type SystemHealth struct {
	// ClickHouse writer metrics.
	TotalFlushed  int64 `json:"totalFlushed"`
	TotalDropped  int64 `json:"totalDropped"`
	FlushCount    int64 `json:"flushCount"`
	FlushErrors   int64 `json:"flushErrors"`
	BufferSize    int   `json:"bufferSize"`
	OverflowSize  int   `json:"overflowSize"`

	// Event counts from ClickHouse.
	TotalEvents       int64 `json:"totalEvents"`
	UniqueStudents    int64 `json:"uniqueStudents"`
	UniqueSessions    int64 `json:"uniqueSessions"`
	CriticalEvents    int64 `json:"criticalEvents"`
	EventsLast5Min    int64 `json:"eventsLast5Min"`
	EventsLast1Hour   int64 `json:"eventsLast1Hour"`
}

// ViolationEntry holds a single violation type + count.
type ViolationEntry struct {
	EventType string `json:"eventType"`
	Severity  string `json:"severity"`
	Count     int64  `json:"count"`
}

// StudentRiskEntry holds per-student risk summary.
type StudentRiskEntry struct {
	StudentID     string `json:"studentId"`
	TotalEvents   int64  `json:"totalEvents"`
	CriticalCount int64  `json:"criticalCount"`
	WarningCount  int64  `json:"warningCount"`
}

// HourlyStatEntry holds one hour's event counts.
type HourlyStatEntry struct {
	Hour           string `json:"hour"`
	EventCount     int64  `json:"eventCount"`
	CriticalCount  int64  `json:"criticalCount"`
}

// OverviewResponse combines all analytics into a single response.
type OverviewResponse struct {
	Risk         *RiskDistribution  `json:"risk"`
	Health       *SystemHealth      `json:"health"`
	Violations   []ViolationEntry   `json:"violations"`
	TopStudents  []StudentRiskEntry `json:"topStudents"`
	HourlyStats  []HourlyStatEntry  `json:"hourlyStats"`
	GeneratedAt  string             `json:"generatedAt"`
}

// ==========================================================================
// Handlers
// ==========================================================================

func (h *AnalyticsHandler) handleRiskDistribution(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	result, err := h.queryRiskDistribution(r.Context(), orgID)
	if err != nil {
		h.logger.Error("risk distribution query failed", zap.Error(err))
		h.jsonError(w, "Failed to query risk distribution", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, result, http.StatusOK)
}

func (h *AnalyticsHandler) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	result, err := h.querySystemHealth(r.Context(), orgID)
	if err != nil {
		h.logger.Error("system health query failed", zap.Error(err))
		h.jsonError(w, "Failed to query system health", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, result, http.StatusOK)
}

func (h *AnalyticsHandler) handleTopViolations(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	limit := h.parseLimit(r, 20)
	result, err := h.queryTopViolations(r.Context(), orgID, limit)
	if err != nil {
		h.logger.Error("top violations query failed", zap.Error(err))
		h.jsonError(w, "Failed to query violations", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, result, http.StatusOK)
}

func (h *AnalyticsHandler) handleStudentRisk(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	limit := h.parseLimit(r, 10)
	result, err := h.queryStudentRisk(r.Context(), orgID, limit)
	if err != nil {
		h.logger.Error("student risk query failed", zap.Error(err))
		h.jsonError(w, "Failed to query student risk", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, result, http.StatusOK)
}

func (h *AnalyticsHandler) handleHourlyStats(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	result, err := h.queryHourlyStats(r.Context(), orgID)
	if err != nil {
		h.logger.Error("hourly stats query failed", zap.Error(err))
		h.jsonError(w, "Failed to query hourly stats", http.StatusInternalServerError)
		return
	}
	h.jsonResponse(w, result, http.StatusOK)
}

func (h *AnalyticsHandler) handleOverview(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	ctx := r.Context()

	risk, err := h.queryRiskDistribution(ctx, orgID)
	if err != nil {
		h.logger.Error("overview: risk query failed", zap.Error(err))
		risk = &RiskDistribution{BySeverity: make(map[string]int64)}
	}

	health, err := h.querySystemHealth(ctx, orgID)
	if err != nil {
		h.logger.Error("overview: health query failed", zap.Error(err))
		health = &SystemHealth{}
	}

	violations, err := h.queryTopViolations(ctx, orgID, 10)
	if err != nil {
		h.logger.Error("overview: violations query failed", zap.Error(err))
		violations = []ViolationEntry{}
	}

	students, err := h.queryStudentRisk(ctx, orgID, 10)
	if err != nil {
		h.logger.Error("overview: student risk query failed", zap.Error(err))
		students = []StudentRiskEntry{}
	}

	hourly, err := h.queryHourlyStats(ctx, orgID)
	if err != nil {
		h.logger.Error("overview: hourly stats query failed", zap.Error(err))
		hourly = []HourlyStatEntry{}
	}

	overview := OverviewResponse{
		Risk:        risk,
		Health:      health,
		Violations:  violations,
		TopStudents: students,
		HourlyStats: hourly,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	h.jsonResponse(w, overview, http.StatusOK)
}

// ==========================================================================
// ClickHouse Queries
// ==========================================================================

func (h *AnalyticsHandler) queryRiskDistribution(ctx context.Context, orgID string) (*RiskDistribution, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := &RiskDistribution{
		BySeverity: make(map[string]int64),
	}

	// Query severity counts.
	var query string
	var rows driver.Rows
	var err error

	if orgID == "*" || orgID == "" {
		query = `SELECT severity, count() AS cnt FROM proctoring_events GROUP BY severity ORDER BY cnt DESC`
		rows, err = h.chConn.Query(qctx, query)
	} else {
		query = `SELECT severity, count() AS cnt FROM proctoring_events WHERE org_id = ? GROUP BY severity ORDER BY cnt DESC`
		rows, err = h.chConn.Query(qctx, query, orgID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var sev string
		var cnt uint64
		if err := rows.Scan(&sev, &cnt); err != nil {
			return nil, err
		}
		result.BySeverity[sev] = int64(cnt)
		result.TotalEvents += int64(cnt)
	}

	// Calculate percentages.
	if result.TotalEvents > 0 {
		info := result.BySeverity["info"]
		warning := result.BySeverity["warning"]
		critical := result.BySeverity["critical"]
		total := float64(result.TotalEvents)
		result.Honest = float64(info) / total * 100
		result.Suspicious = float64(warning) / total * 100
		result.Cheaters = float64(critical) / total * 100
	}

	return result, nil
}

func (h *AnalyticsHandler) querySystemHealth(ctx context.Context, orgID string) (*SystemHealth, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	result := &SystemHealth{}

	// Writer metrics (in-memory, no ClickHouse query needed).
	metrics := h.chMeta.Metrics()
	result.TotalFlushed = metrics.TotalFlushed
	result.TotalDropped = metrics.TotalDropped
	result.FlushCount = metrics.FlushCount
	result.FlushErrors = metrics.FlushErrors
	result.BufferSize = metrics.BufferSize
	result.OverflowSize = metrics.OverflowSize

	// Total events from ClickHouse.
	if orgID == "*" || orgID == "" {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM proctoring_events`)
		_ = row.Scan(&result.TotalEvents)
	} else {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM proctoring_events WHERE org_id = ?`, orgID)
		_ = row.Scan(&result.TotalEvents)
	}

	// Unique students.
	if orgID == "*" || orgID == "" {
		row := h.chConn.QueryRow(qctx, `SELECT count(DISTINCT student_id) FROM proctoring_events`)
		_ = row.Scan(&result.UniqueStudents)
	} else {
		row := h.chConn.QueryRow(qctx, `SELECT count(DISTINCT student_id) FROM proctoring_events WHERE org_id = ?`, orgID)
		_ = row.Scan(&result.UniqueStudents)
	}

	// Unique sessions.
	if orgID == "*" || orgID == "" {
		row := h.chConn.QueryRow(qctx, `SELECT count(DISTINCT session_id) FROM proctoring_events`)
		_ = row.Scan(&result.UniqueSessions)
	} else {
		row := h.chConn.QueryRow(qctx, `SELECT count(DISTINCT session_id) FROM proctoring_events WHERE org_id = ?`, orgID)
		_ = row.Scan(&result.UniqueSessions)
	}

	// Critical events from materialized view.
	if orgID == "*" || orgID == "" {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM critical_events_recent`)
		_ = row.Scan(&result.CriticalEvents)
	} else {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM critical_events_recent WHERE org_id = ?`, orgID)
		_ = row.Scan(&result.CriticalEvents)
	}

	// Events in last 5 minutes.
	fiveMinAgo := time.Now().UTC().Add(-5 * time.Minute)
	if orgID == "*" || orgID == "" {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM proctoring_events WHERE server_timestamp > ?`, fiveMinAgo)
		_ = row.Scan(&result.EventsLast5Min)
	} else {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM proctoring_events WHERE org_id = ? AND server_timestamp > ?`, orgID, fiveMinAgo)
		_ = row.Scan(&result.EventsLast5Min)
	}

	// Events in last 1 hour.
	oneHourAgo := time.Now().UTC().Add(-1 * time.Hour)
	if orgID == "*" || orgID == "" {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM proctoring_events WHERE server_timestamp > ?`, oneHourAgo)
		_ = row.Scan(&result.EventsLast1Hour)
	} else {
		row := h.chConn.QueryRow(qctx, `SELECT count() FROM proctoring_events WHERE org_id = ? AND server_timestamp > ?`, orgID, oneHourAgo)
		_ = row.Scan(&result.EventsLast1Hour)
	}

	return result, nil
}

func (h *AnalyticsHandler) queryTopViolations(ctx context.Context, orgID string, limit int) ([]ViolationEntry, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var rows driver.Rows
	var err error

	if orgID == "*" || orgID == "" {
		rows, err = h.chConn.Query(qctx,
			`SELECT event_type, severity, count() AS cnt
			 FROM proctoring_events
			 WHERE severity IN ('warning', 'critical')
			 GROUP BY event_type, severity
			 ORDER BY cnt DESC
			 LIMIT ?`, limit)
	} else {
		rows, err = h.chConn.Query(qctx,
			`SELECT event_type, severity, count() AS cnt
			 FROM proctoring_events
			 WHERE org_id = ? AND severity IN ('warning', 'critical')
			 GROUP BY event_type, severity
			 ORDER BY cnt DESC
			 LIMIT ?`, orgID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ViolationEntry
	for rows.Next() {
		var v ViolationEntry
		var cnt uint64
		if err := rows.Scan(&v.EventType, &v.Severity, &cnt); err != nil {
			return nil, err
		}
		v.Count = int64(cnt)
		result = append(result, v)
	}

	if result == nil {
		result = []ViolationEntry{}
	}
	return result, nil
}

func (h *AnalyticsHandler) queryStudentRisk(ctx context.Context, orgID string, limit int) ([]StudentRiskEntry, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var rows driver.Rows
	var err error

	if orgID == "*" || orgID == "" {
		rows, err = h.chConn.Query(qctx,
			`SELECT student_id,
			        sum(total_events) AS total,
			        sum(critical_count) AS crit,
			        sum(warning_count) AS warn
			 FROM student_session_summary
			 GROUP BY student_id
			 ORDER BY crit DESC
			 LIMIT ?`, limit)
	} else {
		rows, err = h.chConn.Query(qctx,
			`SELECT student_id,
			        sum(total_events) AS total,
			        sum(critical_count) AS crit,
			        sum(warning_count) AS warn
			 FROM student_session_summary
			 WHERE org_id = ?
			 GROUP BY student_id
			 ORDER BY crit DESC
			 LIMIT ?`, orgID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []StudentRiskEntry
	for rows.Next() {
		var s StudentRiskEntry
		var total, crit, warn uint64
		if err := rows.Scan(&s.StudentID, &total, &crit, &warn); err != nil {
			return nil, err
		}
		s.TotalEvents = int64(total)
		s.CriticalCount = int64(crit)
		s.WarningCount = int64(warn)
		result = append(result, s)
	}

	if result == nil {
		result = []StudentRiskEntry{}
	}
	return result, nil
}

func (h *AnalyticsHandler) queryHourlyStats(ctx context.Context, orgID string) ([]HourlyStatEntry, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var rows driver.Rows
	var err error

	if orgID == "*" || orgID == "" {
		rows, err = h.chConn.Query(qctx,
			`SELECT hour,
			        sum(event_count) AS total,
			        sumIf(event_count, severity = 'critical') AS crit
			 FROM hourly_event_stats
			 GROUP BY hour
			 ORDER BY hour DESC
			 LIMIT 24`)
	} else {
		rows, err = h.chConn.Query(qctx,
			`SELECT hour,
			        sum(event_count) AS total,
			        sumIf(event_count, severity = 'critical') AS crit
			 FROM hourly_event_stats
			 WHERE org_id = ?
			 GROUP BY hour
			 ORDER BY hour DESC
			 LIMIT 24`, orgID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HourlyStatEntry
	for rows.Next() {
		var entry HourlyStatEntry
		var hour time.Time
		var total, crit uint64
		if err := rows.Scan(&hour, &total, &crit); err != nil {
			return nil, err
		}
		entry.Hour = hour.Format("2006-01-02T15:04:05Z")
		entry.EventCount = int64(total)
		entry.CriticalCount = int64(crit)
		result = append(result, entry)
	}

	if result == nil {
		result = []HourlyStatEntry{}
	}
	return result, nil
}

// ==========================================================================
// Infrastructure Stats Handler
// ==========================================================================

// handleInfrastructureStats returns real-time Docker container and system metrics.
//
//	GET /api/v1/analytics/infrastructure-stats
func (h *AnalyticsHandler) handleInfrastructureStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	stats, err := h.infraCollector.Collect(ctx)
	if err != nil {
		h.logger.Error("Failed to collect infrastructure stats", zap.Error(err))
		h.jsonError(w, "Failed to collect infrastructure metrics", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, stats, http.StatusOK)
}

// ==========================================================================
// Auth & Helpers (reuse same JWT pattern as AdminHandler)
// ==========================================================================

func (h *AnalyticsHandler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
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

func (h *AnalyticsHandler) verifyToken(ctx context.Context, tokenStr string) (*entity.User, error) {
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

// resolveOrgID determines the org_id for queries.
// Super admins can specify ?org_id=X, or omit for all orgs.
// Non-super-admins always get their own org_id.
func (h *AnalyticsHandler) resolveOrgID(r *http.Request) string {
	user := getUserFromContext(r.Context())
	if user == nil {
		return ""
	}

	if user.IsSuperAdmin() {
		if qOrgID := r.URL.Query().Get("org_id"); qOrgID != "" {
			return qOrgID
		}
		return "*" // All orgs.
	}

	return user.OrgID
}

func (h *AnalyticsHandler) parseLimit(r *http.Request, defaultLimit int) int {
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 100 {
			return parsed
		}
	}
	return defaultLimit
}

func (h *AnalyticsHandler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", zap.Error(err))
	}
}

func (h *AnalyticsHandler) jsonError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// splitToken splits a JWT into its 3 parts.
func splitToken(s string) []string {
	var parts [3]string
	idx := 0
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			if idx >= 2 {
				return nil
			}
			parts[idx] = s[start:i]
			idx++
			start = i + 1
		}
	}
	if idx != 2 {
		return nil
	}
	parts[2] = s[start:]
	result := parts[:]
	return result
}

// ==========================================================================
// Enterprise Analytics — Exam Performance (Migration 006)
// ==========================================================================

// ExamPerformanceEntry holds per-exam aggregated metrics.
type ExamPerformanceEntry struct {
	OrgID          string  `json:"orgId"`
	ExamID         string  `json:"examId"`
	TotalEvents    uint64  `json:"totalEvents"`
	CriticalCount  uint64  `json:"criticalCount"`
	WarningCount   uint64  `json:"warningCount"`
	UniqueStudents uint64  `json:"uniqueStudents"`
	UniqueSessions uint64  `json:"uniqueSessions"`
	AvgConfidence  float64 `json:"avgConfidence"`
}

// handleExamPerformance returns exam-level performance metrics.
//
//	GET /api/v1/analytics/exam-performance?limit=50
func (h *AnalyticsHandler) handleExamPerformance(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := parseInt(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := `
		SELECT org_id, exam_id,
			sum(total_events), sum(critical_count), sum(warning_count),
			sum(unique_students), sum(unique_sessions),
			if(sum(confidence_count) > 0, sum(avg_confidence * confidence_count) / sum(confidence_count), 0)
		FROM exam_performance_summary`

	var args []interface{}
	if orgID != "*" && orgID != "" {
		query += ` WHERE org_id = ?`
		args = append(args, orgID)
	}
	query += ` GROUP BY org_id, exam_id ORDER BY sum(critical_count) DESC LIMIT ?`
	args = append(args, limit)

	rows, err := h.chConn.Query(ctx, query, args...)
	if err != nil {
		h.jsonError(w, "Failed to query exam performance", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []ExamPerformanceEntry
	for rows.Next() {
		var e ExamPerformanceEntry
		if err := rows.Scan(&e.OrgID, &e.ExamID, &e.TotalEvents, &e.CriticalCount,
			&e.WarningCount, &e.UniqueStudents, &e.UniqueSessions, &e.AvgConfidence); err != nil {
			continue
		}
		results = append(results, e)
	}

	h.jsonResponse(w, results, http.StatusOK)
}

// ==========================================================================
// Enterprise Analytics — Realtime Org Health (Migration 006)
// ==========================================================================

// RealtimeHealthEntry holds a 5-minute health window for an org.
type RealtimeHealthEntry struct {
	OrgID          string  `json:"orgId"`
	WindowStart    string  `json:"windowStart"`
	EventCount     uint64  `json:"eventCount"`
	CriticalCount  uint64  `json:"criticalCount"`
	ActiveSessions uint64  `json:"activeSessions"`
	AvgConfidence  float64 `json:"avgConfidence"`
}

// handleRealtimeHealth returns 5-minute windowed org health metrics.
//
//	GET /api/v1/analytics/realtime-health?windows=24
func (h *AnalyticsHandler) handleRealtimeHealth(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	windows := 24 // last 2 hours by default (24 × 5min)
	if n := r.URL.Query().Get("windows"); n != "" {
		if val, err := parseInt(n); err == nil && val > 0 && val <= 288 {
			windows = val
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := `
		SELECT org_id, window_start,
			sum(event_count), sum(critical_count), sum(active_sessions),
			if(sum(confidence_count) > 0, sum(avg_confidence * confidence_count) / sum(confidence_count), 0)
		FROM realtime_org_health
		WHERE window_start > now() - INTERVAL ? MINUTE`

	args := []interface{}{windows * 5}
	if orgID != "*" && orgID != "" {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}
	query += ` GROUP BY org_id, window_start ORDER BY window_start DESC`

	rows, err := h.chConn.Query(ctx, query, args...)
	if err != nil {
		h.jsonError(w, "Failed to query realtime health", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []RealtimeHealthEntry
	for rows.Next() {
		var e RealtimeHealthEntry
		var windowTime time.Time
		if err := rows.Scan(&e.OrgID, &windowTime, &e.EventCount, &e.CriticalCount,
			&e.ActiveSessions, &e.AvgConfidence); err != nil {
			continue
		}
		e.WindowStart = windowTime.Format(time.RFC3339)
		results = append(results, e)
	}

	h.jsonResponse(w, results, http.StatusOK)
}

// ==========================================================================
// Enterprise Analytics — Daily Violation Trends (Migration 006)
// ==========================================================================

// DailyTrendEntry holds daily violation counts by type and severity.
type DailyTrendEntry struct {
	OrgID     string `json:"orgId"`
	Day       string `json:"day"`
	EventType string `json:"eventType"`
	Severity  string `json:"severity"`
	Count     uint64 `json:"count"`
}

// handleDailyTrends returns daily violation trend data for charting.
//
//	GET /api/v1/analytics/daily-trends?days=30
func (h *AnalyticsHandler) handleDailyTrends(w http.ResponseWriter, r *http.Request) {
	orgID := h.resolveOrgID(r)
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if val, err := parseInt(d); err == nil && val > 0 && val <= 365 {
			days = val
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := `
		SELECT org_id, day, event_type, severity, sum(count)
		FROM daily_violation_trends
		WHERE day >= today() - ?`

	args := []interface{}{days}
	if orgID != "*" && orgID != "" {
		query += ` AND org_id = ?`
		args = append(args, orgID)
	}
	query += ` GROUP BY org_id, day, event_type, severity ORDER BY day DESC, sum(count) DESC`

	rows, err := h.chConn.Query(ctx, query, args...)
	if err != nil {
		h.jsonError(w, "Failed to query daily trends", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var results []DailyTrendEntry
	for rows.Next() {
		var e DailyTrendEntry
		var dayTime time.Time
		if err := rows.Scan(&e.OrgID, &dayTime, &e.EventType, &e.Severity, &e.Count); err != nil {
			continue
		}
		e.Day = dayTime.Format("2006-01-02")
		results = append(results, e)
	}

	h.jsonResponse(w, results, http.StatusOK)
}

// parseInt parses a string to int (helper for query params).
func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// Sentinel errors for auth.
var (
	errInvalidToken = errType("invalid token")
	errTokenExpired = errType("token expired")
	errUserNotFound = errType("user not found")
)

type errType string

func (e errType) Error() string { return string(e) }
