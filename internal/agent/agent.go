// Package agent implements the Argus desktop proctoring agent.
//
// The agent runs as a background process on the student's machine and:
//   - Scans running processes for forbidden applications (AnyDesk, TeamViewer, etc.)
//   - Detects virtual machine environments
//   - Counts connected monitors (multi-monitor cheating prevention)
//   - Collects a hardware fingerprint for session binding
//   - Sends events to the Argus backend via the student's session token
//   - Exposes a local health server (default :7373) so the SDK can verify
//     the agent is running before allowing the exam to start
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/argus-ai/event-collector/internal/agent/reporter"
	"github.com/argus-ai/event-collector/internal/agent/scanner"
)

const agentVersion = "1.0.0"

// Agent orchestrates system scanning and backend reporting.
type Agent struct {
	cfg      Config
	client   *reporter.Client
	sysInfo  scanner.SystemInfo
	mu       sync.RWMutex

	// lastScan holds the result of the most recent process scan.
	lastScan scanResult

	// usbBaseline is the USB snapshot at session start (for change detection).
	usbBaseline scanner.USBSnapshot
}

type scanResult struct {
	At             time.Time
	ForbiddenProcs []scanner.ProcessInfo
	TotalProcesses int
	MonitorCount   int
	IsVM           bool
	VMProduct      string

	// RemoteAccess holds detected remote-control tools (AnyDesk, RustDesk, …).
	RemoteAccess []scanner.RemoteAccessTool
	// RemoteActive is true when a live remote session is detected.
	RemoteActive bool
	// Blocked is true when the exam should be hard-blocked (strict enforcement
	// + an active remote session). The SDK reads this from /health.
	Blocked bool
}

// New creates a new Agent with the given configuration.
func New(cfg Config) *Agent {
	if cfg.LocalPort == 0 {
		cfg.LocalPort = 7373
	}
	return &Agent{
		cfg:    cfg,
		client: reporter.NewClient(cfg.ServerURL, cfg.SessionToken),
	}
}

// Run starts the agent: collects system info, starts scan loop, starts
// local health server, and blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) error {
	// Initial system info collection.
	a.sysInfo = scanner.CollectSystemInfo()

	log.Printf("[argus-agent] started v%s os=%s arch=%s host=%s machineId=%s",
		agentVersion, runtime.GOOS, runtime.GOARCH, a.sysInfo.Hostname, a.sysInfo.MachineID)

	// Capture USB baseline at session start.
	if snap, err := scanner.NewUSBSnapshot(); err == nil {
		a.usbBaseline = snap
		log.Printf("[argus-agent] USB baseline: %d devices", len(snap.Devices))
	}

	if a.sysInfo.IsVM {
		log.Printf("[argus-agent] WARNING: VM detected: %s", a.sysInfo.VMProduct)
		a.reportVMDetected(ctx)
	}

	if a.sysInfo.MonitorCount > 1 {
		log.Printf("[argus-agent] WARNING: %d monitors detected", a.sysInfo.MonitorCount)
		a.reportMultiMonitor(ctx)
	}

	// Start local health HTTP server.
	srv := a.buildLocalServer()
	go func() {
		addr := fmt.Sprintf("127.0.0.1:%d", a.cfg.LocalPort)
		log.Printf("[argus-agent] local health server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[argus-agent] health server error: %v", err)
		}
	}()

	// Scan ticker.
	scanTicker := time.NewTicker(a.cfg.ScanInterval())
	defer scanTicker.Stop()

	// Heartbeat ticker.
	hbTicker := time.NewTicker(a.cfg.HeartbeatInterval())
	defer hbTicker.Stop()

	// Run initial scan immediately.
	a.runScan(ctx)

	for {
		select {
		case <-ctx.Done():
			shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			srv.Shutdown(shutCtx) //nolint:errcheck
			log.Printf("[argus-agent] shutting down")
			return nil

		case <-scanTicker.C:
			a.runScan(ctx)

		case <-hbTicker.C:
			a.sendHeartbeat(ctx)
		}
	}
}

// ---------------------------------------------------------------------------
// Scan
// ---------------------------------------------------------------------------

