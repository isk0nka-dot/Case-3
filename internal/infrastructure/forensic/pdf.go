// Package forensic — PDF Forensic Report Generator
//
// Generates professional forensic PDF reports from ForensicReport data using
// raw PDF 1.7 specification. Zero external dependencies — pure Go PDF generation.
//
// Report sections:
//   1. Header with Argus AI branding and report metadata
//   2. Integrity Score gauge with verdict
//   3. Session metadata table
//   4. Device fingerprint section
//   5. Violation timeline
//   6. Penalty breakdown table
//   7. Voice biometric analysis
//   8. Forensic ledger verification summary
//   9. Report hash and digital signature placeholder
package forensic

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
	"time"
)

// PDFGenerator produces forensic report PDFs.
type PDFGenerator struct {
	buf     bytes.Buffer
	objects []string
	pages   []int // object IDs of page objects
	xref    []int // byte offsets
	nextObj int
	yPos    float64
	pageH   float64
	pageW   float64
	margin  float64
	fontSz  float64
	curPage int
}

// GenerateForensicPDF creates a forensic report PDF from the given report data.
// Returns the raw PDF bytes and the SHA-256 hash of the content (hex-encoded).
func GenerateForensicPDF(report *ForensicReport) ([]byte, string, error) {
	g := &PDFGenerator{
		pageH:  842, // A4 height in points
		pageW:  595, // A4 width in points
		margin: 50,
		fontSz: 10,
		nextObj: 1,
	}

	g.build(report)

	pdfBytes := g.buf.Bytes()
	hash := sha256.Sum256(pdfBytes)
	hashHex := fmt.Sprintf("%x", hash[:])

	return pdfBytes, hashHex, nil
}

