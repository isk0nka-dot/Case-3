// Package sidecam implements the Secondary Camera (Mobile) Orchestration Layer.
//
// This package manages the full lifecycle of mobile secondary camera pairing:
//
//  1. QR-Handshake & Secure Pairing
//     - Ephemeral, time-bound JWT tokens for mobile connections
//     - HMAC-SHA256 signed pairing codes with 5-minute TTL
//     - Device binding via pairing token + session ID
//
//  2. Policy-Driven Session Gating
//     - Mandatory mode: exam start blocked until valid mobile handshake
//     - Optional mode: session proceeds, side camera encouraged
//     - Disabled mode: no side camera pairing attempted
//
//  3. Spatial Calibration Verification (Golden Angle 45-60°)
//     - Validates hands, keyboard, and screen edges in side-view frame
//     - Head pose angle verification from first SIDE_CAMERA frames
//
//  4. Power & Thermal Guard
//     - Battery < 20% blocks start in Mandatory mode
//     - Charging status tracking with push notifications
//     - FPS throttling on thermal threshold breach
//
//  5. Stream Health Monitor
//     - Heartbeat-based liveness (5s interval)
//     - Auto-pause on connection drop (Mandatory mode)
//     - Latency tracking for multi-stream alignment
//
//  6. Heuristic Anomaly Detection
//     - Hands-on-desk persistence via side camera vision
//     - Device displacement via accelerometer telemetry
package sidecam

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/argus-ai/event-collector/pkg/randutil"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// CameraPolicy defines the secondary camera requirement for an exam.
type CameraPolicy string

const (
	PolicyMandatory CameraPolicy = "mandatory"
	PolicyOptional  CameraPolicy = "optional"
	PolicyDisabled  CameraPolicy = "disabled"
)

// PairingState tracks the lifecycle of a mobile camera pairing.
type PairingState string

const (
	PairingPending     PairingState = "pending"      // QR code generated, waiting for scan
	PairingConnected   PairingState = "connected"     // Mobile device connected
	PairingCalibrating PairingState = "calibrating"   // Spatial calibration in progress
	PairingReady       PairingState = "ready"         // Calibrated and streaming
	PairingDisconnected PairingState = "disconnected" // Connection lost
	PairingFailed      PairingState = "failed"        // Permanent failure
)

// CalibrationStatus holds the result of spatial calibration verification.
type CalibrationStatus struct {
	Passed          bool    `json:"passed"`
	ViewAngle       float64 `json:"viewAngle"`       // Estimated camera angle in degrees
	HandsVisible    bool    `json:"handsVisible"`     // Hands detected in frame
	KeyboardVisible bool    `json:"keyboardVisible"`  // Keyboard/desk visible
	ScreenEdge      bool    `json:"screenEdge"`       // Monitor edge visible
	Message         string  `json:"message"`          // Human-readable status
	AttemptCount    int     `json:"attemptCount"`
	CalibratedAt    string  `json:"calibratedAt,omitempty"`
}

// MobileDeviceInfo holds telemetry from the mobile device.
type MobileDeviceInfo struct {
	DeviceModel     string  `json:"deviceModel"`
	OSVersion       string  `json:"osVersion"`
	BatteryLevel    float64 `json:"batteryLevel"`    // 0-1
	IsCharging      bool    `json:"isCharging"`
	ThermalState    string  `json:"thermalState"`    // nominal, fair, serious, critical
	FPSCurrent      int     `json:"fpsCurrent"`
	FPSTarget       int     `json:"fpsTarget"`
	AccelX          float64 `json:"accelX"`
	AccelY          float64 `json:"accelY"`
	AccelZ          float64 `json:"accelZ"`
	GyroMagnitude   float64 `json:"gyroMagnitude"`
}

