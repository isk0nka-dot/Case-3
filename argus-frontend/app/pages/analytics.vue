<script setup lang="ts">
import { useDashboardStore } from '~/stores/useDashboardStore'

const store = useDashboardStore()
const { isDark, accentBg, errorBg, successBg, warningBg, purpleBg } = useColors()
function cyanBg(opacity: number): string {
  return isDark.value ? `rgba(34, 211, 238, ${opacity})` : `rgba(6, 182, 212, ${opacity})`
}

// --- Time Period Filter ---
const timePeriod = ref<'daily' | 'weekly' | 'monthly'>('weekly')

// --- Radial Gauge SVG ---
function gaugeArc(percent: number, radius: number): string {
  const circumference = 2 * Math.PI * radius
  return `${(circumference * percent) / 100} ${circumference}`
}

// --- KPI Computeds ---
const globalIntegrity = computed(() => store.avgIntegrityScore)
const aiConfidence = computed(() => store.globalAiConfidence)
const uptimePercent = computed(() => store.systemHealthUptime)
const totalProctors = computed(() => store.activeProctors)
const proctorSpeed = computed(() => store.avgProctorSpeed)

// --- Violation chart data ---
const maxHourlyViolation = computed(() => {
  return Math.max(...store.hourlyViolations.map(h => h.phone + h.gaze + h.persons + h.tabs + h.audio))
})

const sortedSubjects = computed(() =>
  [...store.subjectViolationRates].sort((a, b) => b.avgViolationRate - a.avgViolationRate)
)
const maxSubjectRate = computed(() =>
  Math.max(...store.subjectViolationRates.map(s => s.avgViolationRate))
)

// --- Region map data ---
const sortedRegions = computed(() =>
  [...store.regions].sort((a, b) => b.students - a.students)
)
const maxRegionStudents = computed(() =>
  Math.max(...store.regions.map(r => r.students))
)

// --- Institution leaderboard ---
const sortedInstitutions = computed(() =>
  [...store.institutionRankings].sort((a, b) => b.avgIntegrity - a.avgIntegrity)
)

// --- Weekly trend helpers ---
const maxWeeklyValue = computed(() => {
  return Math.max(...store.weeklyTrends.map(w => Math.max(w.phone, w.gaze, w.voice, w.tabs, w.persons)))
})

// --- Monthly trend helpers ---
const maxMonthlyViolations = computed(() =>
  Math.max(...store.monthlyTrends.map(m => m.totalViolations))
)
const maxMonthlySessions = computed(() =>
  Math.max(...store.monthlyTrends.map(m => m.totalSessions))
)

// --- Export ---
const showExportMenu = ref(false)
const exportLoading = ref<string | null>(null)

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

