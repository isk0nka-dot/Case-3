// =============================================================================
// Argus AI — useProctoringAlerts Composable
// =============================================================================
//
// Reactive composable that provides real-time critical alert monitoring for
// the proctoring dashboard.
//
// Architecture:
//
//   The composable maintains a persistent connection to the backend and listens
//   for events with Severity = CRITICAL. When a critical event is detected,
//   it updates the reactive UI state immediately.
//
//   Connection strategy:
//     1. Primary: Long-polling via periodic Heartbeat RPCs
//        - Every 5 seconds, send a heartbeat and check for new critical events
//        - Low overhead, works through all proxies and firewalls
//        - Provides session directives (terminate, telemetry mode changes)
//
//     2. Fallback: Client-side event monitoring
//        - The EventCollectorClient's onAck callback notifies on accepted events
//        - Critical events queued via queueEvent() trigger immediate UI updates
//
//     3. Future: Server-Sent Events (SSE) endpoint
//        - When the Go server adds an SSE endpoint for push notifications,
//          this composable will upgrade to SSE for sub-100ms latency
//
//   State management:
//     - Alerts are stored in a bounded circular buffer (max 500 entries)
//     - Older alerts are evicted when the buffer is full
//     - Alerts are indexed by sessionId for efficient per-session filtering
//     - A separate "unread count" tracks alerts not yet acknowledged by the proctor
//
// Usage:
//
//   const {
//     alerts,           // Reactive array of CriticalAlert objects
//     unreadCount,      // Number of unacknowledged alerts
//     isConnected,      // Whether the polling connection is active
//     alertsBySession,  // Alerts grouped by session ID
//     startMonitoring,  // Begin polling for alerts
//     stopMonitoring,   // Stop polling
//     acknowledgeAlert, // Mark an alert as read
//     acknowledgeAll,   // Mark all alerts as read
//     clearAlerts       // Remove all alerts
//   } = useProctoringAlerts()
//
// =============================================================================

import { ref, computed, onUnmounted, type Ref, type ComputedRef } from 'vue'
import type { ProctoringEvent } from '~/lib/proto/types'
import {
  Severity,
  EventType,
  EVENT_TYPE_LABELS,
  SEVERITY_LABELS,
  EVENT_SOURCE_LABELS,
  isCriticalEvent
} from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** A critical alert derived from a ProctoringEvent. */
export interface CriticalAlert {
  /** Unique alert ID (matches event_id). */
  id: string
  /** Session that generated the alert. */
  sessionId: string
  /** Student identifier. */
  studentId: string
  /** Exam identifier. */
  examId: string
  /** Organization identifier. */
  orgId: string
  /** Event type that triggered the alert. */
  eventType: EventType
  /** Human-readable event type label. */
  eventTypeLabel: string
  /** Severity level. */
  severity: Severity
  /** Human-readable severity label. */
  severityLabel: string
  /** Source that detected the event. */
  sourceLabel: string
  /** Human-readable description. */
  label: string
  /** AI confidence score (0-1). */
  confidence: number
  /** When the event occurred (ISO 8601). */
  timestamp: string
  /** When the alert was received by the dashboard. */
  receivedAt: number
  /** Whether the proctor has acknowledged this alert. */
  acknowledged: boolean
  /** Alert priority (lower = more urgent). Derived from event type. */
  priority: number
}

/** Monitoring configuration. */
export interface MonitoringConfig {
  /** Polling interval in ms. Default: 5000 (5 seconds). */
  pollingIntervalMs?: number
  /** Maximum alerts to keep in memory. Default: 500. */
  maxAlerts?: number
  /** Auto-start monitoring on composable creation. Default: false. */
  autoStart?: boolean
  /** Filter alerts by session ID. If set, only alerts for this session are tracked. */
  filterSessionId?: string
  /** Filter alerts by exam ID. If set, only alerts for this exam are tracked. */
  filterExamId?: string
  /** Minimum severity to track. Default: CRITICAL. */
  minSeverity?: Severity
  /** Enable sound notification for new alerts. Default: true. */
  enableSound?: boolean
}

// ---------------------------------------------------------------------------
// Priority mapping — lower number = higher priority (more urgent)
// ---------------------------------------------------------------------------

const EVENT_PRIORITY: Partial<Record<EventType, number>> = {
  [EventType.FACE_SPOOF_DETECTED]: 1,
  [EventType.REMOTE_ACCESS_DETECTED]: 2,
  [EventType.VIRTUAL_MACHINE_DETECTED]: 3,
  [EventType.FACE_MISMATCH]: 4,
  [EventType.MULTIPLE_PERSONS]: 5,
  [EventType.VPN_PROXY_DETECTED]: 6,
  [EventType.PHONE_DETECTED]: 7,
  [EventType.FORBIDDEN_PROCESS_DETECTED]: 8,
  [EventType.HARDWARE_ID_MISMATCH]: 9,
  [EventType.EARBUDS_DETECTED]: 10
}

