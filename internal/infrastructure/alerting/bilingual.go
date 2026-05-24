package alerting

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Core bilingual formatter
// ---------------------------------------------------------------------------

const separator = "━━━━━━━━━━━━━━━━━━━━━━━━━"

// bilingual builds a dual-language KZ+EN Telegram HTML message.
func bilingual(kzBlock, enBlock string) string {
	return kzBlock + "\n\n" + enBlock
}

// ---------------------------------------------------------------------------
// 1. Server Startup
// ---------------------------------------------------------------------------

func MsgServerStartup(timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🚀 ARGUS AI — КҮЗЕТ ЖҮЙЕСІ ІСКЕ ҚОСЫЛДЫ</b>\n"+
			"%s\n"+
			"<b>Күйі:</b>            <code>Онлайн &amp; Жұмыс істеуде</code>\n"+
			"<b>Ажыратқыш:</b>       <code>ClickHouse бақылауда</code>\n"+
			"<b>Kafka:</b>           <code>Қосылған</code>\n"+
			"<b>Уақыт:</b>           <code>%s</code>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>Жүйе қалыпты жұмыс істеп тұр</code>",
		separator, timestamp, separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🚀 ARGUS AI — SENTINEL ACTIVATED</b>\n"+
			"%s\n"+
			"<b>Status:</b>          <code>Online &amp; Healthy</code>\n"+
			"<b>Circuit Breaker:</b>  <code>Monitoring ClickHouse</code>\n"+
			"<b>Kafka:</b>           <code>Connected</code>\n"+
			"<b>Time:</b>            <code>%s</code>\n"+
			"%s\n"+
			"<b>Health:</b> <code>System Nominal</code>",
		separator, timestamp, separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 2. Circuit Breaker Open (reused by DLQ + ClickHouse)
// ---------------------------------------------------------------------------

func MsgCircuitBreakerOpen(component, cause, recovery, timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🔴 СЫНИ ҚАТЕ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Компонент:</b>       <code>%s</code>\n"+
			"<b>Күй:</b>             <code>АШЫҚ</code>\n"+
			"<b>Себеп:</b>           %s\n"+
			"<b>Қалпына келтіру:</b> %s\n"+
			"<b>Уақыт:</b>           <code>%s</code>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>🔴 ҚАУІП: Тұтастық бұзылды</code>",
		separator,
		htmlEscape(component), htmlEscape(cause), htmlEscape(recovery), timestamp,
		separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🔴 CRITICAL — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Component:</b>   <code>%s</code>\n"+
			"<b>State:</b>       <code>OPEN</code>\n"+
			"<b>Cause:</b>       %s\n"+
			"<b>Recovery:</b>    %s\n"+
			"<b>Time:</b>        <code>%s</code>\n"+
			"%s\n"+
			"<b>Health:</b> <code>Integrity Alert — Events buffered locally</code>",
		separator,
		htmlEscape(component), htmlEscape(cause), htmlEscape(recovery), timestamp,
		separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 3. Diagnostic Test Alert
// ---------------------------------------------------------------------------

func MsgDiagnosticTest(timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🧪 ДИАГНОСТИКА — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Компонент:</b>   <code>database-monitor</code>\n"+
			"<b>Маңыздылық:</b>  <code>СЫНИ</code>\n"+
			"<b>Уақыт:</b>       <code>%s</code>\n"+
			"%s\n"+
			"<b>Қате мәліметтері:</b>\n"+
			"<pre>Дерекқорға қосылу қабылданбады: dial tcp 10.0.1.5:9000: connect: connection refused</pre>\n"+
			"%s\n"+
			"<i>POST /api/v1/internal/test-alert арқылы тест хабарландыруы</i>",
		separator, timestamp, separator, separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🧪 DIAGNOSTIC — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Component:</b>   <code>database-monitor</code>\n"+
			"<b>Severity:</b>    <code>CRITICAL</code>\n"+
			"<b>Time:</b>        <code>%s</code>\n"+
			"%s\n"+
			"<b>Error Details:</b>\n"+
			"<pre>Database Connection Refused: dial tcp 10.0.1.5:9000: connect: connection refused</pre>\n"+
			"%s\n"+
			"<i>Test alert triggered via POST /api/v1/internal/test-alert</i>",
		separator, timestamp, separator, separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 4. Worker Startup
// ---------------------------------------------------------------------------

func MsgWorkerStartup(version, redisAddr string, concurrency int, timestamp string, handlers []string) string {
	handlerLines := ""
	for _, h := range handlers {
		handlerLines += fmt.Sprintf("  • <code>%s</code>\n", htmlEscape(h))
	}

	kz := fmt.Sprintf(
		"<b>🇰🇿 🚀 ЖҰМЫСШЫ ІСКЕ ҚОСЫЛДЫ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Нұсқа:</b>          <code>%s</code>\n"+
			"<b>Redis:</b>           <code>%s</code>\n"+
			"<b>Бір мезгілділік:</b> <code>%d</code>\n"+
			"<b>Уақыт:</b>          <code>%s</code>\n"+
			"%s\n"+
			"<b>Өңдеушілер:</b>\n%s",
		separator,
		htmlEscape(version), htmlEscape(redisAddr), concurrency, timestamp,
		separator, handlerLines,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🚀 WORKER STARTED — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Version:</b>     <code>%s</code>\n"+
			"<b>Redis:</b>       <code>%s</code>\n"+
			"<b>Concurrency:</b> <code>%d</code>\n"+
			"<b>Time:</b>        <code>%s</code>\n"+
			"%s\n"+
			"<b>Handlers:</b>\n%s",
		separator,
		htmlEscape(version), htmlEscape(redisAddr), concurrency, timestamp,
		separator, handlerLines,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 5. Worker Shutdown
// ---------------------------------------------------------------------------

func MsgWorkerShutdown(timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🛑 ЖҰМЫСШЫ ТОҚТАТЫЛДЫ — ARGUS AI</b>\n"+
			"<b>Уақыт:</b> <code>%s</code>",
		timestamp,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🛑 WORKER STOPPED — ARGUS AI</b>\n"+
			"<b>Time:</b> <code>%s</code>",
		timestamp,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 6. Inference Startup
// ---------------------------------------------------------------------------

func MsgInferenceStartup(version, engine string, port, concurrency int, timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🧠 ТАЛДАУ ҚЫЗМЕТІ ІСКЕ ҚОСЫЛДЫ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Нұсқа:</b>          <code>%s</code>\n"+
			"<b>Қозғалтқыш:</b>     <code>%s</code>\n"+
			"<b>Порт:</b>           <code>%d</code>\n"+
			"<b>Бір мезгілділік:</b> <code>%d</code>\n"+
			"<b>Уақыт:</b>          <code>%s</code>",
		separator,
		htmlEscape(version), htmlEscape(engine), port, concurrency, timestamp,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🧠 INFERENCE STARTED — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Version:</b>     <code>%s</code>\n"+
			"<b>Engine:</b>      <code>%s</code>\n"+
			"<b>Port:</b>        <code>%d</code>\n"+
			"<b>Concurrency:</b> <code>%d</code>\n"+
			"<b>Time:</b>        <code>%s</code>",
		separator,
		htmlEscape(version), htmlEscape(engine), port, concurrency, timestamp,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 7. Inference Shutdown
// ---------------------------------------------------------------------------

func MsgInferenceShutdown(timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🛑 ТАЛДАУ ҚЫЗМЕТІ ТОҚТАТЫЛДЫ — ARGUS AI</b>\n"+
			"<b>Уақыт:</b> <code>%s</code>",
		timestamp,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🛑 INFERENCE STOPPED — ARGUS AI</b>\n"+
			"<b>Time:</b> <code>%s</code>",
		timestamp,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 8. Job Completed
// ---------------------------------------------------------------------------

func MsgJobCompleted(jobType, jobID, duration, timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 ✅ ТАПСЫРМА ОРЫНДАЛДЫ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Түрі:</b>       <code>%s</code>\n"+
			"<b>Тапсырма ID:</b> <code>%s</code>\n"+
			"<b>Ұзақтығы:</b>   <code>%s</code>\n"+
			"<b>Уақыт:</b>      <code>%s</code>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>Жүйе қалыпты жұмыс істеп тұр</code>",
		separator,
		htmlEscape(jobType), htmlEscape(jobID), htmlEscape(duration), timestamp,
		separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 ✅ JOB COMPLETED — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Type:</b>     <code>%s</code>\n"+
			"<b>Job ID:</b>   <code>%s</code>\n"+
			"<b>Duration:</b> <code>%s</code>\n"+
			"<b>Time:</b>     <code>%s</code>\n"+
			"%s\n"+
			"<b>Health:</b> <code>System Nominal</code>",
		separator,
		htmlEscape(jobType), htmlEscape(jobID), htmlEscape(duration), timestamp,
		separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 9. Job Failed
// ---------------------------------------------------------------------------

func MsgJobFailed(jobType, jobID, timestamp, errorDetail string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 ❌ ТАПСЫРМА СӘТСІЗ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Түрі:</b>       <code>%s</code>\n"+
			"<b>Тапсырма ID:</b> <code>%s</code>\n"+
			"<b>Уақыт:</b>      <code>%s</code>\n"+
			"%s\n"+
			"<b>Қате мәліметтері:</b>\n<pre>%s</pre>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>🔴 ҚАУІП: Тұтастық бұзылды</code>",
		separator,
		htmlEscape(jobType), htmlEscape(jobID), timestamp,
		separator,
		htmlEscape(truncate(errorDetail, 800)),
		separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 ❌ JOB FAILED — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Type:</b>     <code>%s</code>\n"+
			"<b>Job ID:</b>   <code>%s</code>\n"+
			"<b>Time:</b>     <code>%s</code>\n"+
			"%s\n"+
			"<b>Error Details:</b>\n<pre>%s</pre>\n"+
			"%s\n"+
			"<b>Health:</b> <code>Integrity Alert — Investigate immediately</code>",
		separator,
		htmlEscape(jobType), htmlEscape(jobID), timestamp,
		separator,
		htmlEscape(truncate(errorDetail, 800)),
		separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 10. AI Fraud Detected
// ---------------------------------------------------------------------------

func MsgAIFraudDetected(sessionID, studentID, examID, verdict string, maxConf float32, fraudFrames, totalFrames int, timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🚨 AI АЛАЯҚТЫҚ АНЫҚТАЛДЫ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Сессия:</b>          <code>%s</code>\n"+
			"<b>Студент:</b>         <code>%s</code>\n"+
			"<b>Емтихан:</b>         <code>%s</code>\n"+
			"<b>Шешім:</b>           <code>%s</code>\n"+
			"<b>Макс. сенімділік:</b> <code>%.2f</code>\n"+
			"<b>Алаяқтық кадрлары:</b> <code>%d / %d</code>\n"+
			"<b>Уақыт:</b>           <code>%s</code>\n"+
			"%s\n"+
			"<b>Әрекет қажет:</b> Сессияны дереу тексеріңіз",
		separator,
		htmlEscape(sessionID), htmlEscape(studentID), htmlEscape(examID),
		htmlEscape(verdict), maxConf, fraudFrames, totalFrames, timestamp,
		separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🚨 BACKEND AI FRAUD DETECTED — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Session:</b>    <code>%s</code>\n"+
			"<b>Student:</b>    <code>%s</code>\n"+
			"<b>Exam:</b>       <code>%s</code>\n"+
			"<b>Verdict:</b>    <code>%s</code>\n"+
			"<b>Max Conf:</b>   <code>%.2f</code>\n"+
			"<b>Fraud Frames:</b> <code>%d / %d</code>\n"+
			"<b>Time:</b>       <code>%s</code>\n"+
			"%s\n"+
			"<b>Action Required:</b> Review session immediately",
		separator,
		htmlEscape(sessionID), htmlEscape(studentID), htmlEscape(examID),
		htmlEscape(verdict), maxConf, fraudFrames, totalFrames, timestamp,
		separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 14. DLQ Emergency
// ---------------------------------------------------------------------------

func MsgDLQEmergency(timestamp, detail string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 🔴 ШҰҒЫЛ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Компонент:</b>   <code>dlq-badger</code>\n"+
			"<b>Маңыздылық:</b>  <code>СЫНИ</code>\n"+
			"<b>Уақыт:</b>       <code>%s</code>\n"+
			"%s\n"+
			"<b>Қате мәліметтері:</b>\n"+
			"<pre>Жергілікті резервті сақтау қатесі\n%s</pre>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>🔴 ҚАУІП: Тұтастық бұзылды — ДЕРЕКТЕР ЖОҒАЛУ ҚАУПІ</code>",
		separator, timestamp, separator, htmlEscape(detail), separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 🔴 EMERGENCY — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Component:</b>   <code>dlq-badger</code>\n"+
			"<b>Severity:</b>    <code>CRITICAL</code>\n"+
			"<b>Time:</b>        <code>%s</code>\n"+
			"%s\n"+
			"<b>Error Details:</b>\n"+
			"<pre>Local Fallback Storage Failure\n%s</pre>\n"+
			"%s\n"+
			"<b>Health:</b> <code>Integrity Alert — DATA LOSS RISK — Investigate immediately</code>",
		separator, timestamp, separator, htmlEscape(detail), separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 15. Generic Alert (replaces formatTelegramMessage)
// ---------------------------------------------------------------------------

// FormatAlertBilingual builds a bilingual KZ+EN message from a structured Alert.
func FormatAlertBilingual(a Alert) string {
	// Severity → emoji + KZ/EN labels
	var kzHeader, enHeader string
	switch strings.ToUpper(a.Severity) {
	case "CRITICAL":
		kzHeader = "🔴 СЫНИ"
		enHeader = "🔴 CRITICAL"
	case "WARNING":
		kzHeader = "⚠️ ЕСКЕРТУ"
		enHeader = "⚠️ WARNING"
	case "INFO":
		kzHeader = "ℹ️ АҚПАРАТ"
		enHeader = "ℹ️ INFO"
	default:
		kzHeader = "🔔 " + a.Severity
		enHeader = "🔔 " + a.Severity
	}

	incidentID := shortID()
	errorText := htmlEscape(truncate(a.Error, 800))

	// KZ block
	var kzB strings.Builder
	fmt.Fprintf(&kzB, "<b>🇰🇿 %s — ARGUS AI</b>\n", htmlEscape(kzHeader))
	kzB.WriteString(separator + "\n")
	fmt.Fprintf(&kzB, "<b>Компонент:</b>   <code>%s</code>\n", htmlEscape(a.Component))
	fmt.Fprintf(&kzB, "<b>Оқиға ID:</b>    <code>%s</code>\n", incidentID)
	if a.SessionID != "" {
		fmt.Fprintf(&kzB, "<b>Сессия:</b>      <code>%s</code>\n", htmlEscape(a.SessionID))
	}
	fmt.Fprintf(&kzB, "<b>Маңыздылық:</b>  <code>%s</code>\n", htmlEscape(a.Severity))
	fmt.Fprintf(&kzB, "<b>Уақыт:</b>       <code>%s</code>\n", a.Timestamp.Format("2006-01-02 15:04:05 MST"))
	kzB.WriteString(separator + "\n")
	fmt.Fprintf(&kzB, "<b>Қате мәліметтері:</b>\n<pre>%s</pre>\n", errorText)
	kzB.WriteString(separator + "\n")

	switch strings.ToUpper(a.Severity) {
	case "CRITICAL":
		kzB.WriteString("<b>Денсаулығы:</b> <code>🔴 ҚАУІП: Тұтастық бұзылды</code>")
	case "WARNING":
		kzB.WriteString("<b>Денсаулығы:</b> <code>БАҚЫЛАУДА — Тексеру ұсынылады</code>")
	default:
		kzB.WriteString("<b>Денсаулығы:</b> <code>Жүйе қалыпты жұмыс істеп тұр</code>")
	}

	// EN block
	var enB strings.Builder
	fmt.Fprintf(&enB, "<b>🇬🇧 %s — ARGUS AI</b>\n", htmlEscape(enHeader))
	enB.WriteString(separator + "\n")
	fmt.Fprintf(&enB, "<b>Component:</b>   <code>%s</code>\n", htmlEscape(a.Component))
	fmt.Fprintf(&enB, "<b>Incident ID:</b> <code>%s</code>\n", incidentID)
	if a.SessionID != "" {
		fmt.Fprintf(&enB, "<b>Session:</b>     <code>%s</code>\n", htmlEscape(a.SessionID))
	}
	fmt.Fprintf(&enB, "<b>Severity:</b>    <code>%s</code>\n", htmlEscape(a.Severity))
	fmt.Fprintf(&enB, "<b>Time:</b>        <code>%s</code>\n", a.Timestamp.Format("2006-01-02 15:04:05 MST"))
	enB.WriteString(separator + "\n")
	fmt.Fprintf(&enB, "<b>Error Details:</b>\n<pre>%s</pre>\n", errorText)
	enB.WriteString(separator + "\n")

	switch strings.ToUpper(a.Severity) {
	case "CRITICAL":
		enB.WriteString("<b>Health:</b> <code>Integrity Alert — Investigate immediately</code>")
	case "WARNING":
		enB.WriteString("<b>Health:</b> <code>MONITORING — Review recommended</code>")
	default:
		enB.WriteString("<b>Health:</b> <code>System Nominal</code>")
	}

	return bilingual(kzB.String(), enB.String())
}

// ---------------------------------------------------------------------------
// 16. Buffered Mode Heartbeat (Kafka SPOF resilience)
// ---------------------------------------------------------------------------

// MsgBufferedModeHeartbeat sends a periodic pulse to admins during Kafka outage.
// This confirms the system is still ingesting events into the local DLQ.
func MsgBufferedModeHeartbeat(breakerState string, dlqSize int64, degradedFor string, totalBuffered int64, timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 💛 БУФЕРЛІК РЕЖИМ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Күйі:</b>              <code>БУФЕРЛІК РЕЖИМ</code>\n"+
			"<b>Ажыратқыш:</b>         <code>%s</code>\n"+
			"<b>DLQ өлшемі:</b>        <code>%d оқиға</code>\n"+
			"<b>Буферленген барлығы:</b> <code>%d оқиға</code>\n"+
			"<b>Ұзақтығы:</b>          <code>%s</code>\n"+
			"<b>Уақыт:</b>             <code>%s</code>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>⚠️ Kafka қолжетімсіз — оқиғалар жергілікті сақталуда</code>\n"+
			"\n<i>Жүйе оқиғаларды қабылдауда. Kafka қалпына келгенде автоматты түрде жіберіледі.</i>",
		separator, breakerState, dlqSize, totalBuffered, degradedFor, timestamp, separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 💛 BUFFERED MODE — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Status:</b>         <code>BUFFERED MODE</code>\n"+
			"<b>Circuit Breaker:</b> <code>%s</code>\n"+
			"<b>DLQ Size:</b>       <code>%d events</code>\n"+
			"<b>Total Buffered:</b> <code>%d events</code>\n"+
			"<b>Duration:</b>       <code>%s</code>\n"+
			"<b>Time:</b>           <code>%s</code>\n"+
			"%s\n"+
			"<b>Health:</b> <code>Kafka unreachable — events buffered locally</code>\n"+
			"\n<i>System is accepting events. Auto-drain will resume when Kafka recovers.</i>",
		separator, breakerState, dlqSize, totalBuffered, degradedFor, timestamp, separator,
	)

	return bilingual(kz, en)
}

// ---------------------------------------------------------------------------
// 17. Kafka Recovery Notification
// ---------------------------------------------------------------------------

// MsgKafkaRecovered notifies admins that Kafka connectivity was restored and
// the system has exited buffered mode. Auto-drain is in progress.
func MsgKafkaRecovered(degradedFor string, dlqRemaining int64, timestamp string) string {
	kz := fmt.Sprintf(
		"<b>🇰🇿 ✅ KAFKA ҚАЛПЫНА КЕЛДІ — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Күйі:</b>            <code>ОНЛАЙН</code>\n"+
			"<b>Буферлік ұзақтығы:</b> <code>%s</code>\n"+
			"<b>DLQ қалдығы:</b>      <code>%d оқиға</code>\n"+
			"<b>Уақыт:</b>           <code>%s</code>\n"+
			"%s\n"+
			"<b>Денсаулығы:</b> <code>Жүйе қалыпты жұмыс істеп тұр</code>\n"+
			"\n<i>Буферленген оқиғалар автоматты түрде Kafka-ға жіберілуде.</i>",
		separator, degradedFor, dlqRemaining, timestamp, separator,
	)

	en := fmt.Sprintf(
		"<b>🇬🇧 ✅ KAFKA RECOVERED — ARGUS AI</b>\n"+
			"%s\n"+
			"<b>Status:</b>            <code>ONLINE</code>\n"+
			"<b>Buffered Duration:</b> <code>%s</code>\n"+
			"<b>DLQ Remaining:</b>     <code>%d events</code>\n"+
			"<b>Time:</b>              <code>%s</code>\n"+
			"%s\n"+
			"<b>Health:</b> <code>System Nominal</code>\n"+
			"\n<i>Buffered events are being auto-drained to Kafka.</i>",
		separator, degradedFor, dlqRemaining, timestamp, separator,
	)

	return bilingual(kz, en)
}
