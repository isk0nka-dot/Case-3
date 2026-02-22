<script setup lang="ts">
import { Pie, Bar, Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  ArcElement,
  BarElement,
  LineElement,
  PointElement,
  CategoryScale,
  LinearScale,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js'
import { useAdminAPI, type AnalyticsOverview } from '~/composables/useAdminAPI'
import { useAuthStore } from '~/stores/useAuthStore'

ChartJS.register(ArcElement, BarElement, LineElement, PointElement, CategoryScale, LinearScale, Tooltip, Legend, Filler)

const api = useAdminAPI()
const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()
const { formatTimeShort, formatCompactNumber } = useFormatters()
const toast = useToast()

// --- State ---
const loading = ref(true)
const error = ref<string | null>(null)
const data = ref<AnalyticsOverview | null>(null)
const autoRefresh = ref(true)
let refreshInterval: ReturnType<typeof setInterval> | null = null

// --- Demo data for offline/demo mode ---
const DEMO_DATA: AnalyticsOverview = {
  risk: { totalEvents: 84320, bySeverity: { critical: 1240, warning: 8750, info: 74330 }, honest: 72, suspicious: 21, cheaters: 7 },
  health: { totalFlushed: 84320, totalDropped: 12, flushCount: 843, flushErrors: 2, bufferSize: 256, overflowSize: 0, totalEvents: 84320, uniqueStudents: 1847, uniqueSessions: 2304, criticalEvents: 1240, eventsLast5Min: 420, eventsLast1Hour: 5840 },
  violations: [
    { eventType: 'face_absent', severity: 'critical', count: 432 },
    { eventType: 'tab_switch', severity: 'warning', count: 3210 },
    { eventType: 'multiple_faces', severity: 'critical', count: 248 },
    { eventType: 'gaze_deviation', severity: 'warning', count: 2180 },
    { eventType: 'audio_anomaly', severity: 'warning', count: 1840 },
  ],
  topStudents: [
    { studentId: 'STU-00482', totalEvents: 847, criticalCount: 12, warningCount: 34 },
    { studentId: 'STU-01923', totalEvents: 712, criticalCount: 9, warningCount: 28 },
    { studentId: 'STU-00731', totalEvents: 634, criticalCount: 7, warningCount: 21 },
  ],
  hourlyStats: Array.from({ length: 24 }, (_, i) => ({
    hour: `${String(i).padStart(2, '0')}:00`,
    eventCount: Math.floor(Math.random() * 4000 + 500),
    criticalCount: Math.floor(Math.random() * 80 + 10),
  })),
  generatedAt: new Date().toISOString(),
}

// --- Fetch Data ---
async function fetchData() {
  if (!authStore.isAuthenticated) {
    error.value = 'Unauthorized'
    loading.value = false
    return
  }
  try {
    error.value = null
    data.value = await api.getAnalyticsOverview()
  } catch (e: any) {
    // Backend offline or demo session — show demo data instead of error.
    if (!data.value) {
      data.value = DEMO_DATA
      console.info('[Executive] Backend unavailable, showing demo data.')
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchData()
  refreshInterval = setInterval(() => {
    if (autoRefresh.value) fetchData()
  }, 15000) // Auto-refresh every 15 seconds.
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})

// fmtNum/fmtTime → replaced by useFormatters() composable (formatCompactNumber, formatTimeShort)
const fmtNum = formatCompactNumber

function fmtPct(n: number): string {
  return n.toFixed(1) + '%'
}

const fmtTime = formatTimeShort

// --- Risk Distribution Pie Chart ---
const pieData = computed(() => {
  if (!data.value?.risk) return { labels: [], datasets: [] }
  const r = data.value.risk
  return {
    labels: ['Честные (INFO)', 'Подозрительные (WARNING)', 'Нарушители (CRITICAL)'],
    datasets: [{
      data: [
        r.bySeverity['info'] || 0,
        r.bySeverity['warning'] || 0,
        r.bySeverity['critical'] || 0,
      ],
      backgroundColor: [
        isDark.value ? 'rgba(52, 211, 153, 0.85)' : 'rgba(16, 163, 74, 0.85)',
        isDark.value ? 'rgba(251, 191, 36, 0.85)' : 'rgba(230, 126, 34, 0.85)',
        isDark.value ? 'rgba(248, 113, 113, 0.85)' : 'rgba(224, 62, 62, 0.85)',
      ],
      borderColor: isDark.value ? '#1A1F2E' : '#FFFFFF',
      borderWidth: 3,
      hoverOffset: 8,
    }],
  }
})

const pieOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: {
      position: 'bottom' as const,
      labels: {
        color: isDark.value ? '#CBD5E1' : '#334155',
        padding: 16,
        usePointStyle: true,
        pointStyleWidth: 12,
        font: { size: 12, family: 'DM Sans' },
      },
    },
    tooltip: {
      backgroundColor: isDark.value ? '#1E293B' : '#FFFFFF',
      titleColor: isDark.value ? '#F1F5F9' : '#0F172A',
      bodyColor: isDark.value ? '#CBD5E1' : '#475569',
      borderColor: isDark.value ? '#334155' : '#E2E8F0',
      borderWidth: 1,
      padding: 12,
      cornerRadius: 8,
      callbacks: {
        label: (ctx: any) => {
          const total = ctx.dataset.data.reduce((a: number, b: number) => a + b, 0)
          const pct = total > 0 ? ((ctx.raw / total) * 100).toFixed(1) : '0'
          return ` ${ctx.label}: ${fmtNum(ctx.raw)} (${pct}%)`
        },
      },
    },
  },
}))

