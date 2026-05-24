<script setup lang="ts">
import { useDashboardStore, type ArchiveSession } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'
import { useColors } from '~/composables/useColors'
import { formatTime, formatDate, formatVideoTimestamp } from '~/composables/useFormatters'
import {
  sessionStatusLabel as statusLabel,
  sessionStatusColor as statusColor,
  sessionStatusIcon as statusIcon,
  sessionStatusBg as _sessionStatusBg,
  severityColor,
  severityBg as _severityBg,
  eventIcon,
  isBehavioralEvent,
  isAudioEvent,
  sourceLabel,
  sourceIcon,
  sourceColor as _sourceColor,
  isSideEvent,
  integrityColor,
  integrityGradient as _integrityGradient,
  noiseLevelColor,
  noiseLevelLabel
} from '~/composables/useStatusHelpers'

const store = useDashboardStore()
const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg, infoBg } = useColors()
const loadingEvents = ref(false)

// Wrap shared helpers with our colors instance
function statusBg(status: string, opacity: number): string {
  return _sessionStatusBg(status, opacity, { successBg, warningBg, errorBg })
}
function severityBgLocal(severity: string, opacity: number): string {
  return _severityBg(severity, opacity, { errorBg, warningBg, infoBg })
}
function sourceColor(source: string): string {
  return _sourceColor(source, isDark.value)
}
function integrityGradient(score: number): string {
  return _integrityGradient(score, isDark.value)
}

// --- Filter state ---
const searchFocused = ref(false)

// Org-aware archive sessions - applies org filter before exam/search/status filters
const orgAwareArchiveSessions = computed(() => {
  let sessions = store.orgFilteredArchiveSessions

  if (store.archiveSelectedExamId) {
    sessions = sessions.filter(s => s.examId === store.archiveSelectedExamId)
  }

  if (store.archiveStatusFilter !== 'all') {
    sessions = sessions.filter(s => s.status === store.archiveStatusFilter)
  }

  const q = store.archiveSearchQuery.trim().toLowerCase()
  if (q) {
    sessions = sessions.filter(s =>
      s.studentName.toLowerCase().includes(q)
      || s.iin.includes(q)
      || s.phone.replace(/\s/g, '').includes(q.replace(/\s/g, ''))
    )
  }

  return sessions
})

// Org-aware archive stats
const orgAwareArchiveStats = computed(() => {
  const all = store.orgFilteredArchiveSessions
  const total = all.length
  const reviewed = all.filter(s => s.status === 'reviewed').length
  const pending = all.filter(s => s.status === 'pending').length
  const voided = all.filter(s => s.status === 'voided').length
  return { total, reviewed, pending, voided }
})

// Org-aware archive exam options
const orgAwareArchiveExamOptions = computed(() => {
  const allOption = { label: 'Все экзамены', value: '' }
  const examOptions = store.orgFilteredArchiveExams.map(e => ({
    label: `${e.examName} (${e.date})`,
    value: e.id
  }))
  return [allOption, ...examOptions]
})

const archiveExamOptions = computed(() => {
  const allOption = { label: 'Все экзамены', value: '' }
  const examOptions = store.archiveExams.map(e => ({
    label: `${e.examName} (${e.date})`,
    value: e.id
  }))
  return [allOption, ...examOptions]
})

const selectedArchiveExam = computed({
  get: () => store.archiveSelectedExamId ?? '',
  set: (val: string) => store.selectArchiveExam(val || null)
})

// --- Detail modal ---
const expandedArchive = ref<ArchiveSession | null>(null)
const seekPosition = ref(0)
const isPlaying = ref(false)
const videoUrl = ref<string | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)

async function openArchiveSession(session: ArchiveSession) {
  expandedArchive.value = session
  seekPosition.value = 0
  isPlaying.value = false
  camerasSwapped.value = false
  isFullscreen.value = false
  fullscreenTarget.value = null
  isMuted.value = true
  audioVolume.value = 75
  audioSyncActive.value = false
  highlightedCamera.value = null
  videoUrl.value = null

  // Fetch events from API if not already loaded
  if (!session.events || session.events.length === 0) {
    loadingEvents.value = true
    await store.fetchSessionEvents(session.id)
    loadingEvents.value = false
  }

  // Set video URL immediately — the <video> element's @error handler clears it if not found.
  const apiBase = useRuntimeConfig().public.apiBaseUrl || ''
  const token = authStore.jwtToken || ''
  if (token) {
    videoUrl.value = `${apiBase}/api/v1/archive/sessions/${session.id}/video?token=${encodeURIComponent(token)}`
  }
}

async function handleExportSession(sessionId: string) {
  await store.exportSessionReport(sessionId)
}

function closeArchiveSession() {
  if (document.fullscreenElement) {
    document.exitFullscreen()
  }
  expandedArchive.value = null
  camerasSwapped.value = false
}

// Camera highlight on event seek
const highlightedCamera = ref<'webcam' | 'side' | null>(null)

// Compute the total duration of the current session in seconds
const sessionDurationSec = computed(() => {
  if (!expandedArchive.value) return 3600 // default 1 hour
  const events = expandedArchive.value.events
  if (events.length === 0) return 3600
  const maxVideoTs = Math.max(...events.map(e => e.videoTimestamp))
  return Math.max(maxVideoTs + 60, 300) // at least 5 minutes, pad by 60s
})

function seekToEvent(videoTimestamp: number, eventSource?: string) {
  if (!expandedArchive.value) return
  seekPosition.value = Math.min(100, (videoTimestamp / sessionDurationSec.value) * 100)
  // Flash audio sync indicator
  audioSyncActive.value = true
  setTimeout(() => { audioSyncActive.value = false }, 1500)
  // Highlight source camera
  if (eventSource === 'side' || eventSource === 'webcam') {
    highlightedCamera.value = eventSource
    setTimeout(() => { highlightedCamera.value = null }, 2500)
  } else {
    highlightedCamera.value = null
  }
}

function togglePlayback() {
  if (videoRef.value) {
    if (isPlaying.value) {
      videoRef.value.pause()
    } else {
      videoRef.value.play()
    }
  } else {
    isPlaying.value = !isPlaying.value
  }
}

// --- Camera swap ---
const camerasSwapped = ref(false)

function swapCameras() {
  camerasSwapped.value = !camerasSwapped.value
}

// --- Dual fullscreen ---
const webcamRef = ref<HTMLElement | null>(null)
const sideCamRef = ref<HTMLElement | null>(null)
const isFullscreen = ref(false)
const fullscreenTarget = ref<'webcam' | 'side' | null>(null)

async function toggleFullscreenWebcam() {
  const target = camerasSwapped.value ? sideCamRef.value : webcamRef.value
  if (!target) return

  if (document.fullscreenElement) {
    await document.exitFullscreen()
    isFullscreen.value = false
    fullscreenTarget.value = null
  } else {
    try {
      await target.requestFullscreen()
      isFullscreen.value = true
      fullscreenTarget.value = 'webcam'
    } catch (_e) {
      // Fullscreen not available
    }
  }
}

async function toggleFullscreenSide() {
  const target = camerasSwapped.value ? webcamRef.value : sideCamRef.value
  if (!target) return

  if (document.fullscreenElement) {
    await document.exitFullscreen()
    isFullscreen.value = false
    fullscreenTarget.value = null
  } else {
    try {
      await target.requestFullscreen()
      isFullscreen.value = true
      fullscreenTarget.value = 'side'
    } catch (_e) {
      // Fullscreen not available
    }
  }
}

function handleFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement
  if (!document.fullscreenElement) {
    fullscreenTarget.value = null
  }
}

// --- Audio analytics ---
const isMuted = ref(true)
const audioVolume = ref(75)
const audioSyncActive = ref(false)

// Simulated recorded waveform bars (static pattern based on seek position)
const waveformBars = 48
const waveformHeights = computed(() => {
  // Generate deterministic waveform from seek position
  return Array.from({ length: waveformBars }, (_, i) => {
    const phase = (i + seekPosition.value * 0.3) * 0.15
    const base = 30 + Math.sin(phase) * 25 + Math.cos(phase * 2.7) * 15
    return Math.max(8, Math.min(100, base + (i % 3 === 0 ? 20 : 0)))
  })
})

// Noise level derived from seek position (simulated)
const noiseLevel = computed(() => {
  if (!expandedArchive.value) return 0
  const base = expandedArchive.value.violationCount > 3 ? 55 : expandedArchive.value.violationCount > 1 ? 38 : 22
  const variation = Math.sin(seekPosition.value * 0.1) * 12
  return Math.round(Math.max(10, base + variation))
})

// noiseLevelColor/noiseLevelLabel → centralized in useStatusHelpers()
// isAudioEvent → centralized in useStatusHelpers()

// --- Appeal state ---
const appealFormOpen = ref(false)
const appealReason = ref('')
const appealSubmitting = ref(false)
const appealError = ref('')
const appealSuccess = ref(false)

async function handleSubmitAppeal() {
  if (!expandedArchive.value || !appealReason.value.trim()) return

  appealSubmitting.value = true
  appealError.value = ''
  appealSuccess.value = false

  try {
    const { useAdminAPI } = await import('~/composables/useAdminAPI')
    const api = useAdminAPI()
    await api.submitAppeal({
      sessionId: expandedArchive.value.id,
      examId: expandedArchive.value.examId,
      studentId: expandedArchive.value.iin || '',
      reason: appealReason.value.trim()
    })
    appealSuccess.value = true
    appealReason.value = ''
    setTimeout(() => {
      appealFormOpen.value = false
      appealSuccess.value = false
    }, 2000)
  } catch (err: unknown) {
    appealError.value = err instanceof Error ? err.message : 'Failed to submit appeal'
  } finally {
    appealSubmitting.value = false
  }
}

onMounted(() => {
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  // Fetch real archive data from API
  store.fetchArchiveData()
})