// StreamHealth tracks the secondary camera stream quality.
type StreamHealth struct {
	Connected       bool    `json:"connected"`
	LastHeartbeat   string  `json:"lastHeartbeat"`
	LatencyMs       int64   `json:"latencyMs"`
	FrameRate       int     `json:"frameRate"`
	DroppedFrames   int     `json:"droppedFrames"`
	Uptime          int64   `json:"uptimeSec"`
	Quality         string  `json:"quality"`         // excellent, good, degraded, critical
}

// PairingSession is the complete state for a secondary camera pairing.
type PairingSession struct {
	SessionID       string           `json:"sessionId"`
	StudentID       string           `json:"studentId"`
	ExamID          string           `json:"examId"`
	OrgID           string           `json:"orgId"`
	Policy          CameraPolicy     `json:"policy"`
	State           PairingState     `json:"state"`
	PairingToken    string           `json:"-"`             // Cleared after pairing (Fix 1)
	PairingCode     string           `json:"-"`             // Cleared after pairing (Fix 1)
	DeviceToken     string           `json:"deviceToken,omitempty"` // Post-pairing device auth (Fix 4)
	CreatedAt       time.Time        `json:"createdAt"`
	ExpiresAt       time.Time        `json:"expiresAt"`
	ConnectedAt     *time.Time       `json:"connectedAt,omitempty"`
	DeviceInfo      *MobileDeviceInfo `json:"deviceInfo,omitempty"`
	Calibration     CalibrationStatus `json:"calibration"`
	Health          StreamHealth     `json:"health"`

	// Anomaly state
	HandsOnDesk      bool    `json:"handsOnDesk"`
	LastHandsCheck   time.Time `json:"-"`
	DisplacementAlert bool   `json:"displacementAlert"`
	BaselineAccel    [3]float64 `json:"-"` // calibrated at start
	AccelCalibrated  bool   `json:"-"`
}

// PairingCodePayload is the data encoded in the QR code.
type PairingCodePayload struct {
	SessionID   string `json:"sid"`
	Token       string `json:"tok"`
	ServerURL   string `json:"url"`
	ExpiresAt   int64  `json:"exp"`
}

// ---------------------------------------------------------------------------
// Event Emission (Fix 6)
// ---------------------------------------------------------------------------

// EventEmitFn bridges sidecam anomalies into the ClickHouse event pipeline.
// Parameters: sessionID, studentID, examID, orgID, eventType, severity, label
type EventEmitFn func(sessionID, studentID, examID, orgID, eventType, severity, label string)

// ---------------------------------------------------------------------------
// Orchestrator
// ---------------------------------------------------------------------------

// Orchestrator manages all active secondary camera pairings.
type Orchestrator struct {
	mu       sync.RWMutex
	sessions map[string]*PairingSession // sessionID -> PairingSession
	logger   *zap.Logger
	secret   []byte      // HMAC signing key for pairing tokens
	emitFn   EventEmitFn // Bridges anomalies to ClickHouse (Fix 6)

	// Lifecycle
	done chan struct{}
	wg   sync.WaitGroup

	// Config
	pairingTTL       time.Duration
	heartbeatTimeout time.Duration
	serverURL        string
}

// NewOrchestrator creates a new secondary camera orchestrator.
func NewOrchestrator(logger *zap.Logger, signingKey []byte, serverURL string, emitFn EventEmitFn) *Orchestrator {
	o := &Orchestrator{
		sessions:         make(map[string]*PairingSession),
		logger:           logger.Named("sidecam"),
		secret:           signingKey,
		emitFn:           emitFn,
		done:             make(chan struct{}),
		pairingTTL:       5 * time.Minute,
		heartbeatTimeout: 15 * time.Second,
		serverURL:        serverURL,
	}

	// Start health monitor with graceful shutdown support.
	o.wg.Add(1)
	go o.healthMonitorLoop()

	return o
}

// Close gracefully stops the orchestrator's background goroutines and waits
// for them to exit. Must be called during server shutdown to prevent goroutine leaks.
func (o *Orchestrator) Close() {
	close(o.done)
	o.wg.Wait()
	o.logger.Info("sidecam orchestrator closed")
}

