import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { useAuthStore } from '~/stores/useAuthStore'

export interface ActiveExam {
  id: string
  orgId: string
  examName: string
  startTime: string
  participants: number
  violationRate: number
}

export interface ViolationAlert {
  id: string
  orgId: string
  studentName: string
  violationType: string
  timestamp: string
  severity: 'critical' | 'warning' | 'info'
  examName: string
}

export interface AtRiskStudent {
  id: string
  orgId: string
  name: string
  iin: string
  examName: string
  integrityScore: number
  violations: number
}

export interface ViolationTrendPoint {
  time: string
  phoneDetected: number
  gazeDeviation: number
  multiplePersons: number
  tabSwitch: number
}

// --- Infrastructure Health Interfaces ---
export interface ServerNode {
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
}

export interface LatencyPoint {
  time: string
  avg: number
  p95: number
  p99: number
}

export interface NetworkMetric {
  label: string
  value: number
  unit: string
  status: 'healthy' | 'warning' | 'critical'
  trend: 'up' | 'down' | 'stable'
}

// --- Regional Analysis Interfaces ---
export interface RegionData {
  id: string
  name: string
  nameKz: string
  students: number
  activeSessions: number
  violationRate: number
  avgIntegrity: number
  topViolation: string
}

// --- Violation Intel Interfaces ---
export interface ViolationCategory {
  type: string
  icon: string
  count: number
  percentChange: number
  trend: 'up' | 'down' | 'stable'
  severity: 'critical' | 'warning' | 'info'
}

export interface HourlyViolation {
  hour: string
  phone: number
  gaze: number
  persons: number
  tabs: number
  audio: number
}

export interface DetectionAccuracy {
  type: string
  truePositive: number
  falsePositive: number
  accuracy: number
}

export interface ProctorKPI {
  id: string
  name: string
  sessionsReviewed: number
  avgReactionTimeSec: number
  warningsIssued: number
  warningAccuracy: number
  terminationsInitiated: number
  violationsDetected: number
  shift: string
}

export interface RegionalViolationBreakdown {
  regionId: string
  regionName: string
  totalViolations: number
  phone: number
  gaze: number
  persons: number
  tabs: number
  audio: number
  criticalRate: number
  avgReactionSec: number
}

// --- Global Analytics Interfaces ---
export interface WeeklyTrendPoint {
  day: string
  phone: number
  gaze: number
  voice: number
  tabs: number
  persons: number
}

export interface MonthlyTrendPoint {
  month: string
  totalViolations: number
  avgIntegrity: number
  totalSessions: number
}

export interface SubjectViolationRate {
  subject: string
  examCount: number
  avgViolationRate: number
  totalViolations: number
  topViolationType: string
}

export interface InstitutionRanking {
  id: string
  name: string
  city: string
  totalStudents: number
  avgIntegrity: number
  violationRate: number
  totalExams: number
  trend: 'up' | 'down' | 'stable'
}

// --- API & Integrations Interfaces ---
export interface ApiKey {
  id: string
  name: string
  key: string
  created: string
  lastUsed: string
  status: 'active' | 'revoked'
  permissions: string[]
}

export interface Webhook {
  id: string
  url: string
  events: string[]
  status: 'active' | 'paused'
  lastDelivery: string
  successRate: number
}

export interface WebhookDeliveryLog {
  id: string
  webhookId: string
  event: string
  status: 'success' | 'failed'
  timestamp: string
  responseCode: number
  duration: number
}

// --- IP Whitelist ---
export interface IPWhitelistEntry {
  id: string
  orgId: string
  ip: string
  label: string
  createdAt: string
  createdBy: string
}

// --- Exam Proctoring Settings ---
export interface StudentException {
  id: string
  studentName: string
  studentId: string
  phone?: string
  reason: string
  profile: 'low_gaze' | 'extra_time' | 'no_side_camera' | 'high_noise_tolerance' | 'custom'
  profileLabel: string
  overrides: Partial<ExamProctoringSettings>
}

