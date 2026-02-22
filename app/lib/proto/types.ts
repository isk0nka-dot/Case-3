// =============================================================================
// Argus AI — Protobuf TypeScript Types
// =============================================================================
//
// Hand-crafted TypeScript types that mirror the event_collector.proto contract.
// These provide type-safe interfaces for the gRPC-Web client without requiring
// a protoc/buf code-generation step.
//
// Design decision: Hand-crafted vs Generated
//
//   Generated code (@connectrpc/connect, buf.build) is ideal for large proto
//   schemas that change frequently. For Argus, the proto schema is stable and
//   well-defined (40 event types, 9 payloads). Hand-crafted types provide:
//     1. Zero build toolchain — no protoc, buf, or protobuf-ts plugin needed
//     2. Exact TypeScript idioms — optional fields use `?` not `| undefined`
//     3. Human-readable — developers can read these without proto knowledge
//     4. Tree-shakeable — only import what you use
//
//   The trade-off: manual sync when proto changes. Mitigated by a CI check
//   that compares proto field numbers against these types (future TODO).
// =============================================================================

// ---------------------------------------------------------------------------
// Enums — Mirror proto enum definitions with string literal unions
// ---------------------------------------------------------------------------

/**
 * Event type classification — 40 proctoring event types across 8 categories.
 *
 * The numeric values match proto field numbers for wire-format compatibility
 * when encoding to gRPC-Web binary format.
 */
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

  // Audio events (20-23)
  VOICE_ACTIVITY = 20,
  AUDIO_ANOMALY = 21,
  AUDIO_PERIPHERY_DETECTED = 22,
  SMART_NOISE_CLASSIFIED = 23,

  // AI Audio events (24-26)
  WHISPER_DETECTED = 24,
  SECOND_SPEAKER_DETECTED = 25,
  AUDIO_PLAYBACK_DETECTED = 26,

  // Browser events (30-35)
  TAB_SWITCH = 30,
  COPY_PASTE_ATTEMPT = 31,
  PRINT_SCREEN_ATTEMPT = 32,
  CONTEXT_MENU_ATTEMPT = 33,
  FULLSCREEN_EXIT = 34,
  EXTERNAL_DISPLAY_DETECTED = 35,

  // Network events (40-41)
  VPN_PROXY_DETECTED = 40,
  SUSPICIOUS_NETWORK_DEVICE = 41,

  // Engagement Risk Signals (50-52)
  // NOTE: Previously labeled "Psychometry events". Re-classified as
  // non-diagnostic engagement risk signals for privacy compliance (v2.1).
  EMOTION_STRESS_SPIKE = 50,          // Legacy name retained for wire compat
  ENGAGEMENT_RISK_ELEVATED = 50,      // v2.1 alias — preferred label
  FOCUS_LOSS_DETECTED = 51,
  BLINK_PATTERN_ANOMALY = 52,

  // Kernel-level events (60-67)
  TYPING_DYNAMICS_ANOMALY = 60,
  HAND_CURSOR_DESYNC = 61,
  FORBIDDEN_PROCESS_DETECTED = 62,
  HARDWARE_DEVICE_ANOMALY = 63,
  REMOTE_ACCESS_DETECTED = 64,
  HARDWARE_ID_MISMATCH = 65,
  VIRTUAL_MONITOR_DETECTED = 66,
  VIRTUAL_MACHINE_DETECTED = 67,

  // Secondary Camera events (70-75)
  SIDECAM_DEVICE_DISPLACED = 70,
  SIDECAM_HANDS_OFF_DESK = 71,
  SIDECAM_BATTERY_CRITICAL = 72,
  SIDECAM_STREAM_DISCONNECTED = 73,
  SIDECAM_CALIBRATION_FAILED = 74,
  SIDECAM_THERMAL_THROTTLE = 75,

  // Backend AI deep scan events (80-85)
  BACKEND_AI_FACE_MISMATCH = 80,
  BACKEND_AI_SCREEN_REFLECTION = 81,
  BACKEND_AI_MICRO_EXPRESSION = 82,
  BACKEND_AI_HIDDEN_OBJECT = 83,
  BACKEND_AI_DEEPFAKE_DETECTED = 84,
  BACKEND_AI_VOICE_SYNTH = 85,

  // Telemetry (high frequency, low severity: 100-103)
  GAZE_TELEMETRY = 100,
  MOUSE_TELEMETRY = 101,
  KEYBOARD_TELEMETRY = 102,
  FOCUS_SCORE_UPDATE = 103,

  // AI Telemetry (continuous inference streams: 104-106)
  HEAD_POSE_TELEMETRY = 104,
  FACE_EMBEDDING_TELEMETRY = 105,
  AUDIO_LEVEL_TELEMETRY = 106
}

