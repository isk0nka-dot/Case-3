// =============================================================================
// Argus AI — Hard-Fail Toggle Validator
// =============================================================================
//
// Ensures every boolean toggle in ExamProctoringSettings has a corresponding
// entry in ToggleToRuleMap. This prevents frontend toggles from being silently
// ignored by the backend scorer.
//
// If this test fails, it means a frontend toggle was added to the entity without
// updating the ToggleToRuleMap in scorer.go. The build MUST break.
//
// go test ./internal/infrastructure/forensic/... -run TestAllFrontendToggles -v
// =============================================================================
package forensic

import (
	"math"
	"reflect"
	"testing"

	"github.com/argus-ai/event-collector/internal/domain/entity"
)

// floatEq checks if two floats are approximately equal (within epsilon).
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

// TestAllFrontendTogglesHaveBackendMapping verifies that every boolean/toggle
// field in ExamProctoringSettings has a corresponding entry in ToggleToRuleMap.
// The entry may have an empty slice (for future toggles), but MUST exist.
func TestAllFrontendTogglesHaveBackendMapping(t *testing.T) {
	// Fields to skip — metadata fields that are NOT toggles.
	skipFields := map[string]bool{
		"OrgID":            true,
		"ExamID":           true,
		"CleanThreshold":   true,
		"WarningThreshold": true,
		"AutoTerminate":    true,
		"AutoTerminateAt":  true,
		"UpdatedAt":        true,
		"UpdatedBy":        true,
		// Slider fields — NOT boolean toggles, but used as modifiers.
		// They affect rule weights, not enable/disable.
		"GazeSensitivity":         true,
		"GazeDeviationLimitSec":   true,
		"VoiceDetectionThreshold": true,
		"TabSwitchingLimit":       true,
	}

	settingsType := reflect.TypeOf(entity.ExamProctoringSettings{})
	missing := []string{}

	for i := 0; i < settingsType.NumField(); i++ {
		field := settingsType.Field(i)

		// Skip non-toggle fields.
		if skipFields[field.Name] {
			continue
		}

		// Only check boolean fields (the actual on/off toggles).
		if field.Type.Kind() != reflect.Bool {
			continue
		}

		// Get the JSON tag name (this is what ToggleToRuleMap uses as keys).
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// Check if the toggle exists in ToggleToRuleMap.
		if _, exists := ToggleToRuleMap[jsonTag]; !exists {
			missing = append(missing, jsonTag)
		}
	}

	if len(missing) > 0 {
		t.Errorf("HARD-FAIL: %d frontend toggle(s) have NO entry in ToggleToRuleMap:\n", len(missing))
		for _, m := range missing {
			t.Errorf("  - %q (add to ToggleToRuleMap in scorer.go, even if empty []string{})", m)
		}
		t.FailNow()
	}

	t.Logf("OK: All %d boolean toggle fields in ExamProctoringSettings are mapped in ToggleToRuleMap", settingsType.NumField()-len(skipFields))
}

// TestToggleToRuleMapKeysAreValidJSONTags verifies that every key in
// ToggleToRuleMap corresponds to an actual JSON tag in ExamProctoringSettings.
// This catches stale/misspelled keys.
func TestToggleToRuleMapKeysAreValidJSONTags(t *testing.T) {
	validTags := map[string]bool{}

	settingsType := reflect.TypeOf(entity.ExamProctoringSettings{})
	for i := 0; i < settingsType.NumField(); i++ {
		jsonTag := settingsType.Field(i).Tag.Get("json")
		if jsonTag != "" && jsonTag != "-" {
			validTags[jsonTag] = true
		}
	}

	stale := []string{}
	for key := range ToggleToRuleMap {
		if !validTags[key] {
			stale = append(stale, key)
		}
	}

	if len(stale) > 0 {
		t.Errorf("HARD-FAIL: %d ToggleToRuleMap key(s) do NOT match any JSON tag in ExamProctoringSettings:\n", len(stale))
		for _, s := range stale {
			t.Errorf("  - %q (remove from ToggleToRuleMap or fix spelling)", s)
		}
		t.FailNow()
	}

	t.Logf("OK: All %d ToggleToRuleMap keys are valid JSON tags", len(ToggleToRuleMap))
}