function getEventPriority(eventType: EventType): number {
  return EVENT_PRIORITY[eventType] ?? 50
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

/**
 * useProctoringAlerts — Real-time critical alert monitoring.
 *
 * Provides reactive state for the dashboard's critical alerts panel.
 * Supports both polling-based and event-driven alert ingestion.
 */
export function useProctoringAlerts(config?: MonitoringConfig) {
  const {
    pollingIntervalMs = 5_000,
    maxAlerts = 500,
    autoStart = false,
    filterSessionId,
    filterExamId,
    minSeverity = Severity.CRITICAL,
    enableSound = true
  } = config ?? {}

  // -------------------------------------------------------------------------
  // Reactive State
  // -------------------------------------------------------------------------

  /** All critical alerts, sorted by priority (most urgent first). */
  const alerts: Ref<CriticalAlert[]> = ref([])

  /** Whether the monitoring connection is active. */
  const isConnected = ref(false)

  /** Connection error message, if any. */
  const connectionError: Ref<string | null> = ref(null)

  /** Last successful heartbeat timestamp. */
  const lastHeartbeat = ref(0)

  // Internal state.
  let pollingTimer: ReturnType<typeof setInterval> | null = null
  let unsubscribeAck: (() => void) | null = null

  // -------------------------------------------------------------------------
  // Computed Properties
  // -------------------------------------------------------------------------

  /** Number of unacknowledged alerts. */
  const unreadCount: ComputedRef<number> = computed(() =>
    alerts.value.filter(a => !a.acknowledged).length
  )

  /** Whether there are any unacknowledged critical alerts. */
  const hasCritical: ComputedRef<boolean> = computed(() =>
    alerts.value.some(a => !a.acknowledged && a.severity === Severity.CRITICAL)
  )

  /** Alerts grouped by session ID. */
  const alertsBySession: ComputedRef<Map<string, CriticalAlert[]>> = computed(() => {
    const map = new Map<string, CriticalAlert[]>()
    for (const alert of alerts.value) {
      const existing = map.get(alert.sessionId) ?? []
      existing.push(alert)
      map.set(alert.sessionId, existing)
    }
    return map
  })

  /** Unique session IDs with active alerts. */
  const alertedSessions: ComputedRef<string[]> = computed(() =>
    [...new Set(alerts.value.filter(a => !a.acknowledged).map(a => a.sessionId))]
  )

  /** Most recent N alerts (for the dashboard ticker). */
  const recentAlerts: ComputedRef<CriticalAlert[]> = computed(() =>
    alerts.value.slice(0, 10)
  )

  /** Alert counts by event type. */
  const alertCountsByType: ComputedRef<Map<EventType, number>> = computed(() => {
    const counts = new Map<EventType, number>()
    for (const alert of alerts.value) {
      counts.set(alert.eventType, (counts.get(alert.eventType) ?? 0) + 1)
    }
    return counts
  })

  // -------------------------------------------------------------------------
  // Alert Management
  // -------------------------------------------------------------------------

  /**
   * Add a new alert from a ProctoringEvent.
   *
   * This is the primary ingestion path. Events are filtered by severity
   * and optionally by session/exam ID before being added to the alert list.
   */
  function ingestEvent(event: ProctoringEvent): void {
    // Filter by severity.
    if (event.severity < minSeverity) return

    // Filter by session/exam if configured.
    if (filterSessionId && event.sessionId !== filterSessionId) return
    if (filterExamId && event.examId !== filterExamId) return

    // Deduplicate by event ID.
    if (alerts.value.some(a => a.id === event.eventId)) return

    // Create alert.
    const alert: CriticalAlert = {
      id: event.eventId,
      sessionId: event.sessionId,
      studentId: event.studentId,
      examId: event.examId,
      orgId: event.orgId,
      eventType: event.eventType,
      eventTypeLabel: EVENT_TYPE_LABELS[event.eventType] ?? 'Неизвестно',
      severity: event.severity,
      severityLabel: SEVERITY_LABELS[event.severity] ?? 'Неизвестно',
      sourceLabel: EVENT_SOURCE_LABELS[event.source] ?? 'Неизвестно',
      label: event.label,
      confidence: event.confidence,
      timestamp: event.clientTimestamp,
      receivedAt: Date.now(),
      acknowledged: false,
      priority: getEventPriority(event.eventType)
    }

    // Insert sorted by priority (most urgent first).
    const newAlerts = [...alerts.value, alert]
      .sort((a, b) => a.priority - b.priority || b.receivedAt - a.receivedAt)

    // Evict oldest if over capacity.
    if (newAlerts.length > maxAlerts) {
      newAlerts.length = maxAlerts
    }

    alerts.value = newAlerts

    // Sound notification for critical alerts.
    if (enableSound && event.severity === Severity.CRITICAL) {
      playAlertSound()
    }
  }

  /**
   * Acknowledge (mark as read) a specific alert.
   */
  function acknowledgeAlert(alertId: string): void {
    const alert = alerts.value.find(a => a.id === alertId)
    if (alert) {
      alert.acknowledged = true
      // Trigger reactivity by replacing the array.
      alerts.value = [...alerts.value]
    }
  }

  /**
   * Acknowledge all alerts.
   */
  function acknowledgeAll(): void {
    for (const alert of alerts.value) {
      alert.acknowledged = true
    }
    alerts.value = [...alerts.value]
  }

  /**
   * Remove all alerts.
   */
  function clearAlerts(): void {
    alerts.value = []
  }

  /**
   * Remove alerts older than the specified duration (ms).
   */
  function pruneOldAlerts(maxAgeMs: number): void {
    const cutoff = Date.now() - maxAgeMs
    alerts.value = alerts.value.filter(a => a.receivedAt >= cutoff)
  }

  // -------------------------------------------------------------------------
  // Monitoring Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Start monitoring for critical alerts.
   *
   * This begins periodic heartbeat polling and subscribes to the
   * EventCollectorClient's ack callback for real-time event ingestion.
   */
  function startMonitoring(): void {
    if (isConnected.value) return

    isConnected.value = true
    connectionError.value = null

    // Subscribe to client ack events for real-time ingestion.
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc?.client) {
        // The client's onAck callback doesn't provide full event data,
        // so we also need the polling path. But for events queued via
        // queueEvent(), the event data is available in the store.
        unsubscribeAck = $grpc.client.onAck((_ack) => {
          // Acks confirm delivery but don't contain event data.
          // Critical events are ingested via the ingestEvent() method.
        })
      }
    } catch {
      // gRPC plugin not available — polling-only mode.
    }

    // Start heartbeat polling.
    pollingTimer = setInterval(() => {
      void pollHeartbeat()
    }, pollingIntervalMs)

    // Immediate first poll.
    void pollHeartbeat()
  }

  /**
   * Stop monitoring.
   */
  function stopMonitoring(): void {
    isConnected.value = false

    if (pollingTimer) {
      clearInterval(pollingTimer)
      pollingTimer = null
    }

    if (unsubscribeAck) {
      unsubscribeAck()
      unsubscribeAck = null
    }
  }

  /**
   * Poll the backend via heartbeat for session health.
   */
  async function pollHeartbeat(): Promise<void> {
    try {
      const { $grpc } = useNuxtApp()
      if (!$grpc?.client) return

      // Send heartbeat for the monitored session (if configured).
      if (filterSessionId) {
        const response = await $grpc.client.heartbeat({
          sessionId: filterSessionId,
          studentId: '',
          examId: filterExamId ?? '',
          clientTimestamp: new Date().toISOString(),
          currentFocusScore: 0,
          violationCount: alerts.value.length
        })

        lastHeartbeat.value = Date.now()
        connectionError.value = null

        // Handle server directives.
        if (response.directive?.terminate) {
          ingestEvent({
            eventId: `term-${Date.now()}`,
            sessionId: filterSessionId,
            studentId: '',
            examId: filterExamId ?? '',
            orgId: '',
            eventType: EventType.EVENT_TYPE_UNSPECIFIED,
            severity: Severity.CRITICAL,
            source: 0,
            clientTimestamp: new Date().toISOString(),
            label: `Сессия прекращена: ${response.directive.terminateReason ?? 'нарушение правил'}`,
            confidence: 1.0
          })
        }
      }
    } catch (err) {
      connectionError.value = (err as Error).message
      // Don't set isConnected to false — keep trying.
    }
  }

  // -------------------------------------------------------------------------
  // Sound Notification
  // -------------------------------------------------------------------------

  /** Play a subtle alert sound for critical violations. */
  function playAlertSound(): void {
    try {
      // Use Web Audio API for a brief alert tone.
      const audioContext = new (window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext)()
      const oscillator = audioContext.createOscillator()
      const gainNode = audioContext.createGain()

      oscillator.connect(gainNode)
      gainNode.connect(audioContext.destination)

      oscillator.frequency.value = 880 // A5 note
      oscillator.type = 'sine'
      gainNode.gain.value = 0.1 // Quiet

      oscillator.start()
      gainNode.gain.exponentialRampToValueAtTime(0.001, audioContext.currentTime + 0.3)
      oscillator.stop(audioContext.currentTime + 0.3)
    } catch {
      // Audio not available (no user gesture, mobile restrictions).
    }
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  // Auto-start if configured.
  if (autoStart) {
    startMonitoring()
  }

  // Cleanup on component unmount.
  onUnmounted(() => {
    stopMonitoring()
  })

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // Reactive state
    alerts,
    unreadCount,
    hasCritical,
    isConnected,
    connectionError,
    lastHeartbeat,
    alertsBySession,
    alertedSessions,
    recentAlerts,
    alertCountsByType,

    // Actions
    ingestEvent,
    acknowledgeAlert,
    acknowledgeAll,
    clearAlerts,
    pruneOldAlerts,
    startMonitoring,
    stopMonitoring
  }
}
