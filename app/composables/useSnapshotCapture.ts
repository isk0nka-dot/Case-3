// =============================================================================
// Argus AI — useSnapshotCapture Composable
// =============================================================================
//
// Canvas-based JPEG snapshot capture from a video element, controlled by the
// Tier Engine's snapshot configuration.
//
// Capture flow:
//   1. drawImage(video, 0, 0, width, height) onto offscreen <canvas>
//   2. canvas.toBlob(callback, 'image/jpeg', quality) → ArrayBuffer
//   3. crypto.subtle.digest('SHA-256', buffer) → hex hash (tamper evidence)
//   4. Write to IndexedDB via offlineQueue.enqueueSnapshot(blob, meta)
//
// Tier-controlled:
//   - Tier A: snapshots disabled (video handles evidence)
//   - Tier B: every 5s, 480p, 60% JPEG quality
//   - Tier C: every 10s, 240p, 40% JPEG quality
//
// Usage:
//   const snapshotCapture = useSnapshotCapture({
//     videoElement: videoRef,
//     offlineQueue: resilience.offlineQueue,
//     tierConfig: resilience.tierConfig,
//     sessionId: 'sess-123',
//     orgId: 'org-1',
//     examId: 'exam-1',
//     studentId: 'student-1'
//   })
//   snapshotCapture.start()
//   // ... snapshots are automatically captured based on tier config ...
//   snapshotCapture.stop()
//
// =============================================================================

import { ref, watch, type Ref, type ComputedRef } from 'vue'
import { sha256 } from '~/lib/storage/idb'
import type { TierConfig, TierSnapshotConfig } from './useTierEngine'
import type { SnapshotMeta } from './useOfflineQueue'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Configuration for the snapshot capture service. */
export interface SnapshotCaptureConfig {
  /** The video element to capture from. */
  videoElement: Ref<HTMLVideoElement | null>
  /** Offline queue for persisting snapshots. */
  offlineQueue: {
    enqueueSnapshot: (blob: ArrayBuffer, meta: SnapshotMeta) => Promise<number>
  }
  /** Reactive tier configuration from the tier engine. */
  tierConfig: ComputedRef<TierConfig> | Ref<TierConfig>
  /** Proctoring session identifier. */
  sessionId: string
  /** Organization identifier. */
  orgId: string
  /** Exam identifier. */
  examId: string
  /** Student identifier. */
  studentId: string
}

/** Snapshot capture statistics. */
export interface SnapshotStats {
  /** Total snapshots captured this session. */
  totalCaptured: number
  /** Total snapshots successfully enqueued. */
  totalEnqueued: number
  /** Total capture errors. */
  totalErrors: number
  /** Last capture timestamp (epoch ms). */
  lastCaptureAt: number
  /** Current capture interval in ms. */
  currentIntervalMs: number
  /** Whether capturing is active. */
  isActive: boolean
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useSnapshotCapture(config: SnapshotCaptureConfig) {
  // -------------------------------------------------------------------------
  // State
  // -------------------------------------------------------------------------

  const isCapturing = ref(false)
  const stats: Ref<SnapshotStats> = ref({
    totalCaptured: 0,
    totalEnqueued: 0,
    totalErrors: 0,
    lastCaptureAt: 0,
    currentIntervalMs: 0,
    isActive: false
  })

  // Internal
  let captureTimer: ReturnType<typeof setInterval> | null = null
  let canvas: HTMLCanvasElement | null = null
  let canvasCtx: CanvasRenderingContext2D | null = null

  // -------------------------------------------------------------------------
  // Canvas Management
  // -------------------------------------------------------------------------

  /** Get or create the offscreen canvas at the target resolution. */
  function getCanvas(width: number, height: number): { canvas: HTMLCanvasElement, ctx: CanvasRenderingContext2D } | null {
    if (!canvas) {
      canvas = document.createElement('canvas')
      canvasCtx = canvas.getContext('2d')
    }

    if (!canvasCtx) {
      console.error('[argus:snapshot] Failed to get 2D canvas context')
      return null
    }

    // Resize canvas if needed.
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width
      canvas.height = height
    }

    return { canvas, ctx: canvasCtx }
  }

  // -------------------------------------------------------------------------
  // Capture Logic
  // -------------------------------------------------------------------------