// TestGetDefaultPenaltyRulesNotEmpty ensures the rule registry is populated.
func TestGetDefaultPenaltyRulesNotEmpty(t *testing.T) {
	rules := GetDefaultPenaltyRules()
	if len(rules) == 0 {
		t.Fatal("GetDefaultPenaltyRules() returned 0 rules — scorer has no penalty rules")
	}
	t.Logf("OK: %d penalty rules in default registry", len(rules))

	// Verify each rule has required fields.
	for i, r := range rules {
		if r.EventType == "" {
			t.Errorf("Rule %d has empty EventType", i)
		}
		if r.PenaltyPer <= 0 {
			t.Errorf("Rule %d (%s) has non-positive PenaltyPer: %.2f", i, r.EventType, r.PenaltyPer)
		}
		// MaxPenalty = 0 is valid — it means "unlimited" (no cap on penalty).
		if r.MaxPenalty < 0 {
			t.Errorf("Rule %d (%s) has negative MaxPenalty: %.2f", i, r.EventType, r.MaxPenalty)
		}
		if r.Description == "" {
			t.Errorf("Rule %d (%s) has empty Description", i, r.EventType)
		}
	}
}

// TestComputeVerdictWithThresholds verifies verdict classification with custom thresholds.
func TestComputeVerdictWithThresholds(t *testing.T) {
	tests := []struct {
		score     float64
		clean     float64
		warning   float64
		want      string
		wantLabel string
	}{
		{score: 95, clean: 80, warning: 50, want: "clean"},
		{score: 80, clean: 80, warning: 50, want: "clean"},
		{score: 79.9, clean: 80, warning: 50, want: "warning"},
		{score: 50, clean: 80, warning: 50, want: "warning"},
		{score: 49.9, clean: 80, warning: 50, want: "fraud"},
		{score: 0, clean: 80, warning: 50, want: "fraud"},

		// Custom thresholds
		{score: 92, clean: 95, warning: 70, want: "warning"},
		{score: 65, clean: 95, warning: 70, want: "fraud"},
		{score: 96, clean: 95, warning: 70, want: "clean"},
	}

	for _, tt := range tests {
		verdict, label := computeVerdictWithThresholds(tt.score, tt.clean, tt.warning)
		if verdict != tt.want {
			t.Errorf("computeVerdictWithThresholds(%.1f, %.0f, %.0f) = %q, want %q",
				tt.score, tt.clean, tt.warning, verdict, tt.want)
		}
		if label == "" {
			t.Errorf("computeVerdictWithThresholds(%.1f, %.0f, %.0f) returned empty label",
				tt.score, tt.clean, tt.warning)
		}
		_ = label // Used for coverage
	}
}

// TestBuildDisabledSetFiltersCorrectly verifies that disabled toggles
// produce the correct set of disabled event types.
func TestBuildDisabledSetFiltersCorrectly(t *testing.T) {
	// All toggles ON — nothing disabled
	allOn := entity.DefaultExamProctoringSettings()
	// Force all toggles ON for this test
	allOn.RequireSideCamera = true
	allOn.RoomScan360 = true
	allOn.EmotionStressAnalysis = true
	allOn.BlinkPatternAnalysis = true
	allOn.FocusLossScore = true
	allOn.WebDisplayMonitoring = true

	disabled := buildDisabledSet(allOn)
	if len(disabled) != 0 {
		t.Errorf("All toggles ON: expected 0 disabled event types, got %d: %v", len(disabled), disabled)
	}

	// Turn off gazeTracking — GAZE_DEVIATION should be disabled
	cfg := entity.DefaultExamProctoringSettings()
	cfg.GazeTracking = false
	disabled = buildDisabledSet(cfg)
	if !disabled["GAZE_DEVIATION"] {
		t.Error("GazeTracking=false should disable GAZE_DEVIATION")
	}

	// Turn off faceVerification — FACE_MISMATCH and FACE_NOT_DETECTED should be disabled
	cfg2 := entity.DefaultExamProctoringSettings()
	cfg2.FaceVerification = false
	disabled2 := buildDisabledSet(cfg2)
	if !disabled2["FACE_MISMATCH"] {
		t.Error("FaceVerification=false should disable FACE_MISMATCH")
	}
	if !disabled2["FACE_NOT_DETECTED"] {
		t.Error("FaceVerification=false should disable FACE_NOT_DETECTED")
	}

	// RequireSideCamera=false in defaults — all 6 SIDECAM_* types should be disabled
	cfg3 := entity.DefaultExamProctoringSettings()
	// RequireSideCamera is already false in defaults
	disabled3 := buildDisabledSet(cfg3)
	sidecamTypes := []string{
		"SIDECAM_DEVICE_DISPLACED", "SIDECAM_HANDS_OFF_DESK",
		"SIDECAM_BATTERY_CRITICAL", "SIDECAM_STREAM_DISCONNECTED",
		"SIDECAM_CALIBRATION_FAILED", "SIDECAM_THERMAL_THROTTLE",
	}
	for _, st := range sidecamTypes {
		if !disabled3[st] {
			t.Errorf("RequireSideCamera=false should disable %s", st)
		}
	}

	// Enable RequireSideCamera — SIDECAM events should NOT be disabled
	cfg4 := entity.DefaultExamProctoringSettings()
	cfg4.RequireSideCamera = true
	disabled4 := buildDisabledSet(cfg4)
	for _, st := range sidecamTypes {
		if disabled4[st] {
			t.Errorf("RequireSideCamera=true should NOT disable %s", st)
		}
	}
}

