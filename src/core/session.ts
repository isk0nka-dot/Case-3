// =============================================================================
// Argus SDK — Session Lifecycle Manager
// =============================================================================
//
// Manages the proctoring session: event batching, heartbeat, and reporting.
// Extracted from useProctoringSession — Vue refs replaced with class state.
// =============================================================================

import type {
  ProctoringEvent,
  EventType,
  Severity,
  EventSource,
  EventPayload,
  SessionStatus,
  ViolationEvent,
  SDKError,
  IngestBatchResponse,
  HeartbeatResponse,
} from '../types';
import { GrpcWebTransport } from './transport';
import { decodeResponse, encodeIngestBatchRequest, encodeHeartbeatRequest } from './codec';
import { EventEmitter } from './event-emitter';
import { EVENT_COLLECTOR_SERVICE_PATHS } from './service-paths';

/** Session events emitted by the session manager. */
export interface SessionEvents {
  statusChange: SessionStatus;
  violation: ViolationEvent;
  error: SDKError;
  heartbeat: { sequenceNum: number };
  delivery: {
    queueDepth: number;
    inFlight: boolean;
    droppedEvents: number;
    lastAcceptedCount?: number;
    lastRejectedCount?: number;
  };
}

/** JWT claims extracted from the session token. */
interface TokenClaims {
  session_id: string;
  student_id: string;
  exam_id: string;
  org_id: string;
  exp: number;
}

/**
 * Core session manager — handles event batching, heartbeat, and lifecycle.
 *
 * This is the internal engine. ArgusSDK wraps this with the camera widget
 * and preflight UI.
 */
export class SessionManager extends EventEmitter<SessionEvents> {
  private readonly transport: GrpcWebTransport;
  private readonly claims: TokenClaims;
  private readonly token: string;

  // Batch buffer.
  private eventBuffer: ProctoringEvent[] = [];
  private batchCounter = 0;
  private flushTimer: ReturnType<typeof setInterval> | null = null;
  private flushInFlight = false;
  private droppedEvents = 0;

  // Heartbeat.
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private heartbeatSeqNum = 0;

  // State.
  private _status: SessionStatus = 'idle';
  private _violationCount = 0;
  private _focusScore = 100;
  private _eventCount = 0;

  // Config.
  private readonly batchSize: number;
  private readonly flushIntervalMs: number;
  private readonly heartbeatIntervalMs: number;
  private readonly maxQueueSize: number;
  private readonly persistOfflineQueue: boolean;
  private readonly storageKey: string;
  private readonly onlineHandler: () => void;

  constructor(
    transport: GrpcWebTransport,
    token: string,
    options?: {
      batchSize?: number;
      flushIntervalMs?: number;
      heartbeatIntervalMs?: number;
      maxQueueSize?: number;
      persistOfflineQueue?: boolean;
    },
  ) {
    super();
    this.transport = transport;
    this.token = token;
    this.claims = this._parseToken(token);
    this.batchSize = options?.batchSize ?? 50;
    this.flushIntervalMs = options?.flushIntervalMs ?? 2000;
    this.heartbeatIntervalMs = options?.heartbeatIntervalMs ?? 30_000;
    this.maxQueueSize = options?.maxQueueSize ?? 1000;
    this.persistOfflineQueue = options?.persistOfflineQueue ?? true;
    this.storageKey = `argus:sdk:event-queue:${this.claims.session_id}`;
    this.onlineHandler = () => {
      this._flush().catch(() => {/* next timer will retry */});
    };
    this._restoreQueue();
  }

  // ---------------------------------------------------------------------------
  // Public accessors
  // ---------------------------------------------------------------------------

  get status(): SessionStatus { return this._status; }
  get sessionId(): string { return this.claims.session_id; }
  get studentId(): string { return this.claims.student_id; }
  get examId(): string { return this.claims.exam_id; }
  get orgId(): string { return this.claims.org_id; }
  get violationCount(): number { return this._violationCount; }
  get focusScore(): number { return this._focusScore; }
  get eventCount(): number { return this._eventCount; }
  get queueDepth(): number { return this.eventBuffer.length; }
  get droppedEventCount(): number { return this.droppedEvents; }

  // ---------------------------------------------------------------------------
  // Lifecycle
  // ---------------------------------------------------------------------------

  /** Start the session — begins event batching and heartbeat. */
  start(): void {
    if (this._status === 'active') return;

    this._setStatus('active');

    // Start flush timer.
    this.flushTimer = setInterval(() => this._flush(), this.flushIntervalMs);
    if (typeof window !== 'undefined') {
      window.addEventListener('online', this.onlineHandler);
    }

    // Start heartbeat timer.
    this.heartbeatTimer = setInterval(() => this._sendHeartbeat(), this.heartbeatIntervalMs);
    this._sendHeartbeat(); // Immediate first heartbeat.
  }

