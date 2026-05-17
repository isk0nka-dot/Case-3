<script setup lang="ts">
// =============================================================================
// Argus AI — Live Inspector Dashboard (Composition Root)
// =============================================================================
//
// Mission-control Command Center for real-time proctoring. Orchestrates:
//   - InspectorGrid: risk-sorted session cards with AI overlays
//   - FocusModeView: expanded session with filmstrip
//   - IncidentSidebar: filtered event stream with silence mode
//   - Risk scoring engine (5s intervals via useInspectorStore)
//   - Keyboard navigation (arrow/enter/escape)
//
// Replaces the original flat session grid (730 lines) with a modular
// architecture split across 10 sub-components.
// =============================================================================

import { useAdminAPI, type ActiveSession } from '~/composables/useAdminAPI'
import { useAuthStore } from '~/stores/useAuthStore'
import { useEventFeedStore } from '~/stores/useEventFeedStore'
import { useInspectorStore, type GridDensity } from '~/stores/useInspectorStore'
import { useKeyboardNav } from '~/composables/useKeyboardNav'

const api = useAdminAPI()
const authStore = useAuthStore()
const feedStore = useEventFeedStore()
const inspectorStore = useInspectorStore()
const { accentBg, errorBg, successBg, warningBg } = useColors()

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

const sessions = ref<ActiveSession[]>([])
const stats = ref({ totalActive: 0, totalCritical: 0, totalWarning: 0, totalClean: 0, avgRiskScore: 0 })
const loading = ref(true)
const error = ref<string | null>(null)
const lastUpdated = ref('')
const searchQuery = ref('')
const violationFilter = ref<'all' | 'critical' | 'warning'>('all')
const selectedExamId = ref('')
const searchFocused = ref(false)

// Action state
const _actionLoading = ref<Record<string, boolean>>({})

// Poll timer
let pollTimer: ReturnType<typeof setInterval> | null = null

// ---------------------------------------------------------------------------
// Data Fetching
// ---------------------------------------------------------------------------

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

    // Feed sessions to the inspector store for risk computation
    inspectorStore.updateSessions(data.sessions)
  } catch (e: any) {
    console.info('[Inspector] Backend unavailable:', e.message)
    if (sessions.value.length === 0) {
      error.value = null
      stats.value = { totalActive: 0, totalCritical: 0, totalWarning: 0, totalClean: 0, avgRiskScore: 0 }
    }
  } finally {
    loading.value = false
  }
}

// ---------------------------------------------------------------------------
// Filtered Sessions
// ---------------------------------------------------------------------------

const filteredSessions = computed(() => {
  let result = sessions.value

  if (violationFilter.value !== 'all') {
    result = result.filter(s => s.violationLevel === violationFilter.value)
  }

  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    result = result.filter(s =>
      s.studentId.toLowerCase().includes(q)
      || s.sessionId.toLowerCase().includes(q)
      || s.examId.toLowerCase().includes(q)
    )
  }

  if (selectedExamId.value) {
    result = result.filter(s => s.examId === selectedExamId.value)
  }

  return result
})

const examOptions = computed(() => {
  const exams = new Set(sessions.value.map(s => s.examId))
  const options = [{ label: 'Все экзамены', value: '' }]
  for (const examId of exams) {
    options.push({ label: examId, value: examId })
  }
  return options
})

// ---------------------------------------------------------------------------
// Focused Session
// ---------------------------------------------------------------------------

const focusedSession = computed(() => {
  if (!inspectorStore.focusedSessionId) return null
  return sessions.value.find(s => s.sessionId === inspectorStore.focusedSessionId) ?? null
})

// ---------------------------------------------------------------------------
// Grid Density
// ---------------------------------------------------------------------------

const densityOptions: { label: string, value: GridDensity, icon: string }[] = [
  { label: '2×2', value: '2x2', icon: 'i-lucide-grid-2x2' },
  { label: '3×3', value: '3x3', icon: 'i-lucide-grid-3x3' },
  { label: '4×4', value: '4x4', icon: 'i-lucide-layout-grid' },
  { label: '5×5', value: '5x5', icon: 'i-lucide-grip' }
]

// ---------------------------------------------------------------------------
// Keyboard Navigation
// ---------------------------------------------------------------------------

const totalCells = computed(() => filteredSessions.value.length)
useKeyboardNav(totalCells)

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

onMounted(async () => {
  await fetchActiveSessions()
  pollTimer = setInterval(fetchActiveSessions, 10_000)
  feedStore.startUpdates()
  inspectorStore.startRiskUpdates()
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  feedStore.stopUpdates()
  inspectorStore.stopRiskUpdates()
})
</script>