onUnmounted(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
})
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1
          class="text-2xl font-bold"
          style="color: var(--argus-text);"
        >
          Архив сессий
        </h1>
        <p
          class="text-sm mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Завершённые экзамены с записями и AI-анализом — {{ orgAwareArchiveStats.total }} сессий
          <span
            v-if="store.archiveLoading"
            class="inline-flex items-center gap-1 ml-2 text-xs"
            style="color: var(--argus-accent);"
          >
            <span
              class="animate-spin inline-block size-3 border border-t-transparent rounded-full"
              style="border-color: var(--argus-accent); border-top-color: transparent;"
            />
            Загрузка...
          </span>
        </p>
      </div>

      <!-- Stats Badges -->
      <div class="flex items-center gap-3 flex-wrap">
        <div
          class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
          :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
        >
          <UIcon
            name="i-lucide-check-circle"
            class="size-3.5"
          />
          {{ orgAwareArchiveStats.reviewed }} проверено
        </div>
        <div
          class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
          :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
        >
          <UIcon
            name="i-lucide-clock"
            class="size-3.5"
          />
          {{ orgAwareArchiveStats.pending }} на проверке
        </div>
        <div
          class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
          :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
        >
          <UIcon
            name="i-lucide-x-circle"
            class="size-3.5"
          />
          {{ orgAwareArchiveStats.voided }} аннулировано
        </div>
      </div>
    </div>

    <!-- ===== SINGLE ROW FILTER: Exam + Search + Status Filters ===== -->
    <div class="glass-card rounded-xl px-4 py-3">
      <div class="flex items-center gap-3">
        <!-- Org Filter (Super Admin only) -->
        <div
          v-if="authStore.isSuperAdmin"
          class="w-[18%] min-w-40 shrink-0"
        >
          <div
            class="flex items-center gap-2 px-3 py-2 rounded-lg border text-xs cursor-pointer"
            :style="{
              background: 'var(--argus-bg-elevated)',
              borderColor: 'var(--argus-border)',
              color: authStore.selectedOrgId ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
            }"
          >
            <UIcon
              name="i-lucide-building-2"
              class="size-3.5 shrink-0"
            />
            <span class="truncate font-medium">
              {{ authStore.selectedOrgId ? store.getOrgName(authStore.selectedOrgId) : 'Все организации' }}
            </span>
          </div>
        </div>

        <!-- Exam Filter (~20%) -->
        <div class="w-[20%] min-w-44 shrink-0">
          <USelectMenu
            v-model="selectedArchiveExam"
            :items="orgAwareArchiveExamOptions.map(o => ({ label: o.label, value: o.value }))"
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
          <UIcon
            name="i-heroicons-magnifying-glass"
            class="size-4 shrink-0"
            :style="{ color: searchFocused ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)' }"
          />
          <input
            v-model="store.archiveSearchQuery"
            type="text"
            placeholder="Поиск по имени, ИИН или телефону..."
            class="w-full bg-transparent text-sm outline-none"
            :style="{ color: 'var(--argus-text)' }"
            @focus="searchFocused = true"
            @blur="searchFocused = false"
          >
          <span
            v-if="store.archiveSearchQuery"
            class="text-[10px] font-medium px-2 py-0.5 rounded-full whitespace-nowrap"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ orgAwareArchiveSessions.length }}
          </span>
        </div>

        <!-- Status Filter Buttons -->
        <div class="flex items-center gap-1.5 shrink-0">
          <button
            v-for="filter in (['all', 'reviewed', 'pending', 'voided'] as const)"
            :key="filter"
            class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all border"
            :style="{
              background: store.archiveStatusFilter === filter
                ? (filter === 'voided' ? errorBg(0.12) : filter === 'pending' ? warningBg(0.12) : filter === 'reviewed' ? successBg(0.12) : accentBg(0.1))
                : 'transparent',
              borderColor: store.archiveStatusFilter === filter
                ? (filter === 'voided' ? errorBg(0.35) : filter === 'pending' ? warningBg(0.35) : filter === 'reviewed' ? successBg(0.35) : accentBg(0.25))
                : 'var(--argus-border)',
              color: store.archiveStatusFilter === filter
                ? (filter === 'voided' ? 'var(--argus-error)' : filter === 'pending' ? 'var(--argus-warning)' : filter === 'reviewed' ? 'var(--argus-success)' : 'var(--argus-accent)')
                : 'var(--argus-text-dimmed)'
            }"
            @click="store.archiveStatusFilter = filter"
          >
            <UIcon
              :name="filter === 'voided' ? 'i-lucide-x-circle' : filter === 'pending' ? 'i-lucide-clock' : filter === 'reviewed' ? 'i-lucide-check-circle' : 'i-lucide-layout-grid'"
              class="size-3.5"
            />
            {{ filter === 'all' ? 'Все' : filter === 'reviewed' ? 'Проверено' : filter === 'pending' ? 'На проверке' : 'Аннулировано' }}
            <span
              class="text-[10px] font-bold px-1.5 py-0.5 rounded-full ml-0.5"
              :style="{
                background: filter === 'voided' ? errorBg(0.15) : filter === 'pending' ? warningBg(0.15) : filter === 'reviewed' ? successBg(0.15) : accentBg(0.1),
                color: filter === 'voided' ? 'var(--argus-error)' : filter === 'pending' ? 'var(--argus-warning)' : filter === 'reviewed' ? 'var(--argus-success)' : 'var(--argus-accent)'
              }"
            >
              {{ filter === 'all' ? orgAwareArchiveStats.total : filter === 'reviewed' ? orgAwareArchiveStats.reviewed : filter === 'pending' ? orgAwareArchiveStats.pending : orgAwareArchiveStats.voided }}
            </span>
          </button>
        </div>
      </div>
    </div>

    <!-- ===== EXAM SUMMARIES CARDS ===== -->
    <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
      <div
        v-for="exam in store.archiveExams"
        :key="exam.id"
        class="glass-card rounded-xl p-4 cursor-pointer video-card-hover"
        :style="{ borderColor: store.archiveSelectedExamId === exam.id ? 'var(--argus-accent)' : 'var(--argus-border)' }"
        @click="store.selectArchiveExam(store.archiveSelectedExamId === exam.id ? null : exam.id)"
      >
        <div class="flex items-start justify-between gap-2 mb-3">
          <div class="min-w-0 flex-1">
            <p
              class="text-sm font-semibold truncate"
              style="color: var(--argus-text);"
            >
              {{ exam.examName }}
            </p>
            <p
              class="text-[10px] mt-0.5"
              style="color: var(--argus-text-dimmed);"
            >
              {{ formatDate(exam.date) }}
            </p>
          </div>
          <div
            v-if="store.archiveSelectedExamId === exam.id"
            class="flex items-center justify-center size-5 rounded-full shrink-0"
            :style="{ background: accentBg(0.15) }"
          >
            <UIcon
              name="i-lucide-check"
              class="size-3"
              style="color: var(--argus-accent);"
            />
          </div>
        </div>

        <div class="grid grid-cols-3 gap-3">
          <div>
            <p
              class="text-[9px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              Участники
            </p>
            <p
              class="text-sm font-bold mt-0.5"
              style="color: var(--argus-text);"
            >
              {{ exam.participants }}
            </p>
          </div>
          <div>
            <p
              class="text-[9px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              Честность
            </p>
            <p
              class="text-sm font-bold mt-0.5"
              :style="{ color: integrityColor(exam.avgIntegrity) }"
            >
              {{ exam.avgIntegrity }}%
            </p>
          </div>
          <div>
            <p
              class="text-[9px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              Нарушения
            </p>
            <p
              class="text-sm font-bold mt-0.5"
              :style="{ color: exam.totalViolations > 30 ? 'var(--argus-error)' : 'var(--argus-text)' }"
            >
              {{ exam.totalViolations }}
            </p>
          </div>
        </div>

        <!-- Status breakdown bar -->
        <div
          class="mt-3 flex items-center gap-1 h-1.5 rounded-full overflow-hidden"
          style="background: var(--argus-bg-hover);"
        >
          <div
            class="h-full rounded-full"
            :style="{ width: `${(exam.reviewed / exam.participants) * 100}%`, background: 'var(--argus-success)' }"
          />
          <div
            class="h-full rounded-full"
            :style="{ width: `${(exam.pending / exam.participants) * 100}%`, background: 'var(--argus-warning)' }"
          />
          <div
            v-if="exam.voided > 0"
            class="h-full rounded-full"
            :style="{ width: `${(exam.voided / exam.participants) * 100}%`, background: 'var(--argus-error)' }"
          />
        </div>
        <div class="flex items-center gap-3 mt-1.5">
          <span
            class="text-[8px] font-medium"
            style="color: var(--argus-success);"
          >{{ exam.reviewed }} проверено</span>
          <span
            class="text-[8px] font-medium"
            style="color: var(--argus-warning);"
          >{{ exam.pending }} ожидает</span>
          <span
            v-if="exam.voided > 0"
            class="text-[8px] font-medium"
            style="color: var(--argus-error);"
          >{{ exam.voided }} аннул.</span>
        </div>
      </div>
    </div>

    <!-- ===== SESSION TABLE ===== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="px-5 py-3.5 border-b flex items-center justify-between"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-archive"
            class="size-4"
            style="color: var(--argus-text-dimmed);"
          />
          <h3
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Записи сессий
          </h3>
          <span
            class="text-[10px] font-medium px-2 py-0.5 rounded-full"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ orgAwareArchiveSessions.length }} записей
          </span>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Студент
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Экзамен
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Дата
              </th>
              <th
                class="px-5 py-3 text-center text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Длительность
              </th>
              <th
                class="px-5 py-3 text-center text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Честность
              </th>
              <th
                class="px-5 py-3 text-center text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Нарушения
              </th>
              <th
                class="px-5 py-3 text-center text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Статус
              </th>
              <th
                class="px-5 py-3 text-right text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Действия
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="session in orgAwareArchiveSessions"
              :key="session.id"
              class="transition-colors cursor-pointer"
              :style="{ borderBottom: '1px solid var(--argus-border-subtle)' }"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              @click="openArchiveSession(session)"
            >
              <td class="px-5 py-3.5">
                <div>
                  <p
                    class="text-sm font-medium"
                    style="color: var(--argus-text);"
                  >
                    {{ session.studentName }}
                  </p>
                  <p
                    class="text-[10px] mt-0.5"
                    style="color: var(--argus-text-dimmed);"
                  >
                    ИИН: {{ session.iin }}
                  </p>
                </div>
              </td>
              <td class="px-5 py-3.5">
                <div class="flex items-center">
                  <p
                    class="text-xs font-medium truncate max-w-48"
                    style="color: var(--argus-text-muted);"
                  >
                    {{ session.examName }}
                  </p>
                  <span
                    v-if="authStore.isSuperAdmin"
                    class="px-1.5 py-0.5 rounded text-[9px] font-bold shrink-0 ml-1"
                    :style="{
                      background: accentBg(0.08),
                      color: 'var(--argus-accent)',
                      border: `1px solid ${accentBg(0.15)}`
                    }"
                  >
                    {{ store.getOrgName(session.orgId) }}
                  </span>
                </div>
              </td>
              <td class="px-5 py-3.5">
                <p
                  class="text-xs"
                  style="color: var(--argus-text-muted);"
                >
                  {{ formatDate(session.date) }}
                </p>
              </td>
              <td class="px-5 py-3.5 text-center">
                <span
                  class="text-xs font-medium"
                  style="color: var(--argus-text);"
                >{{ session.duration }}</span>
              </td>
              <td class="px-5 py-3.5 text-center">
                <div class="flex items-center justify-center gap-2">
                  <span
                    class="text-xs font-bold tabular-nums"
                    :style="{ color: integrityColor(session.integrityScore) }"
                  >{{ session.integrityScore }}%</span>
                  <div
                    class="w-12 h-1 rounded-full overflow-hidden"
                    style="background: var(--argus-bg-hover);"
                  >
                    <div
                      class="h-full rounded-full"
                      :style="{ width: `${session.integrityScore}%`, background: integrityGradient(session.integrityScore) }"
                    />
                  </div>
                </div>
              </td>
              <td class="px-5 py-3.5 text-center">
                <span
                  v-if="session.violationCount > 0"
                  class="text-[10px] font-bold px-2 py-0.5 rounded-full"
                  :style="{
                    background: session.violationCount >= 5 ? errorBg(0.1) : session.violationCount >= 2 ? warningBg(0.1) : accentBg(0.05),
                    color: session.violationCount >= 5 ? 'var(--argus-error)' : session.violationCount >= 2 ? 'var(--argus-warning)' : 'var(--argus-text-muted)'
                  }"
                >
                  {{ session.violationCount }}
                </span>
                <span
                  v-else
                  class="text-[10px] font-medium"
                  style="color: var(--argus-text-dimmed);"
                >0</span>
              </td>
              <td class="px-5 py-3.5 text-center">
                <span
                  class="inline-flex items-center gap-1 text-[10px] font-semibold px-2 py-1 rounded-full"
                  :style="{ background: statusBg(session.status, 0.1), color: statusColor(session.status) }"
                >
                  <UIcon
                    :name="statusIcon(session.status)"
                    class="size-3"
                  />
                  {{ statusLabel(session.status) }}
                </span>
              </td>
              <td class="px-5 py-3.5 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <button
                    class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all"
                    :style="{ background: accentBg(0.08), color: 'var(--argus-accent)' }"
                    @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.15)"
                    @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.08)"
                    @click.stop="openArchiveSession(session)"
                  >
                    <UIcon
                      name="i-lucide-play-circle"
                      class="size-3"
                    />
                    Просмотр
                  </button>
                  <button
                    class="flex items-center justify-center size-7 rounded-md transition-all"
                    :style="{ color: 'var(--argus-text-dimmed)' }"
                    title="Скачать отчёт (JSON)"
                    @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                    @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                    @click.stop="handleExportSession(session.id)"
                  >
                    <UIcon
                      name="i-lucide-download"
                      class="size-3.5"
                    />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Empty state -->
      <div
        v-if="orgAwareArchiveSessions.length === 0"
        class="flex flex-col items-center justify-center py-16"
      >
        <UIcon
          name="i-lucide-archive-x"
          class="size-12 mb-3"
          style="color: var(--argus-text-dimmed);"
        />
        <p
          class="text-sm font-medium"
          style="color: var(--argus-text);"
        >
          Нет записей по заданному фильтру
        </p>
        <p
          class="text-xs mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Измените критерии поиска или сбросьте фильтр
        </p>
        <button
          class="mt-4 px-4 py-2 rounded-lg text-xs font-medium transition-all"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          @click="store.archiveStatusFilter = 'all'; store.archiveSearchQuery = ''; store.selectArchiveExam(null)"
        >
          Сбросить фильтры
        </button>
      </div>
    </div>

    <!-- ===== SESSION DETAIL MODAL (max-w-7xl) ===== -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="expandedArchive"
          class="fixed inset-0 z-[100] flex items-center justify-center p-4"
        >
          <!-- Backdrop -->
          <div
            class="absolute inset-0"
            :style="{ background: isDark ? 'rgba(0, 0, 0, 0.7)' : 'rgba(0, 0, 0, 0.4)' }"
            @click="closeArchiveSession"
          />

          <!-- Modal -->
          <div
            class="relative w-full max-w-7xl max-h-[92vh] rounded-2xl border overflow-hidden flex flex-col z-10"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <!-- Modal Header -->
            <div
              class="flex items-center justify-between px-6 py-4 border-b shrink-0"
              style="border-color: var(--argus-border);"
            >
              <div class="flex items-center gap-3">
                <span
                  class="flex items-center gap-1.5 px-2.5 py-1 rounded-full"
                  :style="{ background: statusBg(expandedArchive.status, 0.1), color: statusColor(expandedArchive.status) }"
                >
                  <UIcon
                    :name="statusIcon(expandedArchive.status)"
                    class="size-3.5"
                  />
                  <span class="text-[10px] font-bold uppercase">{{ statusLabel(expandedArchive.status) }}</span>
                </span>
                <div>
                  <h2
                    class="text-lg font-bold"
                    style="color: var(--argus-text);"
                  >
                    {{ expandedArchive.studentName }}
                  </h2>
                  <p
                    class="text-xs"
                    style="color: var(--argus-text-dimmed);"
                  >
                    ИИН: {{ expandedArchive.iin }} · {{ expandedArchive.phone }} · {{ expandedArchive.examName }}
                  </p>
                </div>
              </div>

              <div class="flex items-center gap-2">
                <button
                  class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all border"
                  :style="{ borderColor: 'var(--argus-border)', color: 'var(--argus-text-muted)' }"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                  @click="handleExportSession(expandedArchive.id)"
                >
                  <UIcon
                    name="i-lucide-download"
                    class="size-3.5"
                  />
                  Скачать отчёт (JSON)
                </button>
                <button
                  class="flex items-center justify-center size-8 rounded-lg transition-colors"
                  style="color: var(--argus-text-dimmed);"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                  @click="closeArchiveSession"
                >
                  <UIcon
                    name="i-lucide-x"
                    class="size-5"
                  />
                </button>
              </div>
            </div>

            <!-- Modal Body -->
            <div class="flex-1 overflow-y-auto">
              <div class="grid grid-cols-1 lg:grid-cols-7 gap-0">
                <!-- Left: Video + Audio (5 cols) -->
                <div class="lg:col-span-5 p-6 space-y-4">
                  <!-- Dual Camera Playback with Swap + Fullscreen -->
                  <div
                    class="flex gap-3"
                    :class="camerasSwapped ? 'flex-row-reverse' : ''"
                  >
                    <!-- PRIMARY: Webcam recording — fullscreenable -->
                    <div
                      ref="webcamRef"
                      class="relative flex-[3] aspect-video rounded-xl overflow-hidden transition-all duration-500"
                      :class="{ 'rounded-none': isFullscreen && fullscreenTarget === 'webcam' }"
                      :style="{
                        background: 'var(--argus-bg-deep)',
                        boxShadow: (highlightedCamera === 'webcam' && !camerasSwapped) || (highlightedCamera === 'side' && camerasSwapped)
                          ? '0 0 0 3px var(--argus-error), 0 0 20px rgba(248,113,113,0.3)'
                          : 'none'
                      }"
                    >
                      <!-- Recorded video if available -->
                      <video
                        v-if="videoUrl && !camerasSwapped"
                        ref="videoRef"
                        :src="videoUrl"
                        class="absolute inset-0 w-full h-full object-cover"
                        :muted="isMuted"
                        preload="metadata"
                        @timeupdate="seekPosition = videoRef?.duration ? (videoRef.currentTime / videoRef.duration) * 100 : 0"
                        @play="isPlaying = true"
                        @pause="isPlaying = false"
                        @error="videoUrl = null"
                      />
                      <!-- Placeholder when no recording -->
                      <div v-else class="absolute inset-0 flex items-center justify-center">
                        <div class="flex flex-col items-center gap-3 opacity-25">
                          <UIcon
                            name="i-lucide-film"
                            class="size-14"
                            style="color: var(--argus-text-dimmed);"
                          />
                          <span
                            class="text-xs font-medium"
                            style="color: var(--argus-text-dimmed);"
                          >{{ camerasSwapped ? 'БОКОВАЯ КАМЕРА' : 'ЗАПИСЬ ВЕБ-КАМЕРЫ' }}</span>
                        </div>
                      </div>

                      <!-- Playback controls overlay -->
                      <div
                        class="absolute bottom-0 left-0 right-0 p-3 flex flex-col gap-2"
                        :style="{ background: isDark ? 'linear-gradient(transparent, rgba(0,0,0,0.85))' : 'linear-gradient(transparent, rgba(0,0,0,0.55))' }"
                      >
                        <!-- Seek bar with event markers -->
                        <div class="relative">
                          <input
                            v-model.number="seekPosition"
                            type="range"
                            min="0"
                            max="100"
                            class="w-full h-1.5 rounded-full appearance-none cursor-pointer relative z-10"
                            :style="{
                              background: `linear-gradient(to right, var(--argus-accent) ${seekPosition}%, rgba(255,255,255,0.15) ${seekPosition}%)`
                            }"
                          >
                          <!-- Event markers on seekbar -->
                          <div class="absolute inset-x-0 top-1/2 -translate-y-1/2 h-1.5 pointer-events-none">
                            <div
                              v-for="event in expandedArchive.events"
                              :key="event.id + '-marker'"
                              class="absolute top-0 w-1 h-full rounded-full"
                              :style="{
                                left: `${Math.min(100, (event.videoTimestamp / sessionDurationSec) * 100)}%`,
                                background: severityColor(event.severity),
                                opacity: 0.8
                              }"
                            />
                          </div>
                        </div>

                        <!-- Playback buttons row -->
                        <div class="flex items-center justify-between">
                          <div class="flex items-center gap-2">
                            <button
                              class="flex items-center justify-center size-8 rounded-lg transition-all"
                              style="color: white; background: rgba(255,255,255,0.12);"
                              @click="togglePlayback"
                            >
                              <UIcon
                                :name="isPlaying ? 'i-lucide-pause' : 'i-lucide-play'"
                                class="size-4"
                              />
                            </button>
                            <button
                              class="flex items-center justify-center size-7 rounded-md transition-all"
                              style="color: rgba(255,255,255,0.7);"
                            >
                              <UIcon
                                name="i-lucide-skip-back"
                                class="size-3.5"
                              />
                            </button>
                            <button
                              class="flex items-center justify-center size-7 rounded-md transition-all"
                              style="color: rgba(255,255,255,0.7);"
                            >
                              <UIcon
                                name="i-lucide-skip-forward"
                                class="size-3.5"
                              />
                            </button>
                            <span class="text-[10px] font-mono text-white/60 ml-1">
                              {{ formatVideoTimestamp(Math.floor(seekPosition / 100 * sessionDurationSec)) }} / {{ expandedArchive.duration }}
                            </span>
                          </div>
                          <div class="flex items-center gap-1">
                            <!-- Audio sync indicator -->
                            <div
                              v-if="audioSyncActive"
                              class="flex items-center gap-1 px-2 py-1 rounded-md mr-1"
                              style="background: rgba(56, 189, 248, 0.2);"
                            >
                              <UIcon
                                name="i-lucide-audio-waveform"
                                class="size-3"
                                style="color: var(--argus-accent);"
                              />
                              <span
                                class="text-[8px] font-bold"
                                style="color: var(--argus-accent);"
                              >Синхронизация звука</span>
                            </div>
                            <button
                              class="flex items-center justify-center size-7 rounded-md transition-all"
                              :style="{ color: isMuted ? 'rgba(255,255,255,0.4)' : 'rgba(255,255,255,0.8)' }"
                              :title="isMuted ? 'Включить звук' : 'Выключить звук'"
                              @click="isMuted = !isMuted"
                            >
                              <UIcon
                                :name="isMuted ? 'i-lucide-volume-x' : 'i-lucide-volume-2'"
                                class="size-3.5"
                              />
                            </button>
                            <button
                              class="flex items-center justify-center size-7 rounded-md transition-all"
                              style="color: rgba(255,255,255,0.7);"
                              :title="isFullscreen && fullscreenTarget === 'webcam' ? 'Выйти из полноэкранного режима' : camerasSwapped ? 'Во весь экран (Боковая камера)' : 'Во весь экран (Веб-камера)'"
                              @click.stop="toggleFullscreenWebcam"
                            >
                              <UIcon
                                :name="isFullscreen && fullscreenTarget === 'webcam' ? 'i-lucide-minimize' : 'i-lucide-maximize'"
                                class="size-3.5"
                              />
                            </button>
                          </div>
                        </div>
                      </div>

                      <!-- REC badge -->
                      <div
                        class="absolute top-3 left-3 flex items-center gap-1.5 px-2 py-1 rounded-lg ai-badge"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)' }"
                      >
                        <UIcon
                          name="i-lucide-circle-dot"
                          class="size-3"
                          style="color: var(--argus-text-dimmed);"
                        />
                        <span
                          class="text-[9px] font-bold uppercase"
                          style="color: var(--argus-text-dimmed);"
                        >REC</span>
                      </div>

                      <!-- Integrity badge + Fullscreen -->
                      <div class="absolute top-3 right-3 flex items-center gap-1.5">
                        <div
                          class="px-2.5 py-1.5 rounded-lg ai-badge"
                          :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)' }"
                        >
                          <span
                            class="text-base font-bold tabular-nums"
                            :style="{ color: integrityColor(expandedArchive.integrityScore) }"
                          >
                            {{ expandedArchive.integrityScore }}%
                          </span>
                        </div>
                      </div>
                    </div>

                    <!-- SECONDARY: Side camera — fullscreenable -->
                    <div
                      ref="sideCamRef"
                      class="relative flex-[1] min-w-[160px] rounded-xl overflow-hidden border transition-all duration-500"
                      :class="{ 'rounded-none': isFullscreen && fullscreenTarget === 'side' }"
                      :style="{
                        background: 'var(--argus-bg-deep)',
                        borderColor: (highlightedCamera === 'side' && !camerasSwapped) || (highlightedCamera === 'webcam' && camerasSwapped)
                          ? 'var(--argus-error)'
                          : 'var(--argus-border)',
                        boxShadow: (highlightedCamera === 'side' && !camerasSwapped) || (highlightedCamera === 'webcam' && camerasSwapped)
                          ? '0 0 0 2px var(--argus-error), 0 0 15px rgba(248,113,113,0.25)'
                          : 'none'
                      }"
                    >
                      <div class="absolute inset-0 flex items-center justify-center">
                        <div class="flex flex-col items-center gap-2 opacity-25">
                          <UIcon
                            name="i-lucide-camera"
                            class="size-8"
                            style="color: var(--argus-text-dimmed);"
                          />
                          <span
                            class="text-[9px] font-medium text-center px-2"
                            style="color: var(--argus-text-dimmed);"
                          >{{ camerasSwapped ? 'ВЕБ-КАМЕРА' : 'БОКОВАЯ КАМЕРА' }}</span>
                        </div>
                      </div>

                      <!-- Fullscreen button for side camera -->
                      <div class="absolute top-2 right-2">
                        <button
                          class="flex items-center justify-center size-7 rounded-md ai-badge transition-all"
                          :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)', color: 'var(--argus-text-muted)' }"
                          :title="isFullscreen && fullscreenTarget === 'side' ? 'Выйти из полноэкранного режима' : camerasSwapped ? 'Во весь экран (Веб-камера)' : 'Во весь экран (Боковая камера)'"
                          @mouseenter="($event.currentTarget as HTMLElement).style.color = 'var(--argus-accent)'"
                          @mouseleave="($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-muted)'"
                          @click.stop="toggleFullscreenSide"
                        >
                          <UIcon
                            :name="isFullscreen && fullscreenTarget === 'side' ? 'i-lucide-minimize' : 'i-lucide-maximize'"
                            class="size-3.5"
                          />
                        </button>
                      </div>

                      <!-- REC badge -->
                      <div
                        class="absolute top-2 left-2 flex items-center gap-1 px-1.5 py-0.5 rounded ai-badge"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
                      >
                        <UIcon
                          name="i-lucide-circle-dot"
                          class="size-2.5"
                          style="color: var(--argus-text-dimmed);"
                        />
                        <span
                          class="text-[7px] font-bold uppercase"
                          style="color: var(--argus-text-dimmed);"
                        >REC</span>
                      </div>
                    </div>
                  </div>

                  <!-- ===== AUDIO ANALYTICS SECTION ===== -->
                  <div
                    class="rounded-xl border overflow-hidden"
                    :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)' }"
                  >
                    <!-- Audio Header -->
                    <div
                      class="flex items-center justify-between px-4 py-2.5 border-b"
                      style="border-color: var(--argus-border);"
                    >
                      <div class="flex items-center gap-2">
                        <UIcon
                          name="i-lucide-audio-waveform"
                          class="size-4"
                          style="color: var(--argus-accent);"
                        />
                        <span
                          class="text-xs font-semibold"
                          style="color: var(--argus-text);"
                        >Уровень звука</span>
                        <span
                          class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
                          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
                        >
                          Аудио запись
                        </span>
                        <!-- Audio sync flash -->
                        <Transition name="modal">
                          <span
                            v-if="audioSyncActive"
                            class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
                            :style="{ background: accentBg(0.15), color: 'var(--argus-accent)' }"
                          >
                            Синхронизация звука
                          </span>
                        </Transition>
                      </div>

                      <div class="flex items-center gap-3">
                        <!-- Noise level indicator -->
                        <div class="flex items-center gap-1.5">
                          <span
                            class="text-[9px] font-medium"
                            style="color: var(--argus-text-dimmed);"
                          >Уровень шума:</span>
                          <span
                            class="text-[10px] font-bold tabular-nums"
                            :style="{ color: noiseLevelColor(noiseLevel) }"
                          >
                            {{ noiseLevel }} дБ
                          </span>
                          <span
                            class="text-[8px] font-semibold px-1.5 py-0.5 rounded"
                            :style="{
                              background: noiseLevel > 55 ? errorBg(0.1) : noiseLevel > 35 ? warningBg(0.1) : successBg(0.1),
                              color: noiseLevelColor(noiseLevel)
                            }"
                          >
                            {{ noiseLevelLabel(noiseLevel) }}
                          </span>
                        </div>
                      </div>
                    </div>

                    <!-- Waveform + Volume Controls -->
                    <div class="flex items-center gap-4 px-4 py-3">
                      <!-- Waveform bars -->
                      <div class="flex-1 flex items-end gap-[2px] h-10">
                        <div
                          v-for="(h, i) in waveformHeights"
                          :key="i"
                          class="flex-1 rounded-t-sm transition-[height] duration-150"
                          :style="{
                            height: `${isMuted ? 8 : h}%`,
                            background: isMuted
                              ? 'var(--argus-text-dimmed)'
                              : h > 75
                                ? 'var(--argus-error)'
                                : h > 50
                                  ? 'var(--argus-warning)'
                                  : 'var(--argus-accent)',
                            opacity: isMuted ? 0.2 : 0.65
                          }"
                        />
                      </div>

                      <!-- Mute toggle + Volume slider -->
                      <div class="flex items-center gap-2 shrink-0">
                        <button
                          class="flex items-center justify-center size-8 rounded-lg transition-all border"
                          :style="{
                            background: isMuted ? 'transparent' : accentBg(0.1),
                            borderColor: isMuted ? 'var(--argus-border)' : accentBg(0.25),
                            color: isMuted ? 'var(--argus-text-dimmed)' : 'var(--argus-accent)'
                          }"
                          :title="isMuted ? 'Включить звук' : 'Выключить звук'"
                          @mouseenter="($event.currentTarget as HTMLElement).style.background = isMuted ? 'var(--argus-bg-hover)' : accentBg(0.15)"
                          @mouseleave="($event.currentTarget as HTMLElement).style.background = isMuted ? 'transparent' : accentBg(0.1)"
                          @click="isMuted = !isMuted"
                        >
                          <UIcon
                            :name="isMuted ? 'i-lucide-volume-x' : audioVolume > 50 ? 'i-lucide-volume-2' : 'i-lucide-volume-1'"
                            class="size-4"
                          />
                        </button>

                        <div class="flex items-center gap-2 w-24">
                          <input
                            v-model.number="audioVolume"
                            type="range"
                            min="0"
                            max="100"
                            class="w-full h-1 rounded-full appearance-none cursor-pointer"
                            :style="{
                              background: `linear-gradient(to right, var(--argus-accent) ${audioVolume}%, var(--argus-border) ${audioVolume}%)`,
                              opacity: isMuted ? 0.35 : 1
                            }"
                            :disabled="isMuted"
                          >
                          <span
                            class="text-[9px] font-bold tabular-nums w-7 text-right"
                            :style="{ color: isMuted ? 'var(--argus-text-dimmed)' : 'var(--argus-text-muted)' }"
                          >
                            {{ isMuted ? '—' : `${audioVolume}%` }}
                          </span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <!-- Controls Row: Swap Cameras -->
                  <div class="flex items-center justify-between gap-3">
                    <!-- Session Summary (inline) -->
                    <div class="flex items-center gap-3">
                      <div
                        class="flex items-center gap-1.5 px-3 py-2 rounded-lg"
                        :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)' }"
                      >
                        <span
                          class="text-[9px] font-medium"
                          style="color: var(--argus-text-dimmed);"
                        >Дата:</span>
                        <span
                          class="text-[10px] font-bold"
                          style="color: var(--argus-text);"
                        >{{ formatDate(expandedArchive.date) }}</span>
                      </div>
                      <div
                        class="flex items-center gap-1.5 px-3 py-2 rounded-lg"
                        :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)' }"
                      >
                        <span
                          class="text-[9px] font-medium"
                          style="color: var(--argus-text-dimmed);"
                        >Длительность:</span>
                        <span
                          class="text-[10px] font-bold"
                          style="color: var(--argus-text);"
                        >{{ expandedArchive.duration }}</span>
                      </div>
                      <div
                        class="flex items-center gap-1.5 px-3 py-2 rounded-lg"
                        :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)' }"
                      >
                        <span
                          class="text-[9px] font-medium"
                          style="color: var(--argus-text-dimmed);"
                        >Нарушения:</span>
                        <span
                          class="text-[10px] font-bold"
                          :style="{ color: expandedArchive.violationCount > 3 ? 'var(--argus-error)' : 'var(--argus-text)' }"
                        >{{ expandedArchive.violationCount }}</span>
                      </div>
                    </div>

                    <!-- Swap cameras button -->
                    <button
                      class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all border"
                      :style="{
                        background: camerasSwapped ? accentBg(0.1) : 'transparent',
                        borderColor: camerasSwapped ? accentBg(0.25) : 'var(--argus-border)',
                        color: camerasSwapped ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
                      }"
                      title="Поменять камеры местами"
                      @mouseenter="($event.currentTarget as HTMLElement).style.background = camerasSwapped ? accentBg(0.15) : 'var(--argus-bg-hover)'"
                      @mouseleave="($event.currentTarget as HTMLElement).style.background = camerasSwapped ? accentBg(0.1) : 'transparent'"
                      @click="swapCameras"
                    >
                      <UIcon
                        name="i-lucide-arrow-left-right"
                        class="size-3.5"
                      />
                      Поменять камеры
                    </button>
                  </div>
                </div>

                <!-- Right: Event Log + Evidence + Review (2 cols) -->
                <div
                  class="lg:col-span-2 border-l flex flex-col"
                  style="border-color: var(--argus-border);"
                >
                  <div
                    class="px-5 py-4 border-b shrink-0"
                    style="border-color: var(--argus-border);"
                  >
                    <div class="flex items-center justify-between">
                      <div class="flex items-center gap-2">
                        <UIcon
                          name="i-lucide-activity"
                          class="size-4"
                          style="color: var(--argus-text-dimmed);"
                        />
                        <h3
                          class="text-sm font-semibold"
                          style="color: var(--argus-text);"
                        >
                          Журнал событий
                        </h3>
                      </div>
                      <span
                        class="text-[10px] font-medium px-2 py-0.5 rounded-full"
                        :style="{ background: expandedArchive.events.length > 3 ? errorBg(0.1) : accentBg(0.1), color: expandedArchive.events.length > 3 ? 'var(--argus-error)' : 'var(--argus-accent)' }"
                      >
                        {{ expandedArchive.events.length }} событий
                      </span>
                    </div>
                    <!-- Camera source summary -->
                    <div class="flex items-center gap-2 mt-2">
                      <span
                        class="inline-flex items-center gap-1 text-[8px] font-medium px-1.5 py-0.5 rounded"
                        :style="{ background: accentBg(0.06), color: 'var(--argus-accent)' }"
                      >
                        <UIcon
                          name="i-lucide-video"
                          class="size-2.5"
                        />
                        {{ expandedArchive.events.filter(e => e.source === 'webcam').length }} веб
                      </span>
                      <span
                        class="inline-flex items-center gap-1 text-[8px] font-medium px-1.5 py-0.5 rounded"
                        :style="{ background: isDark ? 'rgba(167, 139, 250, 0.08)' : 'rgba(139, 92, 246, 0.06)', color: isDark ? '#a78bfa' : '#7c3aed' }"
                      >
                        <UIcon
                          name="i-lucide-camera"
                          class="size-2.5"
                        />
                        {{ expandedArchive.events.filter(e => e.source === 'side').length }} бок.
                      </span>
                      <span
                        class="inline-flex items-center gap-1 text-[8px] font-medium px-1.5 py-0.5 rounded"
                        :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }"
                      >
                        <UIcon
                          name="i-lucide-monitor"
                          class="size-2.5"
                        />
                        {{ expandedArchive.events.filter(e => e.source === 'system').length }} сист.
                      </span>
                    </div>
                    <!-- Stream sync indicator -->
                    <Transition name="modal">
                      <div
                        v-if="highlightedCamera"
                        class="flex items-center gap-1.5 mt-2 px-2 py-1 rounded-md"
                        :style="{ background: errorBg(0.08), border: `1px solid ${errorBg(0.15)}` }"
                      >
                        <UIcon
                          name="i-lucide-radio"
                          class="size-3"
                          style="color: var(--argus-error);"
                        />
                        <span
                          class="text-[9px] font-bold"
                          style="color: var(--argus-error);"
                        >Синхронизация потоков</span>
                        <span
                          class="text-[8px] ml-auto"
                          style="color: var(--argus-text-dimmed);"
                        >
                          Источник: {{ highlightedCamera === 'side' ? 'Боковая камера' : 'Веб-камера' }}
                        </span>
                      </div>
                    </Transition>
                  </div>

                  <!-- Events Timeline -->
                  <div class="flex-1 overflow-y-auto">
                    <!-- Loading indicator -->
                    <div
                      v-if="loadingEvents"
                      class="flex flex-col items-center justify-center py-12 px-4"
                    >
                      <div
                        class="animate-spin rounded-full size-8 border-2 border-t-transparent mb-3"
                        style="border-color: var(--argus-accent); border-top-color: transparent;"
                      />
                      <p
                        class="text-xs font-medium"
                        style="color: var(--argus-text-dimmed);"
                      >
                        Загрузка событий...
                      </p>
                    </div>
                    <div
                      v-else-if="expandedArchive.events.length > 0"
                      class="px-5 py-3 space-y-0"
                    >
                      <div
                        v-for="(event, idx) in expandedArchive.events"
                        :key="event.id"
                        class="relative flex gap-3 pb-4"
                      >
                        <div class="flex flex-col items-center shrink-0">
                          <div
                            class="flex items-center justify-center size-7 rounded-full border"
                            :style="{
                              background: severityBgLocal(event.severity, 0.1),
                              borderColor: severityBgLocal(event.severity, 0.3),
                              color: severityColor(event.severity)
                            }"
                          >
                            <UIcon
                              :name="eventIcon(event.type)"
                              class="size-3.5"
                            />
                          </div>
                          <div
                            v-if="idx < expandedArchive.events.length - 1"
                            class="w-px flex-1 mt-1"
                            style="background: var(--argus-border);"
                          />
                        </div>
                        <div class="flex-1 min-w-0 pt-0.5">
                          <div class="flex items-start justify-between gap-2">
                            <p
                              class="text-xs font-medium"
                              style="color: var(--argus-text);"
                            >
                              {{ event.label }}
                            </p>
                            <span
                              class="text-[9px] font-bold px-1.5 py-0.5 rounded-full shrink-0 uppercase"
                              :style="{ background: severityBgLocal(event.severity, 0.1), color: severityColor(event.severity) }"
                            >
                              {{ event.severity === 'critical' ? 'КРИТ' : event.severity === 'warning' ? 'ВНИМАНИЕ' : 'ИНФО' }}
                            </span>
                          </div>

                          <!-- Source attribution + timestamp -->
                          <div class="flex items-center gap-2 mt-0.5">
                            <span
                              class="text-[10px]"
                              style="color: var(--argus-text-dimmed);"
                            >{{ formatTime(event.timestamp) }}</span>
                            <span
                              class="text-[8px]"
                              style="color: var(--argus-border);"
                            >·</span>
                            <span
                              class="inline-flex items-center gap-1 text-[8px] font-semibold px-1.5 py-0.5 rounded"
                              :style="{
                                background: isSideEvent(event.source) ? (isDark ? 'rgba(167, 139, 250, 0.1)' : 'rgba(139, 92, 246, 0.08)') : event.source === 'webcam' ? accentBg(0.08) : 'var(--argus-bg-hover)',
                                color: sourceColor(event.source)
                              }"
                            >
                              <UIcon
                                :name="sourceIcon(event.source)"
                                class="size-2.5"
                              />
                              {{ sourceLabel(event.source) }}
                            </span>
                          </div>

                          <!-- Behavioral Analysis / Kernel badge -->
                          <div
                            v-if="isBehavioralEvent(event.type)"
                            class="flex items-center gap-1 mt-1"
                          >
                            <span
                              class="inline-flex items-center gap-1 text-[7px] font-bold px-1.5 py-0.5 rounded-full uppercase"
                              :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }"
                            >
                              <UIcon
                                name="i-lucide-cpu"
                                class="size-2"
                              />
                              Kernel-данные
                            </span>
                            <span
                              class="text-[8px] font-medium"
                              style="color: var(--argus-accent);"
                            >Поведенческий анализ</span>
                          </div>

                          <!-- Audio event tag -->
                          <div
                            v-if="isAudioEvent(event.type)"
                            class="flex items-center gap-1 mt-1"
                          >
                            <UIcon
                              name="i-lucide-audio-waveform"
                              class="size-2.5"
                              style="color: var(--argus-accent);"
                            />
                            <span
                              class="text-[8px] font-medium"
                              style="color: var(--argus-accent);"
                            >Аудио событие</span>
                          </div>

                          <!-- Seek + audio sync + camera highlight button -->
                          <button
                            class="flex items-center gap-1 mt-1.5 px-2 py-1 rounded-md text-[9px] font-medium transition-all"
                            :style="{
                              background: isSideEvent(event.source) ? (isDark ? 'rgba(167, 139, 250, 0.08)' : 'rgba(139, 92, 246, 0.06)') : accentBg(0.08),
                              color: isSideEvent(event.source) ? sourceColor(event.source) : 'var(--argus-accent)'
                            }"
                            @mouseenter="($event.currentTarget as HTMLElement).style.opacity = '0.85'"
                            @mouseleave="($event.currentTarget as HTMLElement).style.opacity = '1'"
                            @click="seekToEvent(event.videoTimestamp, event.source)"
                          >
                            <UIcon
                              :name="isAudioEvent(event.type) ? 'i-lucide-audio-waveform' : isSideEvent(event.source) ? 'i-lucide-camera' : 'i-lucide-play'"
                              class="size-2.5"
                            />
                            {{ isAudioEvent(event.type) ? 'Синхр. звук' : 'Перейти' }} к {{ formatVideoTimestamp(event.videoTimestamp) }}
                          </button>
                        </div>
                      </div>
                    </div>

                    <!-- No events -->
                    <div
                      v-else
                      class="flex flex-col items-center justify-center py-12 px-4"
                    >
                      <UIcon
                        name="i-lucide-check-circle"
                        class="size-10 mb-3"
                        style="color: var(--argus-success);"
                      />
                      <p
                        class="text-sm font-medium"
                        style="color: var(--argus-text);"
                      >
                        Нет нарушений
                      </p>
                      <p
                        class="text-xs mt-1 text-center"
                        style="color: var(--argus-text-dimmed);"
                      >
                        Сессия прошла без инцидентов
                      </p>
                    </div>
                  </div>

                  <!-- Consent Status -->
                  <div
                    class="px-5 py-4 border-t"
                    style="border-color: var(--argus-border);"
                  >
                    <ConsentStatus :session-id="expandedArchive.id" />
                  </div>

                  <!-- Evidence Viewer -->
                  <div
                    class="px-5 py-4 border-t"
                    style="border-color: var(--argus-border);"
                  >
                    <EvidenceViewer :session-id="expandedArchive.id" />
                  </div>

                  <!-- Forensic Ledger / Integrity Verification -->
                  <div
                    class="px-5 py-4 border-t"
                    style="border-color: var(--argus-border);"
                  >
                    <IntegrityReport :session-id="expandedArchive.id" />
                  </div>

                  <!-- Review Panel -->
                  <div
                    class="px-5 py-4 border-t"
                    style="border-color: var(--argus-border);"
                  >
                    <ReviewPanel
                      :session-id="expandedArchive.id"
                      :integrity-score="expandedArchive.integrityScore"
                      @reviewed="() => { store.fetchArchiveData(); closeArchiveSession() }"
                    />
                  </div>

                  <!-- Appeal Section (only for reviewed sessions) -->
                  <div
                    v-if="expandedArchive.status === 'reviewed' || expandedArchive.status === 'voided'"
                    class="px-5 py-4 border-t"
                    style="border-color: var(--argus-border);"
                  >
                    <div class="flex items-center gap-2 mb-3">
                      <UIcon
                        name="i-lucide-scale"
                        class="size-4"
                        style="color: var(--argus-text-dimmed);"
                      />
                      <h4
                        class="text-sm font-semibold"
                        style="color: var(--argus-text);"
                      >
                        Апелляция
                      </h4>
                    </div>

                    <!-- Appeal form (collapsed by default) -->
                    <div
                      v-if="!appealFormOpen"
                      class="flex items-center gap-3"
                    >
                      <button
                        class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all border"
                        :style="{
                          background: warningBg(0.08),
                          borderColor: warningBg(0.2),
                          color: 'var(--argus-warning)'
                        }"
                        @mouseenter="($event.currentTarget as HTMLElement).style.background = warningBg(0.15)"
                        @mouseleave="($event.currentTarget as HTMLElement).style.background = warningBg(0.08)"
                        @click="appealFormOpen = true"
                      >
                        <UIcon
                          name="i-lucide-message-square-plus"
                          class="size-3.5"
                        />
                        Подать апелляцию
                      </button>
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >
                        Студент может оспорить результат проверки
                      </span>
                    </div>

                    <!-- Expanded appeal form -->
                    <div
                      v-else
                      class="space-y-3"
                    >
                      <textarea
                        v-model="appealReason"
                        rows="3"
                        class="w-full px-3 py-2 rounded-lg border text-sm resize-none outline-none transition-all"
                        :style="{
                          background: 'var(--argus-bg-elevated)',
                          borderColor: 'var(--argus-border)',
                          color: 'var(--argus-text)'
                        }"
                        placeholder="Укажите причину апелляции..."
                        @focus="($event.target as HTMLElement).style.borderColor = 'var(--argus-accent)'"
                        @blur="($event.target as HTMLElement).style.borderColor = 'var(--argus-border)'"
                      />
                      <div class="flex items-center gap-2">
                        <button
                          class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all"
                          :disabled="appealSubmitting || !appealReason.trim()"
                          :style="{
                            background: warningBg(0.12),
                            color: 'var(--argus-warning)',
                            opacity: appealSubmitting || !appealReason.trim() ? 0.5 : 1
                          }"
                          @click="handleSubmitAppeal"
                        >
                          <UIcon
                            v-if="appealSubmitting"
                            name="i-lucide-loader"
                            class="size-3.5 animate-spin"
                          />
                          <UIcon
                            v-else
                            name="i-lucide-send"
                            class="size-3.5"
                          />
                          {{ appealSubmitting ? 'Отправка...' : 'Отправить' }}
                        </button>
                        <button
                          class="px-3 py-2 rounded-lg text-xs font-medium transition-all"
                          :style="{ color: 'var(--argus-text-dimmed)' }"
                          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                          @click="appealFormOpen = false; appealReason = ''"
                        >
                          Отмена
                        </button>
                      </div>
                      <p
                        v-if="appealError"
                        class="text-[10px]"
                        style="color: var(--argus-error);"
                      >
                        {{ appealError }}
                      </p>
                      <p
                        v-if="appealSuccess"
                        class="text-[10px]"
                        style="color: var(--argus-success);"
                      >
                        Апелляция успешно подана
                      </p>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