  /** Stop the session — flushes remaining events and stops timers. */
  async stop(): Promise<void> {
    if (this._status !== 'active' && this._status !== 'paused') return;

    // Flush remaining events.
    await this._flush();

    // Stop timers.
    if (this.flushTimer) { clearInterval(this.flushTimer); this.flushTimer = null; }
    if (this.heartbeatTimer) { clearInterval(this.heartbeatTimer); this.heartbeatTimer = null; }
    if (typeof window !== 'undefined') {
      window.removeEventListener('online', this.onlineHandler);
    }

    this._setStatus('completed');
  }

  /** Destroy the session — releases all resources. */
  destroy(): void {
    if (this.flushTimer) { clearInterval(this.flushTimer); this.flushTimer = null; }
    if (this.heartbeatTimer) { clearInterval(this.heartbeatTimer); this.heartbeatTimer = null; }
    if (typeof window !== 'undefined') {
      window.removeEventListener('online', this.onlineHandler);
    }
    this._persistQueue();
    this.removeAllListeners();
  }

  // ---------------------------------------------------------------------------
  // Event reporting
  // ---------------------------------------------------------------------------

  /** Send a proctoring event. Buffers for batch delivery. */
  sendEvent(
    eventType: EventType,
    severity: Severity,
    source: EventSource,
    label: string,
    confidence: number,
    payload?: EventPayload,
  ): void {
    if (this._status !== 'active') return;
    this._recordEvent(eventType, severity, source, label, confidence, payload, true);
  }

  /** Send a lifecycle/evidence event outside the active monitoring loop. */
  sendLifecycleEvent(
    eventType: EventType,
    severity: Severity,
    source: EventSource,
    label: string,
    confidence: number,
    payload?: EventPayload,
  ): void {
    this._recordEvent(eventType, severity, source, label, confidence, payload, false);
  }

  /** Flush buffered events immediately. */
  async flushNow(): Promise<void> {
    await this._flush();
  }

  /** Update focus score (called from health governor). */
  updateFocusScore(score: number): void {
    this._focusScore = Math.max(0, Math.min(100, score));
  }

  // ---------------------------------------------------------------------------
  // Private
  // ---------------------------------------------------------------------------

  private _recordEvent(
    eventType: EventType,
    severity: Severity,
    source: EventSource,
    label: string,
    confidence: number,
    payload: EventPayload | undefined,
    countAsViolation: boolean,
  ): void {
    const event: ProctoringEvent = {
      eventId: this._generateEventId(),
      sessionId: this.claims.session_id,
      studentId: this.claims.student_id,
      examId: this.claims.exam_id,
      orgId: this.claims.org_id,
      eventType,
      severity,
      source,
      clientTimestamp: new Date().toISOString(),
      label,
      confidence,
      payload,
    };

    this.eventBuffer.push(event);
    this._eventCount++;
    this._enforceQueueLimit();
    this._persistQueue();

    // Track violations (WARNING + CRITICAL).
    if (countAsViolation && severity >= 2) {
      this._violationCount++;
      this.emit('violation', {
        type: eventType,
        severity,
        label,
        confidence,
        timestamp: event.clientTimestamp,
      });
    }

    // Auto-flush if buffer is full.
    if (this.eventBuffer.length >= this.batchSize) {
      this._flush().catch(() => {/* swallow — retry will handle */});
    }
  }

  private _setStatus(status: SessionStatus): void {
    if (this._status === status) return;
    this._status = status;
    this.emit('statusChange', status);
  }

  private async _flush(): Promise<void> {
    if (this.eventBuffer.length === 0) return;
    if (this.flushInFlight) return;
    if (typeof navigator !== 'undefined' && navigator.onLine === false) {
      this._emitDelivery();
      return;
    }

    this.flushInFlight = true;
    const batch = this.eventBuffer.splice(0, this.batchSize);
    const batchId = `b-${++this.batchCounter}-${Date.now()}`;
    this._persistQueue();
    this._emitDelivery(true);

    try {
      const encoded = encodeIngestBatchRequest({ events: batch, batchId });
      const response = await this.transport.call(
        EVENT_COLLECTOR_SERVICE_PATHS.ingestBatch,
        JSON.stringify(encoded),
        { Authorization: `Bearer ${this.token}` },
      );
      const decoded = this._decodeBatchResponse(response.data);
      this._persistQueue();
      this._emitDelivery(false, decoded);
    } catch (err) {
      // Re-queue failed events at the front of the buffer.
      this.eventBuffer.unshift(...batch);
      this._enforceQueueLimit();
      this._persistQueue();
      this._emitDelivery();
      this.emit('error', {
        code: 'BATCH_SEND_FAILED',
        message: err instanceof Error ? err.message : 'Batch send failed',
        recoverable: true,
      });
    } finally {
      this.flushInFlight = false;
    }
  }