// --- Hourly Timeline Chart ---
const hourlyData = computed(() => {
  if (!data.value?.hourlyStats?.length) return { labels: [], datasets: [] }
  const stats = [...data.value.hourlyStats].reverse() // Chronological order.
  return {
    labels: stats.map(s => fmtTime(s.hour)),
    datasets: [
      {
        label: 'Все события',
        data: stats.map(s => s.eventCount),
        borderColor: isDark.value ? '#38BDF8' : '#2563EB',
        backgroundColor: isDark.value ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)',
        fill: true,
        tension: 0.4,
        pointRadius: 3,
        pointHoverRadius: 6,
      },
      {
        label: 'Критические',
        data: stats.map(s => s.criticalCount),
        borderColor: isDark.value ? '#F87171' : '#E03E3E',
        backgroundColor: isDark.value ? 'rgba(248, 113, 113, 0.1)' : 'rgba(224, 62, 62, 0.1)',
        fill: true,
        tension: 0.4,
        borderDash: [5, 3],
        pointRadius: 3,
        pointHoverRadius: 6,
      },
    ],
  }
})

const hourlyOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: isDark.value ? '#CBD5E1' : '#334155',
        usePointStyle: true,
        font: { size: 11, family: 'DM Sans' },
      },
    },
    tooltip: {
      backgroundColor: isDark.value ? '#1E293B' : '#FFFFFF',
      titleColor: isDark.value ? '#F1F5F9' : '#0F172A',
      bodyColor: isDark.value ? '#CBD5E1' : '#475569',
      borderColor: isDark.value ? '#334155' : '#E2E8F0',
      borderWidth: 1,
      padding: 10,
      cornerRadius: 8,
    },
  },
  scales: {
    x: {
      ticks: { color: isDark.value ? '#64748B' : '#94A3B8', font: { size: 10 } },
      grid: { color: isDark.value ? 'rgba(51,65,85,0.3)' : 'rgba(203,213,225,0.5)' },
    },
    y: {
      ticks: { color: isDark.value ? '#64748B' : '#94A3B8', font: { size: 10 } },
      grid: { color: isDark.value ? 'rgba(51,65,85,0.3)' : 'rgba(203,213,225,0.5)' },
    },
  },
}))