/** Severity classification for triage and alerting. */
export enum Severity {
  SEVERITY_UNSPECIFIED = 0,
  INFO = 1,
  WARNING = 2,
  CRITICAL = 3
}

/** Source camera/system that generated the event. */
export enum EventSource {
  SOURCE_UNSPECIFIED = 0,
  WEBCAM = 1,
  SIDE_CAMERA = 2,
  SYSTEM = 3,
  BROWSER = 4,
  KERNEL_AGENT = 5,
  NETWORK_PROBE = 6,
  BACKEND_AI = 7
}

/** Telemetry frequency mode — server-controlled via SessionDirective. */
export enum TelemetryMode {
  TELEMETRY_NORMAL = 0,
  TELEMETRY_HIGH_FREQ = 1,
  TELEMETRY_LOW_FREQ = 2
}

// ---------------------------------------------------------------------------
// Human-readable labels — for UI display
// ---------------------------------------------------------------------------

/** Human-readable event type labels for dashboard display (Russian). */
export const EVENT_TYPE_LABELS: Record<EventType, string> = {
  [EventType.EVENT_TYPE_UNSPECIFIED]: 'Неизвестно',
  [EventType.GAZE_DEVIATION]: 'Отклонение взгляда',
  [EventType.FACE_MISMATCH]: 'Несоответствие лица',
  [EventType.FACE_NOT_DETECTED]: 'Лицо не обнаружено',
  [EventType.FACE_SPOOF_DETECTED]: 'Подмена лица',
  [EventType.MULTIPLE_PERSONS]: 'Несколько человек',
  [EventType.DYNAMIC_FACE_RECHECK_FAIL]: 'Повторная проверка не пройдена',
  [EventType.HEAD_POSE_ANOMALY]: 'Аномалия положения головы',
  [EventType.LIVENESS_CHECK_FAILED]: 'Проверка живости не пройдена',
  [EventType.FACE_OCCLUDED]: 'Лицо частично закрыто',
  [EventType.PHONE_DETECTED]: 'Обнаружен телефон',
  [EventType.BOOK_DETECTED]: 'Обнаружена книга',
  [EventType.EARBUDS_DETECTED]: 'Обнаружены наушники',
  [EventType.UNKNOWN_OBJECT_DETECTED]: 'Неизвестный объект',
  [EventType.VOICE_ACTIVITY]: 'Голосовая активность',
  [EventType.AUDIO_ANOMALY]: 'Аудио аномалия',
  [EventType.AUDIO_PERIPHERY_DETECTED]: 'Аудио периферия',
  [EventType.SMART_NOISE_CLASSIFIED]: 'Классификация шума',
  [EventType.WHISPER_DETECTED]: 'Обнаружен шёпот',
  [EventType.SECOND_SPEAKER_DETECTED]: 'Обнаружен второй голос',
  [EventType.AUDIO_PLAYBACK_DETECTED]: 'Воспроизведение аудио',
  [EventType.TAB_SWITCH]: 'Переключение вкладок',
  [EventType.COPY_PASTE_ATTEMPT]: 'Попытка копирования',
  [EventType.PRINT_SCREEN_ATTEMPT]: 'Попытка скриншота',
  [EventType.CONTEXT_MENU_ATTEMPT]: 'Контекстное меню',
  [EventType.FULLSCREEN_EXIT]: 'Выход из полного экрана',
  [EventType.EXTERNAL_DISPLAY_DETECTED]: 'Внешний монитор',
  [EventType.VPN_PROXY_DETECTED]: 'VPN/Прокси обнаружен',
  [EventType.SUSPICIOUS_NETWORK_DEVICE]: 'Пассивный сетевой анализ',
  [EventType.EMOTION_STRESS_SPIKE]: 'Повышенный риск вовлечённости (не диагностический)',
  [EventType.FOCUS_LOSS_DETECTED]: 'Потеря фокуса',
  [EventType.BLINK_PATTERN_ANOMALY]: 'Аномалия паттерна внимания',
  [EventType.TYPING_DYNAMICS_ANOMALY]: 'Аномалия набора',
  [EventType.HAND_CURSOR_DESYNC]: 'Десинхронизация курсора',
  [EventType.FORBIDDEN_PROCESS_DETECTED]: 'Запрещённый процесс',
  [EventType.HARDWARE_DEVICE_ANOMALY]: 'Аномалия устройства',
  [EventType.REMOTE_ACCESS_DETECTED]: 'Удалённый доступ',
  [EventType.HARDWARE_ID_MISMATCH]: 'Токен аттестации устройства',
  [EventType.VIRTUAL_MONITOR_DETECTED]: 'Виртуальный монитор',
  [EventType.VIRTUAL_MACHINE_DETECTED]: 'Виртуальная машина',
  [EventType.SIDECAM_DEVICE_DISPLACED]: 'Боковая камера смещена',
  [EventType.SIDECAM_HANDS_OFF_DESK]: 'Руки вне стола',
  [EventType.SIDECAM_BATTERY_CRITICAL]: 'Критический заряд камеры',
  [EventType.SIDECAM_STREAM_DISCONNECTED]: 'Боковая камера отключена',
  [EventType.SIDECAM_CALIBRATION_FAILED]: 'Калибровка камеры не пройдена',
  [EventType.SIDECAM_THERMAL_THROTTLE]: 'Перегрев боковой камеры',
  [EventType.BACKEND_AI_FACE_MISMATCH]: 'AI: Несоответствие лица',
  [EventType.BACKEND_AI_SCREEN_REFLECTION]: 'AI: Отражение на экране',
  [EventType.BACKEND_AI_MICRO_EXPRESSION]: 'AI: Микро-выражение',
  [EventType.BACKEND_AI_HIDDEN_OBJECT]: 'AI: Скрытый объект',
  [EventType.BACKEND_AI_DEEPFAKE_DETECTED]: 'AI: Дипфейк обнаружен',
  [EventType.BACKEND_AI_VOICE_SYNTH]: 'AI: Синтез голоса',
  [EventType.GAZE_TELEMETRY]: 'Телеметрия взгляда',
  [EventType.MOUSE_TELEMETRY]: 'Телеметрия мыши',
  [EventType.KEYBOARD_TELEMETRY]: 'Телеметрия клавиатуры',
  [EventType.FOCUS_SCORE_UPDATE]: 'Обновление фокуса',
  [EventType.HEAD_POSE_TELEMETRY]: 'Телеметрия положения головы',
  [EventType.FACE_EMBEDDING_TELEMETRY]: 'Телеметрия биометрии лица',
  [EventType.AUDIO_LEVEL_TELEMETRY]: 'Телеметрия аудио уровня'
}

