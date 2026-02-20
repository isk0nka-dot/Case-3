// Package forensic implements the Integrity Scoring and Verdict Engine.
//
// The scorer aggregates all ClickHouse events for a session and computes
// a weighted Integrity Score (0-100%) using a penalty-based algorithm.
//
// Penalty weights (applied per-event occurrence):
//
//	Liveness check failed:     -8   per occurrence
//	Face spoof detected:       -15  per occurrence
//	Face mismatch:             -12  per occurrence
//	Face not detected:         -3   per occurrence (max -15)
//	Tab switch:                -5   per occurrence (max -25)
//	Copy/paste attempt:        -3   per occurrence (max -12)
//	Fullscreen exit:           -5   per occurrence (max -20)
//	External display:          -10  per occurrence
//	Virtual machine:           -20  (one-time)
//	Hardware ID mismatch:      -20  (one-time)
//	Phone detected:            -10  per occurrence
//	Book detected:             -5   per occurrence (max -15)
//	Gaze deviation:            -2   per occurrence (max -20)
//	Audio anomaly:             -3   per occurrence (max -15)
//	Second speaker:            -10  per occurrence (max -20)
//	Print screen attempt:      -3   per occurrence (max -9)
//	Context menu attempt:      -1   per occurrence (max -5)
//	Remote access detected:    -20  (one-time)
//	Forbidden process:         -15  per occurrence (max -30)
//
// The Verdict Engine classifies the final score:
//
//	Clean:   score >= 80   — No significant integrity violations
//	Warning: score >= 50   — Moderate violations requiring human review
//	Fraud:   score <  50   — Critical violations indicating dishonesty
//
// Each verdict includes a human-readable justification listing the top
// contributing factors.
package forensic

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// IntegrityScore is the computed score for a session.
type IntegrityScore struct {
	SessionID     string            `json:"sessionId"`
	StudentID     string            `json:"studentId"`
	ExamID        string            `json:"examId"`
	OrgID         string            `json:"orgId"`
	Score         float64           `json:"score"`         // 0-100
	Verdict       string            `json:"verdict"`       // clean | warning | fraud
	VerdictLabel  string            `json:"verdictLabel"`  // Russian label
	Justification string            `json:"justification"` // Human-readable explanation
	Penalties     []PenaltyEntry    `json:"penalties"`
	EventSummary  map[string]int    `json:"eventSummary"`  // eventType -> count
	TopFactors    []string          `json:"topFactors"`    // Top 5 contributing factors
	DurationSec   int64             `json:"durationSec"`
	TotalEvents   int               `json:"totalEvents"`
	CriticalCount int               `json:"criticalCount"`
	WarningCount  int               `json:"warningCount"`
	ComputedAt    string            `json:"computedAt"`
}

// PenaltyEntry records a single penalty applied to the integrity score.
type PenaltyEntry struct {
	EventType   string  `json:"eventType"`
	Count       int     `json:"count"`
	PenaltyPer  float64 `json:"penaltyPer"`
	MaxPenalty  float64 `json:"maxPenalty"`
	Applied     float64 `json:"applied"` // actual deduction
	Description string  `json:"description"`
}

// GazePoint is a single gaze telemetry record for heatmap generation.
type GazePoint struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Timestamp float64 `json:"timestamp"`
}

// VoiceBiometricResult holds speaker consistency analysis results.
type VoiceBiometricResult struct {
	TotalSegments       int     `json:"totalSegments"`
	MatchedSegments     int     `json:"matchedSegments"`
	MismatchedSegments  int     `json:"mismatchedSegments"`
	ConsistencyScore    float64 `json:"consistencyScore"`    // 0-1
	SpeakerChangeCount  int     `json:"speakerChangeCount"`
	PrimarySpeakerRatio float64 `json:"primarySpeakerRatio"` // 0-1
	Verdict             string  `json:"verdict"`             // consistent | suspicious | anomalous
}

