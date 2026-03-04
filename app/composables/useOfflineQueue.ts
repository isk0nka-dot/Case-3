// =============================================================================
// Argus AI — useOfflineQueue Composable
// =============================================================================
//
// Write-ahead log pattern for proctoring events and snapshots.
// Every event is persisted to IndexedDB BEFORE upload attempt.
// On success → removed from IDB. On failure → remains for retry.
//
// This eliminates the event-loss bug in client.ts where failed batch
// flushes permanently lost events.
//
// Drain logic (on reconnect):
//   1. Critical events (oldest first)
//   2. High priority events
//   3. Normal events
//   4. Snapshots (5 at a time to limit bandwidth)
//
// Usage:
//   const queue = useOfflineQueue()
//   const ids = await queue.enqueueEvents(events)
//   await queue.removeEvents(ids) // on successful upload
//   queue.startDrain() // begin background drain loop
//
// =============================================================================

import { ref, computed, type Ref, type ComputedRef } from 'vue'
import type { ProctoringEvent } from '~/lib/proto/types'
import { Severity } from '~/lib/proto/types'
import {
  addEvents,
  addSnapshot,
  removeEvents as idbRemoveEvents,
  removeSnapshots as idbRemoveSnapshots,
  getEventsByStatus,
  getSnapshotsByStatus,
  countEventsByStatus,
  countSnapshotsByStatus,
  updateEventStatus,
  updateSnapshotStatus,
  getTotalQueueSize,
  getOldestPendingAge,
  sha256,
  hmacSha256,
  type QueuedEvent,
  type QueuedSnapshot
} from '~/lib/storage/idb'
import { probeOPFSHealth, getStorageStats, type OPFSStorageStats } from '~/lib/storage/opfs'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Queue statistics for UI display. */
export interface QueueStats {
  /** Number of pending events. */
  pendingEvents: number
  /** Number of pending snapshots. */
  pendingSnapshots: number
  /** Total size of all pending items in bytes. */
  totalSizeBytes: number
  /** Age of the oldest pending item in ms. */
  oldestItemAge: number
  /** Number of permanently failed items. */
  failedCount: number
}

/** Configuration for the offline queue. */
export interface OfflineQueueConfig {
  /** Maximum total queue size in bytes. Default: 500MB. */
  maxSizeBytes: number
  /** Maximum events per session. Default: 50,000. */
  maxEventsPerSession: number
  /** Events to drain per batch. Default: 50. */
  drainBatchSize: number
  /** Interval between drain attempts in ms. Default: 1000. */
  drainIntervalMs: number
  /** Maximum retries before marking as permanently failed. Default: 10. */
  maxRetries: number
  /** Snapshots to drain per batch. Default: 5. */
  snapshotDrainBatchSize: number
  /** HMAC signing key for tamper-evident local logs. If provided, each event is HMAC-signed before storage. */
  signingKey: string
  /**
   * Maximum jitter delay (ms) before the first drain on reconnect.
   * Desynchronises 5,000+ simultaneous reconnects to prevent a thundering
   * herd from saturating Kafka. Each client waits a random [0, jitter) ms.
   * Default: 5000 (0–5 seconds).
   */
  reconnectJitterMs: number
  /**
   * Backoff multiplier applied to drainIntervalMs after a failed drain batch.
   * Resets to 1× on successful drain. Capped at maxDrainBackoffMs.
   * Default: 1.5.
   */
  drainBackoffMultiplier: number
  /**
   * Maximum drain interval during backoff (ms). Default: 30000 (30 seconds).
   */
  maxDrainBackoffMs: number
}

/** Snapshot metadata for enqueue. */
export interface SnapshotMeta {
  sessionId: string
  capturedAt: number
  resolution: string
  quality: number
  orgId: string
  examId: string
  studentId: string
}

/**
 * Interface for the transport layer to write/remove events.
 * This is what gets injected into EventCollectorClient.
 */