export interface ExamProctoringSettings {
  // --- Видео-правила ---
  requireSideCamera: boolean
  faceVerification: boolean
  dynamicFaceRecheck: boolean
  antiSpoofing: boolean
  roomScan360: boolean
  objectDetectionPhone: boolean
  objectDetectionPerson: boolean
  // --- Чувствительность ИИ ---
  gazeTracking: boolean
  gazeSensitivity: number
  gazeDeviationLimitSec: number
  voiceDetectionThreshold: number
  voiceActivityDetection: boolean
  audioPeripheryDetection: boolean
  smartNoiseFilter: boolean
  // --- Психометрия и AI-аналитика ---
  emotionStressAnalysis: boolean
  focusLossScore: boolean
  blinkPatternAnalysis: boolean
  // --- Ограничения браузера ---
  forceFullscreen: boolean
  fullscreenExitDetection: boolean
  webDisplayMonitoring: boolean
  tabSwitchingLimit: number
  blockCopyPaste: boolean
  blockPrintScreen: boolean
  blockVirtualMachine: boolean
  blockMultiDesktop: boolean
  blockRemoteAccess: boolean
  blockContextMenu: boolean
  // --- Сетевой контроль ---
  vpnProxyDetection: boolean
  localNetworkScan: boolean
  // --- Системный контроль (Kernel-Level) ---
  typingDynamics: boolean
  handCursorSync: boolean
  processScanning: boolean
  hardwareDeviceDetection: boolean
  advancedRemoteBlock: boolean
  hardwareIdBinding: boolean
  deepMultiMonitorCheck: boolean
  // --- Resilience / Performance ---
  /** Force low-spec UI mode for all students in this exam (disables animations, effects, WebGL). */
  forceLowSpecMode: boolean
}

export interface ExamProctoringConfig {
  id: string
  orgId: string
  examName: string
  examCode: string
  eduserId: string
  date: string
  participants: number
  isArgusProtected: boolean
  syncedAt: string
  status: 'active' | 'completed' | 'scheduled'
  violationCount: number
  settings: ExamProctoringSettings
  exceptions: StudentException[]
}

// --- Live Monitoring Interfaces ---
export interface AiDetectionStatus {
  gazeTracking: 'normal' | 'warning' | 'critical'
  faceIdMatch: 'verified' | 'mismatch' | 'unavailable'
  objectDetection: 'clear' | 'phone' | 'book' | 'earbuds'
}

export interface MonitoringEvent {
  id: string
  timestamp: string
  type: 'gaze_deviation' | 'face_mismatch' | 'phone_detected' | 'book_detected' | 'tab_switch' | 'audio_anomaly' | 'earbuds_detected' | 'multiple_persons' | 'background_voices' | 'whispering' | 'whisper_detected' | 'second_speaker_detected'
  label: string
  severity: 'critical' | 'warning' | 'info'
}

export interface MonitoringSession {
  id: string
  orgId: string
  studentName: string
  iin: string
  phone: string
  examId: string
  examName: string
  integrityScore: number
  violationCount: number
  violationLevel: 'critical' | 'warning' | 'clean'
  aiStatus: AiDetectionStatus
  isOnline: boolean
  startedAt: string
  lastActivity: string
  events: MonitoringEvent[]
}

// --- Student Roster (for global search) ---
export interface StudentRecord {
  id: string
  orgId: string
  name: string
  iin: string
  phone: string
  examId: string
  examName: string
  integrityScore: number
  violations: number
  status: 'active' | 'flagged' | 'clean'
}

// --- Session Archive Interfaces ---
export interface ArchiveEvent {
  id: string
  timestamp: string
  type: string
  label: string
  severity: 'critical' | 'warning' | 'info'
  videoTimestamp: number // seconds into recording
  source: 'webcam' | 'side' | 'system'
}

export interface ArchiveSession {
  id: string
  orgId: string
  studentName: string
  iin: string
  phone: string
  examId: string
  examName: string
  date: string
  duration: string
  integrityScore: number
  violationCount: number
  status: 'reviewed' | 'pending' | 'voided'
  events: ArchiveEvent[]
}

export interface ArchiveExamSummary {
  id: string
  orgId: string
  examName: string
  date: string
  participants: number
  avgIntegrity: number
  totalViolations: number
  reviewed: number
  pending: number
  voided: number
}

// --- Test Module Interfaces ---
export interface TestItem {
  id: string
  name: string
  subject: string
  questionsCount: number
  duration: number // minutes
  createdAt: string
  updatedAt: string
  status: 'active' | 'draft' | 'archived'
  author: string
  difficulty: 'easy' | 'medium' | 'hard'
  passScore: number // percentage
  attemptsAllowed: number
}