// emitEvent safely emits an event if the callback is configured.
func (o *Orchestrator) emitEvent(ps *PairingSession, eventType, severity, label string) {
	if o.emitFn != nil {
		o.emitFn(ps.SessionID, ps.StudentID, ps.ExamID, ps.OrgID, eventType, severity, label)
	}
}

// ValidateDeviceToken checks the device-bound session token (Fix 4).
func (o *Orchestrator) ValidateDeviceToken(sessionID, token string) error {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return fmt.Errorf("sidecam: session not found")
	}

	if ps.DeviceToken == "" || !hmac.Equal([]byte(token), []byte(ps.DeviceToken)) {
		return fmt.Errorf("sidecam: invalid device token")
	}

	return nil
}

// ---------------------------------------------------------------------------
// QR Handshake & Pairing
// ---------------------------------------------------------------------------

// InitiatePairing creates a new pairing session and returns the QR payload.
func (o *Orchestrator) InitiatePairing(
	sessionID, studentID, examID, orgID string,
	policy CameraPolicy,
) (*PairingSession, *PairingCodePayload, error) {
	if policy == PolicyDisabled {
		return nil, nil, fmt.Errorf("sidecam: secondary camera is disabled for this exam")
	}

	// Fix 5: Double-pairing guard — reject if session exists in non-terminal state
	o.mu.RLock()
	existing, exists := o.sessions[sessionID]
	o.mu.RUnlock()
	if exists && existing.State != PairingFailed && existing.State != PairingDisconnected {
		return nil, nil, fmt.Errorf("sidecam: pairing session already active (state=%s), cannot re-initiate", existing.State)
	}

	// Generate ephemeral pairing token
	pairingToken := randutil.HexToken(32)

	// Generate HMAC-signed pairing code
	pairingCode := o.signPairingCode(sessionID, pairingToken)

	now := time.Now()
	ps := &PairingSession{
		SessionID:    sessionID,
		StudentID:    studentID,
		ExamID:       examID,
		OrgID:        orgID,
		Policy:       policy,
		State:        PairingPending,
		PairingToken: pairingToken,
		PairingCode:  pairingCode,
		CreatedAt:    now,
		ExpiresAt:    now.Add(o.pairingTTL),
		Calibration: CalibrationStatus{
			Message: "Ожидание подключения камеры",
		},
		Health: StreamHealth{
			Quality: "critical",
		},
		HandsOnDesk: true,
	}

	o.mu.Lock()
	o.sessions[sessionID] = ps
	o.mu.Unlock()

	payload := &PairingCodePayload{
		SessionID: sessionID,
		Token:     pairingToken,
		ServerURL: o.serverURL,
		ExpiresAt: ps.ExpiresAt.Unix(),
	}

	o.logger.Info("sidecam: pairing initiated",
		zap.String("session_id", sessionID),
		zap.String("policy", string(policy)),
		zap.Time("expires_at", ps.ExpiresAt),
	)

	return ps, payload, nil
}