export interface OfflineQueueWriter {
  /** Persist events to IndexedDB. Returns IDB record IDs. */
  enqueueEvents(events: ProctoringEvent[]): Promise<number[]>
  /** Remove successfully uploaded events. */
  removeEvents(ids: number[]): Promise<void>
  /** Persist a snapshot to IndexedDB. Returns IDB record ID. */
  enqueueSnapshot(blob: ArrayBuffer, meta: SnapshotMeta): Promise<number>
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_CONFIG: OfflineQueueConfig = {
  maxSizeBytes: 500 * 1024 * 1024, // 500MB
  maxEventsPerSession: 50_000,
  drainBatchSize: 50,
  drainIntervalMs: 1000,
  maxRetries: 10,
  snapshotDrainBatchSize: 5,
  signingKey: '',
  reconnectJitterMs: 5000,
  drainBackoffMultiplier: 1.5,
  maxDrainBackoffMs: 30_000
}

// ---------------------------------------------------------------------------
// Composable
// ---------------------------------------------------------------------------

export function useOfflineQueue(config?: Partial<OfflineQueueConfig>) {
  const cfg: OfflineQueueConfig = { ...DEFAULT_CONFIG, ...config }

  // -------------------------------------------------------------------------
  // Reactive State
  // -------------------------------------------------------------------------

  const stats: Ref<QueueStats> = ref({
    pendingEvents: 0,
    pendingSnapshots: 0,
    totalSizeBytes: 0,
    oldestItemAge: 0,
    failedCount: 0
  })

  const isDraining: Ref<boolean> = ref(false)
  const isInitialized: Ref<boolean> = ref(false)

  // Internal
  let drainTimer: ReturnType<typeof setTimeout> | null = null
  let drainUploadFn: ((events: ProctoringEvent[]) => Promise<boolean>) | null = null
  let snapshotUploadFn: ((snapshot: QueuedSnapshot) => Promise<boolean>) | null = null
  /** Current drain interval — increases on failure (backoff), resets on success. */
  let currentDrainIntervalMs: number = cfg.drainIntervalMs

  // -------------------------------------------------------------------------
  // Priority Mapping
  // -------------------------------------------------------------------------

  function eventPriority(event: ProctoringEvent): QueuedEvent['priority'] {
    if (event.severity === Severity.CRITICAL) return 'critical'
    if (event.severity === Severity.WARNING) return 'high'
    const eventTypeNum = event.eventType as number
    if (eventTypeNum >= 100 && eventTypeNum <= 103) return 'low' // telemetry
    return 'normal'
  }

  // -------------------------------------------------------------------------
  // Stats Refresh
  // -------------------------------------------------------------------------

  async function refreshStats(): Promise<void> {
    try {
      const [pendingEvents, pendingSnapshots, failedEvents, failedSnapshots, totalSize, oldestAge]
        = await Promise.all([
          countEventsByStatus('pending'),
          countSnapshotsByStatus('pending'),
          countEventsByStatus('failed'),
          countSnapshotsByStatus('failed'),
          getTotalQueueSize(),
          getOldestPendingAge()
        ])

      stats.value = {
        pendingEvents,
        pendingSnapshots,
        totalSizeBytes: totalSize,
        oldestItemAge: oldestAge,
        failedCount: failedEvents + failedSnapshots
      }
    } catch (err) {
      console.error('[argus:queue] Failed to refresh stats:', err)
    }
  }

  // -------------------------------------------------------------------------
  // Enqueue Operations
  // -------------------------------------------------------------------------

  /**
   * Persist events to IndexedDB (write-ahead log).
   * If a signing key is configured, each event payload is HMAC-SHA256 signed
   * for tamper evidence before storage.
   * Returns the IDB record IDs for later removal on success.
   */
  async function enqueueEvents(events: ProctoringEvent[]): Promise<number[]> {
    if (events.length === 0) return []

    const records: Omit<QueuedEvent, 'id'>[] = await Promise.all(
      events.map(async (event) => {
        const payload = JSON.stringify(event)
        const record: Omit<QueuedEvent, 'id'> = {
          sessionId: event.sessionId,
          eventId: event.eventId,
          priority: eventPriority(event),
          payload,
          createdAt: Date.now(),
          retryCount: 0,
          lastRetryAt: null,
          status: 'pending' as const,
          sizeBytes: new Blob([payload]).size
        }

        // Tamper-evident signing: HMAC-SHA256 each event payload
        if (cfg.signingKey) {
          try {
            record.hmac = await hmacSha256(payload, cfg.signingKey)
          } catch {
            // Web Crypto API may not be available in all contexts (e.g., insecure origins).
            // Proceed without HMAC rather than losing the event.
            console.warn('[argus:queue] HMAC signing failed, storing event unsigned')
          }
        }

        return record
      })
    )

    try {
      const ids = await addEvents(records)

      // ADR-007: Evidence is NEVER evicted. Storage pressure triggers
      // HardBlockerModal via checkStorageQuota() instead.

      // Update stats (debounced to avoid thrashing)
      void refreshStats()

      return ids
    } catch (err) {
      console.error('[argus:queue] Failed to enqueue events:', err)
      return []
    }
  }

  /**
   * Remove events that were successfully uploaded.
   */
  async function removeEvents(ids: number[]): Promise<void> {
    if (ids.length === 0) return

    try {
      await idbRemoveEvents(ids)
      void refreshStats()
    } catch (err) {
      console.error('[argus:queue] Failed to remove events:', err)
    }
  }

  /**
   * Persist a snapshot to IndexedDB.
   * Computes SHA-256 hash before storage for tamper evidence.
   */
  async function enqueueSnapshot(blob: ArrayBuffer, meta: SnapshotMeta): Promise<number> {
    try {
      const hash = await sha256(blob)

      const id = await addSnapshot({
        sessionId: meta.sessionId,
        blob,
        sha256: hash,
        capturedAt: meta.capturedAt,
        resolution: meta.resolution,
        quality: meta.quality,
        sizeBytes: blob.byteLength,
        status: 'pending',
        retryCount: 0,
        orgId: meta.orgId,
        examId: meta.examId,
        studentId: meta.studentId
      })

      // ADR-007: Evidence is NEVER evicted. Storage pressure triggers
      // HardBlockerModal via checkStorageQuota() instead.

      void refreshStats()
      return id
    } catch (err) {
      console.error('[argus:queue] Failed to enqueue snapshot:', err)
      return -1
    }
  }

  // -------------------------------------------------------------------------
  // Drain Logic
  // -------------------------------------------------------------------------

  /**
   * Set the upload function used during drain.
   * Called with a batch of events; returns true if upload succeeded.
   */
  function setEventUploader(fn: (events: ProctoringEvent[]) => Promise<boolean>): void {
    drainUploadFn = fn
  }

  /**
   * Set the snapshot upload function used during drain.
   */
  function setSnapshotUploader(fn: (snapshot: QueuedSnapshot) => Promise<boolean>): void {
    snapshotUploadFn = fn
  }

  /**
   * Drain pending events from IndexedDB and upload them.
   * Processes in priority order: critical → high → normal → low → snapshots.
   */
  async function drain(): Promise<void> {
    if (isDraining.value) return
    if (!drainUploadFn) return

    isDraining.value = true
    let drainSucceeded = true

    try {
      // 1. Drain events by priority (all priorities in one query, pre-sorted)
      const pendingEvents = await getEventsByStatus('pending', cfg.drainBatchSize)

      if (pendingEvents.length > 0) {
        // Mark as 'sending' to prevent duplicate drain
        for (const evt of pendingEvents) {
          if (evt.id !== undefined) {
            await updateEventStatus(evt.id, 'sending')
          }
        }

        // Deserialize and upload
        const events: ProctoringEvent[] = []
        const eventIds: number[] = []

        for (const evt of pendingEvents) {
          try {
            events.push(JSON.parse(evt.payload) as ProctoringEvent)
            if (evt.id !== undefined) eventIds.push(evt.id)
          } catch {
            // Corrupt payload — mark as failed
            if (evt.id !== undefined) {
              await updateEventStatus(evt.id, 'failed')
            }
          }
        }

        if (events.length > 0 && drainUploadFn) {
          const success = await drainUploadFn(events)

          if (success) {
            // Upload succeeded — remove from IDB
            await idbRemoveEvents(eventIds)
          } else {
            drainSucceeded = false
            // Upload failed — revert to pending or mark as failed
            for (let i = 0; i < pendingEvents.length; i++) {
              const evt = pendingEvents[i]!
              if (evt.id === undefined) continue

              if (evt.retryCount >= cfg.maxRetries) {
                await updateEventStatus(evt.id, 'failed')
              } else {
                await updateEventStatus(evt.id, 'pending', true)
              }
            }
          }
        }
      }

      // 2. Drain snapshots
      if (snapshotUploadFn) {
        const pendingSnapshots = await getSnapshotsByStatus('pending', cfg.snapshotDrainBatchSize)

        for (const snap of pendingSnapshots) {
          if (snap.id === undefined) continue

          await updateSnapshotStatus(snap.id, 'sending')

          try {
            const success = await snapshotUploadFn(snap)

            if (success) {
              await idbRemoveSnapshots([snap.id])
            } else {
              if (snap.retryCount >= cfg.maxRetries) {
                await updateSnapshotStatus(snap.id, 'failed')
              } else {
                await updateSnapshotStatus(snap.id, 'pending', true)
              }
            }
          } catch {
            await updateSnapshotStatus(snap.id, 'pending', true)
          }
        }
      }

      void refreshStats()
    } catch (err) {
      console.error('[argus:queue] Drain error:', err)
      drainSucceeded = false
    } finally {
      isDraining.value = false
    }

    // ── Adaptive backoff: slow down on failure, reset on success ──
    if (drainSucceeded) {
      currentDrainIntervalMs = cfg.drainIntervalMs
    } else {
      currentDrainIntervalMs = Math.min(
        currentDrainIntervalMs * cfg.drainBackoffMultiplier,
        cfg.maxDrainBackoffMs
      )
    }

    // Schedule next drain iteration (self-scheduling setTimeout loop).
    scheduleDrainTick()
  }

  /**
   * Schedule the next drain tick using the current (possibly backed-off) interval.
   * Uses setTimeout instead of setInterval so the interval can adapt dynamically.
   */
  function scheduleDrainTick(): void {
    if (drainTimer === null) return // stopDrain was called
    drainTimer = setTimeout(() => {
      void drain()
    }, currentDrainIntervalMs)
  }

  /**
   * Start the background drain loop.
   * To prevent a thundering herd when thousands of clients reconnect
   * simultaneously, the first drain is delayed by a random jitter
   * in [0, reconnectJitterMs) milliseconds. Subsequent drains use
   * the configured interval with adaptive backoff on failure.
   */
  function startDrain(): void {
    if (drainTimer) return

    // Reset backoff state on fresh start.
    currentDrainIntervalMs = cfg.drainIntervalMs

    // Jittered initial delay: desynchronise reconnecting clients.
    const jitter = Math.floor(Math.random() * cfg.reconnectJitterMs)

    // Sentinel value — marks the loop as "active" so stopDrain works.
    drainTimer = setTimeout(() => {
      void drain() // drain() will call scheduleDrainTick() internally
    }, jitter)

    console.debug('[argus:queue] Drain loop started', {
      initialJitterMs: jitter,
      intervalMs: cfg.drainIntervalMs,
      batchSize: cfg.drainBatchSize
    })
  }

  /**
   * Stop the background drain loop.
   */
  function stopDrain(): void {
    if (drainTimer) {
      clearTimeout(drainTimer)
      drainTimer = null
    }
    currentDrainIntervalMs = cfg.drainIntervalMs
    console.debug('[argus:queue] Drain loop stopped')
  }

  // -------------------------------------------------------------------------
  // Computed
  // -------------------------------------------------------------------------

  const hasPendingItems: ComputedRef<boolean> = computed(() => {
    return stats.value.pendingEvents > 0 || stats.value.pendingSnapshots > 0
  })

  const totalPending: ComputedRef<number> = computed(() => {
    return stats.value.pendingEvents + stats.value.pendingSnapshots
  })

  const formattedSize: ComputedRef<string> = computed(() => {
    const bytes = stats.value.totalSizeBytes
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  })

  // -------------------------------------------------------------------------
  // OfflineQueueWriter interface (for injection into transport)
  // -------------------------------------------------------------------------

  const writer: OfflineQueueWriter = {
    enqueueEvents,
    removeEvents,
    enqueueSnapshot
  }

  // -------------------------------------------------------------------------
  // Initialize
  // -------------------------------------------------------------------------

  async function initialize(): Promise<void> {
    if (isInitialized.value) return

    try {
      // ADR-007: Request persistent storage BEFORE any writes.
      // Prevents the browser from silently evicting our IndexedDB evidence
      // during storage pressure. Without this, Chrome can delete our data
      // at any time when running low on disk space.
      if (typeof navigator !== 'undefined' && navigator.storage?.persist) {
        try {
          const granted = await navigator.storage.persist()
          if (granted) {
            console.debug('[argus:queue] Persistent storage GRANTED — evidence is protected from browser eviction')
          } else {
            console.warn('[argus:queue] Persistent storage DENIED — evidence may be evicted by the browser under storage pressure')
          }
        } catch (err) {
          console.warn('[argus:queue] navigator.storage.persist() failed:', err)
        }
      }

      await refreshStats()
      isInitialized.value = true

      // Start OPFS health probe for fallback recovery
      startOPFSProbe()

      console.debug('[argus:queue] Offline queue initialized', stats.value)
    } catch (err) {
      console.error('[argus:queue] Failed to initialize:', err)
    }
  }

  // Auto-initialize
  void initialize()

  // -------------------------------------------------------------------------
  // IndexedDB Quota Monitoring (v3.0 — ADR-007: Edge Persistence)
  // -------------------------------------------------------------------------

  /** Browser storage quota info. */
  const storageQuota: Ref<{ usage: number, quota: number, percent: number }> = ref({
    usage: 0,
    quota: 0,
    percent: 0
  })

  /**
   * CRITICAL threshold (90%): when exceeded, the exam is hard-blocked.
   * The HardBlockerModal is displayed and all evidence capture stops until
   * the student frees disk space. Evidence is NEVER evicted.
   */
  const QUOTA_CRITICAL_THRESHOLD = 0.9

  /**
   * Recovery threshold (80%): when storage drops below this after being
   * blocked, the hard blocker is cleared and draining resumes.
   */
  const QUOTA_RECOVERY_THRESHOLD = 0.8

  /**
   * Hard blocker state. When true, the HardBlockerModal is shown to the
   * student and the drain loop is paused. The exam cannot continue until
   * storage drops below QUOTA_RECOVERY_THRESHOLD.
   */
  const isStorageBlocked: Ref<boolean> = ref(false)

  let quotaCheckTimer: ReturnType<typeof setInterval> | null = null

  // -------------------------------------------------------------------------
  // OPFS Health Probe & Fallback Tracking (Gap 4)
  // -------------------------------------------------------------------------

  let opfsProbeTimer: ReturnType<typeof setInterval> | null = null
  const opfsFallbackCount: Ref<number> = ref(0)

  /** Start periodic OPFS health probing (every 60 seconds). */
  function startOPFSProbe(): void {
    if (opfsProbeTimer) return
    opfsProbeTimer = setInterval(async () => {
      const healthy = await probeOPFSHealth()
      if (healthy) {
        // If it was previously failed (fallback count > 0), log recovery
        if (opfsFallbackCount.value > 0) {
          console.info('[argus:queue] OPFS recovered after', opfsFallbackCount.value, 'fallback writes')
        }
      }
    }, 60_000)
    // Initial probe
    void probeOPFSHealth()
    console.debug('[argus:queue] OPFS health probe started (60s interval)')
  }

  /** Stop OPFS health probing. */
  function stopOPFSProbe(): void {
    if (opfsProbeTimer) {
      clearInterval(opfsProbeTimer)
      opfsProbeTimer = null
    }
  }

  /** Handle OPFS → IDB fallback events from the storage layer. */
  function handleStorageFallback(event: Event): void {
    const detail = (event as CustomEvent).detail
    opfsFallbackCount.value++
    console.warn('[argus:queue] OPFS fallback event received', {
      key: detail?.key,
      error: detail?.error,
      totalFallbacks: opfsFallbackCount.value
    })
  }

  // Listen for storage fallback events
  if (typeof window !== 'undefined') {
    window.addEventListener('argus:storage-fallback', handleStorageFallback)
  }

  /**
   * Check IndexedDB/storage quota via navigator.storage.estimate().
   *
   * ADR-007 policy:
   *   - At 90% quota → set isStorageBlocked=true, stop drain loop
   *   - At < 80% quota (when blocked) → clear block, resume drain
   *   - Evidence is NEVER evicted regardless of quota pressure
   */
  async function checkStorageQuota(): Promise<void> {
    if (typeof navigator === 'undefined' || !navigator.storage?.estimate) return

    try {
      const estimate = await navigator.storage.estimate()
      const usage = estimate.usage ?? 0
      const quota = estimate.quota ?? 0
      const percent = quota > 0 ? usage / quota : 0

      storageQuota.value = { usage, quota, percent }

      if (percent >= QUOTA_CRITICAL_THRESHOLD && !isStorageBlocked.value) {
        // CRITICAL: Storage nearly full. Hard-block the exam.
        isStorageBlocked.value = true
        stopDrain()
        console.error('[argus:queue] STORAGE CRITICAL — exam hard-blocked', {
          usageMB: Math.round(usage / (1024 * 1024)),
          quotaMB: Math.round(quota / (1024 * 1024)),
          percent: Math.round(percent * 100) + '%'
        })
      } else if (percent < QUOTA_RECOVERY_THRESHOLD && isStorageBlocked.value) {
        // Recovery: storage dropped below 80%, resume normal operation.
        isStorageBlocked.value = false
        startDrain()
        console.info('[argus:queue] Storage recovered — exam unblocked, drain resumed', {
          usageMB: Math.round(usage / (1024 * 1024)),
          quotaMB: Math.round(quota / (1024 * 1024)),
          percent: Math.round(percent * 100) + '%'
        })
      }
    } catch (err) {
      // navigator.storage.estimate() may not be available in all contexts.
      console.debug('[argus:queue] Storage quota check skipped:', err)
    }
  }

  /** Start periodic quota monitoring (every 30 seconds). */
  function startQuotaMonitoring(): void {
    if (quotaCheckTimer) return
    quotaCheckTimer = setInterval(() => void checkStorageQuota(), 30_000)
    // Initial check.
    void checkStorageQuota()
    console.debug('[argus:queue] Storage quota monitoring started')
  }

  /** Stop periodic quota monitoring. */
  function stopQuotaMonitoring(): void {
    if (quotaCheckTimer) {
      clearInterval(quotaCheckTimer)
      quotaCheckTimer = null
    }
  }

  // -------------------------------------------------------------------------
  // Public API
  // -------------------------------------------------------------------------

  return {
    // State
    stats,
    isDraining,
    isInitialized,
    hasPendingItems,
    totalPending,
    formattedSize,

    // Storage quota (v3.0 — ADR-007)
    storageQuota,
    /** Hard blocker: true when storage ≥90%. Exam must pause until recovery. */
    isStorageBlocked,

    // Operations
    enqueueEvents,
    removeEvents,
    enqueueSnapshot,

    // Writer interface (for transport layer injection)
    writer,

    // Drain control
    startDrain,
    stopDrain,
    drain,
    setEventUploader,
    setSnapshotUploader,

    // Quota monitoring (v3.0 — ADR-007)
    startQuotaMonitoring,
    stopQuotaMonitoring,
    checkStorageQuota,

    // OPFS health probe (Gap 4 — dual-write fallback)
    startOPFSProbe,
    stopOPFSProbe,
    opfsFallbackCount,
    getOPFSStats: getStorageStats,

    // Utilities
    refreshStats
  }
}
