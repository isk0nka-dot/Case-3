package forensic

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

// ExternalReportData holds the data needed to generate the external bilingual
// PDF report for LMS partners. It mirrors the external JSON report DTO.
type ExternalReportData struct {
	SessionID      string
	ExamID         string
	ExamName       string
	StudentID      string
	StudentName    string
	OrgID          string
	Status         string
	Verdict        string // clean | suspicious | violation | ""
	IntegrityScore float64
	ViolationCount int
	ReviewStatus   string
	GeneratedAt    string

	// Timeline: recent violations (max 30 for PDF)
	Violations []ExternalViolation

	// AI deep scan summary
	AIScanned         bool
	IdentityVerified  *bool
	FaceMismatchCount int
	LivenessFailCount int
	ObjectDetections  []ExternalObjectDetection

	// Recordings
	Recordings []ExternalRecordingRef
}

// ExternalViolation is a single violation event for the PDF.
type ExternalViolation struct {
	EventType  string
	Severity   string
	Label      string
	Confidence float64
	Timestamp  string
}

// ExternalObjectDetection is a detected object from AI scan.
type ExternalObjectDetection struct {
	ObjectType string
	Count      int
	MaxConf    float64
}

// ExternalRecordingRef is a reference to a LiveKit recording.
type ExternalRecordingRef struct {
	EgressID  string
	Status    string
	StartedAt string
}

// GenerateExternalPDF creates a bilingual (Russian + Kazakh) PDF proctoring
// report for LMS partners. Uses the same zero-dependency raw PDF approach
// as the forensic report generator.
//
// Returns: (pdfBytes, sha256HexHash, error)
func GenerateExternalPDF(data *ExternalReportData) ([]byte, string, error) {
	g := &externalPDFGen{
		PDFGenerator: PDFGenerator{
			pageH:   842,
			pageW:   595,
			margin:  45,
			fontSz:  10,
			nextObj: 1,
		},
	}

	g.build(data)
	pdfBytes := g.buf.Bytes()
	hash := sha256.Sum256(pdfBytes)
	return pdfBytes, fmt.Sprintf("%x", hash[:]), nil
}

// externalPDFGen wraps PDFGenerator to add bilingual helpers.
type externalPDFGen struct {
	PDFGenerator
}