  /**
   * Capture a single JPEG snapshot from the video element.
   * Computes SHA-256 and enqueues to IndexedDB.
   */
  async function captureSnapshot(): Promise<void> {
    const video = config.videoElement.value
    if (!video) return
    if (video.readyState < 2) return // HAVE_CURRENT_DATA minimum
    if (video.videoWidth === 0 || video.videoHeight === 0) return

    const snapshotConfig = config.tierConfig.value.snapshot
    if (!snapshotConfig.enabled) return

    const { width, height } = snapshotConfig

    try {
      // Step 1: Draw video frame onto offscreen canvas.
      const canvasResult = getCanvas(width, height)
      if (!canvasResult) return

      canvasResult.ctx.drawImage(video, 0, 0, width, height)

      // Step 2: Export as JPEG blob.
      const blob = await new Promise<Blob | null>((resolve) => {
        canvasResult.canvas.toBlob(resolve, 'image/jpeg', snapshotConfig.quality)
      })

      if (!blob) {
        stats.value = { ...stats.value, totalErrors: stats.value.totalErrors + 1 }
        return
      }

      // Step 3: Convert to ArrayBuffer.
      const arrayBuffer = await blob.arrayBuffer()

      // Step 4: Compute SHA-256 (tamper evidence).
      const hash = await sha256(arrayBuffer)

      // Step 5: Enqueue to IndexedDB.
      const meta: SnapshotMeta = {
        sessionId: config.sessionId,
        capturedAt: Date.now(),
        resolution: snapshotConfig.resolution,
        quality: snapshotConfig.quality,
        orgId: config.orgId,
        examId: config.examId,
        studentId: config.studentId
      }

      const id = await config.offlineQueue.enqueueSnapshot(arrayBuffer, meta)

      if (id >= 0) {
        stats.value = {
          ...stats.value,
          totalCaptured: stats.value.totalCaptured + 1,
          totalEnqueued: stats.value.totalEnqueued + 1,
          lastCaptureAt: Date.now()
        }
      } else {
        stats.value = {
          ...stats.value,
          totalCaptured: stats.value.totalCaptured + 1,
          totalErrors: stats.value.totalErrors + 1
        }
      }
    } catch (err) {
      console.error('[argus:snapshot] Capture failed:', err)
      stats.value = { ...stats.value, totalErrors: stats.value.totalErrors + 1 }
    }
  }

  // -------------------------------------------------------------------------
  // Interval Management
  // -------------------------------------------------------------------------

  /** Start or restart the capture interval based on current tier config. */
  function applyInterval(snapshotConfig: TierSnapshotConfig): void {
    // Clear existing timer.
    if (captureTimer) {
      clearInterval(captureTimer)
      captureTimer = null
    }

    if (!snapshotConfig.enabled || snapshotConfig.intervalMs <= 0) {
      stats.value = {
        ...stats.value,
        currentIntervalMs: 0,
        isActive: false
      }
      return
    }

    // Set new timer.
    captureTimer = setInterval(() => {
      void captureSnapshot()
    }, snapshotConfig.intervalMs)

    stats.value = {
      ...stats.value,
      currentIntervalMs: snapshotConfig.intervalMs,
      isActive: true
    }

    console.debug('[argus:snapshot] Capture interval set', {
      intervalMs: snapshotConfig.intervalMs,
      resolution: `${snapshotConfig.width}x${snapshotConfig.height}`,
      quality: snapshotConfig.quality
    })
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Start the snapshot capture service.
   * Listens to tier config changes and adjusts capture interval automatically.
   */
  function start(): void {
    if (isCapturing.value) return

    isCapturing.value = true

    // Apply initial interval.
    applyInterval(config.tierConfig.value.snapshot)

    // Watch for tier config changes.
    watch(
      () => config.tierConfig.value.snapshot,
      (newConfig) => {
        if (isCapturing.value) {
          applyInterval(newConfig)
        }
      },
      { deep: true }
    )

    console.info('[argus:snapshot] Snapshot capture started', {
      sessionId: config.sessionId,
      enabled: config.tierConfig.value.snapshot.enabled,
      intervalMs: config.tierConfig.value.snapshot.intervalMs
    })
  }

  /**
   * Stop the snapshot capture service.
   */
  function stop(): void {
    if (!isCapturing.value) return

    isCapturing.value = false

    if (captureTimer) {
      clearInterval(captureTimer)
      captureTimer = null
    }

    // Release canvas resources.
    if (canvas) {
      canvas.width = 0
      canvas.height = 0
      canvas = null
      canvasCtx = null
    }

    stats.value = {
      ...stats.value,
      isActive: false,
      currentIntervalMs: 0
    }

    console.info('[argus:snapshot] Snapshot capture stopped', {
      totalCaptured: stats.value.totalCaptured,
      totalEnqueued: stats.value.totalEnqueued,
      totalErrors: stats.value.totalErrors
    })
  }

  /**
   * Force an immediate snapshot capture (independent of interval).
   * Useful for on-demand evidence capture.
   */
  async function captureNow(): Promise<void> {
    await captureSnapshot()
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // State
    isCapturing,
    stats,

    // Lifecycle
    start,
    stop,

    // Manual capture
    captureNow
  }
}