function exportCSV(filename: string, headers: string[], rows: (string | number)[][]) {
  const lines = [headers.join(','), ...rows.map(r => r.map(v => `"${v}"`).join(','))]
  downloadBlob(new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8;' }), filename)
}

async function exportPDF() {
  exportLoading.value = 'pdf'
  showExportMenu.value = false
  await nextTick()
  window.print()
  exportLoading.value = null
}

function exportExcel() {
  exportLoading.value = 'excel'
  showExportMenu.value = false
  const rows = store.weeklyTrends.map((w, i) => {
    const days = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']
    return [days[i] ?? i, w.phone, w.gaze, w.voice, w.tabs, w.persons]
  })
  exportCSV(
    `argus-analytics-${new Date().toISOString().split('T')[0]}.csv`,
    ['День', 'Телефон', 'Взгляд', 'Голос', 'Вкладки', 'Люди'],
    rows
  )
  exportLoading.value = null
}

function exportYearlyReport() {
  exportLoading.value = 'yearly'
  showExportMenu.value = false
  const rows = store.monthlyTrends.map(m => [
    m.month, m.totalSessions, m.totalViolations,
    m.avgIntegrity.toFixed(2)
  ])
  exportCSV(
    `argus-yearly-report-2026.csv`,
    ['Месяц', 'Сессий', 'Нарушений', 'Средний индекс'],
    rows
  )
  exportLoading.value = null
}

// --- Detection accuracy average ---
const avgAccuracy = computed(() => {
  const total = store.detectionAccuracy.reduce((s, d) => s + d.accuracy, 0)
  return (total / store.detectionAccuracy.length).toFixed(1)
})

// --- Heatmap helpers ---
const heatmapDays = ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс']
const heatmapHours = ['06', '07', '08', '09', '10', '11', '12', '13', '14', '15', '16', '17']

// Simulated heatmap data (day x hour matrix)
const heatmapData = computed(() => {
  const data: number[][] = []
  const hourlyBase = store.hourlyViolations
  const weeklyBase = store.weeklyTrends
  for (let d = 0; d < 7; d++) {
    const row: number[] = []
    const wb = weeklyBase[d]
    const dayMultiplier = wb ? (wb.phone + wb.gaze) / 120 : 0.5
    for (let h = 0; h < 12; h++) {
      const hb = hourlyBase[h]
      const hourVal = hb ? (hb.phone + hb.gaze + hb.persons) : 0
      const value = Math.round(hourVal * dayMultiplier * (0.7 + Math.random() * 0.6))
      row.push(value)
    }
    data.push(row)
  }
  return data
})

const maxHeatmapValue = computed(() =>
  Math.max(...heatmapData.value.flat())
)

function heatColor(value: number): string {
  if (value === 0) return isDark.value ? 'rgba(30, 41, 59, 0.3)' : 'rgba(224, 234, 250, 0.85)'
  const ratio = value / maxHeatmapValue.value
  if (ratio >= 0.8) return isDark.value ? 'rgba(248, 113, 113, 0.85)' : 'rgba(224, 62, 62, 0.75)'
  if (ratio >= 0.6) return isDark.value ? 'rgba(251, 191, 36, 0.7)' : 'rgba(230, 126, 34, 0.6)'
  if (ratio >= 0.35) return isDark.value ? 'rgba(56, 189, 248, 0.5)' : 'rgba(37, 99, 235, 0.4)'
  return isDark.value ? 'rgba(56, 189, 248, 0.2)' : 'rgba(37, 99, 235, 0.15)'
}
function violationRateColor(rate: number): string {
  if (rate >= 5) return 'var(--argus-error)'
  if (rate >= 3.5) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

function trendIcon(trend: string): string {
  switch (trend) {
    case 'up': return 'i-lucide-trending-up'
    case 'down': return 'i-lucide-trending-down'
    default: return 'i-lucide-minus'
  }
}

function trendColor(trend: string, invert = false): string {
  if (invert) {
    switch (trend) {
      case 'up': return 'var(--argus-error)'
      case 'down': return 'var(--argus-success)'
      default: return 'var(--argus-text-dimmed)'
    }
  }
  switch (trend) {
    case 'up': return 'var(--argus-success)'
    case 'down': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- ========================================== -->
    <!--  HEADER + EXPORT                            -->
    <!-- ========================================== -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-xl"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-bar-chart-3"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <h1
              class="text-xl font-bold"
              style="color: var(--argus-text);"
            >
              Глобальная аналитика
            </h1>
            <p
              class="text-xs mt-0.5"
              style="color: var(--argus-text-dimmed);"
            >
              Стратегический обзор системы · {{ store.totalStudentsAllRegions.toLocaleString() }} студентов · 17 регионов
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <!-- Time period selector -->
        <div
          class="flex items-center rounded-lg border"
          :style="{ borderColor: 'var(--argus-border)' }"
        >
          <button
            v-for="period in ([{ value: 'daily', label: 'День' }, { value: 'weekly', label: 'Неделя' }, { value: 'monthly', label: 'Месяц' }] as const)"
            :key="period.value"
            class="px-3 py-1.5 text-[10px] font-bold transition-all"
            :style="{
              background: timePeriod === period.value ? accentBg(0.12) : 'transparent',
              color: timePeriod === period.value ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)',
              borderRight: '1px solid var(--argus-border)'
            }"
            @click="timePeriod = period.value"
          >
            {{ period.label }}
          </button>
        </div>

        <!-- Export button -->
        <div class="relative">
          <button
            class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
            :style="{ background: 'var(--argus-accent)', color: '#fff' }"
            @click="showExportMenu = !showExportMenu"
          >
            <UIcon
              name="i-lucide-download"
              class="size-3.5"
            />
            Экспорт аналитики
          </button>
          <!-- Export dropdown -->
          <Transition name="fade">
            <div
              v-if="showExportMenu"
              class="absolute right-0 top-12 w-52 rounded-xl border shadow-2xl z-50 py-2"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <button
                class="flex items-center gap-2 w-full px-4 py-2.5 text-xs font-medium transition-all text-left"
                style="color: var(--argus-text);"
                :disabled="exportLoading === 'pdf'"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                @click="exportPDF"
              >
                <UIcon
                  :name="exportLoading === 'pdf' ? 'i-lucide-loader-2' : 'i-lucide-file-text'"
                  class="size-4"
                  :class="exportLoading === 'pdf' ? 'animate-spin' : ''"
                  style="color: var(--argus-error);"
                />
                Скачать PDF отчёт
              </button>
              <button
                class="flex items-center gap-2 w-full px-4 py-2.5 text-xs font-medium transition-all text-left"
                style="color: var(--argus-text);"
                :disabled="exportLoading === 'excel'"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                @click="exportExcel"
              >
                <UIcon
                  :name="exportLoading === 'excel' ? 'i-lucide-loader-2' : 'i-lucide-table'"
                  class="size-4"
                  :class="exportLoading === 'excel' ? 'animate-spin' : ''"
                  style="color: var(--argus-success);"
                />
                Экспорт в Excel
              </button>
              <div
                class="border-t my-1"
                style="border-color: var(--argus-border);"
              />
              <button
                class="flex items-center gap-2 w-full px-4 py-2.5 text-xs font-medium transition-all text-left"
                style="color: var(--argus-text);"
                :disabled="exportLoading === 'yearly'"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                @click="exportYearlyReport"
              >
                <UIcon
                  :name="exportLoading === 'yearly' ? 'i-lucide-loader-2' : 'i-lucide-calendar'"
                  class="size-4"
                  :class="exportLoading === 'yearly' ? 'animate-spin' : ''"
                  style="color: var(--argus-accent);"
                />
                Годовой отчёт 2026
              </button>
            </div>
          </Transition>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  STRATEGIC KPI OVERVIEW (4 RADIAL GAUGES)  -->
    <!-- ========================================== -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
      <!-- Global Integrity Score -->
      <div
        class="relative rounded-2xl border p-5 flex flex-col items-center text-center overflow-hidden"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="absolute inset-0 opacity-5"
          :style="{ background: `radial-gradient(circle at 50% 0%, ${isDark ? '#34D399' : '#10A34A'}, transparent 70%)` }"
        />
        <div class="relative">
          <svg
            width="120"
            height="120"
            viewBox="0 0 120 120"
            class="transform -rotate-90"
          >
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              :stroke="isDark ? 'rgba(30, 41, 59, 0.5)' : 'rgba(193, 207, 232, 0.6)'"
              stroke-width="8"
            />
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              stroke="url(#gaugeGreen)"
              stroke-width="8"
              stroke-linecap="round"
              :stroke-dasharray="gaugeArc(globalIntegrity, 52)"
            />
            <defs>
              <linearGradient
                id="gaugeGreen"
                x1="0%"
                y1="0%"
                x2="100%"
                y2="0%"
              >
                <stop
                  offset="0%"
                  :stop-color="isDark ? '#34D399' : '#10A34A'"
                />
                <stop
                  offset="100%"
                  :stop-color="isDark ? '#10B981' : '#0D8A3E'"
                />
              </linearGradient>
            </defs>
          </svg>
          <div class="absolute inset-0 flex flex-col items-center justify-center">
            <span
              class="text-2xl font-black tabular-nums"
              style="color: var(--argus-success);"
            >{{ globalIntegrity }}%</span>
          </div>
        </div>
        <p
          class="text-[11px] font-bold mt-3"
          style="color: var(--argus-text);"
        >
          Глобальная честность
        </p>
        <p
          class="text-[9px] mt-0.5"
          style="color: var(--argus-text-dimmed);"
        >
          Средний Integrity Score
        </p>
        <div class="flex items-center gap-1 mt-2">
          <UIcon
            name="i-lucide-trending-up"
            class="size-3"
            style="color: var(--argus-success);"
          />
          <span
            class="text-[9px] font-bold"
            style="color: var(--argus-success);"
          >+2.3% за неделю</span>
        </div>
      </div>

      <!-- AI Confidence Level -->
      <div
        class="relative rounded-2xl border p-5 flex flex-col items-center text-center overflow-hidden"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="absolute inset-0 opacity-5"
          :style="{ background: `radial-gradient(circle at 50% 0%, ${isDark ? '#38BDF8' : '#2563EB'}, transparent 70%)` }"
        />
        <div class="relative">
          <svg
            width="120"
            height="120"
            viewBox="0 0 120 120"
            class="transform -rotate-90"
          >
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              :stroke="isDark ? 'rgba(30, 41, 59, 0.5)' : 'rgba(193, 207, 232, 0.6)'"
              stroke-width="8"
            />
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              stroke="url(#gaugeCyan)"
              stroke-width="8"
              stroke-linecap="round"
              :stroke-dasharray="gaugeArc(aiConfidence, 52)"
            />
            <defs>
              <linearGradient
                id="gaugeCyan"
                x1="0%"
                y1="0%"
                x2="100%"
                y2="0%"
              >
                <stop
                  offset="0%"
                  :stop-color="isDark ? '#22D3EE' : '#06B6D4'"
                />
                <stop
                  offset="100%"
                  :stop-color="isDark ? '#38BDF8' : '#2563EB'"
                />
              </linearGradient>
            </defs>
          </svg>
          <div class="absolute inset-0 flex flex-col items-center justify-center">
            <span
              class="text-2xl font-black tabular-nums"
              style="color: var(--argus-accent);"
            >{{ aiConfidence }}%</span>
          </div>
        </div>
        <p
          class="text-[11px] font-bold mt-3"
          style="color: var(--argus-text);"
        >
          Точность ИИ
        </p>
        <p
          class="text-[9px] mt-0.5"
          style="color: var(--argus-text-dimmed);"
        >
          AI Confidence Level
        </p>
        <div class="flex items-center gap-1 mt-2">
          <UIcon
            name="i-lucide-brain"
            class="size-3"
            style="color: var(--argus-accent);"
          />
          <span
            class="text-[9px] font-bold"
            style="color: var(--argus-accent);"
          >{{ avgAccuracy }}% точность</span>
        </div>
      </div>

      <!-- System Uptime -->
      <div
        class="relative rounded-2xl border p-5 flex flex-col items-center text-center overflow-hidden"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="absolute inset-0 opacity-5"
          :style="{ background: `radial-gradient(circle at 50% 0%, ${isDark ? '#A78BFA' : '#7C3AED'}, transparent 70%)` }"
        />
        <div class="relative">
          <svg
            width="120"
            height="120"
            viewBox="0 0 120 120"
            class="transform -rotate-90"
          >
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              :stroke="isDark ? 'rgba(30, 41, 59, 0.5)' : 'rgba(193, 207, 232, 0.6)'"
              stroke-width="8"
            />
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              stroke="url(#gaugePurple)"
              stroke-width="8"
              stroke-linecap="round"
              :stroke-dasharray="gaugeArc(uptimePercent, 52)"
            />
            <defs>
              <linearGradient
                id="gaugePurple"
                x1="0%"
                y1="0%"
                x2="100%"
                y2="0%"
              >
                <stop
                  offset="0%"
                  :stop-color="isDark ? '#A78BFA' : '#7C3AED'"
                />
                <stop
                  offset="100%"
                  :stop-color="isDark ? '#C4B5FD' : '#8B5CF6'"
                />
              </linearGradient>
            </defs>
          </svg>
          <div class="absolute inset-0 flex flex-col items-center justify-center">
            <span
              class="text-2xl font-black tabular-nums"
              :style="{ color: isDark ? '#A78BFA' : '#7C3AED' }"
            >{{ uptimePercent }}%</span>
          </div>
        </div>
        <p
          class="text-[11px] font-bold mt-3"
          style="color: var(--argus-text);"
        >
          Аптайм системы
        </p>
        <p
          class="text-[9px] mt-0.5"
          style="color: var(--argus-text-dimmed);"
        >
          {{ store.serverNodes.length }} серверов · {{ store.systemHealthLabel }}
        </p>
        <div class="flex items-center gap-1 mt-2">
          <UIcon
            name="i-lucide-server"
            class="size-3"
            :style="{ color: isDark ? '#A78BFA' : '#7C3AED' }"
          />
          <span
            class="text-[9px] font-bold"
            :style="{ color: isDark ? '#A78BFA' : '#7C3AED' }"
          >{{ store.loadPercent }}% нагрузка</span>
        </div>
      </div>

      <!-- Proctor Activity -->
      <div
        class="relative rounded-2xl border p-5 flex flex-col items-center text-center overflow-hidden"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="absolute inset-0 opacity-5"
          :style="{ background: `radial-gradient(circle at 50% 0%, ${isDark ? '#FBBF24' : '#E67E22'}, transparent 70%)` }"
        />
        <div class="relative">
          <svg
            width="120"
            height="120"
            viewBox="0 0 120 120"
            class="transform -rotate-90"
          >
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              :stroke="isDark ? 'rgba(30, 41, 59, 0.5)' : 'rgba(193, 207, 232, 0.6)'"
              stroke-width="8"
            />
            <circle
              cx="60"
              cy="60"
              r="52"
              fill="none"
              stroke="url(#gaugeAmber)"
              stroke-width="8"
              stroke-linecap="round"
              :stroke-dasharray="gaugeArc(Number(store.proctorAvgWarningAccuracy), 52)"
            />
            <defs>
              <linearGradient
                id="gaugeAmber"
                x1="0%"
                y1="0%"
                x2="100%"
                y2="0%"
              >
                <stop
                  offset="0%"
                  :stop-color="isDark ? '#FBBF24' : '#E67E22'"
                />
                <stop
                  offset="100%"
                  :stop-color="isDark ? '#F59E0B' : '#C96E1A'"
                />
              </linearGradient>
            </defs>
          </svg>
          <div class="absolute inset-0 flex flex-col items-center justify-center">
            <span
              class="text-2xl font-black tabular-nums"
              style="color: var(--argus-warning);"
            >{{ totalProctors }}</span>
            <span
              class="text-[8px] font-bold"
              style="color: var(--argus-text-dimmed);"
            >активных</span>
          </div>
        </div>
        <p
          class="text-[11px] font-bold mt-3"
          style="color: var(--argus-text);"
        >
          Активность прокторов
        </p>
        <p
          class="text-[9px] mt-0.5"
          style="color: var(--argus-text-dimmed);"
        >
          Средняя скорость: {{ proctorSpeed }}с
        </p>
        <div class="flex items-center gap-1 mt-2">
          <UIcon
            name="i-lucide-zap"
            class="size-3"
            style="color: var(--argus-warning);"
          />
          <span
            class="text-[9px] font-bold"
            style="color: var(--argus-warning);"
          >{{ store.proctorAvgWarningAccuracy }}% точность</span>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  VIOLATION INTELLIGENCE & TRENDS            -->
    <!-- ========================================== -->
    <div class="grid grid-cols-1 xl:grid-cols-3 gap-4">
      <!-- Multi-Line Trend Chart (2 cols) -->
      <div
        class="xl:col-span-2 rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center justify-between mb-5">
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-trending-up"
              class="size-4"
              style="color: var(--argus-accent);"
            />
            <h3
              class="text-sm font-bold"
              style="color: var(--argus-text);"
            >
              Динамика нарушений
            </h3>
          </div>
          <!-- Legend -->
          <div class="flex items-center gap-3">
            <div class="flex items-center gap-1">
              <span
                class="size-2 rounded-full"
                style="background: var(--argus-error);"
              />
              <span
                class="text-[9px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >Телефон</span>
            </div>
            <div class="flex items-center gap-1">
              <span
                class="size-2 rounded-full"
                style="background: var(--argus-accent);"
              />
              <span
                class="text-[9px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >Взгляд</span>
            </div>
            <div class="flex items-center gap-1">
              <span
                class="size-2 rounded-full"
                style="background: var(--argus-warning);"
              />
              <span
                class="text-[9px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >Голос</span>
            </div>
            <div class="flex items-center gap-1">
              <span
                class="size-2 rounded-full"
                :style="{ background: isDark ? '#A78BFA' : '#7C3AED' }"
              />
              <span
                class="text-[9px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >Вкладки</span>
            </div>
          </div>
        </div>

        <!-- DAILY: Hourly violations chart -->
        <div
          v-if="timePeriod === 'daily'"
          class="space-y-3"
        >
          <div
            v-for="hour in store.hourlyViolations"
            :key="hour.hour"
            class="space-y-1"
          >
            <div class="flex items-center gap-2">
              <span
                class="text-[9px] font-mono font-bold w-10 shrink-0"
                style="color: var(--argus-text-dimmed);"
              >{{ hour.hour }}</span>
              <div class="flex-1 flex items-center gap-0.5 h-4">
                <div
                  class="h-full rounded-l transition-all"
                  :style="{ width: `${(hour.phone / (maxHourlyViolation || 1)) * 100}%`, background: 'var(--argus-error)' }"
                />
                <div
                  class="h-full transition-all"
                  :style="{ width: `${(hour.gaze / (maxHourlyViolation || 1)) * 100}%`, background: 'var(--argus-accent)' }"
                />
                <div
                  class="h-full transition-all"
                  :style="{ width: `${(hour.audio / (maxHourlyViolation || 1)) * 100}%`, background: 'var(--argus-warning)' }"
                />
                <div
                  class="h-full rounded-r transition-all"
                  :style="{ width: `${(hour.tabs / (maxHourlyViolation || 1)) * 100}%`, background: isDark ? '#A78BFA' : '#7C3AED' }"
                />
              </div>
              <span
                class="text-[9px] font-bold tabular-nums w-8 text-right"
                style="color: var(--argus-text-muted);"
              >
                {{ hour.phone + hour.gaze + hour.audio + hour.tabs }}
              </span>
            </div>
          </div>
        </div>

        <!-- WEEKLY: Weekly bar chart -->
        <div
          v-else-if="timePeriod === 'weekly'"
          class="flex items-end gap-3 h-48 px-2"
        >
          <div
            v-for="week in store.weeklyTrends"
            :key="week.day"
            class="flex-1 flex flex-col items-center gap-1"
          >
            <div
              class="w-full flex flex-col gap-0.5 items-center"
              style="height: 160px;"
            >
              <div class="w-full flex-1 flex items-end gap-[2px]">
                <div
                  class="flex-1 rounded-t transition-all"
                  :style="{ height: `${(week.phone / (maxWeeklyValue || 1)) * 100}%`, background: 'var(--argus-error)', minHeight: '2px' }"
                />
                <div
                  class="flex-1 rounded-t transition-all"
                  :style="{ height: `${(week.gaze / (maxWeeklyValue || 1)) * 100}%`, background: 'var(--argus-accent)', minHeight: '2px' }"
                />
                <div
                  class="flex-1 rounded-t transition-all"
                  :style="{ height: `${(week.voice / (maxWeeklyValue || 1)) * 100}%`, background: 'var(--argus-warning)', minHeight: '2px' }"
                />
                <div
                  class="flex-1 rounded-t transition-all"
                  :style="{ height: `${(week.tabs / (maxWeeklyValue || 1)) * 100}%`, background: isDark ? '#A78BFA' : '#7C3AED', minHeight: '2px' }"
                />
              </div>
            </div>
            <span
              class="text-[9px] font-bold"
              style="color: var(--argus-text-dimmed);"
            >{{ week.day }}</span>
          </div>
        </div>

        <!-- MONTHLY: Monthly overview -->
        <div
          v-else
          class="space-y-3"
        >
          <div
            v-for="m in store.monthlyTrends"
            :key="m.month"
            class="flex items-center gap-3"
          >
            <span
              class="text-[10px] font-bold w-10 shrink-0"
              style="color: var(--argus-text-dimmed);"
            >{{ m.month }}</span>
            <div
              class="flex-1 h-6 rounded-lg overflow-hidden relative"
              :style="{ background: 'var(--argus-bg-elevated)' }"
            >
              <div
                class="h-full rounded-lg transition-all flex items-center justify-end px-2"
                :style="{ width: `${(m.totalViolations / (maxMonthlyViolations || 1)) * 100}%`, background: `linear-gradient(90deg, ${accentBg(0.3)}, ${accentBg(0.6)})` }"
              >
                <span
                  class="text-[8px] font-bold"
                  style="color: var(--argus-accent);"
                >{{ m.totalViolations }}</span>
              </div>
            </div>
            <div class="flex items-center gap-1 w-20 shrink-0">
              <span
                class="text-[9px] font-bold"
                :style="{ color: integrityColor(m.avgIntegrity) }"
              >{{ m.avgIntegrity }}%</span>
              <span
                class="text-[8px]"
                style="color: var(--argus-text-dimmed);"
              >честность</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Category Breakdown (1 col) -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center gap-2 mb-4">
          <UIcon
            name="i-lucide-pie-chart"
            class="size-4"
            style="color: var(--argus-accent);"
          />
          <h3
            class="text-sm font-bold"
            style="color: var(--argus-text);"
          >
            Категории нарушений
          </h3>
        </div>
        <div class="space-y-3">
          <div
            v-for="cat in store.violationCategories"
            :key="cat.type"
            class="flex items-center gap-3"
          >
            <div
              class="flex items-center justify-center size-8 rounded-lg shrink-0"
              :style="{ background: cat.severity === 'critical' ? errorBg(0.1) : cat.severity === 'warning' ? warningBg(0.1) : accentBg(0.1) }"
            >
              <UIcon
                :name="cat.icon"
                class="size-3.5"
                :style="{ color: cat.severity === 'critical' ? 'var(--argus-error)' : cat.severity === 'warning' ? 'var(--argus-warning)' : 'var(--argus-accent)' }"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between mb-1">
                <span
                  class="text-[10px] font-semibold truncate"
                  style="color: var(--argus-text);"
                >{{ cat.type }}</span>
                <span
                  class="text-[10px] font-bold tabular-nums"
                  style="color: var(--argus-text);"
                >{{ cat.count }}</span>
              </div>
              <div
                class="h-1.5 rounded-full overflow-hidden"
                :style="{ background: 'var(--argus-bg-elevated)' }"
              >
                <div
                  class="h-full rounded-full transition-all"
                  :style="{
                    width: `${(cat.count / store.totalViolationsToday) * 100}%`,
                    background: cat.severity === 'critical' ? 'var(--argus-error)' : cat.severity === 'warning' ? 'var(--argus-warning)' : 'var(--argus-accent)'
                  }"
                />
              </div>
              <div class="flex items-center gap-1 mt-1">
                <UIcon
                  :name="cat.trend === 'up' ? 'i-lucide-arrow-up-right' : 'i-lucide-arrow-down-right'"
                  class="size-2.5"
                  :style="{ color: cat.trend === 'up' ? 'var(--argus-error)' : 'var(--argus-success)' }"
                />
                <span
                  class="text-[8px] font-bold"
                  :style="{ color: cat.trend === 'up' ? 'var(--argus-error)' : 'var(--argus-success)' }"
                >
                  {{ cat.percentChange > 0 ? '+' : '' }}{{ cat.percentChange }}%
                </span>
              </div>
            </div>
          </div>
        </div>

        <div
          class="mt-4 pt-3 border-t"
          style="border-color: var(--argus-border-subtle);"
        >
          <div class="flex items-center justify-between">
            <span
              class="text-[10px] font-semibold"
              style="color: var(--argus-text-dimmed);"
            >Всего за день</span>
            <span
              class="text-sm font-black tabular-nums"
              style="color: var(--argus-text);"
            >{{ store.totalViolationsToday.toLocaleString() }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  PEAK HOUR HEATMAP + SUBJECT BREAKDOWN      -->
    <!-- ========================================== -->
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <!-- Peak Hour Heatmap -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-flame"
              class="size-4"
              style="color: var(--argus-warning);"
            />
            <h3
              class="text-sm font-bold"
              style="color: var(--argus-text);"
            >
              Тепловая карта нарушений
            </h3>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="text-[8px]"
              style="color: var(--argus-text-dimmed);"
            >Мин</span>
            <div class="flex items-center gap-0.5">
              <div
                class="w-3 h-2 rounded-sm"
                :style="{ background: isDark ? 'rgba(30, 41, 59, 0.3)' : 'rgba(224, 234, 250, 0.85)' }"
              />
              <div
                class="w-3 h-2 rounded-sm"
                :style="{ background: accentBg(0.2) }"
              />
              <div
                class="w-3 h-2 rounded-sm"
                :style="{ background: accentBg(0.5) }"
              />
              <div
                class="w-3 h-2 rounded-sm"
                :style="{ background: warningBg(0.7) }"
              />
              <div
                class="w-3 h-2 rounded-sm"
                :style="{ background: errorBg(0.85) }"
              />
            </div>
            <span
              class="text-[8px]"
              style="color: var(--argus-text-dimmed);"
            >Макс</span>
          </div>
        </div>

        <!-- Hour labels row -->
        <div
          class="grid gap-1"
          :style="{ gridTemplateColumns: `40px repeat(${heatmapHours.length}, 1fr)` }"
        >
          <div />
          <div
            v-for="hour in heatmapHours"
            :key="hour"
            class="text-center"
          >
            <span
              class="text-[8px] font-mono font-bold"
              style="color: var(--argus-text-dimmed);"
            >{{ hour }}</span>
          </div>
        </div>

        <!-- Heatmap rows -->
        <div
          v-for="(row, dayIdx) in heatmapData"
          :key="dayIdx"
          class="grid gap-1 mt-1"
          :style="{ gridTemplateColumns: `40px repeat(${heatmapHours.length}, 1fr)` }"
        >
          <div class="flex items-center">
            <span
              class="text-[9px] font-bold"
              style="color: var(--argus-text-dimmed);"
            >{{ heatmapDays[dayIdx] }}</span>
          </div>
          <div
            v-for="(val, hourIdx) in row"
            :key="hourIdx"
            class="aspect-square rounded-md flex items-center justify-center transition-all cursor-default"
            :style="{ background: heatColor(val) }"
            :title="`${heatmapDays[dayIdx]} ${heatmapHours[hourIdx]}:00 — ${val} нарушений`"
          >
            <span
              v-if="val > 0"
              class="text-[7px] font-bold tabular-nums"
              :style="{ color: val / maxHeatmapValue >= 0.6 ? '#fff' : 'var(--argus-text-dimmed)', opacity: val > 0 ? 1 : 0 }"
            >
              {{ val }}
            </span>
          </div>
        </div>

        <p
          class="text-[9px] mt-3"
          style="color: var(--argus-text-dimmed);"
        >
          Пиковые часы: <span
            class="font-bold"
            style="color: var(--argus-warning);"
          >09:00–11:00</span> и <span
            class="font-bold"
            style="color: var(--argus-warning);"
          >13:00–15:00</span>
        </p>
      </div>

      <!-- Subject Violation Rates (Horizontal Bar Chart) -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center gap-2 mb-4">
          <UIcon
            name="i-lucide-book-open"
            class="size-4"
            style="color: var(--argus-accent);"
          />
          <h3
            class="text-sm font-bold"
            style="color: var(--argus-text);"
          >
            Нарушения по предметам
          </h3>
        </div>

        <div class="space-y-2.5">
          <div
            v-for="subj in sortedSubjects"
            :key="subj.subject"
            class="flex items-center gap-3"
          >
            <span
              class="text-[10px] font-medium w-24 shrink-0 truncate"
              style="color: var(--argus-text-muted);"
            >{{ subj.subject }}</span>
            <div
              class="flex-1 h-5 rounded-lg overflow-hidden relative"
              :style="{ background: 'var(--argus-bg-elevated)' }"
            >
              <div
                class="h-full rounded-lg transition-all flex items-center px-2"
                :style="{
                  width: `${(subj.avgViolationRate / (maxSubjectRate || 1)) * 100}%`,
                  background: subj.avgViolationRate >= 5 ? `linear-gradient(90deg, ${errorBg(0.4)}, ${errorBg(0.7)})` : subj.avgViolationRate >= 3.5 ? `linear-gradient(90deg, ${warningBg(0.3)}, ${warningBg(0.6)})` : `linear-gradient(90deg, ${successBg(0.3)}, ${successBg(0.5)})`
                }"
              >
                <span
                  class="text-[8px] font-bold whitespace-nowrap"
                  :style="{ color: violationRateColor(subj.avgViolationRate) }"
                >
                  {{ subj.avgViolationRate }}%
                </span>
              </div>
            </div>
            <div class="flex items-center gap-1 w-16 shrink-0 justify-end">
              <span
                class="text-[9px] font-bold tabular-nums"
                style="color: var(--argus-text-dimmed);"
              >{{ subj.totalViolations }}</span>
              <span
                class="text-[7px]"
                style="color: var(--argus-text-dimmed);"
              >нар.</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  AI DETECTION ACCURACY + PROCTOR KPIS       -->
    <!-- ========================================== -->
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <!-- Detection Accuracy -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center gap-2 mb-4">
          <UIcon
            name="i-lucide-crosshair"
            class="size-4"
            style="color: var(--argus-accent);"
          />
          <h3
            class="text-sm font-bold"
            style="color: var(--argus-text);"
          >
            Точность детекции ИИ
          </h3>
        </div>

        <div class="space-y-3">
          <div
            v-for="det in store.detectionAccuracy"
            :key="det.type"
          >
            <div class="flex items-center justify-between mb-1">
              <span
                class="text-[10px] font-medium"
                style="color: var(--argus-text-muted);"
              >{{ det.type }}</span>
              <div class="flex items-center gap-2">
                <span
                  class="text-[9px] font-bold"
                  :style="{ color: det.accuracy >= 95 ? 'var(--argus-success)' : det.accuracy >= 90 ? 'var(--argus-accent)' : 'var(--argus-warning)' }"
                >
                  {{ det.accuracy }}%
                </span>
              </div>
            </div>
            <div
              class="h-2 rounded-full overflow-hidden"
              :style="{ background: 'var(--argus-bg-elevated)' }"
            >
              <div class="h-full flex">
                <div
                  class="h-full transition-all"
                  :style="{
                    width: `${det.truePositive}%`,
                    background: 'var(--argus-success)',
                    borderRadius: '9999px 0 0 9999px'
                  }"
                />
                <div
                  class="h-full transition-all"
                  :style="{
                    width: `${det.falsePositive}%`,
                    background: 'var(--argus-error)',
                    borderRadius: '0 9999px 9999px 0'
                  }"
                />
              </div>
            </div>
            <div class="flex items-center gap-3 mt-1">
              <span
                class="text-[8px]"
                style="color: var(--argus-success);"
              >TP: {{ det.truePositive }}%</span>
              <span
                class="text-[8px]"
                style="color: var(--argus-error);"
              >FP: {{ det.falsePositive }}%</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Proctor KPIs -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-users"
              class="size-4"
              style="color: var(--argus-warning);"
            />
            <h3
              class="text-sm font-bold"
              style="color: var(--argus-text);"
            >
              Эффективность прокторов
            </h3>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="text-[9px] font-bold px-2 py-0.5 rounded"
              :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
            >
              Сред. реакция: {{ store.proctorAvgReactionTime }}с
            </span>
          </div>
        </div>

        <div class="space-y-2">
          <div
            v-for="proctor in store.proctorKPIs"
            :key="proctor.id"
            class="flex items-center gap-3 p-2.5 rounded-xl transition-all"
            :style="{ background: 'var(--argus-bg-elevated)' }"
          >
            <div
              class="flex items-center justify-center size-8 rounded-lg shrink-0"
              :style="{ background: proctor.warningAccuracy >= 96 ? successBg(0.1) : proctor.warningAccuracy >= 90 ? accentBg(0.1) : warningBg(0.1) }"
            >
              <UIcon
                name="i-lucide-user"
                class="size-3.5"
                :style="{ color: proctor.warningAccuracy >= 96 ? 'var(--argus-success)' : proctor.warningAccuracy >= 90 ? 'var(--argus-accent)' : 'var(--argus-warning)' }"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between">
                <span
                  class="text-[10px] font-semibold truncate"
                  style="color: var(--argus-text);"
                >{{ proctor.name }}</span>
                <span
                  class="text-[9px] font-bold tabular-nums"
                  :style="{ color: proctor.warningAccuracy >= 96 ? 'var(--argus-success)' : proctor.warningAccuracy >= 90 ? 'var(--argus-accent)' : 'var(--argus-warning)' }"
                >
                  {{ proctor.warningAccuracy }}%
                </span>
              </div>
              <div class="flex items-center gap-2 mt-0.5">
                <span
                  class="text-[8px]"
                  style="color: var(--argus-text-dimmed);"
                >{{ proctor.sessionsReviewed }} сессий</span>
                <span
                  class="text-[7px]"
                  style="color: var(--argus-border);"
                >·</span>
                <span
                  class="text-[8px]"
                  style="color: var(--argus-text-dimmed);"
                >{{ proctor.avgReactionTimeSec }}с реакция</span>
                <span
                  class="text-[7px]"
                  style="color: var(--argus-border);"
                >·</span>
                <span
                  class="text-[8px]"
                  style="color: var(--argus-text-dimmed);"
                >{{ proctor.shift }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  REGIONAL MAP + INSTITUTION LEADERBOARD     -->
    <!-- ========================================== -->
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-4">
      <!-- Regional Violation Density -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-map-pin"
              class="size-4"
              style="color: var(--argus-accent);"
            />
            <h3
              class="text-sm font-bold"
              style="color: var(--argus-text);"
            >
              Региональный срез
            </h3>
          </div>
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >
            Средний показатель нарушений: <span
              class="font-bold"
              style="color: var(--argus-accent);"
            >{{ store.avgViolationRateAllRegions }}%</span>
          </span>
        </div>

        <div class="space-y-2 max-h-96 overflow-y-auto pr-1">
          <div
            v-for="(region, idx) in sortedRegions"
            :key="region.id"
            class="flex items-center gap-3 p-2.5 rounded-xl transition-all"
            :style="{ background: 'var(--argus-bg-elevated)' }"
          >
            <!-- Rank -->
            <span
              class="flex items-center justify-center size-6 rounded-full text-[9px] font-black shrink-0"
              :style="{
                background: idx < 3 ? accentBg(0.12) : 'var(--argus-bg-hover)',
                color: idx < 3 ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
              }"
            >
              {{ idx + 1 }}
            </span>

            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between mb-1">
                <span
                  class="text-[10px] font-semibold truncate"
                  style="color: var(--argus-text);"
                >{{ region.name }}</span>
                <div class="flex items-center gap-2">
                  <span
                    class="text-[9px] font-bold tabular-nums"
                    :style="{ color: integrityColor(region.avgIntegrity) }"
                  >{{ region.avgIntegrity }}%</span>
                  <span
                    class="text-[8px] font-bold px-1.5 py-0.5 rounded"
                    :style="{
                      background: region.violationRate >= 4.5 ? errorBg(0.1) : region.violationRate >= 3.5 ? warningBg(0.1) : successBg(0.1),
                      color: violationRateColor(region.violationRate)
                    }"
                  >
                    {{ region.violationRate }}%
                  </span>
                </div>
              </div>
              <div
                class="h-1.5 rounded-full overflow-hidden"
                :style="{ background: 'var(--argus-bg-hover)' }"
              >
                <div
                  class="h-full rounded-full transition-all"
                  :style="{ width: `${(region.students / (maxRegionStudents || 1)) * 100}%`, background: `linear-gradient(90deg, ${accentBg(0.4)}, ${accentBg(0.7)})` }"
                />
              </div>
              <div class="flex items-center gap-2 mt-1">
                <span
                  class="text-[8px]"
                  style="color: var(--argus-text-dimmed);"
                >{{ region.students.toLocaleString() }} студентов</span>
                <span
                  class="text-[7px]"
                  style="color: var(--argus-border);"
                >·</span>
                <span
                  class="text-[8px]"
                  style="color: var(--argus-text-dimmed);"
                >{{ region.activeSessions }} активных</span>
                <span
                  class="text-[7px]"
                  style="color: var(--argus-border);"
                >·</span>
                <span
                  class="text-[8px]"
                  style="color: var(--argus-text-dimmed);"
                >{{ region.topViolation }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Institution Leaderboard -->
      <div
        class="rounded-2xl border p-5"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-building-2"
              class="size-4"
              style="color: var(--argus-warning);"
            />
            <h3
              class="text-sm font-bold"
              style="color: var(--argus-text);"
            >
              Рейтинг учреждений
            </h3>
          </div>
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >
            {{ store.institutionRankings.length }} учреждений
          </span>
        </div>

        <div class="space-y-2 max-h-96 overflow-y-auto pr-1">
          <div
            v-for="(inst, idx) in sortedInstitutions"
            :key="inst.id"
            class="flex items-center gap-3 p-3 rounded-xl border transition-all"
            :style="{
              background: 'var(--argus-bg-elevated)',
              borderColor: idx < 3 ? successBg(0.15) : 'transparent'
            }"
          >
            <!-- Medal / Rank -->
            <div
              class="flex items-center justify-center size-8 rounded-lg shrink-0"
              :style="{
                background: idx === 0 ? warningBg(0.12) : idx === 1 ? accentBg(0.1) : idx === 2 ? successBg(0.1) : 'var(--argus-bg-hover)'
              }"
            >
              <span
                v-if="idx < 3"
                class="text-sm"
              >{{ idx === 0 ? '🥇' : idx === 1 ? '🥈' : '🥉' }}</span>
              <span
                v-else
                class="text-[10px] font-black"
                style="color: var(--argus-text-dimmed);"
              >{{ idx + 1 }}</span>
            </div>

            <div class="flex-1 min-w-0">
              <div class="flex items-center justify-between">
                <div class="min-w-0">
                  <p
                    class="text-[11px] font-semibold truncate"
                    style="color: var(--argus-text);"
                  >
                    {{ inst.name }}
                  </p>
                  <p
                    class="text-[9px]"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ inst.city }} · {{ inst.totalStudents.toLocaleString() }} студентов
                  </p>
                </div>
                <div class="flex flex-col items-end shrink-0 ml-2">
                  <span
                    class="text-sm font-black tabular-nums"
                    :style="{ color: integrityColor(inst.avgIntegrity) }"
                  >{{ inst.avgIntegrity }}%</span>
                  <div class="flex items-center gap-0.5 mt-0.5">
                    <UIcon
                      :name="trendIcon(inst.trend)"
                      class="size-2.5"
                      :style="{ color: trendColor(inst.trend) }"
                    />
                    <span
                      class="text-[8px] font-bold px-1 py-0.5 rounded"
                      :style="{
                        background: inst.violationRate >= 5 ? errorBg(0.1) : inst.violationRate >= 3 ? warningBg(0.1) : successBg(0.1),
                        color: violationRateColor(inst.violationRate)
                      }"
                    >
                      {{ inst.violationRate }}% нар.
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ========================================== -->
    <!--  BOTTOM: SUMMARY STRIP                      -->
    <!-- ========================================== -->
    <div
      class="flex flex-wrap items-center justify-between gap-4 px-5 py-4 rounded-2xl border"
      :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
    >
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-1.5">
          <UIcon
            name="i-lucide-shield-check"
            class="size-3.5"
            style="color: var(--argus-success);"
          />
          <span
            class="text-[10px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >
            Защищено: <span
              class="font-bold"
              style="color: var(--argus-success);"
            >{{ store.examConfigs.filter(e => e.isArgusProtected).length }} экзаменов</span>
          </span>
        </div>
        <span
          class="text-[10px]"
          style="color: var(--argus-border);"
        >·</span>
        <div class="flex items-center gap-1.5">
          <UIcon
            name="i-lucide-activity"
            class="size-3.5"
            style="color: var(--argus-accent);"
          />
          <span
            class="text-[10px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >
            Активные сессии: <span
              class="font-bold"
              style="color: var(--argus-accent);"
            >{{ store.activeSessions }}</span>
          </span>
        </div>
        <span
          class="text-[10px]"
          style="color: var(--argus-border);"
        >·</span>
        <div class="flex items-center gap-1.5">
          <UIcon
            name="i-lucide-alert-triangle"
            class="size-3.5"
            style="color: var(--argus-error);"
          />
          <span
            class="text-[10px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >
            Критические: <span
              class="font-bold"
              style="color: var(--argus-error);"
            >{{ store.criticalViolations }}</span>
          </span>
        </div>
      </div>
      <div class="flex items-center gap-1.5">
        <div
          class="size-2 rounded-full animate-pulse"
          style="background: var(--argus-success);"
        />
        <span
          class="text-[10px] font-bold"
          style="color: var(--argus-success);"
        >СИСТЕМА АКТИВНА</span>
        <span
          class="text-[9px] ml-1"
          style="color: var(--argus-text-dimmed);"
        >Обновлено: {{ new Date().toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
