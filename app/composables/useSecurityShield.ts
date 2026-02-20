// =============================================================================
// Argus AI — useSecurityShield Composable (Orchestrator)
// =============================================================================
//
// Unified orchestrator that wires all security subsystems together:
//
//   1. useLivenessChallenge   — Active liveness verification
//   2. useVirtualCameraDetector — Virtual camera/OBS detection
//   3. useStreamWatermark     — LSB steganographic watermark
//   4. useDeviceFingerprint   — Deep hardware device ID
//   5. useAntiSpoofing        — Photo/screen/replay detection
//   6. useBrowserIntegrity    — Tab focus, second screen, DevTools
//
// Integration:
//   All subsystems report events via a single `sendEvent` callback
//   bound to useProctoringSession.sendEvent(). The orchestrator manages
//   lifecycle (start/stop) and provides reactive state for the
//   SecurityShield.vue and LivenessChallenge.vue components.
//
// Usage:
//   const shield = useSecurityShield({
//     sessionId: 'sess-123',
//     sendEvent: session.sendEvent,
//   })
//   await shield.start(mediaStream)
//   // In vision loop: shield.ingestVisionFrame(frame)
//   shield.stop()
//
// =============================================================================

import { computed, type ComputedRef } from 'vue'
import type { VisionFrame } from './useVisionEngine'
import { EventType, Severity, EventSource } from '~/lib/proto/types'
import type { EventPayload } from '~/lib/proto/types'

import { useLivenessChallenge, type ChallengeConfig } from './useLivenessChallenge'
import { useVirtualCameraDetector } from './useVirtualCameraDetector'
import { useStreamWatermark, type WatermarkConfig } from './useStreamWatermark'
import { useDeviceFingerprint, type DeviceFingerprint } from './useDeviceFingerprint'
import { useAntiSpoofing } from './useAntiSpoofing'
import { useBrowserIntegrity } from './useBrowserIntegrity'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SecurityShieldConfig {
  /** Session ID for watermark binding and event routing. */
  sessionId: string

  /** Event sender bound to useProctoringSession.sendEvent(). */
  sendEvent: (
    eventType: EventType,
    severity: Severity,
    payload?: EventPayload,
    label?: string,
    confidence?: number,
    source?: EventSource,
  ) => void

  /** Optional challenge configuration overrides. */
  challengeConfig?: ChallengeConfig

  /** Optional watermark configuration overrides. */
  watermarkConfig?: Partial<Omit<WatermarkConfig, 'sessionId'>>

  /** Baseline device fingerprint for verification (from session start). */
  baselineFingerprint?: DeviceFingerprint
}

