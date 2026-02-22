<script setup lang="ts">
import { useDashboardStore, type ExamProctoringConfig, type StudentException, type ExamProctoringSettings } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'

const store = useDashboardStore()
const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg, purpleBg } = useColors()
const { formatDate, formatDateTime } = useFormatters()

// --- Search ---
const searchQuery = ref('')
const searchFocused = ref(false)

// Org-aware exam configs
const orgAwareExamConfigs = computed(() => {
  return store.orgFilteredExamConfigs
})

// --- Filtered exams ---
const filteredExams = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return orgAwareExamConfigs.value
  return orgAwareExamConfigs.value.filter(e =>
    e.examName.toLowerCase().includes(q) ||
    e.examCode.toLowerCase().includes(q) ||
    e.eduserId.toLowerCase().includes(q)
  )
})

// --- Stats ---
const totalExams = computed(() => orgAwareExamConfigs.value.length)
const activeExams = computed(() => orgAwareExamConfigs.value.filter(e => e.status === 'active').length)
const completedExams = computed(() => orgAwareExamConfigs.value.filter(e => e.status === 'completed').length)
const withViolations = computed(() => orgAwareExamConfigs.value.filter(e => e.violationCount > 0).length)
const protectedCount = computed(() => orgAwareExamConfigs.value.filter(e => e.isArgusProtected).length)
const totalParticipants = computed(() => orgAwareExamConfigs.value.reduce((s, e) => s + e.participants, 0))

// --- Settings modal ---
const selectedExam = ref<ExamProctoringConfig | null>(null)
const activeTab = ref<'settings' | 'exceptions'>('settings')

// --- Backend persistence state ---
const api = useAdminAPI()
const settingsSaving = ref(false)
const settingsSaved = ref(false)
const settingsError = ref('')
const settingsHydrating = ref(false)
const settingsDirty = ref(false)

// Snapshot of server-persisted settings for dirty tracking
let serverSettingsSnapshot: string | null = null

async function openSettings(exam: ExamProctoringConfig) {
  selectedExam.value = exam
  activeTab.value = 'settings'
  settingsSaved.value = false
  settingsError.value = ''
  settingsDirty.value = false

  // Hydrate from backend
  if (exam.orgId && exam.id) {
    settingsHydrating.value = true
    try {
      const remote = await api.getProctoringSettings(exam.orgId, exam.id)
      // Merge backend settings into local settings (excluding metadata fields)
      const { orgId: _o, examId: _e, updatedAt: _u, updatedBy: _b, cleanThreshold: _ct, warningThreshold: _wt, autoTerminate: _at, autoTerminateAt: _ata, forceLowSpecMode: _f, ...toggles } = remote as any
      Object.assign(exam.settings, toggles)
      serverSettingsSnapshot = JSON.stringify(exam.settings)
    } catch {
      // No server settings — use local defaults (first time)
      serverSettingsSnapshot = JSON.stringify(exam.settings)
    } finally {
      settingsHydrating.value = false
    }
  }
}

async function saveSettings() {
  if (!selectedExam.value) return

  settingsSaving.value = true
  settingsError.value = ''
  settingsSaved.value = false

  try {
    const exam = selectedExam.value
    await api.saveProctoringSettings(exam.orgId, exam.id, {
      orgId: exam.orgId,
      examId: exam.id,
      ...exam.settings,
      // Add verdict thresholds with defaults
      cleanThreshold: 80,
      warningThreshold: 50,
      autoTerminate: false,
      autoTerminateAt: 30,
    } as any)

    settingsSaved.value = true
    settingsDirty.value = false
    serverSettingsSnapshot = JSON.stringify(exam.settings)
    setTimeout(() => { settingsSaved.value = false }, 3000)
  } catch (err: any) {
    settingsError.value = err.message || 'Ошибка сохранения настроек'
  } finally {
    settingsSaving.value = false
  }
}

// Track dirty state when settings change
watch(
  () => selectedExam.value?.settings,
  () => {
    if (!selectedExam.value || !serverSettingsSnapshot) return
    settingsDirty.value = JSON.stringify(selectedExam.value.settings) !== serverSettingsSnapshot
  },
  { deep: true }
)

function closeSettings() {
  selectedExam.value = null
  settingsDirty.value = false
  serverSettingsSnapshot = null
  clearParticipant()
}

// --- Preset Profiles ---
interface PresetProfile {
  id: string
  label: string
  icon: string
  description: string
  color: string
  bgFn: (o: number) => string
  settings: ExamProctoringSettings
}

const presetProfiles: PresetProfile[] = [
  {
    id: 'extreme', label: 'Экстремальная', icon: 'i-lucide-shield-alert',
    description: 'Максимальная защита — все детекторы включены',
    color: 'var(--argus-error)',
    bgFn: errorBg,
    settings: {
      requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: true,
      objectDetectionPhone: true, objectDetectionPerson: true,
      gazeTracking: true, gazeSensitivity: 85, gazeDeviationLimitSec: 3,
      voiceDetectionThreshold: 80, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true,
      emotionStressAnalysis: true, focusLossScore: true, blinkPatternAnalysis: true,
      forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: true,
      tabSwitchingLimit: 0, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: true, blockRemoteAccess: true, blockContextMenu: true,
      vpnProxyDetection: true, localNetworkScan: true,
      typingDynamics: true, handCursorSync: true,
      processScanning: true, hardwareDeviceDetection: true, advancedRemoteBlock: true, hardwareIdBinding: true, deepMultiMonitorCheck: true, forceLowSpecMode: false
    }
  },
  {
    id: 'standard', label: 'Стандарт', icon: 'i-lucide-shield-check',
    description: 'Оптимальный баланс безопасности и удобства',
    color: 'var(--argus-accent)',
    bgFn: accentBg,
    settings: {
      requireSideCamera: false, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, roomScan360: false,
      objectDetectionPhone: true, objectDetectionPerson: true,
      gazeTracking: true, gazeSensitivity: 60, gazeDeviationLimitSec: 8,
      voiceDetectionThreshold: 50, voiceActivityDetection: true, audioPeripheryDetection: true, smartNoiseFilter: true,
      emotionStressAnalysis: false, focusLossScore: true, blinkPatternAnalysis: false,
      forceFullscreen: true, fullscreenExitDetection: true, webDisplayMonitoring: false,
      tabSwitchingLimit: 3, blockCopyPaste: false, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: false, blockRemoteAccess: true, blockContextMenu: true,
      vpnProxyDetection: true, localNetworkScan: false,
      typingDynamics: false, handCursorSync: false,
      processScanning: true, hardwareDeviceDetection: false, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true, forceLowSpecMode: false
    }
  },
  {
    id: 'light', label: 'Лёгкая', icon: 'i-lucide-shield-off',
    description: 'Минимальный контроль — для простых тестов',
    color: 'var(--argus-success)',
    bgFn: successBg,
    settings: {
      requireSideCamera: false, faceVerification: false, dynamicFaceRecheck: false, antiSpoofing: false, roomScan360: false,
      objectDetectionPhone: true, objectDetectionPerson: false,
      gazeTracking: false, gazeSensitivity: 30, gazeDeviationLimitSec: 15,
      voiceDetectionThreshold: 25, voiceActivityDetection: false, audioPeripheryDetection: false, smartNoiseFilter: false,
      emotionStressAnalysis: false, focusLossScore: false, blinkPatternAnalysis: false,
      forceFullscreen: false, fullscreenExitDetection: false, webDisplayMonitoring: false,
      tabSwitchingLimit: 10, blockCopyPaste: false, blockPrintScreen: false, blockVirtualMachine: false, blockMultiDesktop: false, blockRemoteAccess: false, blockContextMenu: false,
      vpnProxyDetection: false, localNetworkScan: false,
      typingDynamics: false, handCursorSync: false,
      processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false, forceLowSpecMode: false
    }
  }
]

// Track active preset — computed so it auto-detects when settings change
const activePresetId = computed<string | null>(() => {
  if (!selectedExam.value) return null
  const s = selectedExam.value.settings
  for (const preset of presetProfiles) {
    const p = preset.settings
    const match = (Object.keys(p) as (keyof ExamProctoringSettings)[]).every(key => s[key] === p[key])
    if (match) return preset.id
  }
  return null
})

function applyPreset(preset: PresetProfile) {
  if (!selectedExam.value) return
  Object.assign(selectedExam.value.settings, preset.settings)
  if (preset.id === 'light') {
    selectedExam.value.isArgusProtected = false
  } else {
    selectedExam.value.isArgusProtected = true
  }
}

function toggleArgusProtection(exam: ExamProctoringConfig) {
  exam.isArgusProtected = !exam.isArgusProtected
  if (!exam.isArgusProtected) {
    applyPreset(presetProfiles[2]!) // Light preset
  } else {
    applyPreset(presetProfiles[1]!) // Standard preset
  }
}

// --- Individual Exceptions (Enhanced — Participant Search + Mirrored Settings) ---
const exceptionSearchQuery = ref('')
const excParticipantQuery = ref('')
const excDropdownOpen = ref(false)
const excSelectedStudent = ref<{ id: string, name: string, iin: string, phone: string } | null>(null)
const excReason = ref('')
const excSaved = ref(false)

// Individual settings — mirrors the general settings structure exactly
const excSettings = reactive<ExamProctoringSettings>({
  requireSideCamera: false,
  faceVerification: true,
  dynamicFaceRecheck: false,
  antiSpoofing: true,
  roomScan360: false,
  objectDetectionPhone: true,
  objectDetectionPerson: true,
  gazeTracking: true,
  gazeSensitivity: 60,
  gazeDeviationLimitSec: 8,
  voiceDetectionThreshold: 50,
  voiceActivityDetection: true,
  audioPeripheryDetection: true,
  smartNoiseFilter: true,
  emotionStressAnalysis: false,
  focusLossScore: true,
  blinkPatternAnalysis: false,
  forceFullscreen: true,
  fullscreenExitDetection: true,
  webDisplayMonitoring: false,
  tabSwitchingLimit: 3,
  blockCopyPaste: false,
  blockPrintScreen: false,
  blockVirtualMachine: false,
  blockMultiDesktop: false,
  blockRemoteAccess: false,
  blockContextMenu: false,
  vpnProxyDetection: true,
  localNetworkScan: false,
  typingDynamics: false,
  handCursorSync: false,
  processScanning: true,
  hardwareDeviceDetection: false,
  advancedRemoteBlock: true,
  hardwareIdBinding: false,
  deepMultiMonitorCheck: true,
  forceLowSpecMode: false
})

// Filtered participants from student roster for the search dropdown
const excParticipantResults = computed(() => {
  if (!selectedExam.value) return []
  const q = excParticipantQuery.value.toLowerCase().trim()
  if (!q) return store.studentRoster.filter(s => s.examId === selectedExam.value?.id).slice(0, 8)
  return store.studentRoster
    .filter(s => s.examId === selectedExam.value?.id)
    .filter(s => s.name.toLowerCase().includes(q) || s.iin.includes(q) || s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, '')))
    .slice(0, 8)
})