/** Human-readable severity labels. */
export const SEVERITY_LABELS: Record<Severity, string> = {
  [Severity.SEVERITY_UNSPECIFIED]: 'Неизвестно',
  [Severity.INFO]: 'Информация',
  [Severity.WARNING]: 'Предупреждение',
  [Severity.CRITICAL]: 'Критический'
}

/** CSS color classes for severity badges. */
export const SEVERITY_COLORS: Record<Severity, string> = {
  [Severity.SEVERITY_UNSPECIFIED]: 'text-gray-400',
  [Severity.INFO]: 'text-blue-400',
  [Severity.WARNING]: 'text-amber-400',
  [Severity.CRITICAL]: 'text-red-400'
}

/** Severity badge background classes for Nuxt UI. */
export const SEVERITY_BADGE_VARIANTS: Record<Severity, string> = {
  [Severity.SEVERITY_UNSPECIFIED]: 'subtle',
  [Severity.INFO]: 'subtle',
  [Severity.WARNING]: 'subtle',
  [Severity.CRITICAL]: 'solid'
}

/** Event source labels. */
export const EVENT_SOURCE_LABELS: Record<EventSource, string> = {
  [EventSource.SOURCE_UNSPECIFIED]: 'Неизвестно',
  [EventSource.WEBCAM]: 'Веб-камера',
  [EventSource.SIDE_CAMERA]: 'Боковая камера',
  [EventSource.SYSTEM]: 'Система',
  [EventSource.BROWSER]: 'Браузер',
  [EventSource.KERNEL_AGENT]: 'Ядро',
  [EventSource.NETWORK_PROBE]: 'Сеть',
  [EventSource.BACKEND_AI]: 'Серверный AI'
}

// ---------------------------------------------------------------------------
// Payload Types — type-specific structured data
// ---------------------------------------------------------------------------

