// =============================================================================
// Argus AI — useLowSpecMode Composable
// =============================================================================
//
// Detects and manages low-spec device mode for the proctoring frontend.
// When activated, disables CSS animations, transitions, backdrop-filter,
// WebGL canvases, and smooth scrolling to reduce CPU/GPU load.
//
// Toggle Logic:
//   - Manual: User toggle, persisted to localStorage('argus-low-spec-mode')
//   - Auto-detect: If Health Governor reports CPU > 80% for 30 consecutive
//     seconds, low-spec mode is automatically enabled
//   - Manual override takes precedence over auto-detect
//
// Implementation:
//   - Adds/removes 'low-spec-mode' class on <html> element
//   - CSS overrides in low-spec.css disable visual effects
//   - Composable exposes reactive state for UI binding
//
// Usage:
//   const lowSpec = useLowSpecMode()
//   lowSpec.toggle() // manual toggle
//   lowSpec.enable()  // force enable
//   lowSpec.disable() // force disable
//
// =============================================================================

import { ref, watch, computed, onMounted, type Ref, type ComputedRef } from 'vue'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STORAGE_KEY = 'argus-low-spec-mode'
const LOW_SPEC_CLASS = 'low-spec-mode'
const AUTO_DETECT_THRESHOLD = 0.8 // CPU pressure > 80%
const AUTO_DETECT_DURATION_MS = 30_000 // 30 seconds sustained

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Source of the low-spec mode activation. */
export type LowSpecSource = 'manual' | 'auto' | 'none'