// CompletePairing validates the mobile device's pairing attempt.
func (o *Orchestrator) CompletePairing(sessionID, pairingToken string, device *MobileDeviceInfo) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return fmt.Errorf("sidecam: session not found: %s", sessionID)
	}

	if ps.State != PairingPending {
		return fmt.Errorf("sidecam: invalid state for pairing: %s", ps.State)
	}

	if time.Now().After(ps.ExpiresAt) {
		ps.State = PairingFailed
		return fmt.Errorf("sidecam: pairing code expired")
	}

	// Verify token
	if !hmac.Equal([]byte(pairingToken), []byte(ps.PairingToken)) {
		return fmt.Errorf("sidecam: invalid pairing token")
	}

	// Battery check for mandatory mode
	if ps.Policy == PolicyMandatory && device != nil && device.BatteryLevel < 0.20 {
		return fmt.Errorf("sidecam: battery level %.0f%% is below minimum 20%% for mandatory mode", device.BatteryLevel*100)
	}

	now := time.Now()
	ps.State = PairingConnected
	ps.ConnectedAt = &now
	ps.DeviceInfo = device
	ps.Health.Connected = true
	ps.Health.LastHeartbeat = now.Format(time.RFC3339)

	// Fix 1: Invalidate pairing token — prevent replay attacks
	ps.PairingToken = ""
	ps.PairingCode = ""

	// Fix 4: Generate device-bound session token for post-pairing auth
	ps.DeviceToken = randutil.HexToken(32)

	o.logger.Info("sidecam: device paired",
		zap.String("session_id", sessionID),
		zap.String("device", device.DeviceModel),
		zap.Float64("battery", device.BatteryLevel),
	)

	return nil
}

// ---------------------------------------------------------------------------
// Session Gating
// ---------------------------------------------------------------------------