export interface GazeDeviationPayload {
  direction: string // "left" | "right" | "up" | "down"
  durationMs: number
  angleDegrees: number
  gazeX: number
  gazeY: number
}

export interface FaceDetectionPayload {
  match: boolean
  similarity: number
  faceCount: number
  isSpoof: boolean
  spoofType?: string
}

export interface ObjectDetectionPayload {
  objectType: string
  bboxX: number
  bboxY: number
  bboxW: number
  bboxH: number
  detectionConfidence: number
}

export interface AudioPayload {
  dbLevel: number
  speechDetected: boolean
  speakerCount: number
  noiseSource?: string
  durationMs: number
}

export interface BrowserPayload {
  tabSwitchCount: number
  targetInfo?: string
  fullscreenExited: boolean
  displayCount: number
  action?: string
}

export interface SystemPayload {
  processName: string
  processId: number
  terminated: boolean
  category: string
}

export interface PsychometryPayload {
  emotion: string
  intensity: number
  focusScore: number
  blinkRate: number
  blinkAnomaly: boolean
}

export interface NetworkPayload {
  detectionType: string
  networkHash: string
  suspiciousDeviceCount: number
  provider?: string
}

export interface KernelPayload {
  wpm: number
  keystrokeStdDevMs: number
  handOnMouse: boolean
  hardwareIdHash?: string
  mismatchComponent?: string
  monitorCount: number
  virtualMonitor: boolean
}

// ---------------------------------------------------------------------------
// AI Vision Payload Types
// ---------------------------------------------------------------------------

/** Head pose estimation from 3D face landmark regression. */
export interface HeadPosePayload {
  yaw: number        // degrees, negative=left, positive=right
  pitch: number      // degrees, negative=down, positive=up
  roll: number       // degrees, negative=tilt left, positive=tilt right
  faceX: number      // bbox normalized 0-1
  faceY: number
  faceW: number
  faceH: number
  ipdPx: number      // inter-pupillary distance in pixels
  landmarkCount: number
  inferenceMs: number
}

/** Multi-factor liveness verification result. */
export interface LivenessPayload {
  livenessScore: number       // 0=spoof, 1=real
  blinkDetected: boolean
  blinkRatePerMin: number
  textureScore: number        // detects photos/screen replays
  depthScore: number          // detects flat surfaces
  spoofVector: string         // "photo", "screen_replay", "mask", "deepfake", "none"
  frameQuality: number        // 0-1
}

/** 512-dim face embedding for continuous identity verification. */
export interface FaceEmbeddingPayload {
  embedding: number[]         // 512-dimensional vector
  similarity: number          // cosine similarity to enrolled reference
  identityMatch: boolean      // >0.65 threshold
  modelVersion: string
  alignmentQuality: number    // 0-1
}

// ---------------------------------------------------------------------------
// AI Audio Payload Types
// ---------------------------------------------------------------------------

/** Audio analysis — SAD + anomaly classification. All processing is local. */
export interface AudioAnalysisPayload {
  rmsDb: number                     // A-weighted RMS dB
  vadActive: boolean                // Voice Activity Detection
  vadConfidence: number             // 0-1
  spectralCentroidHz: number        // frequency centroid
  zcr: number                       // zero-crossing rate
  classification: string            // "silence"|"speech"|"whisper"|"music"|"keyboard"|"ambient"
  classificationConfidence: number  // 0-1
  speakerCount: number
  speakerMatch: boolean             // matches enrolled voiceprint
  speakerSimilarity: number         // cosine similarity to reference
  segmentDurationMs: number
}

/** Union type for all event payloads. */
export type EventPayload =
  | { type: 'gazeDeviation'; data: GazeDeviationPayload }
  | { type: 'faceDetection'; data: FaceDetectionPayload }
  | { type: 'objectDetection'; data: ObjectDetectionPayload }
  | { type: 'audio'; data: AudioPayload }
  | { type: 'browser'; data: BrowserPayload }
  | { type: 'system'; data: SystemPayload }
  | { type: 'psychometry'; data: PsychometryPayload }
  | { type: 'network'; data: NetworkPayload }
  | { type: 'kernel'; data: KernelPayload }
  | { type: 'headPose'; data: HeadPosePayload }
  | { type: 'liveness'; data: LivenessPayload }
  | { type: 'faceEmbedding'; data: FaceEmbeddingPayload }
  | { type: 'audioAnalysis'; data: AudioAnalysisPayload }