/** Health metrics subset needed for auto-detection. */
export interface LowSpecHealthMetrics {
  cpuPressure: number // 0-1 range
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useLowSpecMode(healthMetrics?: Ref<LowSpecHealthMetrics>, forcePolicy?: Ref<boolean>) {
  // -------------------------------------------------------------------------
  // State
  // -------------------------------------------------------------------------

  const isEnabled: Ref<boolean> = ref(false)
  const source: Ref<LowSpecSource> = ref('none')
  const manualOverride: Ref<boolean | null> = ref(null) // null = no manual override

  // Internal auto-detect state
  let highCpuStartTime: number | null = null
  let autoDetectTimer: ReturnType<typeof setInterval> | null = null

  // -------------------------------------------------------------------------
  // DOM Manipulation
  // -------------------------------------------------------------------------

  /** Apply or remove the low-spec CSS class on <html>. */
  function applyToDOM(enabled: boolean): void {
    if (typeof document === 'undefined') return

    const html = document.documentElement
    if (enabled) {
      html.classList.add(LOW_SPEC_CLASS)
    } else {
      html.classList.remove(LOW_SPEC_CLASS)
    }
  }

  // -------------------------------------------------------------------------
  // Persistence
  // -------------------------------------------------------------------------

  /** Load manual preference from localStorage. */
  function loadPreference(): boolean | null {
    if (typeof localStorage === 'undefined') return null

    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored === 'true') return true
      if (stored === 'false') return false
      return null // no stored preference
    } catch {
      return null
    }
  }

  /** Save manual preference to localStorage. */
  function savePreference(value: boolean | null): void {
    if (typeof localStorage === 'undefined') return

    try {
      if (value === null) {
        localStorage.removeItem(STORAGE_KEY)
      } else {
        localStorage.setItem(STORAGE_KEY, String(value))
      }
    } catch {
      // localStorage may not be available
    }
  }

  // -------------------------------------------------------------------------
  // Auto-Detection
  // -------------------------------------------------------------------------

  /** Check CPU pressure and auto-enable if sustained high load. */
  function checkAutoDetect(): void {
    if (!healthMetrics) return
    if (manualOverride.value !== null) return // manual override takes precedence

    const cpu = healthMetrics.value.cpuPressure
    const now = Date.now()

    if (cpu > AUTO_DETECT_THRESHOLD) {
      if (highCpuStartTime === null) {
        highCpuStartTime = now
      } else if (now - highCpuStartTime >= AUTO_DETECT_DURATION_MS) {
        // Sustained high CPU — auto-enable
        if (!isEnabled.value || source.value !== 'auto') {
          isEnabled.value = true
          source.value = 'auto'
          applyToDOM(true)

          console.info('[argus:low-spec] Auto-enabled low-spec mode (CPU > 80% for 30s)', {
            cpuPressure: cpu,
            durationMs: now - highCpuStartTime
          })
        }
      }
    } else {
      // CPU recovered — if auto-enabled, auto-disable
      highCpuStartTime = null

      if (source.value === 'auto' && isEnabled.value) {
        isEnabled.value = false
        source.value = 'none'
        applyToDOM(false)

        console.info('[argus:low-spec] Auto-disabled low-spec mode (CPU recovered)')
      }
    }
  }

  /** Start the auto-detection polling loop. */
  function startAutoDetect(): void {
    if (autoDetectTimer || !healthMetrics) return

    // Check every 5 seconds.
    autoDetectTimer = setInterval(checkAutoDetect, 5000)
  }

  /** Stop the auto-detection polling loop. */
  function stopAutoDetect(): void {
    if (autoDetectTimer) {
      clearInterval(autoDetectTimer)
      autoDetectTimer = null
    }
    highCpuStartTime = null
  }

  // -------------------------------------------------------------------------
  // Public Actions
  // -------------------------------------------------------------------------

  /** Enable low-spec mode manually. */
  function enable(): void {
    manualOverride.value = true
    isEnabled.value = true
    source.value = 'manual'
    applyToDOM(true)
    savePreference(true)

    console.info('[argus:low-spec] Low-spec mode manually enabled')
  }

  /** Disable low-spec mode manually. */
  function disable(): void {
    manualOverride.value = false
    isEnabled.value = false
    source.value = 'none'
    applyToDOM(false)
    savePreference(false)

    console.info('[argus:low-spec] Low-spec mode manually disabled')
  }

  /** Toggle low-spec mode. */
  function toggle(): void {
    if (isEnabled.value) {
      disable()
    } else {
      enable()
    }
  }

  /** Reset to auto-detect (remove manual override). */
  function resetToAuto(): void {
    manualOverride.value = null
    savePreference(null)
    highCpuStartTime = null

    // Re-evaluate based on current CPU state
    if (healthMetrics) {
      checkAutoDetect()
    } else {
      isEnabled.value = false
      source.value = 'none'
      applyToDOM(false)
    }

    console.info('[argus:low-spec] Reset to auto-detect mode')
  }

  // -------------------------------------------------------------------------
  // Computed
  // -------------------------------------------------------------------------

  /** Human-readable label for the current mode source. */
  const sourceLabel: ComputedRef<string> = computed(() => {
    switch (source.value) {
      case 'manual': return 'Ручной'
      case 'auto': return 'Автоматический'
      case 'none': return 'Выключен'
      default: return ''
    }
  })

  /** Whether the mode was set by auto-detection (not manual). */
  const isAutoDetected: ComputedRef<boolean> = computed(() => source.value === 'auto')

  /** Whether a manual override is active. */
  const hasManualOverride: ComputedRef<boolean> = computed(() => manualOverride.value !== null)

  // -------------------------------------------------------------------------
  // Initialization
  // -------------------------------------------------------------------------

  /**
   * Apply admin force policy for low-spec mode.
   * When the exam has forceLowSpecMode = true, this overrides auto-detect
   * and manual preference — the student cannot disable it.
   */
  function setForcePolicy(force: boolean): void {
    if (force) {
      isEnabled.value = true
      source.value = 'manual'
      applyToDOM(true)
      console.info('[argus:low-spec] Low-spec mode forced by exam policy')
    }
  }

  /** Initialize the composable — load stored preference and apply. */
  function initialize(): void {
    // Admin force policy takes highest precedence.
    if (forcePolicy?.value) {
      setForcePolicy(true)
      return
    }

    const stored = loadPreference()

    if (stored !== null) {
      manualOverride.value = stored
      isEnabled.value = stored
      source.value = stored ? 'manual' : 'none'
      applyToDOM(stored)
    }

    // Start auto-detection if health metrics are available.
    if (healthMetrics) {
      startAutoDetect()
    }
  }

  // Watch for force policy changes (e.g., settings updated during exam).
  if (forcePolicy) {
    watch(forcePolicy, (force) => {
      if (force) setForcePolicy(true)
    })
  }

  // Run on mount if in browser context.
  if (typeof window !== 'undefined') {
    initialize()
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // State
    isEnabled,
    source,
    sourceLabel,
    isAutoDetected,
    hasManualOverride,

    // Actions
    enable,
    disable,
    toggle,
    resetToAuto,
    setForcePolicy,

    // Auto-detection control
    startAutoDetect,
    stopAutoDetect
  }
}