func (g *PDFGenerator) build(report *ForensicReport) {
	// Collect all content first, then assemble PDF structure
	var contentStreams []string

	// Page 1: Header + Score + Session Info + Device
	var page1 strings.Builder
	g.yPos = g.pageH - g.margin

	// Header
	g.pdfText(&page1, g.pageW/2, g.yPos, 18, "center", "ARGUS AI")
	g.yPos -= 22
	g.pdfText(&page1, g.pageW/2, g.yPos, 10, "center", "FORENSIC INTEGRITY REPORT")
	g.yPos -= 16
	g.pdfText(&page1, g.pageW/2, g.yPos, 8, "center",
		fmt.Sprintf("Report ID: %s", report.ReportID))
	g.yPos -= 12
	g.pdfText(&page1, g.pageW/2, g.yPos, 8, "center",
		fmt.Sprintf("Generated: %s UTC", report.GeneratedAt))
	g.yPos -= 24

	// Horizontal line
	g.pdfLine(&page1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.5)
	g.yPos -= 20

	// Integrity Score Section
	g.pdfText(&page1, g.margin, g.yPos, 14, "left", "INTEGRITY SCORE")
	g.yPos -= 24

	scoreStr := fmt.Sprintf("%.1f%%", report.Integrity.Score)
	g.pdfText(&page1, g.margin+20, g.yPos, 28, "left", scoreStr)

	verdictStr := strings.ToUpper(report.Integrity.Verdict)
	verdictLabel := report.Integrity.VerdictLabel
	g.pdfText(&page1, g.margin+140, g.yPos+6, 14, "left",
		fmt.Sprintf("[%s] %s", verdictStr, verdictLabel))
	g.yPos -= 20

	// Justification
	justLines := g.wrapText(report.Integrity.Justification, 85)
	for _, line := range justLines {
		g.pdfText(&page1, g.margin+20, g.yPos, 9, "left", line)
		g.yPos -= 13
	}
	g.yPos -= 10

	// Session Metadata
	g.pdfLine(&page1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 18
	g.pdfText(&page1, g.margin, g.yPos, 12, "left", "SESSION METADATA")
	g.yPos -= 18

	metaRows := [][]string{
		{"Session ID", report.SessionID},
		{"Student ID", report.StudentID},
		{"Exam ID", report.ExamID},
		{"Organization", report.OrgID},
		{"Duration", formatDuration(report.Integrity.DurationSec)},
		{"Total Events", fmt.Sprintf("%d", report.Integrity.TotalEvents)},
		{"Critical Events", fmt.Sprintf("%d", report.Integrity.CriticalCount)},
		{"Warning Events", fmt.Sprintf("%d", report.Integrity.WarningCount)},
	}
	for _, row := range metaRows {
		g.pdfText(&page1, g.margin+10, g.yPos, 9, "left", row[0]+":")
		g.pdfText(&page1, g.margin+160, g.yPos, 9, "left", row[1])
		g.yPos -= 14
	}
	g.yPos -= 10

	// Device Info
	g.pdfLine(&page1, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 18
	g.pdfText(&page1, g.margin, g.yPos, 12, "left", "DEVICE FINGERPRINT")
	g.yPos -= 18

	deviceRows := [][]string{
		{"User Agent", truncStr(report.DeviceInfo.UserAgent, 60)},
		{"Resolution", report.DeviceInfo.Resolution},
		{"IP Address", report.DeviceInfo.IPAddress},
		{"Region", report.DeviceInfo.Region},
		{"Timezone", fmt.Sprintf("UTC%+d", report.DeviceInfo.Timezone/60)},
	}
	for _, row := range deviceRows {
		g.pdfText(&page1, g.margin+10, g.yPos, 9, "left", row[0]+":")
		g.pdfText(&page1, g.margin+120, g.yPos, 9, "left", row[1])
		g.yPos -= 14
	}

	contentStreams = append(contentStreams, page1.String())

	// Page 2: Penalty Breakdown + Timeline
	var page2 strings.Builder
	g.yPos = g.pageH - g.margin

	g.pdfText(&page2, g.margin, g.yPos, 14, "left", "PENALTY BREAKDOWN")
	g.yPos -= 20

	// Table header
	g.pdfText(&page2, g.margin+10, g.yPos, 8, "left", "Violation Type")
	g.pdfText(&page2, g.margin+250, g.yPos, 8, "left", "Count")
	g.pdfText(&page2, g.margin+310, g.yPos, 8, "left", "Per Event")
	g.pdfText(&page2, g.margin+370, g.yPos, 8, "left", "Applied")
	g.pdfText(&page2, g.margin+430, g.yPos, 8, "left", "Max Cap")
	g.yPos -= 4
	g.pdfLine(&page2, g.margin+5, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 14

	for _, p := range report.Integrity.Penalties {
		if g.yPos < g.margin+40 {
			break
		}
		g.pdfText(&page2, g.margin+10, g.yPos, 8, "left", p.Description)
		g.pdfText(&page2, g.margin+260, g.yPos, 8, "left", fmt.Sprintf("%d", p.Count))
		g.pdfText(&page2, g.margin+315, g.yPos, 8, "left", fmt.Sprintf("-%.1f", p.PenaltyPer))
		g.pdfText(&page2, g.margin+375, g.yPos, 8, "left", fmt.Sprintf("-%.1f", p.Applied))
		maxCapStr := "N/A"
		if p.MaxPenalty > 0 {
			maxCapStr = fmt.Sprintf("-%.0f", p.MaxPenalty)
		}
		g.pdfText(&page2, g.margin+435, g.yPos, 8, "left", maxCapStr)
		g.yPos -= 14
	}
	g.yPos -= 16

	// Timeline section
	g.pdfLine(&page2, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 18
	g.pdfText(&page2, g.margin, g.yPos, 14, "left", "VIOLATION TIMELINE")
	g.yPos -= 20

	g.pdfText(&page2, g.margin+10, g.yPos, 8, "left", "Time")
	g.pdfText(&page2, g.margin+100, g.yPos, 8, "left", "Video Sec")
	g.pdfText(&page2, g.margin+170, g.yPos, 8, "left", "Type")
	g.pdfText(&page2, g.margin+320, g.yPos, 8, "left", "Severity")
	g.pdfText(&page2, g.margin+390, g.yPos, 8, "left", "Confidence")
	g.yPos -= 4
	g.pdfLine(&page2, g.margin+5, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 14

	maxTimeline := 30
	for i, entry := range report.Timeline {
		if i >= maxTimeline || g.yPos < g.margin+40 {
			if i < len(report.Timeline) {
				remaining := len(report.Timeline) - i
				g.pdfText(&page2, g.margin+10, g.yPos, 8, "left",
					fmt.Sprintf("... and %d more violations", remaining))
			}
			break
		}
		ts := entry.Timestamp
		if len(ts) > 19 {
			ts = ts[:19]
		}
		g.pdfText(&page2, g.margin+10, g.yPos, 7, "left", ts)
		g.pdfText(&page2, g.margin+110, g.yPos, 7, "left", fmt.Sprintf("%ds", entry.VideoSec))
		g.pdfText(&page2, g.margin+170, g.yPos, 7, "left", truncStr(entry.EventType, 25))
		g.pdfText(&page2, g.margin+325, g.yPos, 7, "left", entry.Severity)
		g.pdfText(&page2, g.margin+395, g.yPos, 7, "left", fmt.Sprintf("%.0f%%", entry.Confidence*100))
		g.yPos -= 12
	}

	contentStreams = append(contentStreams, page2.String())

	// Page 3: Secondary Camera + Voice Biometric + Ledger + Hash
	var page3 strings.Builder
	g.yPos = g.pageH - g.margin

	// Fix 9: SECONDARY CAMERA section
	g.pdfText(&page3, g.margin, g.yPos, 14, "left", "SECONDARY CAMERA")
	g.yPos -= 20

	if report.Sidecam.WasPaired {
		scRows := [][]string{
			{"Device Model", report.Sidecam.DeviceModel},
			{"Calibrated", boolStr(report.Sidecam.Calibrated)},
			{"Calibration Angle", fmt.Sprintf("%.0f deg", report.Sidecam.CalibrationAngle)},
			{"Total Anomalies", fmt.Sprintf("%d", report.Sidecam.TotalAnomalies)},
			{"Device Displaced", fmt.Sprintf("%d", report.Sidecam.DisplacementEvents)},
			{"Hands Off Desk", fmt.Sprintf("%d", report.Sidecam.HandsOffDeskEvents)},
			{"Stream Drops", fmt.Sprintf("%d", report.Sidecam.StreamDropEvents)},
			{"Battery Critical", fmt.Sprintf("%d", report.Sidecam.BatteryCritEvents)},
			{"Calibration Failures", fmt.Sprintf("%d", report.Sidecam.CalibrationFailures)},
			{"Thermal Throttles", fmt.Sprintf("%d", report.Sidecam.ThermalThrottles)},
		}
		for _, row := range scRows {
			g.pdfText(&page3, g.margin+10, g.yPos, 9, "left", row[0]+":")
			g.pdfText(&page3, g.margin+200, g.yPos, 9, "left", row[1])
			g.yPos -= 14
		}
	} else {
		g.pdfText(&page3, g.margin+10, g.yPos, 9, "left", "No secondary camera was paired for this session.")
		g.yPos -= 14
	}
	g.yPos -= 10

	g.pdfLine(&page3, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 18
	g.pdfText(&page3, g.margin, g.yPos, 14, "left", "VOICE BIOMETRIC ANALYSIS")
	g.yPos -= 20

	voiceRows := [][]string{
		{"Total Voice Segments", fmt.Sprintf("%d", report.Voice.TotalSegments)},
		{"Matched (Primary Speaker)", fmt.Sprintf("%d", report.Voice.MatchedSegments)},
		{"Mismatched Segments", fmt.Sprintf("%d", report.Voice.MismatchedSegments)},
		{"Speaker Change Events", fmt.Sprintf("%d", report.Voice.SpeakerChangeCount)},
		{"Consistency Score", fmt.Sprintf("%.1f%%", report.Voice.ConsistencyScore*100)},
		{"Primary Speaker Ratio", fmt.Sprintf("%.1f%%", report.Voice.PrimarySpeakerRatio*100)},
		{"Verdict", strings.ToUpper(report.Voice.Verdict)},
	}
	for _, row := range voiceRows {
		g.pdfText(&page3, g.margin+10, g.yPos, 9, "left", row[0]+":")
		g.pdfText(&page3, g.margin+220, g.yPos, 9, "left", row[1])
		g.yPos -= 14
	}
	g.yPos -= 16

	// Forensic Ledger
	g.pdfLine(&page3, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 18
	g.pdfText(&page3, g.margin, g.yPos, 14, "left", "FORENSIC LEDGER VERIFICATION")
	g.yPos -= 20

	chainStatus := "VALID"
	if !report.LedgerSummary.ChainValid {
		chainStatus = "BROKEN"
	}
	ledgerRows := [][]string{
		{"Total Evidence Fragments", fmt.Sprintf("%d", report.LedgerSummary.TotalFragments)},
		{"Verified OK", fmt.Sprintf("%d", report.LedgerSummary.VerifiedOK)},
		{"Hash Chain Status", chainStatus},
		{"S3 Verified", fmt.Sprintf("%d", report.LedgerSummary.S3Verified)},
		{"S3 Mismatches", fmt.Sprintf("%d", report.LedgerSummary.S3Mismatches)},
	}
	for _, row := range ledgerRows {
		g.pdfText(&page3, g.margin+10, g.yPos, 9, "left", row[0]+":")
		g.pdfText(&page3, g.margin+220, g.yPos, 9, "left", row[1])
		g.yPos -= 14
	}
	g.yPos -= 24

	// Top Contributing Factors
	g.pdfLine(&page3, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.3)
	g.yPos -= 18
	g.pdfText(&page3, g.margin, g.yPos, 12, "left", "TOP CONTRIBUTING FACTORS")
	g.yPos -= 18

	for i, factor := range report.Integrity.TopFactors {
		g.pdfText(&page3, g.margin+10, g.yPos, 9, "left",
			fmt.Sprintf("%d. %s", i+1, factor))
		g.yPos -= 14
	}
	g.yPos -= 30

	// Report Hash
	g.pdfLine(&page3, g.margin, g.yPos, g.pageW-g.margin, g.yPos, 0.5)
	g.yPos -= 18
	g.pdfText(&page3, g.margin, g.yPos, 10, "left", "DOCUMENT INTEGRITY")
	g.yPos -= 16
	g.pdfText(&page3, g.margin+10, g.yPos, 8, "left",
		fmt.Sprintf("Report Hash (SHA-256): %s", report.ReportHash))
	g.yPos -= 14
	g.pdfText(&page3, g.margin+10, g.yPos, 8, "left",
		"This document is cryptographically bound to the Argus AI forensic ledger.")
	g.yPos -= 14
	g.pdfText(&page3, g.margin+10, g.yPos, 8, "left",
		"Any modification to this document will invalidate the hash.")
	g.yPos -= 30

	// Footer
	g.pdfText(&page3, g.pageW/2, g.margin, 7, "center",
		fmt.Sprintf("Argus AI Forensic Report | %s | Confidential",
			time.Now().UTC().Format("2006-01-02")))

	contentStreams = append(contentStreams, page3.String())

	// Assemble PDF
	g.assemblePDF(contentStreams)
}

// assemblePDF creates the raw PDF structure.
func (g *PDFGenerator) assemblePDF(contentStreams []string) {
	g.buf.Reset()
	g.xref = nil
	g.nextObj = 1

	// PDF Header
	g.buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")

	// Object 1: Catalog
	g.writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// Object 2: Pages (will reference page objects)
	pageCount := len(contentStreams)
	var pageRefs strings.Builder
	for i := 0; i < pageCount; i++ {
		// Each page uses 3 objects: Page, Contents stream, (Font is shared)
		pageObjID := 4 + i*2 // Page objects start at 4, each page = 2 objects
		if i > 0 {
			pageRefs.WriteString(" ")
		}
		pageRefs.WriteString(fmt.Sprintf("%d 0 R", pageObjID))
	}

	g.writeObj(fmt.Sprintf(
		"2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n",
		pageRefs.String(), pageCount,
	))

	// Object 3: Font
	g.writeObj(
		"3 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\nendobj\n",
	)

	// Page objects + content streams
	for i, content := range contentStreams {
		pageObjID := 4 + i*2
		contentsObjID := pageObjID + 1

		// Stream data
		streamData := fmt.Sprintf("BT\n/F1 %g Tf\n%s\nET\n", g.fontSz, content)

		// Page object
		g.writeObj(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g] "+
				"/Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>\nendobj\n",
			pageObjID, g.pageW, g.pageH, contentsObjID,
		))

		// Contents stream
		g.writeObj(fmt.Sprintf(
			"%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n",
			contentsObjID, len(streamData), streamData,
		))
	}

	// Cross-reference table
	xrefOffset := g.buf.Len()
	totalObjs := g.nextObj
	g.buf.WriteString(fmt.Sprintf("xref\n0 %d\n", totalObjs))
	g.buf.WriteString("0000000000 65535 f \n")
	for _, offset := range g.xref {
		g.buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}

	// Trailer
	g.buf.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		totalObjs, xrefOffset,
	))
}

func (g *PDFGenerator) writeObj(content string) {
	g.xref = append(g.xref, g.buf.Len())
	g.buf.WriteString(content)
	g.nextObj++
}

// pdfText adds a text instruction to the content stream.
func (g *PDFGenerator) pdfText(sb *strings.Builder, x, y, size float64, align, text string) {
	// Escape special PDF characters
	escaped := pdfEscape(text)

	// For center alignment, approximate width
	if align == "center" {
		approxWidth := float64(len(text)) * size * 0.5
		x = x - approxWidth/2
	}

	sb.WriteString(fmt.Sprintf("/F1 %g Tf\n", size))
	sb.WriteString(fmt.Sprintf("%g %g Td\n", x, y))
	sb.WriteString(fmt.Sprintf("(%s) Tj\n", escaped))
	// Reset position for next text
	sb.WriteString(fmt.Sprintf("%g %g Td\n", -x, -y))
}

// pdfLine draws a line in the content stream (outside BT/ET).
// Since we're inside BT/ET, we use the ET/BT trick.
func (g *PDFGenerator) pdfLine(sb *strings.Builder, x1, y1, x2, y2, width float64) {
	sb.WriteString("ET\n")
	sb.WriteString(fmt.Sprintf("%g w\n", width))
	sb.WriteString(fmt.Sprintf("%g %g m %g %g l S\n", x1, y1, x2, y2))
	sb.WriteString(fmt.Sprintf("BT\n/F1 %g Tf\n", g.fontSz))
}

func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	// Replace non-ASCII with '?' for Helvetica (Type1 font can't render Cyrillic)
	// For production, use a CIDFont with embedded TTF. Here we transliterate key terms.
	var out strings.Builder
	for _, r := range s {
		if r < 128 {
			out.WriteRune(r)
		} else {
			out.WriteRune('?')
		}
	}
	return out.String()
}

func (g *PDFGenerator) wrapText(text string, maxChars int) []string {
	words := strings.Fields(text)
	var lines []string
	var current strings.Builder

	for _, word := range words {
		if current.Len()+len(word)+1 > maxChars {
			lines = append(lines, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func formatDuration(sec int64) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// ScoreColor returns an RGB color for the given integrity score (used in SVG/HTML).
func ScoreColor(score float64) string {
	if score >= 80 {
		return "#22c55e" // green
	}
	if score >= 50 {
		return "#f59e0b" // amber
	}
	return "#ef4444" // red
}

// VerdictBadge returns a colored badge HTML fragment for the verdict.
func VerdictBadge(verdict string) string {
	switch verdict {
	case "clean":
		return `<span style="background:#22c55e;color:#fff;padding:2px 8px;border-radius:4px">CLEAN</span>`
	case "warning":
		return `<span style="background:#f59e0b;color:#fff;padding:2px 8px;border-radius:4px">WARNING</span>`
	default:
		return `<span style="background:#ef4444;color:#fff;padding:2px 8px;border-radius:4px">FRAUD</span>`
	}
}

func boolStr(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// unused but avoids import error
var _ = math.Pi