// TestBuildEffectiveRulesSliderAdjustments verifies slider-based weight modifications.
func TestBuildEffectiveRulesSliderAdjustments(t *testing.T) {
	// GAZE_DEVIATION default penaltyPer = 2.0, maxPenalty = 20.0

	cfg := entity.DefaultExamProctoringSettings()
	cfg.GazeSensitivity = 30 // Below default 60, should reduce penalty

	rules := buildEffectiveRules(cfg)

	// Find GAZE_DEVIATION rule
	var gazeRule *penaltyRule
	for i := range rules {
		if rules[i].eventType == "GAZE_DEVIATION" {
			gazeRule = &rules[i]
			break
		}
	}

	if gazeRule == nil {
		t.Fatal("GAZE_DEVIATION rule not found in effective rules")
	}

	// sensitivity 30/60 = 0.5x → penaltyPer = 2.0 * 0.5 = 1.0
	expectedPenalty := 2.0 * (30.0 / 60.0)
	if !floatEq(gazeRule.penaltyPer, expectedPenalty) {
		t.Errorf("GAZE_DEVIATION penaltyPer with gazeSensitivity=30: got %.4f, want %.4f",
			gazeRule.penaltyPer, expectedPenalty)
	}

	// High sensitivity should increase penalty
	cfg2 := entity.DefaultExamProctoringSettings()
	cfg2.GazeSensitivity = 90

	rules2 := buildEffectiveRules(cfg2)
	var gazeRule2 *penaltyRule
	for i := range rules2 {
		if rules2[i].eventType == "GAZE_DEVIATION" {
			gazeRule2 = &rules2[i]
			break
		}
	}

	if gazeRule2 == nil {
		t.Fatal("GAZE_DEVIATION rule not found with high sensitivity")
	}

	// sensitivity 90/60 = 1.5x → penaltyPer = 2.0 * 1.5 = 3.0
	expectedPenalty2 := 2.0 * (90.0 / 60.0)
	if !floatEq(gazeRule2.penaltyPer, expectedPenalty2) {
		t.Errorf("GAZE_DEVIATION penaltyPer with gazeSensitivity=90: got %.4f, want %.4f",
			gazeRule2.penaltyPer, expectedPenalty2)
	}

	// Test tab switching limit adjustment
	cfg3 := entity.DefaultExamProctoringSettings()
	cfg3.TabSwitchingLimit = 5

	rules3 := buildEffectiveRules(cfg3)
	var tabRule *penaltyRule
	for i := range rules3 {
		if rules3[i].eventType == "TAB_SWITCH" {
			tabRule = &rules3[i]
			break
		}
	}

	if tabRule == nil {
		t.Fatal("TAB_SWITCH rule not found in effective rules")
	}

	// tabSwitchingLimit=5, default penaltyPer=5 → maxPenalty = 5 * 5 = 25
	expectedMaxPenalty := float64(5) * 5.0
	if !floatEq(tabRule.maxPenalty, expectedMaxPenalty) {
		t.Errorf("TAB_SWITCH maxPenalty with tabSwitchingLimit=5: got %.4f, want %.4f",
			tabRule.maxPenalty, expectedMaxPenalty)
	}

	// Test voice detection threshold
	cfg4 := entity.DefaultExamProctoringSettings()
	cfg4.VoiceDetectionThreshold = 80

	rules4 := buildEffectiveRules(cfg4)
	var audioRule *penaltyRule
	for i := range rules4 {
		if rules4[i].eventType == "AUDIO_ANOMALY" {
			audioRule = &rules4[i]
			break
		}
	}

	if audioRule == nil {
		t.Fatal("AUDIO_ANOMALY rule not found in effective rules")
	}

	// threshold 80/50 = 1.6x → penaltyPer = 3.0 * 1.6 = 4.8
	expectedAudioPenalty := 3.0 * (80.0 / 50.0)
	if !floatEq(audioRule.penaltyPer, expectedAudioPenalty) {
		t.Errorf("AUDIO_ANOMALY penaltyPer with voiceDetectionThreshold=80: got %.4f, want %.4f",
			audioRule.penaltyPer, expectedAudioPenalty)
	}
}