func (a *Agent) runScan(ctx context.Context) {
	procs, err := scanner.ScanProcesses()
	if err != nil {
		log.Printf("[argus-agent] process scan error: %v", err)
		return
	}

	forbidden := scanner.FindForbiddenProcesses(procs, a.cfg.ForbiddenProcesses)
	monCount := scanner.CollectSystemInfo().MonitorCount

	// Remote-access detection: process signatures + active network sessions.
	remote := scanner.ClassifyRemoteAccessProcesses(procs)
	if conns, err := scanner.ScanConnections(); err == nil {
		remote = scanner.MergeRemoteAccess(remote, scanner.DetectRemoteAccessPorts(conns))
	}
	remoteActive := scanner.AnyActiveRemoteAccess(remote)
	blocked := a.cfg.EnforceBlockRemoteAccess && remoteActive

	// Report remote-access tools separately (stronger, specific signal) and
	// exclude them from the generic forbidden report to avoid duplicates.
	otherForbidden := make([]scanner.ProcessInfo, 0, len(forbidden))
	for _, p := range forbidden {
		if !scanner.IsRemoteAccessProcess(p) {
			otherForbidden = append(otherForbidden, p)
		}
	}

	a.mu.Lock()
	a.lastScan = scanResult{
		At:             time.Now(),
		ForbiddenProcs: forbidden,
		TotalProcesses: len(procs),
		MonitorCount:   monCount,
		IsVM:           a.sysInfo.IsVM,
		VMProduct:      a.sysInfo.VMProduct,
		RemoteAccess:   remote,
		RemoteActive:   remoteActive,
		Blocked:        blocked,
	}
	a.mu.Unlock()

	if len(remote) > 0 {
		log.Printf("[argus-agent] remote-access tools detected: %d (active=%v, blocked=%v)", len(remote), remoteActive, blocked)
		a.reportRemoteAccess(ctx, remote)
	}

	if len(otherForbidden) > 0 {
		log.Printf("[argus-agent] forbidden processes detected: %d", len(otherForbidden))
		a.reportForbiddenProcesses(ctx, otherForbidden)
	}

	// USB device change detection (new device added during exam = suspicious)
	if snap, err := scanner.NewUSBSnapshot(); err == nil {
		if a.usbBaseline.TakenAt.IsZero() {
			a.usbBaseline = snap
		} else {
			diff := snap.Diff(a.usbBaseline)
			for _, added := range diff.Added {
				log.Printf("[argus-agent] USB device added: %s %s", added.Vendor, added.Product)
				a.reportUSBChange(ctx, added, "added")
			}
			// Update baseline to current state (only alert on new additions)
			a.usbBaseline = snap
		}
	}

	// Report multi-monitor dynamically (e.g., second monitor plugged in during exam).
	if monCount > 1 && monCount > a.sysInfo.MonitorCount {
		a.sysInfo.MonitorCount = monCount
		a.reportMultiMonitor(ctx)
	}
}

// ---------------------------------------------------------------------------
// Event reporting
// ---------------------------------------------------------------------------

func (a *Agent) reportForbiddenProcesses(ctx context.Context, procs []scanner.ProcessInfo) {
	events := make([]reporter.AgentEvent, 0, len(procs))
	for _, p := range procs {
		events = append(events, reporter.AgentEvent{
			EventType:  "FORBIDDEN_PROCESS_DETECTED",
			Severity:   "critical",
			Source:     "KERNEL_AGENT",
			Label:      fmt.Sprintf("Запрещённый процесс: %s", p.Name),
			Confidence: 0.99,
			Details:    fmt.Sprintf("pid=%s cmd=%s", p.PID, truncate(p.Cmd, 200)),
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		})
	}
	if err := a.client.SendEvents(ctx, events); err != nil {
		log.Printf("[argus-agent] failed to report forbidden processes: %v", err)
	}
}

func (a *Agent) reportRemoteAccess(ctx context.Context, tools []scanner.RemoteAccessTool) {
	events := make([]reporter.AgentEvent, 0, len(tools))
	for _, t := range tools {
		label := fmt.Sprintf("Средство удалённого доступа: %s", t.Name)
		if t.Active {
			label = fmt.Sprintf("Активная удалённая сессия: %s", t.Name)
		}
		confidence := 0.9
		if t.Active {
			confidence = 0.99
		}
		events = append(events, reporter.AgentEvent{
			EventType:  "REMOTE_ACCESS_DETECTED",
			Severity:   "critical",
			Source:     "KERNEL_AGENT",
			Label:      label,
			Confidence: confidence,
			Details:    fmt.Sprintf("tool=%s via=%s active=%v %s", t.Name, t.Via, t.Active, t.Detail),
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		})
	}
	if err := a.client.SendEvents(ctx, events); err != nil {
		log.Printf("[argus-agent] failed to report remote access: %v", err)
	}
}

