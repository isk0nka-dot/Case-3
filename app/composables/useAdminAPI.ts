// =============================================================================
// Argus AI — Admin REST API Client
// =============================================================================
//
// Provides typed composable for the admin SaaS management API.
// Handles authentication, organization, user, and API key CRUD operations.
//
// All endpoints require JWT Bearer authentication (except /auth/login).
// The token is automatically injected from useAuthStore.
//
// Base URL is configurable via NUXT_PUBLIC_GRPC_URL runtime config.
// =============================================================================

import { useAuthStore } from '~/stores/useAuthStore'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Organization {
  id: string
  orgId: string
  name: string
  slug: string
  orgType: string
  contactEmail?: string
  contactPhone?: string
  city?: string
  region?: string
  plan: string
  maxSessions: number
  maxEventsRps: number
  retentionDays: number
  isActive: boolean
  createdAt: string
  updatedAt: string
  createdBy: string
  updatedBy: string
}

export interface User {
  id: string
  orgId: string
  phone: string
  fullName: string
  email?: string
  role: 'super_admin' | 'org_admin' | 'proctor' | 'viewer'
  isActive: boolean
  lastLoginAt?: string
  createdAt: string
  updatedAt: string
  createdBy: string
  updatedBy: string
}

export interface APIKey {
  id: string
  orgId: string
  name: string
  keyId: string
  secretPrefix: string
  permissions: string[]
  rateLimitRps: number
  isActive: boolean
  expiresAt?: string
  lastUsedAt?: string
  createdAt: string
  updatedAt: string
  createdBy: string
  updatedBy: string
}

export interface CreateOrgRequest {
  orgId: string
  name: string
  slug: string
  orgType: string
  contactEmail?: string
  contactPhone?: string
  city?: string
  region?: string
  plan?: string
  maxSessions?: number
  maxEventsRps?: number
}

export interface CreateOrgWithAdminRequest extends CreateOrgRequest {
  adminFullName: string
  adminPhone: string
  adminPassword: string
}

export interface CreateOrgWithAdminResponse {
  organization: Organization
  admin: User
}

export interface CreateUserRequest {
  phone: string
  password: string
  fullName: string
  email?: string
  role: string
}

export interface CreateAPIKeyRequest {
  name: string
  permissions?: string[]
  environment?: 'live' | 'test'
}

export interface CreateAPIKeyResponse {
  key: APIKey
  secret: string
}

export interface AdminStats {
  totalOrganizations: number
  totalUsers: number
  totalApiKeys: number
}

export interface AuditEntry {
  id: string
  userId: string
  userPhone: string
  userRole: string
  orgId: string
  action: string
  resourceType: string
  resourceId: string
  details: Record<string, unknown>
  ipAddress: string
  userAgent: string
  createdAt: string
}

export interface AuditLogParams {
  orgId?: string
  userId?: string
  action?: string
  resourceType?: string
  limit?: number
  offset?: number
}

// ---------------------------------------------------------------------------
// Analytics Types (Executive Dashboard)
// ---------------------------------------------------------------------------

export interface RiskDistribution {
  totalEvents: number
  bySeverity: Record<string, number>
  honest: number
  suspicious: number
  cheaters: number
}

export interface SystemHealthData {
  totalFlushed: number
  totalDropped: number
  flushCount: number
  flushErrors: number
  bufferSize: number
  overflowSize: number
  totalEvents: number
  uniqueStudents: number
  uniqueSessions: number
  criticalEvents: number
  eventsLast5Min: number
  eventsLast1Hour: number
}

export interface ViolationEntry {
  eventType: string
  severity: string
  count: number
}

export interface StudentRiskEntry {
  studentId: string
  totalEvents: number
  criticalCount: number
  warningCount: number
}

export interface HourlyStatEntry {
  hour: string
  eventCount: number
  criticalCount: number
}

export interface AnalyticsOverview {
  risk: RiskDistribution
  health: SystemHealthData
  violations: ViolationEntry[]
  topStudents: StudentRiskEntry[]
  hourlyStats: HourlyStatEntry[]
  generatedAt: string
}

// ---------------------------------------------------------------------------
// Infrastructure Stats Types
// ---------------------------------------------------------------------------

export interface InfraContainerStats {
  id: string
  name: string
  region: string
  cpuLoad: number
  memoryUsage: number
  diskUsage: number
  status: 'healthy' | 'warning' | 'critical'
  latencyMs: number
  uptime: number
  connections: number
  memUsedMB: number
  memLimitMB: number
}

export interface InfraNetworkMetric {
  label: string
  value: number
  unit: string
  status: 'healthy' | 'warning' | 'critical'
  trend: 'up' | 'down' | 'stable'
}

export interface InfraLatencyPoint {
  time: string
  avg: number
  p95: number
  p99: number
}

export interface InfrastructureStats {
  serverNodes: InfraContainerStats[]
  networkMetrics: InfraNetworkMetric[]
  latencyHistory: InfraLatencyPoint[]
  totalCapacity: number
  currentLoad: number
  loadPercent: number
  avgLatencyMs: number
  p99LatencyMs: number
  collectedAt: string
}

// ---------------------------------------------------------------------------
// Monitoring Types (Live Monitoring Dashboard)
// ---------------------------------------------------------------------------

export interface RecentViolation {
  eventId: string
  eventType: string
  severity: string
  label: string
  confidence: number
  timestamp: string
}

