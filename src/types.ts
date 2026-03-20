// =============================================================================
// Argus AI SDK — Public Types
// =============================================================================
//
// Subset of the internal proto types needed by external SDK consumers.
// Mirrors event_collector.proto for wire-format compatibility.
// =============================================================================

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

/** Event type classification — proctoring event types. */
export enum EventType {
  EVENT_TYPE_UNSPECIFIED = 0,

  // Video & Face events (1-6)
  GAZE_DEVIATION = 1,
  FACE_MISMATCH = 2,
  FACE_NOT_DETECTED = 3,
  FACE_SPOOF_DETECTED = 4,
  MULTIPLE_PERSONS = 5,
  DYNAMIC_FACE_RECHECK_FAIL = 6,

  // AI Vision events (7-9)
  HEAD_POSE_ANOMALY = 7,
  LIVENESS_CHECK_FAILED = 8,
  FACE_OCCLUDED = 9,

  // Object detection events (10-13)
  PHONE_DETECTED = 10,
  BOOK_DETECTED = 11,
  EARBUDS_DETECTED = 12,
  UNKNOWN_OBJECT_DETECTED = 13,

  // Audio events (20-26)
  VOICE_ACTIVITY = 20,
  AUDIO_ANOMALY = 21,
  AUDIO_PERIPHERY_DETECTED = 22,
  SMART_NOISE_CLASSIFIED = 23,
  WHISPER_DETECTED = 24,
  SECOND_SPEAKER_DETECTED = 25,
  AUDIO_PLAYBACK_DETECTED = 26,

  // Browser events (30-36)
  TAB_SWITCH = 30,
  COPY_PASTE_ATTEMPT = 31,
  PRINT_SCREEN_ATTEMPT = 32,
  CONTEXT_MENU_ATTEMPT = 33,
  FULLSCREEN_EXIT = 34,
  EXTERNAL_DISPLAY_DETECTED = 35,
  CAMERA_BLOCKED = 36,

  // Telemetry (100-106)
  GAZE_TELEMETRY = 100,
  HEAD_POSE_TELEMETRY = 104,
  AUDIO_LEVEL_TELEMETRY = 106,
}

/** Severity classification for triage and alerting. */
export enum Severity {
  SEVERITY_UNSPECIFIED = 0,
  INFO = 1,
  WARNING = 2,
  CRITICAL = 3,
}

/** Source camera/system that generated the event. */
export enum EventSource {
  SOURCE_UNSPECIFIED = 0,
  WEBCAM = 1,
  SIDE_CAMERA = 2,
  SYSTEM = 3,
  BROWSER = 4,
}

/** Telemetry frequency mode — server-controlled. */
export enum TelemetryMode {
  TELEMETRY_NORMAL = 0,
  TELEMETRY_HIGH_FREQ = 1,
  TELEMETRY_LOW_FREQ = 2,
}

// ---------------------------------------------------------------------------
// SDK Configuration
// ---------------------------------------------------------------------------

/** Configuration for the ArgusSDK. */
export interface ArgusSDKConfig {
  /** JWT session token from POST /v1/external/sessions. */
  sessionToken: string;

  /** Argus server URL (e.g., 'https://argusai.kz'). */
  serverUrl: string;

  /** Optional DOM element ID to mount the camera widget. Default: floating. */
  containerId?: string;

  /** Locale for UI strings. Default: 'ru'. */
  locale?: 'kk' | 'ru' | 'en';

  /** MediaPipe WASM/model CDN base URL. Default: jsdelivr CDN. */
  mediapipeBasePath?: string;

  /** Callback: SDK ready (camera + network initialized). */
  onReady?: () => void;

  /** Callback: unrecoverable error. */
  onError?: (error: SDKError) => void;

  /** Callback: violation detected. */
  onViolation?: (event: ViolationEvent) => void;

  /** Callback: session status changed. */
  onStatusChange?: (status: SessionStatus) => void;

  /** Callback: preflight step completed. */
  onPreflightStep?: (step: PreflightStep) => void;
}

// ---------------------------------------------------------------------------
// Events & Payloads
// ---------------------------------------------------------------------------

/** Proctoring event sent to the server. */
export interface ProctoringEvent {
  eventId: string;
  sessionId: string;
  studentId: string;
  examId: string;
  orgId: string;
  eventType: EventType;
  severity: Severity;
  source: EventSource;
  clientTimestamp: string;
  label: string;
  confidence: number;
  payload?: EventPayload;
}

