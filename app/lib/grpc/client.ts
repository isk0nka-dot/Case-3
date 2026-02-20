// =============================================================================
// Argus AI — EventCollectorService gRPC Client
// =============================================================================
//
// Type-safe gRPC client for the Event Collector service.
// Maps TypeScript method calls to gRPC-Web unary RPCs.
//
// Usage:
//   const client = new EventCollectorClient(transport)
//   const response = await client.ingestEvent({ event: proctoringEvent })
//
// Architecture:
//
//   This client provides 4 methods matching the proto service definition:
//     1. ingestEvent  — Single event ingestion (critical violations)
//     2. ingestBatch  — Batch ingestion (high-frequency telemetry)
//     3. heartbeat    — Session keepalive + server directives
//     4. streamEvents — Bidirectional streaming (via polling fallback)
//
//   The StreamEvents RPC is bidirectional streaming which is NOT supported
//   by native gRPC-Web (only server-streaming). For the browser, we
//   implement streaming via:
//     1. Batch collection on client → periodic IngestBatch calls
//     2. Server push → long-polling or SSE for critical alerts
//
//   This is the correct pattern for high-frequency browser telemetry:
//   the browser SDK buffers events locally and flushes them in batches
//   every 100ms-1s, rather than opening a persistent stream.
//
// =============================================================================

import type {
  IngestEventRequest,
  IngestEventResponse,
  IngestBatchRequest,
  IngestBatchResponse,
  HeartbeatRequest,
  HeartbeatResponse,
  ProctoringEvent,
  StreamAck
} from '../proto/types'

import {
  encodeIngestEventRequest,
  encodeIngestBatchRequest,
  encodeHeartbeatRequest,
  decodeResponse
} from '../proto/codec'

import type { GrpcWebTransport } from './transport'

// ---------------------------------------------------------------------------
// Offline Queue Writer Interface
// ---------------------------------------------------------------------------

/**
 * Interface for the IndexedDB-backed offline queue.
 * When set, the client persists events before upload and removes
 * them on success — eliminating the event-loss bug.
 */
export interface OfflineQueueWriter {
  /** Persist events to IndexedDB (write-ahead log). Returns IDB record IDs. */
  enqueueEvents(events: ProctoringEvent[]): Promise<number[]>
  /** Remove successfully uploaded events from IndexedDB. */
  removeEvents(ids: number[]): Promise<void>
}

// ---------------------------------------------------------------------------
// Service Paths — gRPC method paths
// ---------------------------------------------------------------------------

/**
 * gRPC method paths for the EventCollectorService.
 * Format: /<package>.<service>/<method>
 */
const SERVICE_PATHS = {
  ingestEvent: '/argus.eventcollector.v1.EventCollectorService/IngestEvent',
  ingestBatch: '/argus.eventcollector.v1.EventCollectorService/IngestBatch',
  streamEvents: '/argus.eventcollector.v1.EventCollectorService/StreamEvents',
  heartbeat: '/argus.eventcollector.v1.EventCollectorService/Heartbeat'
} as const

// ---------------------------------------------------------------------------
// Client Interface
// ---------------------------------------------------------------------------

/** Options for event ingestion. */
export interface IngestOptions {
  /** Override the default timeout for this call. */
  timeout?: number
  /** AbortSignal for cancellation. */
  signal?: AbortSignal
}

/** Batch flush configuration. */
export interface BatchConfig {
  /** Maximum events per batch. Default: 100. */
  maxBatchSize: number
  /** Maximum time to buffer before flushing (ms). Default: 500. */
  flushIntervalMs: number
  /** Separate telemetry from violations. Default: true. */
  separateTelemetry: boolean
}

// ---------------------------------------------------------------------------
// EventCollectorClient
// ---------------------------------------------------------------------------

/**
 * EventCollectorClient provides a type-safe interface to the gRPC Event
 * Collector service. It supports:
 *
 *   - Unary calls (IngestEvent, IngestBatch, Heartbeat)
 *   - Client-side batching for telemetry events
 *   - Automatic batch flushing on timer or size threshold
 *   - Event priority separation (critical → immediate, telemetry → batched)
 */