export interface ActiveSession {
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  totalEvents: number
  criticalCount: number
  warningCount: number
  infoCount: number
  riskScore: number
  violationLevel: 'critical' | 'warning' | 'clean'
  lastEventTime: string
  firstEventTime: string
  status: 'active' | 'terminated'
  recentViolations: RecentViolation[]
}

export interface ActiveSessionsResponse {
  sessions: ActiveSession[]
  totalActive: number
  totalCritical: number
  totalWarning: number
  totalClean: number
  avgRiskScore: number
  generatedAt: string
}

// ---------------------------------------------------------------------------
// Archive Types (Архив сессий)
// ---------------------------------------------------------------------------

export interface ArchiveSessionItem {
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  totalEvents: number
  criticalCount: number
  warningCount: number
  infoCount: number
  integrityScore: number
  violationCount: number
  firstEventTime: string
  lastEventTime: string
  durationSec: number
  duration: string
  status: 'reviewed' | 'pending' | 'voided'
}

export interface ArchiveSessionsResponse {
  sessions: ArchiveSessionItem[]
  total: number
  totalReviewed: number
  totalPending: number
  totalVoided: number
  generatedAt: string
}

export interface ArchiveEventItem {
  eventId: string
  eventType: string
  severity: 'critical' | 'warning' | 'info'
  source: string
  label: string
  confidence: number
  timestamp: string
  videoTimestamp: number
}

export interface ArchiveSessionEventsResponse {
  sessionId: string
  events: ArchiveEventItem[]
  totalEvents: number
  criticalCount: number
  warningCount: number
  durationSec: number
  integrityScore: number
  generatedAt: string
}

export interface ArchiveExamSummaryItem {
  examId: string
  orgId: string
  sessionCount: number
  totalEvents: number
  totalCritical: number
  totalWarning: number
  avgIntegrity: number
  firstEventTime: string
  lastEventTime: string
}

export interface ArchiveExamsResponse {
  exams: ArchiveExamSummaryItem[]
  total: number
  generatedAt: string
}

export interface ExportSessionResponse {
  session: ArchiveSessionItem
  events: ArchiveEventItem[]
  exportedAt: string
  exportedBy: string
  format: string
}

// ---------------------------------------------------------------------------
// Evidence Types (Video Evidence Fragments)
// ---------------------------------------------------------------------------

export interface EvidenceFragment {
  fragmentId: string
  sessionId: string
  eventId: string
  orgId: string
  examId: string
  studentId: string
  sha256Hash: string
  uri: string
  sizeBytes: number
  contentType: string
  durationSec: number
  startTime: string
  endTime: string
  uploadedAt: string
}

export interface EvidenceListResponse {
  sessionId: string
  fragments: EvidenceFragment[]
  total: number
}

export interface PresignedURLResponse {
  fragmentId: string
  url: string
  expiresAt: string
  ttlSeconds: number
}

export interface EvidenceVerifyResponse {
  fragmentId: string
  valid: boolean
  expectedHash: string
  actualHash: string
  verifiedAt: string
}

// ---------------------------------------------------------------------------
// Review Types (Human Review Workflow)
// ---------------------------------------------------------------------------

export interface ReviewDecision {
  sessionId: string
  reviewerId: string
  reviewerName: string
  decision: 'confirmed' | 'dismissed' | 'escalated'
  notes: string
  evidenceIds: string[]
  integrityScore: number
  reviewedAt: string
}

export interface ReviewStatsResponse {
  confirmed: number
  dismissed: number
  escalated: number
  orgId: string
}

export interface SubmitReviewRequest {
  decision: 'confirmed' | 'dismissed' | 'escalated'
  notes: string
  evidenceIds?: string[]
  integrityScore?: number
}

export interface LoginRequest {
  phone: string
  password: string
}

export interface LoginResponse {
  token: string
  user: User
}

// ---------------------------------------------------------------------------
// Consent Types
// ---------------------------------------------------------------------------

export interface ConsentRecord {
  id: string
  sessionId: string
  studentId: string
  orgId: string
  examId: string
  consentVersion: string
  accepted: boolean
  createdAt: string
}

export interface RecordConsentRequest {
  sessionId: string
  studentId: string
  orgId: string
  examId: string
  consentVersion?: string
  consentText?: string
  accepted: boolean
}

// ---------------------------------------------------------------------------
// Integrity Verification Types (Forensic Ledger)
// ---------------------------------------------------------------------------

export interface IntegrityFragmentResult {
  fragmentId: string
  sequenceNum: number
  sha256Match: boolean
  chainValid: boolean
  previousHash: string
  recordHash: string
  errorMessage?: string
}

export interface IntegritySessionReport {
  sessionId: string
  totalFragments: number
  verifiedOk: number
  chainValid: boolean
  s3Verified: number
  s3Mismatches: number
  s3Errors: number
  brokenLinks: IntegrityFragmentResult[]
  mismatches: IntegrityFragmentResult[]
  fragments: IntegrityFragmentResult[]
  verifiedAt: string
  durationMs: number
}

// ── Forensic Report Types ─────────────────────────────────────────────

export interface ForensicPenaltyEntry {
  eventType: string
  count: number
  penaltyPer: number
  maxPenalty: number
  applied: number
  description: string
}