// Count how many settings differ from global
const excDiffCount = computed(() => {
  if (!selectedExam.value) return 0
  const g = selectedExam.value.settings
  let count = 0
  for (const key of Object.keys(g) as (keyof ExamProctoringSettings)[]) {
    if (excSettings[key] !== g[key]) count++
  }
  return count
})

// Select a participant and load their existing overrides (if any) or global defaults
function selectParticipant(student: { id: string, name: string, iin: string, phone: string }) {
  excSelectedStudent.value = student
  excDropdownOpen.value = false
  excParticipantQuery.value = ''
  excSaved.value = false

  // Check if this student already has an exception
  const existing = selectedExam.value?.exceptions.find(e => e.studentId === student.iin)
  if (existing) {
    // Load global + merge overrides
    Object.assign(excSettings, { ...selectedExam.value!.settings, ...existing.overrides })
    excReason.value = existing.reason || ''
  } else {
    // Start from global settings
    Object.assign(excSettings, { ...selectedExam.value!.settings })
    excReason.value = ''
  }
}

function clearParticipant() {
  excSelectedStudent.value = null
  excReason.value = ''
  excSaved.value = false
}

// Reset individual exception settings to match the global exam settings
function resetToGlobal() {
  if (!selectedExam.value) return
  Object.assign(excSettings, { ...selectedExam.value.settings })
  excReason.value = ''
}

// Check if the currently selected student already has a saved exception
const excHasExistingException = computed(() => {
  if (!excSelectedStudent.value || !selectedExam.value) return false
  return selectedExam.value.exceptions.some(e => e.studentId === excSelectedStudent.value!.iin)
})

function saveException() {
  if (!selectedExam.value || !excSelectedStudent.value) return

  // Build overrides — only include settings that differ from global
  const g = selectedExam.value.settings
  const overrides: Partial<ExamProctoringSettings> = {}
  for (const key of Object.keys(g) as (keyof ExamProctoringSettings)[]) {
    if (excSettings[key] !== g[key]) {
      (overrides as any)[key] = excSettings[key]
    }
  }

  const overrideCount = Object.keys(overrides).length
  const profileLabel = overrideCount === 0 ? 'Без изменений' : `${overrideCount} переопределений`

  // Check if exception already exists — update or create
  const existingIdx = selectedExam.value.exceptions.findIndex(e => e.studentId === excSelectedStudent.value!.iin)
  const exception: StudentException = {
    id: existingIdx >= 0 ? selectedExam.value.exceptions[existingIdx]!.id : `se-${Date.now()}`,
    studentName: excSelectedStudent.value.name,
    studentId: excSelectedStudent.value.iin,
    phone: excSelectedStudent.value.phone,
    reason: excReason.value,
    profile: 'custom',
    profileLabel,
    overrides
  }

  if (existingIdx >= 0) {
    selectedExam.value.exceptions[existingIdx] = exception
  } else {
    selectedExam.value.exceptions.push(exception)
  }

  excSaved.value = true
  setTimeout(() => { excSaved.value = false }, 2000)
}

function removeException(exceptionId: string) {
  if (!selectedExam.value) return
  selectedExam.value.exceptions = selectedExam.value.exceptions.filter(e => e.id !== exceptionId)
  // If the removed student was selected, clear
  if (excSelectedStudent.value) {
    const stillExists = selectedExam.value.exceptions.find(e => e.studentId === excSelectedStudent.value!.iin)
    if (!stillExists) {
      clearParticipant()
    }
  }
}

// Filtered exceptions list by search
const filteredExceptions = computed(() => {
  if (!selectedExam.value) return []
  const q = exceptionSearchQuery.value.toLowerCase().trim()
  if (!q) return selectedExam.value.exceptions
  return selectedExam.value.exceptions.filter(e =>
    e.studentName.toLowerCase().includes(q) ||
    e.studentId.toLowerCase().includes(q) ||
    (e.reason && e.reason.toLowerCase().includes(q))
  )
})

// Get human-readable override label
function overrideLabel(key: string): string {
  const labels: Record<string, string> = {
    requireSideCamera: 'Бок. камера',
    faceVerification: 'Face ID',
    dynamicFaceRecheck: 'Dyn. Face ID',
    antiSpoofing: 'Anti-Spoof',
    roomScan360: 'Room Scan',
    objectDetectionPhone: 'Тел. детекция',
    objectDetectionPerson: 'Дет. лиц',
    gazeTracking: 'Взгляд',
    gazeSensitivity: 'Чувств. взгляда',
    gazeDeviationLimitSec: 'Лимит взгл.',
    voiceDetectionThreshold: 'Порог голоса',
    voiceActivityDetection: 'Голос. акт.',
    audioPeripheryDetection: 'Периф. звуки',
    smartNoiseFilter: 'Шумоподавитель',
    emotionStressAnalysis: 'Эмоции/Стресс',
    focusLossScore: 'Скоринг фокуса',
    blinkPatternAnalysis: 'Паттерн моргания',
    forceFullscreen: 'Полный экран',
    fullscreenExitDetection: 'Дет. выхода FS',
    webDisplayMonitoring: 'Внеш. дисплеи (Web)',
    tabSwitchingLimit: 'Лимит вкладок',
    blockCopyPaste: 'Буфер обмена',
    blockPrintScreen: 'PrintScreen',
    blockVirtualMachine: 'VM блок.',
    blockMultiDesktop: 'Мульти-десктоп',
    blockRemoteAccess: 'Удал. доступ',
    blockContextMenu: 'Контекст. меню',
    vpnProxyDetection: 'VPN/Proxy',
    localNetworkScan: 'Сетевой скан',
    typingDynamics: 'Typing Dyn.',
    handCursorSync: 'Рука-курсор',
    processScanning: 'Скан. процессов',
    hardwareDeviceDetection: 'Внеш. устройства',
    advancedRemoteBlock: 'Удал. доступ (Adv.)',
    hardwareIdBinding: 'Hardware ID',
    deepMultiMonitorCheck: 'Multi-Monitor'
  }
  return labels[key] ?? key
}

function overrideDisplayValue(value: any): string {
  if (typeof value === 'boolean') return value ? 'Вкл' : 'Выкл'
  if (typeof value === 'number') return String(value)
  return String(value)
}

// formatDate/formatDatetime → replaced by useFormatters() composable

// Exam status helpers delegated to useStatusHelpers composable
const { examStatusLabel: statusLabel, examStatusColor: statusColor, examStatusBg: statusBg } = useStatusHelpers()

function sensitivityLabel(val: number): string {
  if (val >= 65) return 'Высокая'
  if (val >= 45) return 'Средняя'
  return 'Низкая'
}

