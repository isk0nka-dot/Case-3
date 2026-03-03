// =============================================================================
// Argus AI — useProctoringSession Composable
// =============================================================================
//
// Manages the lifecycle of a proctoring session from the browser client.
// Handles event collection, batching, heartbeat, and telemetry optimization.
//
// Architecture:
//
//   A proctoring session has 4 phases:
//     1. Initialization: Camera/mic permissions, face verification
//     2. Active Monitoring: Event collection + periodic heartbeat
//     3. Cooldown: Flush remaining events, wait for acks
//     4. Termination: Clean disconnect, session summary
//
//   Event flow:
//     Browser Sensor → useProctoringSession.sendEvent() →
//     → EventCollectorClient.queueEvent() →
//     → Batch Buffer (500ms flush) →
//     → HTTP POST /IngestBatch →
//     → Go gRPC Server → Kafka + ClickHouse
//
//   Telemetry optimization:
//     High-frequency events (gaze, mouse, keyboard: 30-60Hz) are downsampled
//     to 10Hz before batching. This reduces event volume from ~50/sec to ~10/sec
//     per session while preserving detection accuracy.
//
// Usage:
//
//   const session = useProctoringSession({
//     sessionId: 'sess-123',
//     studentId: 'student-456',
//     examId: 'exam-789',
//     orgId: 'org-001'
//   })
//
//   await session.start()
//   session.sendEvent(EventType.GAZE_DEVIATION, Severity.WARNING, { ... })
//   await session.stop()
//
// =============================================================================

import { ref, computed, watch, onUnmounted, type Ref, type ComputedRef } from 'vue'
import type { ProctoringEvent, ClientMeta, EventPayload, SessionDirective } from '~/lib/proto/types'
import { EventType, Severity, EventSource, TelemetryMode, isTelemetryEvent } from '~/lib/proto/types'
import { useResilience } from './useResilience'
import { registerDebugSession, clearDebugSession } from './useDebugBridge'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Session configuration. */
export interface SessionConfig {
  /** Unique session identifier. */
  sessionId: string
  /** Student identifier (IIN). */
  studentId: string
  /** Exam identifier. */
  examId: string
  /** Organization identifier. */
  orgId: string
  /** Heartbeat interval in ms. Default: 30_000 (30 seconds). */
  heartbeatIntervalMs?: number
  /** Telemetry downsampling interval in ms. Default: 100 (10Hz). */
  telemetrySampleIntervalMs?: number
  /** SDK version string. Default: '1.0.0'. */
  sdkVersion?: string
  /** Enable the resilience layer (offline-first, tier degradation). Default: false. */
  enableResilience?: boolean
  /** Health governor sampling interval (ms). Default: 5000. */
  healthSampleIntervalMs?: number
  /** Offline queue max size (bytes). Default: 500MB. */
  maxQueueSizeBytes?: number
}

/** Session status. */
export type SessionStatus = 'idle' | 'starting' | 'active' | 'stopping' | 'stopped' | 'terminated'