export interface ForensicIntegrityScore {
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  score: number
  verdict: 'clean' | 'warning' | 'fraud'
  verdictLabel: string
  justification: string
  penalties: ForensicPenaltyEntry[]
  eventSummary: Record<string, number>
  topFactors: string[]
  durationSec: number
  totalEvents: number
  criticalCount: number
  warningCount: number
  computedAt: string
}

export interface ForensicVoiceBiometric {
  totalSegments: number
  matchedSegments: number
  mismatchedSegments: number
  consistencyScore: number
  speakerChangeCount: number
  primarySpeakerRatio: number
  verdict: 'consistent' | 'suspicious' | 'anomalous'
}

export interface ForensicTimelineEntry {
  timestamp: string
  videoSec: number
  eventType: string
  severity: string
  label: string
  confidence: number
  source: string
}

export interface ForensicDeviceInfo {
  userAgent: string
  resolution: string
  ipAddress: string
  region: string
  timezone: number
}

export interface ForensicLedgerSummary {
  totalFragments: number
  verifiedOk: number
  chainValid: boolean
  s3Verified: number
  s3Mismatches: number
}

export interface ForensicGazePoint {
  x: number
  y: number
  timestamp: number
}

export interface ForensicReport {
  reportId: string
  generatedAt: string
  reportHash: string
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  integrity: ForensicIntegrityScore
  voiceBiometric: ForensicVoiceBiometric
  timeline: ForensicTimelineEntry[]
  deviceInfo: ForensicDeviceInfo
  gazeData: ForensicGazePoint[]
  ledgerSummary: ForensicLedgerSummary
}

export interface ForensicVerifyResult {
  sessionId: string
  submitted: string
  expected: string
  match: boolean
  verified: boolean
  verifiedAt: string
}

export interface AppealDetail {
  id: string
  sessionId: string
  studentId: string
  orgId: string
  examId: string
  reason: string
  status: 'submitted' | 'under_review' | 'upheld' | 'overturned' | 'withdrawn'
  reviewedBy?: string
  reviewNotes?: string
  reviewedAt?: string
  createdAt: string
  updatedAt: string
}

// ---------------------------------------------------------------------------
// Secondary Camera (Mobile) Orchestration Types
// ---------------------------------------------------------------------------

export type SidecamPolicy = 'mandatory' | 'optional' | 'disabled'

export type SidecamPairingState = 'pending' | 'connected' | 'calibrating' | 'ready' | 'disconnected' | 'failed'

export interface SidecamCalibrationStatus {
  passed: boolean
  viewAngle: number
  handsVisible: boolean
  keyboardVisible: boolean
  screenEdge: boolean
  message: string
  attemptCount: number
  calibratedAt?: string
}

export interface SidecamMobileDevice {
  deviceModel: string
  osVersion: string
  batteryLevel: number
  isCharging: boolean
  thermalState: 'nominal' | 'fair' | 'serious' | 'critical'
  fpsCurrent: number
  fpsTarget: number
  accelX: number
  accelY: number
  accelZ: number
  gyroMagnitude: number
}

export interface SidecamStreamHealth {
  connected: boolean
  lastHeartbeat: string
  latencyMs: number
  frameRate: number
  droppedFrames: number
  uptimeSec: number
  quality: 'excellent' | 'good' | 'degraded' | 'critical'
}

export interface SidecamPairingSession {
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  policy: SidecamPolicy
  state: SidecamPairingState
  pairingToken: string
  pairingCode: string
  deviceToken?: string  // Fix 12: Post-pairing device auth token
  createdAt: string
  expiresAt: string
  connectedAt?: string
  deviceInfo?: SidecamMobileDevice
  calibration: SidecamCalibrationStatus
  health: SidecamStreamHealth
  handsOnDesk: boolean
  displacementAlert: boolean
}

export interface SidecamQRPayload {
  sid: string
  tok: string
  url: string
  exp: number
}

export interface SidecamDeviceDirective {
  targetFps: number
  pauseSession: boolean
  alerts: Array<{
    type: string
    severity: 'warning' | 'critical'
    message: string
  }>
}

export interface SidecamCalibrationFrame {
  estimatedAngle: number
  handsDetected: boolean
  keyboardDetected: boolean
  screenEdgeDetected: boolean
  frameQuality: number
}

// ---------------------------------------------------------------------------
// API Client
// ---------------------------------------------------------------------------

class AdminAPIError extends Error {
  constructor(
    message: string,
    public status: number,
    public body?: string
  ) {
    super(message)
    this.name = 'AdminAPIError'
  }
}

// ---------------------------------------------------------------------------
// Request Deduplication + TTL Cache (Performance Optimization)
// ---------------------------------------------------------------------------
//
// Two-layer performance optimization:
//
//   1. In-flight Deduplication: If multiple components call the same GET endpoint
//      simultaneously, only ONE fetch fires. The others await the same promise.
//      This prevents N concurrent identical requests during page load.
//
//   2. TTL Cache: GET responses are cached for a configurable TTL (default 10s).
//      Subsequent identical requests within the TTL window return cached data
//      without any network call. Cache is keyed on method + URL.
//
// POST/PUT/DELETE requests bypass both layers entirely.
// ---------------------------------------------------------------------------

interface CacheEntry {
  data: unknown
  expiresAt: number
}

/** Shared cache and in-flight map — singleton across all useAdminAPI() instances. */
const _responseCache = new Map<string, CacheEntry>()
const _inflightRequests = new Map<string, Promise<unknown>>()

/** Default TTL for GET request cache (ms). */
const DEFAULT_CACHE_TTL_MS = 10_000