  private async _sendHeartbeat(): Promise<void> {
    try {
      const encoded = encodeHeartbeatRequest({
        sessionId: this.claims.session_id,
        studentId: this.claims.student_id,
        examId: this.claims.exam_id,
        clientTimestamp: new Date().toISOString(),
        currentFocusScore: this._focusScore,
        violationCount: this._violationCount,
      });
      const response = await this.transport.call(
        EVENT_COLLECTOR_SERVICE_PATHS.heartbeat,
        JSON.stringify(encoded),
        { Authorization: `Bearer ${this.token}` },
      );
      const decoded = this._decodeHeartbeatResponse(response.data);
      if (decoded.directive?.terminate) {
        this.emit('error', {
          code: 'SESSION_TERMINATED_BY_SERVER',
          message: decoded.directive.terminateReason || 'Session terminated by server directive',
          recoverable: false,
        });
        await this.stop();
        return;
      }
      if (decoded.sessionActive === false) {
        this.emit('error', {
          code: 'SESSION_INACTIVE',
          message: 'Server reported that the session is inactive',
          recoverable: false,
        });
      }
      this.heartbeatSeqNum++;
      this.emit('heartbeat', { sequenceNum: this.heartbeatSeqNum });
    } catch {
      // Heartbeat failures are non-fatal — next one will try again.
    }
  }

  private _generateEventId(): string {
    const ts = Date.now().toString(36);
    const rand = Math.random().toString(36).substring(2, 8);
    return `${ts}-${rand}`;
  }

  private _parseToken(token: string): TokenClaims {
    try {
      const parts = token.split('.');
      if (parts.length !== 3) throw new Error('Invalid JWT format');
      const payload = JSON.parse(this._decodeBase64Url(parts[1]));
      return {
        session_id: payload.session_id || payload.sessionId || '',
        student_id: payload.student_id || payload.studentId || '',
        exam_id: payload.exam_id || payload.examId || '',
        org_id: payload.org_id || payload.orgId || '',
        exp: payload.exp || 0,
      };
    } catch {
      throw new Error('Failed to parse session token. Ensure it is a valid JWT.');
    }
  }

  private _decodeBase64Url(input: string): string {
    const normalized = input.replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized.padEnd(normalized.length + ((4 - normalized.length % 4) % 4), '=');
    return atob(padded);
  }

  private _decodeBatchResponse(data: unknown): IngestBatchResponse {
    if (!data || typeof data !== 'object') {
      return { acceptedCount: 0, rejectedCount: 0 };
    }
    const decoded = decodeResponse<Partial<IngestBatchResponse>>(data as Record<string, unknown>);
    return {
      acceptedCount: decoded.acceptedCount ?? 0,
      rejectedCount: decoded.rejectedCount ?? 0,
      rejectedEventIds: decoded.rejectedEventIds,
      batchSequence: decoded.batchSequence,
    };
  }

  private _decodeHeartbeatResponse(data: unknown): HeartbeatResponse {
    if (!data || typeof data !== 'object') {
      return { sessionActive: true };
    }
    const decoded = decodeResponse<Partial<HeartbeatResponse>>(data as Record<string, unknown>);
    return {
      sessionActive: decoded.sessionActive ?? true,
      serverTimestamp: decoded.serverTimestamp,
      directive: decoded.directive,
    };
  }

  private _enforceQueueLimit(): void {
    while (this.eventBuffer.length > this.maxQueueSize) {
      const infoIndex = this.eventBuffer.findIndex(event => event.severity <= 1);
      const dropIndex = infoIndex >= 0 ? infoIndex : 0;
      this.eventBuffer.splice(dropIndex, 1);
      this.droppedEvents++;
    }
  }

  private _emitDelivery(inFlight = this.flushInFlight, response?: IngestBatchResponse): void {
    this.emit('delivery', {
      queueDepth: this.eventBuffer.length,
      inFlight,
      droppedEvents: this.droppedEvents,
      lastAcceptedCount: response?.acceptedCount,
      lastRejectedCount: response?.rejectedCount,
    });
  }

  private _restoreQueue(): void {
    if (!this.persistOfflineQueue || typeof window === 'undefined') return;
    try {
      const raw = window.sessionStorage.getItem(this.storageKey);
      if (!raw) return;
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return;
      this.eventBuffer = parsed
        .filter(item => item && typeof item === 'object')
        .slice(0, this.maxQueueSize) as ProctoringEvent[];
    } catch {
      // Corrupt sessionStorage should not prevent an exam session from starting.
      this.eventBuffer = [];
    }
  }

  private _persistQueue(): void {
    if (!this.persistOfflineQueue || typeof window === 'undefined') return;
    try {
      if (this.eventBuffer.length === 0) {
        window.sessionStorage.removeItem(this.storageKey);
        return;
      }
      window.sessionStorage.setItem(this.storageKey, JSON.stringify(this.eventBuffer));
    } catch {
      // Storage quota/privacy errors are non-fatal; in-memory queue remains active.
    }
  }
}