export interface ShieldStatus {
  isRunning: boolean
  browserIntegrity: 'ok' | 'warning' | 'critical'
  antiSpoofing: 'ok' | 'warning' | 'critical' | 'inactive'
  virtualCamera: 'ok' | 'critical' | 'checking' | 'inactive'
  deviceFingerprint: 'ok' | 'critical' | 'capturing'
  liveness: 'ok' | 'warning' | 'critical' | 'challenging'
  watermark: 'active' | 'inactive'
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useSecurityShield(config: SecurityShieldConfig) {
  const { sessionId, sendEvent, challengeConfig, watermarkConfig, baselineFingerprint } = config

  // -------------------------------------------------------------------------
  // Initialize subsystems
  // -------------------------------------------------------------------------

  const liveness = useLivenessChallenge(challengeConfig)
  const virtualCam = useVirtualCameraDetector()
  const watermark = useStreamWatermark({
    sessionId,
    ...watermarkConfig,
  })
  const fingerprint = useDeviceFingerprint()
  const antiSpoof = useAntiSpoofing()
  const browser = useBrowserIntegrity()

  // -------------------------------------------------------------------------
  // Wire event callbacks
  // -------------------------------------------------------------------------

  liveness.onEvent(sendEvent)
  virtualCam.onEvent(sendEvent)
  fingerprint.onEvent(sendEvent)
  antiSpoof.onEvent(sendEvent)
  browser.onEvent(sendEvent)

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  let started = false
  let periodicVerifyTimer: ReturnType<typeof setInterval> | null = null

  /**
   * Start all shield subsystems.
   *
   * @param stream - MediaStream from getUserMedia for virtual camera analysis
   */
  async function start(stream?: MediaStream): Promise<void> {
    if (started) return
    started = true

    // 1. Browser integrity — immediate
    browser.start()

    // 2. Liveness challenges — starts scheduling
    liveness.start()

    // 3. Anti-spoofing — starts analysis loop
    antiSpoof.start()

    // 4. Watermark — starts token rotation
    watermark.start()

    // 5. Device fingerprint — capture baseline
    const fp = await fingerprint.capture()

    // Verify against baseline if provided
    if (baselineFingerprint) {
      const similarity = fingerprint.verify(baselineFingerprint)
      if (similarity < 0.7) {
        sendEvent(
          EventType.HARDWARE_ID_MISMATCH,
          Severity.CRITICAL,
          {
            type: 'kernel',
            data: {
              wpm: 0,
              keystrokeStdDevMs: 0,
              handOnMouse: false,
              hardwareIdHash: fingerprint.fingerprint.value?.deviceId,
              mismatchComponent: `baseline_sim=${similarity.toFixed(3)}`,
              monitorCount: 1,
              virtualMonitor: false,
            }
          },
          `Устройство изменилось: сходство ${(similarity * 100).toFixed(0)}%`,
          1 - similarity,
          EventSource.BROWSER,
        )
      }
    }

    // 6. Virtual camera analysis — async, non-blocking
    if (stream) {
      virtualCam.analyze(stream).catch(() => {
        // Non-fatal — analysis may fail on some browsers
      })
    }

    // Periodic fingerprint re-verification (every 5 minutes)
    periodicVerifyTimer = setInterval(async () => {
      const current = await fingerprint.capture()
      if (baselineFingerprint || fp) {
        const baseline = baselineFingerprint ?? fp
        fingerprint.verify(baseline)
      }
    }, 300_000)
  }

  /**
   * Stop all shield subsystems.
   */
  function stop(): void {
    if (!started) return
    started = false

    browser.stop()
    liveness.stop()
    antiSpoof.stop()
    watermark.stop()

    if (periodicVerifyTimer) {
      clearInterval(periodicVerifyTimer)
      periodicVerifyTimer = null
    }
  }

  // -------------------------------------------------------------------------
  // Vision frame ingestion
  // -------------------------------------------------------------------------

  /**
   * Feed a VisionFrame from the vision engine into all relevant subsystems.
   * Call this from the vision engine's frame callback.
   */
  function ingestVisionFrame(frame: VisionFrame): void {
    if (!started) return

    // Anti-spoofing ingestion
    antiSpoof.ingestFrame(frame)

    // Liveness challenge verification
    if (liveness.currentChallenge.value?.status === 'active') {
      liveness.verifyFrame(frame)
    }
  }

  // -------------------------------------------------------------------------
  // Watermark helpers
  // -------------------------------------------------------------------------

  /**
   * Apply watermark to a video element's current frame.
   * Returns a watermarked canvas or null.
   */
  function watermarkVideoFrame(video: HTMLVideoElement) {
    return watermark.watermarkVideoFrame(video)
  }

  /**
   * Apply watermark to an existing canvas.
   */
  function applyWatermarkToCanvas(canvas: HTMLCanvasElement): void {
    watermark.applyToCanvas(canvas)
  }

  // -------------------------------------------------------------------------
  // Composite status
  // -------------------------------------------------------------------------

  const status: ComputedRef<ShieldStatus> = computed(() => {
    const bi = browser.state.value
    const biIssues = (bi.tabSwitchCount > 3 ? 1 : 0) +
      (bi.devToolsOpen ? 1 : 0) +
      (bi.isSecondScreenDetected ? 1 : 0)

    const spoof = antiSpoof.currentAnalysis.value
    const vc = virtualCam.report.value

    return {
      isRunning: started,

      browserIntegrity: biIssues >= 2 ? 'critical' : biIssues >= 1 ? 'warning' : 'ok',

      antiSpoofing: !spoof ? 'inactive'
        : spoof.isSpoof ? 'critical'
        : spoof.confidence > 0.3 ? 'warning'
        : 'ok',

      virtualCamera: virtualCam.isChecking.value ? 'checking'
        : !vc ? 'inactive'
        : vc.isVirtual ? 'critical'
        : 'ok',

      deviceFingerprint: fingerprint.isCapturing.value ? 'capturing'
        : fingerprint.fingerprint.value?.isVirtualMachine ? 'critical'
        : 'ok',

      liveness: liveness.currentChallenge.value?.status === 'active' ? 'challenging'
        : liveness.passRate.value < 0.5 ? 'critical'
        : liveness.passRate.value < 0.8 ? 'warning'
        : 'ok',

      watermark: watermark.state.value.isActive ? 'active' : 'inactive',
    }
  })

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // Lifecycle
    start,
    stop,

    // Vision integration
    ingestVisionFrame,

    // Watermark helpers
    watermarkVideoFrame,
    applyWatermarkToCanvas,

    // Composite status
    status,

    // Subsystem access (for SecurityShield.vue props)
    browserState: browser.state,
    spoofAnalysis: antiSpoof.currentAnalysis,
    virtualCameraReport: virtualCam.report,
    deviceFingerprint: fingerprint.fingerprint,
    currentChallenge: liveness.currentChallenge,
    livenessPassRate: liveness.passRate,
    watermarkActive: computed(() => watermark.state.value.isActive),
    challengeHistory: liveness.challengeHistory,
    consecutiveFailures: liveness.consecutiveFailures,
  }
}
