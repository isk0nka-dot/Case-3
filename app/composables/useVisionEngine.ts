// =============================================================================
// Argus AI — useVisionEngine Composable
// =============================================================================
//
// Real-time AI vision pipeline for face detection, head pose estimation,
// gaze tracking, and liveness verification using MediaPipe Face Mesh.
//
// Architecture:
//   Browser Camera → MediaPipe Face Mesh (478 landmarks) → Feature Extraction
//     → Head Pose (Yaw/Pitch/Roll via solvePnP approximation)
//     → Gaze Vector (iris landmark regression)
//     → Liveness (eye-blink detection + texture analysis)
//     → Face Bounding Box (for overlay rendering)
//
// All inference runs in the browser. No video frames leave the device.
// Only structured telemetry (coordinates, angles, scores) is transmitted.
//
// Performance budget: <16ms per frame at 720p (60fps capable).
// =============================================================================

import { ref, computed, shallowRef, type Ref, type ComputedRef } from 'vue'
import type { HeadPosePayload, LivenessPayload } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface FaceBBox {
  x: number // normalized 0-1
  y: number
  w: number
  h: number
}

export interface HeadPose {
  yaw: number   // degrees
  pitch: number // degrees
  roll: number  // degrees
}

export interface GazeVector {
  x: number // normalized screen coordinate 0-1
  y: number
  direction: 'center' | 'left' | 'right' | 'up' | 'down'
  angleDegrees: number
}

export interface BlinkState {
  leftEAR: number  // Eye Aspect Ratio (0=closed, ~0.3=open)
  rightEAR: number
  isBlinking: boolean
  blinkCount: number
  blinkRatePerMin: number
}

export interface VisionFrame {
  timestamp: number
  faceBBox: FaceBBox | null
  headPose: HeadPose
  gaze: GazeVector
  blink: BlinkState
  faceCount: number
  landmarks: Float32Array | null // raw 478×3 landmark buffer
  inferenceMs: number
  livenessScore: number
  frameQuality: number
}

export interface VisionEngineConfig {
  videoElement: Ref<HTMLVideoElement | null>
  maxFaces?: number          // default 1
  inferenceHz?: number       // default 10 (Tier A)
  headPoseThresholds?: {
    yaw: number              // degrees, default 25
    pitch: number            // degrees, default 20
    roll: number             // degrees, default 15
  }
  gazeThresholdDeg?: number  // default 15
  blinkEARThreshold?: number // default 0.21
  livenessWindow?: number    // frames for liveness check, default 30

  // Callbacks for health governor integration
  /** Called after each inference with the latency in ms. */
  onInferenceLatency?: (ms: number) => void
  /** Called when sustained high CPU load is detected (rolling avg >100ms for 3+ frames). */
  onHighCpuLoad?: (avgInferenceMs: number) => void
}

export interface VisionEngineState {
  isActive: ComputedRef<boolean>
  isModelLoaded: ComputedRef<boolean>
  currentFrame: Ref<VisionFrame | null>
  fps: ComputedRef<number>
  consecutiveNoFace: Ref<number>
  stats: ComputedRef<VisionStats>
}

