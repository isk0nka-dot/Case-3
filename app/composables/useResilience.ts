// =============================================================================
// Argus AI — useResilience Composable (Master Orchestrator)
// =============================================================================
//
// Top-level composable that wires together:
//   - Health Governor (CPU/FPS/RTT/bandwidth monitoring)
//   - Tier Engine (adaptive degradation state machine)
//   - Offline Queue (IndexedDB-backed write-ahead log)
//   - Transport Layer (online/offline detection)
//
// Provides a unified API for the proctoring session to:
//   - Monitor device and network health
//   - Automatically degrade/upgrade monitoring quality
//   - Queue events offline and drain on reconnect
//   - Upload binary evidence via chunked uploads
//
// Usage:
//   const resilience = useResilience({
//     sessionId: 'sess-123',
//     orgId: 'org-1',
//     examId: 'exam-1',
//     studentId: 'student-1'
//   })
//   resilience.start()
//   // ... events automatically route through IndexedDB queue ...
//   resilience.stop()
//
// =============================================================================

import { ref, computed, watch, type Ref, type ComputedRef } from 'vue'
import { useHealthGovernor, type ResilienceTier } from './useHealthGovernor'
import { useTierEngine, type TierConfig } from './useTierEngine'
import { useOfflineQueue, type OfflineQueueWriter, type QueueStats } from './useOfflineQueue'
import { uploadChunked } from '~/lib/grpc/chunked-upload'
import { sha256 } from '~/lib/storage/idb'
import type { QueuedSnapshot } from '~/lib/storage/idb'
import type { ProctoringEvent, HeartbeatResponse, SessionDirective, TelemetryMode } from '~/lib/proto/types'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Resilience session configuration. */
export interface ResilienceConfig {
  /** Session identifier. */
  sessionId: string
  /** Organization identifier. */
  orgId: string
  /** Exam identifier. */
  examId: string
  /** Student identifier. */
  studentId: string
  /** Health governor sampling interval (ms). Default: 5000. */
  healthSampleIntervalMs?: number
  /** Offline queue max size (bytes). Default: 500MB. */
  maxQueueSizeBytes?: number
  /** Signing key for tamper-evident local event logs (HMAC-SHA256). Typically the session JWT or a derived key. */
  signingKey?: string
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useResilience(config: ResilienceConfig) {
  // -------------------------------------------------------------------------
  // Sub-Systems
  // -------------------------------------------------------------------------

  const healthGovernor = useHealthGovernor({
    sampleIntervalMs: config.healthSampleIntervalMs ?? 5_000
  })

  const tierEngine = useTierEngine(healthGovernor)

  const offlineQueue = useOfflineQueue({
    maxSizeBytes: config.maxQueueSizeBytes ?? 500 * 1024 * 1024,
    signingKey: config.signingKey ?? ''
  })

  // -------------------------------------------------------------------------
  // State
  // -------------------------------------------------------------------------

  const isStarted: Ref<boolean> = ref(false)
  const connectionMessage: Ref<string> = ref('')

  // Server-side backpressure state
  const serverDirective: Ref<SessionDirective | null> = ref(null)
  const serverTerminated: Ref<boolean> = ref(false)
  const serverTerminateReason: Ref<string> = ref('')
  let heartbeatTimer: ReturnType<typeof setInterval> | null = null
  const heartbeatIntervalMs = 15_000 // Send heartbeat every 15s

  // -------------------------------------------------------------------------
  // Transport Integration
  // -------------------------------------------------------------------------

  /** Wire the offline queue into the gRPC client as a write-ahead log. */
  function wireTransport(): void {
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc?.client) {
        // Inject the offline queue writer into the gRPC client
        $grpc.client.setOfflineQueue(offlineQueue.writer)

        // Set up the event drain uploader
        offlineQueue.setEventUploader(async (events: ProctoringEvent[]) => {
          try {
            const batchId = `drain-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
            await $grpc.client.ingestBatch({
              events,
              batchId
            })
            return true
          } catch {
            return false
          }
        })

        // Set up snapshot drain uploader (chunked upload with tier-based pacing)
        offlineQueue.setSnapshotUploader(async (snapshot: QueuedSnapshot) => {
          try {
            const baseUrl = getBaseUrl()
            const headers = getAuthHeaders()

            // Bandwidth pacing: limit chunk upload speed based on current tier
            // to leave headroom for real-time event streaming.
            const paceDelayByTier: Record<string, number> = { A: 0, B: 200, C: 500 }
            const paceDelayMs = paceDelayByTier[healthGovernor.currentTier.value] ?? 0

            await uploadChunked(baseUrl, {
              sessionId: snapshot.sessionId,
              fragmentId: `snap-${snapshot.id}-${snapshot.capturedAt}`,
              data: snapshot.blob,
              contentType: 'image/jpeg',
              sha256: snapshot.sha256,
              orgId: snapshot.orgId,
              examId: snapshot.examId,
              studentId: snapshot.studentId,
              paceDelayMs
            }, headers)

            return true
          } catch {
            return false
          }
        })

        // Listen for online/offline transitions
        if ($grpc.transport) {
          $grpc.transport.onConnectionChange((online: boolean) => {
            if (online) {
              connectionMessage.value = ''
              offlineQueue.startDrain()
            } else {
              connectionMessage.value = 'Подключение потеряно. События сохраняются локально.'
              offlineQueue.stopDrain()
            }
          })
        }
      }
    } catch {
      console.warn('[argus:resilience] gRPC plugin not available yet')
    }
  }

  // -------------------------------------------------------------------------
  // Server-Side Backpressure — Heartbeat Loop
  // -------------------------------------------------------------------------

  /**
   * Start a periodic heartbeat that sends session keepalive to the backend.
   * The server responds with directives that override local tier decisions:
   *
   *   - telemetryMode: NORMAL → Tier A, HIGH_FREQ → force Tier A, LOW_FREQ → force Tier B/C
   *   - terminate: true → immediately stop the proctoring session
   *
   * This integrates server-side load shedding with the client Tier Engine.
   */
  function startHeartbeatLoop(): void {
    if (heartbeatTimer) return

    heartbeatTimer = setInterval(async () => {
      if (!isStarted.value || serverTerminated.value) return

      try {
        const { $grpc } = useNuxtApp()
        if (!$grpc?.client) return

        const response: HeartbeatResponse = await $grpc.client.heartbeat({
          sessionId: config.sessionId,
          studentId: config.studentId,
          examId: config.examId,
          clientTimestamp: new Date().toISOString(),
          currentFocusScore: healthGovernor.healthScore.value,
          violationCount: 0 // Will be populated by session state
        })

        // Process server directives
        if (response.directive) {
          serverDirective.value = response.directive
          applyServerDirective(response.directive)
        }

        // Check session termination
        if (!response.sessionActive || response.directive?.terminate) {
          serverTerminated.value = true
          serverTerminateReason.value = response.directive?.terminateReason || 'Session terminated by server'
          connectionMessage.value = `Сессия завершена сервером: ${serverTerminateReason.value}`
          stop()
        }
      } catch {
        // Heartbeat failed — not fatal, next iteration will retry.
        // The transport layer handles offline detection separately.
        console.debug('[argus:resilience] Heartbeat failed, will retry')
      }
    }, heartbeatIntervalMs)
  }

  function stopHeartbeatLoop(): void {
    if (heartbeatTimer) {
      clearInterval(heartbeatTimer)
      heartbeatTimer = null
    }
  }

  /**
   * Apply a server directive to the local Tier Engine.
   *
   * Server-side backpressure mapping:
   *   TELEMETRY_NORMAL   → No override, local Health Governor drives tier selection
   *   TELEMETRY_HIGH_FREQ → Server wants MORE data; force upgrade toward Tier A
   *   TELEMETRY_LOW_FREQ  → Server is overloaded; force downgrade toward Tier C
   *
   * The TelemetryMode acts as a multiplier on the health score:
   *   NORMAL    → healthScore × 1.0 (no change)
   *   HIGH_FREQ → healthScore + 30 (bias toward upgrade)
   *   LOW_FREQ  → healthScore - 30 (bias toward downgrade)
   */
  function applyServerDirective(directive: SessionDirective): void {
    const mode = directive.telemetryMode

    // Map server telemetry mode to a health score modifier
    // This biases the Health Governor's tier selection without hard-overriding
    // client-side health measurements (respects local device constraints).
    const TelemetryMode_NORMAL = 0
    const TelemetryMode_HIGH_FREQ = 1
    const TelemetryMode_LOW_FREQ = 2

    let scoreBias = 0
    if (mode === TelemetryMode_HIGH_FREQ) {
      scoreBias = 30 // Bias toward Tier A
      connectionMessage.value = 'Сервер запрашивает увеличенную телеметрию'
    } else if (mode === TelemetryMode_LOW_FREQ) {
      scoreBias = -30 // Bias toward Tier C
      connectionMessage.value = 'Сервер снижает частоту телеметрии (backpressure)'
    } else if (mode === TelemetryMode_NORMAL) {
      scoreBias = 0
      if (connectionMessage.value.includes('телеметрию') || connectionMessage.value.includes('backpressure')) {
        connectionMessage.value = ''
      }
    }

    // Apply bias to the current health score to influence tier selection.
    // The Health Governor's next sample will incorporate this bias.
    const currentScore = healthGovernor.healthScore.value
    const biasedScore = Math.max(0, Math.min(100, currentScore + scoreBias))
    healthGovernor.healthScore.value = biasedScore

    console.info('[argus:resilience] Server directive applied', {
      telemetryMode: mode,
      scoreBias,
      originalScore: currentScore,
      biasedScore,
      currentTier: healthGovernor.currentTier.value
    })

    // Reconfigure gRPC batch params based on server directive.
    // LOW_FREQ → force Tier C batch config (max batching, 5s flush)
    // HIGH_FREQ → force Tier A batch config (small batches, 500ms flush)
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc?.client) {
        if (mode === TelemetryMode_LOW_FREQ) {
          $grpc.client.reconfigureBatch('C')
        } else if (mode === TelemetryMode_HIGH_FREQ) {
          $grpc.client.reconfigureBatch('A')
        } else {
          // NORMAL: let the tier engine drive batch config
          $grpc.client.reconfigureBatch(healthGovernor.currentTier.value)
        }
      }
    } catch {
      // Client not available
    }
  }

  /** Get the backend base URL from the gRPC transport. */
  function getBaseUrl(): string {
    try {
      const { $grpc } = useNuxtApp()
      // Access the config's baseUrl via the transport
      const transport = $grpc?.transport as unknown as Record<string, unknown> | undefined
      const transportConfig = transport?.config as Record<string, string> | undefined
      return transportConfig?.baseUrl ?? ''
    } catch {
      return ''
    }
  }

  /** Get auth headers for chunked uploads. */
  function getAuthHeaders(): Record<string, string> {
    try {
      const { $grpc } = useNuxtApp()
      // The auth interceptor adds the token — we need to replicate it
      // for non-gRPC requests (chunked uploads use fetch directly)
      const headers: Record<string, string> = {}

      // Try to get the JWT from the auth store
      try {
        const authStore = useNuxtApp().$pinia?.state?.value?.auth
        const token = authStore?.jwtToken
        if (token) {
          headers['Authorization'] = `Bearer ${token}`
        }
      } catch {
        // Auth store not available
      }

      return headers
    } catch {
      return {}
    }
  }

  // -------------------------------------------------------------------------
  // Lifecycle
  // -------------------------------------------------------------------------

  /**
   * Start the resilience layer.
   * Initializes health monitoring, wires transport, and starts drain loop.
   */
  function start(): void {
    if (isStarted.value) return

    isStarted.value = true

    // Wire the offline queue into the transport layer
    wireTransport()

    // Start health monitoring
    healthGovernor.start()

    // Start drain loop (if online)
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc?.transport?.isOnline) {
        offlineQueue.startDrain()
      }
    } catch {
      // Start drain anyway — it'll check online status internally
      offlineQueue.startDrain()
    }

    // Start server-side backpressure heartbeat loop
    startHeartbeatLoop()

    // Watch tier changes for UX messages + adaptive batch reconfiguration
    watch(
      () => healthGovernor.currentTier.value,
      (newTier, oldTier) => {
        if (newTier !== oldTier) {
          if (newTier === 'B') {
            connectionMessage.value = 'Подключение нестабильно. Переход в облегчённый режим.'
          } else if (newTier === 'C') {
            connectionMessage.value = 'Подключение потеряно. Переход в автономный режим.'
          } else if (newTier === 'A' && oldTier !== undefined) {
            connectionMessage.value = 'Подключение восстановлено.'
            // Clear message after 3 seconds
            setTimeout(() => {
              if (connectionMessage.value === 'Подключение восстановлено.') {
                connectionMessage.value = ''
              }
            }, 3000)
          }

          // Dynamically adjust gRPC batch parameters based on tier.
          // Tier A: small batches, 500ms flush (low latency)
          // Tier B: medium batches, 2s flush (reduced RPS)
          // Tier C: large batches, 5s flush (max batching, store-and-forward)
          try {
            const { $grpc } = useNuxtApp()
            if ($grpc?.client) {
              $grpc.client.reconfigureBatch(newTier)
            }
          } catch {
            // Client not available
          }
        }
      }
    )

    console.info('[argus:resilience] Resilience layer started', {
      sessionId: config.sessionId,
      maxQueueSize: `${(config.maxQueueSizeBytes ?? 500 * 1024 * 1024) / (1024 * 1024)}MB`
    })
  }

  /**
   * Stop the resilience layer.
   * Stops health monitoring, drain loop, and detaches from transport.
   */
  function stop(): void {
    if (!isStarted.value) return

    isStarted.value = false

    // Stop server-side backpressure heartbeat
    stopHeartbeatLoop()

    // Stop health monitoring
    healthGovernor.stop()

    // Stop drain loop
    offlineQueue.stopDrain()

    // Detach offline queue from transport
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc?.client) {
        $grpc.client.setOfflineQueue(null)
      }
    } catch {
      // Plugin not available
    }

    console.info('[argus:resilience] Resilience layer stopped')
  }

  // -------------------------------------------------------------------------
  // Computed Properties
  // -------------------------------------------------------------------------

  /** Current resilience tier (A/B/C). */
  const tier: ComputedRef<ResilienceTier> = computed(() => healthGovernor.currentTier.value)

  /** Active tier configuration. */
  const tierConfig: ComputedRef<TierConfig> = computed(() => tierEngine.activeTierConfig.value)

  /** Is the system online? */
  const isOnline: ComputedRef<boolean> = computed(() => {
    try {
      const { $grpc } = useNuxtApp()
      return $grpc?.transport?.isOnline ?? true
    } catch {
      return typeof navigator !== 'undefined' ? navigator.onLine : true
    }
  })

  /** Is the system effectively offline (browser offline OR consecutive failures)? */
  const isEffectivelyOffline: ComputedRef<boolean> = computed(() => {
    try {
      const { $grpc } = useNuxtApp()
      return $grpc?.transport?.effectivelyOffline ?? false
    } catch {
      return typeof navigator !== 'undefined' ? !navigator.onLine : false
    }
  })

  /** Queue statistics. */
  const queueStats: ComputedRef<QueueStats> = computed(() => offlineQueue.stats.value)

  /** Whether the system is degraded (not Tier A). */
  const isDegraded: ComputedRef<boolean> = computed(() => tier.value !== 'A')

  /** Whether there are pending items in the queue. */
  const hasPendingItems: ComputedRef<boolean> = computed(() => offlineQueue.hasPendingItems.value)

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // State
    tier,
    tierConfig,
    healthScore: healthGovernor.healthScore,
    healthMetrics: healthGovernor.metrics,
    queueStats,
    isOnline,
    isEffectivelyOffline,
    isDegraded,
    hasPendingItems,
    connectionMessage,
    isStarted,

    // Server-side backpressure state
    serverDirective,
    serverTerminated,
    serverTerminateReason,

    // Sub-systems (for advanced usage)
    healthGovernor,
    tierEngine,
    offlineQueue,

    // Lifecycle
    start,
    stop,

    // Server directive control
    applyServerDirective,

    // Utilities
    sha256
  }
}