export interface ComplexTestItem {
  id: string
  name: string
  subjects: string[]
  testsCount: number
  totalQuestions: number
  totalDuration: number
  createdAt: string
  status: 'active' | 'draft' | 'archived'
  author: string
}

export const useDashboardStore = defineStore('dashboard', () => {
  // --- UI State ---
  const leftSidebarOpen = ref(true)
  const rightPanelOpen = ref(true)
  const dashboardMenuOpen = ref(true)

  function toggleLeftSidebar() {
    leftSidebarOpen.value = !leftSidebarOpen.value
  }

  function toggleRightPanel() {
    rightPanelOpen.value = !rightPanelOpen.value
  }

  function toggleDashboardMenu() {
    dashboardMenuOpen.value = !dashboardMenuOpen.value
  }

  // ============================================
  //  FILTER STATE
  // ============================================
  const selectedExamId = ref<string | null>(null)
  const studentSearchQuery = ref('')

  function selectExam(examId: string | null) {
    selectedExamId.value = examId
  }

  function clearFilter() {
    selectedExamId.value = null
  }

  // Selected exam object
  const selectedExam = computed(() => {
    if (!selectedExamId.value) return null
    return activeExams.value.find(e => e.id === selectedExamId.value) ?? null
  })

  const isFiltered = computed(() => selectedExamId.value !== null)

  // --- KPI ---
  const activeSessions = ref(247)
  const criticalViolations = ref(18)
  const avgIntegrityScore = ref(87)
  const systemHealth = ref<'operational' | 'degraded' | 'down'>('operational')
  const systemHealthUptime = ref(99.97)

  // Overview real-data loading (KPI cards). Demo values above are preserved on
  // failure so the dashboard never renders empty.
  const overviewLoading = ref(false)
  const overviewError = ref<string | null>(null)
  const overviewUsingRealData = ref(false)

  const systemHealthLabel = computed(() => {
    const map = { operational: 'В норме', degraded: 'Снижение', down: 'Недоступно' }
    return map[systemHealth.value]
  })

  // --- Active Exams ---
  const activeExams = ref<ActiveExam[]>([])

  // --- Violation Alerts (Real-time Feed) ---
  const violationAlerts = ref<ViolationAlert[]>([])

  // --- At-Risk Students ---
  const atRiskStudents = ref<AtRiskStudent[]>([])

  // --- Student Roster (mock 10k — represented as sample) ---
  const studentRoster = ref<StudentRecord[]>([])

  // --- Violation Trend Data (last 12 hours) ---
  const violationTrends = ref<ViolationTrendPoint[]>([])

  // ============================================
  //  FILTERED COMPUTEDS
  // ============================================

  // Filtered KPIs
  const filteredActiveSessions = computed(() => {
    if (!selectedExam.value) return activeSessions.value
    return selectedExam.value.participants
  })

  const filteredTotalParticipants = computed(() => {
    if (!selectedExam.value) return totalParticipants.value
    return selectedExam.value.participants
  })

  const filteredCriticalViolations = computed(() => {
    if (!selectedExam.value) return criticalViolations.value
    return filteredAlerts.value.filter(a => a.severity === 'critical').length
  })

  const filteredAvgIntegrity = computed(() => {
    if (!selectedExam.value) return avgIntegrityScore.value
    const students = filteredAtRiskStudents.value
    if (students.length === 0) return avgIntegrityScore.value
    return Math.round(students.reduce((s, st) => s + st.integrityScore, 0) / students.length)
  })

  // Filtered violation alerts
  const filteredAlerts = computed(() => {
    if (!selectedExam.value) return violationAlerts.value
    return violationAlerts.value.filter(a => a.examName === selectedExam.value?.examName)
  })

  // Filtered at-risk students
  const filteredAtRiskStudents = computed(() => {
    if (!selectedExam.value) return atRiskStudents.value
    return atRiskStudents.value.filter(s => s.examName === selectedExam.value?.examName)
  })

  // Filtered critical alerts
  const filteredCriticalAlerts = computed(() =>
    filteredAlerts.value.filter(a => a.severity === 'critical')
  )

  // Search results
  const searchResults = computed(() => {
    const q = studentSearchQuery.value.trim().toLowerCase()
    if (!q) return []
    let roster = studentRoster.value
    if (selectedExam.value) {
      roster = roster.filter(s => s.examId === selectedExamId.value)
    }
    return roster.filter(s =>
      s.name.toLowerCase().includes(q) || s.iin.includes(q) || s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, ''))
    ).slice(0, 8)
  })

  // ============================================
  //  INFRASTRUCTURE HEALTH DATA
  // ============================================
  const serverNodes = ref<ServerNode[]>([])

  const latencyHistory = ref<LatencyPoint[]>([])

  const networkMetrics = ref<NetworkMetric[]>([])

  const totalCapacity = ref(10000)
  const currentLoad = ref(4872)
  const loadPercent = computed(() => Math.round((currentLoad.value / totalCapacity.value) * 100))

  // ============================================
  //  REGIONAL ANALYSIS DATA
  // ============================================
  const regions = ref<RegionData[]>([])

  const totalStudentsAllRegions = computed(() =>
    regions.value.reduce((sum, r) => sum + r.students, 0)
  )

  const avgViolationRateAllRegions = computed(() => {
    const total = regions.value.reduce((sum, r) => sum + r.violationRate, 0)
    return (total / regions.value.length).toFixed(1)
  })

  // ============================================
  //  VIOLATION INTEL DATA
  // ============================================
  const violationCategories = ref<ViolationCategory[]>([])

  const hourlyViolations = ref<HourlyViolation[]>([])

  const detectionAccuracy = ref<DetectionAccuracy[]>([])

  const totalViolationsToday = computed(() =>
    violationCategories.value.reduce((sum, c) => sum + c.count, 0)
  )

  // Violation page filter state
  const violationSelectedExamId = ref<string | null>(null)
  const violationDateRange = ref<'today' | '7d' | '30d' | 'custom'>('today')

  // Proctor Efficiency KPIs
  const proctorKPIs = ref<ProctorKPI[]>([])

  const proctorAvgReactionTime = computed(() => {
    const total = proctorKPIs.value.reduce((s, p) => s + p.avgReactionTimeSec, 0)
    return (total / proctorKPIs.value.length).toFixed(1)
  })

  const proctorAvgWarningAccuracy = computed(() => {
    const total = proctorKPIs.value.reduce((s, p) => s + p.warningAccuracy, 0)
    return (total / proctorKPIs.value.length).toFixed(1)
  })

  // Regional Violation Breakdown
  const regionalViolations = ref<RegionalViolationBreakdown[]>([])

  const sortedRegionalViolations = computed(() =>
    [...regionalViolations.value].sort((a, b) => b.totalViolations - a.totalViolations)
  )

  // ============================================
  //  GLOBAL ANALYTICS DATA
  // ============================================
  const weeklyTrends = ref<WeeklyTrendPoint[]>([])

  const monthlyTrends = ref<MonthlyTrendPoint[]>([])

  const subjectViolationRates = ref<SubjectViolationRate[]>([])

  const institutionRankings = ref<InstitutionRanking[]>([])

  const globalAiConfidence = ref(94.3)
  const activeProctors = ref(6)
  const avgProctorSpeed = ref(10.8)

  // ============================================
  //  API & INTEGRATIONS DATA
  // ============================================
  const apiKeys = ref<ApiKey[]>([])

  const webhooks = ref<Webhook[]>([])

  const webhookDeliveryLogs = ref<WebhookDeliveryLog[]>([])

  const ipWhitelist = ref<IPWhitelistEntry[]>([])

  function addIPEntry(entry: IPWhitelistEntry) {
    ipWhitelist.value.push(entry)
  }

  function removeIPEntry(entryId: string) {
    ipWhitelist.value = ipWhitelist.value.filter(e => e.id !== entryId)
  }

  const apiSystemStatus = ref<'operational' | 'degraded' | 'down'>('operational')

  // ============================================
  //  EXAM PROCTORING SETTINGS (EDUSER SYNC)
  // ============================================
  const eduserSyncing = ref(false)
  const eduserLastSync = ref('2026-02-11T08:30:00Z')

  const examConfigs = ref<ExamProctoringConfig[]>([])

  // Lookup: does a monitoring session student have an exception?
  function getStudentExceptionInfo(studentName: string, iin: string, examName: string): StudentException | null {
    // Find the matching exam config by name
    const exam = examConfigs.value.find(e => e.examName === examName)
    if (!exam || exam.exceptions.length === 0) return null
    // Match by studentId (IIN) or by studentName
    return exam.exceptions.find(exc =>
      exc.studentId === iin || exc.studentName === studentName
    ) ?? null
  }

  function syncEduser() {
    eduserSyncing.value = true
    setTimeout(() => {
      eduserSyncing.value = false
      eduserLastSync.value = new Date().toISOString()
    }, 2500)
  }

  // --- Computed ---
  const totalParticipants = computed(() =>
    activeExams.value.reduce((sum, exam) => sum + exam.participants, 0)
  )

  const criticalAlerts = computed(() =>
    violationAlerts.value.filter(a => a.severity === 'critical')
  )

  // --- Simulated real-time updates ---
  function simulateNewViolation() {
    const names = ['Ной Браун', 'Исла Дэвис', 'Лукас Мартинес', 'Мия Джонсон', 'Этан Ли']
    const types = ['Телефон обнаружен', 'Отклонение взгляда', 'Посторонние лица', 'Смена вкладки', 'Аудио аномалия']
    const severities: ViolationAlert['severity'][] = ['critical', 'warning', 'info']
    const exams = activeExams.value

    const randomExam = exams[Math.floor(Math.random() * exams.length)]
    const newAlert: ViolationAlert = {
      id: `alert-${Date.now()}`,
      orgId: randomExam?.orgId ?? 'org-eduser',
      studentName: names[Math.floor(Math.random() * names.length)] ?? 'Неизвестно',
      violationType: types[Math.floor(Math.random() * types.length)] ?? 'Неизвестно',
      timestamp: new Date().toISOString(),
      severity: severities[Math.floor(Math.random() * severities.length)] ?? 'info',
      examName: randomExam?.examName ?? 'Неизвестно'
    }

    violationAlerts.value.unshift(newAlert)

    if (violationAlerts.value.length > 50) {
      violationAlerts.value.pop()
    }

    if (newAlert.severity === 'critical') {
      criticalViolations.value++
    }

    activeSessions.value += Math.floor(Math.random() * 3) - 1
  }

  // ============================================
  //  LIVE MONITORING DATA
  // ============================================
  const monitoringSessions = ref<MonitoringSession[]>([])

  // Monitoring filter state
  const monitoringSearchQuery = ref('')
  const monitoringViolationFilter = ref<'all' | 'critical' | 'warning'>('all')
  const monitoringSelectedExamId = ref<string | null>(null)

  function selectMonitoringExam(examId: string | null) {
    monitoringSelectedExamId.value = examId
  }

  const filteredMonitoringSessions = computed(() => {
    let sessions = monitoringSessions.value

    // Filter by selected exam
    if (monitoringSelectedExamId.value) {
      sessions = sessions.filter(s => s.examId === monitoringSelectedExamId.value)
    }

    // Filter by violation level
    if (monitoringViolationFilter.value !== 'all') {
      sessions = sessions.filter(s => s.violationLevel === monitoringViolationFilter.value)
    }

    // Filter by search query (name, IIN, phone)
    const q = monitoringSearchQuery.value.trim().toLowerCase()
    if (q) {
      sessions = sessions.filter(s =>
        s.studentName.toLowerCase().includes(q)
        || s.iin.includes(q)
        || s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, ''))
      )
    }

    return sessions
  })

  const monitoringStats = computed(() => {
    const total = monitoringSessions.value.length
    const critical = monitoringSessions.value.filter(s => s.violationLevel === 'critical').length
    const warning = monitoringSessions.value.filter(s => s.violationLevel === 'warning').length
    const clean = monitoringSessions.value.filter(s => s.violationLevel === 'clean').length
    const avgIntegrity = Math.round(monitoringSessions.value.reduce((s, m) => s + m.integrityScore, 0) / total)
    return { total, critical, warning, clean, avgIntegrity }
  })

  // ============================================
  //  SESSION ARCHIVE DATA
  // ============================================
  const archiveExams = ref<ArchiveExamSummary[]>([])

  const archiveSessions = ref<ArchiveSession[]>([])
  const archiveLoading = ref(false)
  const archiveError = ref<string | null>(null)

  // Archive filter state
  const archiveSearchQuery = ref('')
  const archiveSelectedExamId = ref<string | null>(null)
  const archiveStatusFilter = ref<'all' | 'reviewed' | 'pending' | 'voided'>('all')

  function selectArchiveExam(examId: string | null) {
    archiveSelectedExamId.value = examId
  }

  // Fetch real archive data from ClickHouse via API
  async function fetchArchiveData(orgId?: string) {
    archiveLoading.value = true
    archiveError.value = null
    try {
      const { useAdminAPI } = await import('~/composables/useAdminAPI')
      const api = useAdminAPI()

      // Fetch sessions and exams in parallel
      const [sessionsResp, examsResp] = await Promise.all([
        api.getArchiveSessions({ orgId, limit: 200 }),
        api.getArchiveExams(orgId)
      ])

      // Map API sessions to store ArchiveSession type
      archiveSessions.value = sessionsResp.sessions.map(s => ({
        id: s.sessionId,
        orgId: s.orgId,
        studentName: s.studentId, // Will display studentId until we have student names
        iin: '',
        phone: '',
        examId: s.examId,
        examName: s.examId, // Will display examId
        date: s.firstEventTime,
        duration: s.duration,
        integrityScore: s.integrityScore,
        violationCount: Number(s.violationCount),
        status: s.status,
        events: [] // Events loaded on-demand when modal opens
      }))

      // Map API exams to store ArchiveExamSummary type
      archiveExams.value = examsResp.exams.map(e => ({
        id: e.examId,
        orgId: e.orgId,
        examName: e.examId,
        date: e.firstEventTime,
        participants: Number(e.sessionCount),
        avgIntegrity: e.avgIntegrity,
        totalViolations: Number(e.totalCritical) + Number(e.totalWarning),
        reviewed: 0, // Calculated from sessions
        pending: 0,
        voided: 0
      }))

      // Calculate exam status breakdown from sessions
      for (const exam of archiveExams.value) {
        const examSessions = archiveSessions.value.filter(s => s.examId === exam.id)
        exam.reviewed = examSessions.filter(s => s.status === 'reviewed').length
        exam.pending = examSessions.filter(s => s.status === 'pending').length
        exam.voided = examSessions.filter(s => s.status === 'voided').length
        exam.participants = examSessions.length
      }
    } catch (err) {
      console.error('[Store] Failed to fetch archive data:', err)
      archiveError.value = String(err)
    } finally {
      archiveLoading.value = false
    }
  }

  /**
   * Load the dashboard KPI cards from the analytics API. On any failure the
   * existing demo values are preserved (graceful fallback) so the dashboard
   * never renders empty.
   */
  async function fetchOverview(orgId?: string) {
    overviewLoading.value = true
    overviewError.value = null
    try {
      const { useAdminAPI } = await import('~/composables/useAdminAPI')
      const api = useAdminAPI()

      const [overview, sessions] = await Promise.all([
        api.getAnalyticsOverview(orgId),
        api.getActiveSessions(orgId)
      ])

      activeSessions.value = sessions.totalActive
      criticalViolations.value = overview.health.criticalEvents
      const integrity = Math.round(100 - sessions.avgRiskScore)
      avgIntegrityScore.value = Math.min(100, Math.max(0, integrity))
      systemHealth.value = overview.health.flushErrors > 0 ? 'degraded' : 'operational'

      overviewUsingRealData.value = true
    } catch (err) {
      console.error('[Store] Failed to fetch dashboard overview:', err)
      overviewError.value = String(err)
      overviewUsingRealData.value = false // keep demo values
    } finally {
      overviewLoading.value = false
    }
  }

  // Fetch events for a specific session (on-demand when modal opens)
  async function fetchSessionEvents(sessionId: string): Promise<ArchiveEvent[]> {
    try {
      const { useAdminAPI } = await import('~/composables/useAdminAPI')
      const api = useAdminAPI()
      const resp = await api.getArchiveSessionEvents(sessionId)

      // Map API events to store ArchiveEvent type
      const events: ArchiveEvent[] = resp.events.map(e => ({
        id: e.eventId,
        timestamp: e.timestamp,
        type: e.eventType,
        label: e.label,
        severity: e.severity,
        videoTimestamp: e.videoTimestamp,
        source: (e.source as 'webcam' | 'side' | 'system') || 'system'
      }))

      // Update the session's events in the store
      const session = archiveSessions.value.find(s => s.id === sessionId)
      if (session) {
        session.events = events
      }

      return events
    } catch (err) {
      console.error('[Store] Failed to fetch session events:', err)
      return []
    }
  }

  // Export session report as JSON
  async function exportSessionReport(sessionId: string): Promise<void> {
    try {
      const { useAdminAPI } = await import('~/composables/useAdminAPI')
      const api = useAdminAPI()
      const data = await api.exportArchiveSession(sessionId)

      // Trigger download
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `argus-session-${sessionId}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch (err) {
      console.error('[Store] Failed to export session:', err)
    }
  }

  const filteredArchiveSessions = computed(() => {
    let sessions = archiveSessions.value

    if (archiveSelectedExamId.value) {
      sessions = sessions.filter(s => s.examId === archiveSelectedExamId.value)
    }

    if (archiveStatusFilter.value !== 'all') {
      sessions = sessions.filter(s => s.status === archiveStatusFilter.value)
    }

    const q = archiveSearchQuery.value.trim().toLowerCase()
    if (q) {
      sessions = sessions.filter(s =>
        s.studentName.toLowerCase().includes(q)
        || s.iin.includes(q)
        || s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, ''))
      )
    }

    return sessions
  })

  const archiveStats = computed(() => {
    const total = archiveSessions.value.length
    const reviewed = archiveSessions.value.filter(s => s.status === 'reviewed').length
    const pending = archiveSessions.value.filter(s => s.status === 'pending').length
    const voided = archiveSessions.value.filter(s => s.status === 'voided').length
    return { total, reviewed, pending, voided }
  })

  // ============================================
  //  MONITORING MODAL STATE (shared across pages)
  // ============================================
  const monitoringModalSession = ref<MonitoringSession | null>(null)
  const monitoringModalIsLive = ref(true)

  /**
   * Open the monitoring modal for a given session.
   * Can be called from any page (Dashboard, Violations, Archive).
   * If sessionId is provided, finds it in monitoringSessions.
   */
  function openMonitoringModal(session: MonitoringSession, isLive: boolean = true) {
    monitoringModalSession.value = session
    monitoringModalIsLive.value = isLive
  }

  /**
   * Open monitoring modal by matching a ViolationAlert to a MonitoringSession.
   * Matches by studentName (since alerts don't carry session IDs).
   */
  function openMonitoringModalFromAlert(alert: ViolationAlert) {
    // Try exact match first (studentName + examName)
    let session = monitoringSessions.value.find(s =>
      s.studentName === alert.studentName && s.examName === alert.examName
    )
    // Fallback: match by studentName only
    if (!session) {
      session = monitoringSessions.value.find(s =>
        s.studentName === alert.studentName
      )
    }
    if (session) {
      monitoringModalSession.value = session
      monitoringModalIsLive.value = session.isOnline
    } else {
      // No matching session found — create a synthetic session from alert data
      const syntheticSession: MonitoringSession = {
        id: `alert-${alert.id}`,
        orgId: alert.orgId,
        studentName: alert.studentName,
        iin: '—',
        phone: '—',
        examId: '',
        examName: alert.examName,
        integrityScore: 0,
        violationCount: 1,
        violationLevel: alert.severity === 'critical' ? 'critical' : alert.severity === 'warning' ? 'warning' : 'clean',
        aiStatus: {
          gazeTracking: 'normal',
          faceIdMatch: 'verified',
          objectDetection: 'clear'
        },
        isOnline: true,
        startedAt: alert.timestamp,
        lastActivity: alert.timestamp,
        events: [{
          id: `evt-${alert.id}`,
          timestamp: alert.timestamp,
          type: alert.violationType as MonitoringEvent['type'],
          label: alert.violationType,
          severity: alert.severity
        }]
      }
      monitoringModalSession.value = syntheticSession
      monitoringModalIsLive.value = true
    }
  }

  function closeMonitoringModal() {
    monitoringModalSession.value = null
  }

  // --- Tests Module ---
  const tests = ref<TestItem[]>([])

  const complexTests = ref<ComplexTestItem[]>([])

  function deleteTest(testId: string) {
    tests.value = tests.value.filter(t => t.id !== testId)
  }

  function deleteComplexTest(testId: string) {
    complexTests.value = complexTests.value.filter(t => t.id !== testId)
  }

  // ============================================
  //  ORG-AWARE FILTERING (Multi-Tenant)
  // ============================================

  const orgNameMap: Record<string, string> = {
    'org-eduser': 'EDUSER',
    'org-nis': 'НИШ',
    'org-kaznu': 'КазНУ'
  }

  function getOrgName(orgId: string): string {
    return orgNameMap[orgId] || orgId
  }

  // Org-filtered active exams
  const orgFilteredActiveExams = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return activeExams.value // null = all orgs (Super Admin)
    return activeExams.value.filter(e => e.orgId === orgId)
  })

  // Org-filtered violation alerts
  const orgFilteredViolationAlerts = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return violationAlerts.value
    return violationAlerts.value.filter(a => a.orgId === orgId)
  })

  // Org-filtered exam configs
  const orgFilteredExamConfigs = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return examConfigs.value
    return examConfigs.value.filter(e => e.orgId === orgId)
  })

  // Org-filtered monitoring sessions
  const orgFilteredMonitoringSessions = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return monitoringSessions.value
    return monitoringSessions.value.filter(s => s.orgId === orgId)
  })

  // Org-filtered archive sessions
  const orgFilteredArchiveSessions = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return archiveSessions.value
    return archiveSessions.value.filter(s => s.orgId === orgId)
  })

  // Org-filtered archive exam summaries
  const orgFilteredArchiveExams = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return archiveExams.value
    return archiveExams.value.filter(e => e.orgId === orgId)
  })

  return {
    // UI State
    leftSidebarOpen,
    rightPanelOpen,
    dashboardMenuOpen,
    toggleLeftSidebar,
    toggleRightPanel,
    toggleDashboardMenu,
    // Filter State
    selectedExamId,
    selectedExam,
    isFiltered,
    studentSearchQuery,
    selectExam,
    clearFilter,
    // KPI
    activeSessions,
    criticalViolations,
    avgIntegrityScore,
    systemHealth,
    systemHealthLabel,
    systemHealthUptime,
    overviewLoading,
    overviewError,
    overviewUsingRealData,
    fetchOverview,
    // Filtered KPI
    filteredActiveSessions,
    filteredTotalParticipants,
    filteredCriticalViolations,
    filteredAvgIntegrity,
    filteredAlerts,
    filteredAtRiskStudents,
    filteredCriticalAlerts,
    // Search
    studentRoster,
    searchResults,
    // Data
    activeExams,
    violationAlerts,
    atRiskStudents,
    violationTrends,
    totalParticipants,
    criticalAlerts,
    simulateNewViolation,
    // Infrastructure Health
    serverNodes,
    latencyHistory,
    networkMetrics,
    totalCapacity,
    currentLoad,
    loadPercent,
    // Regional Analysis
    regions,
    totalStudentsAllRegions,
    avgViolationRateAllRegions,
    // Violation Intel
    violationCategories,
    hourlyViolations,
    detectionAccuracy,
    totalViolationsToday,
    violationSelectedExamId,
    violationDateRange,
    proctorKPIs,
    proctorAvgReactionTime,
    proctorAvgWarningAccuracy,
    regionalViolations,
    sortedRegionalViolations,
    // Live Monitoring
    monitoringSessions,
    monitoringSearchQuery,
    monitoringViolationFilter,
    monitoringSelectedExamId,
    selectMonitoringExam,
    filteredMonitoringSessions,
    monitoringStats,
    // Session Archive
    archiveExams,
    archiveSessions,
    archiveSearchQuery,
    archiveSelectedExamId,
    archiveStatusFilter,
    selectArchiveExam,
    filteredArchiveSessions,
    archiveStats,
    archiveLoading,
    archiveError,
    fetchArchiveData,
    fetchSessionEvents,
    exportSessionReport,
    // API & Integrations
    apiKeys,
    webhooks,
    webhookDeliveryLogs,
    apiSystemStatus,
    ipWhitelist,
    addIPEntry,
    removeIPEntry,
    // Global Analytics
    weeklyTrends,
    monthlyTrends,
    subjectViolationRates,
    institutionRankings,
    globalAiConfidence,
    activeProctors,
    avgProctorSpeed,
    // Tests Module
    tests,
    complexTests,
    deleteTest,
    deleteComplexTest,
    // Exam Proctoring Settings
    examConfigs,
    eduserSyncing,
    eduserLastSync,
    syncEduser,
    getStudentExceptionInfo,
    // Monitoring Modal (shared)
    monitoringModalSession,
    monitoringModalIsLive,
    openMonitoringModal,
    openMonitoringModalFromAlert,
    closeMonitoringModal,
    // Org-Aware Filtering (Multi-Tenant)
    orgFilteredActiveExams,
    orgFilteredViolationAlerts,
    orgFilteredExamConfigs,
    orgFilteredMonitoringSessions,
    orgFilteredArchiveSessions,
    orgFilteredArchiveExams,
    getOrgName
  }
})