// --- Top Violations Bar Chart ---
const violationsBarData = computed(() => {
  if (!data.value?.violations?.length) return { labels: [], datasets: [] }
  const items = data.value.violations.slice(0, 8)
  return {
    labels: items.map(v => v.eventType.replace(/_/g, ' ')),
    datasets: [{
      label: 'Count',
      data: items.map(v => v.count),
      backgroundColor: items.map(v =>
        v.severity === 'critical'
          ? (isDark.value ? 'rgba(248, 113, 113, 0.7)' : 'rgba(224, 62, 62, 0.7)')
          : (isDark.value ? 'rgba(251, 191, 36, 0.7)' : 'rgba(230, 126, 34, 0.7)')
      ),
      borderColor: items.map(v =>
        v.severity === 'critical'
          ? (isDark.value ? '#F87171' : '#E03E3E')
          : (isDark.value ? '#FBBF24' : '#E67E22')
      ),
      borderWidth: 1,
      borderRadius: 6,
    }],
  }
})

const violationsBarOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  indexAxis: 'y' as const,
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: isDark.value ? '#1E293B' : '#FFFFFF',
      titleColor: isDark.value ? '#F1F5F9' : '#0F172A',
      bodyColor: isDark.value ? '#CBD5E1' : '#475569',
      borderColor: isDark.value ? '#334155' : '#E2E8F0',
      borderWidth: 1,
      padding: 10,
      cornerRadius: 8,
    },
  },
  scales: {
    x: {
      ticks: { color: isDark.value ? '#64748B' : '#94A3B8', font: { size: 10 } },
      grid: { color: isDark.value ? 'rgba(51,65,85,0.3)' : 'rgba(203,213,225,0.5)' },
    },
    y: {
      ticks: { color: isDark.value ? '#CBD5E1' : '#475569', font: { size: 10 } },
      grid: { display: false },
    },
  },
}))

// --- Health Status ---
const healthStatus = computed(() => {
  if (!data.value?.health) return 'unknown'
  const h = data.value.health
  if (h.flushErrors > 0 || h.overflowSize > 0) return 'critical'
  if (h.totalDropped > 0 || h.bufferSize > 100) return 'warning'
  return 'healthy'
})

const healthStatusLabel = computed(() => {
  switch (healthStatus.value) {
    case 'healthy': return 'Система работает штатно'
    case 'warning': return 'Обнаружены предупреждения'
    case 'critical': return 'Критическое состояние'
    default: return 'Статус неизвестен'
  }
})