/** Maximum cache entries before pruning. */
const MAX_CACHE_SIZE = 200

/** Prune expired entries from the cache. */
function _pruneCache(): void {
  if (_responseCache.size <= MAX_CACHE_SIZE) return
  const now = Date.now()
  for (const [key, entry] of _responseCache) {
    if (entry.expiresAt < now) {
      _responseCache.delete(key)
    }
  }
  // If still over limit, remove oldest entries
  if (_responseCache.size > MAX_CACHE_SIZE) {
    const keys = Array.from(_responseCache.keys())
    const toRemove = keys.slice(0, keys.length - MAX_CACHE_SIZE)
    for (const key of toRemove) {
      _responseCache.delete(key)
    }
  }
}

/**
 * Invalidate cache entries matching a path prefix.
 * Call after mutations to ensure stale data isn't served.
 *
 * Usage: invalidateCache('/api/v1/archive') clears all archive-related cache.
 */
export function invalidateAPICache(pathPrefix?: string): void {
  if (!pathPrefix) {
    _responseCache.clear()
    return
  }
  for (const key of _responseCache.keys()) {
    if (key.includes(pathPrefix)) {
      _responseCache.delete(key)
    }
  }
}

export function useAdminAPI() {
  const config = useRuntimeConfig()
  const authStore = useAuthStore()

  const baseURL = computed(() => {
    const url = (config.public.apiBaseUrl as string)
      || (config.public.grpcUrl as string)
      || 'http://localhost:8080'
    return url.replace(/\/$/, '') // Remove trailing slash.
  })

  // ----- Helpers -----

  async function request<T>(
    method: string,
    path: string,
    body?: unknown,
    requireAuth = true,
    extraHeaders?: Record<string, string>,
  ): Promise<T> {
    const url = `${baseURL.value}${path}`
    const cacheKey = `${method}:${url}`

    // ── Layer 1: TTL Cache (GET only) ───────────────────────────────────
    if (method === 'GET') {
      const cached = _responseCache.get(cacheKey)
      if (cached && cached.expiresAt > Date.now()) {
        if (import.meta.dev) {
          console.log(`[AdminAPI] Cache HIT ${method} ${url}`)
        }
        return cached.data as T
      }
    }

    // ── Layer 2: In-Flight Deduplication (GET only) ─────────────────────
    if (method === 'GET') {
      const inflight = _inflightRequests.get(cacheKey)
      if (inflight) {
        if (import.meta.dev) {
          console.log(`[AdminAPI] Dedup JOIN ${method} ${url}`)
        }
        return inflight as Promise<T>
      }
    }

    // ── Layer 3: Actual Fetch ───────────────────────────────────────────
    const fetchPromise = _doFetch<T>(method, url, body, requireAuth, extraHeaders)

    // Register in-flight request (GET only)
    if (method === 'GET') {
      _inflightRequests.set(cacheKey, fetchPromise)
    }

    try {
      const result = await fetchPromise

      // Cache successful GET responses
      if (method === 'GET') {
        _pruneCache()
        _responseCache.set(cacheKey, {
          data: result,
          expiresAt: Date.now() + DEFAULT_CACHE_TTL_MS
        })
      }

      // Invalidate related caches on mutations
      if (method === 'POST' || method === 'PUT' || method === 'DELETE') {
        // Extract the resource path (e.g., /api/v1/archive from /api/v1/archive/sessions/123/review)
        const pathParts = path.split('/')
        if (pathParts.length >= 4) {
          const resourcePrefix = pathParts.slice(0, 4).join('/')
          invalidateAPICache(resourcePrefix)
        }
      }

      return result
    } finally {
      // Always clean up in-flight map
      if (method === 'GET') {
        _inflightRequests.delete(cacheKey)
      }
    }
  }

  async function _doFetch<T>(
    method: string,
    url: string,
    body?: unknown,
    requireAuth = true,
    extraHeaders?: Record<string, string>,
  ): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...extraHeaders, // Fix 12: Support device token headers
    }

    if (requireAuth) {
      if (!authStore.jwtToken) {
        console.warn(`[AdminAPI] No JWT token available for ${method} ${url}. Request will likely fail with 401.`)
      } else {
        headers['Authorization'] = `Bearer ${authStore.jwtToken}`
      }
    }

    const opts: RequestInit = {
      method,
      headers
    }

    if (body && (method === 'POST' || method === 'PUT' || method === 'PATCH')) {
      opts.body = JSON.stringify(body)
    }

    if (import.meta.dev) {
      console.log(`[AdminAPI] ${method} ${url}`, body ? JSON.stringify(body) : '')
    }

    let response: Response
    try {
      response = await fetch(url, opts)
    } catch (networkErr) {
      // Network error (CORS block, server unreachable, DNS failure).
      console.error(`[AdminAPI] Network error for ${method} ${url}:`, networkErr)
      throw new AdminAPIError(
        `Сервер недоступен (${baseURL.value}). Проверьте, что backend запущен.`,
        0,
        String(networkErr)
      )
    }

    if (!response.ok) {
      let errorMsg = `API error: ${response.status}`
      let errorBody = ''
      try {
        const errBody = await response.json()
        errorBody = JSON.stringify(errBody)
        if (errBody.error) errorMsg = errBody.error
      } catch {
        // Ignore JSON parse errors.
      }

      console.error(`[AdminAPI] ${method} ${url} → ${response.status}: ${errorMsg}`, errorBody)

      // Handle 401 — clear auth state, but not for demo sessions.
      if (response.status === 401 && authStore.jwtToken !== 'demo-jwt-token') {
        authStore.logout()
      }

      throw new AdminAPIError(errorMsg, response.status, errorBody)
    }

    return response.json() as Promise<T>
  }

  // ----- Auth Endpoints -----

  async function login(phone: string, password: string): Promise<LoginResponse> {
    const data = await request<LoginResponse>('POST', '/api/v1/auth/login', { phone, password }, false)

    // Store the JWT token and user data.
    authStore.setToken(data.token, undefined, data.user.id, data.user.orgId)
    authStore.setUserData(data.user)

    return data
  }

  async function getMe(): Promise<User> {
    return request<User>('GET', '/api/v1/auth/me')
  }

  // ----- Organization Endpoints -----

  async function listOrgs(params?: {
    search?: string
    type?: string
    plan?: string
  }): Promise<Organization[]> {
    const query = new URLSearchParams()
    if (params?.search) query.set('search', params.search)
    if (params?.type) query.set('type', params.type)
    if (params?.plan) query.set('plan', params.plan)

    const qs = query.toString()
    const path = `/api/v1/admin/organizations${qs ? `?${qs}` : ''}`
    return request<Organization[]>('GET', path)
  }

  async function getOrg(orgId: string): Promise<Organization> {
    return request<Organization>('GET', `/api/v1/admin/organizations/${orgId}`)
  }

  async function createOrg(data: CreateOrgRequest): Promise<Organization> {
    return request<Organization>('POST', '/api/v1/admin/organizations', data)
  }

  async function createOrgWithAdmin(data: CreateOrgWithAdminRequest): Promise<CreateOrgWithAdminResponse> {
    return request<CreateOrgWithAdminResponse>('POST', '/api/v1/admin/organizations-with-admin', data)
  }

  async function updateOrg(orgId: string, data: Partial<CreateOrgRequest>): Promise<Organization> {
    return request<Organization>('PUT', `/api/v1/admin/organizations/${orgId}`, data)
  }

  async function deleteOrg(orgId: string): Promise<{ status: string }> {
    return request<{ status: string }>('DELETE', `/api/v1/admin/organizations/${orgId}`)
  }

  // ----- User Endpoints -----

  async function listUsers(orgId: string): Promise<User[]> {
    return request<User[]>('GET', `/api/v1/admin/organizations/${orgId}/users`)
  }

  async function createUser(orgId: string, data: CreateUserRequest): Promise<User> {
    return request<User>('POST', `/api/v1/admin/organizations/${orgId}/users`, data)
  }

  // ----- API Key Endpoints -----

  async function listAPIKeys(orgId: string): Promise<APIKey[]> {
    return request<APIKey[]>('GET', `/api/v1/admin/organizations/${orgId}/keys`)
  }

  async function createAPIKey(orgId: string, data: CreateAPIKeyRequest): Promise<CreateAPIKeyResponse> {
    return request<CreateAPIKeyResponse>('POST', `/api/v1/admin/organizations/${orgId}/keys`, data)
  }

  async function revokeAPIKey(keyId: string): Promise<{ status: string }> {
    return request<{ status: string }>('DELETE', `/api/v1/admin/keys/${keyId}`)
  }

  // ----- Stats -----

  async function getStats(): Promise<AdminStats> {
    return request<AdminStats>('GET', '/api/v1/admin/stats')
  }

  // ----- Analytics (Executive Dashboard) -----

  async function getAnalyticsOverview(orgId?: string): Promise<AnalyticsOverview> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    const qs = query.toString()
    return request<AnalyticsOverview>('GET', `/api/v1/analytics/overview${qs ? `?${qs}` : ''}`)
  }

  async function getRiskDistribution(orgId?: string): Promise<RiskDistribution> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    const qs = query.toString()
    return request<RiskDistribution>('GET', `/api/v1/analytics/risk-distribution${qs ? `?${qs}` : ''}`)
  }

  async function getSystemHealth(orgId?: string): Promise<SystemHealthData> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    const qs = query.toString()
    return request<SystemHealthData>('GET', `/api/v1/analytics/system-health${qs ? `?${qs}` : ''}`)
  }

  async function getTopViolations(orgId?: string, limit?: number): Promise<ViolationEntry[]> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    if (limit) query.set('limit', String(limit))
    const qs = query.toString()
    return request<ViolationEntry[]>('GET', `/api/v1/analytics/top-violations${qs ? `?${qs}` : ''}`)
  }

  async function getStudentRisk(orgId?: string, limit?: number): Promise<StudentRiskEntry[]> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    if (limit) query.set('limit', String(limit))
    const qs = query.toString()
    return request<StudentRiskEntry[]>('GET', `/api/v1/analytics/student-risk${qs ? `?${qs}` : ''}`)
  }

  async function getHourlyStats(orgId?: string): Promise<HourlyStatEntry[]> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    const qs = query.toString()
    return request<HourlyStatEntry[]>('GET', `/api/v1/analytics/hourly-stats${qs ? `?${qs}` : ''}`)
  }

  async function getInfrastructureStats(): Promise<InfrastructureStats> {
    return request<InfrastructureStats>('GET', '/api/v1/analytics/infrastructure-stats')
  }

  // ----- Audit Logs -----

  async function listAuditLogs(params?: AuditLogParams): Promise<AuditEntry[]> {
    const query = new URLSearchParams()
    if (params?.orgId) query.set('org_id', params.orgId)
    if (params?.userId) query.set('user_id', params.userId)
    if (params?.action) query.set('action', params.action)
    if (params?.resourceType) query.set('resource_type', params.resourceType)
    if (params?.limit) query.set('limit', String(params.limit))
    if (params?.offset) query.set('offset', String(params.offset))

    const qs = query.toString()
    const path = `/api/v1/admin/audit-logs${qs ? `?${qs}` : ''}`
    return request<AuditEntry[]>('GET', path)
  }

  // ----- Monitoring (Live Dashboard) -----

  async function getActiveSessions(orgId?: string, examId?: string): Promise<ActiveSessionsResponse> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    if (examId) query.set('exam_id', examId)
    const qs = query.toString()
    return request<ActiveSessionsResponse>('GET', `/api/v1/monitoring/active-sessions${qs ? `?${qs}` : ''}`)
  }

  async function warnSession(sessionId: string, message?: string): Promise<{ status: string; sessionId: string; message: string }> {
    return request<{ status: string; sessionId: string; message: string }>(
      'POST',
      `/api/v1/monitoring/sessions/${sessionId}/warn`,
      { message: message || 'Проктор отправил предупреждение студенту' }
    )
  }

  async function terminateSession(sessionId: string, reason?: string): Promise<{ status: string; sessionId: string; reason: string }> {
    return request<{ status: string; sessionId: string; reason: string }>(
      'POST',
      `/api/v1/monitoring/sessions/${sessionId}/terminate`,
      { reason: reason || 'Сессия завершена проктором' }
    )
  }

  // ── Archive (Архив сессий) ───────────────────────────────────────────────

  async function getArchiveSessions(params?: {
    orgId?: string
    examId?: string
    status?: 'reviewed' | 'pending' | 'voided'
    limit?: number
    offset?: number
  }): Promise<ArchiveSessionsResponse> {
    const query = new URLSearchParams()
    if (params?.orgId) query.set('org_id', params.orgId)
    if (params?.examId) query.set('exam_id', params.examId)
    if (params?.status) query.set('status', params.status)
    if (params?.limit) query.set('limit', String(params.limit))
    if (params?.offset) query.set('offset', String(params.offset))
    const qs = query.toString()
    return request<ArchiveSessionsResponse>('GET', `/api/v1/archive/sessions${qs ? `?${qs}` : ''}`)
  }

  async function getArchiveSessionEvents(sessionId: string): Promise<ArchiveSessionEventsResponse> {
    return request<ArchiveSessionEventsResponse>('GET', `/api/v1/archive/sessions/${sessionId}/events`)
  }

  async function getArchiveExams(orgId?: string): Promise<ArchiveExamsResponse> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    const qs = query.toString()
    return request<ArchiveExamsResponse>('GET', `/api/v1/archive/exams${qs ? `?${qs}` : ''}`)
  }

  async function exportArchiveSession(sessionId: string): Promise<ExportSessionResponse> {
    return request<ExportSessionResponse>('GET', `/api/v1/archive/sessions/${sessionId}/export`)
  }

  // ── Evidence (Video Evidence Fragments) ─────────────────────────────────

  async function getSessionEvidence(sessionId: string): Promise<EvidenceListResponse> {
    return request<EvidenceListResponse>('GET', `/api/v1/evidence/sessions/${sessionId}`)
  }

  async function getEvidencePresignedURL(fragmentId: string): Promise<PresignedURLResponse> {
    return request<PresignedURLResponse>('GET', `/api/v1/evidence/fragments/${fragmentId}/url`)
  }

  async function verifyEvidence(fragmentId: string): Promise<EvidenceVerifyResponse> {
    return request<EvidenceVerifyResponse>('GET', `/api/v1/evidence/fragments/${fragmentId}/verify`)
  }

  // ── Review (Human Review Workflow) ────────────────────────────────────

  async function submitReview(sessionId: string, data: SubmitReviewRequest): Promise<ReviewDecision> {
    return request<ReviewDecision>('POST', `/api/v1/archive/sessions/${sessionId}/review`, data)
  }

  async function getReview(sessionId: string): Promise<ReviewDecision | { sessionId: string; status: string; message: string }> {
    return request<ReviewDecision | { sessionId: string; status: string; message: string }>(
      'GET', `/api/v1/archive/sessions/${sessionId}/review`
    )
  }

  async function getReviewStats(orgId?: string): Promise<ReviewStatsResponse> {
    const query = new URLSearchParams()
    if (orgId) query.set('org_id', orgId)
    const qs = query.toString()
    return request<ReviewStatsResponse>('GET', `/api/v1/archive/review-stats${qs ? `?${qs}` : ''}`)
  }

  // ── Bulk Export (Evidence Archives) ───────────────────────────────────

  async function listExportJobs(): Promise<Array<{
    id: string
    orgId: string
    requestedBy: string
    sessionIds: string[]
    status: 'pending' | 'processing' | 'completed' | 'failed' | 'expired'
    archiveUri?: string
    manifestUri?: string
    sha256Archive?: string
    presignTtlSec: number
    downloadUrl?: string
    errorMessage?: string
    createdAt: string
    completedAt?: string
    expiresAt?: string
  }>> {
    return request('GET', '/api/v1/export')
  }

  async function createExportJob(sessionIds: string[]): Promise<{ id: string; status: string }> {
    return request('POST', '/api/v1/export/bulk', { sessionIds })
  }

  async function getExportJob(exportId: string): Promise<{
    id: string
    status: string
    downloadUrl?: string
    sha256Archive?: string
    errorMessage?: string
  }> {
    return request('GET', `/api/v1/export/${exportId}`)
  }

  async function cancelExportJob(exportId: string): Promise<{ status: string }> {
    return request('DELETE', `/api/v1/export/${exportId}`)
  }

  // ── Appeals (Student Appeal Workflow) ──────────────────────────────────

  async function listAppeals(params?: {
    orgId?: string
    sessionId?: string
    status?: string
  }): Promise<Array<{
    id: string
    orgId: string
    sessionId: string
    examId: string
    studentId: string
    status: 'submitted' | 'under_review' | 'upheld' | 'overturned' | 'withdrawn'
    reason: string
    reviewerNotes?: string
    reviewedBy?: string
    createdAt: string
    updatedAt: string
  }>> {
    const query = new URLSearchParams()
    if (params?.orgId) query.set('org_id', params.orgId)
    if (params?.sessionId) query.set('session_id', params.sessionId)
    if (params?.status) query.set('status', params.status)
    const qs = query.toString()
    return request('GET', `/api/v1/appeals${qs ? `?${qs}` : ''}`)
  }

  async function submitAppeal(data: {
    sessionId: string
    examId: string
    studentId: string
    reason: string
  }): Promise<{ id: string; status: string }> {
    return request('POST', '/api/v1/appeals', data)
  }

  async function reviewAppeal(appealId: string, data: {
    status: 'under_review' | 'upheld' | 'overturned' | 'withdrawn'
    reviewerNotes?: string
  }): Promise<{ id: string; status: string }> {
    return request('PUT', `/api/v1/appeals/${appealId}/review`, data)
  }

  // ── Consent (Student Consent Management) ─────────────────────────────

  async function recordConsent(data: RecordConsentRequest): Promise<ConsentRecord> {
    return request<ConsentRecord>('POST', '/api/v1/consent', data)
  }

  async function getConsent(sessionId: string): Promise<ConsentRecord> {
    return request<ConsentRecord>('GET', `/api/v1/consent/${sessionId}`)
  }

  // ── Integrity Verification (Forensic Ledger) ────────────────────────

  async function verifySessionIntegrity(sessionId: string): Promise<IntegritySessionReport> {
    return request<IntegritySessionReport>('POST', '/api/v1/integrity/verify-session', { sessionId })
  }

  async function verifyFragmentIntegrity(fragmentId: string): Promise<IntegrityFragmentResult> {
    return request<IntegrityFragmentResult>('POST', '/api/v1/integrity/verify-fragment', { fragmentId })
  }

  async function getIntegrityReport(sessionId: string): Promise<IntegritySessionReport> {
    return request<IntegritySessionReport>('GET', `/api/v1/integrity/session/${sessionId}/report`)
  }

  // ── Appeal Detail ───────────────────────────────────────────────────

  async function getAppeal(appealId: string): Promise<AppealDetail> {
    return request<AppealDetail>('GET', `/api/v1/appeals/${appealId}`)
  }

  // ── Forensic Reporting ──────────────────────────────────────────────

  async function getForensicReport(sessionId: string): Promise<ForensicReport> {
    return request<ForensicReport>('GET', `/api/v1/forensic/session/${sessionId}/report`)
  }

  async function getForensicScore(sessionId: string): Promise<ForensicIntegrityScore> {
    return request<ForensicIntegrityScore>('GET', `/api/v1/forensic/session/${sessionId}/score`)
  }

  function getForensicPDFUrl(sessionId: string): string {
    const config = useRuntimeConfig()
    const auth = useAuthStore()
    const base = config.public.grpcUrl || ''
    return `${base}/api/v1/forensic/session/${sessionId}/pdf?token=${auth.jwtToken}`
  }

  function getForensicHeatmapUrl(sessionId: string): string {
    const config = useRuntimeConfig()
    const auth = useAuthStore()
    const base = config.public.grpcUrl || ''
    return `${base}/api/v1/forensic/session/${sessionId}/heatmap?token=${auth.jwtToken}`
  }

  async function getForensicVoice(sessionId: string): Promise<ForensicVoiceBiometric> {
    return request<ForensicVoiceBiometric>('GET', `/api/v1/forensic/session/${sessionId}/voice`)
  }

  async function verifyForensicReport(sessionId: string, reportHash: string): Promise<ForensicVerifyResult> {
    return request<ForensicVerifyResult>('POST', '/api/v1/forensic/verify', { sessionId, reportHash })
  }

  // ── Secondary Camera (Mobile Orchestration) ───────────────────────────

  async function initiateSidecamPairing(data: {
    sessionId: string
    studentId: string
    examId: string
    orgId: string
    policy: SidecamPolicy
  }): Promise<{
    session: SidecamPairingSession
    qrData: string
    payload: SidecamQRPayload
  }> {
    return request('POST', '/api/v1/sidecam/pair/initiate', data)
  }

  async function completeSidecamPairing(data: {
    sessionId: string
    pairingToken: string
    device: SidecamMobileDevice
  }): Promise<{ status: string; session: SidecamPairingSession }> {
    return request('POST', '/api/v1/sidecam/pair/complete', data)
  }

  async function validateSidecamStart(sessionId: string, policy: SidecamPolicy): Promise<{
    allowed: boolean
    reason?: string
  }> {
    return request('POST', '/api/v1/sidecam/validate-start', { sessionId, policy })
  }

  // Fix 12: Device-authenticated endpoints pass X-Device-Token + X-Session-ID headers
  async function submitSidecamCalibration(sessionId: string, frame: SidecamCalibrationFrame, deviceToken?: string): Promise<SidecamCalibrationStatus> {
    const hdrs = deviceToken ? { 'X-Device-Token': deviceToken, 'X-Session-ID': sessionId } : undefined
    return request('POST', '/api/v1/sidecam/calibrate', { sessionId, frame }, false, hdrs)
  }

  async function sendSidecamTelemetry(sessionId: string, device: SidecamMobileDevice, deviceToken?: string): Promise<SidecamDeviceDirective> {
    const hdrs = deviceToken ? { 'X-Device-Token': deviceToken, 'X-Session-ID': sessionId } : undefined
    return request('POST', '/api/v1/sidecam/telemetry', { sessionId, device }, false, hdrs)
  }

  async function sendSidecamHeartbeat(sessionId: string, frameRate: number, droppedFrames: number, deviceToken?: string): Promise<SidecamStreamHealth> {
    const hdrs = deviceToken ? { 'X-Device-Token': deviceToken, 'X-Session-ID': sessionId } : undefined
    return request('POST', '/api/v1/sidecam/heartbeat', {
      sessionId,
      clientTs: new Date().toISOString(),
      frameRate,
      droppedFrames,
    }, false, hdrs)
  }

  async function sendSidecamHandsDetection(sessionId: string, handsVisible: boolean, deviceToken?: string): Promise<{ status: string }> {
    const hdrs = deviceToken ? { 'X-Device-Token': deviceToken, 'X-Session-ID': sessionId } : undefined
    return request('POST', '/api/v1/sidecam/hands', { sessionId, handsVisible }, false, hdrs)
  }

  async function getSidecamSession(sessionId: string): Promise<SidecamPairingSession> {
    return request<SidecamPairingSession>('GET', `/api/v1/sidecam/session/${sessionId}`)
  }

  async function cleanupSidecamSession(sessionId: string): Promise<{ status: string }> {
    return request<{ status: string }>('DELETE', `/api/v1/sidecam/session/${sessionId}`)
  }

  // ── Media (WebRTC / LiveKit) ────────────────────────────────────────────

  async function getMediaToken(sessionId: string, role: 'student' | 'proctor' = 'proctor'): Promise<{ token: string; wsUrl: string; room: string }> {
    return request<{ token: string; wsUrl: string; room: string }>(
      'POST',
      '/api/v1/media/token',
      { sessionId, role }
    )
  }

  async function getMediaRooms(): Promise<{ livekitUrl: string; apiKey: string; status: string }> {
    return request<{ livekitUrl: string; apiKey: string; status: string }>(
      'GET',
      '/api/v1/media/rooms'
    )
  }

  return {
    // Auth
    login,
    getMe,

    // Organizations
    listOrgs,
    getOrg,
    createOrg,
    createOrgWithAdmin,
    updateOrg,
    deleteOrg,

    // Users
    listUsers,
    createUser,

    // API Keys
    listAPIKeys,
    createAPIKey,
    revokeAPIKey,

    // Stats
    getStats,

    // Analytics (Executive Dashboard)
    getAnalyticsOverview,
    getRiskDistribution,
    getSystemHealth,
    getTopViolations,
    getStudentRisk,
    getHourlyStats,
    getInfrastructureStats,

    // Monitoring (Live Dashboard)
    getActiveSessions,
    warnSession,
    terminateSession,

    // Audit Logs
    listAuditLogs,

    // Archive (Архив сессий)
    getArchiveSessions,
    getArchiveSessionEvents,
    getArchiveExams,
    exportArchiveSession,

    // Evidence (Video Evidence Fragments)
    getSessionEvidence,
    getEvidencePresignedURL,
    verifyEvidence,

    // Review (Human Review Workflow)
    submitReview,
    getReview,
    getReviewStats,

    // Bulk Export (Evidence Archives)
    listExportJobs,
    createExportJob,
    getExportJob,
    cancelExportJob,

    // Appeals (Student Appeal Workflow)
    listAppeals,
    submitAppeal,
    reviewAppeal,
    getAppeal,

    // Consent (Student Consent Management)
    recordConsent,
    getConsent,

    // Integrity Verification (Forensic Ledger)
    verifySessionIntegrity,
    verifyFragmentIntegrity,
    getIntegrityReport,

    // Forensic Reporting (Integrity Audit)
    getForensicReport,
    getForensicScore,
    getForensicPDFUrl,
    getForensicHeatmapUrl,
    getForensicVoice,
    verifyForensicReport,

    // Secondary Camera (Mobile Orchestration)
    initiateSidecamPairing,
    completeSidecamPairing,
    validateSidecamStart,
    submitSidecamCalibration,
    sendSidecamTelemetry,
    sendSidecamHeartbeat,
    sendSidecamHandsDetection,
    getSidecamSession,
    cleanupSidecamSession,

    // Media (WebRTC / LiveKit)
    getMediaToken,
    getMediaRooms,

    // Error class for instanceof checks
    AdminAPIError
  }
}