/** Session metrics snapshot. */
export interface SessionMetrics {
  /** Total events sent in this session. */
  totalEventsSent: number
  /** Total events dropped (failed to send). */
  totalEventsDropped: number
  /** Total violations detected. */
  violationCount: number
  /** Current focus score (0-1). */
  currentFocusScore: number
  /** Session duration in seconds. */
  durationSec: number
  /** Events per second (rolling average). */
  eventsPerSecond: number
  /** Current telemetry mode from server. */
  telemetryMode: TelemetryMode
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useProctoringSession(config: SessionConfig) {
  const {
    sessionId,
    studentId,
    examId,
    orgId,
    heartbeatIntervalMs = 30_000,
    telemetrySampleIntervalMs = 100,
    sdkVersion = '1.0.0',
    enableResilience = false,
    healthSampleIntervalMs,
    maxQueueSizeBytes
  } = config

  // -------------------------------------------------------------------------
  // Resilience Layer (optional, backward-compatible)
  // -------------------------------------------------------------------------

  const resilience = enableResilience
    ? useResilience({
        sessionId,
        orgId,
        examId,
        studentId,
        healthSampleIntervalMs,
        maxQueueSizeBytes
      })
    : null

  // Dynamic telemetry adjustment based on tier config.
  // When the tier engine changes the telemetry sample interval,
  // we propagate it to the session's downsampling logic.
  let dynamicTelemetryIntervalMs = telemetrySampleIntervalMs

  if (resilience) {
    watch(
      () => resilience.tierConfig.value.telemetry.sampleIntervalMs,
      (newInterval) => {
        dynamicTelemetryIntervalMs = newInterval
        console.debug('[argus:session] Telemetry interval adjusted by tier engine', {
          intervalMs: newInterval,
          tier: resilience.tier.value
        })
      }
    )
  }

  // -------------------------------------------------------------------------
  // Reactive State
  // -------------------------------------------------------------------------

  const status: Ref<SessionStatus> = ref('idle')
  const isActive: ComputedRef<boolean> = computed(() => status.value === 'active')
  const currentDirective: Ref<SessionDirective | null> = ref(null)
  const error: Ref<string | null> = ref(null)

  // Metrics
  const totalEventsSent = ref(0)
  const totalEventsDropped = ref(0)
  const violationCount = ref(0)
  const currentFocusScore = ref(1.0) // 1.0 = perfect focus
  const startedAt = ref(0)
  const telemetryMode = ref(TelemetryMode.TELEMETRY_NORMAL)

  // Telemetry downsampling state
  const lastTelemetryTimestamps = new Map<EventType, number>()

  // Timers
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null
  let eventCounter = 0

  // -------------------------------------------------------------------------
  // Client Metadata — cached at session start
  // -------------------------------------------------------------------------

  // Pre-compute immutable client metadata ONCE instead of re-reading
  // navigator.userAgent, screen dimensions, and constructing a Date object
  // on every event (~10 events/sec = 10 unnecessary allocations/sec).
  // Only timezoneOffsetMin could theoretically change mid-session (DST
  // transition), but in practice this never happens during a 2-3 hour exam.
  const cachedClientMeta: ClientMeta = {
    userAgent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
    sdkVersion,
    resolution: typeof screen !== 'undefined' ? `${screen.width}x${screen.height}` : '0x0',
    timezoneOffsetMin: new Date().getTimezoneOffset()
  }

  function getClientMeta(): ClientMeta {
    return cachedClientMeta
  }

  // -------------------------------------------------------------------------
  // Event ID Generation (UUIDv7-like: time-ordered)
  // -------------------------------------------------------------------------

  function generateEventId(): string {
    // Use server-adjusted time for time-ordered event IDs.
    // This ensures correct chronological ordering even when the
    // student's system clock is skewed.
    const adjustedNow = resilience ? resilience.serverAdjustedNow() : Date.now()
    const timestamp = adjustedNow.toString(36)
    const random = Math.random().toString(36).substring(2, 10)
    eventCounter++
    return `evt-${timestamp}-${random}-${eventCounter.toString(36)}`
  }

  // -------------------------------------------------------------------------
  // Session Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Start the proctoring session.
   *
   * Initializes batching, begins heartbeat polling, and transitions
   * the session to 'active' state.
   */
  async function start(): Promise<void> {
    if (status.value !== 'idle') {
      console.warn('[argus:session] Cannot start: session is not idle')
      return
    }

    status.value = 'starting'
    error.value = null

    try {
      const { $grpc } = useNuxtApp()

      // Start client-side batching.
      $grpc.client.startBatching()

      // Start heartbeat polling.
      heartbeatTimer = setInterval(() => {
        void sendHeartbeat()
      }, heartbeatIntervalMs)

      // Start resilience layer (health monitoring, offline queue, transport wiring).
      if (resilience) {
        resilience.start()
      }

      startedAt.value = Date.now()
      status.value = 'active'

      // Register with debug bridge for PerformanceDebugger overlay (Ctrl+Shift+D)
      // This is a no-op in production — the debugger panel is hidden by default.
      // We register here (instead of at construction) so the debugger only shows
      // data for actively running sessions.
      registerDebugSession(
        // Defer registration — the return object is created below.
        // For now, pass a partial proxy that the debugger can read from.
        { status, isActive, metrics, resilience } as ReturnType<typeof useProctoringSession>,
        resilience
      )

      console.debug(`[argus:session] Started session ${sessionId}`, {
        resilience: enableResilience,
        tier: resilience?.tier.value ?? 'N/A'
      })
    } catch (err) {
      error.value = (err as Error).message
      status.value = 'idle'
    }
  }

  /**
   * Stop the proctoring session gracefully.
   *
   * Flushes remaining events, stops heartbeat, and transitions to 'stopped'.
   */
  async function stop(): Promise<void> {
    if (status.value !== 'active') return

    status.value = 'stopping'

    try {
      // Stop heartbeat.
      if (heartbeatTimer) {
        clearInterval(heartbeatTimer)
        heartbeatTimer = null
      }

      // Flush remaining events.
      const { $grpc } = useNuxtApp()
      await $grpc.client.stopBatching()

      // Stop resilience layer.
      if (resilience) {
        resilience.stop()
      }

      // Final heartbeat.
      await sendHeartbeat()

      status.value = 'stopped'
      clearDebugSession()
      console.debug(`[argus:session] Stopped session ${sessionId}. Total events: ${totalEventsSent.value}`)
    } catch (err) {
      error.value = (err as Error).message
      status.value = 'stopped'
      clearDebugSession()
    }
  }

  // -------------------------------------------------------------------------
  // Event Sending
  // -------------------------------------------------------------------------

  /**
   * Send a proctoring event.
   *
   * This is the main API for the browser SDK to report detections.
   *
   * @param eventType - The type of event detected
   * @param severity - Severity classification
   * @param payload - Type-specific structured data
   * @param label - Human-readable description
   * @param confidence - AI model confidence (0-1)
   * @param source - Detection source (webcam, browser, kernel, etc.)
   */
  function sendEvent(
    eventType: EventType,
    severity: Severity,
    payload?: EventPayload,
    label?: string,
    confidence: number = 1.0,
    source: EventSource = EventSource.SYSTEM
  ): void {
    if (status.value !== 'active') return

    // Telemetry downsampling: skip if too soon since last sample.
    if (isTelemetryEvent(eventType)) {
      const now = Date.now()
      const lastTimestamp = lastTelemetryTimestamps.get(eventType) ?? 0
      const sampleInterval = getSampleInterval()

      if (now - lastTimestamp < sampleInterval) {
        return // Skip this sample.
      }
      lastTelemetryTimestamps.set(eventType, now)
    }

    // Track violations.
    if (severity === Severity.WARNING || severity === Severity.CRITICAL) {
      violationCount.value++
    }

    // Build the event with server-adjusted timestamp for correct chronological ordering.
    const adjustedNow = resilience ? resilience.serverAdjustedNow() : Date.now()
    const clientMeta = getClientMeta()

    // Include clock offset in client metadata for server-side audit trail.
    // This lets the server know the client was aware of its clock skew.
    if (resilience) {
      (clientMeta as unknown as Record<string, unknown>).clockOffsetMs = resilience.clockOffsetMs.value
    }

    const event: ProctoringEvent = {
      eventId: generateEventId(),
      sessionId,
      studentId,
      examId,
      orgId,
      eventType,
      severity,
      source,
      clientTimestamp: new Date(adjustedNow).toISOString(),
      label: label ?? '',
      confidence,
      payload,
      clientMeta
    }

    // Queue for batched delivery.
    try {
      const { $grpc } = useNuxtApp()
      void $grpc.client.queueEvent(event)
      totalEventsSent.value++
    } catch {
      totalEventsDropped.value++
    }
  }

  /**
   * Get the telemetry sample interval based on the server's directive.
   */
  function getSampleInterval(): number {
    // Use tier-adjusted interval when resilience is active, otherwise fall back
    // to the configured static interval.
    const baseInterval = resilience ? dynamicTelemetryIntervalMs : telemetrySampleIntervalMs

    switch (telemetryMode.value) {
      case TelemetryMode.TELEMETRY_HIGH_FREQ:
        return Math.max(baseInterval / 2, 33) // ~30Hz max
      case TelemetryMode.TELEMETRY_LOW_FREQ:
        return baseInterval * 3 // ~3.3Hz
      default:
        return baseInterval // tier-controlled
    }
  }

  // -------------------------------------------------------------------------
  // Convenience Event Methods
  // -------------------------------------------------------------------------

  /** Report a gaze deviation event. */
  function reportGazeDeviation(
    direction: string,
    durationMs: number,
    angleDegrees: number,
    gazeX: number,
    gazeY: number
  ): void {
    sendEvent(
      EventType.GAZE_DEVIATION,
      durationMs > 5000 ? Severity.CRITICAL : durationMs > 2000 ? Severity.WARNING : Severity.INFO,
      { type: 'gazeDeviation', data: { direction, durationMs, angleDegrees, gazeX, gazeY } },
      `Взгляд отведён ${direction} на ${(durationMs / 1000).toFixed(1)}с`,
      0.9,
      EventSource.WEBCAM
    )
  }

  /** Report a gaze telemetry point (high frequency). */
  function reportGazeTelemetry(gazeX: number, gazeY: number): void {
    sendEvent(
      EventType.GAZE_TELEMETRY,
      Severity.INFO,
      { type: 'gazeDeviation', data: { direction: 'center', durationMs: 0, angleDegrees: 0, gazeX, gazeY } },
      '',
      1.0,
      EventSource.WEBCAM
    )
  }

  /** Report mouse telemetry (high frequency). */
  function reportMouseTelemetry(x: number, y: number): void {
    sendEvent(
      EventType.MOUSE_TELEMETRY,
      Severity.INFO,
      undefined,
      `mouse:${x},${y}`,
      1.0,
      EventSource.BROWSER
    )
  }

  /** Report a face mismatch detection. */
  function reportFaceMismatch(similarity: number, isSpoof: boolean, spoofType?: string): void {
    sendEvent(
      EventType.FACE_MISMATCH,
      Severity.CRITICAL,
      { type: 'faceDetection', data: { match: false, similarity, faceCount: 1, isSpoof, spoofType } },
      `Несоответствие лица: сходство ${(similarity * 100).toFixed(0)}%`,
      similarity,
      EventSource.WEBCAM
    )
  }

  /** Report a phone detection. */
  function reportPhoneDetected(confidence: number, bboxX: number, bboxY: number, bboxW: number, bboxH: number): void {
    sendEvent(
      EventType.PHONE_DETECTED,
      Severity.CRITICAL,
      { type: 'objectDetection', data: { objectType: 'phone', bboxX, bboxY, bboxW, bboxH, detectionConfidence: confidence } },
      `Обнаружен телефон (${(confidence * 100).toFixed(0)}%)`,
      confidence,
      EventSource.WEBCAM
    )
  }

  /** Report a tab switch. */
  function reportTabSwitch(tabSwitchCount: number, targetInfo?: string): void {
    sendEvent(
      EventType.TAB_SWITCH,
      tabSwitchCount > 3 ? Severity.CRITICAL : Severity.WARNING,
      { type: 'browser', data: { tabSwitchCount, targetInfo, fullscreenExited: false, displayCount: 1, action: 'tab_switch' } },
      `Переключение вкладки #${tabSwitchCount}`,
      1.0,
      EventSource.BROWSER
    )
  }

  /** Update focus score from AI model. */
  function updateFocusScore(score: number): void {
    currentFocusScore.value = Math.max(0, Math.min(1, score))
    sendEvent(
      EventType.FOCUS_SCORE_UPDATE,
      Severity.INFO,
      { type: 'psychometry', data: { emotion: 'neutral', intensity: 0, focusScore: score, blinkRate: 0, blinkAnomaly: false } },
      '',
      1.0,
      EventSource.WEBCAM
    )
  }

  // -------------------------------------------------------------------------
  // Heartbeat
  // -------------------------------------------------------------------------

  /**
   * Send a heartbeat to the server and process directives.
   */
  async function sendHeartbeat(): Promise<void> {
    try {
      const { $grpc } = useNuxtApp()
      const response = await $grpc.client.heartbeat({
        sessionId,
        studentId,
        examId,
        clientTimestamp: new Date().toISOString(),
        currentFocusScore: currentFocusScore.value,
        violationCount: violationCount.value
      })

      // Process server directives.
      if (response.directive) {
        currentDirective.value = response.directive

        // Update telemetry mode.
        if (response.directive.telemetryMode !== undefined) {
          telemetryMode.value = response.directive.telemetryMode
        }

        // Handle termination directive.
        if (response.directive.terminate) {
          status.value = 'terminated'
          error.value = response.directive.terminateReason ?? 'Сессия прекращена сервером'
          await stop()
        }
      }

      // Session no longer active on server.
      if (!response.sessionActive && status.value === 'active') {
        status.value = 'terminated'
        error.value = 'Сессия истекла на сервере'
        await stop()
      }
    } catch {
      // Heartbeat failure is non-fatal — will retry on next interval.
    }
  }

  // -------------------------------------------------------------------------
  // Metrics
  // -------------------------------------------------------------------------

  const metrics: ComputedRef<SessionMetrics> = computed(() => {
    const durationSec = status.value === 'idle' ? 0
      : (Date.now() - startedAt.value) / 1000

    return {
      totalEventsSent: totalEventsSent.value,
      totalEventsDropped: totalEventsDropped.value,
      violationCount: violationCount.value,
      currentFocusScore: currentFocusScore.value,
      durationSec: Math.round(durationSec),
      eventsPerSecond: durationSec > 0
        ? Math.round((totalEventsSent.value / durationSec) * 10) / 10
        : 0,
      telemetryMode: telemetryMode.value
    }
  })

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  onUnmounted(() => {
    if (status.value === 'active') {
      void stop()
    }
  })

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // State
    status,
    isActive,
    error,
    currentDirective,
    metrics,

    // Resilience layer (null if enableResilience=false)
    resilience,

    // Lifecycle
    start,
    stop,

    // Generic event sender
    sendEvent,

    // Convenience methods
    reportGazeDeviation,
    reportGazeTelemetry,
    reportMouseTelemetry,
    reportFaceMismatch,
    reportPhoneDetected,
    reportTabSwitch,
    updateFocusScore
  }
}