function sensitivityColor(val: number): string {
  if (val >= 65) return 'var(--argus-error)'
  if (val >= 45) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

function activeRulesCount(exam: ExamProctoringConfig): number {
  let count = 0
  if (exam.settings.requireSideCamera) count++
  if (exam.settings.faceVerification) count++
  if (exam.settings.dynamicFaceRecheck) count++
  if (exam.settings.antiSpoofing) count++
  if (exam.settings.roomScan360) count++
  if (exam.settings.objectDetectionPhone) count++
  if (exam.settings.objectDetectionPerson) count++
  if (exam.settings.gazeTracking) count++
  if (exam.settings.voiceDetectionThreshold >= 50) count++
  if (exam.settings.voiceActivityDetection) count++
  if (exam.settings.audioPeripheryDetection) count++
  if (exam.settings.smartNoiseFilter) count++
  if (exam.settings.emotionStressAnalysis) count++
  if (exam.settings.focusLossScore) count++
  if (exam.settings.blinkPatternAnalysis) count++
  if (exam.settings.forceFullscreen) count++
  if (exam.settings.fullscreenExitDetection) count++
  if (exam.settings.webDisplayMonitoring) count++
  if (exam.settings.blockCopyPaste) count++
  if (exam.settings.tabSwitchingLimit < 5) count++
  if (exam.settings.blockPrintScreen) count++
  if (exam.settings.blockVirtualMachine) count++
  if (exam.settings.blockMultiDesktop) count++
  if (exam.settings.blockRemoteAccess) count++
  if (exam.settings.blockContextMenu) count++
  if (exam.settings.vpnProxyDetection) count++
  if (exam.settings.localNetworkScan) count++
  if (exam.settings.typingDynamics) count++
  if (exam.settings.handCursorSync) count++
  if (exam.settings.processScanning) count++
  if (exam.settings.hardwareDeviceDetection) count++
  if (exam.settings.advancedRemoteBlock) count++
  if (exam.settings.hardwareIdBinding) count++
  if (exam.settings.deepMultiMonitorCheck) count++
  return count
}

function exceptionProfileIcon(_profile: string): string {
  return 'i-lucide-user-cog'
}
</script>

<template>
  <div class="p-6 space-y-5">
    <!-- ============================== -->
    <!--  HEADER + EDUSER SYNC          -->
    <!-- ============================== -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-xl"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon name="i-lucide-graduation-cap" class="size-5" style="color: var(--argus-accent);" />
          </div>
          <div>
            <h1 class="text-xl font-bold" style="color: var(--argus-text);">Экзамены</h1>
            <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
              Интеграция с EDUSER · Управление прокторингом
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <div
          class="flex items-center gap-2 px-3 py-2 rounded-lg"
          :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)' }"
        >
          <UIcon name="i-lucide-cloud" class="size-3.5" style="color: var(--argus-text-dimmed);" />
          <span class="text-[10px] font-medium" style="color: var(--argus-text-dimmed);">{{ formatDateTime(store.eduserLastSync) }}</span>
        </div>
        <button
          class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
          :style="{ background: store.eduserSyncing ? accentBg(0.2) : 'var(--argus-accent)', color: '#fff' }"
          :disabled="store.eduserSyncing"
          @click="store.syncEduser()"
        >
          <UIcon name="i-lucide-refresh-cw" class="size-3.5" :class="{ 'animate-spin': store.eduserSyncing }" />
          {{ store.eduserSyncing ? 'Синхронизация...' : 'Sync Now' }}
        </button>
      </div>
    </div>

    <!-- ============================== -->
    <!--  COMPACT KPI CARDS             -->
    <!-- ============================== -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <div class="flex items-center gap-3 px-4 py-3 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
        <div class="flex items-center justify-center size-9 rounded-lg shrink-0" :style="{ background: accentBg(0.08) }">
          <UIcon name="i-lucide-book-open" class="size-4" style="color: var(--argus-accent);" />
        </div>
        <div class="min-w-0">
          <p class="text-lg font-bold tabular-nums leading-tight" style="color: var(--argus-text);">{{ totalExams }}</p>
          <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Всего</p>
        </div>
      </div>
      <div class="flex items-center gap-3 px-4 py-3 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: successBg(0.2) }">
        <div class="flex items-center justify-center size-9 rounded-lg shrink-0" :style="{ background: successBg(0.08) }">
          <UIcon name="i-lucide-play-circle" class="size-4" style="color: var(--argus-success);" />
        </div>
        <div class="min-w-0">
          <p class="text-lg font-bold tabular-nums leading-tight" style="color: var(--argus-success);">{{ activeExams }}</p>
          <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Активные</p>
        </div>
      </div>
      <div class="flex items-center gap-3 px-4 py-3 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
        <div class="flex items-center justify-center size-9 rounded-lg shrink-0" :style="{ background: isDark ? 'rgba(148, 163, 184, 0.08)' : 'rgba(100, 116, 139, 0.06)' }">
          <UIcon name="i-lucide-check-circle-2" class="size-4" style="color: var(--argus-text-dimmed);" />
        </div>
        <div class="min-w-0">
          <p class="text-lg font-bold tabular-nums leading-tight" style="color: var(--argus-text-muted);">{{ completedExams }}</p>
          <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Завершённые</p>
        </div>
      </div>
      <div class="flex items-center gap-3 px-4 py-3 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: warningBg(0.2) }">
        <div class="flex items-center justify-center size-9 rounded-lg shrink-0" :style="{ background: warningBg(0.08) }">
          <UIcon name="i-lucide-alert-triangle" class="size-4" style="color: var(--argus-warning);" />
        </div>
        <div class="min-w-0">
          <p class="text-lg font-bold tabular-nums leading-tight" style="color: var(--argus-warning);">{{ withViolations }}</p>
          <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">С нарушениями</p>
        </div>
      </div>
    </div>

    <!-- Secondary stats -->
    <div class="flex flex-wrap items-center gap-4 px-1">
      <div class="flex items-center gap-1.5">
        <UIcon name="i-lucide-shield-check" class="size-3.5" style="color: var(--argus-success);" />
        <span class="text-[11px] font-medium" style="color: var(--argus-text-dimmed);">
          Argus Protected: <span class="font-bold" style="color: var(--argus-success);">{{ protectedCount }}</span>
        </span>
      </div>
      <span class="text-[10px]" style="color: var(--argus-border);">·</span>
      <div class="flex items-center gap-1.5">
        <UIcon name="i-lucide-users" class="size-3.5" style="color: var(--argus-text-dimmed);" />
        <span class="text-[11px] font-medium" style="color: var(--argus-text-dimmed);">
          Участников: <span class="font-bold" style="color: var(--argus-text);">{{ totalParticipants.toLocaleString() }}</span>
        </span>
      </div>
      <span class="text-[10px]" style="color: var(--argus-border);">·</span>
      <div class="flex items-center gap-1.5">
        <UIcon name="i-lucide-cloud-check" class="size-3.5" style="color: var(--argus-accent);" />
        <span class="text-[11px] font-medium" style="color: var(--argus-text-dimmed);">
          EDUSER: <span class="font-bold" style="color: var(--argus-accent);">Активна</span>
        </span>
      </div>
    </div>

    <!-- ============================== -->
    <!--  SEARCH BAR + TABLE            -->
    <!-- ============================== -->
    <div class="rounded-xl border overflow-hidden" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
      <!-- Search bar -->
      <div class="flex items-center gap-3 px-5 py-3 border-b" :style="{ borderColor: 'var(--argus-border)' }">
        <div
          class="flex items-center gap-2 flex-1 px-3 py-2 rounded-lg transition-all"
          :style="{ background: 'var(--argus-bg-elevated)', border: `1px solid ${searchFocused ? 'var(--argus-accent)' : 'var(--argus-border)'}`, boxShadow: searchFocused ? `0 0 0 3px ${accentBg(0.1)}` : 'none' }"
        >
          <UIcon name="i-lucide-search" class="size-3.5 shrink-0" style="color: var(--argus-text-dimmed);" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Поиск экзамена..."
            class="flex-1 bg-transparent text-xs font-medium outline-none placeholder:text-[var(--argus-text-dimmed)]"
            :style="{ color: 'var(--argus-text)' }"
            @focus="searchFocused = true"
            @blur="searchFocused = false"
          >
          <button v-if="searchQuery" class="flex items-center justify-center size-4 rounded-full" style="color: var(--argus-text-dimmed);" @click="searchQuery = ''">
            <UIcon name="i-lucide-x" class="size-3" />
          </button>
        </div>
        <!-- Org Context (Super Admin) -->
        <div
          v-if="authStore.isSuperAdmin"
          class="flex items-center gap-2 px-3 py-2 rounded-lg border text-xs shrink-0"
          :style="{
            background: 'var(--argus-bg-elevated)',
            borderColor: authStore.selectedOrgId ? accentBg(0.3) : 'var(--argus-border)',
            color: authStore.selectedOrgId ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
          }"
        >
          <UIcon name="i-lucide-building-2" class="size-3.5 shrink-0" />
          <span class="font-medium truncate">
            {{ authStore.selectedOrgId ? store.getOrgName(authStore.selectedOrgId) : 'Все организации' }}
          </span>
        </div>
        <span class="text-[10px] font-medium shrink-0" style="color: var(--argus-text-dimmed);">{{ filteredExams.length }} из {{ totalExams }}</span>
      </div>

      <!-- Table header -->
      <div
        class="hidden lg:grid grid-cols-12 gap-2 px-5 py-2.5 text-[9px] font-bold uppercase tracking-wider border-b"
        :style="{ color: 'var(--argus-text-dimmed)', borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
      >
        <div class="col-span-3">Экзамен</div>
        <div class="col-span-1 text-center">Статус</div>
        <div class="col-span-2 text-center">Дата</div>
        <div class="col-span-1 text-center">Студенты</div>
        <div class="col-span-1 text-center">Нарушения</div>
        <div class="col-span-2 text-center">Защита</div>
        <div class="col-span-2 text-right">Действие</div>
      </div>

      <!-- Table rows -->
      <div v-if="filteredExams.length > 0">
        <div
          v-for="exam in filteredExams"
          :key="exam.id"
          class="grid grid-cols-1 lg:grid-cols-12 gap-2 lg:gap-2 px-5 py-3.5 items-center transition-all border-b last:border-b-0 cursor-pointer"
          :style="{ borderColor: 'var(--argus-border-subtle)' }"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          @click="openSettings(exam)"
        >
          <div class="lg:col-span-3 min-w-0">
            <div class="flex items-center gap-2">
              <p class="text-xs font-semibold truncate" style="color: var(--argus-text);">{{ exam.examName }}</p>
              <span
                v-if="authStore.isSuperAdmin"
                class="px-1.5 py-0.5 rounded text-[9px] font-bold shrink-0"
                :style="{
                  background: accentBg(0.08),
                  color: 'var(--argus-accent)',
                  border: `1px solid ${accentBg(0.15)}`
                }"
              >
                {{ store.getOrgName(exam.orgId) }}
              </span>
            </div>
            <div class="flex items-center gap-2 mt-0.5">
              <span class="text-[9px] font-mono" style="color: var(--argus-text-dimmed);">{{ exam.examCode }}</span>
              <span class="text-[8px]" style="color: var(--argus-border);">·</span>
              <span class="text-[9px]" style="color: var(--argus-text-dimmed);">{{ exam.eduserId }}</span>
              <span
                v-if="exam.exceptions.length > 0"
                class="inline-flex items-center gap-0.5 text-[7px] font-bold px-1 py-0.5 rounded"
                :style="{ background: purpleBg(0.1), color: isDark ? '#a78bfa' : '#7c3aed' }"
              >
                <UIcon name="i-lucide-user-cog" class="size-2" />
                {{ exam.exceptions.length }}
              </span>
            </div>
          </div>

          <div class="lg:col-span-1 flex lg:justify-center">
            <span class="inline-flex items-center gap-1 text-[9px] font-bold px-2 py-1 rounded-full" :style="{ background: statusBg(exam.status, 0.1), color: statusColor(exam.status) }">
              <span class="size-1.5 rounded-full" :style="{ background: statusColor(exam.status) }" />
              {{ statusLabel(exam.status) }}
            </span>
          </div>

          <div class="lg:col-span-2 flex lg:justify-center">
            <span class="text-[11px] font-medium tabular-nums" style="color: var(--argus-text-muted);">{{ formatDate(exam.date) }}</span>
          </div>

          <div class="lg:col-span-1 flex lg:justify-center">
            <div class="flex items-center gap-1">
              <UIcon name="i-lucide-users" class="size-3" style="color: var(--argus-text-dimmed);" />
              <span class="text-[11px] font-bold tabular-nums" style="color: var(--argus-text);">{{ exam.participants }}</span>
            </div>
          </div>

          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              v-if="exam.violationCount > 0"
              class="inline-flex items-center gap-1 text-[10px] font-bold px-1.5 py-0.5 rounded"
              :style="{ background: exam.violationCount >= 10 ? errorBg(0.1) : warningBg(0.1), color: exam.violationCount >= 10 ? 'var(--argus-error)' : 'var(--argus-warning)' }"
            >
              {{ exam.violationCount }}
            </span>
            <span v-else class="text-[10px]" style="color: var(--argus-text-dimmed);">—</span>
          </div>

          <div class="lg:col-span-2 flex lg:justify-center">
            <div class="flex items-center gap-1.5">
              <span
                class="inline-flex items-center gap-1 text-[8px] font-bold px-2 py-1 rounded-full"
                :style="{ background: exam.isArgusProtected ? successBg(0.1) : 'var(--argus-bg-hover)', color: exam.isArgusProtected ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
              >
                <UIcon :name="exam.isArgusProtected ? 'i-lucide-shield-check' : 'i-lucide-shield-off'" class="size-2.5" />
                {{ exam.isArgusProtected ? 'ARGUS' : 'СТАНДАРТ' }}
              </span>
              <span
                v-if="exam.isArgusProtected"
                class="text-[8px] font-medium px-1.5 py-0.5 rounded"
                :style="{ background: accentBg(0.06), color: 'var(--argus-accent)' }"
              >
                {{ activeRulesCount(exam) }} правил
              </span>
            </div>
          </div>

          <div class="lg:col-span-2 flex lg:justify-end">
            <button
              class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-[10px] font-bold transition-all"
              :style="{ background: accentBg(0.08), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.15)}` }"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.15)"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.08)"
              @click.stop="openSettings(exam)"
            >
              <UIcon name="i-lucide-settings-2" class="size-3" />
              Настроить прокторинг
            </button>
          </div>
        </div>
      </div>

      <div v-else class="flex flex-col items-center justify-center py-16 px-4">
        <UIcon name="i-lucide-search-x" class="size-10 mb-3" style="color: var(--argus-text-dimmed); opacity: 0.3;" />
        <p class="text-sm font-medium" style="color: var(--argus-text-dimmed);">Экзамены не найдены</p>
        <p class="text-[10px] mt-1" style="color: var(--argus-text-dimmed);">Попробуйте изменить поисковый запрос</p>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  COMPREHENSIVE SETTINGS MODAL              -->
    <!-- ========================================== -->
    <Teleport to="body">
      <Transition name="modal">
        <div v-if="selectedExam" class="fixed inset-0 z-[100] flex items-center justify-center p-4">
          <div
            class="absolute inset-0"
            :style="{ background: isDark ? 'rgba(0, 0, 0, 0.7)' : 'rgba(0, 0, 0, 0.4)' }"
            @click="closeSettings"
          />

          <div
            class="relative w-full max-w-3xl max-h-[92vh] rounded-2xl border overflow-hidden flex flex-col z-10"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <!-- Modal header -->
            <div class="flex items-center justify-between px-6 py-4 border-b shrink-0" style="border-color: var(--argus-border);">
              <div>
                <div class="flex items-center gap-2">
                  <h2 class="text-lg font-bold" style="color: var(--argus-text);">Настройки прокторинга</h2>
                  <span
                    class="text-[8px] font-bold px-2 py-0.5 rounded-full"
                    :style="{ background: selectedExam.isArgusProtected ? successBg(0.1) : 'var(--argus-bg-hover)', color: selectedExam.isArgusProtected ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
                  >
                    {{ selectedExam.isArgusProtected ? 'ARGUS PROTECTED' : 'STANDARD' }}
                  </span>
                </div>
                <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">{{ selectedExam.examName }} · {{ selectedExam.examCode }}</p>
              </div>
              <button
                class="flex items-center justify-center size-8 rounded-lg transition-colors cursor-pointer"
                style="color: var(--argus-text-dimmed);"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                @click="closeSettings"
              >
                <UIcon name="i-lucide-x" class="size-5" />
              </button>
            </div>

            <!-- Tab switcher -->
            <div class="flex items-center gap-1 px-6 pt-3 shrink-0">
              <button
                class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all"
                :style="{
                  background: activeTab === 'settings' ? accentBg(0.1) : 'transparent',
                  color: activeTab === 'settings' ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)',
                  border: activeTab === 'settings' ? `1px solid ${accentBg(0.2)}` : '1px solid transparent'
                }"
                @click="activeTab = 'settings'"
              >
                <UIcon name="i-lucide-settings" class="size-3.5" />
                Настройки
              </button>
              <button
                class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all"
                :style="{
                  background: activeTab === 'exceptions' ? purpleBg(0.1) : 'transparent',
                  color: activeTab === 'exceptions' ? (isDark ? '#a78bfa' : '#7c3aed') : 'var(--argus-text-dimmed)',
                  border: activeTab === 'exceptions' ? `1px solid ${purpleBg(0.2)}` : '1px solid transparent'
                }"
                @click="activeTab = 'exceptions'"
              >
                <UIcon name="i-lucide-user-cog" class="size-3.5" />
                Индивидуальные исключения
                <span
                  v-if="selectedExam.exceptions.length > 0"
                  class="text-[9px] font-bold px-1.5 py-0.5 rounded-full ml-0.5"
                  :style="{ background: purpleBg(0.15), color: isDark ? '#a78bfa' : '#7c3aed' }"
                >
                  {{ selectedExam.exceptions.length }}
                </span>
              </button>
            </div>

            <!-- ============================== -->
            <!-- TAB: SETTINGS                  -->
            <!-- ============================== -->
            <div v-if="activeTab === 'settings'" class="flex-1 overflow-y-auto px-6 py-5 space-y-6">
              <!-- Preset Profiles -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-zap" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Быстрые профили</h3>
                </div>
                <div class="grid grid-cols-3 gap-2">
                  <button
                    v-for="preset in presetProfiles"
                    :key="preset.id"
                    class="flex flex-col items-center gap-2 p-3 rounded-xl border-2 transition-all cursor-pointer text-center relative"
                    :style="{
                      background: activePresetId === preset.id ? preset.bgFn(0.1) : 'var(--argus-bg-elevated)',
                      borderColor: activePresetId === preset.id ? preset.color : 'var(--argus-border)',
                      boxShadow: activePresetId === preset.id ? `0 0 12px ${preset.bgFn(0.2)}, inset 0 1px 0 ${preset.bgFn(0.1)}` : 'none'
                    }"
                    @mouseenter="if (activePresetId !== preset.id) { ($event.currentTarget as HTMLElement).style.borderColor = preset.color; ($event.currentTarget as HTMLElement).style.background = preset.bgFn(0.05) }"
                    @mouseleave="if (activePresetId !== preset.id) { ($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-border)'; ($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-elevated)' }"
                    @click="applyPreset(preset)"
                  >
                    <!-- Active indicator dot -->
                    <div
                      v-if="activePresetId === preset.id"
                      class="absolute top-2 right-2 size-2 rounded-full"
                      :style="{ background: preset.color, boxShadow: `0 0 6px ${preset.color}` }"
                    />
                    <div
                      class="flex items-center justify-center size-8 rounded-lg transition-all"
                      :style="{ background: activePresetId === preset.id ? preset.bgFn(0.2) : preset.bgFn(0.12), border: activePresetId === preset.id ? `1px solid ${preset.bgFn(0.3)}` : '1px solid transparent' }"
                    >
                      <UIcon :name="preset.icon" class="size-4" :style="{ color: preset.color }" />
                    </div>
                    <span class="text-[11px] font-bold" :style="{ color: preset.color }">{{ preset.label }}</span>
                    <span class="text-[9px] leading-tight" style="color: var(--argus-text-dimmed);">{{ preset.description }}</span>
                  </button>
                </div>
              </div>

              <!-- Master toggle -->
              <div
                class="flex items-center justify-between p-4 rounded-xl border"
                :style="{ background: selectedExam.isArgusProtected ? successBg(0.05) : 'var(--argus-bg-elevated)', borderColor: selectedExam.isArgusProtected ? successBg(0.15) : 'var(--argus-border)' }"
              >
                <div class="flex items-center gap-3">
                  <div class="flex items-center justify-center size-10 rounded-lg" :style="{ background: selectedExam.isArgusProtected ? successBg(0.12) : 'var(--argus-bg-hover)', border: `1px solid ${selectedExam.isArgusProtected ? successBg(0.2) : 'var(--argus-border)'}` }">
                    <UIcon name="i-lucide-shield-check" class="size-5" :style="{ color: selectedExam.isArgusProtected ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }" />
                  </div>
                  <div>
                    <p class="text-sm font-semibold" style="color: var(--argus-text);">Argus AI Защита</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Активировать полный AI-мониторинг</p>
                  </div>
                </div>
                <button class="relative w-12 h-6 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.isArgusProtected ? 'var(--argus-success)' : 'var(--argus-border)' }" @click="toggleArgusProtection(selectedExam)">
                  <div class="absolute top-0.5 size-5 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.isArgusProtected ? '26px' : '2px' }" />
                </button>
              </div>

              <!-- SECTION 1: ВИДЕО-ПРАВИЛА -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-video" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Видео-правила</h3>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Требовать боковую камеру</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Подключение мобильного телефона как боковой камеры</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.requireSideCamera ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.requireSideCamera = !selectedExam.settings.requireSideCamera">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.requireSideCamera ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Верификация лица (Face ID)</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Проверка личности перед началом экзамена</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.faceVerification ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.faceVerification = !selectedExam.settings.faceVerification">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.faceVerification ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция телефона</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-обнаружение мобильных устройств в кадре</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.objectDetectionPhone ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.objectDetectionPhone = !selectedExam.settings.objectDetectionPhone">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.objectDetectionPhone ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция посторонних лиц</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Обнаружение дополнительных людей в кадре камеры</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.objectDetectionPerson ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.objectDetectionPerson = !selectedExam.settings.objectDetectionPerson">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.objectDetectionPerson ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Dynamic Face ID</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Периодическая повторная проверка лица во время экзамена</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.dynamicFaceRecheck ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.dynamicFaceRecheck = !selectedExam.settings.dynamicFaceRecheck">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.dynamicFaceRecheck ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Anti-Spoofing</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Защита от подмены лица (фото, видео, маска)</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.antiSpoofing ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.antiSpoofing = !selectedExam.settings.antiSpoofing">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.antiSpoofing ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Room Scan 360°</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Полный осмотр помещения перед началом экзамена</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.roomScan360 ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.roomScan360 = !selectedExam.settings.roomScan360">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.roomScan360 ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- SECTION 2: ЧУВСТВИТЕЛЬНОСТЬ ИИ -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-brain" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Чувствительность ИИ</h3>
                </div>

                <div class="py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div class="flex items-center justify-between mb-2">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Порог детекции голоса</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Чувствительность к фоновому шуму (0% — выкл., 100% — макс.)</p>
                    </div>
                    <span class="text-xs font-bold tabular-nums" :style="{ color: sensitivityColor(selectedExam.settings.voiceDetectionThreshold) }">
                      {{ selectedExam.settings.voiceDetectionThreshold }}%
                    </span>
                  </div>
                  <input v-model.number="selectedExam.settings.voiceDetectionThreshold" type="range" min="0" max="100" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, var(--argus-accent) ${selectedExam.settings.voiceDetectionThreshold}%, var(--argus-border) ${selectedExam.settings.voiceDetectionThreshold}%)` }">
                  <div class="flex items-center justify-between mt-1">
                    <span class="text-[9px]" style="color: var(--argus-text-dimmed);">0% Выкл.</span>
                    <span class="text-[9px]" style="color: var(--argus-text-dimmed);">100% Макс.</span>
                  </div>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Отслеживание взгляда</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-мониторинг направления взгляда студента</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.gazeTracking ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.gazeTracking = !selectedExam.settings.gazeTracking">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.gazeTracking ? '22px' : '2px' }" />
                  </button>
                </div>

                <div v-if="selectedExam.settings.gazeTracking" class="py-3">
                  <div class="flex items-center justify-between mb-2">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Чувствительность взгляда</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Порог срабатывания при отклонении взгляда</p>
                    </div>
                    <span class="text-xs font-bold tabular-nums" :style="{ color: sensitivityColor(selectedExam.settings.gazeSensitivity) }">
                      {{ selectedExam.settings.gazeSensitivity }}% · {{ sensitivityLabel(selectedExam.settings.gazeSensitivity) }}
                    </span>
                  </div>
                  <input v-model.number="selectedExam.settings.gazeSensitivity" type="range" min="10" max="95" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, var(--argus-accent) ${(selectedExam.settings.gazeSensitivity - 10) / 85 * 100}%, var(--argus-border) ${(selectedExam.settings.gazeSensitivity - 10) / 85 * 100}%)` }">
                  <div class="flex items-center justify-between mt-1">
                    <span class="text-[9px]" style="color: var(--argus-text-dimmed);">Низкая</span>
                    <span class="text-[9px]" style="color: var(--argus-text-dimmed);">Высокая</span>
                  </div>
                </div>

                <div v-if="selectedExam.settings.gazeTracking" class="py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div class="flex items-center justify-between mb-2">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Лимит отклонения взгляда</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Максимальное время отклонения взгляда до срабатывания (сек)</p>
                    </div>
                    <span class="text-xs font-bold tabular-nums px-2 py-0.5 rounded" :style="{ background: selectedExam.settings.gazeDeviationLimitSec <= 5 ? errorBg(0.1) : selectedExam.settings.gazeDeviationLimitSec <= 10 ? warningBg(0.1) : 'var(--argus-bg-hover)', color: selectedExam.settings.gazeDeviationLimitSec <= 5 ? 'var(--argus-error)' : selectedExam.settings.gazeDeviationLimitSec <= 10 ? 'var(--argus-warning)' : 'var(--argus-text-muted)' }">
                      {{ selectedExam.settings.gazeDeviationLimitSec }} сек
                    </span>
                  </div>
                  <input v-model.number="selectedExam.settings.gazeDeviationLimitSec" type="range" min="3" max="30" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, var(--argus-accent) ${(selectedExam.settings.gazeDeviationLimitSec - 3) / 27 * 100}%, var(--argus-border) ${(selectedExam.settings.gazeDeviationLimitSec - 3) / 27 * 100}%)` }">
                  <div class="flex items-center justify-between mt-1">
                    <span class="text-[9px]" style="color: var(--argus-error);">3 сек</span>
                    <span class="text-[9px]" style="color: var(--argus-text-dimmed);">30 сек</span>
                  </div>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция голосовой активности</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-распознавание речи и разговоров во время экзамена</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.voiceActivityDetection ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.voiceActivityDetection = !selectedExam.settings.voiceActivityDetection">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.voiceActivityDetection ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция периферийных звуков</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-анализ периферийных звуков (шёпот, наушники, второе устройство)</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.audioPeripheryDetection ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.audioPeripheryDetection = !selectedExam.settings.audioPeripheryDetection">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.audioPeripheryDetection ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Умный шумоподавитель</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-классификация источника звука и интеллектуальная фильтрация</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.smartNoiseFilter ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.smartNoiseFilter = !selectedExam.settings.smartNoiseFilter">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.smartNoiseFilter ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- SECTION 3: ПСИХОМЕТРИЯ И AI-АНАЛИТИКА -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-scan-face" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Психометрия и AI-аналитика</h3>
                  <span class="text-[8px] font-bold px-1.5 py-0.5 rounded" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }">NEW</span>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Анализ эмоций и стресса</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-анализ микроэкспрессий: стресс, уверенность, замешательство</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.emotionStressAnalysis ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.emotionStressAnalysis = !selectedExam.settings.emotionStressAnalysis">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.emotionStressAnalysis ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Скоринг потери фокуса</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Непрерывный AI-скоринг концентрации по мимике и движениям</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.focusLossScore ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.focusLossScore = !selectedExam.settings.focusLossScore">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.focusLossScore ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Анализ паттерна моргания</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Детекция аномального моргания (чтение с экрана, подсказки)</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blinkPatternAnalysis ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.blinkPatternAnalysis = !selectedExam.settings.blinkPatternAnalysis">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blinkPatternAnalysis ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- SECTION 4: ОГРАНИЧЕНИЯ БРАУЗЕРА -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-monitor" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Ограничения браузера</h3>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Принудительный полный экран</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Автоматический переход в полноэкранный режим при старте и запрет выхода до завершения</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.forceFullscreen ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.forceFullscreen = !selectedExam.settings.forceFullscreen">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.forceFullscreen ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция выхода из полноэкранного режима</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Мгновенная фиксация попытки свернуть браузер или переключиться на другое приложение</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.fullscreenExitDetection ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.fullscreenExitDetection = !selectedExam.settings.fullscreenExitDetection">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.fullscreenExitDetection ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Мониторинг внешних дисплеев (Web-level)</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Обнаружение и запрет прохождения экзамена при подключении второго монитора через Browser API</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.webDisplayMonitoring ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.webDisplayMonitoring = !selectedExam.settings.webDisplayMonitoring">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.webDisplayMonitoring ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div class="flex items-center justify-between mb-2">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Лимит переключения вкладок</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Максимум переключений до авто-прерывания</p>
                    </div>
                    <span class="text-xs font-bold tabular-nums px-2 py-0.5 rounded" :style="{ background: selectedExam.settings.tabSwitchingLimit === 0 ? errorBg(0.1) : selectedExam.settings.tabSwitchingLimit <= 2 ? warningBg(0.1) : 'var(--argus-bg-hover)', color: selectedExam.settings.tabSwitchingLimit === 0 ? 'var(--argus-error)' : selectedExam.settings.tabSwitchingLimit <= 2 ? 'var(--argus-warning)' : 'var(--argus-text-muted)' }">
                      {{ selectedExam.settings.tabSwitchingLimit === 0 ? 'Запрещено' : selectedExam.settings.tabSwitchingLimit }}
                    </span>
                  </div>
                  <input v-model.number="selectedExam.settings.tabSwitchingLimit" type="range" min="0" max="10" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, var(--argus-accent) ${selectedExam.settings.tabSwitchingLimit / 10 * 100}%, var(--argus-border) ${selectedExam.settings.tabSwitchingLimit / 10 * 100}%)` }">
                  <div class="flex items-center justify-between mt-1">
                    <span class="text-[9px]" style="color: var(--argus-error);">Запрещено</span>
                    <span class="text-[9px]" style="color: var(--argus-text-dimmed);">10 раз</span>
                  </div>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка буфера обмена</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить копирование и вставку</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blockCopyPaste ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.blockCopyPaste = !selectedExam.settings.blockCopyPaste">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blockCopyPaste ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка PrintScreen</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить создание скриншотов экрана</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blockPrintScreen ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.blockPrintScreen = !selectedExam.settings.blockPrintScreen">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blockPrintScreen ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка виртуальных машин</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить прохождение экзамена в виртуальной среде</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blockVirtualMachine ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.blockVirtualMachine = !selectedExam.settings.blockVirtualMachine">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blockVirtualMachine ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка мульти-десктопа</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить использование нескольких рабочих столов</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blockMultiDesktop ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.blockMultiDesktop = !selectedExam.settings.blockMultiDesktop">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blockMultiDesktop ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка удалённого доступа</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить TeamViewer, AnyDesk и другие программы</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blockRemoteAccess ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.blockRemoteAccess = !selectedExam.settings.blockRemoteAccess">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blockRemoteAccess ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка контекстного меню</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Блокировка правой кнопки мыши и контекстного меню</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.blockContextMenu ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="selectedExam.settings.blockContextMenu = !selectedExam.settings.blockContextMenu">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.blockContextMenu ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- SECTION 5: СЕТЕВОЙ КОНТРОЛЬ -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-wifi" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Сетевой контроль</h3>
                  <span class="text-[8px] font-bold px-1.5 py-0.5 rounded" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }">NEW</span>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Обнаружение VPN/Proxy</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-обнаружение VPN, прокси-серверов и Tor-подключений</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.vpnProxyDetection ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.vpnProxyDetection = !selectedExam.settings.vpnProxyDetection">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.vpnProxyDetection ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Сканирование локальной сети</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Сканирование локальной сети на подозрительные устройства и подключения</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.localNetworkScan ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.localNetworkScan = !selectedExam.settings.localNetworkScan">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.localNetworkScan ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- SECTION 6: ПОВЕДЕНЧЕСКИЙ АНАЛИЗ -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-brain-circuit" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Поведенческий анализ</h3>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-1.5">
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Динамика набора текста</p>
                      <span class="shrink-0 px-1.5 py-px rounded text-[7px] font-bold uppercase tracking-wide" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }">Kernel-данные</span>
                    </div>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Анализ биометрического почерка клавиатурного ввода: WPM, латентность между нажатиями, ритм набора</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer shrink-0 ml-3" :style="{ background: selectedExam.settings.typingDynamics ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.typingDynamics = !selectedExam.settings.typingDynamics">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.typingDynamics ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div class="min-w-0 flex-1">
                    <div class="flex items-center gap-1.5">
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Синхронизация руки и курсора</p>
                      <span class="shrink-0 px-1.5 py-px rounded text-[7px] font-bold uppercase tracking-wide" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }">Kernel-данные</span>
                    </div>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Корреляция физических движений руки с перемещением курсора для детекции удалённого управления</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer shrink-0 ml-3" :style="{ background: selectedExam.settings.handCursorSync ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.handCursorSync = !selectedExam.settings.handCursorSync">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.handCursorSync ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- SECTION 7: СИСТЕМНЫЙ КОНТРОЛЬ (KERNEL-LEVEL) -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-cpu" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Системный контроль (Kernel-Level)</h3>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Контроль процессов</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-сканирование всех процессов ОС на несанкционированное ПО</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.processScanning ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.processScanning = !selectedExam.settings.processScanning">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.processScanning ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция внешних устройств</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Блокировка USB-захвата, HDMI-карт на уровне ядра</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.hardwareDeviceDetection ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.hardwareDeviceDetection = !selectedExam.settings.hardwareDeviceDetection">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.hardwareDeviceDetection ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Удалённый доступ (Advanced)</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Блокировка TeamViewer, AnyDesk, VNC + фоновые скрипты</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.advancedRemoteBlock ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.advancedRemoteBlock = !selectedExam.settings.advancedRemoteBlock">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.advancedRemoteBlock ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Hardware ID Binding</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Привязка к CPU/Motherboard ID — защита от подмены устройства</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.hardwareIdBinding ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.hardwareIdBinding = !selectedExam.settings.hardwareIdBinding">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.hardwareIdBinding ? '22px' : '2px' }" />
                  </button>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Deep Multi-Monitor Check</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">100% детекция физических и виртуальных мониторов</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.deepMultiMonitorCheck ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.deepMultiMonitorCheck = !selectedExam.settings.deepMultiMonitorCheck">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.deepMultiMonitorCheck ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>

              <!-- Performance & Resilience -->
              <div class="space-y-1 pt-2">
                <div class="flex items-center gap-2 mb-2">
                  <UIcon name="i-lucide-gauge" class="size-4" style="color: var(--argus-accent);" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Производительность</h3>
                </div>

                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium" style="color: var(--argus-text);">Принудительный Low-Spec режим</p>
                    <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Отключает анимации и эффекты для слабых устройств</p>
                  </div>
                  <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: selectedExam.settings.forceLowSpecMode ? 'var(--argus-accent)' : 'var(--argus-border)' }" @click="selectedExam.settings.forceLowSpecMode = !selectedExam.settings.forceLowSpecMode">
                    <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: selectedExam.settings.forceLowSpecMode ? '22px' : '2px' }" />
                  </button>
                </div>
              </div>
            </div>

            <!-- ============================== -->
            <!-- TAB: EXCEPTIONS (ENHANCED)     -->
            <!-- ============================== -->
            <div v-if="activeTab === 'exceptions'" class="flex-1 overflow-y-auto px-6 py-5 space-y-5">
              <!-- ===== PARTICIPANT SEARCH & SELECTION ===== -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon name="i-lucide-search" class="size-4" style="color: #3182CE;" />
                  <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Поиск участника</h3>
                  <span class="text-[9px]" style="color: var(--argus-text-dimmed);">— выберите студента для персональных настроек</span>
                </div>

                <!-- Search bar with dropdown -->
                <div class="relative">
                  <div
                    v-if="!excSelectedStudent"
                    class="flex items-center gap-2 px-3 py-2.5 rounded-lg transition-all"
                    :style="{ background: 'var(--argus-bg-elevated)', border: `1px solid ${excDropdownOpen ? '#3182CE' : 'var(--argus-border)'}`, boxShadow: excDropdownOpen ? '0 0 0 3px rgba(49, 130, 206, 0.1)' : 'none' }"
                  >
                    <UIcon name="i-lucide-search" class="size-3.5 shrink-0" style="color: var(--argus-text-dimmed);" />
                    <input
                      v-model="excParticipantQuery"
                      type="text"
                      placeholder="Поиск по ФИО, ИИН или телефону..."
                      class="flex-1 bg-transparent text-xs font-medium outline-none placeholder:text-[var(--argus-text-dimmed)]"
                      :style="{ color: 'var(--argus-text)' }"
                      @focus="excDropdownOpen = true"
                      @blur="setTimeout(() => excDropdownOpen = false, 200)"
                    >
                  </div>

                  <!-- Selected student chip — enhanced with Custom Settings label & Manual Override badge -->
                  <div
                    v-else
                    class="rounded-xl border overflow-hidden"
                    :style="{ borderColor: 'rgba(49, 130, 206, 0.2)' }"
                  >
                    <!-- Student info row -->
                    <div
                      class="flex items-center justify-between gap-3 px-4 py-3"
                      :style="{ background: 'rgba(49, 130, 206, 0.06)' }"
                    >
                      <div class="flex items-center gap-3">
                        <div class="flex items-center justify-center size-9 rounded-lg" style="background: rgba(49, 130, 206, 0.12);">
                          <UIcon name="i-lucide-user" class="size-4" style="color: #3182CE;" />
                        </div>
                        <div>
                          <div class="flex items-center gap-2">
                            <p class="text-xs font-bold" style="color: var(--argus-text);">{{ excSelectedStudent.name }}</p>
                            <!-- Manual Override badge (if this student already has saved exceptions) -->
                            <span
                              v-if="excHasExistingException"
                              class="inline-flex items-center gap-1 text-[8px] font-bold px-1.5 py-0.5 rounded"
                              style="background: rgba(230, 126, 34, 0.12); color: #E67E22;"
                            >
                              <UIcon name="i-lucide-pen-line" class="size-2.5" />
                              Manual Override
                            </span>
                          </div>
                          <p class="text-[10px] font-mono" style="color: var(--argus-text-dimmed);">{{ excSelectedStudent.iin }} · {{ excSelectedStudent.phone }}</p>
                        </div>
                      </div>
                      <div class="flex items-center gap-2">
                        <span v-if="excDiffCount > 0" class="text-[9px] font-bold px-2 py-0.5 rounded-full" style="background: rgba(49, 130, 206, 0.12); color: #3182CE;">
                          {{ excDiffCount }} отличий
                        </span>
                        <span v-else class="text-[9px] font-medium px-2 py-0.5 rounded-full" :style="{ background: successBg(0.1), color: 'var(--argus-success)' }">
                          Глобальные
                        </span>
                        <button
                          class="flex items-center justify-center size-7 rounded-lg transition-colors cursor-pointer"
                          style="color: var(--argus-text-dimmed);"
                          @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.1)"
                          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                          @click="clearParticipant"
                        >
                          <UIcon name="i-lucide-x" class="size-3.5" />
                        </button>
                      </div>
                    </div>
                    <!-- Custom Settings label + Reset to Global row -->
                    <div
                      class="flex items-center justify-between px-4 py-2 border-t"
                      :style="{ borderColor: 'rgba(49, 130, 206, 0.1)', background: 'rgba(49, 130, 206, 0.03)' }"
                    >
                      <div class="flex items-center gap-1.5">
                        <UIcon name="i-lucide-sliders-horizontal" class="size-3" style="color: #3182CE;" />
                        <span class="text-[10px] font-semibold" style="color: #3182CE;">Custom Settings</span>
                        <span v-if="excDiffCount > 0" class="text-[9px]" style="color: var(--argus-text-dimmed);">
                          — {{ excDiffCount }} параметров отличаются от глобальных
                        </span>
                      </div>
                      <button
                        v-if="excDiffCount > 0"
                        class="flex items-center gap-1 px-2 py-1 rounded-md text-[9px] font-bold transition-all cursor-pointer"
                        style="color: var(--argus-text-dimmed);"
                        @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-elevated)'; ($event.currentTarget as HTMLElement).style.color = 'var(--argus-text)'"
                        @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'; ($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-dimmed)'"
                        @click="resetToGlobal"
                      >
                        <UIcon name="i-lucide-rotate-ccw" class="size-3" />
                        Reset to Global
                      </button>
                    </div>
                  </div>

                  <!-- Dropdown results -->
                  <div
                    v-if="excDropdownOpen && !excSelectedStudent && excParticipantResults.length > 0"
                    class="absolute top-full left-0 right-0 z-20 mt-1 rounded-xl border overflow-hidden"
                    :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)', boxShadow: '0 8px 32px rgba(0,0,0,0.3)' }"
                  >
                    <div class="max-h-60 overflow-y-auto">
                      <button
                        v-for="student in excParticipantResults"
                        :key="student.id"
                        class="flex items-center gap-3 w-full px-4 py-3 text-left transition-colors cursor-pointer"
                        style="border-bottom: 1px solid var(--argus-border-subtle);"
                        @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                        @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                        @mousedown.prevent="selectParticipant({ id: student.id, name: student.name, iin: student.iin, phone: student.phone })"
                      >
                        <div class="flex items-center justify-center size-8 rounded-lg shrink-0" :style="{ background: student.status === 'flagged' ? errorBg(0.08) : student.status === 'clean' ? successBg(0.08) : accentBg(0.08) }">
                          <UIcon name="i-lucide-user" class="size-3.5" :style="{ color: student.status === 'flagged' ? 'var(--argus-error)' : student.status === 'clean' ? 'var(--argus-success)' : 'var(--argus-accent)' }" />
                        </div>
                        <div class="flex-1 min-w-0">
                          <p class="text-xs font-semibold truncate" style="color: var(--argus-text);">{{ student.name }}</p>
                          <p class="text-[10px] font-mono" style="color: var(--argus-text-dimmed);">{{ student.iin }} · {{ student.phone }}</p>
                        </div>
                        <span class="text-[9px] font-bold px-1.5 py-0.5 rounded-full shrink-0" :style="{ background: student.integrityScore >= 70 ? successBg(0.1) : student.integrityScore >= 50 ? warningBg(0.1) : errorBg(0.1), color: student.integrityScore >= 70 ? 'var(--argus-success)' : student.integrityScore >= 50 ? 'var(--argus-warning)' : 'var(--argus-error)' }">
                          {{ student.integrityScore }}%
                        </span>
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <!-- ===== MIRRORED SETTINGS (shown when student selected) ===== -->
              <template v-if="excSelectedStudent">
                <!-- Reason input -->
                <div>
                  <label class="text-[10px] font-semibold block mb-1.5" style="color: var(--argus-text-dimmed);">Причина исключения</label>
                  <input
                    v-model="excReason"
                    type="text"
                    placeholder="Медицинские показания, техническая причина..."
                    class="w-full px-3 py-2.5 rounded-lg text-xs outline-none transition-all"
                    :style="{ background: 'var(--argus-bg-elevated)', color: 'var(--argus-text)', border: '1px solid var(--argus-border)' }"
                  >
                </div>

                <!-- SECTION 1: ВИДЕО-ПРАВИЛА (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-video" class="size-4" style="color: #3182CE;" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Видео-правила</h3>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Требовать боковую камеру</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Подключение мобильного телефона как боковой камеры</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.requireSideCamera ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.requireSideCamera = !excSettings.requireSideCamera">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.requireSideCamera ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Верификация лица (Face ID)</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Проверка личности перед началом экзамена</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.faceVerification ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.faceVerification = !excSettings.faceVerification">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.faceVerification ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция телефона</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-обнаружение мобильных устройств в кадре</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.objectDetectionPhone ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.objectDetectionPhone = !excSettings.objectDetectionPhone">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.objectDetectionPhone ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция посторонних лиц</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Обнаружение дополнительных людей в кадре камеры</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.objectDetectionPerson ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.objectDetectionPerson = !excSettings.objectDetectionPerson">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.objectDetectionPerson ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Dynamic Face ID</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Периодическая повторная проверка лица во время экзамена</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.dynamicFaceRecheck ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.dynamicFaceRecheck = !excSettings.dynamicFaceRecheck">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.dynamicFaceRecheck ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Anti-Spoofing</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Защита от подмены лица (фото, видео, маска)</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.antiSpoofing ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.antiSpoofing = !excSettings.antiSpoofing">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.antiSpoofing ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Room Scan 360°</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Полный осмотр помещения перед началом экзамена</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.roomScan360 ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.roomScan360 = !excSettings.roomScan360">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.roomScan360 ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SECTION 2: ЧУВСТВИТЕЛЬНОСТЬ ИИ (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-brain" class="size-4" style="color: #3182CE;" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Чувствительность ИИ</h3>
                  </div>

                  <div class="py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div class="flex items-center justify-between mb-2">
                      <div>
                        <p class="text-xs font-medium" style="color: var(--argus-text);">Порог детекции голоса</p>
                        <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Чувствительность к фоновому шуму (0% — выкл., 100% — макс.)</p>
                      </div>
                      <span class="text-xs font-bold tabular-nums" :style="{ color: sensitivityColor(excSettings.voiceDetectionThreshold) }">
                        {{ excSettings.voiceDetectionThreshold }}%
                      </span>
                    </div>
                    <input v-model.number="excSettings.voiceDetectionThreshold" type="range" min="0" max="100" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, #3182CE ${excSettings.voiceDetectionThreshold}%, var(--argus-border) ${excSettings.voiceDetectionThreshold}%)` }">
                    <div class="flex items-center justify-between mt-1">
                      <span class="text-[9px]" style="color: var(--argus-text-dimmed);">0% Выкл.</span>
                      <span class="text-[9px]" style="color: var(--argus-text-dimmed);">100% Макс.</span>
                    </div>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Отслеживание взгляда</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-мониторинг направления взгляда студента</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.gazeTracking ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.gazeTracking = !excSettings.gazeTracking">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.gazeTracking ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div v-if="excSettings.gazeTracking" class="py-3">
                    <div class="flex items-center justify-between mb-2">
                      <div>
                        <p class="text-xs font-medium" style="color: var(--argus-text);">Чувствительность взгляда</p>
                        <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Порог срабатывания при отклонении взгляда</p>
                      </div>
                      <span class="text-xs font-bold tabular-nums" :style="{ color: sensitivityColor(excSettings.gazeSensitivity) }">
                        {{ excSettings.gazeSensitivity }}% · {{ sensitivityLabel(excSettings.gazeSensitivity) }}
                      </span>
                    </div>
                    <input v-model.number="excSettings.gazeSensitivity" type="range" min="10" max="95" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, #3182CE ${(excSettings.gazeSensitivity - 10) / 85 * 100}%, var(--argus-border) ${(excSettings.gazeSensitivity - 10) / 85 * 100}%)` }">
                    <div class="flex items-center justify-between mt-1">
                      <span class="text-[9px]" style="color: var(--argus-text-dimmed);">Низкая</span>
                      <span class="text-[9px]" style="color: var(--argus-text-dimmed);">Высокая</span>
                    </div>
                  </div>

                  <div v-if="excSettings.gazeTracking" class="py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div class="flex items-center justify-between mb-2">
                      <div>
                        <p class="text-xs font-medium" style="color: var(--argus-text);">Лимит отклонения взгляда</p>
                        <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Максимальное время отклонения взгляда до срабатывания (сек)</p>
                      </div>
                      <span class="text-xs font-bold tabular-nums px-2 py-0.5 rounded" :style="{ background: excSettings.gazeDeviationLimitSec <= 5 ? errorBg(0.1) : excSettings.gazeDeviationLimitSec <= 10 ? warningBg(0.1) : 'var(--argus-bg-hover)', color: excSettings.gazeDeviationLimitSec <= 5 ? 'var(--argus-error)' : excSettings.gazeDeviationLimitSec <= 10 ? 'var(--argus-warning)' : 'var(--argus-text-muted)' }">
                        {{ excSettings.gazeDeviationLimitSec }} сек
                      </span>
                    </div>
                    <input v-model.number="excSettings.gazeDeviationLimitSec" type="range" min="3" max="30" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, #3182CE ${(excSettings.gazeDeviationLimitSec - 3) / 27 * 100}%, var(--argus-border) ${(excSettings.gazeDeviationLimitSec - 3) / 27 * 100}%)` }">
                    <div class="flex items-center justify-between mt-1">
                      <span class="text-[9px]" style="color: var(--argus-error);">3 сек</span>
                      <span class="text-[9px]" style="color: var(--argus-text-dimmed);">30 сек</span>
                    </div>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция голосовой активности</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-распознавание речи и разговоров во время экзамена</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.voiceActivityDetection ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.voiceActivityDetection = !excSettings.voiceActivityDetection">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.voiceActivityDetection ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция периферийных звуков</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-анализ периферийных звуков (шёпот, наушники, второе устройство)</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.audioPeripheryDetection ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.audioPeripheryDetection = !excSettings.audioPeripheryDetection">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.audioPeripheryDetection ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Умный шумоподавитель</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-классификация источника звука и интеллектуальная фильтрация</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.smartNoiseFilter ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.smartNoiseFilter = !excSettings.smartNoiseFilter">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.smartNoiseFilter ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SECTION 3: ПСИХОМЕТРИЯ И AI-АНАЛИТИКА (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-scan-face" class="size-4" style="color: #3182CE;" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Психометрия и AI-аналитика</h3>
                    <span class="text-[8px] font-bold px-1.5 py-0.5 rounded" style="background: rgba(49, 130, 206, 0.1); color: #3182CE;">NEW</span>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Анализ эмоций и стресса</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-анализ микроэкспрессий: стресс, уверенность, замешательство</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.emotionStressAnalysis ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.emotionStressAnalysis = !excSettings.emotionStressAnalysis">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.emotionStressAnalysis ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Скоринг потери фокуса</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Непрерывный AI-скоринг концентрации по мимике и движениям</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.focusLossScore ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.focusLossScore = !excSettings.focusLossScore">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.focusLossScore ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Анализ паттерна моргания</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Детекция аномального моргания (чтение с экрана, подсказки)</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blinkPatternAnalysis ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.blinkPatternAnalysis = !excSettings.blinkPatternAnalysis">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blinkPatternAnalysis ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SECTION 4: ОГРАНИЧЕНИЯ БРАУЗЕРА (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-monitor" class="size-4" style="color: #3182CE;" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Ограничения браузера</h3>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Принудительный полный экран</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Автоматический переход в полноэкранный режим при старте и запрет выхода до завершения</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.forceFullscreen ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.forceFullscreen = !excSettings.forceFullscreen">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.forceFullscreen ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция выхода из полноэкранного режима</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Мгновенная фиксация попытки свернуть браузер или переключиться на другое приложение</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.fullscreenExitDetection ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.fullscreenExitDetection = !excSettings.fullscreenExitDetection">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.fullscreenExitDetection ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Мониторинг внешних дисплеев (Web-level)</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Обнаружение и запрет прохождения экзамена при подключении второго монитора через Browser API</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.webDisplayMonitoring ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.webDisplayMonitoring = !excSettings.webDisplayMonitoring">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.webDisplayMonitoring ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div class="flex items-center justify-between mb-2">
                      <div>
                        <p class="text-xs font-medium" style="color: var(--argus-text);">Лимит переключения вкладок</p>
                        <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Максимум переключений до авто-прерывания</p>
                      </div>
                      <span class="text-xs font-bold tabular-nums px-2 py-0.5 rounded" :style="{ background: excSettings.tabSwitchingLimit === 0 ? errorBg(0.1) : excSettings.tabSwitchingLimit <= 2 ? warningBg(0.1) : 'var(--argus-bg-hover)', color: excSettings.tabSwitchingLimit === 0 ? 'var(--argus-error)' : excSettings.tabSwitchingLimit <= 2 ? 'var(--argus-warning)' : 'var(--argus-text-muted)' }">
                        {{ excSettings.tabSwitchingLimit === 0 ? 'Запрещено' : excSettings.tabSwitchingLimit }}
                      </span>
                    </div>
                    <input v-model.number="excSettings.tabSwitchingLimit" type="range" min="0" max="10" class="w-full h-1.5 rounded-full appearance-none cursor-pointer" :style="{ background: `linear-gradient(to right, #3182CE ${excSettings.tabSwitchingLimit / 10 * 100}%, var(--argus-border) ${excSettings.tabSwitchingLimit / 10 * 100}%)` }">
                    <div class="flex items-center justify-between mt-1">
                      <span class="text-[9px]" style="color: var(--argus-error);">Запрещено</span>
                      <span class="text-[9px]" style="color: var(--argus-text-dimmed);">10 раз</span>
                    </div>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка буфера обмена</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить копирование и вставку</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blockCopyPaste ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.blockCopyPaste = !excSettings.blockCopyPaste">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blockCopyPaste ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка PrintScreen</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить создание скриншотов экрана</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blockPrintScreen ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.blockPrintScreen = !excSettings.blockPrintScreen">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blockPrintScreen ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка виртуальных машин</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить прохождение экзамена в виртуальной среде</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blockVirtualMachine ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.blockVirtualMachine = !excSettings.blockVirtualMachine">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blockVirtualMachine ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка мульти-десктопа</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить использование нескольких рабочих столов</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blockMultiDesktop ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.blockMultiDesktop = !excSettings.blockMultiDesktop">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blockMultiDesktop ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка удалённого доступа</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Запретить TeamViewer, AnyDesk и другие программы</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blockRemoteAccess ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.blockRemoteAccess = !excSettings.blockRemoteAccess">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blockRemoteAccess ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Блокировка контекстного меню</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Блокировка правой кнопки мыши и контекстного меню</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.blockContextMenu ? 'var(--argus-error)' : 'var(--argus-border)' }" @click="excSettings.blockContextMenu = !excSettings.blockContextMenu">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.blockContextMenu ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SECTION 5: СЕТЕВОЙ КОНТРОЛЬ (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-wifi" class="size-4" style="color: #3182CE;" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Сетевой контроль</h3>
                    <span class="text-[8px] font-bold px-1.5 py-0.5 rounded" style="background: rgba(49, 130, 206, 0.1); color: #3182CE;">NEW</span>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Обнаружение VPN/Proxy</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-обнаружение VPN, прокси-серверов и Tor-подключений</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.vpnProxyDetection ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.vpnProxyDetection = !excSettings.vpnProxyDetection">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.vpnProxyDetection ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Сканирование локальной сети</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Сканирование локальной сети на подозрительные устройства и подключения</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.localNetworkScan ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.localNetworkScan = !excSettings.localNetworkScan">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.localNetworkScan ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SECTION 6: ПОВЕДЕНЧЕСКИЙ АНАЛИЗ (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-brain-circuit" class="size-4" style="color: var(--argus-accent);" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Поведенческий анализ</h3>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <p class="text-xs font-medium" style="color: var(--argus-text);">Динамика набора текста</p>
                        <span class="shrink-0 px-1.5 py-px rounded text-[7px] font-bold uppercase tracking-wide" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }">Kernel-данные</span>
                      </div>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Анализ биометрического почерка клавиатурного ввода: WPM, латентность между нажатиями, ритм набора</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer shrink-0 ml-3" :style="{ background: excSettings.typingDynamics ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.typingDynamics = !excSettings.typingDynamics">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.typingDynamics ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <p class="text-xs font-medium" style="color: var(--argus-text);">Синхронизация руки и курсора</p>
                        <span class="shrink-0 px-1.5 py-px rounded text-[7px] font-bold uppercase tracking-wide" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }">Kernel-данные</span>
                      </div>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Корреляция физических движений руки с перемещением курсора для детекции удалённого управления</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer shrink-0 ml-3" :style="{ background: excSettings.handCursorSync ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.handCursorSync = !excSettings.handCursorSync">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.handCursorSync ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SECTION 7: СИСТЕМНЫЙ КОНТРОЛЬ (KERNEL-LEVEL) (mirrored) -->
                <div>
                  <div class="flex items-center gap-2 mb-3">
                    <UIcon name="i-lucide-cpu" class="size-4" style="color: #3182CE;" />
                    <h3 class="text-sm font-semibold" style="color: var(--argus-text);">Системный контроль (Kernel-Level)</h3>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Контроль процессов</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">AI-сканирование всех процессов ОС на несанкционированное ПО</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.processScanning ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.processScanning = !excSettings.processScanning">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.processScanning ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Детекция внешних устройств</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Блокировка USB-захвата, HDMI-карт на уровне ядра</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.hardwareDeviceDetection ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.hardwareDeviceDetection = !excSettings.hardwareDeviceDetection">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.hardwareDeviceDetection ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Удалённый доступ (Advanced)</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Блокировка TeamViewer, AnyDesk, VNC + фоновые скрипты</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.advancedRemoteBlock ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.advancedRemoteBlock = !excSettings.advancedRemoteBlock">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.advancedRemoteBlock ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3 border-b" style="border-color: var(--argus-border-subtle);">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Hardware ID Binding</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">Привязка к CPU/Motherboard ID — защита от подмены устройства</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.hardwareIdBinding ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.hardwareIdBinding = !excSettings.hardwareIdBinding">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.hardwareIdBinding ? '22px' : '2px' }" />
                    </button>
                  </div>

                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium" style="color: var(--argus-text);">Deep Multi-Monitor Check</p>
                      <p class="text-xs leading-relaxed" style="color: var(--argus-text-dimmed);">100% детекция физических и виртуальных мониторов</p>
                    </div>
                    <button class="relative w-10 h-5 rounded-full transition-all cursor-pointer" :style="{ background: excSettings.deepMultiMonitorCheck ? '#3182CE' : 'var(--argus-border)' }" @click="excSettings.deepMultiMonitorCheck = !excSettings.deepMultiMonitorCheck">
                      <div class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all" :style="{ left: excSettings.deepMultiMonitorCheck ? '22px' : '2px' }" />
                    </button>
                  </div>
                </div>

                <!-- SAVE BUTTON — Scanner Effect -->
                <div class="sticky bottom-0 pt-4 pb-1" style="background: linear-gradient(to top, var(--argus-bg-card) 80%, transparent);">
                  <button
                    class="exc-scanner-btn w-full py-3 rounded-xl text-sm font-bold text-white cursor-pointer transition-all"
                    @click="saveException"
                  >
                    <span class="relative z-10 flex items-center justify-center gap-2">
                      <UIcon v-if="excSaved" name="i-lucide-check" class="size-4" />
                      <UIcon v-else name="i-lucide-save" class="size-4" />
                      {{ excSaved ? 'Сохранено!' : 'Сохранить исключение' }}
                      <span v-if="excDiffCount > 0 && !excSaved" class="text-[10px] font-medium opacity-80">({{ excDiffCount }} изменений)</span>
                    </span>
                  </button>
                </div>
              </template>

              <!-- ===== EXISTING EXCEPTIONS LIST (enhanced clean view) ===== -->
              <template v-if="!excSelectedStudent">
                <div v-if="selectedExam.exceptions.length > 0" class="space-y-3">
                  <!-- Header with count + filter -->
                  <div class="flex items-center justify-between px-1">
                    <div class="flex items-center gap-2">
                      <UIcon name="i-lucide-users" class="size-3.5" style="color: var(--argus-text-dimmed);" />
                      <span class="text-[10px] font-bold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
                        Активные исключения
                      </span>
                      <span class="text-[10px] font-bold px-1.5 py-0.5 rounded-full" style="background: rgba(230, 126, 34, 0.1); color: #E67E22;">
                        {{ selectedExam.exceptions.length }}
                      </span>
                    </div>
                    <!-- Filter existing exceptions -->
                    <div class="flex items-center gap-1.5 px-2 py-1 rounded-md" :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)' }">
                      <UIcon name="i-lucide-filter" class="size-3" style="color: var(--argus-text-dimmed);" />
                      <input
                        v-model="exceptionSearchQuery"
                        type="text"
                        placeholder="Фильтр..."
                        class="bg-transparent text-[10px] outline-none w-20 placeholder:text-[var(--argus-text-dimmed)]"
                        :style="{ color: 'var(--argus-text)' }"
                      >
                    </div>
                  </div>

                  <!-- Exception cards — clean summary view -->
                  <div
                    v-for="exc in filteredExceptions"
                    :key="exc.id"
                    class="rounded-xl border transition-all cursor-pointer overflow-hidden"
                    :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)' }"
                    @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'rgba(49, 130, 206, 0.3)'; ($event.currentTarget as HTMLElement).style.boxShadow = '0 2px 12px rgba(49, 130, 206, 0.08)'"
                    @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-border)'; ($event.currentTarget as HTMLElement).style.boxShadow = 'none'"
                    @click="selectParticipant({ id: exc.id, name: exc.studentName, iin: exc.studentId, phone: exc.phone || '' })"
                  >
                    <!-- Top row: student info + actions -->
                    <div class="flex items-center gap-3 px-4 py-3">
                      <div class="flex items-center justify-center size-9 rounded-lg shrink-0" style="background: rgba(49, 130, 206, 0.1);">
                        <UIcon :name="exceptionProfileIcon(exc.profile)" class="size-4" style="color: #3182CE;" />
                      </div>
                      <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-2">
                          <p class="text-xs font-semibold truncate" style="color: var(--argus-text);">{{ exc.studentName }}</p>
                          <!-- Manual Override badge -->
                          <span
                            class="inline-flex items-center gap-1 text-[7px] font-bold px-1.5 py-0.5 rounded shrink-0"
                            style="background: rgba(230, 126, 34, 0.12); color: #E67E22;"
                          >
                            <UIcon name="i-lucide-pen-line" class="size-2" />
                            Manual Override
                          </span>
                        </div>
                        <div class="flex items-center gap-1.5 mt-0.5">
                          <p class="text-[10px] font-mono" style="color: var(--argus-text-dimmed);">{{ exc.studentId }}</p>
                          <span v-if="exc.reason" class="text-[8px]" style="color: var(--argus-border);">·</span>
                          <p v-if="exc.reason" class="text-[10px] truncate" style="color: var(--argus-text-muted);">{{ exc.reason }}</p>
                        </div>
                      </div>
                      <div class="flex items-center gap-1.5 shrink-0">
                        <!-- Edit button -->
                        <button
                          class="flex items-center justify-center size-7 rounded-lg transition-colors cursor-pointer"
                          style="color: var(--argus-text-dimmed);"
                          title="Редактировать"
                          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'rgba(49, 130, 206, 0.1)'; ($event.currentTarget as HTMLElement).style.color = '#3182CE'"
                          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'; ($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-dimmed)'"
                        >
                          <UIcon name="i-lucide-pencil" class="size-3" />
                        </button>
                        <!-- Delete button -->
                        <button
                          class="flex items-center justify-center size-7 rounded-lg transition-colors shrink-0 cursor-pointer"
                          style="color: var(--argus-text-dimmed);"
                          title="Удалить исключение"
                          @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.1); ($event.currentTarget as HTMLElement).style.color = 'var(--argus-error)'"
                          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'; ($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-dimmed)'"
                          @click.stop="removeException(exc.id)"
                        >
                          <UIcon name="i-lucide-trash-2" class="size-3" />
                        </button>
                      </div>
                    </div>

                    <!-- Bottom row: Custom Settings summary — clean override pills at a glance -->
                    <div
                      v-if="Object.keys(exc.overrides).length > 0"
                      class="flex items-center gap-2 px-4 py-2 border-t"
                      :style="{ borderColor: 'var(--argus-border-subtle)', background: 'rgba(49, 130, 206, 0.02)' }"
                    >
                      <span class="text-[8px] font-bold uppercase tracking-wider shrink-0" style="color: #3182CE;">
                        Custom Settings
                      </span>
                      <div class="flex flex-wrap gap-1 flex-1">
                        <span
                          v-for="(value, key) in exc.overrides"
                          :key="String(key)"
                          class="inline-flex items-center gap-0.5 text-[8px] font-bold px-1.5 py-0.5 rounded"
                          :style="{
                            background: typeof value === 'boolean' && value ? 'rgba(49, 130, 206, 0.08)' : typeof value === 'boolean' && !value ? errorBg(0.06) : 'rgba(49, 130, 206, 0.08)',
                            color: typeof value === 'boolean' && !value ? 'var(--argus-error)' : '#3182CE'
                          }"
                        >
                          <span
                            class="size-1.5 rounded-full"
                            :style="{ background: typeof value === 'boolean' && !value ? 'var(--argus-error)' : '#3182CE' }"
                          />
                          {{ overrideLabel(String(key)) }}: {{ overrideDisplayValue(value) }}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Empty state -->
                <div v-else class="flex flex-col items-center justify-center py-12 px-4">
                  <div class="flex items-center justify-center size-16 rounded-2xl mb-4" style="background: rgba(49, 130, 206, 0.06);">
                    <UIcon name="i-lucide-user-search" class="size-8" style="color: #3182CE; opacity: 0.35;" />
                  </div>
                  <p class="text-sm font-medium" style="color: var(--argus-text-dimmed);">Нет исключений</p>
                  <p class="text-[10px] mt-1 text-center max-w-xs" style="color: var(--argus-text-dimmed);">
                    Воспользуйтесь поиском выше, чтобы найти участника и настроить индивидуальные параметры прокторинга
                  </p>
                </div>
              </template>
            </div>

            <!-- Footer -->
            <div class="flex items-center justify-between px-6 py-4 border-t shrink-0" style="border-color: var(--argus-border);">
              <div class="flex items-center gap-2">
                <!-- Hydrating indicator -->
                <template v-if="settingsHydrating">
                  <div class="animate-spin rounded-full size-3 border-2 border-t-transparent" style="border-color: var(--argus-accent); border-top-color: transparent;" />
                  <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Загрузка с сервера...</span>
                </template>
                <!-- Error -->
                <template v-else-if="settingsError">
                  <UIcon name="i-lucide-alert-circle" class="size-3.5" style="color: var(--argus-error);" />
                  <span class="text-[10px]" style="color: var(--argus-error);">{{ settingsError }}</span>
                </template>
                <!-- Saved confirmation -->
                <template v-else-if="settingsSaved">
                  <UIcon name="i-lucide-check-circle" class="size-3.5" style="color: var(--argus-success);" />
                  <span class="text-[10px] font-medium" style="color: var(--argus-success);">Сохранено на сервере</span>
                </template>
                <!-- Dirty (unsaved changes) -->
                <template v-else-if="settingsDirty">
                  <UIcon name="i-lucide-circle-dot" class="size-3.5" style="color: var(--argus-warning);" />
                  <span class="text-[10px]" style="color: var(--argus-warning);">Несохранённые изменения</span>
                </template>
                <!-- Default -->
                <template v-else>
                  <UIcon name="i-lucide-cloud-check" class="size-3.5" style="color: var(--argus-text-dimmed);" />
                  <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Синхронизировано с сервером</span>
                </template>
              </div>
              <div class="flex items-center gap-2">
                <button
                  v-if="settingsDirty"
                  class="px-4 py-2 rounded-lg text-xs font-bold transition-all cursor-pointer flex items-center gap-1.5"
                  :style="{ background: 'var(--argus-accent)', color: '#fff', opacity: settingsSaving ? '0.6' : '1' }"
                  :disabled="settingsSaving"
                  @click="saveSettings"
                >
                  <div v-if="settingsSaving" class="animate-spin rounded-full size-3 border-2 border-t-transparent" style="border-color: #fff; border-top-color: transparent;" />
                  <UIcon v-else name="i-lucide-save" class="size-3.5" />
                  {{ settingsSaving ? 'Сохранение...' : 'Сохранить' }}
                </button>
                <button class="px-5 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer" :style="{ background: settingsDirty ? 'var(--argus-bg-hover)' : 'var(--argus-accent)', color: settingsDirty ? 'var(--argus-text)' : '#fff' }" @click="closeSettings">
                  {{ settingsDirty ? 'Закрыть' : 'Готово' }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
/* Scanner effect button for exceptions save */
.exc-scanner-btn {
  position: relative;
  overflow: hidden;
  background: #3182CE;
  border: 1px solid rgba(49, 130, 206, 0.6);
  box-shadow: 0 0 15px rgba(49, 130, 206, 0.15), inset 0 1px 0 rgba(255, 255, 255, 0.1);
}
.exc-scanner-btn::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  width: 50px;
  background: linear-gradient(90deg, transparent, rgba(255, 255, 255, 0.2), transparent);
  animation: exc-scanner-sweep 3s ease-in-out infinite;
}
.exc-scanner-btn:hover {
  background: #2B6CB0;
  box-shadow: 0 0 25px rgba(49, 130, 206, 0.25), inset 0 1px 0 rgba(255, 255, 255, 0.15);
  transform: translateY(-1px);
}
.exc-scanner-btn:hover::before {
  animation: exc-scanner-sweep-fast 1.6s ease-in-out infinite;
}
.exc-scanner-btn:active {
  transform: scale(0.98);
  box-shadow: 0 0 40px rgba(49, 130, 206, 0.3);
}

@keyframes exc-scanner-sweep {
  0% { left: -60px; opacity: 0; }
  10% { opacity: 1; }
  100% { left: calc(100% + 60px); opacity: 0; }
}
@keyframes exc-scanner-sweep-fast {
  0% { left: -60px; opacity: 0; }
  8% { opacity: 1; }
  100% { left: calc(100% + 60px); opacity: 0; }
}
</style>