<template>
  <div class="flex flex-col h-[calc(100vh-64px)]">
    <!-- ===== HEADER BAR ===== -->
    <div
      class="shrink-0 px-5 py-3 border-b border-[var(--argus-border)]"
      style="background: var(--argus-bg-card);"
    >
      <div class="flex items-center justify-between gap-4">
        <!-- Title + stats -->
        <div class="flex items-center gap-4">
          <div>
            <h1
              class="text-lg font-bold"
              style="color: var(--argus-text);"
            >
              Инспектор
            </h1>
            <p
              class="text-[10px]"
              style="color: var(--argus-text-dimmed);"
            >
              {{ stats.totalActive }} активных сессий
              <span
                v-if="lastUpdated"
                class="ml-2 font-mono"
              >обн. {{ lastUpdated }}</span>
            </p>
          </div>

          <!-- Status badges -->
          <div class="flex items-center gap-2">
            <div
              class="flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full"
              :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
            >
              <span class="relative flex size-1.5">
                <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
                <span class="relative inline-flex size-1.5 rounded-full bg-red-500" />
              </span>
              {{ stats.totalCritical }}
            </div>
            <div
              class="flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full"
              :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
            >
              {{ stats.totalWarning }}
            </div>
            <div
              class="flex items-center gap-1.5 text-xs font-medium px-2.5 py-1 rounded-full"
              :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
            >
              {{ stats.totalClean }}
            </div>
          </div>
        </div>

        <!-- Controls (right side) -->
        <div class="flex items-center gap-3">
          <!-- Transport -->
          <TransportHealthIndicator />

          <!-- Exam filter -->
          <div class="w-44">
            <USelectMenu
              v-model="selectedExamId"
              :items="examOptions.map(o => ({ label: o.label, value: o.value }))"
              value-key="value"
              placeholder="Все экзамены"
              class="w-full"
            />
          </div>

          <!-- Search -->
          <div
            class="flex items-center gap-2 px-3 py-1.5 rounded-lg border transition-all w-56"
            :style="{
              background: 'var(--argus-bg-elevated)',
              borderColor: searchFocused ? 'var(--argus-accent)' : 'var(--argus-border)',
              boxShadow: searchFocused ? `0 0 0 2px ${accentBg(0.1)}` : 'none'
            }"
          >
            <UIcon
              name="i-heroicons-magnifying-glass"
              class="size-3.5 shrink-0"
              :style="{ color: searchFocused ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)' }"
            />
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Поиск..."
              class="w-full bg-transparent text-xs outline-none"
              :style="{ color: 'var(--argus-text)' }"
              @focus="searchFocused = true"
              @blur="searchFocused = false"
            >
          </div>

          <!-- Violation filter buttons -->
          <div class="flex items-center gap-1">
            <button
              v-for="filter in (['all', 'critical', 'warning'] as const)"
              :key="filter"
              class="px-2.5 py-1.5 rounded-md text-[10px] font-semibold transition-all border cursor-pointer"
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
              {{ filter === 'all' ? 'Все' : filter === 'critical' ? 'Крит.' : 'Пред.' }}
            </button>
          </div>

          <!-- Grid density toggle -->
          <div
            class="flex items-center gap-0.5 p-0.5 rounded-lg"
            style="background: var(--argus-bg-elevated);"
          >
            <button
              v-for="opt in densityOptions"
              :key="opt.value"
              class="p-1.5 rounded-md transition-all cursor-pointer"
              :style="{
                background: inspectorStore.gridDensity === opt.value ? accentBg(0.15) : 'transparent',
                color: inspectorStore.gridDensity === opt.value ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
              }"
              :title="opt.label"
              @click="inspectorStore.setGridDensity(opt.value)"
            >
              <UIcon
                :name="opt.icon"
                class="size-3.5"
              />
            </button>
          </div>

          <!-- Silence toggle -->
          <button
            class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all border cursor-pointer"
            :style="{
              background: inspectorStore.silenceMode ? warningBg(0.15) : 'transparent',
              color: inspectorStore.silenceMode ? 'var(--argus-warning)' : 'var(--argus-text-dimmed)',
              borderColor: inspectorStore.silenceMode ? warningBg(0.3) : 'var(--argus-border)'
            }"
            @click="inspectorStore.toggleSilence()"
          >
            <UIcon
              :name="inspectorStore.silenceMode ? 'i-lucide-bell-off' : 'i-lucide-bell'"
              class="size-3"
            />
          </button>
        </div>
      </div>
    </div>

    <!-- ===== MAIN CONTENT ===== -->
    <div class="flex-1 flex overflow-hidden">
      <!-- Loading -->
      <div
        v-if="loading"
        class="flex-1 flex flex-col items-center justify-center"
      >
        <div
          class="animate-spin rounded-full h-10 w-10 border-t-2 border-b-2 mb-4"
          style="border-color: var(--argus-accent);"
        />
        <p
          class="text-sm"
          style="color: var(--argus-text-dimmed);"
        >
          Загрузка сессий...
        </p>
      </div>

      <!-- Error -->
      <div
        v-else-if="error"
        class="flex-1 flex flex-col items-center justify-center"
      >
        <UIcon
          name="i-lucide-alert-circle"
          class="size-12 mb-3"
          style="color: var(--argus-error);"
        />
        <p
          class="text-sm font-medium"
          style="color: var(--argus-text);"
        >
          {{ error }}
        </p>
        <button
          class="mt-4 px-4 py-2 rounded-lg text-xs font-medium transition-all cursor-pointer"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          @click="loading = true; fetchActiveSessions()"
        >
          Повторить
        </button>
      </div>

      <!-- Main area -->
      <template v-else>
        <!-- Focus Mode (when a session is focused) -->
        <FocusModeView
          v-if="focusedSession"
          :session="focusedSession"
          :all-sessions="filteredSessions"
          class="flex-1"
        />

        <!-- Inspector Grid (default view) -->
        <div
          v-else
          class="flex-1 overflow-y-auto p-4"
        >
          <InspectorGrid
            :sessions="filteredSessions"
            @focus="(id: string) => inspectorStore.setFocus(id)"
          />
        </div>

        <!-- Incident Sidebar -->
        <IncidentSidebar />
      </template>
    </div>
  </div>
</template>