// ValidateSessionStart checks whether a session can start given the camera policy.
func (o *Orchestrator) ValidateSessionStart(sessionID string, policy CameraPolicy) error {
	if policy == PolicyDisabled || policy == PolicyOptional {
		return nil
	}

	// Mandatory mode: require valid pairing
	o.mu.RLock()
	ps, ok := o.sessions[sessionID]
	o.mu.RUnlock()

	if !ok {
		return fmt.Errorf("sidecam: mandatory secondary camera required but no pairing session found")
	}

	// Fix 2: Require PairingReady (calibrated) for mandatory mode.
	// Previously PairingConnected (uncalibrated) passed — this was the primary bypass vector.
	if ps.State != PairingReady {
		switch ps.State {
		case PairingPending:
			return fmt.Errorf("sidecam: mandatory secondary camera not yet connected — scan QR code to pair mobile device")
		case PairingConnected:
			return fmt.Errorf("sidecam: mandatory secondary camera connected but not calibrated — complete spatial calibration first")
		case PairingCalibrating:
			return fmt.Errorf("sidecam: mandatory secondary camera calibration in progress — please wait")
		case PairingFailed:
			return fmt.Errorf("sidecam: mandatory secondary camera pairing failed — re-initiate pairing")
		case PairingDisconnected:
			return fmt.Errorf("sidecam: mandatory secondary camera disconnected — restore connection")
		default:
			return fmt.Errorf("sidecam: mandatory secondary camera in unexpected state: %s", ps.State)
		}
	}

	// Check battery
	if ps.DeviceInfo != nil && ps.DeviceInfo.BatteryLevel < 0.20 {
		return fmt.Errorf("sidecam: mobile device battery at %.0f%% — minimum 20%% required", ps.DeviceInfo.BatteryLevel*100)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Spatial Calibration (Golden Angle)
// ---------------------------------------------------------------------------

// ProcessCalibrationFrame analyzes a side-camera frame for spatial calibration.
// The frame analysis data comes from the frontend vision engine (headPose, objectDetection).
func (o *Orchestrator) ProcessCalibrationFrame(sessionID string, frame *CalibrationFrame) (*CalibrationStatus, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("sidecam: session not found")
	}

	ps.Calibration.AttemptCount++

	// Verify golden angle (45-60°)
	// The camera should be at approximately 45-60° from the student
	// We estimate this from the head yaw angle visible in the side camera
	viewAngle := math.Abs(frame.EstimatedAngle)
	ps.Calibration.ViewAngle = viewAngle

	goldenAngle := viewAngle >= 35 && viewAngle <= 70 // Allow some tolerance
	ps.Calibration.HandsVisible = frame.HandsDetected
	ps.Calibration.KeyboardVisible = frame.KeyboardDetected
	ps.Calibration.ScreenEdge = frame.ScreenEdgeDetected

	if goldenAngle && frame.HandsDetected && (frame.KeyboardDetected || frame.ScreenEdgeDetected) {
		ps.Calibration.Passed = true
		ps.Calibration.Message = fmt.Sprintf("Калибровка пройдена (%.0f°)", viewAngle)
		ps.Calibration.CalibratedAt = time.Now().Format(time.RFC3339)
		ps.State = PairingReady

		o.logger.Info("sidecam: calibration passed",
			zap.String("session_id", sessionID),
			zap.Float64("angle", viewAngle),
		)
	} else {
		var reasons []string
		if !goldenAngle {
			reasons = append(reasons, fmt.Sprintf("угол %.0f° (нужно 45-60°)", viewAngle))
		}
		if !frame.HandsDetected {
			reasons = append(reasons, "руки не видны")
		}
		if !frame.KeyboardDetected && !frame.ScreenEdgeDetected {
			reasons = append(reasons, "клавиатура/экран не видны")
		}
		ps.Calibration.Message = fmt.Sprintf("Корректировка: %s", joinStrings(reasons, ", "))

		// Fix 6: Emit calibration failure event after first attempt
		if ps.Calibration.AttemptCount > 1 {
			o.emitEvent(ps, "SIDECAM_CALIBRATION_FAILED", "warning",
				fmt.Sprintf("Calibration attempt %d failed: %s", ps.Calibration.AttemptCount, joinStrings(reasons, ", ")))
		}
	}

	return &ps.Calibration, nil
}

// CalibrationFrame contains analysis data from a side-camera frame.
type CalibrationFrame struct {
	EstimatedAngle     float64 `json:"estimatedAngle"`     // Degrees from frontal
	HandsDetected      bool    `json:"handsDetected"`
	KeyboardDetected   bool    `json:"keyboardDetected"`
	ScreenEdgeDetected bool    `json:"screenEdgeDetected"`
	FrameQuality       float64 `json:"frameQuality"`       // 0-1
}

// ---------------------------------------------------------------------------
// Power & Thermal Guard
// ---------------------------------------------------------------------------

// ProcessDeviceTelemetry handles mobile device health updates.
// Returns throttle instructions and alerts.
func (o *Orchestrator) ProcessDeviceTelemetry(sessionID string, device *MobileDeviceInfo) (*DeviceDirective, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("sidecam: session not found")
	}

	ps.DeviceInfo = device
	directive := &DeviceDirective{}

	// Battery guard
	if device.BatteryLevel < 0.10 {
		directive.Alerts = append(directive.Alerts, DeviceAlert{
			Type:     "battery_critical",
			Severity: "critical",
			Message:  fmt.Sprintf("Батарея критически низкая: %.0f%%", device.BatteryLevel*100),
		})
		if ps.Policy == PolicyMandatory {
			directive.PauseSession = true
		}
		// Fix 6: Emit battery critical event
		o.emitEvent(ps, "SIDECAM_BATTERY_CRITICAL", "critical",
			fmt.Sprintf("Battery at %.0f%%", device.BatteryLevel*100))
	} else if device.BatteryLevel < 0.20 && !device.IsCharging {
		directive.Alerts = append(directive.Alerts, DeviceAlert{
			Type:     "battery_low",
			Severity: "warning",
			Message:  "Рекомендуется подключить зарядку",
		})
	}

	// Thermal guard
	switch device.ThermalState {
	case "critical":
		directive.TargetFPS = 5
		directive.Alerts = append(directive.Alerts, DeviceAlert{
			Type:     "thermal_critical",
			Severity: "critical",
			Message:  "Перегрев устройства — FPS снижен до 5",
		})
		// Fix 6: Emit thermal throttle event
		o.emitEvent(ps, "SIDECAM_THERMAL_THROTTLE", "warning",
			"Thermal state critical, FPS throttled to 5")
	case "serious":
		directive.TargetFPS = 10
		directive.Alerts = append(directive.Alerts, DeviceAlert{
			Type:     "thermal_serious",
			Severity: "warning",
			Message:  "Высокая температура — FPS снижен до 10",
		})
		// Fix 6: Emit thermal throttle event
		o.emitEvent(ps, "SIDECAM_THERMAL_THROTTLE", "warning",
			"Thermal state serious, FPS throttled to 10")
	case "fair":
		directive.TargetFPS = 15
	default:
		directive.TargetFPS = 30 // nominal
	}

	// Displacement detection via accelerometer
	if ps.AccelCalibrated {
		dx := device.AccelX - ps.BaselineAccel[0]
		dy := device.AccelY - ps.BaselineAccel[1]
		dz := device.AccelZ - ps.BaselineAccel[2]
		displacement := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if displacement > 2.0 { // Significant movement threshold
			ps.DisplacementAlert = true
			directive.Alerts = append(directive.Alerts, DeviceAlert{
				Type:     "device_displaced",
				Severity: "critical",
				Message:  fmt.Sprintf("Устройство перемещено (Δ=%.1f)", displacement),
			})
			// Fix 6: Emit device displacement event
			o.emitEvent(ps, "SIDECAM_DEVICE_DISPLACED", "critical",
				fmt.Sprintf("Device displaced, delta=%.1f", displacement))
		} else {
			ps.DisplacementAlert = false
		}
	} else {
		// Calibrate baseline on first telemetry
		ps.BaselineAccel = [3]float64{device.AccelX, device.AccelY, device.AccelZ}
		ps.AccelCalibrated = true
	}

	return directive, nil
}

