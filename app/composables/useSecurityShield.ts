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

import { ref, computed, watch, type ComputedRef, type Ref } from 'vue'
import type { VisionFrame } from './useVisionEngine'
import { useVisionEngine } from './useVisionEngine'
import type { AudioFrame } from './useAudioEngine'
import { useAudioEngine } from './useAudioEngine'
import { EventType, Severity, EventSource } from '~/lib/proto/types'
import type { EventPayload } from '~/lib/proto/types'
import {
  createDefaultRealtimeAIRuleState,
  createDefaultRealtimeAIThresholds,
  createDefaultRealtimeAudioThresholds,
  evaluateAudioFrame,
  evaluateVisionFrame,
  type RealtimeAIEventDecision,
  type RealtimeAIRuleThresholds,
  type RealtimeAudioRuleThresholds
} from '~/lib/ai/realtimeEventRules'

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
    source?: EventSource
  ) => void

  /** Optional challenge configuration overrides. */
  challengeConfig?: ChallengeConfig

  /** Optional watermark configuration overrides. */
  watermarkConfig?: Partial<Omit<WatermarkConfig, 'sessionId'>>

  /** Baseline device fingerprint for verification (from session start). */
  baselineFingerprint?: DeviceFingerprint

  /** Existing webcam video element for local MediaPipe inference. */
  videoElement?: Ref<HTMLVideoElement | null>

  /** Real-time browser AI settings. Defaults are intentionally conservative. */
  realtimeAI?: {
    enabled?: boolean
    inferenceHz?: number
    thresholds?: Partial<RealtimeAIRuleThresholds>
  }

  /** Real-time browser audio AI settings. Raw microphone audio never leaves the device. */
  realtimeAudio?: {
    enabled?: boolean
    analysisHz?: number
    vadThresholdDb?: number
    thresholds?: Partial<RealtimeAudioRuleThresholds>
  }
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
    ...watermarkConfig
  })
  const fingerprint = useDeviceFingerprint()
  const antiSpoof = useAntiSpoofing()
  const browser = useBrowserIntegrity()
  const realtimeAIEnabled = config.realtimeAI?.enabled ?? true
  const realtimeAIThresholds: RealtimeAIRuleThresholds = {
    ...createDefaultRealtimeAIThresholds(),
    ...config.realtimeAI?.thresholds
  }
  const realtimeAudioThresholds: RealtimeAudioRuleThresholds = {
    ...createDefaultRealtimeAudioThresholds(),
    ...config.realtimeAudio?.thresholds
  }
  const realtimeAIState = createDefaultRealtimeAIRuleState()
  const vision = realtimeAIEnabled && config.videoElement
    ? useVisionEngine({
        videoElement: config.videoElement,
        inferenceHz: config.realtimeAI?.inferenceHz ?? 2
      })
    : null
  const audio = realtimeAIEnabled && (config.realtimeAudio?.enabled ?? true)
    ? useAudioEngine({
        analysisHz: config.realtimeAudio?.analysisHz ?? 4,
        vadThresholdDb: config.realtimeAudio?.vadThresholdDb
      })
    : null

  // -------------------------------------------------------------------------
  // Wire event callbacks
  // -------------------------------------------------------------------------

  liveness.onEvent(sendEvent)
  virtualCam.onEvent(sendEvent)
  fingerprint.onEvent(sendEvent)
  antiSpoof.onEvent(sendEvent)
  browser.onEvent(sendEvent)

  if (vision) {
    watch(vision.currentFrame, (frame) => {
      if (!started || !frame) return

      ingestVisionFrame(frame)
      const decisions = evaluateVisionFrame(frame, realtimeAIThresholds, realtimeAIState)
      for (const decision of decisions) {
        emitRealtimeAIDecision(decision, frame)
      }
    })
  }

  if (audio) {
    watch(audio.currentFrame, (frame) => {
      if (!started || !frame) return

      const decisions = evaluateAudioFrame(frame, realtimeAudioThresholds, realtimeAIState)
      for (const decision of decisions) {
        emitRealtimeAIDecision(decision, undefined, frame)
      }
    })
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  let started = false
  let periodicVerifyTimer: ReturnType<typeof setInterval> | null = null

  // Permission-blocked state: exam cannot proceed without camera/mic
  const permissionBlocked = ref(false)

  /**
   * Start all shield subsystems.
   *
   * If no MediaStream is provided, attempts to acquire camera/mic permissions.
   * If permissions are denied, the shield enters BLOCKED state — the exam
   * page should show a HardBlockerModal and prevent the student from proceeding.
   *
   * @param stream - MediaStream from getUserMedia for virtual camera analysis
   */
  async function start(stream?: MediaStream): Promise<void> {
    if (started) return
    started = true

    // ── Camera/mic permission enforcement ─────────────────────────────────
    // If no stream was provided, attempt to acquire permissions.
    // Camera is REQUIRED for proctoring — exam cannot proceed without it.
    if (!stream) {
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          video: { width: 640, height: 480, facingMode: 'user' },
          audio: true
        })
      } catch {
        // Camera/mic denied or unavailable — hard block the exam
        permissionBlocked.value = true
        sendEvent(
          EventType.CAMERA_BLOCKED,
          Severity.CRITICAL,
          {
            type: 'system',
            data: {
              message: 'Camera/microphone permission denied — exam cannot proceed'
            }
          },
          'Камера/микрофон отклонены — экзамен невозможен',
          1.0,
          EventSource.BROWSER
        )
        // Do NOT start any subsystems — the exam is blocked
        started = false
        return
      }
    }

    // 1. Browser integrity — immediate
    browser.start()

    // 2. Liveness challenges — starts scheduling
    liveness.start()

    // 3. Anti-spoofing — starts analysis loop
    antiSpoof.start()

    // 4. Watermark — starts token rotation
    watermark.start()

    // 4b. Browser-local MediaPipe inference. Runs at low frequency by default
    // and emits structured events only; raw frames never leave the browser.
    if (vision) {
      const startedVision = await vision.start()
      if (!startedVision) {
        sendEvent(
          EventType.CAMERA_BLOCKED,
          Severity.WARNING,
          {
            type: 'system',
            data: {
              message: 'MediaPipe vision model failed to start'
            }
          },
          'AI vision model unavailable',
          1.0,
          EventSource.WEBCAM
        )
      }
    }

    // 4c. Browser-local audio analysis. This sends structured telemetry only:
    // RMS dB, VAD flags, classifications and speaker count heuristics.
    if (audio) {
      const startedAudio = await audio.start()
      if (!startedAudio) {
        sendEvent(
          EventType.AUDIO_ANOMALY,
          Severity.WARNING,
          {
            type: 'system',
            data: {
              message: 'Audio analysis engine failed to start'
            }
          },
          'Audio AI model unavailable',
          1.0,
          EventSource.BROWSER
        )
      }
    }

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
              virtualMonitor: false
            }
          },
          `Устройство изменилось: сходство ${(similarity * 100).toFixed(0)}%`,
          1 - similarity,
          EventSource.BROWSER
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
    vision?.stop()
    audio?.stop()

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

  function emitRealtimeAIDecision(decision: RealtimeAIEventDecision, frame?: VisionFrame, audioFrame?: AudioFrame): void {
    switch (decision.kind) {
      case 'gaze_telemetry':
        sendEvent(
          EventType.GAZE_TELEMETRY,
          Severity.INFO,
          { type: 'gazeDeviation', data: { direction: 'center', durationMs: 0, angleDegrees: 0, gazeX: decision.gazeX, gazeY: decision.gazeY } },
          '',
          1.0,
          EventSource.WEBCAM
        )
        break
      case 'gaze_deviation':
        sendEvent(
          EventType.GAZE_DEVIATION,
          decision.durationMs > 5000 ? Severity.CRITICAL : Severity.WARNING,
          {
            type: 'gazeDeviation',
            data: {
              direction: decision.direction,
              durationMs: decision.durationMs,
              angleDegrees: decision.angleDegrees,
              gazeX: decision.gazeX,
              gazeY: decision.gazeY
            }
          },
          `Взгляд отведён ${decision.direction} на ${(decision.durationMs / 1000).toFixed(1)}с`,
          0.9,
          EventSource.WEBCAM
        )
        break
      case 'face_not_detected':
        sendEvent(
          EventType.FACE_NOT_DETECTED,
          Severity.WARNING,
          { type: 'faceDetection', data: { match: false, similarity: 0, faceCount: 0, isSpoof: false } },
          `Лицо не обнаружено (${decision.consecutiveFrames} кадров)`,
          1.0,
          EventSource.WEBCAM
        )
        break
      case 'multiple_persons':
        sendEvent(
          EventType.MULTIPLE_PERSONS,
          Severity.CRITICAL,
          { type: 'faceDetection', data: { match: false, similarity: 0, faceCount: decision.faceCount, isSpoof: false } },
          `Обнаружено несколько лиц: ${decision.faceCount}`,
          1.0,
          EventSource.WEBCAM
        )
        break
      case 'head_pose_anomaly':
        if (!frame) break
        sendEvent(
          EventType.HEAD_POSE_ANOMALY,
          Severity.WARNING,
          { type: 'headPose', data: vision?.buildHeadPosePayload(frame) ?? buildFallbackHeadPosePayload(frame) },
          `Положение головы вне допустимого диапазона: yaw=${decision.yaw.toFixed(1)}, pitch=${decision.pitch.toFixed(1)}, roll=${decision.roll.toFixed(1)}`,
          0.85,
          EventSource.WEBCAM
        )
        break
      case 'liveness_failed':
        if (!frame) break
        sendEvent(
          EventType.LIVENESS_CHECK_FAILED,
          Severity.CRITICAL,
          { type: 'liveness', data: vision?.buildLivenessPayload(frame) ?? buildFallbackLivenessPayload(frame) },
          `Проверка живости не пройдена: ${(decision.livenessScore * 100).toFixed(0)}%`,
          1 - decision.livenessScore,
          EventSource.WEBCAM
        )
        break
      case 'audio_level_telemetry':
        if (!audioFrame) break
        sendEvent(
          EventType.AUDIO_LEVEL_TELEMETRY,
          Severity.INFO,
          { type: 'audioAnalysis', data: audio?.buildAudioAnalysisPayload(audioFrame) ?? buildFallbackAudioPayload(audioFrame) },
          '',
          1.0,
          EventSource.BROWSER
        )
        break
      case 'voice_activity':
        if (!audioFrame) break
        sendEvent(
          EventType.VOICE_ACTIVITY,
          Severity.WARNING,
          { type: 'audioAnalysis', data: audio?.buildAudioAnalysisPayload(audioFrame) ?? buildFallbackAudioPayload(audioFrame) },
          `Голос обнаружен: VAD ${(decision.vadConfidence * 100).toFixed(0)}%, RMS ${decision.rmsDb.toFixed(1)} dB`,
          decision.vadConfidence,
          EventSource.BROWSER
        )
        break
      case 'audio_anomaly':
        if (!audioFrame) break
        sendEvent(
          EventType.AUDIO_ANOMALY,
          Severity.WARNING,
          { type: 'audioAnalysis', data: audio?.buildAudioAnalysisPayload(audioFrame) ?? buildFallbackAudioPayload(audioFrame) },
          `Аудио аномалия: ${decision.classification}, RMS ${decision.rmsDb.toFixed(1)} dB`,
          decision.confidence,
          EventSource.BROWSER
        )
        break
      case 'whisper_detected':
        if (!audioFrame) break
        sendEvent(
          EventType.WHISPER_DETECTED,
          Severity.WARNING,
          { type: 'audioAnalysis', data: audio?.buildAudioAnalysisPayload(audioFrame) ?? buildFallbackAudioPayload(audioFrame) },
          `Шёпот обнаружен: ${(decision.confidence * 100).toFixed(0)}%`,
          decision.confidence,
          EventSource.BROWSER
        )
        break
      case 'second_speaker_detected':
        if (!audioFrame) break
        sendEvent(
          EventType.SECOND_SPEAKER_DETECTED,
          Severity.CRITICAL,
          { type: 'audioAnalysis', data: audio?.buildAudioAnalysisPayload(audioFrame) ?? buildFallbackAudioPayload(audioFrame) },
          `Обнаружено несколько голосов: ${decision.speakerCount}`,
          decision.confidence,
          EventSource.BROWSER
        )
        break
    }
  }

  function buildFallbackHeadPosePayload(frame: VisionFrame) {
    return {
      yaw: frame.headPose.yaw,
      pitch: frame.headPose.pitch,
      roll: frame.headPose.roll,
      faceX: frame.faceBBox?.x ?? 0,
      faceY: frame.faceBBox?.y ?? 0,
      faceW: frame.faceBBox?.w ?? 0,
      faceH: frame.faceBBox?.h ?? 0,
      ipdPx: 0,
      landmarkCount: frame.landmarks ? frame.landmarks.length / 3 : 0,
      inferenceMs: frame.inferenceMs
    }
  }

  function buildFallbackLivenessPayload(frame: VisionFrame) {
    return {
      livenessScore: frame.livenessScore,
      blinkDetected: frame.blink.isBlinking,
      blinkRatePerMin: frame.blink.blinkRatePerMin,
      textureScore: frame.frameQuality,
      depthScore: 0.5,
      spoofVector: frame.livenessScore < realtimeAIThresholds.livenessThreshold ? 'unknown' : 'none',
      frameQuality: frame.frameQuality
    }
  }

  function buildFallbackAudioPayload(frame: AudioFrame) {
    return {
      rmsDb: frame.rmsDb,
      vadActive: frame.vadActive,
      vadConfidence: frame.vadConfidence,
      spectralCentroidHz: frame.spectralCentroidHz,
      zcr: frame.zcr,
      classification: frame.classification,
      classificationConfidence: frame.classificationConfidence,
      speakerCount: frame.speakerCount,
      speakerMatch: true,
      speakerSimilarity: 1.0,
      segmentDurationMs: Math.round(1000 / (config.realtimeAudio?.analysisHz ?? 4))
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
    const biIssues = (bi.tabSwitchCount > 3 ? 1 : 0)
      + (bi.devToolsOpen ? 1 : 0)
      + (bi.isSecondScreenDetected ? 1 : 0)

    const spoof = antiSpoof.currentAnalysis.value
    const vc = virtualCam.report.value

    return {
      isRunning: started,

      browserIntegrity: biIssues >= 2 ? 'critical' : biIssues >= 1 ? 'warning' : 'ok',

      antiSpoofing: !spoof
        ? 'inactive'
        : spoof.isSpoof
          ? 'critical'
          : spoof.confidence > 0.3
            ? 'warning'
            : 'ok',

      virtualCamera: virtualCam.isChecking.value
        ? 'checking'
        : !vc
            ? 'inactive'
            : vc.isVirtual
              ? 'critical'
              : 'ok',

      deviceFingerprint: fingerprint.isCapturing.value
        ? 'capturing'
        : fingerprint.fingerprint.value?.isVirtualMachine
          ? 'critical'
          : 'ok',

      liveness: liveness.currentChallenge.value?.status === 'active'
        ? 'challenging'
        : liveness.passRate.value < 0.5
          ? 'critical'
          : liveness.passRate.value < 0.8
            ? 'warning'
            : 'ok',

      watermark: watermark.state.value.isActive ? 'active' : 'inactive'
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

    // Permission state (for HardBlockerModal)
    permissionBlocked,

    // Subsystem access (for SecurityShield.vue props)
    browserState: browser.state,
    spoofAnalysis: antiSpoof.currentAnalysis,
    virtualCameraReport: virtualCam.report,
    deviceFingerprint: fingerprint.fingerprint,
    currentChallenge: liveness.currentChallenge,
    livenessPassRate: liveness.passRate,
    watermarkActive: computed(() => watermark.state.value.isActive),
    visionFrame: vision?.currentFrame ?? ref<VisionFrame | null>(null),
    visionActive: computed(() => vision?.isActive.value ?? false),
    audioFrame: audio?.currentFrame ?? ref<AudioFrame | null>(null),
    audioActive: computed(() => audio?.isActive.value ?? false),
    challengeHistory: liveness.challengeHistory,
    consecutiveFailures: liveness.consecutiveFailures
  }
}