export class EventCollectorClient {
  private readonly transport: GrpcWebTransport

  // Client-side batch buffer for telemetry events.
  private telemetryBuffer: ProctoringEvent[] = []
  private violationBuffer: ProctoringEvent[] = []
  private flushTimer: ReturnType<typeof setInterval> | null = null
  private batchConfig: BatchConfig
  private batchCounter = 0

  // Callbacks for streaming simulation.
  private onAckCallbacks: Array<(ack: StreamAck) => void> = []

  // Offline queue for write-ahead event persistence.
  // When set, events are saved to IndexedDB before upload and
  // removed on success — preventing data loss on network failure.
  private offlineQueue: OfflineQueueWriter | null = null

  constructor(transport: GrpcWebTransport, batchConfig?: Partial<BatchConfig>) {
    this.transport = transport
    this.batchConfig = {
      maxBatchSize: batchConfig?.maxBatchSize ?? 100,
      flushIntervalMs: batchConfig?.flushIntervalMs ?? 500,
      separateTelemetry: batchConfig?.separateTelemetry ?? true
    }
  }

  /**
   * Attach or detach the offline queue writer.
   *
   * When set, the flushBuffer method writes events to IndexedDB
   * before attempting upload. On upload success, events are removed
   * from IndexedDB. On failure, they remain for later retry.
   *
   * Pass null to detach (reverts to fire-and-forget behavior).
   */
  setOfflineQueue(queue: OfflineQueueWriter | null): void {
    this.offlineQueue = queue
  }

  // -------------------------------------------------------------------------
  // Unary RPCs
  // -------------------------------------------------------------------------

  /**
   * IngestEvent — Send a single proctoring event.
   *
   * Use this for CRITICAL events that need immediate server processing
   * (face mismatch, phone detected, remote access). For telemetry
   * events, use `queueEvent()` instead for automatic batching.
   */
  async ingestEvent(request: IngestEventRequest): Promise<IngestEventResponse> {
    const body = encodeIngestEventRequest(request)
    const response = await this.transport.unary<Record<string, unknown>>(
      SERVICE_PATHS.ingestEvent,
      body
    )
    return decodeResponse<IngestEventResponse>(response)
  }

  /**
   * IngestBatch — Send a batch of proctoring events.
   *
   * Preferred for high-frequency telemetry (gaze, mouse, keyboard).
   * Amortizes HTTP overhead across N events.
   */
  async ingestBatch(request: IngestBatchRequest): Promise<IngestBatchResponse> {
    const body = encodeIngestBatchRequest(request)
    const response = await this.transport.unary<Record<string, unknown>>(
      SERVICE_PATHS.ingestBatch,
      body
    )
    return decodeResponse<IngestBatchResponse>(response)
  }

  /**
   * Heartbeat — Session keepalive with integrity metrics.
   *
   * Returns server directives: telemetry mode changes, session termination.
   */
  async heartbeat(request: HeartbeatRequest): Promise<HeartbeatResponse> {
    const body = encodeHeartbeatRequest(request)
    const response = await this.transport.unary<Record<string, unknown>>(
      SERVICE_PATHS.heartbeat,
      body
    )
    return decodeResponse<HeartbeatResponse>(response)
  }

  // -------------------------------------------------------------------------
  // Smart Batching — Client-Side Event Buffering
  // -------------------------------------------------------------------------