func (a *Agent) reportVMDetected(ctx context.Context) {
	events := []reporter.AgentEvent{{
		EventType:  "VIRTUAL_MACHINE_DETECTED",
		Severity:   "critical",
		Source:     "KERNEL_AGENT",
		Label:      fmt.Sprintf("Виртуальная машина: %s", a.sysInfo.VMProduct),
		Confidence: 0.95,
		Details:    fmt.Sprintf("product=%s", a.sysInfo.VMProduct),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}}
	if err := a.client.SendEvents(ctx, events); err != nil {
		log.Printf("[argus-agent] failed to report VM: %v", err)
	}
}

func (a *Agent) reportUSBChange(ctx context.Context, dev scanner.USBDevice, action string) {
	events := []reporter.AgentEvent{{
		EventType:  "HARDWARE_DEVICE_ANOMALY",
		Severity:   "warning",
		Source:     "KERNEL_AGENT",
		Label:      fmt.Sprintf("USB устройство %s: %s %s", action, dev.Vendor, dev.Product),
		Confidence: 0.95,
		Details:    fmt.Sprintf("action=%s id=%s vendor=%s product=%s class=%s", action, dev.ID, dev.Vendor, dev.Product, dev.Class),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}}
	if err := a.client.SendEvents(ctx, events); err != nil {
		log.Printf("[argus-agent] failed to report USB change: %v", err)
	}
}

func (a *Agent) reportMultiMonitor(ctx context.Context) {
	a.mu.RLock()
	count := a.lastScan.MonitorCount
	if count == 0 {
		count = a.sysInfo.MonitorCount
	}
	a.mu.RUnlock()

	events := []reporter.AgentEvent{{
		EventType:  "VIRTUAL_MONITOR_DETECTED",
		Severity:   "warning",
		Source:     "KERNEL_AGENT",
		Label:      fmt.Sprintf("Обнаружено %d монитора(ов)", count),
		Confidence: 0.98,
		Details:    fmt.Sprintf("monitor_count=%d", count),
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}}
	if err := a.client.SendEvents(ctx, events); err != nil {
		log.Printf("[argus-agent] failed to report multi-monitor: %v", err)
	}
}

func (a *Agent) sendHeartbeat(ctx context.Context) {
	a.mu.RLock()
	scan := a.lastScan
	a.mu.RUnlock()

	forbiddenNames := make([]string, 0, len(scan.ForbiddenProcs))
	for _, p := range scan.ForbiddenProcs {
		forbiddenNames = append(forbiddenNames, p.Name)
	}

	hb := reporter.AgentHeartbeat{
		SessionToken:   a.cfg.SessionToken,
		AgentVersion:   agentVersion,
		OS:             runtime.GOOS,
		Hostname:       a.sysInfo.Hostname,
		MachineID:      a.sysInfo.MachineID,
		MonitorCount:   scan.MonitorCount,
		IsVM:           scan.IsVM,
		VMProduct:      scan.VMProduct,
		ProcessCount:   scan.TotalProcesses,
		ForbiddenProcs: forbiddenNames,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	if err := a.client.SendHeartbeat(ctx, hb); err != nil {
		log.Printf("[argus-agent] heartbeat failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Local health server (SDK checks http://localhost:7373/health)
// ---------------------------------------------------------------------------

func (a *Agent) buildLocalServer() *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		a.mu.RLock()
		scan := a.lastScan
		a.mu.RUnlock()

		forbiddenNames := make([]string, 0, len(scan.ForbiddenProcs))
		for _, p := range scan.ForbiddenProcs {
			forbiddenNames = append(forbiddenNames, p.Name)
		}

		remoteTools := make([]string, 0, len(scan.RemoteAccess))
		for _, t := range scan.RemoteAccess {
			remoteTools = append(remoteTools, t.Name)
		}

		resp := map[string]interface{}{
			"status":         "ok",
			"version":        agentVersion,
			"os":             runtime.GOOS,
			"machineId":      a.sysInfo.MachineID,
			"monitorCount":   scan.MonitorCount,
			"isVm":           scan.IsVM,
			"vmProduct":      scan.VMProduct,
			"forbiddenProcs": forbiddenNames,
			"remoteAccess": map[string]interface{}{
				"detected": len(scan.RemoteAccess) > 0,
				"active":   scan.RemoteActive,
				"tools":    remoteTools,
			},
			// shouldBlock tells the SDK to hard-block the exam (strict mode +
			// active remote session).
			"shouldBlock": scan.Blocked,
			"lastScanAt":  scan.At.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})

	mux.HandleFunc("OPTIONS /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"forbiddenProcesses": a.cfg.ForbiddenProcesses,
			"scanIntervalSec":    a.cfg.ScanIntervalSec,
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	})

	return &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", a.cfg.LocalPort),
		Handler: mux,
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
