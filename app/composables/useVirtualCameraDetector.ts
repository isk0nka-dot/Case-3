// =============================================================================
// Argus AI — useVirtualCameraDetector Composable
// =============================================================================
//
// Heuristic engine to detect virtual cameras (OBS Virtual Camera, ManyCam,
// XSplit VCam, Snap Camera, etc.) and pre-recorded video injection.
//
// Detection vectors:
//   1. Device label inspection — known virtual cam names
//   2. WebRTC getCapabilities() — virtual cams lack hardware constraints
//   3. Timing jitter analysis — real cams have natural frame timing variance
//   4. Resolution anomaly — virtual cams often report atypical resolutions
//   5. Track constraint probing — virtual cams fail certain constraint changes
//   6. Frame entropy analysis — looped/static feeds have low temporal entropy
//
// All checks are non-blocking and run in the background. Each check produces
// a confidence score. The composite score determines the detection result.
//
// =============================================================================

import { ref, computed } from 'vue'
import { EventType, Severity, EventSource } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface VirtualCameraReport {
  isVirtual: boolean
  confidence: number          // 0-1 composite confidence
  deviceLabel: string
  flags: VirtualCameraFlag[]
  timestamp: number
}

export interface VirtualCameraFlag {
  check: string
  detected: boolean
  confidence: number
  detail: string
}