  /**
   * Queue an event for batched delivery.
   *
   * Events are classified by severity:
   *   - CRITICAL → Immediately sent via IngestEvent (bypass buffer)
   *   - WARNING/INFO → Buffered and sent in batches
   *   - Telemetry → Separate buffer with higher flush threshold
   *
   * This reduces gRPC calls from 10,000/sec to ~100/sec while maintaining
   * sub-second latency for critical violations.
   */
  async queueEvent(event: ProctoringEvent): Promise<void> {
    // Critical events bypass the buffer for immediate delivery.
    if (event.severity === 3) { // Severity.CRITICAL
      try {
        const response = await this.ingestEvent({ event })
        this.notifyAck({
          eventId: response.eventId,
          sequence: response.sequence,
          accepted: response.accepted
        })
      } catch {
        // Failed to send critical event — add to violation buffer for retry.
        this.violationBuffer.push(event)
        this.checkFlushThreshold()
      }
      return
    }

    // Classify into telemetry vs violation buffer.
    const eventTypeNum = event.eventType as number
    if (this.batchConfig.separateTelemetry && eventTypeNum >= 100 && eventTypeNum <= 103) {
      this.telemetryBuffer.push(event)
    } else {
      this.violationBuffer.push(event)
    }

    this.checkFlushThreshold()
  }

  /**
   * Start automatic batch flushing on a timer.
   *
   * Call this when the proctoring session begins.
   */
  startBatching(): void {
    if (this.flushTimer) return

    this.flushTimer = setInterval(() => {
      void this.flush()
    }, this.batchConfig.flushIntervalMs)
  }

  /**
   * Stop automatic batch flushing and flush remaining events.
   *
   * Call this when the proctoring session ends.
   */
  async stopBatching(): Promise<void> {
    if (this.flushTimer) {
      clearInterval(this.flushTimer)
      this.flushTimer = null
    }

    // Final flush.
    await this.flush()
  }

  /**
   * Manually flush all buffered events.
   */
  async flush(): Promise<void> {
    const promises: Promise<void>[] = []

    // Flush violation buffer.
    if (this.violationBuffer.length > 0) {
      const events = this.violationBuffer.splice(0)
      promises.push(this.flushBuffer(events))
    }

    // Flush telemetry buffer.
    if (this.telemetryBuffer.length > 0) {
      const events = this.telemetryBuffer.splice(0)
      promises.push(this.flushBuffer(events))
    }

    await Promise.allSettled(promises)
  }

  /**
   * Register a callback for stream acknowledgments.
   * Simulates bidirectional streaming feedback.
   */
  onAck(callback: (ack: StreamAck) => void): () => void {
    this.onAckCallbacks.push(callback)
    return () => {
      const idx = this.onAckCallbacks.indexOf(callback)
      if (idx !== -1) this.onAckCallbacks.splice(idx, 1)
    }
  }

  /**
   * Get current buffer sizes for monitoring.
   */
  getBufferStats(): { telemetry: number; violations: number; batchConfig: BatchConfig } {
    return {
      telemetry: this.telemetryBuffer.length,
      violations: this.violationBuffer.length,
      batchConfig: { ...this.batchConfig }
    }
  }

  /**
   * Dynamically reconfigure batch parameters based on current resilience tier.
   *
   * This is the key performance lever: under Tier A (optimal), we flush
   * frequently with small batches for low latency. Under Tier C (critical),
   * we accumulate large batches with long intervals to minimize network calls.
   *
   * Call this method when the Tier Engine transitions between tiers.
   *
   * @param tier - Current resilience tier (A, B, or C)
   */
  reconfigureBatch(tier: 'A' | 'B' | 'C'): void {
    const TIER_BATCH_CONFIGS: Record<string, Partial<BatchConfig>> = {
      A: { maxBatchSize: 100, flushIntervalMs: 500 },   // Low latency, small batches
      B: { maxBatchSize: 200, flushIntervalMs: 2000 },   // Reduced RPS, larger batches
      C: { maxBatchSize: 500, flushIntervalMs: 5000 }    // Store-and-forward, max batching
    }

    const tierConfig = TIER_BATCH_CONFIGS[tier]
    if (!tierConfig) return

    const changed =
      this.batchConfig.maxBatchSize !== tierConfig.maxBatchSize
      || this.batchConfig.flushIntervalMs !== tierConfig.flushIntervalMs

    if (!changed) return

    this.batchConfig = {
      ...this.batchConfig,
      ...tierConfig
    }

    // Restart the flush timer with the new interval if batching is active
    if (this.flushTimer) {
      clearInterval(this.flushTimer)
      this.flushTimer = setInterval(() => {
        void this.flush()
      }, this.batchConfig.flushIntervalMs)
    }

    console.info(`[argus:grpc] Batch config updated for Tier ${tier}`, {
      maxBatchSize: this.batchConfig.maxBatchSize,
      flushIntervalMs: this.batchConfig.flushIntervalMs
    })
  }

