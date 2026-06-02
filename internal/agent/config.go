package agent

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration for the desktop agent.
type Config struct {
	// SessionToken is the JWT argusSessionToken from the SDK (required).
	SessionToken string

	// ServerURL is the Argus backend base URL (required).
	ServerURL string

	// LocalPort is the port where the agent exposes its local health server.
	// The SDK checks http://localhost:<port>/health to confirm the agent is running.
	// Default: 7373.
	LocalPort int

	// ScanIntervalSec is how often to scan processes/monitors/VM. Default: 30.
	ScanIntervalSec int

	// HeartbeatIntervalSec is how often to send heartbeat to backend. Default: 60.
	HeartbeatIntervalSec int

	// ExamID and StudentID are passed through to events for context.
	ExamID    string
	StudentID string
	OrgID     string

	// ForbiddenProcesses is the list of process name substrings to detect.
	// Populated from defaults + policy override from server.
	ForbiddenProcesses []string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	port := 7373
	if p := os.Getenv("ARGUS_AGENT_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}
	return Config{
		SessionToken:         os.Getenv("ARGUS_SESSION_TOKEN"),
		ServerURL:            os.Getenv("ARGUS_SERVER_URL"),
		LocalPort:            port,
		ScanIntervalSec:      30,
		HeartbeatIntervalSec: 60,
		ForbiddenProcesses:   DefaultForbiddenProcesses(),
	}
}

func (c Config) ScanInterval() time.Duration {
	if c.ScanIntervalSec < 5 {
		return 30 * time.Second
	}
	return time.Duration(c.ScanIntervalSec) * time.Second
}

func (c Config) HeartbeatInterval() time.Duration {
	if c.HeartbeatIntervalSec < 10 {
		return 60 * time.Second
	}
	return time.Duration(c.HeartbeatIntervalSec) * time.Second
}

// DefaultForbiddenProcesses returns the default list of forbidden process
// name fragments. Detection is case-insensitive substring match.
func DefaultForbiddenProcesses() []string {
	return []string{
		// Remote access / desktop sharing
		"anydesk",
		"teamviewer",
		"radmin",
		"uvnc",
		"tigervnc",
		"ultravnc",
		"tightvnc",
		"realvnc",
		"vnc",
		"rustdesk",
		"parsec",
		"nomachine",
		"chrome remote desktop",
		"remotepc",
		"splashtop",
		// Screen recording / capture
		"obs",
		"bandicam",
		"fraps",
		"xsplit",
		"shadowplay",
		"action recorder",
		// Screen sharing in conferencing
		"discord",
		"zoom",
		"teams",
		"skype",
		"webex",
		"gotomeeting",
		// AI cheating assistance
		"chatgpt",
		// Virtual camera injection
		"virtual camera",
		"manycam",
		"splitcam",
	}
}