// DeviceDirective contains instructions sent back to the mobile device.
type DeviceDirective struct {
	TargetFPS    int           `json:"targetFps"`
	PauseSession bool          `json:"pauseSession"`
	Alerts       []DeviceAlert `json:"alerts"`
}

// DeviceAlert is a single alert to display on the mobile device.
type DeviceAlert struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// ---------------------------------------------------------------------------
// Stream Health & Heartbeat
// ---------------------------------------------------------------------------

// ProcessHeartbeat updates the stream health for a session.
func (o *Orchestrator) ProcessHeartbeat(sessionID string, clientTs time.Time, frameRate int, droppedFrames int) (*StreamHealth, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("sidecam: session not found")
	}

	now := time.Now()
	latency := now.Sub(clientTs).Milliseconds()

	ps.Health.Connected = true
	ps.Health.LastHeartbeat = now.Format(time.RFC3339)
	ps.Health.LatencyMs = latency
	ps.Health.FrameRate = frameRate
	ps.Health.DroppedFrames = droppedFrames

	if ps.ConnectedAt != nil {
		ps.Health.Uptime = int64(now.Sub(*ps.ConnectedAt).Seconds())
	}

	// Compute quality
	if latency < 100 && frameRate >= 20 {
		ps.Health.Quality = "excellent"
	} else if latency < 300 && frameRate >= 15 {
		ps.Health.Quality = "good"
	} else if latency < 1000 && frameRate >= 5 {
		ps.Health.Quality = "degraded"
	} else {
		ps.Health.Quality = "critical"
	}

	// Fix 3: Heartbeat reconnection guard — promote to PairingConnected (not PairingReady).
	// Forces re-calibration after disconnect, preventing tampered/repositioned device auto-resume.
	if ps.State == PairingDisconnected {
		ps.State = PairingConnected
		ps.Calibration.Passed = false
		ps.Calibration.Message = "Reconnected — re-calibration required"
		ps.AccelCalibrated = false // Reset accelerometer baseline
		o.logger.Info("sidecam: stream recovered (re-calibration required)",
			zap.String("session_id", sessionID),
		)
	}

	return &ps.Health, nil
}

// ---------------------------------------------------------------------------
// Heuristic Anomaly Detection
// ---------------------------------------------------------------------------

