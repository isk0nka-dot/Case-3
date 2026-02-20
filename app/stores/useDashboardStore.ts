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
  type: 'gaze_deviation' | 'face_mismatch' | 'phone_detected' | 'book_detected' | 'tab_switch' | 'audio_anomaly' | 'earbuds_detected' | 'multiple_persons'
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

  const systemHealthLabel = computed(() => {
    const map = { operational: 'В норме', degraded: 'Снижение', down: 'Недоступно' }
    return map[systemHealth.value]
  })

  // --- Active Exams ---
  const activeExams = ref<ActiveExam[]>([
    {
      id: 'exam-001',
      orgId: 'org-eduser',
      examName: 'Высшая математика — Финал',
      startTime: '2026-02-11T09:00:00Z',
      participants: 86,
      violationRate: 3.2
    },
    {
      id: 'exam-002',
      orgId: 'org-eduser',
      examName: 'Информатика 101 — Промежуточный',
      startTime: '2026-02-11T10:30:00Z',
      participants: 124,
      violationRate: 1.8
    },
    {
      id: 'exam-003',
      orgId: 'org-nis',
      examName: 'Органическая химия — Тест 4',
      startTime: '2026-02-11T11:00:00Z',
      participants: 45,
      violationRate: 6.7
    },
    {
      id: 'exam-004',
      orgId: 'org-nis',
      examName: 'Английская литература — Эссе',
      startTime: '2026-02-11T08:00:00Z',
      participants: 67,
      violationRate: 0.9
    },
    {
      id: 'exam-005',
      orgId: 'org-eduser',
      examName: 'Структуры данных — Практика',
      startTime: '2026-02-11T13:00:00Z',
      participants: 38,
      violationRate: 4.5
    }
  ])

  // --- Violation Alerts (Real-time Feed) ---
  const violationAlerts = ref<ViolationAlert[]>([
    { id: 'alert-001', orgId: 'org-nis', studentName: 'Алексей Чен', violationType: 'Телефон обнаружен', timestamp: '2026-02-11T11:42:15Z', severity: 'critical', examName: 'Органическая химия — Тест 4' },
    { id: 'alert-002', orgId: 'org-eduser', studentName: 'Мария Сантос', violationType: 'Отклонение взгляда', timestamp: '2026-02-11T11:41:58Z', severity: 'warning', examName: 'Высшая математика — Финал' },
    { id: 'alert-003', orgId: 'org-eduser', studentName: 'Джеймс Уилсон', violationType: 'Посторонние лица', timestamp: '2026-02-11T11:41:30Z', severity: 'critical', examName: 'Информатика 101 — Промежуточный' },
    { id: 'alert-004', orgId: 'org-eduser', studentName: 'Сара Ким', violationType: 'Смена вкладки', timestamp: '2026-02-11T11:40:45Z', severity: 'warning', examName: 'Структуры данных — Практика' },
    { id: 'alert-005', orgId: 'org-nis', studentName: 'Давид Окафор', violationType: 'Телефон обнаружен', timestamp: '2026-02-11T11:40:12Z', severity: 'critical', examName: 'Органическая химия — Тест 4' },
    { id: 'alert-006', orgId: 'org-eduser', studentName: 'Эмили Чжан', violationType: 'Отклонение взгляда', timestamp: '2026-02-11T11:39:50Z', severity: 'warning', examName: 'Высшая математика — Финал' },
    { id: 'alert-007', orgId: 'org-nis', studentName: 'Радж Патель', violationType: 'Аудио аномалия', timestamp: '2026-02-11T11:38:30Z', severity: 'info', examName: 'Английская литература — Эссе' },
    { id: 'alert-008', orgId: 'org-eduser', studentName: 'Лина Мюллер', violationType: 'Посторонние лица', timestamp: '2026-02-11T11:37:15Z', severity: 'critical', examName: 'Информатика 101 — Промежуточный' },
    { id: 'alert-009', orgId: 'org-eduser', studentName: 'Том Бейкер', violationType: 'Смена вкладки', timestamp: '2026-02-11T11:36:42Z', severity: 'warning', examName: 'Структуры данных — Практика' },
    { id: 'alert-010', orgId: 'org-nis', studentName: 'Айко Танака', violationType: 'Телефон обнаружен', timestamp: '2026-02-11T11:35:20Z', severity: 'critical', examName: 'Органическая химия — Тест 4' }
  ])

  // --- At-Risk Students ---
  const atRiskStudents = ref<AtRiskStudent[]>([
    { id: 'student-001', orgId: 'org-nis', name: 'Алексей Чен', iin: '010315500421', examName: 'Органическая химия — Тест 4', integrityScore: 34, violations: 7 },
    { id: 'student-002', orgId: 'org-eduser', name: 'Джеймс Уилсон', iin: '020728600312', examName: 'Информатика 101 — Промежуточный', integrityScore: 41, violations: 5 },
    { id: 'student-003', orgId: 'org-nis', name: 'Давид Окафор', iin: '990916400589', examName: 'Органическая химия — Тест 4', integrityScore: 52, violations: 4 },
    { id: 'student-004', orgId: 'org-eduser', name: 'Лина Мюллер', iin: '030204700143', examName: 'Информатика 101 — Промежуточный', integrityScore: 58, violations: 3 },
    { id: 'student-005', orgId: 'org-eduser', name: 'Сара Ким', iin: '011130500278', examName: 'Структуры данных — Практика', integrityScore: 63, violations: 3 }
  ])

  // --- Student Roster (mock 10k — represented as sample) ---
  const studentRoster = ref<StudentRecord[]>([
    { id: 'sr-001', orgId: 'org-nis', name: 'Алексей Чен', iin: '010315500421', phone: '+7 701 234 5601', examId: 'exam-003', examName: 'Органическая химия — Тест 4', integrityScore: 34, violations: 7, status: 'flagged' },
    { id: 'sr-002', orgId: 'org-eduser', name: 'Мария Сантос', iin: '020819500632', phone: '+7 702 345 6702', examId: 'exam-001', examName: 'Высшая математика — Финал', integrityScore: 78, violations: 2, status: 'active' },
    { id: 'sr-003', orgId: 'org-eduser', name: 'Джеймс Уилсон', iin: '020728600312', phone: '+7 705 456 7803', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный', integrityScore: 41, violations: 5, status: 'flagged' },
    { id: 'sr-004', orgId: 'org-eduser', name: 'Сара Ким', iin: '011130500278', phone: '+7 707 567 8904', examId: 'exam-005', examName: 'Структуры данных — Практика', integrityScore: 63, violations: 3, status: 'flagged' },
    { id: 'sr-005', orgId: 'org-nis', name: 'Давид Окафор', iin: '990916400589', phone: '+7 700 678 9005', examId: 'exam-003', examName: 'Органическая химия — Тест 4', integrityScore: 52, violations: 4, status: 'flagged' },
    { id: 'sr-006', orgId: 'org-eduser', name: 'Эмили Чжан', iin: '031005500847', phone: '+7 708 789 0106', examId: 'exam-001', examName: 'Высшая математика — Финал', integrityScore: 81, violations: 1, status: 'active' },
    { id: 'sr-007', orgId: 'org-nis', name: 'Радж Патель', iin: '000412600923', phone: '+7 771 890 1207', examId: 'exam-004', examName: 'Английская литература — Эссе', integrityScore: 88, violations: 1, status: 'clean' },
    { id: 'sr-008', orgId: 'org-eduser', name: 'Лина Мюллер', iin: '030204700143', phone: '+7 775 901 2308', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный', integrityScore: 58, violations: 3, status: 'flagged' },
    { id: 'sr-009', orgId: 'org-eduser', name: 'Том Бейкер', iin: '010622500164', phone: '+7 778 012 3409', examId: 'exam-005', examName: 'Структуры данных — Практика', integrityScore: 72, violations: 2, status: 'active' },
    { id: 'sr-010', orgId: 'org-nis', name: 'Айко Танака', iin: '020917500735', phone: '+7 701 123 4510', examId: 'exam-003', examName: 'Органическая химия — Тест 4', integrityScore: 45, violations: 6, status: 'flagged' },
    { id: 'sr-011', orgId: 'org-eduser', name: 'Нурлан Касымов', iin: '990403500128', phone: '+7 702 234 5611', examId: 'exam-001', examName: 'Высшая математика — Финал', integrityScore: 92, violations: 0, status: 'clean' },
    { id: 'sr-012', orgId: 'org-eduser', name: 'Айгерим Тулебаева', iin: '010814500396', phone: '+7 705 345 6712', examId: 'exam-001', examName: 'Высшая математика — Финал', integrityScore: 85, violations: 1, status: 'clean' },
    { id: 'sr-013', orgId: 'org-eduser', name: 'Бекзат Серикбаев', iin: '000127600542', phone: '+7 707 456 7813', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный', integrityScore: 76, violations: 2, status: 'active' },
    { id: 'sr-014', orgId: 'org-eduser', name: 'Динара Жумабекова', iin: '021103500418', phone: '+7 700 567 8914', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный', integrityScore: 69, violations: 3, status: 'active' },
    { id: 'sr-015', orgId: 'org-nis', name: 'Ерлан Абдрахманов', iin: '980615500873', phone: '+7 708 678 9015', examId: 'exam-003', examName: 'Органическая химия — Тест 4', integrityScore: 55, violations: 4, status: 'flagged' },
    { id: 'sr-016', orgId: 'org-nis', name: 'Жанар Муратова', iin: '030529700261', phone: '+7 771 789 0116', examId: 'exam-004', examName: 'Английская литература — Эссе', integrityScore: 91, violations: 0, status: 'clean' },
    { id: 'sr-017', orgId: 'org-nis', name: 'Канат Нурмагамбетов', iin: '010207500547', phone: '+7 775 890 1217', examId: 'exam-004', examName: 'Английская литература — Эссе', integrityScore: 83, violations: 1, status: 'clean' },
    { id: 'sr-018', orgId: 'org-eduser', name: 'Мадина Оспанова', iin: '020416500689', phone: '+7 778 901 2318', examId: 'exam-005', examName: 'Структуры данных — Практика', integrityScore: 74, violations: 2, status: 'active' },
    { id: 'sr-019', orgId: 'org-eduser', name: 'Руслан Байжанов', iin: '991228600327', phone: '+7 701 012 3419', examId: 'exam-005', examName: 'Структуры данных — Практика', integrityScore: 67, violations: 3, status: 'active' },
    { id: 'sr-020', orgId: 'org-eduser', name: 'Салтанат Ережепова', iin: '000831500412', phone: '+7 702 123 4520', examId: 'exam-001', examName: 'Высшая математика — Финал', integrityScore: 79, violations: 2, status: 'active' }
  ])

  // --- Violation Trend Data (last 12 hours) ---
  const violationTrends = ref<ViolationTrendPoint[]>([
    { time: '00:00', phoneDetected: 2, gazeDeviation: 5, multiplePersons: 0, tabSwitch: 3 },
    { time: '01:00', phoneDetected: 1, gazeDeviation: 3, multiplePersons: 0, tabSwitch: 1 },
    { time: '02:00', phoneDetected: 0, gazeDeviation: 1, multiplePersons: 0, tabSwitch: 0 },
    { time: '03:00', phoneDetected: 0, gazeDeviation: 0, multiplePersons: 0, tabSwitch: 0 },
    { time: '04:00', phoneDetected: 0, gazeDeviation: 0, multiplePersons: 0, tabSwitch: 0 },
    { time: '05:00', phoneDetected: 1, gazeDeviation: 2, multiplePersons: 0, tabSwitch: 1 },
    { time: '06:00', phoneDetected: 3, gazeDeviation: 4, multiplePersons: 1, tabSwitch: 2 },
    { time: '07:00', phoneDetected: 5, gazeDeviation: 8, multiplePersons: 2, tabSwitch: 4 },
    { time: '08:00', phoneDetected: 8, gazeDeviation: 12, multiplePersons: 3, tabSwitch: 7 },
    { time: '09:00', phoneDetected: 12, gazeDeviation: 18, multiplePersons: 4, tabSwitch: 9 },
    { time: '10:00', phoneDetected: 15, gazeDeviation: 22, multiplePersons: 5, tabSwitch: 11 },
    { time: '11:00', phoneDetected: 18, gazeDeviation: 25, multiplePersons: 6, tabSwitch: 14 }
  ])

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
  const serverNodes = ref<ServerNode[]>([
    { id: 'srv-01', name: 'KZ-Primary-01', region: 'Алматы', cpuLoad: 67, memoryUsage: 72, diskUsage: 45, status: 'healthy', latencyMs: 12, uptime: 99.99, connections: 3420 },
    { id: 'srv-02', name: 'KZ-Primary-02', region: 'Алматы', cpuLoad: 73, memoryUsage: 68, diskUsage: 51, status: 'healthy', latencyMs: 14, uptime: 99.98, connections: 2890 },
    { id: 'srv-03', name: 'KZ-North-01', region: 'Астана', cpuLoad: 82, memoryUsage: 85, diskUsage: 62, status: 'warning', latencyMs: 28, uptime: 99.91, connections: 2140 },
    { id: 'srv-04', name: 'KZ-West-01', region: 'Актау', cpuLoad: 45, memoryUsage: 52, diskUsage: 38, status: 'healthy', latencyMs: 45, uptime: 99.95, connections: 890 },
    { id: 'srv-05', name: 'KZ-South-01', region: 'Шымкент', cpuLoad: 58, memoryUsage: 61, diskUsage: 44, status: 'healthy', latencyMs: 32, uptime: 99.97, connections: 1650 },
    { id: 'srv-06', name: 'KZ-East-01', region: 'Усть-Каменогорск', cpuLoad: 91, memoryUsage: 88, diskUsage: 73, status: 'critical', latencyMs: 67, uptime: 99.82, connections: 1120 },
    { id: 'srv-07', name: 'CDN-Edge-01', region: 'Франкфурт', cpuLoad: 34, memoryUsage: 41, diskUsage: 29, status: 'healthy', latencyMs: 89, uptime: 100, connections: 540 },
    { id: 'srv-08', name: 'CDN-Edge-02', region: 'Москва', cpuLoad: 52, memoryUsage: 58, diskUsage: 35, status: 'healthy', latencyMs: 42, uptime: 99.99, connections: 780 }
  ])

  const latencyHistory = ref<LatencyPoint[]>([
    { time: '00:00', avg: 18, p95: 45, p99: 92 },
    { time: '01:00', avg: 15, p95: 38, p99: 78 },
    { time: '02:00', avg: 12, p95: 32, p99: 65 },
    { time: '03:00', avg: 11, p95: 28, p99: 58 },
    { time: '04:00', avg: 13, p95: 30, p99: 62 },
    { time: '05:00', avg: 16, p95: 42, p99: 85 },
    { time: '06:00', avg: 22, p95: 55, p99: 110 },
    { time: '07:00', avg: 28, p95: 68, p99: 135 },
    { time: '08:00', avg: 35, p95: 82, p99: 168 },
    { time: '09:00', avg: 42, p95: 95, p99: 195 },
    { time: '10:00', avg: 38, p95: 88, p99: 178 },
    { time: '11:00', avg: 32, p95: 75, p99: 152 }
  ])

  const networkMetrics = ref<NetworkMetric[]>([
    { label: 'Пропускная способность', value: 847, unit: 'Мбит/с', status: 'healthy', trend: 'stable' },
    { label: 'Пакетная потеря', value: 0.02, unit: '%', status: 'healthy', trend: 'down' },
    { label: 'TCP подключения', value: 13430, unit: '', status: 'healthy', trend: 'up' },
    { label: 'WebSocket соединения', value: 4872, unit: '', status: 'healthy', trend: 'up' },
    { label: 'DNS резолвинг', value: 8, unit: 'мс', status: 'healthy', trend: 'stable' },
    { label: 'SSL рукопожатие', value: 24, unit: 'мс', status: 'healthy', trend: 'stable' }
  ])

  const totalCapacity = ref(10000)
  const currentLoad = ref(4872)
  const loadPercent = computed(() => Math.round((currentLoad.value / totalCapacity.value) * 100))

  // ============================================
  //  REGIONAL ANALYSIS DATA
  // ============================================
  const regions = ref<RegionData[]>([
    { id: 'reg-01', name: 'Алматы', nameKz: 'Алматы қ.', students: 28450, activeSessions: 67, violationRate: 2.8, avgIntegrity: 89, topViolation: 'Отклонение взгляда' },
    { id: 'reg-02', name: 'Астана', nameKz: 'Астана қ.', students: 22180, activeSessions: 54, violationRate: 3.1, avgIntegrity: 87, topViolation: 'Смена вкладки' },
    { id: 'reg-03', name: 'Шымкент', nameKz: 'Шымкент қ.', students: 14320, activeSessions: 38, violationRate: 4.2, avgIntegrity: 82, topViolation: 'Телефон обнаружен' },
    { id: 'reg-04', name: 'Караганда', nameKz: 'Қарағанды обл.', students: 9870, activeSessions: 24, violationRate: 3.5, avgIntegrity: 85, topViolation: 'Отклонение взгляда' },
    { id: 'reg-05', name: 'Актобе', nameKz: 'Ақтөбе обл.', students: 7650, activeSessions: 18, violationRate: 2.9, avgIntegrity: 88, topViolation: 'Смена вкладки' },
    { id: 'reg-06', name: 'Павлодар', nameKz: 'Павлодар обл.', students: 6240, activeSessions: 15, violationRate: 3.8, avgIntegrity: 84, topViolation: 'Посторонние лица' },
    { id: 'reg-07', name: 'Усть-Каменогорск', nameKz: 'Шығыс Қазақстан обл.', students: 5890, activeSessions: 14, violationRate: 5.1, avgIntegrity: 79, topViolation: 'Телефон обнаружен' },
    { id: 'reg-08', name: 'Семей', nameKz: 'Абай обл.', students: 4750, activeSessions: 11, violationRate: 4.6, avgIntegrity: 81, topViolation: 'Посторонние лица' },
    { id: 'reg-09', name: 'Атырау', nameKz: 'Атырау обл.', students: 4320, activeSessions: 9, violationRate: 3.2, avgIntegrity: 86, topViolation: 'Отклонение взгляда' },
    { id: 'reg-10', name: 'Костанай', nameKz: 'Қостанай обл.', students: 5120, activeSessions: 12, violationRate: 3.4, avgIntegrity: 85, topViolation: 'Смена вкладки' },
    { id: 'reg-11', name: 'Тараз', nameKz: 'Жамбыл обл.', students: 4980, activeSessions: 10, violationRate: 4.0, avgIntegrity: 83, topViolation: 'Телефон обнаружен' },
    { id: 'reg-12', name: 'Петропавловск', nameKz: 'Солтүстік Қазақстан обл.', students: 3780, activeSessions: 8, violationRate: 2.6, avgIntegrity: 90, topViolation: 'Смена вкладки' },
    { id: 'reg-13', name: 'Актау', nameKz: 'Маңғыстау обл.', students: 3450, activeSessions: 7, violationRate: 3.0, avgIntegrity: 87, topViolation: 'Отклонение взгляда' },
    { id: 'reg-14', name: 'Кызылорда', nameKz: 'Қызылорда обл.', students: 3210, activeSessions: 6, violationRate: 4.4, avgIntegrity: 80, topViolation: 'Телефон обнаружен' },
    { id: 'reg-15', name: 'Туркестан', nameKz: 'Түркістан обл.', students: 6890, activeSessions: 16, violationRate: 4.8, avgIntegrity: 78, topViolation: 'Телефон обнаружен' },
    { id: 'reg-16', name: 'Кокшетау', nameKz: 'Ақмола обл.', students: 3420, activeSessions: 7, violationRate: 2.7, avgIntegrity: 89, topViolation: 'Смена вкладки' },
    { id: 'reg-17', name: 'Талдыкорган', nameKz: 'Жетісу обл.', students: 2890, activeSessions: 5, violationRate: 3.6, avgIntegrity: 84, topViolation: 'Отклонение взгляда' }
  ])

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
  const violationCategories = ref<ViolationCategory[]>([
    { type: 'Телефон обнаружен', icon: 'i-lucide-smartphone', count: 342, percentChange: 12.5, trend: 'up', severity: 'critical' },
    { type: 'Отклонение взгляда', icon: 'i-lucide-eye-off', count: 518, percentChange: -3.2, trend: 'down', severity: 'warning' },
    { type: 'Посторонние лица', icon: 'i-lucide-users', count: 127, percentChange: 8.1, trend: 'up', severity: 'critical' },
    { type: 'Смена вкладки', icon: 'i-lucide-app-window', count: 289, percentChange: -1.8, trend: 'down', severity: 'warning' },
    { type: 'Аудио аномалия', icon: 'i-lucide-mic-off', count: 94, percentChange: 5.4, trend: 'up', severity: 'info' },
    { type: 'Динамика набора', icon: 'i-lucide-keyboard', count: 73, percentChange: 15.2, trend: 'up', severity: 'warning' },
    { type: 'Десинхронизация курсора', icon: 'i-lucide-mouse-pointer', count: 41, percentChange: 9.8, trend: 'up', severity: 'critical' }
  ])

  const hourlyViolations = ref<HourlyViolation[]>([
    { hour: '06:00', phone: 1, gaze: 2, persons: 0, tabs: 1, audio: 0 },
    { hour: '07:00', phone: 3, gaze: 5, persons: 1, tabs: 2, audio: 1 },
    { hour: '08:00', phone: 8, gaze: 12, persons: 3, tabs: 5, audio: 2 },
    { hour: '09:00', phone: 15, gaze: 22, persons: 5, tabs: 10, audio: 4 },
    { hour: '10:00', phone: 22, gaze: 30, persons: 7, tabs: 14, audio: 5 },
    { hour: '11:00', phone: 28, gaze: 35, persons: 9, tabs: 18, audio: 6 },
    { hour: '12:00', phone: 18, gaze: 25, persons: 6, tabs: 12, audio: 4 },
    { hour: '13:00', phone: 24, gaze: 32, persons: 8, tabs: 16, audio: 5 },
    { hour: '14:00', phone: 30, gaze: 38, persons: 10, tabs: 20, audio: 7 },
    { hour: '15:00', phone: 20, gaze: 28, persons: 6, tabs: 13, audio: 4 },
    { hour: '16:00', phone: 12, gaze: 18, persons: 4, tabs: 8, audio: 3 },
    { hour: '17:00', phone: 5, gaze: 8, persons: 2, tabs: 4, audio: 1 }
  ])

  const detectionAccuracy = ref<DetectionAccuracy[]>([
    { type: 'Телефон обнаружен', truePositive: 96.2, falsePositive: 2.1, accuracy: 97.1 },
    { type: 'Отклонение взгляда', truePositive: 89.5, falsePositive: 5.8, accuracy: 91.8 },
    { type: 'Посторонние лица', truePositive: 94.8, falsePositive: 3.4, accuracy: 95.7 },
    { type: 'Смена вкладки', truePositive: 99.9, falsePositive: 0.1, accuracy: 99.9 },
    { type: 'Аудио аномалия', truePositive: 82.3, falsePositive: 8.7, accuracy: 86.8 },
    { type: 'Динамика набора', truePositive: 91.4, falsePositive: 4.3, accuracy: 93.5 },
    { type: 'Десинхронизация курсора', truePositive: 88.7, falsePositive: 6.1, accuracy: 91.3 }
  ])

  const totalViolationsToday = computed(() =>
    violationCategories.value.reduce((sum, c) => sum + c.count, 0)
  )

  // Violation page filter state
  const violationSelectedExamId = ref<string | null>(null)
  const violationDateRange = ref<'today' | '7d' | '30d' | 'custom'>('today')

  // Proctor Efficiency KPIs
  const proctorKPIs = ref<ProctorKPI[]>([
    { id: 'pr-01', name: 'Ержан Касымов', sessionsReviewed: 48, avgReactionTimeSec: 12, warningsIssued: 34, warningAccuracy: 94.2, terminationsInitiated: 3, violationsDetected: 87, shift: '09:00–17:00' },
    { id: 'pr-02', name: 'Айгерим Нурланова', sessionsReviewed: 52, avgReactionTimeSec: 9, warningsIssued: 41, warningAccuracy: 96.8, terminationsInitiated: 5, violationsDetected: 104, shift: '09:00–17:00' },
    { id: 'pr-03', name: 'Дамир Абдрахманов', sessionsReviewed: 39, avgReactionTimeSec: 15, warningsIssued: 28, warningAccuracy: 89.3, terminationsInitiated: 2, violationsDetected: 65, shift: '13:00–21:00' },
    { id: 'pr-04', name: 'Мадина Сатпаева', sessionsReviewed: 55, avgReactionTimeSec: 8, warningsIssued: 45, warningAccuracy: 97.5, terminationsInitiated: 6, violationsDetected: 112, shift: '09:00–17:00' },
    { id: 'pr-05', name: 'Бауыржан Токтаров', sessionsReviewed: 43, avgReactionTimeSec: 14, warningsIssued: 30, warningAccuracy: 91.0, terminationsInitiated: 4, violationsDetected: 78, shift: '13:00–21:00' },
    { id: 'pr-06', name: 'Жанна Оспанова', sessionsReviewed: 61, avgReactionTimeSec: 7, warningsIssued: 50, warningAccuracy: 98.1, terminationsInitiated: 7, violationsDetected: 128, shift: '06:00–14:00' }
  ])

  const proctorAvgReactionTime = computed(() => {
    const total = proctorKPIs.value.reduce((s, p) => s + p.avgReactionTimeSec, 0)
    return (total / proctorKPIs.value.length).toFixed(1)
  })

  const proctorAvgWarningAccuracy = computed(() => {
    const total = proctorKPIs.value.reduce((s, p) => s + p.warningAccuracy, 0)
    return (total / proctorKPIs.value.length).toFixed(1)
  })

  // Regional Violation Breakdown
  const regionalViolations = ref<RegionalViolationBreakdown[]>([
    { regionId: 'reg-01', regionName: 'Алматы', totalViolations: 287, phone: 68, gaze: 95, persons: 32, tabs: 62, audio: 30, criticalRate: 34.8, avgReactionSec: 10 },
    { regionId: 'reg-02', regionName: 'Астана', totalViolations: 234, phone: 52, gaze: 78, persons: 28, tabs: 54, audio: 22, criticalRate: 34.2, avgReactionSec: 11 },
    { regionId: 'reg-03', regionName: 'Шымкент', totalViolations: 198, phone: 62, gaze: 54, persons: 24, tabs: 38, audio: 20, criticalRate: 43.4, avgReactionSec: 14 },
    { regionId: 'reg-04', regionName: 'Караганда', totalViolations: 145, phone: 35, gaze: 48, persons: 18, tabs: 30, audio: 14, criticalRate: 36.6, avgReactionSec: 12 },
    { regionId: 'reg-05', regionName: 'Актобе', totalViolations: 112, phone: 24, gaze: 38, persons: 14, tabs: 26, audio: 10, criticalRate: 33.9, avgReactionSec: 13 },
    { regionId: 'reg-06', regionName: 'Павлодар', totalViolations: 98, phone: 28, gaze: 30, persons: 12, tabs: 18, audio: 10, criticalRate: 40.8, avgReactionSec: 15 },
    { regionId: 'reg-07', regionName: 'Усть-Каменогорск', totalViolations: 134, phone: 42, gaze: 36, persons: 16, tabs: 28, audio: 12, criticalRate: 43.3, avgReactionSec: 16 },
    { regionId: 'reg-08', regionName: 'Семей', totalViolations: 89, phone: 28, gaze: 24, persons: 12, tabs: 16, audio: 9, criticalRate: 44.9, avgReactionSec: 17 },
    { regionId: 'reg-09', regionName: 'Атырау', totalViolations: 76, phone: 18, gaze: 22, persons: 10, tabs: 18, audio: 8, criticalRate: 36.8, avgReactionSec: 12 },
    { regionId: 'reg-10', regionName: 'Костанай', totalViolations: 84, phone: 20, gaze: 28, persons: 10, tabs: 18, audio: 8, criticalRate: 35.7, avgReactionSec: 13 },
    { regionId: 'reg-11', regionName: 'Тараз', totalViolations: 92, phone: 30, gaze: 26, persons: 10, tabs: 16, audio: 10, criticalRate: 43.5, avgReactionSec: 15 },
    { regionId: 'reg-12', regionName: 'Петропавловск', totalViolations: 54, phone: 12, gaze: 18, persons: 6, tabs: 12, audio: 6, criticalRate: 33.3, avgReactionSec: 11 },
    { regionId: 'reg-13', regionName: 'Актау', totalViolations: 62, phone: 14, gaze: 20, persons: 8, tabs: 14, audio: 6, criticalRate: 35.5, avgReactionSec: 12 },
    { regionId: 'reg-14', regionName: 'Кызылорда', totalViolations: 78, phone: 26, gaze: 20, persons: 10, tabs: 14, audio: 8, criticalRate: 46.2, avgReactionSec: 16 },
    { regionId: 'reg-15', regionName: 'Туркестан', totalViolations: 156, phone: 48, gaze: 42, persons: 20, tabs: 30, audio: 16, criticalRate: 43.6, avgReactionSec: 18 },
    { regionId: 'reg-16', regionName: 'Кокшетау', totalViolations: 48, phone: 10, gaze: 16, persons: 6, tabs: 10, audio: 6, criticalRate: 33.3, avgReactionSec: 10 },
    { regionId: 'reg-17', regionName: 'Талдыкорган', totalViolations: 58, phone: 16, gaze: 18, persons: 8, tabs: 10, audio: 6, criticalRate: 41.4, avgReactionSec: 14 }
  ])

  const sortedRegionalViolations = computed(() =>
    [...regionalViolations.value].sort((a, b) => b.totalViolations - a.totalViolations)
  )

  // ============================================
  //  GLOBAL ANALYTICS DATA
  // ============================================
  const weeklyTrends = ref<WeeklyTrendPoint[]>([
    { day: 'Пн', phone: 48, gaze: 72, voice: 15, tabs: 38, persons: 18 },
    { day: 'Вт', phone: 52, gaze: 68, voice: 18, tabs: 42, persons: 22 },
    { day: 'Ср', phone: 61, gaze: 85, voice: 22, tabs: 51, persons: 28 },
    { day: 'Чт', phone: 45, gaze: 78, voice: 14, tabs: 35, persons: 16 },
    { day: 'Пт', phone: 72, gaze: 95, voice: 28, tabs: 58, persons: 34 },
    { day: 'Сб', phone: 28, gaze: 42, voice: 8, tabs: 22, persons: 10 },
    { day: 'Вс', phone: 12, gaze: 18, voice: 4, tabs: 10, persons: 5 }
  ])

  const monthlyTrends = ref<MonthlyTrendPoint[]>([
    { month: 'Сент', totalViolations: 1240, avgIntegrity: 84, totalSessions: 8420 },
    { month: 'Окт', totalViolations: 1580, avgIntegrity: 82, totalSessions: 10250 },
    { month: 'Нояб', totalViolations: 1890, avgIntegrity: 80, totalSessions: 12800 },
    { month: 'Дек', totalViolations: 2340, avgIntegrity: 78, totalSessions: 15600 },
    { month: 'Янв', totalViolations: 1680, avgIntegrity: 85, totalSessions: 9400 },
    { month: 'Фев', totalViolations: 1370, avgIntegrity: 88, totalSessions: 11200 }
  ])

  const subjectViolationRates = ref<SubjectViolationRate[]>([
    { subject: 'Математика', examCount: 24, avgViolationRate: 4.8, totalViolations: 342, topViolationType: 'Телефон' },
    { subject: 'Информатика', examCount: 18, avgViolationRate: 3.2, totalViolations: 218, topViolationType: 'Смена вкладки' },
    { subject: 'Химия', examCount: 15, avgViolationRate: 5.6, totalViolations: 289, topViolationType: 'Телефон' },
    { subject: 'Физика', examCount: 12, avgViolationRate: 3.8, totalViolations: 178, topViolationType: 'Взгляд' },
    { subject: 'Биология', examCount: 10, avgViolationRate: 2.4, totalViolations: 96, topViolationType: 'Взгляд' },
    { subject: 'Английский язык', examCount: 20, avgViolationRate: 1.8, totalViolations: 124, topViolationType: 'Аудио' },
    { subject: 'История', examCount: 16, avgViolationRate: 3.5, totalViolations: 198, topViolationType: 'Телефон' },
    { subject: 'Казахский язык', examCount: 14, avgViolationRate: 2.9, totalViolations: 145, topViolationType: 'Взгляд' },
    { subject: 'Экономика', examCount: 8, avgViolationRate: 2.1, totalViolations: 68, topViolationType: 'Смена вкладки' },
    { subject: 'Литература', examCount: 6, avgViolationRate: 0.9, totalViolations: 22, topViolationType: 'Взгляд' }
  ])

  const institutionRankings = ref<InstitutionRanking[]>([
    { id: 'inst-01', name: 'Назарбаев Университет', city: 'Астана', totalStudents: 4820, avgIntegrity: 94, violationRate: 1.2, totalExams: 86, trend: 'up' },
    { id: 'inst-02', name: 'КазНУ им. аль-Фараби', city: 'Алматы', totalStudents: 8450, avgIntegrity: 91, violationRate: 2.1, totalExams: 124, trend: 'stable' },
    { id: 'inst-03', name: 'КБТУ', city: 'Алматы', totalStudents: 3200, avgIntegrity: 90, violationRate: 2.4, totalExams: 52, trend: 'up' },
    { id: 'inst-04', name: 'ЕНУ им. Гумилёва', city: 'Астана', totalStudents: 6780, avgIntegrity: 89, violationRate: 2.8, totalExams: 98, trend: 'down' },
    { id: 'inst-05', name: 'МУИТ', city: 'Алматы', totalStudents: 2450, avgIntegrity: 88, violationRate: 3.0, totalExams: 44, trend: 'stable' },
    { id: 'inst-06', name: 'Satbayev University', city: 'Алматы', totalStudents: 5120, avgIntegrity: 87, violationRate: 3.2, totalExams: 76, trend: 'up' },
    { id: 'inst-07', name: 'КарУ им. Букетова', city: 'Караганда', totalStudents: 4200, avgIntegrity: 85, violationRate: 3.8, totalExams: 62, trend: 'down' },
    { id: 'inst-08', name: 'ЮКУ им. Ауэзова', city: 'Шымкент', totalStudents: 5800, avgIntegrity: 82, violationRate: 4.5, totalExams: 88, trend: 'down' },
    { id: 'inst-09', name: 'АУЭС', city: 'Алматы', totalStudents: 3100, avgIntegrity: 86, violationRate: 3.4, totalExams: 48, trend: 'stable' },
    { id: 'inst-10', name: 'НИШ (все филиалы)', city: 'Казахстан', totalStudents: 12400, avgIntegrity: 93, violationRate: 1.5, totalExams: 156, trend: 'up' },
    { id: 'inst-11', name: 'Колледж KBTU', city: 'Алматы', totalStudents: 1800, avgIntegrity: 80, violationRate: 5.2, totalExams: 32, trend: 'down' },
    { id: 'inst-12', name: 'ВКТУ им. Серикбаева', city: 'Усть-Каменогорск', totalStudents: 3400, avgIntegrity: 79, violationRate: 5.8, totalExams: 54, trend: 'down' }
  ])

  const globalAiConfidence = ref(94.3)
  const activeProctors = ref(6)
  const avgProctorSpeed = ref(10.8)

  // ============================================
  //  API & INTEGRATIONS DATA
  // ============================================
  const apiKeys = ref<ApiKey[]>([
    { id: 'ak-001', name: 'Eduser Production', key: 'argus_live_sk_7f8a9b2c3d4e5f6g7h8i9j0k', created: '2025-11-15', lastUsed: '2026-02-11T10:42:00Z', status: 'active', permissions: ['read:sessions', 'write:violations', 'read:reports'] },
    { id: 'ak-002', name: 'Staging Environment', key: 'argus_test_sk_1a2b3c4d5e6f7g8h9i0j1k2l', created: '2025-12-01', lastUsed: '2026-02-10T15:30:00Z', status: 'active', permissions: ['read:sessions', 'read:reports'] },
    { id: 'ak-003', name: 'Legacy Integration', key: 'argus_live_sk_9z8y7x6w5v4u3t2s1r0q9p8o', created: '2025-08-20', lastUsed: '2025-12-15T08:00:00Z', status: 'revoked', permissions: ['read:sessions'] },
    { id: 'ak-004', name: 'Moodle Connector', key: 'argus_live_sk_3m4n5o6p7q8r9s0t1u2v3w4x', created: '2026-01-10', lastUsed: '2026-02-11T09:15:00Z', status: 'active', permissions: ['read:sessions', 'write:violations', 'read:reports', 'write:webhooks'] }
  ])

  const webhooks = ref<Webhook[]>([
    { id: 'wh-001', url: 'https://eduser.kz/api/webhooks/argus', events: ['violation.detected', 'session.ended', 'integrity.finalized'], status: 'active', lastDelivery: '2026-02-11T10:41:55Z', successRate: 99.2 },
    { id: 'wh-002', url: 'https://lms.university.kz/hooks/proctoring', events: ['session.ended', 'integrity.finalized'], status: 'active', lastDelivery: '2026-02-11T10:38:20Z', successRate: 97.8 },
    { id: 'wh-003', url: 'https://admin.testcenter.kz/notifications', events: ['violation.detected'], status: 'paused', lastDelivery: '2026-02-09T16:22:00Z', successRate: 85.4 }
  ])

  const webhookDeliveryLogs = ref<WebhookDeliveryLog[]>([
    { id: 'dl-001', webhookId: 'wh-001', event: 'violation.detected', status: 'success', timestamp: '2026-02-11T10:41:55Z', responseCode: 200, duration: 124 },
    { id: 'dl-002', webhookId: 'wh-001', event: 'session.ended', status: 'success', timestamp: '2026-02-11T10:40:12Z', responseCode: 200, duration: 89 },
    { id: 'dl-003', webhookId: 'wh-002', event: 'integrity.finalized', status: 'success', timestamp: '2026-02-11T10:38:20Z', responseCode: 200, duration: 156 },
    { id: 'dl-004', webhookId: 'wh-001', event: 'violation.detected', status: 'success', timestamp: '2026-02-11T10:35:00Z', responseCode: 200, duration: 102 },
    { id: 'dl-005', webhookId: 'wh-002', event: 'session.ended', status: 'failed', timestamp: '2026-02-11T10:30:45Z', responseCode: 503, duration: 5012 },
    { id: 'dl-006', webhookId: 'wh-001', event: 'integrity.finalized', status: 'success', timestamp: '2026-02-11T10:28:00Z', responseCode: 200, duration: 78 },
    { id: 'dl-007', webhookId: 'wh-003', event: 'violation.detected', status: 'failed', timestamp: '2026-02-09T16:22:00Z', responseCode: 500, duration: 3045 },
    { id: 'dl-008', webhookId: 'wh-001', event: 'session.ended', status: 'success', timestamp: '2026-02-11T10:20:00Z', responseCode: 200, duration: 95 },
    { id: 'dl-009', webhookId: 'wh-002', event: 'violation.detected', status: 'success', timestamp: '2026-02-11T10:15:30Z', responseCode: 200, duration: 134 },
    { id: 'dl-010', webhookId: 'wh-001', event: 'violation.detected', status: 'success', timestamp: '2026-02-11T10:10:00Z', responseCode: 200, duration: 112 }
  ])

  const apiSystemStatus = ref<'operational' | 'degraded' | 'down'>('operational')

  // ============================================
  //  EXAM PROCTORING SETTINGS (EDUSER SYNC)
  // ============================================
  const eduserSyncing = ref(false)
  const eduserLastSync = ref('2026-02-11T08:30:00Z')

  const examConfigs = ref<ExamProctoringConfig[]>([
    {
      id: 'ec-001', orgId: 'org-eduser', examName: 'Высшая математика — Финал', examCode: 'MATH-401-F', eduserId: 'EDU-2026-0847',
      date: '2026-02-15', participants: 86, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'active', violationCount: 12,
      settings: { requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: true, objectDetectionPhone: true, objectDetectionPerson: true, gazeTracking: true, gazeSensitivity: 70, gazeDeviationLimitSec: 5, voiceDetectionThreshold: 65, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true, emotionStressAnalysis: true, focusLossScore: true, blinkPatternAnalysis: true, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: true, tabSwitchingLimit: 3, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: true, blockRemoteAccess: true, blockContextMenu: true, vpnProxyDetection: true, localNetworkScan: true, typingDynamics: true, handCursorSync: true, processScanning: true, hardwareDeviceDetection: true, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true, forceLowSpecMode: false },
      exceptions: [
        { id: 'se-001', studentName: 'Айдана Нурмагамбетова', studentId: 'STU-2026-4421', reason: 'Шейный остеохондроз — необходимо часто поворачивать голову', profile: 'low_gaze', profileLabel: 'Низкая чувствительность взгляда', overrides: { gazeSensitivity: 20, gazeTracking: true } },
        { id: 'se-002', studentName: 'Марат Касымов', studentId: 'STU-2026-4438', reason: 'Перелом руки — не может установить боковую камеру', profile: 'no_side_camera', profileLabel: 'Без боковой камеры', overrides: { requireSideCamera: false } },
        { id: 'se-005', studentName: 'Мария Сантос', studentId: '020819500632', reason: 'Тревожное расстройство — пониженная чувствительность взгляда', profile: 'low_gaze', profileLabel: 'Низкая чувствительность взгляда', overrides: { gazeSensitivity: 25, gazeDeviationLimitSec: 20 } },
        { id: 'se-006', studentName: 'Айгерим Тулебаева', studentId: '010814500396', reason: 'Астигматизм — требуются частые движения глаз', profile: 'low_gaze', profileLabel: 'Низкая чувствительность взгляда', overrides: { gazeSensitivity: 15, gazeTracking: true } }
      ]
    },
    {
      id: 'ec-002', orgId: 'org-eduser', examName: 'Информатика 101 — Промежуточный', examCode: 'CS-101-M', eduserId: 'EDU-2026-0848',
      date: '2026-02-16', participants: 124, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'active', violationCount: 7,
      settings: { requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: true, gazeTracking: true, gazeSensitivity: 60, gazeDeviationLimitSec: 8, voiceDetectionThreshold: 50, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true, emotionStressAnalysis: false, focusLossScore: true, blinkPatternAnalysis: false, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: false, tabSwitchingLimit: 2, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: false, blockRemoteAccess: true, blockContextMenu: true, vpnProxyDetection: true, localNetworkScan: false, typingDynamics: false, handCursorSync: false, processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false },
      exceptions: [
        { id: 'se-007', studentName: 'Лина Мюллер', studentId: '030204700143', reason: 'Нарушение слуха — высокий уровень фонового шума в среде', profile: 'high_noise_tolerance', profileLabel: 'Высокая толерантность к шуму', overrides: { voiceDetectionThreshold: 10 } }
      ]
    },
    {
      id: 'ec-003', orgId: 'org-nis', examName: 'Физика — Лабораторная работа', examCode: 'PHYS-201-L', eduserId: 'EDU-2026-0849',
      date: '2026-02-17', participants: 45, isArgusProtected: false, syncedAt: '2026-02-11T08:30:00Z',
      status: 'scheduled', violationCount: 0,
      settings: { requireSideCamera: false, faceVerification: true, dynamicFaceRecheck: false, antiSpoofing: false, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: false, gazeTracking: false, gazeSensitivity: 50, gazeDeviationLimitSec: 15, voiceDetectionThreshold: 40, voiceActivityDetection: false, audioPeripheryDetection: false, smartNoiseFilter: false, emotionStressAnalysis: false, focusLossScore: false, blinkPatternAnalysis: false, forceFullscreen: false, fullscreenExitDetection: false, webDisplayMonitoring: false, tabSwitchingLimit: 5, blockCopyPaste: false, blockPrintScreen: false, blockVirtualMachine: false, blockMultiDesktop: false, blockRemoteAccess: false, blockContextMenu: false, vpnProxyDetection: false, localNetworkScan: false, typingDynamics: false, handCursorSync: false, processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false },
      exceptions: []
    },
    {
      id: 'ec-004', orgId: 'org-nis', examName: 'Английский язык B2 — Аудирование', examCode: 'ENG-B2-AUD', eduserId: 'EDU-2026-0850',
      date: '2026-02-18', participants: 210, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'active', violationCount: 23,
      settings: { requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: true, objectDetectionPhone: true, objectDetectionPerson: true, gazeTracking: true, gazeSensitivity: 50, gazeDeviationLimitSec: 10, voiceDetectionThreshold: 30, voiceActivityDetection: false, audioPeripheryDetection: false, smartNoiseFilter: false, emotionStressAnalysis: true, focusLossScore: true, blinkPatternAnalysis: true, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: true, tabSwitchingLimit: 1, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: true, blockRemoteAccess: true, blockContextMenu: true, vpnProxyDetection: false, localNetworkScan: false, typingDynamics: false, handCursorSync: false, processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false },
      exceptions: [
        { id: 'se-003', studentName: 'Ерлан Тулегенов', studentId: 'STU-2026-5102', reason: 'Нарушение слуха — высокий порог голосовой детекции может ложно срабатывать', profile: 'high_noise_tolerance', profileLabel: 'Высокая толерантность к шуму', overrides: { voiceDetectionThreshold: 15 } }
      ]
    },
    {
      id: 'ec-005', orgId: 'org-eduser', examName: 'Структуры данных — Практика', examCode: 'CS-210-P', eduserId: 'EDU-2026-0851',
      date: '2026-02-19', participants: 67, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'scheduled', violationCount: 0,
      settings: { requireSideCamera: false, faceVerification: true, dynamicFaceRecheck: false, antiSpoofing: true, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: false, gazeTracking: true, gazeSensitivity: 60, gazeDeviationLimitSec: 8, voiceDetectionThreshold: 55, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true, emotionStressAnalysis: false, focusLossScore: true, blinkPatternAnalysis: false, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: false, tabSwitchingLimit: 4, blockCopyPaste: false, blockPrintScreen: false, blockVirtualMachine: true, blockMultiDesktop: false, blockRemoteAccess: false, blockContextMenu: false, vpnProxyDetection: true, localNetworkScan: true, typingDynamics: true, handCursorSync: true, processScanning: true, hardwareDeviceDetection: true, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true, forceLowSpecMode: false },
      exceptions: []
    },
    {
      id: 'ec-006', orgId: 'org-nis', examName: 'История Казахстана — ЕНТ', examCode: 'HIST-KZ-ENT', eduserId: 'EDU-2026-0852',
      date: '2026-02-20', participants: 1850, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'scheduled', violationCount: 0,
      settings: { requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: true, objectDetectionPhone: true, objectDetectionPerson: true, gazeTracking: true, gazeSensitivity: 80, gazeDeviationLimitSec: 3, voiceDetectionThreshold: 70, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true, emotionStressAnalysis: true, focusLossScore: true, blinkPatternAnalysis: true, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: true, tabSwitchingLimit: 0, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: true, blockRemoteAccess: true, blockContextMenu: true, vpnProxyDetection: true, localNetworkScan: true, typingDynamics: true, handCursorSync: true, processScanning: true, hardwareDeviceDetection: true, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true, forceLowSpecMode: false },
      exceptions: []
    },
    {
      id: 'ec-007', orgId: 'org-kaznu', examName: 'Биология — Тестирование', examCode: 'BIO-101-T', eduserId: 'EDU-2026-0853',
      date: '2026-02-21', participants: 93, isArgusProtected: false, syncedAt: '2026-02-11T08:30:00Z',
      status: 'completed', violationCount: 3,
      settings: { requireSideCamera: false, faceVerification: false, dynamicFaceRecheck: false, antiSpoofing: false, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: false, gazeTracking: false, gazeSensitivity: 50, gazeDeviationLimitSec: 15, voiceDetectionThreshold: 40, voiceActivityDetection: false, audioPeripheryDetection: false, smartNoiseFilter: false, emotionStressAnalysis: false, focusLossScore: false, blinkPatternAnalysis: false, forceFullscreen: false, fullscreenExitDetection: false, webDisplayMonitoring: false, tabSwitchingLimit: 10, blockCopyPaste: false, blockPrintScreen: false, blockVirtualMachine: false, blockMultiDesktop: false, blockRemoteAccess: false, blockContextMenu: false, vpnProxyDetection: false, localNetworkScan: false, typingDynamics: false, handCursorSync: false, processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false },
      exceptions: []
    },
    {
      id: 'ec-008', orgId: 'org-kaznu', examName: 'Казахский язык — Грамматика', examCode: 'KAZ-301-G', eduserId: 'EDU-2026-0854',
      date: '2026-02-13', participants: 156, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'completed', violationCount: 18,
      settings: { requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: true, gazeTracking: true, gazeSensitivity: 65, gazeDeviationLimitSec: 6, voiceDetectionThreshold: 60, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true, emotionStressAnalysis: false, focusLossScore: false, blinkPatternAnalysis: false, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: false, tabSwitchingLimit: 2, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: true, blockRemoteAccess: true, blockContextMenu: true, vpnProxyDetection: false, localNetworkScan: false, typingDynamics: false, handCursorSync: false, processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false },
      exceptions: []
    },
    {
      id: 'ec-009', orgId: 'org-kaznu', examName: 'Химия — Органика', examCode: 'CHEM-202-O', eduserId: 'EDU-2026-0855',
      date: '2026-02-12', participants: 78, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'completed', violationCount: 5,
      settings: { requireSideCamera: false, faceVerification: true, dynamicFaceRecheck: false, antiSpoofing: true, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: false, gazeTracking: true, gazeSensitivity: 55, gazeDeviationLimitSec: 10, voiceDetectionThreshold: 45, voiceActivityDetection: false, audioPeripheryDetection: false, smartNoiseFilter: false, emotionStressAnalysis: false, focusLossScore: false, blinkPatternAnalysis: false, forceFullscreen: false, fullscreenExitDetection: false, webDisplayMonitoring: false, tabSwitchingLimit: 3, blockCopyPaste: false, blockPrintScreen: false, blockVirtualMachine: true, blockMultiDesktop: false, blockRemoteAccess: false, blockContextMenu: false, vpnProxyDetection: false, localNetworkScan: false, typingDynamics: false, handCursorSync: false, processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false },
      exceptions: []
    },
    {
      id: 'ec-010', orgId: 'org-eduser', examName: 'Экономика — Макроэкономика', examCode: 'ECON-401-M', eduserId: 'EDU-2026-0856',
      date: '2026-02-22', participants: 112, isArgusProtected: true, syncedAt: '2026-02-11T08:30:00Z',
      status: 'active', violationCount: 4,
      settings: { requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: false, objectDetectionPhone: true, objectDetectionPerson: true, gazeTracking: true, gazeSensitivity: 60, gazeDeviationLimitSec: 7, voiceDetectionThreshold: 55, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true, emotionStressAnalysis: true, focusLossScore: true, blinkPatternAnalysis: false, forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: true, tabSwitchingLimit: 3, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: false, blockRemoteAccess: true, blockContextMenu: true, vpnProxyDetection: true, localNetworkScan: true, typingDynamics: true, handCursorSync: true, processScanning: true, hardwareDeviceDetection: true, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true, forceLowSpecMode: false },
      exceptions: [
        { id: 'se-004', studentName: 'Динара Жумабаева', studentId: 'STU-2026-6205', reason: 'Визуальные нарушения — требуется большой экран, частый взгляд в сторону', profile: 'low_gaze', profileLabel: 'Низкая чувствительность взгляда', overrides: { gazeSensitivity: 25, gazeTracking: true } }
      ]
    }
  ])

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
  const monitoringSessions = ref<MonitoringSession[]>([
    {
      id: 'mon-001', orgId: 'org-nis', studentName: 'Алексей Чен', iin: '010315500421', phone: '+7 701 234 5601', examId: 'exam-003', examName: 'Органическая химия — Тест 4',
      integrityScore: 34, violationCount: 7, violationLevel: 'critical',
      aiStatus: { gazeTracking: 'critical', faceIdMatch: 'verified', objectDetection: 'phone' },
      isOnline: true, startedAt: '2026-02-11T09:05:00Z', lastActivity: '2026-02-11T11:42:15Z',
      events: [
        { id: 'ev-001', timestamp: '2026-02-11T11:42:15Z', type: 'phone_detected', label: 'Телефон обнаружен на столе', severity: 'critical' },
        { id: 'ev-002', timestamp: '2026-02-11T11:38:00Z', type: 'gaze_deviation', label: 'Взгляд вниз >15 сек', severity: 'warning' },
        { id: 'ev-003', timestamp: '2026-02-11T11:30:12Z', type: 'phone_detected', label: 'Телефон обнаружен в руке', severity: 'critical' },
        { id: 'ev-004', timestamp: '2026-02-11T11:22:00Z', type: 'tab_switch', label: 'Переключение вкладки (3 раза)', severity: 'warning' },
        { id: 'ev-005', timestamp: '2026-02-11T10:58:00Z', type: 'gaze_deviation', label: 'Взгляд в сторону >10 сек', severity: 'warning' }
      ]
    },
    {
      id: 'mon-002', orgId: 'org-eduser', studentName: 'Мария Сантос', iin: '020819500632', phone: '+7 702 345 6702', examId: 'exam-001', examName: 'Высшая математика — Финал',
      integrityScore: 78, violationCount: 2, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'warning', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T09:00:00Z', lastActivity: '2026-02-11T11:41:58Z',
      events: [
        { id: 'ev-006', timestamp: '2026-02-11T11:41:58Z', type: 'gaze_deviation', label: 'Отклонение взгляда вправо', severity: 'warning' },
        { id: 'ev-007', timestamp: '2026-02-11T10:20:00Z', type: 'audio_anomaly', label: 'Фоновые голоса обнаружены', severity: 'info' }
      ]
    },
    {
      id: 'mon-003', orgId: 'org-eduser', studentName: 'Джеймс Уилсон', iin: '020728600312', phone: '+7 705 456 7803', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный',
      integrityScore: 41, violationCount: 5, violationLevel: 'critical',
      aiStatus: { gazeTracking: 'warning', faceIdMatch: 'mismatch', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T10:30:00Z', lastActivity: '2026-02-11T11:41:30Z',
      events: [
        { id: 'ev-008', timestamp: '2026-02-11T11:41:30Z', type: 'multiple_persons', label: '2 лица в кадре', severity: 'critical' },
        { id: 'ev-009', timestamp: '2026-02-11T11:35:00Z', type: 'face_mismatch', label: 'Лицо не совпадает с ID', severity: 'critical' },
        { id: 'ev-010', timestamp: '2026-02-11T11:20:00Z', type: 'gaze_deviation', label: 'Взгляд вниз >20 сек', severity: 'warning' }
      ]
    },
    {
      id: 'mon-004', orgId: 'org-eduser', studentName: 'Сара Ким', iin: '011130500278', phone: '+7 707 567 8904', examId: 'exam-005', examName: 'Структуры данных — Практика',
      integrityScore: 63, violationCount: 3, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'book' },
      isOnline: true, startedAt: '2026-02-11T13:00:00Z', lastActivity: '2026-02-11T13:40:45Z',
      events: [
        { id: 'ev-011', timestamp: '2026-02-11T13:40:45Z', type: 'book_detected', label: 'Книга/тетрадь обнаружена', severity: 'warning' },
        { id: 'ev-012', timestamp: '2026-02-11T13:25:00Z', type: 'tab_switch', label: 'Переключение вкладки', severity: 'warning' }
      ]
    },
    {
      id: 'mon-005', orgId: 'org-nis', studentName: 'Давид Окафор', iin: '990916400589', phone: '+7 700 678 9005', examId: 'exam-003', examName: 'Органическая химия — Тест 4',
      integrityScore: 52, violationCount: 4, violationLevel: 'critical',
      aiStatus: { gazeTracking: 'critical', faceIdMatch: 'verified', objectDetection: 'phone' },
      isOnline: true, startedAt: '2026-02-11T11:00:00Z', lastActivity: '2026-02-11T11:40:12Z',
      events: [
        { id: 'ev-013', timestamp: '2026-02-11T11:40:12Z', type: 'phone_detected', label: 'Телефон обнаружен', severity: 'critical' },
        { id: 'ev-014', timestamp: '2026-02-11T11:28:00Z', type: 'gaze_deviation', label: 'Продолжительное отклонение взгляда', severity: 'critical' }
      ]
    },
    {
      id: 'mon-006', orgId: 'org-eduser', studentName: 'Эмили Чжан', iin: '031005500847', phone: '+7 708 789 0106', examId: 'exam-001', examName: 'Высшая математика — Финал',
      integrityScore: 81, violationCount: 1, violationLevel: 'clean',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T09:00:00Z', lastActivity: '2026-02-11T11:39:50Z',
      events: [
        { id: 'ev-015', timestamp: '2026-02-11T10:45:00Z', type: 'gaze_deviation', label: 'Кратковременное отклонение взгляда', severity: 'info' }
      ]
    },
    {
      id: 'mon-007', orgId: 'org-nis', studentName: 'Радж Патель', iin: '000412600923', phone: '+7 771 890 1207', examId: 'exam-004', examName: 'Английская литература — Эссе',
      integrityScore: 88, violationCount: 1, violationLevel: 'clean',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T08:00:00Z', lastActivity: '2026-02-11T11:38:30Z',
      events: [
        { id: 'ev-016', timestamp: '2026-02-11T11:38:30Z', type: 'audio_anomaly', label: 'Шум клавиатуры усилен', severity: 'info' }
      ]
    },
    {
      id: 'mon-008', orgId: 'org-eduser', studentName: 'Лина Мюллер', iin: '030204700143', phone: '+7 775 901 2308', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный',
      integrityScore: 58, violationCount: 3, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'warning', faceIdMatch: 'verified', objectDetection: 'earbuds' },
      isOnline: true, startedAt: '2026-02-11T10:30:00Z', lastActivity: '2026-02-11T11:37:15Z',
      events: [
        { id: 'ev-017', timestamp: '2026-02-11T11:37:15Z', type: 'earbuds_detected', label: 'Наушники обнаружены', severity: 'warning' },
        { id: 'ev-018', timestamp: '2026-02-11T11:15:00Z', type: 'multiple_persons', label: 'Второе лицо в кадре', severity: 'critical' }
      ]
    },
    {
      id: 'mon-009', orgId: 'org-eduser', studentName: 'Том Бейкер', iin: '010622500164', phone: '+7 778 012 3409', examId: 'exam-005', examName: 'Структуры данных — Практика',
      integrityScore: 72, violationCount: 2, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T13:00:00Z', lastActivity: '2026-02-11T13:36:42Z',
      events: [
        { id: 'ev-019', timestamp: '2026-02-11T13:36:42Z', type: 'tab_switch', label: 'Переключение вкладки', severity: 'warning' },
        { id: 'ev-020', timestamp: '2026-02-11T13:18:00Z', type: 'tab_switch', label: 'Переключение вкладки', severity: 'warning' }
      ]
    },
    {
      id: 'mon-010', orgId: 'org-nis', studentName: 'Айко Танака', iin: '020917500735', phone: '+7 701 123 4510', examId: 'exam-003', examName: 'Органическая химия — Тест 4',
      integrityScore: 45, violationCount: 6, violationLevel: 'critical',
      aiStatus: { gazeTracking: 'critical', faceIdMatch: 'verified', objectDetection: 'phone' },
      isOnline: true, startedAt: '2026-02-11T11:00:00Z', lastActivity: '2026-02-11T11:35:20Z',
      events: [
        { id: 'ev-021', timestamp: '2026-02-11T11:35:20Z', type: 'phone_detected', label: 'Телефон обнаружен повторно', severity: 'critical' },
        { id: 'ev-022', timestamp: '2026-02-11T11:25:00Z', type: 'gaze_deviation', label: 'Отклонение взгляда влево', severity: 'warning' },
        { id: 'ev-023', timestamp: '2026-02-11T11:10:00Z', type: 'phone_detected', label: 'Телефон обнаружен', severity: 'critical' }
      ]
    },
    {
      id: 'mon-011', orgId: 'org-eduser', studentName: 'Нурлан Касымов', iin: '990403500128', phone: '+7 702 234 5611', examId: 'exam-001', examName: 'Высшая математика — Финал',
      integrityScore: 92, violationCount: 0, violationLevel: 'clean',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T09:00:00Z', lastActivity: '2026-02-11T11:42:00Z',
      events: []
    },
    {
      id: 'mon-012', orgId: 'org-eduser', studentName: 'Айгерим Тулебаева', iin: '010814500396', phone: '+7 705 345 6712', examId: 'exam-001', examName: 'Высшая математика — Финал',
      integrityScore: 85, violationCount: 1, violationLevel: 'clean',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T09:00:00Z', lastActivity: '2026-02-11T11:40:30Z',
      events: [
        { id: 'ev-024', timestamp: '2026-02-11T10:30:00Z', type: 'gaze_deviation', label: 'Кратковременное отклонение', severity: 'info' }
      ]
    },
    {
      id: 'mon-013', orgId: 'org-eduser', studentName: 'Бекзат Серикбаев', iin: '000127600542', phone: '+7 707 456 7813', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный',
      integrityScore: 76, violationCount: 2, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'warning', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T10:30:00Z', lastActivity: '2026-02-11T11:39:00Z',
      events: [
        { id: 'ev-025', timestamp: '2026-02-11T11:39:00Z', type: 'gaze_deviation', label: 'Отклонение взгляда вправо', severity: 'warning' },
        { id: 'ev-026', timestamp: '2026-02-11T11:05:00Z', type: 'tab_switch', label: 'Переключение вкладки', severity: 'warning' }
      ]
    },
    {
      id: 'mon-014', orgId: 'org-eduser', studentName: 'Динара Жумабекова', iin: '021103500418', phone: '+7 700 567 8914', examId: 'exam-002', examName: 'Информатика 101 — Промежуточный',
      integrityScore: 69, violationCount: 3, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'book' },
      isOnline: true, startedAt: '2026-02-11T10:30:00Z', lastActivity: '2026-02-11T11:36:00Z',
      events: [
        { id: 'ev-027', timestamp: '2026-02-11T11:36:00Z', type: 'book_detected', label: 'Конспект обнаружен', severity: 'warning' },
        { id: 'ev-028', timestamp: '2026-02-11T11:20:00Z', type: 'gaze_deviation', label: 'Отклонение взгляда вниз', severity: 'warning' }
      ]
    },
    {
      id: 'mon-015', orgId: 'org-nis', studentName: 'Ерлан Абдрахманов', iin: '980615500873', phone: '+7 708 678 9015', examId: 'exam-003', examName: 'Органическая химия — Тест 4',
      integrityScore: 55, violationCount: 4, violationLevel: 'critical',
      aiStatus: { gazeTracking: 'critical', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T11:00:00Z', lastActivity: '2026-02-11T11:34:00Z',
      events: [
        { id: 'ev-029', timestamp: '2026-02-11T11:34:00Z', type: 'gaze_deviation', label: 'Постоянное отклонение взгляда', severity: 'critical' },
        { id: 'ev-030', timestamp: '2026-02-11T11:18:00Z', type: 'audio_anomaly', label: 'Шёпот обнаружен', severity: 'warning' }
      ]
    },
    {
      id: 'mon-016', orgId: 'org-nis', studentName: 'Жанар Муратова', iin: '030529700261', phone: '+7 771 789 0116', examId: 'exam-004', examName: 'Английская литература — Эссе',
      integrityScore: 91, violationCount: 0, violationLevel: 'clean',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T08:00:00Z', lastActivity: '2026-02-11T11:42:10Z',
      events: []
    },
    {
      id: 'mon-017', orgId: 'org-nis', studentName: 'Канат Нурмагамбетов', iin: '010207500547', phone: '+7 775 890 1217', examId: 'exam-004', examName: 'Английская литература — Эссе',
      integrityScore: 83, violationCount: 1, violationLevel: 'clean',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T08:00:00Z', lastActivity: '2026-02-11T11:41:00Z',
      events: [
        { id: 'ev-031', timestamp: '2026-02-11T09:45:00Z', type: 'audio_anomaly', label: 'Фоновый шум', severity: 'info' }
      ]
    },
    {
      id: 'mon-018', orgId: 'org-eduser', studentName: 'Мадина Оспанова', iin: '020416500689', phone: '+7 778 901 2318', examId: 'exam-005', examName: 'Структуры данных — Практика',
      integrityScore: 74, violationCount: 2, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'warning', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T13:00:00Z', lastActivity: '2026-02-11T13:38:00Z',
      events: [
        { id: 'ev-032', timestamp: '2026-02-11T13:38:00Z', type: 'gaze_deviation', label: 'Отклонение взгляда', severity: 'warning' },
        { id: 'ev-033', timestamp: '2026-02-11T13:15:00Z', type: 'tab_switch', label: 'Переключение вкладки', severity: 'warning' }
      ]
    },
    {
      id: 'mon-019', orgId: 'org-eduser', studentName: 'Руслан Байжанов', iin: '991228600327', phone: '+7 701 012 3419', examId: 'exam-005', examName: 'Структуры данных — Практика',
      integrityScore: 67, violationCount: 3, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'normal', faceIdMatch: 'verified', objectDetection: 'earbuds' },
      isOnline: true, startedAt: '2026-02-11T13:00:00Z', lastActivity: '2026-02-11T13:35:00Z',
      events: [
        { id: 'ev-034', timestamp: '2026-02-11T13:35:00Z', type: 'earbuds_detected', label: 'Беспроводные наушники', severity: 'warning' },
        { id: 'ev-035', timestamp: '2026-02-11T13:22:00Z', type: 'tab_switch', label: 'Переключение вкладки', severity: 'warning' }
      ]
    },
    {
      id: 'mon-020', orgId: 'org-eduser', studentName: 'Салтанат Ережепова', iin: '000831500412', phone: '+7 702 123 4520', examId: 'exam-001', examName: 'Высшая математика — Финал',
      integrityScore: 79, violationCount: 2, violationLevel: 'warning',
      aiStatus: { gazeTracking: 'warning', faceIdMatch: 'verified', objectDetection: 'clear' },
      isOnline: true, startedAt: '2026-02-11T09:00:00Z', lastActivity: '2026-02-11T11:40:00Z',
      events: [
        { id: 'ev-036', timestamp: '2026-02-11T11:40:00Z', type: 'gaze_deviation', label: 'Отклонение взгляда вниз', severity: 'warning' },
        { id: 'ev-037', timestamp: '2026-02-11T10:50:00Z', type: 'gaze_deviation', label: 'Отклонение взгляда влево', severity: 'warning' }
      ]
    }
  ])

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
        s.studentName.toLowerCase().includes(q) ||
        s.iin.includes(q) ||
        s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, ''))
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
        s.studentName.toLowerCase().includes(q) ||
        s.iin.includes(q) ||
        s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, ''))
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
  const tests = ref<TestItem[]>([
    { id: 'test-001', name: 'Математика: Линейная алгебра', subject: 'Математика', questionsCount: 40, duration: 90, createdAt: '2025-09-12T10:00:00Z', updatedAt: '2025-11-20T14:30:00Z', status: 'active', author: 'Ахметов Б.К.', difficulty: 'hard', passScore: 60, attemptsAllowed: 2 },
    { id: 'test-002', name: 'Физика: Механика', subject: 'Физика', questionsCount: 30, duration: 60, createdAt: '2025-08-05T09:00:00Z', updatedAt: '2025-11-18T11:00:00Z', status: 'active', author: 'Сериков Д.А.', difficulty: 'medium', passScore: 50, attemptsAllowed: 3 },
    { id: 'test-003', name: 'Информатика: Алгоритмы и структуры данных', subject: 'Информатика', questionsCount: 50, duration: 120, createdAt: '2025-07-22T08:00:00Z', updatedAt: '2025-11-15T16:45:00Z', status: 'active', author: 'Касымова А.Н.', difficulty: 'hard', passScore: 65, attemptsAllowed: 1 },
    { id: 'test-004', name: 'История Казахстана: XX век', subject: 'История', questionsCount: 35, duration: 45, createdAt: '2025-10-01T12:00:00Z', updatedAt: '2025-11-10T09:20:00Z', status: 'active', author: 'Жумабаев Е.Т.', difficulty: 'easy', passScore: 55, attemptsAllowed: 3 },
    { id: 'test-005', name: 'Английский язык: Grammar & Vocabulary', subject: 'Английский язык', questionsCount: 60, duration: 75, createdAt: '2025-06-15T14:00:00Z', updatedAt: '2025-11-22T10:10:00Z', status: 'active', author: 'Нурланова С.М.', difficulty: 'medium', passScore: 70, attemptsAllowed: 2 },
    { id: 'test-006', name: 'Химия: Органическая химия', subject: 'Химия', questionsCount: 25, duration: 50, createdAt: '2025-09-28T11:00:00Z', updatedAt: '2025-10-30T13:00:00Z', status: 'draft', author: 'Байжанов К.Р.', difficulty: 'hard', passScore: 60, attemptsAllowed: 2 },
    { id: 'test-007', name: 'Биология: Генетика', subject: 'Биология', questionsCount: 45, duration: 60, createdAt: '2025-08-18T10:30:00Z', updatedAt: '2025-11-05T15:00:00Z', status: 'active', author: 'Мухтарова Г.О.', difficulty: 'medium', passScore: 55, attemptsAllowed: 2 },
    { id: 'test-008', name: 'Казахский язык: Грамматика', subject: 'Казахский язык', questionsCount: 50, duration: 60, createdAt: '2025-07-10T09:00:00Z', updatedAt: '2025-10-25T12:00:00Z', status: 'archived', author: 'Оспанова Л.Б.', difficulty: 'easy', passScore: 50, attemptsAllowed: 5 },
    { id: 'test-009', name: 'Экономика: Микроэкономика', subject: 'Экономика', questionsCount: 30, duration: 55, createdAt: '2025-10-15T08:00:00Z', updatedAt: '2025-11-21T17:00:00Z', status: 'active', author: 'Токаев Н.С.', difficulty: 'medium', passScore: 60, attemptsAllowed: 2 },
    { id: 'test-010', name: 'Философия: Введение', subject: 'Философия', questionsCount: 20, duration: 30, createdAt: '2025-11-01T10:00:00Z', updatedAt: '2025-11-19T11:30:00Z', status: 'draft', author: 'Абдраимов Р.К.', difficulty: 'easy', passScore: 45, attemptsAllowed: 3 }
  ])

  const complexTests = ref<ComplexTestItem[]>([
    { id: 'ctest-001', name: 'ЕНТ — Блок МГН (Мат-Грамотность)', subjects: ['Математика', 'Логика', 'Грамотность'], testsCount: 3, totalQuestions: 120, totalDuration: 180, createdAt: '2025-09-01T08:00:00Z', status: 'active', author: 'Ахметов Б.К.' },
    { id: 'ctest-002', name: 'Комплексный IT-экзамен', subjects: ['Информатика', 'Математика', 'Английский язык'], testsCount: 3, totalQuestions: 140, totalDuration: 240, createdAt: '2025-10-10T10:00:00Z', status: 'active', author: 'Касымова А.Н.' },
    { id: 'ctest-003', name: 'Естественные науки (Блок)', subjects: ['Физика', 'Химия', 'Биология'], testsCount: 3, totalQuestions: 100, totalDuration: 150, createdAt: '2025-08-20T09:00:00Z', status: 'draft', author: 'Сериков Д.А.' }
  ])

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
    'org-kaznu': 'КазНУ',
  }

  function getOrgName(orgId: string): string {
    return orgNameMap[orgId] || orgId
  }

  // Org-filtered active exams
  const orgFilteredActiveExams = computed(() => {
    const authStore = useAuthStore()
    const orgId = authStore.effectiveOrgId
    if (!orgId) return activeExams.value  // null = all orgs (Super Admin)
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
