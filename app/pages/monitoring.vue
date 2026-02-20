<script setup lang="ts">
import { useAdminAPI, type ActiveSession, type ActiveSessionsResponse } from '~/composables/useAdminAPI'
import { useAuthStore } from '~/stores/useAuthStore'
import { useResilience } from '~/composables/useResilience'

const api = useAdminAPI()
const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()

// --- State ---
const sessions = ref<ActiveSession[]>([])
const stats = ref({ totalActive: 0, totalCritical: 0, totalWarning: 0, totalClean: 0, avgRiskScore: 0 })
const loading = ref(true)
const error = ref<string | null>(null)
const lastUpdated = ref('')
const searchQuery = ref('')
const violationFilter = ref<'all' | 'critical' | 'warning'>('all')
const selectedExamId = ref('')

// --- Action state ---
const actionLoading = ref<Record<string, boolean>>({})
const actionResult = ref<{ sessionId: string; type: 'warn' | 'terminate'; success: boolean; message: string } | null>(null)

// --- Resilience data (for TransportHealthIndicator + ResilienceIndicator) ---
const resilienceData = computed(() => {
  try {
    // The resilience layer is initialized per-session in the student client.
    // On the admin monitoring page, we provide a stub for the indicator components.
    // In production, these would come from the active session's resilience state.
    return null as {
      tier: 'A' | 'B' | 'C'
      isOnline: boolean
      isEffectivelyOffline: boolean
      connectionMessage: string
      queueStats: { pendingEvents: number; pendingSnapshots: number; totalSizeBytes: number; oldestItemAge: number; failedCount: number }
      hasPendingItems: boolean
      healthScore: number
    } | null
  } catch {
    return null
  }
})

// --- Poll timer ---
let pollTimer: ReturnType<typeof setInterval> | null = null

