// Package entity — ExamProctoringSettings domain entity.
//
// Mirrors the frontend ExamProctoringSettings interface (exams.vue) and
// provides the backend representation for per-exam, per-org proctoring
// rule configuration. Used by the forensic scorer to dynamically filter
// penalty rules and adjust thresholds at runtime.
package entity

import "time"

// ExamProctoringSettings stores the complete proctoring configuration for an exam.
// Each boolean toggle maps to one or more penalty rules in the forensic scorer
// via the ToggleToRuleMap. Slider values adjust penalty weights dynamically.
type ExamProctoringSettings struct {
	OrgID  string `json:"orgId"`
	ExamID string `json:"examId"`

	// -----------------------------------------------------------------------
	// Identity & Anti-Fraud
	// -----------------------------------------------------------------------
	RequireSideCamera     bool `json:"requireSideCamera"`
	FaceVerification      bool `json:"faceVerification"`
	DynamicFaceRecheck    bool `json:"dynamicFaceRecheck"`
	AntiSpoofing          bool `json:"antiSpoofing"`
	RoomScan360           bool `json:"roomScan360"`
	ObjectDetectionPhone  bool `json:"objectDetectionPhone"`
	ObjectDetectionPerson bool `json:"objectDetectionPerson"`

	// -----------------------------------------------------------------------
	// AI Sensitivity
	// -----------------------------------------------------------------------
	GazeTracking            bool    `json:"gazeTracking"`
	GazeSensitivity         float64 `json:"gazeSensitivity"`         // 10-95 (%)
	GazeDeviationLimitSec   float64 `json:"gazeDeviationLimitSec"`   // 3-30 (seconds)
	VoiceDetectionThreshold float64 `json:"voiceDetectionThreshold"` // 0-100 (%)
	VoiceActivityDetection  bool    `json:"voiceActivityDetection"`
	AudioPeripheryDetection bool    `json:"audioPeripheryDetection"`
	SmartNoiseFilter        bool    `json:"smartNoiseFilter"`

	// -----------------------------------------------------------------------
	// Psychometrics & AI Analytics
	// -----------------------------------------------------------------------
	EmotionStressAnalysis bool `json:"emotionStressAnalysis"`
	FocusLossScore        bool `json:"focusLossScore"`
	BlinkPatternAnalysis  bool `json:"blinkPatternAnalysis"`

	// -----------------------------------------------------------------------
	// Browser Restrictions
	// -----------------------------------------------------------------------
	ForceFullscreen         bool `json:"forceFullscreen"`
	FullscreenExitDetection bool `json:"fullscreenExitDetection"`
	WebDisplayMonitoring    bool `json:"webDisplayMonitoring"`
	TabSwitchingLimit       int  `json:"tabSwitchingLimit"` // 0 = prohibited, 1-10 = limit
	BlockCopyPaste          bool `json:"blockCopyPaste"`
	BlockPrintScreen        bool `json:"blockPrintScreen"`
	BlockVirtualMachine     bool `json:"blockVirtualMachine"`
	BlockMultiDesktop       bool `json:"blockMultiDesktop"`
	BlockRemoteAccess       bool `json:"blockRemoteAccess"`
	BlockContextMenu        bool `json:"blockContextMenu"`

	// -----------------------------------------------------------------------
	// Network
	// -----------------------------------------------------------------------
	VPNProxyDetection bool `json:"vpnProxyDetection"`
	LocalNetworkScan  bool `json:"localNetworkScan"`

	// -----------------------------------------------------------------------
	// Advanced Security (Kernel-Level)
	// -----------------------------------------------------------------------
	TypingDynamics          bool `json:"typingDynamics"`
	HandCursorSync          bool `json:"handCursorSync"`
	ProcessScanning         bool `json:"processScanning"`
	HardwareDeviceDetection bool `json:"hardwareDeviceDetection"`
	AdvancedRemoteBlock     bool `json:"advancedRemoteBlock"`
	HardwareIdBinding       bool `json:"hardwareIdBinding"`
	DeepMultiMonitorCheck   bool `json:"deepMultiMonitorCheck"`
	ForceLowSpecMode        bool `json:"forceLowSpecMode"`

	// -----------------------------------------------------------------------
	// Verdict Thresholds (configurable per-exam)
	// -----------------------------------------------------------------------
	CleanThreshold   float64 `json:"cleanThreshold"`   // default 80
	WarningThreshold float64 `json:"warningThreshold"`  // default 50

	// -----------------------------------------------------------------------
	// Auto-Terminate
	// -----------------------------------------------------------------------
	AutoTerminate   bool    `json:"autoTerminate"`   // if true, auto-terminate on fraud
	AutoTerminateAt float64 `json:"autoTerminateAt"` // score threshold (default 30)

	// -----------------------------------------------------------------------
	// Metadata
	// -----------------------------------------------------------------------
	UpdatedAt time.Time `json:"updatedAt"`
	UpdatedBy string    `json:"updatedBy"`
}

// DefaultExamProctoringSettings returns a settings object with all toggles
// enabled and sliders at their standard defaults. This matches the "Standard"
// preset from the frontend.
func DefaultExamProctoringSettings() *ExamProctoringSettings {
	return &ExamProctoringSettings{
		// Identity & Anti-Fraud
		RequireSideCamera:     false,
		FaceVerification:      true,
		DynamicFaceRecheck:    true,
		AntiSpoofing:          true,
		RoomScan360:           false,
		ObjectDetectionPhone:  true,
		ObjectDetectionPerson: true,

		// AI Sensitivity
		GazeTracking:            true,
		GazeSensitivity:         60,
		GazeDeviationLimitSec:   10,
		VoiceDetectionThreshold: 50,
		VoiceActivityDetection:  true,
		AudioPeripheryDetection: true,
		SmartNoiseFilter:        true,

		// Psychometrics
		EmotionStressAnalysis: false,
		FocusLossScore:        true,
		BlinkPatternAnalysis:  false,

		// Browser Restrictions
		ForceFullscreen:         true,
		FullscreenExitDetection: true,
		WebDisplayMonitoring:    true,
		TabSwitchingLimit:       3,
		BlockCopyPaste:          true,
		BlockPrintScreen:        true,
		BlockVirtualMachine:     true,
		BlockMultiDesktop:       true,
		BlockRemoteAccess:       true,
		BlockContextMenu:        true,

		// Network
		VPNProxyDetection: false,
		LocalNetworkScan:  false,

		// Advanced Security
		TypingDynamics:          false,
		HandCursorSync:          false,
		ProcessScanning:         true,
		HardwareDeviceDetection: false,
		AdvancedRemoteBlock:     false,
		HardwareIdBinding:       true,
		DeepMultiMonitorCheck:   false,
		ForceLowSpecMode:        false,

		// Verdict Thresholds
		CleanThreshold:   80,
		WarningThreshold: 50,

		// Auto-Terminate
		AutoTerminate:   false,
		AutoTerminateAt: 30,
	}
}