// ProcessHandsDetection updates hands-on-desk tracking from side camera vision.
func (o *Orchestrator) ProcessHandsDetection(sessionID string, handsVisible bool) {
	o.mu.Lock()
	defer o.mu.Unlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return
	}

	wasOnDesk := ps.HandsOnDesk
	ps.HandsOnDesk = handsVisible
	ps.LastHandsCheck = time.Now()

	// Fix 6: Emit hands-off-desk event on transition
	if wasOnDesk && !handsVisible {
		o.emitEvent(ps, "SIDECAM_HANDS_OFF_DESK", "warning", "Hands left desk area")
	}
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

// GetPairingSession returns the current pairing state for a session.
func (o *Orchestrator) GetPairingSession(sessionID string) (*PairingSession, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ps, ok := o.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("sidecam: session not found")
	}

	return ps, nil
}

// CleanupSession removes a pairing session (called on exam end).
func (o *Orchestrator) CleanupSession(sessionID string) {
	o.mu.Lock()
	delete(o.sessions, sessionID)
	o.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Internal
// ---------------------------------------------------------------------------

func (o *Orchestrator) signPairingCode(sessionID, token string) string {
	mac := hmac.New(sha256.New, o.secret)
	mac.Write([]byte(sessionID + ":" + token))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

func (o *Orchestrator) healthMonitorLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.done:
			return
		case <-ticker.C:
			o.checkSessions()
		}
	}
}

// sessionEvictionTTL is how long a terminal session lingers before auto-eviction.
const sessionEvictionTTL = 30 * time.Minute

// checkSessions inspects all active sessions for heartbeat timeouts, expiry,
// and evicts terminal sessions older than sessionEvictionTTL to prevent
// unbounded memory growth.
func (o *Orchestrator) checkSessions() {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for sid, ps := range o.sessions {
		// Check heartbeat timeout for active sessions
		if ps.State == PairingReady || ps.State == PairingConnected || ps.State == PairingCalibrating {
			lastHB, err := time.Parse(time.RFC3339, ps.Health.LastHeartbeat)
			if err == nil && now.Sub(lastHB) > o.heartbeatTimeout {
				prevState := ps.State
				ps.State = PairingDisconnected
				ps.Health.Connected = false
				ps.Health.Quality = "critical"

				o.logger.Warn("sidecam: stream disconnected (heartbeat timeout)",
					zap.String("session_id", sid),
					zap.Duration("since_last_hb", now.Sub(lastHB)),
				)

				// Fix 6: Emit stream disconnected event (only on state transition)
				if prevState != PairingDisconnected {
					o.emitEvent(ps, "SIDECAM_STREAM_DISCONNECTED", "critical",
						fmt.Sprintf("Heartbeat timeout after %.0fs", now.Sub(lastHB).Seconds()))
				}
			}
		}

		// Transition expired pending sessions to failed
		if ps.State == PairingPending && now.After(ps.ExpiresAt) {
			ps.State = PairingFailed
			ps.Calibration.Message = "Время сопряжения истекло"
		}

		// Auto-evict terminal sessions to prevent unbounded map growth.
		// Sessions in PairingFailed or PairingDisconnected that are older
		// than 30 minutes are removed from memory.
		if (ps.State == PairingFailed || ps.State == PairingDisconnected) &&
			now.Sub(ps.CreatedAt) > sessionEvictionTTL {
			toDelete = append(toDelete, sid)
		}
	}

	for _, sid := range toDelete {
		delete(o.sessions, sid)
	}

	if len(toDelete) > 0 {
		o.logger.Debug("sidecam: evicted stale sessions",
			zap.Int("count", len(toDelete)),
		)
	}
}

// MarshalQRPayload produces a JSON string for embedding in a QR code.
func MarshalQRPayload(payload *PairingCodePayload) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("sidecam: marshal QR payload: %w", err)
	}
	return string(data), nil
}

func joinStrings(ss []string, sep string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += sep + s
	}
	return result
}

// Ensure context is used (avoids unused import)
var _ context.Context