// ForensicReport is the complete forensic report for a session.
type ForensicReport struct {
	// Report metadata
	ReportID     string    `json:"reportId"`
	GeneratedAt  string    `json:"generatedAt"`
	ReportHash   string    `json:"reportHash"` // SHA-256 of JSON content

	// Session identity
	SessionID    string    `json:"sessionId"`
	StudentID    string    `json:"studentId"`
	ExamID       string    `json:"examId"`
	OrgID        string    `json:"orgId"`

	// Scores and verdict
	Integrity    IntegrityScore        `json:"integrity"`
	Voice        VoiceBiometricResult  `json:"voiceBiometric"`

	// Violation timeline
	Timeline     []TimelineEntry       `json:"timeline"`

	// Device fingerprint
	DeviceInfo   DeviceInfo            `json:"deviceInfo"`

	// Gaze heatmap data
	GazeData     []GazePoint           `json:"gazeData"`

	// Forensic ledger
	LedgerSummary LedgerSummary        `json:"ledgerSummary"`
}

// TimelineEntry is a violation in the timeline.
type TimelineEntry struct {
	Timestamp     string  `json:"timestamp"`
	VideoSec      int64   `json:"videoSec"`
	EventType     string  `json:"eventType"`
	Severity      string  `json:"severity"`
	Label         string  `json:"label"`
	Confidence    float64 `json:"confidence"`
	Source        string  `json:"source"`
}

// DeviceInfo is device metadata from the session.
type DeviceInfo struct {
	UserAgent    string `json:"userAgent"`
	Resolution   string `json:"resolution"`
	IPAddress    string `json:"ipAddress"`
	Region       string `json:"region"`
	Timezone     int32  `json:"timezone"`
}

// LedgerSummary is a summary of the forensic ledger state.
type LedgerSummary struct {
	TotalFragments int    `json:"totalFragments"`
	VerifiedOK     int    `json:"verifiedOk"`
	ChainValid     bool   `json:"chainValid"`
	S3Verified     int    `json:"s3Verified"`
	S3Mismatches   int    `json:"s3Mismatches"`
}

// ---------------------------------------------------------------------------
// Penalty Configuration
// ---------------------------------------------------------------------------

type penaltyRule struct {
	eventType   string
	penaltyPer  float64
	maxPenalty  float64
	description string
}

var penaltyRules = []penaltyRule{
	{"LIVENESS_CHECK_FAILED", 8, 0, "Провал проверки живости"},
	{"FACE_SPOOF_DETECTED", 15, 0, "Обнаружена подмена лица"},
	{"FACE_MISMATCH", 12, 0, "Несоответствие лица"},
	{"FACE_NOT_DETECTED", 3, 15, "Лицо не обнаружено"},
	{"TAB_SWITCH", 5, 25, "Переключение вкладки"},
	{"COPY_PASTE_ATTEMPT", 3, 12, "Попытка копирования/вставки"},
	{"FULLSCREEN_EXIT", 5, 20, "Выход из полноэкранного режима"},
	{"EXTERNAL_DISPLAY_DETECTED", 10, 0, "Внешний дисплей"},
	{"VIRTUAL_MACHINE_DETECTED", 20, 20, "Виртуальная машина"},
	{"HARDWARE_ID_MISMATCH", 20, 20, "Несоответствие устройства"},
	{"PHONE_DETECTED", 10, 0, "Обнаружен телефон"},
	{"BOOK_DETECTED", 5, 15, "Обнаружена книга/шпаргалка"},
	{"GAZE_DEVIATION", 2, 20, "Отвод взгляда"},
	{"AUDIO_ANOMALY", 3, 15, "Аудио-аномалия"},
	{"SECOND_SPEAKER_DETECTED", 10, 20, "Обнаружен второй голос"},
	{"PRINT_SCREEN_ATTEMPT", 3, 9, "Попытка скриншота"},
	{"CONTEXT_MENU_ATTEMPT", 1, 5, "Контекстное меню"},
	{"REMOTE_ACCESS_DETECTED", 20, 20, "Удалённый доступ"},
	{"FORBIDDEN_PROCESS_DETECTED", 15, 30, "Запрещённый процесс"},
	{"MULTIPLE_PERSONS", 8, 0, "Множественные лица"},
	{"EARBUDS_DETECTED", 8, 24, "Обнаружены наушники"},
	{"WHISPER_DETECTED", 5, 15, "Обнаружен шёпот"},
	{"AUDIO_PLAYBACK_DETECTED", 10, 20, "Обнаружено воспроизведение аудио"},
}

