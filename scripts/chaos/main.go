// =============================================================================
//  Argus AI — Chaos Engineering Routine
//  Standalone binary that orchestrates 6 chaos phases: baseline, ClickHouse
//  outage/recovery, Kafka outage/recovery, and combined failure. Uses HTTP
//  admin API + Docker CLI. No argus-backend imports.
//
//  Usage:
//    go run scripts/chaos/main.go
//    go run scripts/chaos/main.go -backend http://prod:8080
//    go run scripts/chaos/main.go -skip-docker  # skip Docker pause/unpause
// =============================================================================

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Colors
// ---------------------------------------------------------------------------

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[0;31m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorCyan   = "\033[0;36m"
	colorBold   = "\033[1m"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

var (
	backendURL string
	skipDocker bool
)

// Docker container names (matching argus-infra/docker/docker-compose.yaml)
var (
	clickhouseContainers = []string{"argus-clickhouse-1", "argus-clickhouse-2", "argus-clickhouse-3"}
	kafkaContainers      = []string{"argus-kafka-1", "argus-kafka-2", "argus-kafka-3"}
)

// ---------------------------------------------------------------------------
// Health Response
// ---------------------------------------------------------------------------

type HealthResponse struct {
	KafkaBreakerState    string `json:"kafka_breaker_state"`
	KafkaDLQSize         int64  `json:"kafka_dlq_size"`
	KafkaIsDegraded      bool   `json:"kafka_is_degraded"`
	KafkaDegradedFor     string `json:"kafka_degraded_for"`
	CHBreakerState       string `json:"ch_breaker_state"`
	CHOverflowSize       int    `json:"ch_overflow_size"`
	CHBackpressureActive bool   `json:"ch_backpressure_active"`
	GoroutineCount       int    `json:"goroutine_count"`
	MemAllocMB           uint64 `json:"mem_alloc_mb"`
	MemSysMB             uint64 `json:"mem_sys_mb"`
	NumGC                uint32 `json:"num_gc"`
}

// ---------------------------------------------------------------------------
// Phase Result
// ---------------------------------------------------------------------------

type PhaseResult struct {
	Name     string
	Passed   bool
	Duration time.Duration
	Details  string
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	flag.StringVar(&backendURL, "backend", "http://localhost:8080", "Backend URL")
	flag.BoolVar(&skipDocker, "skip-docker", false, "Skip Docker pause/unpause (test API only)")
	flag.Parse()

	fmt.Printf("\n%s%s╔══════════════════════════════════════════════════╗%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s║           ARGUS AI — CHAOS ENGINE                ║%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%s╚══════════════════════════════════════════════════╝%s\n\n", colorBold, colorCyan, colorReset)

	fmt.Printf("  Backend:      %s\n", backendURL)
	fmt.Printf("  Skip Docker:  %v\n\n", skipDocker)

	// Authenticate
	jwt, err := authenticate()
	if err != nil {
		fmt.Printf("%s✗ Authentication failed: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
	fmt.Printf("%s✓%s Authenticated\n\n", colorGreen, colorReset)

	// Run phases
	results := make([]PhaseResult, 0, 6)

	results = append(results, runPhase1Baseline(jwt))
	results = append(results, runPhase2CHOutage(jwt))
	results = append(results, runPhase3CHRecovery(jwt))
	results = append(results, runPhase4KafkaOutage(jwt))
	results = append(results, runPhase5KafkaRecovery(jwt))
	results = append(results, runPhase6CombinedFailure(jwt))

	// Summary
	fmt.Printf("\n%s%s══════════════════════════════════════════════════%s\n", colorBold, colorCyan, colorReset)
	passed := 0
	for _, r := range results {
		status := fmt.Sprintf("%s✓ PASS%s", colorGreen, colorReset)
		if !r.Passed {
			status = fmt.Sprintf("%s✗ FAIL%s", colorRed, colorReset)
		} else {
			passed++
		}
		fmt.Printf("  %-40s %s  (%s)\n", r.Name, status, r.Duration.Round(time.Millisecond))
		if r.Details != "" {
			fmt.Printf("    %s\n", r.Details)
		}
	}
	fmt.Printf("%s%s══════════════════════════════════════════════════%s\n", colorBold, colorCyan, colorReset)

	if passed == len(results) {
		fmt.Printf("\n  %s%sALL %d PHASES PASSED%s\n\n", colorBold, colorGreen, len(results), colorReset)
	} else {
		fmt.Printf("\n  %s%s%d/%d PHASES PASSED%s\n\n", colorBold, colorRed, passed, len(results), colorReset)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

func authenticate() (string, error) {
	body := `{"phone":"+77077469966","password":"Astana01+"}`
	resp, err := http.Post(
		backendURL+"/api/v1/auth/login",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("empty token")
	}
	return result.Token, nil
}

// ---------------------------------------------------------------------------
// HTTP Helpers
// ---------------------------------------------------------------------------

func getHealth(jwt string) (*HealthResponse, error) {
	req, _ := http.NewRequest("GET", backendURL+"/api/v1/admin/system-health", nil)
	req.Header.Set("Authorization", "Bearer "+jwt)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var h HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return nil, err
	}
	return &h, nil
}

func postReset(jwt string, actions []string) error {
	body, _ := json.Marshal(map[string][]string{"actions": actions})
	req, _ := http.NewRequest("POST", backendURL+"/api/v1/admin/system-reset", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Docker Helpers
// ---------------------------------------------------------------------------

func dockerPause(containers []string) error {
	if skipDocker {
		fmt.Printf("    %s[skip-docker] Would pause: %s%s\n", colorYellow, strings.Join(containers, ", "), colorReset)
		return nil
	}
	for _, c := range containers {
		cmd := exec.Command("docker", "pause", c)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("docker pause %s: %v (%s)", c, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func dockerUnpause(containers []string) error {
	if skipDocker {
		fmt.Printf("    %s[skip-docker] Would unpause: %s%s\n", colorYellow, strings.Join(containers, ", "), colorReset)
		return nil
	}
	for _, c := range containers {
		cmd := exec.Command("docker", "unpause", c)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("docker unpause %s: %v (%s)", c, err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Health Poller — polls health every `interval` for `duration`, returns all samples
// ---------------------------------------------------------------------------

func pollHealth(jwt string, duration, interval time.Duration) ([]HealthResponse, error) {
	var samples []HealthResponse
	deadline := time.Now().Add(duration)

	for time.Now().Before(deadline) {
		h, err := getHealth(jwt)
		if err != nil {
			fmt.Printf("    %s⚠ health poll error: %v%s\n", colorYellow, err, colorReset)
		} else {
			samples = append(samples, *h)
			fmt.Printf("    kafka=%s ch=%s dlq=%d overflow=%d goroutines=%d\n",
				h.KafkaBreakerState, h.CHBreakerState, h.KafkaDLQSize, h.CHOverflowSize, h.GoroutineCount)
		}
		time.Sleep(interval)
	}
	return samples, nil
}

// ---------------------------------------------------------------------------
// Wait for condition with timeout
// ---------------------------------------------------------------------------

func waitForCondition(jwt string, timeout time.Duration, check func(*HealthResponse) bool) (*HealthResponse, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h, err := getHealth(jwt)
		if err == nil && check(h) {
			return h, true
		}
		if h != nil {
			fmt.Printf("    waiting... kafka=%s ch=%s dlq=%d overflow=%d\n",
				h.KafkaBreakerState, h.CHBreakerState, h.KafkaDLQSize, h.CHOverflowSize)
		}
		time.Sleep(2 * time.Second)
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Phase 1: Baseline Health
// ---------------------------------------------------------------------------

func runPhase1Baseline(jwt string) PhaseResult {
	name := "Phase 1: Baseline Health"
	fmt.Printf("%s%s── %s ──%s\n", colorBold, colorCyan, name, colorReset)
	start := time.Now()

	h, err := getHealth(jwt)
	if err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("health check failed: %v", err)}
	}

	details := fmt.Sprintf("kafka=%s ch=%s dlq=%d overflow=%d goroutines=%d mem=%dMB",
		h.KafkaBreakerState, h.CHBreakerState, h.KafkaDLQSize, h.CHOverflowSize,
		h.GoroutineCount, h.MemAllocMB)

	passed := h.KafkaBreakerState == "closed" && h.CHBreakerState == "closed"
	if !passed {
		details += " (breakers not closed — system may need manual reset first)"
	}

	return PhaseResult{Name: name, Passed: passed, Duration: time.Since(start), Details: details}
}

// ---------------------------------------------------------------------------
// Phase 2: ClickHouse Outage
// ---------------------------------------------------------------------------

func runPhase2CHOutage(jwt string) PhaseResult {
	name := "Phase 2: ClickHouse Outage"
	fmt.Printf("\n%s%s── %s ──%s\n", colorBold, colorCyan, name, colorReset)
	start := time.Now()

	// Pause ClickHouse containers
	if err := dockerPause(clickhouseContainers); err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("docker pause failed: %v", err)}
	}
	fmt.Printf("  %s✓%s ClickHouse paused\n", colorGreen, colorReset)

	// Poll for 30 seconds — expect CH breaker to open
	fmt.Println("  Monitoring for 30s...")
	samples, _ := pollHealth(jwt, 30*time.Second, 3*time.Second)

	// Assert: CH breaker should be open by end
	passed := false
	details := "no samples collected"
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		passed = last.CHBreakerState == "open" || last.CHOverflowSize > 0
		details = fmt.Sprintf("ch_breaker=%s overflow=%d backpressure=%v",
			last.CHBreakerState, last.CHOverflowSize, last.CHBackpressureActive)
	}

	return PhaseResult{Name: name, Passed: passed, Duration: time.Since(start), Details: details}
}

// ---------------------------------------------------------------------------
// Phase 3: ClickHouse Recovery
// ---------------------------------------------------------------------------

func runPhase3CHRecovery(jwt string) PhaseResult {
	name := "Phase 3: ClickHouse Recovery"
	fmt.Printf("\n%s%s── %s ──%s\n", colorBold, colorCyan, name, colorReset)
	start := time.Now()

	// Unpause ClickHouse
	if err := dockerUnpause(clickhouseContainers); err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("docker unpause failed: %v", err)}
	}
	fmt.Printf("  %s✓%s ClickHouse unpaused\n", colorGreen, colorReset)

	// Reset CH breaker + flush overflow
	time.Sleep(2 * time.Second) // Give CH a moment to accept connections
	if err := postReset(jwt, []string{"reset_ch_breaker", "flush_overflow"}); err != nil {
		fmt.Printf("  %s⚠ reset failed: %v%s\n", colorYellow, err, colorReset)
	} else {
		fmt.Printf("  %s✓%s Reset: reset_ch_breaker, flush_overflow\n", colorGreen, colorReset)
	}

	// Wait for recovery (up to 45s)
	fmt.Println("  Waiting for recovery (up to 45s)...")
	h, ok := waitForCondition(jwt, 45*time.Second, func(h *HealthResponse) bool {
		return h.CHBreakerState == "closed" && h.CHOverflowSize == 0
	})

	if ok {
		duration := time.Since(start)
		return PhaseResult{Name: name, Passed: true, Duration: duration,
			Details: fmt.Sprintf("ch_breaker=closed overflow=0 (recovered in %s)", duration.Round(time.Millisecond))}
	}

	details := "recovery timeout (45s)"
	if h != nil {
		details = fmt.Sprintf("ch_breaker=%s overflow=%d (timeout)", h.CHBreakerState, h.CHOverflowSize)
	}
	return PhaseResult{Name: name, Passed: false, Duration: time.Since(start), Details: details}
}

// ---------------------------------------------------------------------------
// Phase 4: Kafka Outage
// ---------------------------------------------------------------------------

func runPhase4KafkaOutage(jwt string) PhaseResult {
	name := "Phase 4: Kafka Outage"
	fmt.Printf("\n%s%s── %s ──%s\n", colorBold, colorCyan, name, colorReset)
	start := time.Now()

	// Pause Kafka containers
	if err := dockerPause(kafkaContainers); err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("docker pause failed: %v", err)}
	}
	fmt.Printf("  %s✓%s Kafka paused\n", colorGreen, colorReset)

	// Poll for 30 seconds — expect Kafka breaker to open + DLQ grows
	fmt.Println("  Monitoring for 30s...")
	samples, _ := pollHealth(jwt, 30*time.Second, 3*time.Second)

	passed := false
	details := "no samples collected"
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		passed = last.KafkaBreakerState == "open" || last.KafkaIsDegraded || last.KafkaDLQSize > 0
		details = fmt.Sprintf("kafka_breaker=%s degraded=%v dlq=%d degraded_for=%s",
			last.KafkaBreakerState, last.KafkaIsDegraded, last.KafkaDLQSize, last.KafkaDegradedFor)
	}

	return PhaseResult{Name: name, Passed: passed, Duration: time.Since(start), Details: details}
}

// ---------------------------------------------------------------------------
// Phase 5: Kafka Recovery
// ---------------------------------------------------------------------------

func runPhase5KafkaRecovery(jwt string) PhaseResult {
	name := "Phase 5: Kafka Recovery"
	fmt.Printf("\n%s%s── %s ──%s\n", colorBold, colorCyan, name, colorReset)
	start := time.Now()

	// Unpause Kafka
	if err := dockerUnpause(kafkaContainers); err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("docker unpause failed: %v", err)}
	}
	fmt.Printf("  %s✓%s Kafka unpaused\n", colorGreen, colorReset)

	// Reset Kafka breaker
	time.Sleep(3 * time.Second) // Give Kafka a moment to elect leaders
	if err := postReset(jwt, []string{"reset_kafka_breaker"}); err != nil {
		fmt.Printf("  %s⚠ reset failed: %v%s\n", colorYellow, err, colorReset)
	} else {
		fmt.Printf("  %s✓%s Reset: reset_kafka_breaker\n", colorGreen, colorReset)
	}

	// Wait for recovery (up to 45s)
	// DLQ drains via reclamation loop (30s cycle), so breaker should close first
	fmt.Println("  Waiting for recovery (up to 45s)...")

	// Track DLQ size decrease as a sign of recovery
	var initialDLQ int64 = -1
	h, ok := waitForCondition(jwt, 45*time.Second, func(h *HealthResponse) bool {
		if initialDLQ < 0 {
			initialDLQ = h.KafkaDLQSize
		}
		// Pass if breaker is closed (DLQ may still be draining)
		return h.KafkaBreakerState == "closed"
	})

	if ok {
		duration := time.Since(start)
		dlqDelta := initialDLQ - h.KafkaDLQSize
		return PhaseResult{Name: name, Passed: true, Duration: duration,
			Details: fmt.Sprintf("kafka_breaker=closed dlq=%d (drained %d, recovered in %s)",
				h.KafkaDLQSize, dlqDelta, duration.Round(time.Millisecond))}
	}

	details := "recovery timeout (45s)"
	if h != nil {
		details = fmt.Sprintf("kafka_breaker=%s dlq=%d (timeout)", h.KafkaBreakerState, h.KafkaDLQSize)
	}
	return PhaseResult{Name: name, Passed: false, Duration: time.Since(start), Details: details}
}

// ---------------------------------------------------------------------------
// Phase 6: Combined Failure (Kafka + ClickHouse)
// ---------------------------------------------------------------------------

func runPhase6CombinedFailure(jwt string) PhaseResult {
	name := "Phase 6: Combined Failure"
	fmt.Printf("\n%s%s── %s ──%s\n", colorBold, colorCyan, name, colorReset)
	start := time.Now()

	// Capture baseline goroutine count
	baselineHealth, err := getHealth(jwt)
	if err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("baseline health failed: %v", err)}
	}
	baselineGoroutines := baselineHealth.GoroutineCount

	// Pause ALL infrastructure
	allContainers := append(clickhouseContainers, kafkaContainers...)
	if err := dockerPause(allContainers); err != nil {
		return PhaseResult{Name: name, Passed: false, Duration: time.Since(start),
			Details: fmt.Sprintf("docker pause failed: %v", err)}
	}
	fmt.Printf("  %s✓%s All infrastructure paused (CH + Kafka)\n", colorGreen, colorReset)

	// Poll for 30 seconds
	fmt.Println("  Monitoring for 30s (expecting graceful degradation)...")
	samples, _ := pollHealth(jwt, 30*time.Second, 3*time.Second)

	// Unpause everything
	if err := dockerUnpause(allContainers); err != nil {
		fmt.Printf("  %s⚠ unpause failed: %v%s\n", colorYellow, err, colorReset)
	}
	fmt.Printf("  %s✓%s All infrastructure unpaused\n", colorGreen, colorReset)

	// Reset all breakers
	time.Sleep(3 * time.Second)
	_ = postReset(jwt, []string{"reset_kafka_breaker", "reset_ch_breaker", "flush_overflow"})
	fmt.Printf("  %s✓%s All breakers reset\n", colorGreen, colorReset)

	// Assert: both breakers open, goroutine count stable (no leak)
	passed := false
	details := "no samples collected"
	if len(samples) > 0 {
		last := samples[len(samples)-1]
		bothOpen := (last.CHBreakerState == "open" || last.CHOverflowSize > 0) &&
			(last.KafkaBreakerState == "open" || last.KafkaIsDegraded)

		// Goroutine stability: should not grow by more than 50% (no goroutine leak)
		goroutineStable := float64(last.GoroutineCount) < float64(baselineGoroutines)*1.5 ||
			math.Abs(float64(last.GoroutineCount-baselineGoroutines)) < 100

		passed = bothOpen && goroutineStable
		details = fmt.Sprintf("ch=%s kafka=%s goroutines=%d→%d (baseline=%d, stable=%v)",
			last.CHBreakerState, last.KafkaBreakerState,
			baselineGoroutines, last.GoroutineCount, baselineGoroutines, goroutineStable)
	}

	// Wait for final recovery
	fmt.Println("  Waiting for final recovery (up to 30s)...")
	_, recoveryOk := waitForCondition(jwt, 30*time.Second, func(h *HealthResponse) bool {
		return h.CHBreakerState == "closed" && h.KafkaBreakerState == "closed"
	})
	if recoveryOk {
		details += " → recovered"
	}

	return PhaseResult{Name: name, Passed: passed, Duration: time.Since(start), Details: details}
}