/** Typed event payload wrapper. */
export interface EventPayload {
  type: string;
  data: Record<string, unknown>;
}

/** Batch ingest request. */
export interface IngestBatchRequest {
  events: ProctoringEvent[];
  batchId: string;
  signal?: AbortSignal;
}

/** Heartbeat request. */
export interface HeartbeatRequest {
  sessionId: string;
  studentId: string;
  examId: string;
  clientTimestamp: string;
  currentFocusScore: number;
  violationCount: number;
}

// ---------------------------------------------------------------------------
// SDK State
// ---------------------------------------------------------------------------

/** Session lifecycle status. */
export type SessionStatus =
  | 'idle'
  | 'initializing'
  | 'preflight'
  | 'ready'
  | 'active'
  | 'paused'
  | 'completed'
  | 'error';

/** Preflight check result. */
export interface PreflightStep {
  id: 'hardware' | 'environment' | 'system';
  status: 'pending' | 'checking' | 'passed' | 'failed';
  message: string;
  details?: string;
}

/** Violation event emitted to the partner. */
export interface ViolationEvent {
  type: EventType;
  severity: Severity;
  label: string;
  confidence: number;
  timestamp: string;
}

/** SDK error object. */
export interface SDKError {
  code: string;
  message: string;
  recoverable: boolean;
}

// ---------------------------------------------------------------------------
// Health & Tiers
// ---------------------------------------------------------------------------

/** Resilience tier based on device/network health. */
export type ResilienceTier = 'A' | 'B' | 'C';

/** Health metrics snapshot. */
export interface HealthMetrics {
  fps: number;
  rttMs: number;
  packetLossRate: number;
  score: number;
  tier: ResilienceTier;
  cpuPressure: 'nominal' | 'fair' | 'serious' | 'critical';
}

/** Tier configuration applied per tier level. */
export interface TierConfig {
  video: { width: number; height: number; fps: number };
  ai: { intervalMs: number; enabled: boolean };
  telemetry: { sampleIntervalMs: number };
}

/** Static tier configuration map. */
export const TIER_CONFIGS: Record<ResilienceTier, TierConfig> = {
  A: {
    video: { width: 1280, height: 720, fps: 30 },
    ai: { intervalMs: 100, enabled: true },
    telemetry: { sampleIntervalMs: 1000 },
  },
  B: {
    video: { width: 320, height: 240, fps: 15 },
    ai: { intervalMs: 200, enabled: true },
    telemetry: { sampleIntervalMs: 3000 },
  },
  C: {
    video: { width: 160, height: 120, fps: 5 },
    ai: { intervalMs: 0, enabled: false },
    telemetry: { sampleIntervalMs: 10000 },
  },
};

// ---------------------------------------------------------------------------
// Vision Frame (output from face detection)
// ---------------------------------------------------------------------------

/** Frame data from the vision engine (MediaPipe face detection). */
export interface VisionFrame {
  faceDetected: boolean;
  faceCount: number;
  faceBbox?: { x: number; y: number; w: number; h: number };
  headPose?: { yaw: number; pitch: number; roll: number };
  gazeDirection?: { x: number; y: number };
  eyeAspectRatio?: { left: number; right: number };
  blinkDetected: boolean;
  quality: number;
  timestamp: number;
}

// ---------------------------------------------------------------------------
// Audio Frame (output from audio engine)
// ---------------------------------------------------------------------------

/** Frame data from the audio engine (Web Audio API). */
export interface AudioFrame {
  rmsDb: number;
  peakDb: number;
  zcr: number;
  spectralCentroid: number;
  voiceActivityDetected: boolean;
  speakerCount: number;
  classification: 'silence' | 'speech' | 'noise' | 'music';
  timestamp: number;
}

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

/** Transport configuration. */
export interface TransportConfig {
  baseUrl: string;
  timeout?: number;
  maxRetries?: number;
  retryBaseDelay?: number;
  retryMaxDelay?: number;
}

/** Request interceptor. */
export interface TransportInterceptor {
  beforeRequest?(request: TransportRequest): TransportRequest | Promise<TransportRequest>;
  afterResponse?(response: TransportResponse): TransportResponse | Promise<TransportResponse>;
  onError?(error: Error): void;
}

/** Internal request representation. */
export interface TransportRequest {
  url: string;
  method: 'POST';
  headers: Record<string, string>;
  body: string;
  signal?: AbortSignal;
}

/** Internal response representation. */
export interface TransportResponse {
  status: number;
  headers: Record<string, string>;
  data: unknown;
}