func (g *externalPDFGen) build(d *ExternalReportData) {
	var pages []string

	// ── Page 1: Header + Verdict + Session Info ──────────────────────────────
	var p1 strings.Builder
	g.yPos = g.pageH - g.margin

	// Argus header
	g.pdfText(&p1, g.pageW/2, g.yPos, 16, "center", "ARGUS AI PROCTORING")
	g.yPos -= 18
	g.pdfText(&p1, g.pageW/2, g.yPos, 9, "center", "PROCTORING REPORT  /  PROKTORING ESEBІ")
	g.yPos -= 14
	g.pdfLine(&p1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.8)
	g.yPos -= 18

	// Report meta
	g.biLine(&p1, "Сформировано / Жасалды:", fmtTime(d.GeneratedAt))
	g.biLine(&p1, "Report Hash:", "")
	g.yPos -= 8

	// ── Verdict block ─────────────────────────────────────────────────────────
	g.pdfLine(&p1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 16
	g.pdfText(&p1, g.margin, g.yPos, 11, "left", "ВЕРДИКТ / ҮКІМ")
	g.yPos -= 18

	verdictRU, verdictKZ, verdictIcon := verdictStrings(d.Verdict)
	g.pdfText(&p1, g.margin+10, g.yPos, 20, "left",
		fmt.Sprintf("%s %s", verdictIcon, strings.ToUpper(verdictRU)))
	g.yPos -= 14
	g.pdfText(&p1, g.margin+10, g.yPos, 10, "left",
		fmt.Sprintf("KZ: %s", verdictKZ))
	g.yPos -= 10

	if d.IntegrityScore > 0 {
		g.pdfText(&p1, g.margin+10, g.yPos, 10, "left",
			fmt.Sprintf("Integrity Score: %.1f%%  |  Нарушений / Бұзушылықтар: %d",
				d.IntegrityScore, d.ViolationCount))
		g.yPos -= 18
	} else {
		g.yPos -= 8
	}

	// ── Session metadata ──────────────────────────────────────────────────────
	g.pdfLine(&p1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 16
	g.pdfText(&p1, g.margin, g.yPos, 11, "left", "ДАННЫЕ СЕССИИ / СЕССИЯ МӘЛІМЕТТЕРІ")
	g.yPos -= 16

	rows := [][]string{
		{"Сессия / Session ID", d.SessionID},
		{"Студент / Студент", d.StudentID},
		{"Имя / Аты-жөні", d.StudentName},
		{"Экзамен / Емтихан", d.ExamName},
		{"Exam ID", d.ExamID},
		{"Организация / Ұйым", d.OrgID},
		{"Статус / Мәртебе", statusStr(d.Status)},
		{"Проверка / Тексеру", d.ReviewStatus},
	}
	for _, row := range rows {
		if row[1] == "" {
			continue
		}
		g.biLine(&p1, row[0]+":", row[1])
	}

	// ── AI Deep Scan ──────────────────────────────────────────────────────────
	g.yPos -= 8
	g.pdfLine(&p1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 16
	g.pdfText(&p1, g.margin, g.yPos, 11, "left", "АНАЛИЗ ИИ / ЖИ ТАЛДАУЫ")
	g.yPos -= 16

	if d.AIScanned {
		if d.IdentityVerified != nil {
			identStr := "ПОДТВЕРЖДЕНА / РАСТАЛДЫ"
			if !*d.IdentityVerified {
				identStr = "НЕСООТВЕТСТВИЕ / СӘЙКЕССІЗДІК"
			}
			g.biLine(&p1, "Идентификация / Сәйкестендіру:", identStr)
		}
		g.biLine(&p1, "Несоответствий лица / Бет сәйкессіздіктері:",
			fmt.Sprintf("%d", d.FaceMismatchCount))
		g.biLine(&p1, "Живость (сбои) / Тіршілік (сәтсіздіктер):",
			fmt.Sprintf("%d", d.LivenessFailCount))
		if len(d.ObjectDetections) > 0 {
			for _, obj := range d.ObjectDetections {
				g.biLine(&p1,
					fmt.Sprintf("  Объект / Зат [%s]:", obj.ObjectType),
					fmt.Sprintf("%d раз(а), макс. уверенность / макс. сенімділік: %.0f%%",
						obj.Count, obj.MaxConf*100))
			}
		} else {
			g.biLine(&p1, "Запрещённые объекты / Тыйым салынған заттар:", "Не обнаружены / Табылмады")
		}
	} else {
		g.pdfText(&p1, g.margin+10, g.yPos, 9, "left",
			"AI-анализ не запускался / ЖИ талдауы жүргізілмеді")
		g.yPos -= 14
	}

	pages = append(pages, p1.String())

	// ── Page 2: Violations timeline ───────────────────────────────────────────
	var p2 strings.Builder
	g.yPos = g.pageH - g.margin

	g.pdfText(&p2, g.pageW/2, g.yPos, 12, "center", "НАРУШЕНИЯ / БҰЗУШЫЛЫҚТАР")
	g.yPos -= 18
	g.pdfLine(&p2, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.5)
	g.yPos -= 14

	violations := d.Violations
	if len(violations) > 30 {
		violations = violations[:30]
	}

	if len(violations) == 0 {
		g.pdfText(&p2, g.margin+10, g.yPos, 10, "left",
			"Нарушений не обнаружено / Бұзушылықтар табылмады")
		g.yPos -= 20
	} else {
		// Table header
		g.pdfText(&p2, g.margin, g.yPos, 8, "left", "Время / Уақыт")
		g.pdfText(&p2, g.margin+85, g.yPos, 8, "left", "Тип / Түрі")
		g.pdfText(&p2, g.margin+260, g.yPos, 8, "left", "Важность / Маңыздылық")
		g.pdfText(&p2, g.margin+380, g.yPos, 8, "left", "Уверенность")
		g.yPos -= 4
		g.pdfLine(&p2, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.2)
		g.yPos -= 14

		for _, v := range violations {
			if g.yPos < g.margin+30 {
				// would need page break, truncate for now
				g.pdfText(&p2, g.margin, g.yPos, 8, "left",
					fmt.Sprintf("... и ещё %d нарушений", len(d.Violations)-30))
				break
			}
			ts := fmtTime(v.Timestamp)
			label := truncateStr(v.Label, 40)
			sevRU := severityRU(v.Severity)
			conf := fmt.Sprintf("%.0f%%", v.Confidence*100)

			g.pdfText(&p2, g.margin, g.yPos, 7, "left", ts)
			g.pdfText(&p2, g.margin+85, g.yPos, 7, "left", label)
			g.pdfText(&p2, g.margin+260, g.yPos, 7, "left", sevRU)
			g.pdfText(&p2, g.margin+380, g.yPos, 7, "left", conf)
			g.yPos -= 13
		}
	}

	// Recordings
	g.yPos -= 10
	g.pdfLine(&p2, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 16
	g.pdfText(&p2, g.margin, g.yPos, 11, "left", "ЗАПИСИ / ЖАЗБАЛАР")
	g.yPos -= 16

	if len(d.Recordings) == 0 {
		g.pdfText(&p2, g.margin+10, g.yPos, 9, "left",
			"Записи отсутствуют / Жазбалар жоқ")
	} else {
		for _, rec := range d.Recordings {
			g.biLine(&p2, "Egress ID:", rec.EgressID)
			g.biLine(&p2, "Статус / Мәртебе:", rec.Status)
			g.biLine(&p2, "Начало / Басталуы:", fmtTime(rec.StartedAt))
			g.yPos -= 6
		}
	}

	// Footer
	g.yPos -= 20
	g.pdfLine(&p2, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.5)
	g.yPos -= 14
	g.pdfText(&p2, g.pageW/2, g.yPos, 7, "center",
		"Argus AI Proctoring — Конфиденциальный документ / Құпия құжат")
	g.yPos -= 12
	g.pdfText(&p2, g.pageW/2, g.yPos, 7, "center",
		"Данный отчёт сформирован автоматически системой прокторинга Argus AI.")
	g.yPos -= 10
	g.pdfText(&p2, g.pageW/2, g.yPos, 7, "center",
		"Бұл есеп Argus AI прокторинг жүйесімен автоматты түрде жасалған.")

	pages = append(pages, p2.String())

	g.assemblePDF(pages)
}

// biLine renders a label:value pair with label in bold size.
func (g *externalPDFGen) biLine(sb *strings.Builder, label, value string) {
	g.pdfText(sb, g.margin+10, g.yPos, 8.5, "left",
		fmt.Sprintf("%-40s %s", label, value))
	g.yPos -= 13
}

// ── Translation helpers ────────────────────────────────────────────────────

func verdictStrings(v string) (ru, kz, icon string) {
	switch strings.ToLower(v) {
	case "clean":
		return "Чисто", "Таза", "✓"
	case "suspicious":
		return "Подозрительно", "Күмәнді", "!"
	case "violation":
		return "Нарушение", "Бұзушылық", "✗"
	default:
		return "Ожидает / Күтілуде", "Күтілуде", "?"
	}
}

func severityRU(s string) string {
	switch strings.ToLower(s) {
	case "critical":
		return "Критический / Критикалық"
	case "warning":
		return "Предупреждение / Ескерту"
	default:
		return "Информация / Ақпарат"
	}
}

func statusStr(s string) string {
	switch s {
	case "completed":
		return "Завершён / Аяқталды"
	case "active":
		return "Активен / Белсенді"
	case "cancelled":
		return "Отменён / Бас тартылды"
	case "created":
		return "Создан / Жасалды"
	default:
		return s
	}
}

func fmtTime(ts string) string {
	if ts == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.UTC().Format("02.01.2006 15:04:05 UTC")
}

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n-3] + "..."
	}
	return s
}

// externalReportHash returns the SHA-256 hex hash of the PDF bytes.
func externalReportHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:])
}

// ensure unused import doesn't fail
var _ = bytes.NewBuffer