// --- Risk Score helpers ---
function riskColor(score: number): string {
  if (score >= 10) return 'var(--argus-error)'
  if (score >= 3) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

function integrityScore(session: ActiveSession): number {
  // Integrity = 100 - riskScore (clamped 0-100)
  return Math.max(0, Math.min(100, Math.round(100 - session.riskScore)))
}

// integrityColor is auto-imported from useStatusHelpers
function integrityGradientLocal(score: number): string {
  return integrityGradient(score, isDark.value)
}

function violationLevelBorder(level: string): string {
  switch (level) {
    case 'critical': return 'var(--argus-error)'
    case 'warning': return 'var(--argus-warning)'
    default: return 'var(--argus-border)'
  }
}

function eventIcon(type: string): string {
  switch (type) {
    case 'gaze_deviation': return 'i-lucide-eye-off'
    case 'face_not_detected': return 'i-lucide-user-x'
    case 'tab_switch': return 'i-lucide-app-window'
    case 'audio_anomaly': return 'i-lucide-mic-off'
    case 'multiple_persons': return 'i-lucide-users'
    case 'voice_activity': return 'i-lucide-mic'
    case 'print_screen_attempt': return 'i-lucide-camera'
    case 'remote_access_detected': return 'i-lucide-monitor'
    case 'virtual_machine_detected': return 'i-lucide-cpu'
    case 'forbidden_process_detected': return 'i-lucide-shield-alert'
    case 'typing_dynamics_anomaly': return 'i-lucide-keyboard'
    case 'hand_cursor_desync': return 'i-lucide-mouse-pointer'
    default: return 'i-lucide-alert-triangle'
  }
}

function eventLabel(type: string): string {
  switch (type) {
    case 'gaze_deviation': return 'Отклонение взгляда'
    case 'face_not_detected': return 'Лицо не обнаружено'
    case 'tab_switch': return 'Переключение вкладки'
    case 'audio_anomaly': return 'Аудио аномалия'
    case 'multiple_persons': return 'Несколько лиц'
    case 'voice_activity': return 'Голосовая активность'
    case 'print_screen_attempt': return 'Скриншот'
    case 'remote_access_detected': return 'Удалённый доступ'
    case 'virtual_machine_detected': return 'Виртуальная машина'
    case 'forbidden_process_detected': return 'Запрещённый процесс'
    case 'typing_dynamics_anomaly': return 'Динамика набора'
    case 'hand_cursor_desync': return 'Десинхронизация курсора'
    default: return type
  }
}

// Category classification for violations
function eventCategory(type: string): { label: string; color: string } {
  switch (type) {
    case 'typing_dynamics_anomaly':
    case 'hand_cursor_desync':
      return { label: 'Поведенческий', color: 'var(--argus-accent)' }
    case 'gaze_deviation':
    case 'face_not_detected':
    case 'multiple_persons':
      return { label: 'Видео', color: 'var(--argus-info)' }
    case 'audio_anomaly':
    case 'voice_activity':
      return { label: 'Аудио', color: 'var(--argus-warning)' }
    case 'tab_switch':
    case 'print_screen_attempt':
      return { label: 'Браузер', color: 'var(--argus-error)' }
    case 'remote_access_detected':
    case 'virtual_machine_detected':
    case 'forbidden_process_detected':
      return { label: 'Системный', color: 'var(--argus-text-dimmed)' }
    default:
      return { label: 'Другое', color: 'var(--argus-text-dimmed)' }
  }
}

// severityColor is auto-imported from useStatusHelpers

// formatTime is auto-imported from useFormatters

function formatStudentName(studentId: string): string {
  // Format student-NNNNNN into "Студент #NNNNNN"
  const match = studentId.match(/student-(\d+)/)
  if (match) return `Студент #${match[1]}`
  return studentId
}

// --- Fetch data ---
async function fetchActiveSessions() {
  try {
    if (!authStore.isAuthenticated) return

    const data = await api.getActiveSessions()
    sessions.value = data.sessions
    stats.value = {
      totalActive: data.totalActive,
      totalCritical: data.totalCritical,
      totalWarning: data.totalWarning,
      totalClean: data.totalClean,
      avgRiskScore: data.avgRiskScore
    }
    lastUpdated.value = new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
    error.value = null
  } catch (e: any) {
    // Backend offline — show empty state rather than error for monitoring.
    console.info('[Monitoring] Backend unavailable:', e.message)
    if (sessions.value.length === 0) {
      error.value = null
      stats.value = { totalActive: 0, totalCritical: 0, totalWarning: 0, totalClean: 0, avgRiskScore: 0 }
    }
  } finally {
    loading.value = false
  }
}

// --- Filtered sessions ---
const filteredSessions = computed(() => {
  let result = sessions.value

  // Filter by violation level
  if (violationFilter.value !== 'all') {
    result = result.filter(s => s.violationLevel === violationFilter.value)
  }

  // Filter by search query
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    result = result.filter(s =>
      s.studentId.toLowerCase().includes(q) ||
      s.sessionId.toLowerCase().includes(q) ||
      s.examId.toLowerCase().includes(q) ||
      formatStudentName(s.studentId).toLowerCase().includes(q)
    )
  }

  // Filter by exam
  if (selectedExamId.value) {
    result = result.filter(s => s.examId === selectedExamId.value)
  }

  return result
})

// --- Unique exams for filter ---
const examOptions = computed(() => {
  const exams = new Set(sessions.value.map(s => s.examId))
  const options = [{ label: 'Все экзамены', value: '' }]
  for (const examId of exams) {
    options.push({ label: examId, value: examId })
  }
  return options
})

// --- Actions ---
async function handleWarn(session: ActiveSession) {
  const key = `warn-${session.sessionId}`
  actionLoading.value[key] = true
  try {
    await api.warnSession(session.sessionId)
    actionResult.value = {
      sessionId: session.sessionId,
      type: 'warn',
      success: true,
      message: 'Предупреждение отправлено'
    }
    // Auto-clear after 3s
    setTimeout(() => {
      if (actionResult.value?.sessionId === session.sessionId && actionResult.value?.type === 'warn') {
        actionResult.value = null
      }
    }, 3000)
  } catch (e: any) {
    actionResult.value = {
      sessionId: session.sessionId,
      type: 'warn',
      success: false,
      message: e.message || 'Ошибка'
    }
  } finally {
    actionLoading.value[key] = false
  }
}