  // -------------------------------------------------------------------------
  // Internal
  // -------------------------------------------------------------------------

  /**
   * Flush a buffer of events via IngestBatch.
   * Splits into chunks of maxBatchSize if necessary.
   *
   * When an OfflineQueueWriter is attached, uses the write-ahead log pattern:
   *   1. Write events to IndexedDB (persist before upload)
   *   2. Attempt upload via IngestBatch
   *   3. On success: remove from IndexedDB
   *   4. On failure: events remain in IndexedDB for later retry
   *
   * This eliminates the event-loss bug where failed flushes permanently
   * lost events that had already been removed from the in-memory buffer.
   */
  private async flushBuffer(events: ProctoringEvent[]): Promise<void> {
    // Split into chunks.
    const chunks: ProctoringEvent[][] = []
    for (let i = 0; i < events.length; i += this.batchConfig.maxBatchSize) {
      chunks.push(events.slice(i, i + this.batchConfig.maxBatchSize))
    }

    for (const chunk of chunks) {
      // Step 1: Write-ahead to IndexedDB (if queue is attached).
      let idbIds: number[] = []
      if (this.offlineQueue) {
        try {
          idbIds = await this.offlineQueue.enqueueEvents(chunk)
        } catch {
          console.warn('[argus] IndexedDB write-ahead failed, attempting direct send')
        }
      }

      try {
        const batchId = this.generateBatchId()
        const response = await this.ingestBatch({
          events: chunk,
          batchId
        })

        // Step 2: Upload succeeded — remove from IndexedDB.
        if (this.offlineQueue && idbIds.length > 0) {
          try {
            await this.offlineQueue.removeEvents(idbIds)
          } catch {
            // Non-fatal: events uploaded but IDB cleanup failed.
            // They'll be deduplicated on the server if re-uploaded.
            console.warn('[argus] IndexedDB cleanup failed after successful upload')
          }
        }

        // Notify ack callbacks for each accepted event.
        for (const event of chunk) {
          if (!response.rejectedEventIds.includes(event.eventId)) {
            this.notifyAck({
              eventId: event.eventId,
              sequence: response.batchSequence,
              accepted: true
            })
          }
        }
      } catch {
        // Upload failed — events are SAFE in IndexedDB (if queue attached).
        // The drain loop will retry them when connectivity is restored.
        if (this.offlineQueue && idbIds.length > 0) {
          console.warn(`[argus] Batch flush failed, ${chunk.length} events queued for retry in IndexedDB`)
        } else {
          console.error(`[argus] Batch flush failed, ${chunk.length} events lost (no offline queue)`)
        }
      }
    }
  }

  /**
   * Check if buffers have exceeded the flush threshold.
   */
  private checkFlushThreshold(): void {
    if (
      this.violationBuffer.length >= this.batchConfig.maxBatchSize
      || this.telemetryBuffer.length >= this.batchConfig.maxBatchSize
    ) {
      void this.flush()
    }
  }

  /**
   * Generate a unique batch ID for deduplication.
   */
  private generateBatchId(): string {
    this.batchCounter++
    return `batch-${Date.now()}-${this.batchCounter}`
  }

  /**
   * Notify all ack callbacks.
   */
  private notifyAck(ack: StreamAck): void {
    for (const callback of this.onAckCallbacks) {
      try {
        callback(ack)
      } catch {
        // Swallow callback errors.
      }
    }
  }
}
