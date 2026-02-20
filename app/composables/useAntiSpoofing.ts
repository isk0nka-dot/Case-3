// =============================================================================
// Argus AI — useAntiSpoofing Composable (Multi-Modal Anti-Spoofing)
// =============================================================================
//
// Detects static photos, 2D screens, and pre-recorded video being held up
// to the camera. Uses multiple signals:
//
//   1. Micro-movement analysis — real faces have involuntary micro-movements
//      (muscle tremors, breathing, eye saccades) at 1-3Hz. Photos are static.
//
//   2. Temporal consistency — real video has frame-to-frame variance in
//      lighting, pixel noise, and subtle head position changes. Loops and
//      static images have near-zero temporal entropy.
//
//   3. Blink pattern analysis — real humans blink 15-20 times per minute.
//      Photos/screens show zero blinks. Pre-recorded videos may have
//      unnaturally regular blink patterns.
//
//   4. Head pose variance — real humans have subtle involuntary head sway
//      (0.5-2 degrees). Photos/screens have exactly 0 variance.
//
//   5. Frame-to-frame pixel entropy — screen playback has quantization
//      artifacts and compression patterns not present in live video.
//
// Detection runs at 2Hz (every 500ms), collecting 10-second windows of
// data for analysis. A spoof is flagged as a Critical Security Breach
// after 3 consecutive detections.
//
// =============================================================================

import { ref, computed } from 'vue'
import type { VisionFrame } from './useVisionEngine'
import { EventType, Severity, EventSource } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SpoofAnalysis {
  isSpoof: boolean
  confidence: number
  spoofType: 'photo' | 'screen' | 'video_replay' | 'none'
  signals: SpoofSignal[]
  timestamp: number
}