// ---------------------------------------------------------------------------
// Client Metadata
// ---------------------------------------------------------------------------

export interface ClientMeta {
  userAgent: string
  sdkVersion: string
  resolution: string
  timezoneOffsetMin: number
  ipAddress?: string // Set server-side
  region?: string // Set server-side
}

// ---------------------------------------------------------------------------
// Core Event Message
// ---------------------------------------------------------------------------

/**
 * ProctoringEvent — the canonical event envelope.
 *
 * This is the TypeScript equivalent of the proto ProctoringEvent message.
 * Every field maps 1:1 to the proto definition with camelCase naming.
 */
export interface ProctoringEvent {
  eventId: string
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  eventType: EventType
  severity: Severity
  source: EventSource
  serverTimestamp?: string // ISO 8601
  clientTimestamp: string // ISO 8601
  videoTimestampSec?: number
  label: string
  confidence: number
  payload?: EventPayload
  clientMeta?: ClientMeta
}

// ---------------------------------------------------------------------------
// Request / Response Messages
// ---------------------------------------------------------------------------

export interface IngestEventRequest {
  event: ProctoringEvent
}

export interface IngestEventResponse {
  accepted: boolean
  eventId: string
  sequence: number
}

export interface IngestBatchRequest {
  events: ProctoringEvent[]
  batchId: string
}

export interface IngestBatchResponse {
  acceptedCount: number
  rejectedCount: number
  rejectedEventIds: string[]
  batchSequence: number
}

export interface StreamAck {
  eventId: string
  sequence: number
  accepted: boolean
}

export interface HeartbeatRequest {
  sessionId: string
  studentId: string
  examId: string
  clientTimestamp: string // ISO 8601
  currentFocusScore: number
  violationCount: number
}

export interface HeartbeatResponse {
  sessionActive: boolean
  serverTimestamp: string // ISO 8601
  directive?: SessionDirective
}

export interface SessionDirective {
  telemetryMode: TelemetryMode
  terminate: boolean
  terminateReason?: string
}

// ---------------------------------------------------------------------------
// Event Classification Helpers
// ---------------------------------------------------------------------------

/** Event types that are CRITICAL violations — require immediate proctor attention. */
export const CRITICAL_EVENT_TYPES: ReadonlySet<EventType> = new Set([
  EventType.FACE_MISMATCH,
  EventType.FACE_SPOOF_DETECTED,
  EventType.MULTIPLE_PERSONS,
  EventType.PHONE_DETECTED,
  EventType.REMOTE_ACCESS_DETECTED,
  EventType.VIRTUAL_MACHINE_DETECTED,
  EventType.VPN_PROXY_DETECTED,
  EventType.LIVENESS_CHECK_FAILED,
  EventType.SECOND_SPEAKER_DETECTED
])

/** High-frequency telemetry events — batched, not individually displayed. */
export const TELEMETRY_EVENT_TYPES: ReadonlySet<EventType> = new Set([
  EventType.GAZE_TELEMETRY,
  EventType.MOUSE_TELEMETRY,
  EventType.KEYBOARD_TELEMETRY,
  EventType.FOCUS_SCORE_UPDATE,
  EventType.HEAD_POSE_TELEMETRY,
  EventType.FACE_EMBEDDING_TELEMETRY,
  EventType.AUDIO_LEVEL_TELEMETRY
])

/** Check if an event type is a telemetry event (high-frequency, low-severity). */
export function isTelemetryEvent(eventType: EventType): boolean {
  return TELEMETRY_EVENT_TYPES.has(eventType)
}

/** Check if an event type is critical (requires immediate attention). */
export function isCriticalEvent(eventType: EventType): boolean {
  return CRITICAL_EVENT_TYPES.has(eventType)
}

/** Get the event category string for grouping. */
export function getEventCategory(eventType: EventType): string {
  const val = eventType as number
  if (val >= 1 && val <= 9) return 'video'
  if (val >= 10 && val <= 13) return 'object'
  if (val >= 20 && val <= 26) return 'audio'
  if (val >= 30 && val <= 35) return 'browser'
  if (val >= 40 && val <= 41) return 'network'
  if (val >= 50 && val <= 52) return 'psychometry'
  if (val >= 60 && val <= 67) return 'kernel'
  if (val >= 70 && val <= 75) return 'sidecam'
  if (val >= 80 && val <= 85) return 'backend_ai'
  if (val >= 100 && val <= 106) return 'telemetry'
  return 'unknown'
}