// Computed: events per second (last 5 min).
const eventsPerSec = computed(() => {
  if (!data.value?.health) return 0
  return Math.round(data.value.health.eventsLast5Min / 300)
})
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- Page Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold" style="color: var(--argus-text);">
          Executive Dashboard
        </h1>
        <p class="text-sm mt-1" style="color: var(--argus-text-dimmed);">
          Сводная аналитика прокторинга в реальном времени
        </p>
      </div>
      <div class="flex items-center gap-3">
        <!-- Auto-refresh Toggle -->
        <label class="flex items-center gap-2 text-xs cursor-pointer" style="color: var(--argus-text-dimmed);">
          <input v-model="autoRefresh" type="checkbox" class="rounded" />
          Автообновление (15с)
        </label>
        <!-- Refresh Button -->
        <UButton
          icon="i-lucide-refresh-cw"
          size="sm"
          variant="ghost"
          :loading="loading"
          @click="fetchData"
        />
        <!-- Generated time -->
        <span v-if="data?.generatedAt" class="text-xs" style="color: var(--argus-text-dimmed);">
          {{ fmtTime(data.generatedAt) }}
        </span>
      </div>
    </div>

    <!-- Error Banner -->
    <div
      v-if="error"
      class="p-4 rounded-xl border text-sm"
      :style="{ background: errorBg(0.1), borderColor: errorBg(0.3), color: 'var(--argus-error)' }"
    >
      <div class="flex items-center gap-2">
        <UIcon name="i-lucide-alert-triangle" class="text-lg" />
        <span>{{ error }}</span>
        <UButton size="xs" variant="ghost" @click="fetchData">Повторить</UButton>
      </div>
    </div>

    <!-- Loading Skeleton -->
    <div v-if="loading && !data" class="grid grid-cols-4 gap-4">
      <div v-for="i in 4" :key="i" class="h-24 rounded-xl animate-pulse" style="background: var(--argus-bg-card);" />
    </div>

    <!-- Main Content -->
    <template v-if="data">
      <!-- KPI Cards Row -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
        <!-- Total Events -->
        <div class="p-4 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center gap-2 mb-2">
            <div class="w-8 h-8 rounded-lg flex items-center justify-center" :style="{ background: accentBg(0.15) }">
              <UIcon name="i-lucide-activity" class="text-lg" :style="{ color: 'var(--argus-accent)' }" />
            </div>
            <span class="text-xs font-medium" style="color: var(--argus-text-dimmed);">Всего событий</span>
          </div>
          <div class="text-2xl font-bold" style="color: var(--argus-text);">{{ fmtNum(data.health.totalEvents) }}</div>
          <div class="text-xs mt-1" style="color: var(--argus-text-dimmed);">
            {{ eventsPerSec }} событий/сек
          </div>
        </div>

        <!-- Unique Students -->
        <div class="p-4 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center gap-2 mb-2">
            <div class="w-8 h-8 rounded-lg flex items-center justify-center" :style="{ background: successBg(0.15) }">
              <UIcon name="i-lucide-users" class="text-lg" :style="{ color: 'var(--argus-success)' }" />
            </div>
            <span class="text-xs font-medium" style="color: var(--argus-text-dimmed);">Студенты</span>
          </div>
          <div class="text-2xl font-bold" style="color: var(--argus-text);">{{ fmtNum(data.health.uniqueStudents) }}</div>
          <div class="text-xs mt-1" style="color: var(--argus-text-dimmed);">
            {{ fmtNum(data.health.uniqueSessions) }} сессий
          </div>
        </div>

        <!-- Critical Events -->
        <div class="p-4 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center gap-2 mb-2">
            <div class="w-8 h-8 rounded-lg flex items-center justify-center" :style="{ background: errorBg(0.15) }">
              <UIcon name="i-lucide-shield-alert" class="text-lg" :style="{ color: 'var(--argus-error)' }" />
            </div>
            <span class="text-xs font-medium" style="color: var(--argus-text-dimmed);">Критические</span>
          </div>
          <div class="text-2xl font-bold" :style="{ color: 'var(--argus-error)' }">{{ fmtNum(data.health.criticalEvents) }}</div>
          <div class="text-xs mt-1" style="color: var(--argus-text-dimmed);">
            {{ fmtPct(data.risk.cheaters) }} от общего
          </div>
        </div>

        <!-- System Health -->
        <div class="p-4 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center gap-2 mb-2">
            <div
              class="w-8 h-8 rounded-lg flex items-center justify-center"
              :style="{
                background: healthStatus === 'healthy' ? successBg(0.15) : healthStatus === 'warning' ? warningBg(0.15) : errorBg(0.15)
              }"
            >
              <UIcon
                :name="healthStatus === 'healthy' ? 'i-lucide-check-circle' : healthStatus === 'warning' ? 'i-lucide-alert-circle' : 'i-lucide-x-circle'"
                class="text-lg"
                :style="{ color: healthStatus === 'healthy' ? 'var(--argus-success)' : healthStatus === 'warning' ? 'var(--argus-warning)' : 'var(--argus-error)' }"
              />
            </div>
            <span class="text-xs font-medium" style="color: var(--argus-text-dimmed);">Здоровье</span>
          </div>
          <div class="text-sm font-semibold" :style="{
            color: healthStatus === 'healthy' ? 'var(--argus-success)' : healthStatus === 'warning' ? 'var(--argus-warning)' : 'var(--argus-error)'
          }">
            {{ healthStatusLabel }}
          </div>
          <div class="text-xs mt-1" style="color: var(--argus-text-dimmed);">
            Буфер: {{ data.health.bufferSize }} | Записано: {{ fmtNum(data.health.totalFlushed) }}
          </div>
        </div>
      </div>

      <!-- Charts Row -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Risk Distribution Pie Chart -->
        <div class="p-5 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center justify-between mb-4">
            <div>
              <h2 class="text-base font-semibold" style="color: var(--argus-text);">
                Распределение рисков
              </h2>
              <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
                По уровню серьёзности событий
              </p>
            </div>
            <UIcon name="i-lucide-pie-chart" class="text-xl" style="color: var(--argus-text-dimmed);" />
          </div>

          <!-- Pie Chart -->
          <div class="h-64">
            <Pie :data="pieData" :options="pieOptions" />
          </div>

          <!-- Percentage Breakdown -->
          <div class="grid grid-cols-3 gap-3 mt-4 pt-4 border-t" :style="{ borderColor: 'var(--argus-border)' }">
            <div class="text-center">
              <div class="text-lg font-bold" :style="{ color: 'var(--argus-success)' }">
                {{ fmtPct(data.risk.honest) }}
              </div>
              <div class="text-xs" style="color: var(--argus-text-dimmed);">Честные</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-bold" :style="{ color: 'var(--argus-warning)' }">
                {{ fmtPct(data.risk.suspicious) }}
              </div>
              <div class="text-xs" style="color: var(--argus-text-dimmed);">Подозрительные</div>
            </div>
            <div class="text-center">
              <div class="text-lg font-bold" :style="{ color: 'var(--argus-error)' }">
                {{ fmtPct(data.risk.cheaters) }}
              </div>
              <div class="text-xs" style="color: var(--argus-text-dimmed);">Нарушители</div>
            </div>
          </div>
        </div>

        <!-- Top Violations Bar Chart -->
        <div class="p-5 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center justify-between mb-4">
            <div>
              <h2 class="text-base font-semibold" style="color: var(--argus-text);">
                Топ нарушений
              </h2>
              <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
                Наиболее частые типы нарушений
              </p>
            </div>
            <UIcon name="i-lucide-bar-chart-horizontal" class="text-xl" style="color: var(--argus-text-dimmed);" />
          </div>

          <div v-if="data.violations.length" class="h-72">
            <Bar :data="violationsBarData" :options="violationsBarOptions" />
          </div>
          <div v-else class="h-72 flex items-center justify-center">
            <span class="text-sm" style="color: var(--argus-text-dimmed);">Нет данных о нарушениях</span>
          </div>
        </div>
      </div>

      <!-- Hourly Timeline -->
      <div class="p-5 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
        <div class="flex items-center justify-between mb-4">
          <div>
            <h2 class="text-base font-semibold" style="color: var(--argus-text);">
              Почасовая активность
            </h2>
            <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
              Динамика событий за последние 24 часа
            </p>
          </div>
          <UIcon name="i-lucide-trending-up" class="text-xl" style="color: var(--argus-text-dimmed);" />
        </div>

        <div v-if="data.hourlyStats.length" class="h-56">
          <Line :data="hourlyData" :options="hourlyOptions" />
        </div>
        <div v-else class="h-56 flex items-center justify-center">
          <span class="text-sm" style="color: var(--argus-text-dimmed);">Нет почасовых данных</span>
        </div>
      </div>

      <!-- Bottom Row: Students Risk + System Metrics -->
      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <!-- Top Risk Students Table -->
        <div class="p-5 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center justify-between mb-4">
            <div>
              <h2 class="text-base font-semibold" style="color: var(--argus-text);">
                Студенты в зоне риска
              </h2>
              <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
                Топ-10 по количеству критических событий
              </p>
            </div>
            <UIcon name="i-lucide-user-x" class="text-xl" style="color: var(--argus-text-dimmed);" />
          </div>

          <div v-if="data.topStudents.length" class="space-y-2">
            <div
              v-for="(student, idx) in data.topStudents"
              :key="student.studentId"
              class="flex items-center justify-between p-3 rounded-lg"
              :style="{ background: idx < 3 ? errorBg(0.05) : 'var(--argus-bg-hover)' }"
            >
              <div class="flex items-center gap-3">
                <span
                  class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-bold"
                  :style="{
                    background: idx < 3 ? errorBg(0.2) : accentBg(0.15),
                    color: idx < 3 ? 'var(--argus-error)' : 'var(--argus-accent)',
                  }"
                >
                  {{ idx + 1 }}
                </span>
                <span class="text-sm font-medium" style="color: var(--argus-text);">
                  {{ student.studentId }}
                </span>
              </div>

              <div class="flex items-center gap-4 text-xs">
                <span :style="{ color: 'var(--argus-error)' }">
                  {{ student.criticalCount }} крит.
                </span>
                <span :style="{ color: 'var(--argus-warning)' }">
                  {{ student.warningCount }} пред.
                </span>
                <span style="color: var(--argus-text-dimmed);">
                  {{ fmtNum(student.totalEvents) }} всего
                </span>
              </div>
            </div>
          </div>
          <div v-else class="py-8 text-center">
            <span class="text-sm" style="color: var(--argus-text-dimmed);">Нет данных о студентах</span>
          </div>
        </div>

        <!-- System Health Monitor -->
        <div class="p-5 rounded-xl border" :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }">
          <div class="flex items-center justify-between mb-4">
            <div>
              <h2 class="text-base font-semibold" style="color: var(--argus-text);">
                Здоровье системы
              </h2>
              <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
                Метрики ClickHouse Writer и пропускная способность
              </p>
            </div>
            <UIcon name="i-lucide-heart-pulse" class="text-xl" style="color: var(--argus-text-dimmed);" />
          </div>

          <div class="space-y-3">
            <!-- Flush Success Rate -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-database" class="text-base" style="color: var(--argus-accent);" />
                <span class="text-sm" style="color: var(--argus-text);">Записано в ClickHouse</span>
              </div>
              <span class="text-sm font-semibold" :style="{ color: 'var(--argus-success)' }">
                {{ fmtNum(data.health.totalFlushed) }}
              </span>
            </div>

            <!-- Flush Count -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-repeat" class="text-base" style="color: var(--argus-accent);" />
                <span class="text-sm" style="color: var(--argus-text);">Flush циклов</span>
              </div>
              <span class="text-sm font-semibold" style="color: var(--argus-text);">
                {{ fmtNum(data.health.flushCount) }}
              </span>
            </div>

            <!-- Flush Errors -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: data.health.flushErrors > 0 ? errorBg(0.05) : 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-alert-circle" class="text-base" :style="{ color: data.health.flushErrors > 0 ? 'var(--argus-error)' : 'var(--argus-text-dimmed)' }" />
                <span class="text-sm" style="color: var(--argus-text);">Ошибки записи</span>
              </div>
              <span class="text-sm font-semibold" :style="{ color: data.health.flushErrors > 0 ? 'var(--argus-error)' : 'var(--argus-success)' }">
                {{ data.health.flushErrors }}
              </span>
            </div>

            <!-- Buffer Size -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-layers" class="text-base" style="color: var(--argus-accent);" />
                <span class="text-sm" style="color: var(--argus-text);">Буфер (очередь)</span>
              </div>
              <span class="text-sm font-semibold" style="color: var(--argus-text);">
                {{ data.health.bufferSize }} событий
              </span>
            </div>

            <!-- Dropped -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: data.health.totalDropped > 0 ? errorBg(0.05) : 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-trash-2" class="text-base" :style="{ color: data.health.totalDropped > 0 ? 'var(--argus-error)' : 'var(--argus-text-dimmed)' }" />
                <span class="text-sm" style="color: var(--argus-text);">Потеряно событий</span>
              </div>
              <span class="text-sm font-semibold" :style="{ color: data.health.totalDropped > 0 ? 'var(--argus-error)' : 'var(--argus-success)' }">
                {{ data.health.totalDropped }}
              </span>
            </div>

            <!-- Events Last 5 Min -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-timer" class="text-base" style="color: var(--argus-accent);" />
                <span class="text-sm" style="color: var(--argus-text);">За последние 5 мин</span>
              </div>
              <span class="text-sm font-semibold" style="color: var(--argus-text);">
                {{ fmtNum(data.health.eventsLast5Min) }} ({{ eventsPerSec }}/сек)
              </span>
            </div>

            <!-- Events Last Hour -->
            <div class="flex items-center justify-between p-3 rounded-lg" :style="{ background: 'var(--argus-bg-hover)' }">
              <div class="flex items-center gap-2">
                <UIcon name="i-lucide-clock" class="text-base" style="color: var(--argus-accent);" />
                <span class="text-sm" style="color: var(--argus-text);">За последний час</span>
              </div>
              <span class="text-sm font-semibold" style="color: var(--argus-text);">
                {{ fmtNum(data.health.eventsLast1Hour) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>
