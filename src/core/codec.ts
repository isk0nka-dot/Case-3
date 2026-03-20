// =============================================================================
// Argus SDK — Proto JSON Codec
// =============================================================================
//
// Bidirectional conversion between TypeScript interfaces and the JSON encoding
// used by the gRPC-Web server. Extracted from argus-frontend/app/lib/proto/codec.ts.
// =============================================================================

import type {
  ProctoringEvent,
  IngestBatchRequest,
  HeartbeatRequest,
  EventPayload,
} from '../types';

// ---------------------------------------------------------------------------
// Case conversion utilities
// ---------------------------------------------------------------------------

function toSnakeCase(str: string): string {
  return str.replace(/[A-Z]/g, letter => `_${letter.toLowerCase()}`);
}

function toCamelCase(str: string): string {
  return str.replace(/_([a-z])/g, (_, letter: string) => letter.toUpperCase());
}

export function keysToSnakeCase(obj: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const key of Object.keys(obj)) {
    const snakeKey = toSnakeCase(key);
    const value = obj[key];
    if (value !== null && value !== undefined) {
      if (Array.isArray(value)) {
        result[snakeKey] = value.map(item =>
          typeof item === 'object' && item !== null
            ? keysToSnakeCase(item as Record<string, unknown>)
            : item,
        );
      } else if (typeof value === 'object') {
        result[snakeKey] = keysToSnakeCase(value as Record<string, unknown>);
      } else {
        result[snakeKey] = value;
      }
    }
  }
  return result;
}

export function keysToCamelCase(obj: Record<string, unknown>): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const key of Object.keys(obj)) {
    const camelKey = toCamelCase(key);
    const value = obj[key];
    if (value !== null && value !== undefined) {
      if (Array.isArray(value)) {
        result[camelKey] = value.map(item =>
          typeof item === 'object' && item !== null
            ? keysToCamelCase(item as Record<string, unknown>)
            : item,
        );
      } else if (typeof value === 'object') {
        result[camelKey] = keysToCamelCase(value as Record<string, unknown>);
      } else {
        result[camelKey] = value;
      }
    }
  }
  return result;
}

// ---------------------------------------------------------------------------
// Payload encoding
// ---------------------------------------------------------------------------

const PAYLOAD_TYPE_MAP: Record<string, string> = {
  gazeDeviation: 'gaze_deviation',
  faceDetection: 'face_detection',
  objectDetection: 'object_detection',
  audio: 'audio',
  browser: 'browser',
  system: 'system',
  psychometry: 'psychometry',
  network: 'network',
  kernel: 'kernel',
  headPose: 'head_pose',
  liveness: 'liveness',
  audioAnalysis: 'audio_analysis',
  faceEmbedding: 'face_embedding',
};

function encodePayload(payload: EventPayload): Record<string, unknown> {
  const protoKey = PAYLOAD_TYPE_MAP[payload.type];
  if (!protoKey) return {};
  return { [protoKey]: keysToSnakeCase(payload.data) };
}

function toProtoTimestamp(isoString: string): string {
  return new Date(isoString).toISOString();
}

// ---------------------------------------------------------------------------
// Public encoding functions
// ---------------------------------------------------------------------------

/** Encode a ProctoringEvent to Proto3 JSON. */
export function encodeProctoringEvent(event: ProctoringEvent): Record<string, unknown> {
  const proto: Record<string, unknown> = {
    event_id: event.eventId,
    session_id: event.sessionId,
    student_id: event.studentId,
    exam_id: event.examId,
    org_id: event.orgId,
    event_type: event.eventType,
    severity: event.severity,
    source: event.source,
    client_timestamp: toProtoTimestamp(event.clientTimestamp),
    label: event.label,
    confidence: event.confidence,
  };

  if (event.payload) {
    Object.assign(proto, encodePayload(event.payload));
  }

  return proto;
}

/** Encode an IngestBatchRequest to Proto3 JSON. */
export function encodeIngestBatchRequest(req: IngestBatchRequest): Record<string, unknown> {
  return {
    events: req.events.map(encodeProctoringEvent),
    batch_id: req.batchId,
  };
}

/** Encode a HeartbeatRequest to Proto3 JSON. */
export function encodeHeartbeatRequest(req: HeartbeatRequest): Record<string, unknown> {
  return {
    session_id: req.sessionId,
    student_id: req.studentId,
    exam_id: req.examId,
    client_timestamp: toProtoTimestamp(req.clientTimestamp),
    current_focus_score: req.currentFocusScore,
    violation_count: req.violationCount,
  };
}

/** Decode a Proto3 JSON response, converting snake_case keys to camelCase. */
export function decodeResponse<T>(json: Record<string, unknown>): T {
  return keysToCamelCase(json) as T;
}