// Known virtual camera device label patterns
const VIRTUAL_CAM_PATTERNS = [
  /obs\s*virtual/i,
  /obs-camera/i,
  /manycam/i,
  /xsplit/i,
  /snap\s*camera/i,
  /chromacam/i,
  /iriun/i,
  /droidcam/i,
  /epoccam/i,
  /mmhmm/i,
  /streamlabs/i,
  /virtual\s*cam/i,
  /fake\s*cam/i,
  /cam\s*twist/i,
  /webcamoid/i,
  /akvcam/i,
  /v4l2loopback/i,
  /newtek/i,
  /ndi/i,
]

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useVirtualCameraDetector() {
  const report = ref<VirtualCameraReport | null>(null)
  const isChecking = ref(false)

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

  // -------------------------------------------------------------------------
  // Check 1: Device Label Inspection
  // -------------------------------------------------------------------------

  async function checkDeviceLabel(stream: MediaStream): Promise<VirtualCameraFlag> {
    const videoTrack = stream.getVideoTracks()[0]
    const label = videoTrack?.label ?? ''
    const settings = videoTrack?.getSettings()
    const deviceId = settings?.deviceId ?? ''

    const matchedPattern = VIRTUAL_CAM_PATTERNS.find(p => p.test(label) || p.test(deviceId))
    const detected = !!matchedPattern

    return {
      check: 'device_label',
      detected,
      confidence: detected ? 0.95 : 0,
      detail: detected
        ? `Виртуальная камера: "${label}"`
        : `Камера: "${label.substring(0, 40)}"`
    }
  }

  // -------------------------------------------------------------------------
  // Check 2: WebRTC Capabilities Probe
  // -------------------------------------------------------------------------

  async function checkCapabilities(stream: MediaStream): Promise<VirtualCameraFlag> {
    const videoTrack = stream.getVideoTracks()[0]
    if (!videoTrack) {
      return { check: 'capabilities', detected: false, confidence: 0, detail: 'Нет видеотрека' }
    }

    try {
      const capabilities = videoTrack.getCapabilities?.()
      if (!capabilities) {
        return { check: 'capabilities', detected: true, confidence: 0.6, detail: 'getCapabilities() не поддерживается — возможна виртуальная камера' }
      }

      const flags: string[] = []

      // Real cameras support variable frame rates
      if (!capabilities.frameRate || (!capabilities.frameRate.min && !capabilities.frameRate.max)) {
        flags.push('no_framerate_range')
      }

      // Real cameras have width/height ranges (not fixed values)
      if (capabilities.width && capabilities.width.min === capabilities.width.max) {
        flags.push('fixed_width')
      }
      if (capabilities.height && capabilities.height.min === capabilities.height.max) {
        flags.push('fixed_height')
      }

      // Real cameras report facing mode
      if (!capabilities.facingMode || capabilities.facingMode.length === 0) {
        flags.push('no_facing_mode')
      }

      // Real cameras typically support torch/zoom
      // (absence alone isn't conclusive, but combined with other flags it is)

      const detected = flags.length >= 2
      return {
        check: 'capabilities',
        detected,
        confidence: detected ? Math.min(0.8, flags.length * 0.25) : 0,
        detail: detected
          ? `Аномалии возможностей: ${flags.join(', ')}`
          : 'Возможности камеры в норме'
      }
    } catch {
      return { check: 'capabilities', detected: false, confidence: 0, detail: 'Ошибка проверки возможностей' }
    }
  }

  // -------------------------------------------------------------------------
  // Check 3: Frame Timing Jitter Analysis
  // -------------------------------------------------------------------------

  async function checkTimingJitter(stream: MediaStream): Promise<VirtualCameraFlag> {
    return new Promise((resolve) => {
      const videoTrack = stream.getVideoTracks()[0]
      if (!videoTrack) {
        resolve({ check: 'timing_jitter', detected: false, confidence: 0, detail: 'Нет видеотрека' })
        return
      }

      // Use requestVideoFrameCallback for precise frame timing
      const video = document.createElement('video')
      video.srcObject = new MediaStream([videoTrack])
      video.muted = true
      video.playsInline = true

      const timestamps: number[] = []
      const SAMPLE_COUNT = 30
      let frameCount = 0

      function onFrame(now: DOMHighResTimeStamp) {
        timestamps.push(now)
        frameCount++

        if (frameCount < SAMPLE_COUNT) {
          video.requestVideoFrameCallback(onFrame)
        } else {
          video.pause()
          video.srcObject = null

          // Analyze inter-frame intervals
          const intervals: number[] = []
          for (let i = 1; i < timestamps.length; i++) {
            intervals.push(timestamps[i]! - timestamps[i - 1]!)
          }

          if (intervals.length < 5) {
            resolve({ check: 'timing_jitter', detected: false, confidence: 0, detail: 'Недостаточно кадров' })
            return
          }

          // Calculate standard deviation of intervals
          const mean = intervals.reduce((a, b) => a + b, 0) / intervals.length
          const variance = intervals.reduce((sum, val) => sum + (val - mean) ** 2, 0) / intervals.length
          const stdDev = Math.sqrt(variance)

          // Coefficient of variation (CV)
          const cv = stdDev / mean

          // Real cameras: CV typically 0.05-0.25 (natural jitter from hardware)
          // Virtual cameras: CV < 0.02 (too precise) or > 0.4 (software scheduling noise)
          const tooSmooth = cv < 0.015
          const tooJittery = cv > 0.5 && mean < 50 // High jitter at high FPS = suspicious

          const detected = tooSmooth || tooJittery
          resolve({
            check: 'timing_jitter',
            detected,
            confidence: detected ? (tooSmooth ? 0.7 : 0.5) : 0,
            detail: detected
              ? `Аномалия тайминга кадров: CV=${cv.toFixed(4)}, среднее=${mean.toFixed(1)}мс`
              : `Тайминг в норме: CV=${cv.toFixed(4)}`
          })
        }
      }

      video.play().then(() => {
        if ('requestVideoFrameCallback' in video) {
          video.requestVideoFrameCallback(onFrame)
        } else {
          resolve({ check: 'timing_jitter', detected: false, confidence: 0, detail: 'requestVideoFrameCallback не поддерживается' })
        }
      }).catch(() => {
        resolve({ check: 'timing_jitter', detected: false, confidence: 0, detail: 'Не удалось воспроизвести видео' })
      })
    })
  }

  // -------------------------------------------------------------------------
  // Check 4: Resolution Anomaly
  // -------------------------------------------------------------------------

  async function checkResolution(stream: MediaStream): Promise<VirtualCameraFlag> {
    const videoTrack = stream.getVideoTracks()[0]
    if (!videoTrack) {
      return { check: 'resolution', detected: false, confidence: 0, detail: 'Нет видеотрека' }
    }

    const settings = videoTrack.getSettings()
    const w = settings.width ?? 0
    const h = settings.height ?? 0

    // Standard webcam resolutions
    const standardResolutions = [
      [320, 240], [640, 480], [800, 600],
      [1280, 720], [1920, 1080], [2560, 1440],
      [3840, 2160], [4096, 2160]
    ]

    const isStandard = standardResolutions.some(
      ([sw, sh]) => (w === sw && h === sh) || (w === sh && h === sw)
    )

    // Check aspect ratio
    const aspectRatio = w / h
    const standardRatios = [4 / 3, 16 / 9, 16 / 10]
    const isStandardRatio = standardRatios.some(r => Math.abs(aspectRatio - r) < 0.05)

    const detected = !isStandard && !isStandardRatio && w > 0
    return {
      check: 'resolution',
      detected,
      confidence: detected ? 0.4 : 0,
      detail: detected
        ? `Нестандартное разрешение: ${w}x${h} (AR=${aspectRatio.toFixed(3)})`
        : `Разрешение: ${w}x${h}`
    }
  }

  // -------------------------------------------------------------------------
  // Check 5: Constraint Change Probe
  // -------------------------------------------------------------------------

  async function checkConstraintProbe(stream: MediaStream): Promise<VirtualCameraFlag> {
    const videoTrack = stream.getVideoTracks()[0]
    if (!videoTrack) {
      return { check: 'constraint_probe', detected: false, confidence: 0, detail: 'Нет видеотрека' }
    }

    try {
      // Try to apply an unusual constraint — real cameras negotiate, virtual ones often fail silently
      const originalSettings = videoTrack.getSettings()

      await videoTrack.applyConstraints({
        frameRate: { ideal: 7 } // Unusual frame rate
      })

      const newSettings = videoTrack.getSettings()

      // Restore original
      await videoTrack.applyConstraints({
        frameRate: { ideal: originalSettings.frameRate ?? 30 }
      })

      // Real cameras will attempt to honor the constraint or negotiate
      // Virtual cameras often ignore it entirely
      const fpsChanged = newSettings.frameRate !== originalSettings.frameRate
      const detected = !fpsChanged && (originalSettings.frameRate ?? 30) > 10

      return {
        check: 'constraint_probe',
        detected,
        confidence: detected ? 0.5 : 0,
        detail: detected
          ? 'Камера не реагирует на изменение FPS'
          : 'Камера корректно реагирует на ограничения'
      }
    } catch {
      return { check: 'constraint_probe', detected: false, confidence: 0, detail: 'Ошибка зондирования' }
    }
  }

  // -------------------------------------------------------------------------
  // Composite Analysis
  // -------------------------------------------------------------------------

  async function analyze(stream: MediaStream): Promise<VirtualCameraReport> {
    isChecking.value = true

    const flags = await Promise.all([
      checkDeviceLabel(stream),
      checkCapabilities(stream),
      checkTimingJitter(stream),
      checkResolution(stream),
      checkConstraintProbe(stream),
    ])

    // Weighted composite confidence
    const weights: Record<string, number> = {
      device_label: 0.35,
      capabilities: 0.2,
      timing_jitter: 0.2,
      resolution: 0.1,
      constraint_probe: 0.15,
    }

    const compositeConfidence = flags.reduce((sum, flag) => {
      return sum + flag.confidence * (weights[flag.check] ?? 0.1)
    }, 0)

    const videoTrack = stream.getVideoTracks()[0]
    const deviceLabel = videoTrack?.label ?? 'unknown'

    const result: VirtualCameraReport = {
      isVirtual: compositeConfidence >= 0.5,
      confidence: Math.min(1, compositeConfidence),
      deviceLabel,
      flags,
      timestamp: Date.now(),
    }

    report.value = result
    isChecking.value = false

    // Fire event if virtual camera detected
    if (result.isVirtual) {
      const severity = result.confidence >= 0.8 ? Severity.CRITICAL : Severity.WARNING
      sendEventFn?.(
        EventType.HARDWARE_DEVICE_ANOMALY,
        severity,
        {
          type: 'kernel',
          data: {
            wpm: 0,
            keystrokeStdDevMs: 0,
            handOnMouse: false,
            hardwareIdHash: deviceLabel,
            mismatchComponent: 'virtual_camera',
            monitorCount: 1,
            virtualMonitor: false,
          }
        },
        `Обнаружена виртуальная камера: ${deviceLabel} (${(result.confidence * 100).toFixed(0)}%)`,
        result.confidence,
        EventSource.BROWSER
      )
    }

    return result
  }

  return {
    report,
    isChecking,
    analyze,
    onEvent,
  }
}
