// Argus Desktop Agent — background process monitor and proctoring enforcer.
//
// Usage:
//
//	argus-agent --session-token=eyJ... --server-url=https://argusai.kz
//	argus-agent --session-token=eyJ... --server-url=https://argusai.kz --port=7373
//
// Environment variables (alternative to flags):
//
//	ARGUS_SESSION_TOKEN  — student session JWT
//	ARGUS_SERVER_URL     — Argus backend base URL
//	ARGUS_AGENT_PORT     — local health port (default 7373)
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/argus-ai/event-collector/internal/agent"
)

func main() {
	var (
		sessionToken = flag.String("session-token", "", "Argus JWT session token (argusSessionToken from SDK)")
		serverURL    = flag.String("server-url", "", "Argus backend base URL (e.g. https://argusai.kz)")
		port         = flag.Int("port", 7373, "Local health server port (SDK checks this)")
		scanInterval = flag.Int("scan-interval", 30, "Process scan interval in seconds")
		studentID    = flag.String("student-id", "", "Student ID for event context")
		examID       = flag.String("exam-id", "", "Exam ID for event context")
	)
	flag.Parse()

	cfg := agent.DefaultConfig()

	// CLI flags override env vars.
	if *sessionToken != "" {
		cfg.SessionToken = *sessionToken
	}
	if *serverURL != "" {
		cfg.ServerURL = *serverURL
	}
	if *port != 7373 {
		cfg.LocalPort = *port
	}
	if *scanInterval != 30 {
		cfg.ScanIntervalSec = *scanInterval
	}
	if *studentID != "" {
		cfg.StudentID = *studentID
	}
	if *examID != "" {
		cfg.ExamID = *examID
	}

	if cfg.SessionToken == "" {
		log.Fatal("[argus-agent] --session-token (or ARGUS_SESSION_TOKEN) is required")
	}
	if cfg.ServerURL == "" {
		log.Fatal("[argus-agent] --server-url (or ARGUS_SERVER_URL) is required")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a := agent.New(cfg)
	if err := a.Run(ctx); err != nil {
		log.Fatalf("[argus-agent] error: %v", err)
	}
}