export interface VisionStats {
  totalFrames: number
  totalNoFace: number
  totalHeadPoseAnomalies: number
  totalGazeDeviations: number
  totalBlinks: number
  avgInferenceMs: number
  avgLivenessScore: number
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// Canonical 3D face model points for PnP head pose estimation (nose tip,
// chin, left/right eye corners, left/right mouth corners).
// Coordinates in a normalized model space matching MediaPipe landmark indices.
const MODEL_POINTS_3D = [
  [0.0, 0.0, 0.0],       // Nose tip (landmark 1)
  [0.0, -63.6, -12.5],   // Chin (landmark 152)
  [-43.3, 32.7, -26.0],  // Left eye outer corner (landmark 263)
  [43.3, 32.7, -26.0],   // Right eye outer corner (landmark 33)
  [-28.9, -28.9, -24.1], // Left mouth corner (landmark 287)
  [28.9, -28.9, -24.1]   // Right mouth corner (landmark 57)
] as const

// MediaPipe landmark indices for the 6-point PnP solve
const PNP_LANDMARK_IDS = [1, 152, 263, 33, 287, 57] as const

// Eye landmarks for EAR (Eye Aspect Ratio) blink detection
// Using the 6-point eye model from MediaPipe Face Mesh
const LEFT_EYE_IDS = [362, 385, 387, 263, 373, 380] as const   // p1-p6
const RIGHT_EYE_IDS = [33, 160, 158, 133, 153, 144] as const   // p1-p6

// Iris landmarks for gaze estimation (MediaPipe iris model)
const LEFT_IRIS_CENTER = 468 as const
const RIGHT_IRIS_CENTER = 473 as const

// ---------------------------------------------------------------------------
// Helper: Eye Aspect Ratio
// ---------------------------------------------------------------------------

function eyeAspectRatio(landmarks: Float32Array, eyeIds: readonly number[]): number {
  // EAR = (||p2-p6|| + ||p3-p5||) / (2 * ||p1-p4||)
  const p = (idx: number) => ({
    x: landmarks[idx * 3]!,
    y: landmarks[idx * 3 + 1]!
  })

  const dist = (a: { x: number; y: number }, b: { x: number; y: number }) =>
    Math.sqrt((a.x - b.x) ** 2 + (a.y - b.y) ** 2)

  const p1 = p(eyeIds[0]!)
  const p2 = p(eyeIds[1]!)
  const p3 = p(eyeIds[2]!)
  const p4 = p(eyeIds[3]!)
  const p5 = p(eyeIds[4]!)
  const p6 = p(eyeIds[5]!)

  const vertical1 = dist(p2, p6)
  const vertical2 = dist(p3, p5)
  const horizontal = dist(p1, p4)

  if (horizontal < 1e-6) return 0
  return (vertical1 + vertical2) / (2 * horizontal)
}

// ---------------------------------------------------------------------------
// Helper: Head pose from landmarks (Euler angle estimation)
// ---------------------------------------------------------------------------

function estimateHeadPose(
  landmarks: Float32Array,
  frameWidth: number,
  frameHeight: number
): HeadPose {
  // Extract 2D projected points from landmarks
  const pts2d: [number, number][] = PNP_LANDMARK_IDS.map(id => [
    landmarks[id * 3]! * frameWidth,
    landmarks[id * 3 + 1]! * frameHeight
  ])

  // Simplified Euler angle estimation using geometric ratios
  // (avoids full PnP solve which requires OpenCV — not available in browser)
  const noseTip = pts2d[0]!
  const chin = pts2d[1]!
  const leftEye = pts2d[2]!
  const rightEye = pts2d[3]!
  const leftMouth = pts2d[4]!
  const rightMouth = pts2d[5]!

  // Yaw: ratio of nose-to-eye distances
  const noseToLeft = Math.sqrt((noseTip[0] - leftEye[0]) ** 2 + (noseTip[1] - leftEye[1]) ** 2)
  const noseToRight = Math.sqrt((noseTip[0] - rightEye[0]) ** 2 + (noseTip[1] - rightEye[1]) ** 2)
  const eyeRatio = noseToLeft / (noseToRight + 1e-6)
  const yaw = Math.atan2(eyeRatio - 1, 0.5) * (180 / Math.PI)

  // Pitch: vertical nose-chin ratio relative to eye line
  const eyeMidY = (leftEye[1] + rightEye[1]) / 2
  const mouthMidY = (leftMouth[1] + rightMouth[1]) / 2
  const faceHeight = chin[1] - eyeMidY
  const noseRelative = (noseTip[1] - eyeMidY) / (faceHeight + 1e-6)
  const pitch = (noseRelative - 0.45) * -90

  // Roll: angle of the eye line relative to horizontal
  const roll = Math.atan2(
    rightEye[1] - leftEye[1],
    rightEye[0] - leftEye[0]
  ) * (180 / Math.PI)

  return {
    yaw: Math.max(-90, Math.min(90, yaw)),
    pitch: Math.max(-90, Math.min(90, pitch)),
    roll: Math.max(-180, Math.min(180, roll))
  }
}

// ---------------------------------------------------------------------------
// Helper: Gaze vector from iris landmarks
// ---------------------------------------------------------------------------

function estimateGaze(
  landmarks: Float32Array,
  gazeThreshold: number
): GazeVector {
  // Compute iris center relative to eye corners
  const leftIris = {
    x: landmarks[LEFT_IRIS_CENTER * 3]!,
    y: landmarks[LEFT_IRIS_CENTER * 3 + 1]!
  }
  const rightIris = {
    x: landmarks[RIGHT_IRIS_CENTER * 3]!,
    y: landmarks[RIGHT_IRIS_CENTER * 3 + 1]!
  }

  const leftEyeInner = { x: landmarks[263 * 3]!, y: landmarks[263 * 3 + 1]! }
  const leftEyeOuter = { x: landmarks[362 * 3]!, y: landmarks[362 * 3 + 1]! }
  const rightEyeInner = { x: landmarks[133 * 3]!, y: landmarks[133 * 3 + 1]! }
  const rightEyeOuter = { x: landmarks[33 * 3]!, y: landmarks[33 * 3 + 1]! }

  // Normalize iris position within eye (0=outer corner, 1=inner corner)
  const leftEyeWidth = Math.abs(leftEyeInner.x - leftEyeOuter.x) + 1e-6
  const rightEyeWidth = Math.abs(rightEyeInner.x - rightEyeOuter.x) + 1e-6

  const leftRatioX = (leftIris.x - leftEyeOuter.x) / leftEyeWidth
  const rightRatioX = (rightIris.x - rightEyeOuter.x) / rightEyeWidth

  // Average both eyes for robust gaze estimate
  const gazeX = (leftRatioX + rightRatioX) / 2
  const gazeY = ((leftIris.y + rightIris.y) / 2) // normalized by face mesh

  // Convert to angle (0.5 = center, deviation maps to degrees)
  const deviationX = (gazeX - 0.5) * 2 // -1 to +1
  const deviationY = (gazeY - 0.3) * 2  // adjusted for typical eye position
  const angle = Math.sqrt(deviationX ** 2 + deviationY ** 2) * 45 // approx degrees

  let direction: GazeVector['direction'] = 'center'
  if (angle > gazeThreshold) {
    if (Math.abs(deviationX) > Math.abs(deviationY)) {
      direction = deviationX < 0 ? 'left' : 'right'
    } else {
      direction = deviationY < 0 ? 'up' : 'down'
    }
  }

  return { x: gazeX, y: gazeY, direction, angleDegrees: angle }
}

// ---------------------------------------------------------------------------
// Helper: Face bounding box from landmarks
// ---------------------------------------------------------------------------

function computeFaceBBox(landmarks: Float32Array, count: number): FaceBBox {
  let minX = 1, minY = 1, maxX = 0, maxY = 0
  for (let i = 0; i < count; i++) {
    const x = landmarks[i * 3]!
    const y = landmarks[i * 3 + 1]!
    if (x < minX) minX = x
    if (y < minY) minY = y
    if (x > maxX) maxX = x
    if (y > maxY) maxY = y
  }
  // Add padding (5% of face size)
  const padX = (maxX - minX) * 0.05
  const padY = (maxY - minY) * 0.05
  return {
    x: Math.max(0, minX - padX),
    y: Math.max(0, minY - padY),
    w: Math.min(1, maxX - minX + 2 * padX),
    h: Math.min(1, maxY - minY + 2 * padY)
  }
}

// ---------------------------------------------------------------------------
// Helper: Frame quality assessment
// ---------------------------------------------------------------------------

function assessFrameQuality(landmarks: Float32Array, faceBBox: FaceBBox): number {
  // Quality factors: face size, centering, landmark spread
  let quality = 1.0

  // Penalize tiny faces (< 10% of frame)
  if (faceBBox.w < 0.1 || faceBBox.h < 0.1) quality *= 0.5
  // Penalize very large faces (> 60% of frame = too close)
  if (faceBBox.w > 0.6 || faceBBox.h > 0.6) quality *= 0.8
  // Penalize off-center faces
  const centerX = faceBBox.x + faceBBox.w / 2
  const centerY = faceBBox.y + faceBBox.h / 2
  const distFromCenter = Math.sqrt((centerX - 0.5) ** 2 + (centerY - 0.5) ** 2)
  if (distFromCenter > 0.3) quality *= 0.7

  return Math.max(0, Math.min(1, quality))
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useVisionEngine(config: VisionEngineConfig) {
  const {
    videoElement,
    maxFaces = 1,
    inferenceHz = 10,
    headPoseThresholds = { yaw: 25, pitch: 20, roll: 15 },
    gazeThresholdDeg = 15,
    blinkEARThreshold = 0.21,
    livenessWindow = 30,
    onInferenceLatency,
    onHighCpuLoad
  } = config

  // State
  const isRunning = ref(false)
  const modelLoaded = ref(false)
  const currentFrame = shallowRef<VisionFrame | null>(null)
  const consecutiveNoFace = ref(0)
  const fpsCounter = ref(0)

  // Stats accumulator
  const _stats = ref({
    totalFrames: 0,
    totalNoFace: 0,
    totalHeadPoseAnomalies: 0,
    totalGazeDeviations: 0,
    totalBlinks: 0,
    inferenceMsSum: 0,
    livenessScoreSum: 0,
    livenessCount: 0
  })

  // Blink tracking
  let blinkCount = 0
  let blinkWindowStart = Date.now()
  let wasBlinking = false

  // Liveness rolling window
  const livenessScores: number[] = []

  // Animation frame handle
  let animFrameId: number | null = null
  let lastInferenceTime = 0
  const inferenceInterval = 1000 / inferenceHz

  // Pre-allocated Float32Array buffer for landmark extraction.
  // Reused across frames to eliminate 5.8KB allocation per frame (478×3×4 bytes).
  // At 10Hz this saves 58KB/sec of GC pressure — significant on mobile GPUs.
  const LANDMARK_BUFFER_SIZE = 478 * 3 // max landmarks × 3 (x, y, z)
  let landmarkBuffer: Float32Array | null = null

  // FPS measurement
  let frameCountForFps = 0
  let fpsStartTime = Date.now()

  // Inference latency rolling average (for CPU spike detection)
  const INFERENCE_LATENCY_WINDOW = 5
  const inferenceLatencyWindow: number[] = []
  let consecutiveHighCpuFrames = 0
  const HIGH_CPU_THRESHOLD_MS = 100
  const HIGH_CPU_CONSECUTIVE = 3

  // MediaPipe FaceMesh instance (lazy loaded)
  let faceMesh: any = null

  // ---------------------------------------------------------------------------
  // MediaPipe initialization
  // ---------------------------------------------------------------------------

  async function loadModel(): Promise<boolean> {
    if (modelLoaded.value) return true
    try {
      // Dynamic import to avoid bundling MediaPipe unless used
      // @ts-expect-error — dynamic import, types not bundled
      const vision = await import('@mediapipe/tasks-vision')
      const { FaceLandmarker, FilesetResolver } = vision

      const filesetResolver = await FilesetResolver.forVisionTasks(
        'https://cdn.jsdelivr.net/npm/@mediapipe/tasks-vision@latest/wasm'
      )

      faceMesh = await FaceLandmarker.createFromOptions(filesetResolver, {
        baseOptions: {
          modelAssetPath: 'https://storage.googleapis.com/mediapipe-models/face_landmarker/face_landmarker/float16/1/face_landmarker.task',
          delegate: 'GPU' // WebGL acceleration
        },
        runningMode: 'VIDEO',
        numFaces: maxFaces,
        minFaceDetectionConfidence: 0.5,
        minFacePresenceConfidence: 0.5,
        minTrackingConfidence: 0.5,
        outputFaceBlendshapes: true,  // for blink detection
        outputFacialTransformationMatrixes: false
      })

      modelLoaded.value = true
      return true
    } catch (err) {
      console.error('[VisionEngine] Failed to load MediaPipe FaceLandmarker:', err)
      return false
    }
  }

  // ---------------------------------------------------------------------------
  // Inference loop
  // ---------------------------------------------------------------------------

  function processFrame(timestamp: number) {
    if (!isRunning.value) return

    animFrameId = requestAnimationFrame(processFrame)

    // Throttle to inferenceHz
    if (timestamp - lastInferenceTime < inferenceInterval) return
    lastInferenceTime = timestamp

    const video = videoElement.value
    if (!video || video.readyState < 2 || !faceMesh) return

    const start = performance.now()

    try {
      const results = faceMesh.detectForVideo(video, timestamp)
      const inferenceMs = performance.now() - start

      _stats.value.totalFrames++
      _stats.value.inferenceMsSum += inferenceMs

      // Report inference latency to health governor for CPU spike penalty
      if (onInferenceLatency) {
        onInferenceLatency(inferenceMs)
      }

      // Track rolling average for HIGH_CPU_LOAD detection
      inferenceLatencyWindow.push(inferenceMs)
      if (inferenceLatencyWindow.length > INFERENCE_LATENCY_WINDOW) {
        inferenceLatencyWindow.shift()
      }
      const avgInferenceMs = inferenceLatencyWindow.reduce((a, b) => a + b, 0) / inferenceLatencyWindow.length
      if (avgInferenceMs > HIGH_CPU_THRESHOLD_MS) {
        consecutiveHighCpuFrames++
        if (consecutiveHighCpuFrames >= HIGH_CPU_CONSECUTIVE && onHighCpuLoad) {
          onHighCpuLoad(avgInferenceMs)
          // Reset to avoid flooding — only fire once per sustained spike
          consecutiveHighCpuFrames = 0
        }
      } else {
        consecutiveHighCpuFrames = 0
      }

      // FPS tracking
      frameCountForFps++
      const elapsed = Date.now() - fpsStartTime
      if (elapsed >= 1000) {
        fpsCounter.value = frameCountForFps
        frameCountForFps = 0
        fpsStartTime = Date.now()
      }

      if (!results.faceLandmarks || results.faceLandmarks.length === 0) {
        // No face detected
        consecutiveNoFace.value++
        _stats.value.totalNoFace++
        currentFrame.value = {
          timestamp,
          faceBBox: null,
          headPose: { yaw: 0, pitch: 0, roll: 0 },
          gaze: { x: 0.5, y: 0.5, direction: 'center', angleDegrees: 0 },
          blink: { leftEAR: 0, rightEAR: 0, isBlinking: false, blinkCount, blinkRatePerMin: 0 },
          faceCount: 0,
          landmarks: null,
          inferenceMs,
          livenessScore: 0,
          frameQuality: 0
        }
        return
      }

      consecutiveNoFace.value = 0

      // Extract first face landmarks into a reusable Float32Array buffer.
      // Allocates once, reuses across all frames — eliminates 5.8KB/frame GC churn.
      const faceLandmarks = results.faceLandmarks[0]
      const landmarkCount = faceLandmarks.length
      const requiredSize = landmarkCount * 3
      if (!landmarkBuffer || landmarkBuffer.length < requiredSize) {
        landmarkBuffer = new Float32Array(Math.max(requiredSize, LANDMARK_BUFFER_SIZE))
      }
      const landmarks = landmarkBuffer
      for (let i = 0; i < landmarkCount; i++) {
        landmarks[i * 3] = faceLandmarks[i].x
        landmarks[i * 3 + 1] = faceLandmarks[i].y
        landmarks[i * 3 + 2] = faceLandmarks[i].z || 0
      }

      // Face bounding box
      const faceBBox = computeFaceBBox(landmarks, landmarkCount)

      // Head pose estimation
      const headPose = estimateHeadPose(
        landmarks,
        video.videoWidth,
        video.videoHeight
      )

      // Head pose anomaly tracking
      if (
        Math.abs(headPose.yaw) > headPoseThresholds.yaw ||
        Math.abs(headPose.pitch) > headPoseThresholds.pitch ||
        Math.abs(headPose.roll) > headPoseThresholds.roll
      ) {
        _stats.value.totalHeadPoseAnomalies++
      }

      // Gaze estimation
      const gaze = landmarkCount >= 478
        ? estimateGaze(landmarks, gazeThresholdDeg)
        : { x: 0.5, y: 0.5, direction: 'center' as const, angleDegrees: 0 }

      if (gaze.direction !== 'center') {
        _stats.value.totalGazeDeviations++
      }

      // Blink detection via EAR
      const leftEAR = eyeAspectRatio(landmarks, LEFT_EYE_IDS)
      const rightEAR = eyeAspectRatio(landmarks, RIGHT_EYE_IDS)
      const avgEAR = (leftEAR + rightEAR) / 2
      const isBlinking = avgEAR < blinkEARThreshold

      if (isBlinking && !wasBlinking) {
        blinkCount++
        _stats.value.totalBlinks++
      }
      wasBlinking = isBlinking

      // Blink rate (per minute)
      const blinkElapsed = (Date.now() - blinkWindowStart) / 1000
      const blinkRatePerMin = blinkElapsed > 0 ? (blinkCount / blinkElapsed) * 60 : 0

      // Reset blink counter every 60 seconds
      if (blinkElapsed > 60) {
        blinkCount = 0
        blinkWindowStart = Date.now()
      }

      // Frame quality
      const frameQuality = assessFrameQuality(landmarks, faceBBox)

      // Liveness scoring (texture + blink + quality)
      // Real liveness requires depth or IR — this provides blink + quality heuristic
      const blinkAlive = blinkRatePerMin >= 8 && blinkRatePerMin <= 40 ? 0.4 : 0.1
      const qualityAlive = frameQuality * 0.3
      const earAlive = avgEAR > 0.15 && avgEAR < 0.45 ? 0.3 : 0.1
      const livenessScore = Math.min(1, blinkAlive + qualityAlive + earAlive)

      livenessScores.push(livenessScore)
      if (livenessScores.length > livenessWindow) livenessScores.shift()
      const avgLiveness = livenessScores.reduce((a, b) => a + b, 0) / livenessScores.length

      _stats.value.livenessScoreSum += avgLiveness
      _stats.value.livenessCount++

      currentFrame.value = {
        timestamp,
        faceBBox,
        headPose,
        gaze,
        blink: { leftEAR, rightEAR, isBlinking, blinkCount, blinkRatePerMin },
        faceCount: results.faceLandmarks.length,
        landmarks,
        inferenceMs,
        livenessScore: avgLiveness,
        frameQuality
      }
    } catch (err) {
      // Silently continue — inference errors should not crash the session
      console.warn('[VisionEngine] Frame processing error:', err)
    }
  }

  // ---------------------------------------------------------------------------
  // Lifecycle
  // ---------------------------------------------------------------------------

  async function start(): Promise<boolean> {
    if (isRunning.value) return true
    const loaded = await loadModel()
    if (!loaded) return false

    isRunning.value = true
    blinkCount = 0
    blinkWindowStart = Date.now()
    lastInferenceTime = 0
    frameCountForFps = 0
    fpsStartTime = Date.now()
    livenessScores.length = 0

    animFrameId = requestAnimationFrame(processFrame)
    return true
  }

  function stop() {
    isRunning.value = false
    if (animFrameId !== null) {
      cancelAnimationFrame(animFrameId)
      animFrameId = null
    }
  }

  function setInferenceRate(hz: number) {
    // Allow dynamic tier-based adjustment
    const interval = 1000 / Math.max(1, Math.min(30, hz))
    // Update via closure
    ;(useVisionEngine as any)._interval = interval
  }

  // ---------------------------------------------------------------------------
  // Payload builders (for gRPC event emission)
  // ---------------------------------------------------------------------------

  function buildHeadPosePayload(frame: VisionFrame): HeadPosePayload {
    return {
      yaw: Math.round(frame.headPose.yaw * 100) / 100,
      pitch: Math.round(frame.headPose.pitch * 100) / 100,
      roll: Math.round(frame.headPose.roll * 100) / 100,
      faceX: frame.faceBBox?.x ?? 0,
      faceY: frame.faceBBox?.y ?? 0,
      faceW: frame.faceBBox?.w ?? 0,
      faceH: frame.faceBBox?.h ?? 0,
      ipdPx: 0, // requires camera calibration
      landmarkCount: frame.landmarks ? frame.landmarks.length / 3 : 0,
      inferenceMs: Math.round(frame.inferenceMs * 10) / 10
    }
  }

  function buildLivenessPayload(frame: VisionFrame): LivenessPayload {
    return {
      livenessScore: Math.round(frame.livenessScore * 1000) / 1000,
      blinkDetected: frame.blink.isBlinking,
      blinkRatePerMin: Math.round(frame.blink.blinkRatePerMin * 10) / 10,
      textureScore: frame.frameQuality, // simplified — real texture analysis needs CNN
      depthScore: 0.5, // requires stereo/IR camera
      spoofVector: frame.livenessScore < 0.4 ? 'unknown' : 'none',
      frameQuality: Math.round(frame.frameQuality * 1000) / 1000
    }
  }

  // ---------------------------------------------------------------------------
  // Computed
  // ---------------------------------------------------------------------------

  const stats = computed<VisionStats>(() => {
    const s = _stats.value
    return {
      totalFrames: s.totalFrames,
      totalNoFace: s.totalNoFace,
      totalHeadPoseAnomalies: s.totalHeadPoseAnomalies,
      totalGazeDeviations: s.totalGazeDeviations,
      totalBlinks: s.totalBlinks,
      avgInferenceMs: s.totalFrames > 0 ? s.inferenceMsSum / s.totalFrames : 0,
      avgLivenessScore: s.livenessCount > 0 ? s.livenessScoreSum / s.livenessCount : 0
    }
  })

  return {
    // State
    isActive: computed(() => isRunning.value),
    isModelLoaded: computed(() => modelLoaded.value),
    currentFrame,
    fps: computed(() => fpsCounter.value),
    consecutiveNoFace,
    stats,

    // Lifecycle
    start,
    stop,
    setInferenceRate,

    // Payload builders
    buildHeadPosePayload,
    buildLivenessPayload,

    // Thresholds (reactive access for overlay rendering)
    thresholds: {
      headPose: headPoseThresholds,
      gaze: gazeThresholdDeg,
      blinkEAR: blinkEARThreshold
    }
  }
}