async function handleTerminate(session: ActiveSession) {
  const key = `terminate-${session.sessionId}`
  actionLoading.value[key] = true
  try {
    await api.terminateSession(session.sessionId)
    actionResult.value = {
      sessionId: session.sessionId,
      type: 'terminate',
      success: true,
      message: 'Сессия завершена'
    }
    // Refresh data
    await fetchActiveSessions()
    setTimeout(() => {
      if (actionResult.value?.sessionId === session.sessionId && actionResult.value?.type === 'terminate') {
        actionResult.value = null
      }
    }, 3000)
  } catch (e: any) {
    actionResult.value = {
      sessionId: session.sessionId,
      type: 'terminate',
      success: false,
      message: e.message || 'Ошибка'
    }
  } finally {
    actionLoading.value[key] = false
  }
}

// --- Lifecycle ---
onMounted(async () => {
  await fetchActiveSessions()
  // 10-second polling
  pollTimer = setInterval(fetchActiveSessions, 10_000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

// --- Search focus ---
const searchFocused = ref(false)

// --- Expanded session ---
const expandedSession = ref<ActiveSession | null>(null)
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1 class="text-2xl font-bold" style="color: var(--argus-text);">
          Мониторинг
        </h1>
        <p class="text-sm mt-1" style="color: var(--argus-text-dimmed);">
          Прямые сессии с AI-анализом — {{ stats.totalActive }} активных сессий
        </p>
      </div>

      <!-- Live Stats + Refresh indicator -->
      <div class="flex items-center gap-3 flex-wrap">
        <!-- Transport & Resilience Indicators -->
        <TransportHealthIndicator />
        <ResilienceIndicator
          v-if="resilienceData"
          :tier="resilienceData.tier"
          :is-online="resilienceData.isOnline"
          :is-effectively-offline="resilienceData.isEffectivelyOffline"
          :connection-message="resilienceData.connectionMessage"
          :queue-stats="resilienceData.queueStats"
          :has-pending-items="resilienceData.hasPendingItems"
          :health-score="resilienceData.healthScore"
        />

        <div class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full" :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }">
          <span class="relative flex size-1.5">
            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
            <span class="relative inline-flex size-1.5 rounded-full bg-red-500" />
          </span>
          {{ stats.totalCritical }} критических
        </div>
        <div class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full" :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }">
          {{ stats.totalWarning }} предупреждений
        </div>
        <div class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full" :style="{ background: successBg(0.1), color: 'var(--argus-success)' }">
          {{ stats.totalClean }} чисто
        </div>
        <div v-if="lastUpdated" class="text-[10px] font-mono px-2 py-1 rounded" :style="{ color: 'var(--argus-text-dimmed)', background: 'var(--argus-bg-elevated)' }">
          Обновлено: {{ lastUpdated }}
        </div>
      </div>
    </div>

    <!-- Filters Row -->
    <div class="glass-card rounded-xl px-4 py-3">
      <div class="flex items-center gap-3">
        <!-- Exam Filter -->
        <div class="w-[20%] min-w-44 shrink-0">
          <USelectMenu
            v-model="selectedExamId"
            :items="examOptions.map(o => ({ label: o.label, value: o.value }))"
            value-key="value"
            placeholder="Все экзамены"
            class="w-full"
          />
        </div>

        <!-- Search Input -->
        <div
          class="flex items-center gap-2.5 px-3.5 py-2 rounded-lg border flex-1 transition-all"
          :style="{
            background: 'var(--argus-bg-elevated)',
            borderColor: searchFocused ? 'var(--argus-accent)' : 'var(--argus-border)',
            boxShadow: searchFocused ? `0 0 0 3px ${accentBg(0.1)}` : 'none'
          }"
        >
          <UIcon name="i-heroicons-magnifying-glass" class="size-4 shrink-0" :style="{ color: searchFocused ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)' }" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Поиск по ID студента, сессии..."
            class="w-full bg-transparent text-sm outline-none"
            :style="{ color: 'var(--argus-text)' }"
            @focus="searchFocused = true"
            @blur="searchFocused = false"
          >
          <span v-if="searchQuery" class="text-[10px] font-medium px-2 py-0.5 rounded-full whitespace-nowrap" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }">
            {{ filteredSessions.length }}
          </span>
        </div>

        <!-- Status Filter Buttons -->
        <div class="flex items-center gap-1.5 shrink-0">
          <button
            v-for="filter in (['all', 'critical', 'warning'] as const)"
            :key="filter"
            class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all border"
            :style="{
              background: violationFilter === filter
                ? (filter === 'critical' ? errorBg(0.12) : filter === 'warning' ? warningBg(0.12) : accentBg(0.1))
                : 'transparent',
              borderColor: violationFilter === filter
                ? (filter === 'critical' ? errorBg(0.35) : filter === 'warning' ? warningBg(0.35) : accentBg(0.25))
                : 'var(--argus-border)',
              color: violationFilter === filter
                ? (filter === 'critical' ? 'var(--argus-error)' : filter === 'warning' ? 'var(--argus-warning)' : 'var(--argus-accent)')
                : 'var(--argus-text-dimmed)'
            }"
            @click="violationFilter = filter"
          >
            <UIcon
              :name="filter === 'critical' ? 'i-heroicons-shield-exclamation' : filter === 'warning' ? 'i-lucide-alert-triangle' : 'i-lucide-layout-grid'"
              class="size-3.5"
            />
            {{ filter === 'all' ? 'Все' : filter === 'critical' ? 'Критические' : 'Предупреждения' }}
            <span
              class="text-[10px] font-bold px-1.5 py-0.5 rounded-full ml-0.5"
              :style="{
                background: filter === 'critical' ? errorBg(0.15) : filter === 'warning' ? warningBg(0.15) : accentBg(0.1),
                color: filter === 'critical' ? 'var(--argus-error)' : filter === 'warning' ? 'var(--argus-warning)' : 'var(--argus-accent)'
              }"
            >
              {{ filter === 'all' ? stats.totalActive : filter === 'critical' ? stats.totalCritical : stats.totalWarning }}
            </span>
          </button>
        </div>
      </div>
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="flex flex-col items-center justify-center py-20">
      <div class="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 mb-4" style="border-color: var(--argus-accent);" />
      <p class="text-sm" style="color: var(--argus-text-dimmed);">Загрузка активных сессий...</p>
    </div>

    <!-- Error state -->
    <div v-else-if="error" class="flex flex-col items-center justify-center py-20">
      <UIcon name="i-lucide-alert-circle" class="size-12 mb-3" style="color: var(--argus-error);" />
      <p class="text-sm font-medium" style="color: var(--argus-text);">{{ error }}</p>
      <button
        class="mt-4 px-4 py-2 rounded-lg text-xs font-medium transition-all"
        :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
        @click="loading = true; fetchActiveSessions()"
      >
        Повторить
      </button>
    </div>

    <!-- Session Cards Grid -->
    <div v-else class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-4">
      <div
        v-for="session in filteredSessions"
        :key="session.sessionId"
        class="glass-card rounded-xl overflow-hidden video-card-hover cursor-pointer transition-all duration-300"
        :class="[
          session.violationLevel === 'critical' ? 'pulse-critical' : session.violationLevel === 'warning' ? 'pulse-warning' : '',
          session.status === 'terminated' ? 'opacity-50' : ''
        ]"
        :style="{
          borderColor: violationLevelBorder(session.violationLevel),
          borderWidth: session.violationLevel === 'critical' ? '2px' : '1px'
        }"
        @click="expandedSession = session"
      >
        <!-- Video Feed (LiveKit WebRTC) -->
        <div class="relative aspect-video overflow-hidden" style="background: var(--argus-bg-deep);">
          <!-- VideoPlayer component replaces static placeholder -->
          <VideoPlayer
            :session-id="session.sessionId"
            :compact="true"
          />

          <!-- Live / Terminated indicator -->
          <div class="absolute top-2 left-2 flex items-center gap-1.5">
            <div
              class="flex items-center gap-1.5 px-2 py-1 rounded-md"
              :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
            >
              <template v-if="session.status === 'active'">
                <span class="relative flex size-1.5">
                  <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
                  <span class="relative inline-flex size-1.5 rounded-full bg-red-500" />
                </span>
                <span class="text-[9px] font-bold uppercase" style="color: var(--argus-error);">LIVE</span>
              </template>
              <template v-else>
                <UIcon name="i-lucide-ban" class="size-3" style="color: var(--argus-text-dimmed);" />
                <span class="text-[9px] font-bold uppercase" style="color: var(--argus-text-dimmed);">ЗАВЕРШЕНА</span>
              </template>
            </div>
          </div>

          <!-- Risk Score Badge (top-right) -->
          <div
            class="absolute top-2 right-2 px-2 py-1 rounded-md"
            :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
          >
            <span class="text-[11px] font-bold tabular-nums" :style="{ color: integrityColor(integrityScore(session)) }">
              {{ integrityScore(session) }}%
            </span>
          </div>

          <!-- Violation counts (bottom-left) -->
          <div class="absolute bottom-2 left-2 flex items-center gap-1">
            <div
              v-if="session.criticalCount > 0"
              class="flex items-center gap-1 px-1.5 py-0.5 rounded"
              :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
            >
              <UIcon name="i-lucide-shield-alert" class="size-2.5" style="color: var(--argus-error);" />
              <span class="text-[8px] font-bold" style="color: var(--argus-error);">{{ session.criticalCount }}</span>
            </div>
            <div
              v-if="session.warningCount > 0"
              class="flex items-center gap-1 px-1.5 py-0.5 rounded"
              :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
            >
              <UIcon name="i-lucide-alert-triangle" class="size-2.5" style="color: var(--argus-warning);" />
              <span class="text-[8px] font-bold" style="color: var(--argus-warning);">{{ session.warningCount }}</span>
            </div>
          </div>

          <!-- Risk Score % (bottom-right) -->
          <div
            class="absolute bottom-2 right-2 px-1.5 py-0.5 rounded"
            :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
          >
            <span class="text-[8px] font-bold" :style="{ color: riskColor(session.riskScore) }">
              Риск: {{ session.riskScore.toFixed(1) }}%
            </span>
          </div>
        </div>

        <!-- Card Footer -->
        <div class="p-3 space-y-2">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0 flex-1">
              <p class="text-sm font-semibold truncate" style="color: var(--argus-text);">
                {{ formatStudentName(session.studentId) }}
              </p>
              <p class="text-[10px] mt-0.5" style="color: var(--argus-text-dimmed);">
                Сессия: {{ session.sessionId.slice(0, 20) }}...
              </p>
            </div>
            <span
              v-if="session.criticalCount + session.warningCount > 0"
              class="text-[10px] font-bold px-2 py-0.5 rounded-full shrink-0"
              :style="{
                background: session.violationLevel === 'critical' ? errorBg(0.1) : warningBg(0.1),
                color: session.violationLevel === 'critical' ? 'var(--argus-error)' : 'var(--argus-warning)'
              }"
            >
              {{ session.criticalCount + session.warningCount }} нар.
            </span>
          </div>

          <!-- Integrity bar -->
          <div class="flex items-center gap-2">
            <div class="flex-1 h-1 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
              <div
                class="h-full rounded-full transition-all duration-700"
                :style="{ width: `${integrityScore(session)}%`, background: integrityGradientLocal(integrityScore(session)) }"
              />
            </div>
          </div>

          <!-- Recent violations preview (last 2) -->
          <div v-if="session.recentViolations.length > 0" class="space-y-0.5">
            <div
              v-for="v in session.recentViolations.slice(0, 2)"
              :key="v.eventId"
              class="flex items-center gap-1.5 text-[9px] py-0.5"
            >
              <UIcon :name="eventIcon(v.eventType)" class="size-2.5 shrink-0" :style="{ color: severityColor(v.severity) }" />
              <span class="truncate" :style="{ color: severityColor(v.severity) }">{{ eventLabel(v.eventType) }}</span>
              <span
                v-if="eventCategory(v.eventType).label === 'Поведенческий'"
                class="shrink-0 px-1 py-px rounded text-[7px] font-bold uppercase"
                :style="{ background: 'rgba(244,114,182,0.15)', color: eventCategory(v.eventType).color }"
              >ПА</span>
              <span class="ml-auto shrink-0 font-mono" style="color: var(--argus-text-dimmed);">{{ formatTime(v.timestamp) }}</span>
            </div>
          </div>

          <!-- Action buttons -->
          <div class="flex items-center gap-1.5 pt-1">
            <!-- Action result toast -->
            <div
              v-if="actionResult && actionResult.sessionId === session.sessionId"
              class="text-[9px] font-medium px-2 py-0.5 rounded mr-auto"
              :style="{ background: actionResult.success ? successBg(0.15) : errorBg(0.15), color: actionResult.success ? 'var(--argus-success)' : 'var(--argus-error)' }"
            >
              {{ actionResult.message }}
            </div>

            <button
              v-if="session.status === 'active'"
              class="flex items-center gap-1 px-2 py-1 rounded-md text-[10px] font-medium transition-all"
              :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
              :disabled="actionLoading[`warn-${session.sessionId}`]"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = warningBg(0.2)"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = warningBg(0.1)"
              @click.stop="handleWarn(session)"
            >
              <UIcon v-if="!actionLoading[`warn-${session.sessionId}`]" name="i-lucide-alert-triangle" class="size-3" />
              <span v-else class="size-3 animate-spin rounded-full border border-t-transparent" :style="{ borderColor: 'var(--argus-warning)' }" />
              Предупредить
            </button>
            <button
              v-if="session.status === 'active'"
              class="flex items-center gap-1 px-2 py-1 rounded-md text-[10px] font-medium transition-all"
              :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
              :disabled="actionLoading[`terminate-${session.sessionId}`]"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.2)"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = errorBg(0.1)"
              @click.stop="handleTerminate(session)"
            >
              <UIcon v-if="!actionLoading[`terminate-${session.sessionId}`]" name="i-lucide-ban" class="size-3" />
              <span v-else class="size-3 animate-spin rounded-full border border-t-transparent" :style="{ borderColor: 'var(--argus-error)' }" />
              Завершить
            </button>
            <span
              v-if="session.status === 'terminated'"
              class="text-[10px] font-bold px-2 py-1 rounded-md"
              :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }"
            >
              ЗАВЕРШЕНА
            </span>
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-if="filteredSessions.length === 0 && !loading"
        class="col-span-full flex flex-col items-center justify-center py-16"
      >
        <UIcon name="i-lucide-video-off" class="size-12 mb-3" style="color: var(--argus-text-dimmed);" />
        <p class="text-sm font-medium" style="color: var(--argus-text);">Нет активных сессий</p>
        <p class="text-xs mt-1" style="color: var(--argus-text-dimmed);">
          {{ sessions.length > 0 ? 'Измените критерии поиска или сбросьте фильтр' : 'Запустите нагрузочный тест для генерации сессий' }}
        </p>
        <button
          v-if="sessions.length > 0"
          class="mt-4 px-4 py-2 rounded-lg text-xs font-medium transition-all"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          @click="violationFilter = 'all'; searchQuery = ''; selectedExamId = ''"
        >
          Сбросить фильтры
        </button>
      </div>
    </div>

    <!-- ===== EXPANDED SESSION DETAIL MODAL ===== -->
    <UModal v-if="expandedSession" :open="!!expandedSession" :close="false" @update:open="(v: boolean) => { if (!v) expandedSession = null }">
      <template #content>
        <div class="p-6 space-y-4 max-w-lg mx-auto">
          <div class="flex items-start justify-between">
            <div>
              <h3 class="text-lg font-bold" style="color: var(--argus-text);">
                {{ formatStudentName(expandedSession.studentId) }}
              </h3>
              <p class="text-xs mt-1" style="color: var(--argus-text-dimmed);">
                Сессия: {{ expandedSession.sessionId }}
              </p>
              <p class="text-xs" style="color: var(--argus-text-dimmed);">
                Экзамен: {{ expandedSession.examId }} · Орг: {{ expandedSession.orgId }}
              </p>
            </div>
            <button @click="expandedSession = null" class="p-1.5 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-800">
              <UIcon name="i-lucide-x" class="size-5" style="color: var(--argus-text-dimmed);" />
            </button>
          </div>

          <!-- Stats grid -->
          <div class="grid grid-cols-4 gap-3">
            <div class="text-center p-3 rounded-lg" :style="{ background: 'var(--argus-bg-elevated)' }">
              <div class="text-xl font-bold tabular-nums" style="color: var(--argus-text);">{{ expandedSession.totalEvents }}</div>
              <div class="text-[10px]" style="color: var(--argus-text-dimmed);">Событий</div>
            </div>
            <div class="text-center p-3 rounded-lg" :style="{ background: errorBg(0.05) }">
              <div class="text-xl font-bold tabular-nums" style="color: var(--argus-error);">{{ expandedSession.criticalCount }}</div>
              <div class="text-[10px]" style="color: var(--argus-text-dimmed);">Критических</div>
            </div>
            <div class="text-center p-3 rounded-lg" :style="{ background: warningBg(0.05) }">
              <div class="text-xl font-bold tabular-nums" style="color: var(--argus-warning);">{{ expandedSession.warningCount }}</div>
              <div class="text-[10px]" style="color: var(--argus-text-dimmed);">Предупр.</div>
            </div>
            <div class="text-center p-3 rounded-lg" :style="{ background: 'var(--argus-bg-elevated)' }">
              <div class="text-xl font-bold tabular-nums" :style="{ color: riskColor(expandedSession.riskScore) }">{{ expandedSession.riskScore.toFixed(1) }}%</div>
              <div class="text-[10px]" style="color: var(--argus-text-dimmed);">Риск</div>
            </div>
          </div>

          <!-- Timeline -->
          <div class="space-y-1">
            <h4 class="text-xs font-semibold" style="color: var(--argus-text);">Последние нарушения</h4>
            <div
              v-for="v in expandedSession.recentViolations"
              :key="v.eventId"
              class="flex items-center gap-2 px-3 py-2 rounded-lg"
              :style="{ background: 'var(--argus-bg-elevated)' }"
            >
              <UIcon :name="eventIcon(v.eventType)" class="size-4 shrink-0" :style="{ color: severityColor(v.severity) }" />
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-1.5">
                  <p class="text-xs font-medium truncate" :style="{ color: severityColor(v.severity) }">{{ eventLabel(v.eventType) }}</p>
                  <span
                    class="shrink-0 px-1.5 py-px rounded text-[8px] font-bold"
                    :style="{ background: `${eventCategory(v.eventType).color}15`, color: eventCategory(v.eventType).color }"
                  >{{ eventCategory(v.eventType).label }}</span>
                </div>
                <p class="text-[10px] truncate" style="color: var(--argus-text-dimmed);">{{ v.label }}</p>
              </div>
              <div class="text-right shrink-0">
                <span class="text-[10px] font-mono" style="color: var(--argus-text-dimmed);">{{ formatTime(v.timestamp) }}</span>
                <div class="text-[9px]" :style="{ color: severityColor(v.severity) }">{{ (v.confidence * 100).toFixed(0) }}%</div>
              </div>
            </div>
            <div v-if="expandedSession.recentViolations.length === 0" class="text-center py-4">
              <p class="text-xs" style="color: var(--argus-text-dimmed);">Нет нарушений</p>
            </div>
          </div>

          <!-- Actions -->
          <div v-if="expandedSession.status === 'active'" class="flex gap-2 pt-2">
            <button
              class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg text-sm font-medium transition-all"
              :style="{ background: warningBg(0.15), color: 'var(--argus-warning)' }"
              @click="handleWarn(expandedSession!)"
            >
              <UIcon name="i-lucide-alert-triangle" class="size-4" />
              Предупредить
            </button>
            <button
              class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg text-sm font-medium transition-all"
              :style="{ background: errorBg(0.15), color: 'var(--argus-error)' }"
              @click="handleTerminate(expandedSession!)"
            >
              <UIcon name="i-lucide-ban" class="size-4" />
              Завершить сессию
            </button>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>