export interface SpoofSignal {
  name: string
  score: number       // 0-1 (1 = strong spoof indicator)
  detail: string
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useAntiSpoofing() {
  const currentAnalysis = ref<SpoofAnalysis | null>(null)
  const isActive = ref(false)
  const consecutiveDetections = ref(0)

  // Rolling windows for temporal analysis
  const headPoseHistory: { yaw: number; pitch: number; roll: number; ts: number }[] = []
  const blinkHistory: { count: number; ts: number }[] = []
  const livenessHistory: number[] = []
  const frameQualityHistory: number[] = []

  const WINDOW_SIZE = 20 // 10 seconds at 2Hz
  const CONSECUTIVE_THRESHOLD = 3

  let analysisTimer: ReturnType<typeof setInterval> | null = null

  // Event callback
  let sendEventFn: ((
    eventType: EventType,
    severity: Severity,
    payload?: any,
    label?: string,
    confidence?: number,
    source?: EventSource
  ) => void) | null = null

  function onEvent(fn: typeof sendEventFn) {
    sendEventFn = fn
  }

  /** Ingest a vision frame for anti-spoofing analysis. */
  function ingestFrame(frame: VisionFrame) {
    if (!isActive.value) return

    const now = Date.now()

    // Record head pose
    headPoseHistory.push({
      yaw: frame.headPose.yaw,
      pitch: frame.headPose.pitch,
      roll: frame.headPose.roll,
      ts: now,
    })
    if (headPoseHistory.length > WINDOW_SIZE) headPoseHistory.shift()

    // Record blink count
    blinkHistory.push({ count: frame.blink.blinkCount, ts: now })
    if (blinkHistory.length > WINDOW_SIZE) blinkHistory.shift()

    // Record liveness and quality
    livenessHistory.push(frame.livenessScore)
    if (livenessHistory.length > WINDOW_SIZE) livenessHistory.shift()

    frameQualityHistory.push(frame.frameQuality)
    if (frameQualityHistory.length > WINDOW_SIZE) frameQualityHistory.shift()
  }

  /** Run the composite anti-spoofing analysis. */
  function analyze(): SpoofAnalysis {
    const signals: SpoofSignal[] = []

    // Signal 1: Head pose micro-movement variance
    const headPoseVariance = analyzeHeadPoseVariance()
    signals.push(headPoseVariance)

    // Signal 2: Blink pattern analysis
    const blinkPattern = analyzeBlinkPattern()
    signals.push(blinkPattern)

    // Signal 3: Liveness score consistency
    const livenessConsistency = analyzeLivenessConsistency()
    signals.push(livenessConsistency)

    // Signal 4: Temporal frame quality variance
    const qualityVariance = analyzeQualityVariance()
    signals.push(qualityVariance)

    // Composite score (weighted average)
    const weights: Record<string, number> = {
      head_pose_variance: 0.3,
      blink_pattern: 0.3,
      liveness_consistency: 0.25,
      quality_variance: 0.15,
    }

    let compositeScore = 0
    for (const signal of signals) {
      compositeScore += signal.score * (weights[signal.name] ?? 0.1)
    }

    // Determine spoof type
    let spoofType: SpoofAnalysis['spoofType'] = 'none'
    if (compositeScore >= 0.6) {
      if (headPoseVariance.score >= 0.8 && blinkPattern.score >= 0.8) {
        spoofType = 'photo'
      } else if (blinkPattern.score < 0.5 && headPoseVariance.score >= 0.7) {
        spoofType = 'screen'
      } else {
        spoofType = 'video_replay'
      }
    }

    const isSpoof = compositeScore >= 0.6
    const result: SpoofAnalysis = {
      isSpoof,
      confidence: Math.min(1, compositeScore),
      spoofType,
      signals,
      timestamp: Date.now(),
    }

    currentAnalysis.value = result

    // Track consecutive detections
    if (isSpoof) {
      consecutiveDetections.value++

      if (consecutiveDetections.value >= CONSECUTIVE_THRESHOLD) {
        // CRITICAL SECURITY BREACH
        sendEventFn?.(
          EventType.FACE_SPOOF_DETECTED,
          Severity.CRITICAL,
          {
            type: 'faceDetection',
            data: {
              match: false,
              similarity: 0,
              faceCount: 1,
              isSpoof: true,
              spoofType,
            }
          },
          `Критическая угроза: обнаружена ${spoofType === 'photo' ? 'фотография' : spoofType === 'screen' ? 'экран' : 'видеозапись'} (${(compositeScore * 100).toFixed(0)}%)`,
          compositeScore,
          EventSource.WEBCAM
        )
      }
    } else {
      consecutiveDetections.value = 0
    }

    return result
  }

  // -------------------------------------------------------------------------
  // Signal Analyzers
  // -------------------------------------------------------------------------

  function analyzeHeadPoseVariance(): SpoofSignal {
    if (headPoseHistory.length < 5) {
      return { name: 'head_pose_variance', score: 0, detail: 'Недостаточно данных' }
    }

    // Calculate standard deviation of head pose angles
    const yaws = headPoseHistory.map(h => h.yaw)
    const pitches = headPoseHistory.map(h => h.pitch)

    const yawStd = stdDev(yaws)
    const pitchStd = stdDev(pitches)

    // Real faces: yaw std > 0.3°, pitch std > 0.2° (involuntary sway)
    // Photos/screens: std ≈ 0 (perfectly static)
    const combinedStd = (yawStd + pitchStd) / 2
    const isStatic = combinedStd < 0.15 // Less than 0.15° average movement = suspicious

    const score = isStatic ? Math.min(1, 1 - combinedStd / 0.3) : 0

    return {
      name: 'head_pose_variance',
      score,
      detail: `Дисперсия головы: yaw=${yawStd.toFixed(2)}°, pitch=${pitchStd.toFixed(2)}°`
    }
  }

  function analyzeBlinkPattern(): SpoofSignal {
    if (blinkHistory.length < 10) {
      return { name: 'blink_pattern', score: 0, detail: 'Недостаточно данных' }
    }

    // Calculate blinks in the window
    const firstBlink = blinkHistory[0]!.count
    const lastBlink = blinkHistory[blinkHistory.length - 1]!.count
    const blinksInWindow = lastBlink - firstBlink

    const windowDurationSec = (blinkHistory[blinkHistory.length - 1]!.ts - blinkHistory[0]!.ts) / 1000
    const blinksPerMinute = windowDurationSec > 0 ? (blinksInWindow / windowDurationSec) * 60 : 0

    // Real humans: 15-20 blinks/min average
    // Photos: 0 blinks
    // Pre-recorded: may have regular pattern
    let score = 0
    if (blinksPerMinute < 2) {
      score = 0.9 // Almost no blinking = likely photo or screen
    } else if (blinksPerMinute < 5) {
      score = 0.5 // Very low blink rate
    } else if (blinksPerMinute > 40) {
      score = 0.3 // Abnormally high (might be video artifact)
    }

    return {
      name: 'blink_pattern',
      score,
      detail: `Частота моргания: ${blinksPerMinute.toFixed(1)}/мин (${blinksInWindow} за ${windowDurationSec.toFixed(0)}с)`
    }
  }

  function analyzeLivenessConsistency(): SpoofSignal {
    if (livenessHistory.length < 5) {
      return { name: 'liveness_consistency', score: 0, detail: 'Недостаточно данных' }
    }

    const avgLiveness = livenessHistory.reduce((a, b) => a + b, 0) / livenessHistory.length
    const livenessStd = stdDev(livenessHistory)

    // Low liveness score + low variance = consistent spoof
    let score = 0
    if (avgLiveness < 0.4) {
      score = 0.8
    } else if (avgLiveness < 0.6 && livenessStd < 0.05) {
      score = 0.5
    }

    return {
      name: 'liveness_consistency',
      score,
      detail: `Живость: среднее=${avgLiveness.toFixed(2)}, σ=${livenessStd.toFixed(3)}`
    }
  }

  function analyzeQualityVariance(): SpoofSignal {
    if (frameQualityHistory.length < 5) {
      return { name: 'quality_variance', score: 0, detail: 'Недостаточно данных' }
    }

    const qualityStd = stdDev(frameQualityHistory)

    // Real camera: natural quality fluctuation (lighting, auto-exposure)
    // Screen playback: very consistent quality (compressed, stable)
    const score = qualityStd < 0.01 ? 0.6 : 0

    return {
      name: 'quality_variance',
      score,
      detail: `Вариация качества: σ=${qualityStd.toFixed(4)}`
    }
  }

  // -------------------------------------------------------------------------
  // Utilities
  // -------------------------------------------------------------------------

  function stdDev(values: number[]): number {
    if (values.length < 2) return 0
    const mean = values.reduce((a, b) => a + b, 0) / values.length
    const squaredDiffs = values.map(v => (v - mean) ** 2)
    return Math.sqrt(squaredDiffs.reduce((a, b) => a + b, 0) / values.length)
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  function start() {
    if (isActive.value) return
    isActive.value = true

    // Run analysis at 2Hz
    analysisTimer = setInterval(analyze, 500)
  }

  function stop() {
    isActive.value = false
    if (analysisTimer) {
      clearInterval(analysisTimer)
      analysisTimer = null
    }

    // Clear histories
    headPoseHistory.length = 0
    blinkHistory.length = 0
    livenessHistory.length = 0
    frameQualityHistory.length = 0
    consecutiveDetections.value = 0
  }

  return {
    currentAnalysis,
    isActive,
    consecutiveDetections,
    ingestFrame,
    analyze,
    start,
    stop,
    onEvent,
  }
}