// ---------------------------------------------------------------------------
// Scorer
// ---------------------------------------------------------------------------

// Scorer computes integrity scores for proctoring sessions.
type Scorer struct {
	chConn driver.Conn
	logger *zap.Logger
}

// NewScorer creates a new integrity scorer.
func NewScorer(chConn driver.Conn, logger *zap.Logger) *Scorer {
	return &Scorer{
		chConn: chConn,
		logger: logger.Named("forensic_scorer"),
	}
}

// ComputeScore aggregates events from ClickHouse and computes the integrity score.
func (s *Scorer) ComputeScore(ctx context.Context, sessionID string) (*IntegrityScore, error) {
	qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Query event type counts for this session
	rows, err := s.chConn.Query(qctx, `
		SELECT
			event_type,
			severity,
			count() AS cnt
		FROM proctoring_events
		WHERE session_id = ?
		GROUP BY event_type, severity
		ORDER BY cnt DESC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("forensic scorer: query failed: %w", err)
	}
	defer rows.Close()

	// Aggregate counts
	eventCounts := make(map[string]int)
	totalEvents := 0
	criticalCount := 0
	warningCount := 0

	for rows.Next() {
		var eventType, severity string
		var cnt uint64
		if err := rows.Scan(&eventType, &severity, &cnt); err != nil {
			return nil, fmt.Errorf("forensic scorer: scan failed: %w", err)
		}
		eventCounts[eventType] += int(cnt)
		totalEvents += int(cnt)
		switch severity {
		case "critical":
			criticalCount += int(cnt)
		case "warning":
			warningCount += int(cnt)
		}
	}

	// Query session metadata
	var studentID, examID, orgID string
	var firstTime, lastTime time.Time
	metaRow := s.chConn.QueryRow(qctx, `
		SELECT
			any(student_id), any(exam_id), any(org_id),
			min(server_timestamp), max(server_timestamp)
		FROM proctoring_events
		WHERE session_id = ?`,
		sessionID,
	)
	if err := metaRow.Scan(&studentID, &examID, &orgID, &firstTime, &lastTime); err != nil {
		// Non-fatal — use defaults
		s.logger.Warn("forensic scorer: metadata query failed", zap.Error(err))
	}

	durationSec := int64(0)
	if !firstTime.IsZero() && !lastTime.IsZero() {
		durationSec = int64(lastTime.Sub(firstTime).Seconds())
	}

	// Apply penalty rules
	score := 100.0
	var penalties []PenaltyEntry

	for _, rule := range penaltyRules {
		count := eventCounts[rule.eventType]
		if count == 0 {
			continue
		}

		rawPenalty := float64(count) * rule.penaltyPer
		if rule.maxPenalty > 0 && rawPenalty > rule.maxPenalty {
			rawPenalty = rule.maxPenalty
		}

		score -= rawPenalty

		penalties = append(penalties, PenaltyEntry{
			EventType:   rule.eventType,
			Count:       count,
			PenaltyPer:  rule.penaltyPer,
			MaxPenalty:   rule.maxPenalty,
			Applied:      rawPenalty,
			Description: rule.description,
		})
	}

	// Clamp score
	score = math.Max(0, math.Min(100, score))

	// Sort penalties by applied amount (descending)
	sort.Slice(penalties, func(i, j int) bool {
		return penalties[i].Applied > penalties[j].Applied
	})

	// Determine verdict
	verdict, verdictLabel := computeVerdict(score)

	// Build justification
	topFactors := buildTopFactors(penalties)
	justification := buildJustification(verdict, score, topFactors, totalEvents, criticalCount)

	result := &IntegrityScore{
		SessionID:     sessionID,
		StudentID:     studentID,
		ExamID:        examID,
		OrgID:         orgID,
		Score:         math.Round(score*10) / 10,
		Verdict:       verdict,
		VerdictLabel:  verdictLabel,
		Justification: justification,
		Penalties:     penalties,
		EventSummary:  eventCounts,
		TopFactors:    topFactors,
		DurationSec:   durationSec,
		TotalEvents:   totalEvents,
		CriticalCount: criticalCount,
		WarningCount:  warningCount,
		ComputedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	s.logger.Info("forensic score computed",
		zap.String("session_id", sessionID),
		zap.Float64("score", result.Score),
		zap.String("verdict", verdict),
		zap.Int("total_events", totalEvents),
		zap.Int("penalty_types", len(penalties)),
	)

	return result, nil
}

// QueryGazeData retrieves gaze telemetry points for heatmap generation.
func (s *Scorer) QueryGazeData(ctx context.Context, sessionID string) ([]GazePoint, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := s.chConn.Query(qctx, `
		SELECT
			head_yaw,
			head_pitch,
			video_timestamp_sec
		FROM proctoring_events
		WHERE session_id = ?
		  AND event_type IN ('GAZE_TELEMETRY', 'HEAD_POSE_TELEMETRY', 'GAZE_DEVIATION')
		  AND head_yaw != 0
		ORDER BY server_timestamp ASC
		LIMIT 5000`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("forensic scorer: gaze query failed: %w", err)
	}
	defer rows.Close()

	var points []GazePoint
	for rows.Next() {
		var yaw, pitch float32
		var videoTs float64
		if err := rows.Scan(&yaw, &pitch, &videoTs); err != nil {
			continue
		}
		// Normalize yaw (-90..+90) to 0..1 x-axis
		// Normalize pitch (-90..+90) to 0..1 y-axis
		x := (float64(yaw) + 90.0) / 180.0
		y := (float64(pitch) + 90.0) / 180.0
		points = append(points, GazePoint{
			X:         math.Max(0, math.Min(1, x)),
			Y:         math.Max(0, math.Min(1, y)),
			Timestamp: videoTs,
		})
	}

	if points == nil {
		points = []GazePoint{}
	}
	return points, nil
}

// QueryVoiceBiometric analyzes speaker consistency across the session.
func (s *Scorer) QueryVoiceBiometric(ctx context.Context, sessionID string) (*VoiceBiometricResult, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Query audio segments with speaker match data
	rows, err := s.chConn.Query(qctx, `
		SELECT
			speaker_match,
			speaker_count,
			audio_classification,
			vad_active
		FROM proctoring_events
		WHERE session_id = ?
		  AND event_type IN ('VOICE_ACTIVITY', 'AUDIO_LEVEL_TELEMETRY', 'SECOND_SPEAKER_DETECTED', 'WHISPER_DETECTED')
		  AND vad_active = 1
		ORDER BY server_timestamp ASC
		LIMIT 3000`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("forensic scorer: voice query failed: %w", err)
	}
	defer rows.Close()

	result := &VoiceBiometricResult{}
	speakerCounts := make(map[uint8]int) // speakerCount -> occurrences

	for rows.Next() {
		var speakerMatch bool
		var speakerCount, vadActive uint8
		var audioClass string
		if err := rows.Scan(&speakerMatch, &speakerCount, &audioClass, &vadActive); err != nil {
			continue
		}

		result.TotalSegments++
		speakerCounts[speakerCount]++

		if speakerMatch {
			result.MatchedSegments++
		} else {
			result.MismatchedSegments++
		}

		if speakerCount > 1 {
			result.SpeakerChangeCount++
		}
	}

	if result.TotalSegments > 0 {
		result.ConsistencyScore = float64(result.MatchedSegments) / float64(result.TotalSegments)
		// Primary speaker ratio: % of segments with exactly 1 speaker
		result.PrimarySpeakerRatio = float64(speakerCounts[1]) / float64(result.TotalSegments)
	} else {
		result.ConsistencyScore = 1.0
		result.PrimarySpeakerRatio = 1.0
	}

	// Voice verdict
	if result.ConsistencyScore >= 0.9 && result.SpeakerChangeCount <= 2 {
		result.Verdict = "consistent"
	} else if result.ConsistencyScore >= 0.7 || result.SpeakerChangeCount <= 5 {
		result.Verdict = "suspicious"
	} else {
		result.Verdict = "anomalous"
	}

	return result, nil
}

// QueryTimeline retrieves violation events for the timeline.
func (s *Scorer) QueryTimeline(ctx context.Context, sessionID string) ([]TimelineEntry, error) {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := s.chConn.Query(qctx, `
		SELECT
			server_timestamp,
			video_timestamp_sec,
			event_type,
			severity,
			label,
			confidence,
			source
		FROM proctoring_events
		WHERE session_id = ?
		  AND severity IN ('warning', 'critical')
		ORDER BY server_timestamp ASC
		LIMIT 500`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("forensic scorer: timeline query failed: %w", err)
	}
	defer rows.Close()

	var timeline []TimelineEntry
	for rows.Next() {
		var ts time.Time
		var videoTs float64
		var conf float32
		var e TimelineEntry
		if err := rows.Scan(&ts, &videoTs, &e.EventType, &e.Severity, &e.Label, &conf, &e.Source); err != nil {
			continue
		}
		e.Timestamp = ts.Format(time.RFC3339)
		e.VideoSec = int64(videoTs)
		e.Confidence = float64(conf)
		timeline = append(timeline, e)
	}

	if timeline == nil {
		timeline = []TimelineEntry{}
	}
	return timeline, nil
}

// QueryDeviceInfo retrieves device metadata from the first event.
func (s *Scorer) QueryDeviceInfo(ctx context.Context, sessionID string) (*DeviceInfo, error) {
	qctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var info DeviceInfo
	row := s.chConn.QueryRow(qctx, `
		SELECT
			any(user_agent),
			any(resolution),
			any(ip_address),
			any(region),
			any(timezone_offset_min)
		FROM proctoring_events
		WHERE session_id = ?`,
		sessionID,
	)
	if err := row.Scan(&info.UserAgent, &info.Resolution, &info.IPAddress, &info.Region, &info.Timezone); err != nil {
		return &DeviceInfo{}, nil
	}
	return &info, nil
}

// ---------------------------------------------------------------------------
// Verdict Engine
// ---------------------------------------------------------------------------

func computeVerdict(score float64) (string, string) {
	if score >= 80 {
		return "clean", "Чистая сессия"
	}
	if score >= 50 {
		return "warning", "Требует проверки"
	}
	return "fraud", "Подозрение на нарушение"
}

func buildTopFactors(penalties []PenaltyEntry) []string {
	var factors []string
	for i, p := range penalties {
		if i >= 5 {
			break
		}
		factors = append(factors, fmt.Sprintf("%s: %d случ. (-%s%%)",
			p.Description, p.Count, formatFloat(p.Applied)))
	}
	return factors
}

func buildJustification(verdict string, score float64, factors []string, totalEvents, criticalCount int) string {
	var sb strings.Builder

	switch verdict {
	case "clean":
		sb.WriteString(fmt.Sprintf("Сессия завершена с оценкой целостности %.1f%%. ", score))
		sb.WriteString("Значительных нарушений не обнаружено. ")
	case "warning":
		sb.WriteString(fmt.Sprintf("Сессия имеет оценку целостности %.1f%% и требует ручной проверки. ", score))
	case "fraud":
		sb.WriteString(fmt.Sprintf("КРИТИЧЕСКАЯ УГРОЗА: Оценка целостности %.1f%%. ", score))
		sb.WriteString(fmt.Sprintf("Обнаружено %d критических нарушений из %d событий. ", criticalCount, totalEvents))
	}

	if len(factors) > 0 {
		sb.WriteString("Основные факторы: ")
		sb.WriteString(strings.Join(factors, "; "))
		sb.WriteString(".")
	}

	return sb.String()
}

func formatFloat(f float64) string {
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.1f", f)
}
