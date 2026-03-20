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
} from '../types';
import { GrpcWebTransport } from './transport';
import { encodeIngestBatchRequest, encodeHeartbeatRequest } from './codec';
import { EventEmitter } from './event-emitter';

/** Session events emitted by the session manager. */
export interface SessionEvents {
  statusChange: SessionStatus;
  violation: ViolationEvent;
  error: SDKError;
  heartbeat: { sequenceNum: number };
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

  constructor(
    transport: GrpcWebTransport,
    token: string,
    options?: {
      batchSize?: number;
      flushIntervalMs?: number;
      heartbeatIntervalMs?: number;
    },
  ) {
    super();
    this.transport = transport;
    this.token = token;
    this.claims = this._parseToken(token);
    this.batchSize = options?.batchSize ?? 50;
    this.flushIntervalMs = options?.flushIntervalMs ?? 2000;
    this.heartbeatIntervalMs = options?.heartbeatIntervalMs ?? 30_000;
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

  // ---------------------------------------------------------------------------
  // Lifecycle
  // ---------------------------------------------------------------------------

  /** Start the session — begins event batching and heartbeat. */
  start(): void {
    if (this._status === 'active') return;

    this._setStatus('active');

    // Start flush timer.
    this.flushTimer = setInterval(() => this._flush(), this.flushIntervalMs);

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

    this._setStatus('completed');
  }

  /** Destroy the session — releases all resources. */
  destroy(): void {
    if (this.flushTimer) { clearInterval(this.flushTimer); this.flushTimer = null; }
    if (this.heartbeatTimer) { clearInterval(this.heartbeatTimer); this.heartbeatTimer = null; }
    this.eventBuffer = [];
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

    // Track violations (WARNING + CRITICAL).
    if (severity >= 2) {
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

  /** Update focus score (called from health governor). */
  updateFocusScore(score: number): void {
    this._focusScore = Math.max(0, Math.min(100, score));
  }

  // ---------------------------------------------------------------------------
  // Private
  // ---------------------------------------------------------------------------

  private _setStatus(status: SessionStatus): void {
    if (this._status === status) return;
    this._status = status;
    this.emit('statusChange', status);
  }

  private async _flush(): Promise<void> {
    if (this.eventBuffer.length === 0) return;

    const batch = this.eventBuffer.splice(0, this.batchSize);
    const batchId = `b-${++this.batchCounter}-${Date.now()}`;

    try {
      const encoded = encodeIngestBatchRequest({ events: batch, batchId });
      await this.transport.call(
        '/argus.v1.EventCollector/IngestBatch',
        JSON.stringify(encoded),
        { Authorization: `Bearer ${this.token}` },
      );
    } catch (err) {
      // Re-queue failed events at the front of the buffer.
      this.eventBuffer.unshift(...batch);
      this.emit('error', {
        code: 'BATCH_SEND_FAILED',
        message: err instanceof Error ? err.message : 'Batch send failed',
        recoverable: true,
      });
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
      await this.transport.call(
        '/argus.v1.EventCollector/Heartbeat',
        JSON.stringify(encoded),
        { Authorization: `Bearer ${this.token}` },
      );
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
      const payload = JSON.parse(atob(parts[1]));
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
}
