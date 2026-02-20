<script setup lang="ts">
import { useDashboardStore } from '~/stores/useDashboardStore'

const store = useDashboardStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()

// --- Filter State ---
const selectedExam = computed({
  get: () => store.violationSelectedExamId,
  set: (v) => { store.violationSelectedExamId = v }
})

const dateRange = computed({
  get: () => store.violationDateRange,
  set: (v) => { store.violationDateRange = v }
})

const dateRangeOptions = [
  { value: 'today', label: 'Сегодня' },
  { value: '7d', label: '7 дней' },
  { value: '30d', label: '30 дней' },
  { value: 'custom', label: 'Произвольный' }
] as const

const examOptions = computed(() =>
  store.activeExams.map(e => ({ label: e.examName, value: e.id }))
)

// Date range multipliers for simulated data scaling
const dateMultiplier = computed(() => {
  switch (dateRange.value) {
    case '7d': return 7
    case '30d': return 30
    default: return 1
  }
})

function severityBg(severity: string, opacity: number): string {
  switch (severity) {
    case 'critical': return errorBg(opacity)
    case 'warning': return warningBg(opacity)
    default: return accentBg(opacity)
  }
}

function severityColor(severity: string): string {
  switch (severity) {
    case 'critical': return 'var(--argus-error)'
    case 'warning': return 'var(--argus-warning)'
    default: return 'var(--argus-accent)'
  }
}

function accuracyGradient(score: number): string {
  if (score >= 95) {
    return isDark.value ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)'
  }
  if (score >= 90) {
    return isDark.value ? 'linear-gradient(90deg, #38BDF8, #0EA5E9)' : 'linear-gradient(90deg, #2563EB, #1D4ED8)'
  }
  if (score >= 85) {
    return isDark.value ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)'
  }
  return isDark.value ? 'linear-gradient(90deg, #F87171, #EF4444)' : 'linear-gradient(90deg, #E03E3E, #C92B2B)'
}

function reactionTimeColor(sec: number): string {
  if (sec <= 10) return 'var(--argus-success)'
  if (sec <= 14) return 'var(--argus-accent)'
  return 'var(--argus-warning)'
}

function warningAccuracyColor(pct: number): string {
  if (pct >= 95) return 'var(--argus-success)'
  if (pct >= 90) return 'var(--argus-accent)'
  return 'var(--argus-warning)'
}

function criticalRateColor(rate: number): string {
  if (rate >= 43) return 'var(--argus-error)'
  if (rate >= 37) return 'var(--argus-warning)'
  return 'var(--argus-text-muted)'
}

// --- Peak hour ---
const peakHour = computed(() => {
  let max = 0
  let peakTime = '—'
  store.hourlyViolations.forEach(h => {
    const total = h.phone + h.gaze + h.persons + h.tabs + h.audio
    if (total > max) {
      max = total
      peakTime = h.hour
    }
  })
  return { time: peakTime, count: max }
})

// --- Hourly totals ---
const hourlyTotals = computed(() =>
  store.hourlyViolations.map(h => ({
    hour: h.hour,
    total: h.phone + h.gaze + h.persons + h.tabs + h.audio,
    phone: h.phone,
    gaze: h.gaze,
    persons: h.persons,
    tabs: h.tabs,
    audio: h.audio
  }))
)

const maxHourlyTotal = computed(() =>
  Math.max(...hourlyTotals.value.map(h => h.total))
)

// --- Regional stats ---
const maxRegionalViolations = computed(() =>
  Math.max(...store.sortedRegionalViolations.map(r => r.totalViolations))
)

// --- Proctor summary ---
const topProctor = computed(() => {
  const sorted = [...store.proctorKPIs].sort((a, b) => b.warningAccuracy - a.warningAccuracy)
  return sorted[0]
})

const totalProctorSessions = computed(() =>
  store.proctorKPIs.reduce((s, p) => s + p.sessionsReviewed, 0)
)

// --- Export simulation ---
const exportLoading = ref(false)
function handleExportReport() {
  exportLoading.value = true
  setTimeout(() => { exportLoading.value = false }, 2000)
}

// --- Date range label ---
const dateRangeLabel = computed(() => {
  const opt = dateRangeOptions.find(o => o.value === dateRange.value)
  return opt ? opt.label : 'Сегодня'
})
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- ============================== -->
    <!--  HEADER + FILTERS              -->
    <!-- ============================== -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold" style="color: var(--argus-text);">
          Аналитика нарушений
        </h1>
        <p class="text-sm mt-1" style="color: var(--argus-text-dimmed);">
          Глубокий анализ трендов AI-детекции, категорий нарушений и эффективности прокторов
        </p>
      </div>

      <div class="flex items-center gap-3 flex-wrap">
        <!-- Exam Filter -->
        <USelectMenu
          v-model="selectedExam"
          :items="examOptions"
          value-key="value"
          placeholder="Все экзамены"
          class="w-56"
          :ui="{ base: 'cursor-pointer' }"
        />

        <!-- Date Range Picker -->
        <div class="flex items-center rounded-lg overflow-hidden border" style="border-color: var(--argus-border);">
          <button
            v-for="opt in dateRangeOptions"
            :key="opt.value"
            class="px-3 py-2 text-xs font-medium transition-all duration-200 cursor-pointer"
            :style="{
              background: dateRange === opt.value ? 'var(--argus-accent)' : 'transparent',
              color: dateRange === opt.value ? '#fff' : 'var(--argus-text-muted)',
              borderRight: opt.value !== 'custom' ? '1px solid var(--argus-border)' : 'none'
            }"
            @click="dateRange = opt.value"
          >
            {{ opt.label }}
          </button>
        </div>

        <!-- Export Button -->
        <button
          class="flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-semibold transition-all duration-200 cursor-pointer"
          :style="{
            background: exportLoading ? accentBg(0.2) : accentBg(0.1),
            color: 'var(--argus-accent)',
            border: `1px solid ${accentBg(0.2)}`
          }"
          @click="handleExportReport"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.2)"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.1)"
        >
          <UIcon :name="exportLoading ? 'i-lucide-loader-2' : 'i-lucide-download'" class="size-4" :class="{ 'animate-spin': exportLoading }" />
          {{ exportLoading ? 'Формирование...' : 'Скачать отчёт' }}
        </button>
      </div>
    </div>

    <!-- ============================== -->
    <!--  SUMMARY KPIs                  -->
    <!-- ============================== -->
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-5 gap-4">
      <div class="glass-card rounded-xl p-5">
        <p class="text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
          Всего нарушений
        </p>
        <p class="text-3xl font-bold mt-2 tabular-nums" style="color: var(--argus-text);">
          {{ (store.totalViolationsToday * dateMultiplier).toLocaleString() }}
        </p>
        <p class="text-xs mt-1.5" style="color: var(--argus-text-dimmed);">
          {{ dateRangeLabel }} · {{ store.violationCategories.length }} категорий
        </p>
      </div>

      <div class="glass-card glow-error rounded-xl p-5">
        <p class="text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
          Критические
        </p>
        <p class="text-3xl font-bold mt-2 tabular-nums" style="color: var(--argus-error);">
          {{ (store.violationCategories.filter(c => c.severity === 'critical').reduce((s, c) => s + c.count, 0) * dateMultiplier).toLocaleString() }}
        </p>
        <p class="text-xs mt-1.5" style="color: var(--argus-text-dimmed);">
          Телефоны + Посторонние
        </p>
      </div>

      <div class="glass-card rounded-xl p-5">
        <p class="text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
          Пиковый час
        </p>
        <p class="text-3xl font-bold mt-2" style="color: var(--argus-text);">
          {{ peakHour.time }}
        </p>
        <p class="text-xs mt-1.5" style="color: var(--argus-text-dimmed);">
          {{ peakHour.count }} нарушений в пик
        </p>
      </div>

      <div class="glass-card rounded-xl p-5">
        <p class="text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
          Средн. реакция прокторов
        </p>
        <p class="text-3xl font-bold mt-2 tabular-nums" style="color: var(--argus-accent);">
          {{ store.proctorAvgReactionTime }}с
        </p>
        <p class="text-xs mt-1.5" style="color: var(--argus-text-dimmed);">
          {{ store.proctorKPIs.length }} прокторов на смене
        </p>
      </div>

      <div class="glass-card rounded-xl p-5">
        <p class="text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
          Средняя точность AI
        </p>
        <p class="text-3xl font-bold mt-2 tabular-nums" style="color: var(--argus-success);">
          {{ (store.detectionAccuracy.reduce((s, d) => s + d.accuracy, 0) / store.detectionAccuracy.length).toFixed(1) }}%
        </p>
        <p class="text-xs mt-1.5" style="color: var(--argus-text-dimmed);">
          По всем типам детекции
        </p>
      </div>
    </div>

    <!-- ============================== -->
    <!--  CATEGORIES + HOURLY HEATMAP   -->
    <!-- ============================== -->
    <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
      <!-- Category Cards -->
      <div class="glass-card rounded-xl overflow-hidden">
        <div class="flex items-center justify-between px-5 py-4 border-b" style="border-color: var(--argus-border);">
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-layers" class="size-4" style="color: var(--argus-text-dimmed);" />
            <h2 class="text-sm font-semibold" style="color: var(--argus-text);">
              Категории нарушений
            </h2>
          </div>
          <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }">
            {{ dateRangeLabel }}
          </span>
        </div>

        <div class="divide-y" style="border-color: var(--argus-border-subtle);">
          <div
            v-for="cat in store.violationCategories"
            :key="cat.type"
            class="flex items-center gap-4 px-5 py-4 transition-colors"
            style="border-bottom: 1px solid var(--argus-border-subtle);"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          >
            <div
              class="flex items-center justify-center size-10 rounded-lg shrink-0"
              :style="{ background: severityBg(cat.severity, 0.1), border: `1px solid ${severityBg(cat.severity, 0.15)}` }"
            >
              <UIcon :name="cat.icon" class="size-5" :style="{ color: severityColor(cat.severity) }" />
            </div>

            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-sm font-medium" style="color: var(--argus-text);">{{ cat.type }}</span>
                <span
                  class="text-[9px] font-bold px-1.5 py-0.5 rounded-full uppercase"
                  :style="{ background: severityBg(cat.severity, 0.1), color: severityColor(cat.severity) }"
                >
                  {{ cat.severity === 'critical' ? 'крит.' : cat.severity === 'warning' ? 'осторожно' : 'инфо' }}
                </span>
              </div>
              <div class="mt-2 flex items-center gap-2">
                <div class="flex-1 h-1 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
                  <div
                    class="h-full rounded-full transition-all duration-700"
                    :style="{
                      width: `${(cat.count / store.totalViolationsToday) * 100}%`,
                      background: `${severityColor(cat.severity)}`
                    }"
                  />
                </div>
                <span class="text-[10px] font-mono tabular-nums" style="color: var(--argus-text-dimmed);">
                  {{ ((cat.count / store.totalViolationsToday) * 100).toFixed(1) }}%
                </span>
              </div>
            </div>

            <div class="text-right shrink-0">
              <p class="text-lg font-bold tabular-nums" style="color: var(--argus-text);">{{ (cat.count * dateMultiplier).toLocaleString() }}</p>
              <div class="flex items-center gap-1 justify-end mt-0.5">
                <UIcon
                  :name="cat.trend === 'up' ? 'i-lucide-trending-up' : 'i-lucide-trending-down'"
                  class="size-3"
                  :style="{ color: cat.trend === 'up' ? 'var(--argus-error)' : 'var(--argus-success)' }"
                />
                <span
                  class="text-[11px] font-medium"
                  :style="{ color: cat.trend === 'up' ? 'var(--argus-error)' : 'var(--argus-success)' }"
                >
                  {{ cat.percentChange > 0 ? '+' : '' }}{{ cat.percentChange }}%
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Hourly Heatmap -->
      <div class="glass-card rounded-xl overflow-hidden">
        <div class="flex items-center justify-between px-5 py-4 border-b" style="border-color: var(--argus-border);">
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-clock" class="size-4" style="color: var(--argus-text-dimmed);" />
            <h2 class="text-sm font-semibold" style="color: var(--argus-text);">
              Почасовая активность
            </h2>
          </div>
          <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }">
            {{ dateRangeLabel }}
          </span>
        </div>

        <div class="p-5 space-y-2">
          <div
            v-for="hour in hourlyTotals"
            :key="hour.hour"
            class="flex items-center gap-3"
          >
            <span class="text-xs font-mono tabular-nums w-12 text-right" style="color: var(--argus-text-dimmed);">
              {{ hour.hour }}
            </span>

            <div class="flex-1 flex h-5 rounded overflow-hidden gap-px" style="background: var(--argus-bg-hover);">
              <div
                v-if="hour.phone > 0"
                class="h-full transition-all duration-500"
                :style="{
                  width: `${(hour.phone / maxHourlyTotal) * 100}%`,
                  background: isDark ? 'rgba(66, 133, 244, 0.8)' : 'rgba(26, 115, 232, 0.75)'
                }"
                :title="`Телефон: ${hour.phone}`"
              />
              <div
                v-if="hour.gaze > 0"
                class="h-full transition-all duration-500"
                :style="{
                  width: `${(hour.gaze / maxHourlyTotal) * 100}%`,
                  background: isDark ? 'rgba(162, 89, 255, 0.7)' : 'rgba(123, 31, 162, 0.65)'
                }"
                :title="`Взгляд: ${hour.gaze}`"
              />
              <div
                v-if="hour.persons > 0"
                class="h-full transition-all duration-500"
                :style="{
                  width: `${(hour.persons / maxHourlyTotal) * 100}%`,
                  background: isDark ? 'rgba(234, 67, 53, 0.7)' : 'rgba(217, 48, 37, 0.65)'
                }"
                :title="`Посторонние: ${hour.persons}`"
              />
              <div
                v-if="hour.tabs > 0"
                class="h-full transition-all duration-500"
                :style="{
                  width: `${(hour.tabs / maxHourlyTotal) * 100}%`,
                  background: isDark ? 'rgba(52, 168, 83, 0.7)' : 'rgba(24, 128, 56, 0.65)'
                }"
                :title="`Вкладки: ${hour.tabs}`"
              />
              <div
                v-if="hour.audio > 0"
                class="h-full transition-all duration-500"
                :style="{
                  width: `${(hour.audio / maxHourlyTotal) * 100}%`,
                  background: isDark ? 'rgba(251, 188, 5, 0.65)' : 'rgba(227, 116, 0, 0.6)'
                }"
                :title="`Аудио: ${hour.audio}`"
              />
            </div>

            <span class="text-xs font-mono tabular-nums w-8 text-right" style="color: var(--argus-text-dimmed);">
              {{ hour.total }}
            </span>
          </div>
        </div>

        <!-- Legend -->
        <div class="px-5 py-3 border-t flex flex-wrap gap-4" style="border-color: var(--argus-border);">
          <div class="flex items-center gap-1.5">
            <div class="size-2 rounded-sm" :style="{ background: isDark ? 'rgba(66, 133, 244, 0.8)' : 'rgba(26, 115, 232, 0.75)' }" />
            <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Телефон</span>
          </div>
          <div class="flex items-center gap-1.5">
            <div class="size-2 rounded-sm" :style="{ background: isDark ? 'rgba(162, 89, 255, 0.7)' : 'rgba(123, 31, 162, 0.65)' }" />
            <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Взгляд</span>
          </div>
          <div class="flex items-center gap-1.5">
            <div class="size-2 rounded-sm" :style="{ background: isDark ? 'rgba(234, 67, 53, 0.7)' : 'rgba(217, 48, 37, 0.65)' }" />
            <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Посторонние</span>
          </div>
          <div class="flex items-center gap-1.5">
            <div class="size-2 rounded-sm" :style="{ background: isDark ? 'rgba(52, 168, 83, 0.7)' : 'rgba(24, 128, 56, 0.65)' }" />
            <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Вкладки</span>
          </div>
          <div class="flex items-center gap-1.5">
            <div class="size-2 rounded-sm" :style="{ background: isDark ? 'rgba(251, 188, 5, 0.65)' : 'rgba(227, 116, 0, 0.6)' }" />
            <span class="text-[10px]" style="color: var(--argus-text-dimmed);">Аудио</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  REGIONAL BREAKDOWN            -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div class="flex items-center justify-between px-5 py-4 border-b" style="border-color: var(--argus-border);">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-map-pin" class="size-4" style="color: var(--argus-accent);" />
          <h2 class="text-sm font-semibold" style="color: var(--argus-text);">
            Региональная разбивка нарушений
          </h2>
        </div>
        <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }">
          {{ store.sortedRegionalViolations.length }} регионов
        </span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Регион</th>
              <th class="text-left px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Всего</th>
              <th class="text-left px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Распределение по типам</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Крит. %</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Реакция</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="region in store.sortedRegionalViolations"
              :key="region.regionId"
              class="transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <!-- Region name -->
              <td class="px-5 py-3.5">
                <div class="flex items-center gap-2">
                  <div class="size-2 rounded-full shrink-0" :style="{ background: region.criticalRate >= 43 ? 'var(--argus-error)' : region.criticalRate >= 37 ? 'var(--argus-warning)' : 'var(--argus-success)' }" />
                  <span class="font-medium" style="color: var(--argus-text);">{{ region.regionName }}</span>
                </div>
              </td>

              <!-- Total with bar -->
              <td class="px-4 py-3.5">
                <div class="flex items-center gap-2 min-w-24">
                  <div class="flex-1 h-1.5 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
                    <div
                      class="h-full rounded-full transition-all duration-700"
                      :style="{ width: `${(region.totalViolations / maxRegionalViolations) * 100}%`, background: 'var(--argus-accent)' }"
                    />
                  </div>
                  <span class="text-xs font-mono font-bold tabular-nums" style="color: var(--argus-text);">{{ (region.totalViolations * dateMultiplier).toLocaleString() }}</span>
                </div>
              </td>

              <!-- Stacked type breakdown -->
              <td class="px-4 py-3.5">
                <div class="flex h-4 rounded-sm overflow-hidden gap-px min-w-48" style="background: var(--argus-bg-hover);">
                  <div
                    class="h-full"
                    :style="{ width: `${(region.phone / region.totalViolations) * 100}%`, background: isDark ? 'rgba(66, 133, 244, 0.8)' : 'rgba(26, 115, 232, 0.75)' }"
                    :title="`Телефон: ${region.phone}`"
                  />
                  <div
                    class="h-full"
                    :style="{ width: `${(region.gaze / region.totalViolations) * 100}%`, background: isDark ? 'rgba(162, 89, 255, 0.7)' : 'rgba(123, 31, 162, 0.65)' }"
                    :title="`Взгляд: ${region.gaze}`"
                  />
                  <div
                    class="h-full"
                    :style="{ width: `${(region.persons / region.totalViolations) * 100}%`, background: isDark ? 'rgba(234, 67, 53, 0.7)' : 'rgba(217, 48, 37, 0.65)' }"
                    :title="`Посторонние: ${region.persons}`"
                  />
                  <div
                    class="h-full"
                    :style="{ width: `${(region.tabs / region.totalViolations) * 100}%`, background: isDark ? 'rgba(52, 168, 83, 0.7)' : 'rgba(24, 128, 56, 0.65)' }"
                    :title="`Вкладки: ${region.tabs}`"
                  />
                  <div
                    class="h-full"
                    :style="{ width: `${(region.audio / region.totalViolations) * 100}%`, background: isDark ? 'rgba(251, 188, 5, 0.65)' : 'rgba(227, 116, 0, 0.6)' }"
                    :title="`Аудио: ${region.audio}`"
                  />
                </div>
              </td>

              <!-- Critical rate -->
              <td class="px-4 py-3.5 text-center">
                <span
                  class="text-xs font-bold font-mono tabular-nums px-2 py-0.5 rounded"
                  :style="{ color: criticalRateColor(region.criticalRate), background: region.criticalRate >= 43 ? errorBg(0.1) : region.criticalRate >= 37 ? warningBg(0.1) : 'var(--argus-bg-hover)' }"
                >
                  {{ region.criticalRate }}%
                </span>
              </td>

              <!-- Avg reaction -->
              <td class="px-4 py-3.5 text-center">
                <span class="text-xs font-mono font-bold tabular-nums" :style="{ color: reactionTimeColor(region.avgReactionSec) }">
                  {{ region.avgReactionSec }}с
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  PROCTOR EFFICIENCY KPIs       -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div class="flex items-center justify-between px-5 py-4 border-b" style="border-color: var(--argus-border);">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-shield-check" class="size-4" style="color: var(--argus-accent);" />
          <h2 class="text-sm font-semibold" style="color: var(--argus-text);">
            Эффективность прокторов
          </h2>
        </div>
        <div class="flex items-center gap-3">
          <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }">
            Реакция: {{ store.proctorAvgReactionTime }}с
          </span>
          <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: successBg(0.1), color: 'var(--argus-success)' }">
            Точность: {{ store.proctorAvgWarningAccuracy }}%
          </span>
          <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }">
            {{ totalProctorSessions }} сессий
          </span>
        </div>
      </div>

      <!-- Summary KPI strip -->
      <div class="grid grid-cols-4 gap-px" style="background: var(--argus-border-subtle);">
        <div class="p-4 text-center" style="background: var(--argus-card-bg);">
          <p class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--argus-text-dimmed);">Лучший проктор</p>
          <p class="text-sm font-bold mt-1" style="color: var(--argus-text);">{{ topProctor?.name }}</p>
          <p class="text-[10px] mt-0.5" style="color: var(--argus-success);">{{ topProctor?.warningAccuracy }}% точность</p>
        </div>
        <div class="p-4 text-center" style="background: var(--argus-card-bg);">
          <p class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--argus-text-dimmed);">Сред. реакция</p>
          <p class="text-2xl font-bold mt-1 tabular-nums" :style="{ color: reactionTimeColor(parseFloat(store.proctorAvgReactionTime)) }">
            {{ store.proctorAvgReactionTime }}с
          </p>
        </div>
        <div class="p-4 text-center" style="background: var(--argus-card-bg);">
          <p class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--argus-text-dimmed);">Сред. точность предупр.</p>
          <p class="text-2xl font-bold mt-1 tabular-nums" :style="{ color: warningAccuracyColor(parseFloat(store.proctorAvgWarningAccuracy)) }">
            {{ store.proctorAvgWarningAccuracy }}%
          </p>
        </div>
        <div class="p-4 text-center" style="background: var(--argus-card-bg);">
          <p class="text-[10px] uppercase tracking-wider font-medium" style="color: var(--argus-text-dimmed);">Всего прерываний</p>
          <p class="text-2xl font-bold mt-1 tabular-nums" style="color: var(--argus-error);">
            {{ store.proctorKPIs.reduce((s, p) => s + p.terminationsInitiated, 0) }}
          </p>
        </div>
      </div>

      <!-- Proctor table -->
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Проктор</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Смена</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Сессий</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Реакция</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Предупр.</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Точность</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Прерыв.</th>
              <th class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Нарушений</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="proctor in store.proctorKPIs"
              :key="proctor.id"
              class="transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <td class="px-5 py-3.5">
                <div class="flex items-center gap-2.5">
                  <div
                    class="size-7 rounded-full flex items-center justify-center text-xs font-bold shrink-0"
                    :style="{ background: proctor.warningAccuracy >= 95 ? successBg(0.15) : accentBg(0.15), color: proctor.warningAccuracy >= 95 ? 'var(--argus-success)' : 'var(--argus-accent)' }"
                  >
                    {{ proctor.name.charAt(0) }}
                  </div>
                  <span class="font-medium" style="color: var(--argus-text);">{{ proctor.name }}</span>
                </div>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span class="text-xs font-mono" style="color: var(--argus-text-dimmed);">{{ proctor.shift }}</span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span class="text-sm font-bold tabular-nums" style="color: var(--argus-text);">{{ proctor.sessionsReviewed }}</span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span class="text-sm font-mono font-bold tabular-nums" :style="{ color: reactionTimeColor(proctor.avgReactionTimeSec) }">
                  {{ proctor.avgReactionTimeSec }}с
                </span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span class="text-sm tabular-nums" style="color: var(--argus-text);">{{ proctor.warningsIssued }}</span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <div class="flex items-center gap-2 justify-center">
                  <div class="w-16 h-1.5 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
                    <div
                      class="h-full rounded-full transition-all duration-700"
                      :style="{ width: `${proctor.warningAccuracy}%`, background: accuracyGradient(proctor.warningAccuracy) }"
                    />
                  </div>
                  <span class="text-xs font-bold font-mono tabular-nums" :style="{ color: warningAccuracyColor(proctor.warningAccuracy) }">
                    {{ proctor.warningAccuracy }}%
                  </span>
                </div>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span
                  class="text-xs font-bold tabular-nums px-2 py-0.5 rounded"
                  :style="{ background: proctor.terminationsInitiated >= 5 ? errorBg(0.1) : 'var(--argus-bg-hover)', color: proctor.terminationsInitiated >= 5 ? 'var(--argus-error)' : 'var(--argus-text-muted)' }"
                >
                  {{ proctor.terminationsInitiated }}
                </span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span class="text-sm font-bold tabular-nums" style="color: var(--argus-text);">{{ proctor.violationsDetected }}</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  AI DETECTION ACCURACY         -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div class="flex items-center justify-between px-5 py-4 border-b" style="border-color: var(--argus-border);">
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-brain" class="size-4" style="color: var(--argus-accent);" />
          <h2 class="text-sm font-semibold" style="color: var(--argus-text);">
            Точность AI-детекции
          </h2>
        </div>
        <span class="text-[10px] font-medium px-2 py-1 rounded-full" :style="{ background: successBg(0.1), color: 'var(--argus-success)' }">
          Среднее {{ (store.detectionAccuracy.reduce((s, d) => s + d.accuracy, 0) / store.detectionAccuracy.length).toFixed(1) }}%
        </span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Тип детекции</th>
              <th class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">True Positive</th>
              <th class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">False Positive</th>
              <th class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Общая точность</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="det in store.detectionAccuracy"
              :key="det.type"
              class="transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <td class="px-5 py-3.5 font-medium" style="color: var(--argus-text);">{{ det.type }}</td>
              <td class="px-5 py-3.5">
                <span class="text-sm font-mono tabular-nums" style="color: var(--argus-success);">{{ det.truePositive }}%</span>
              </td>
              <td class="px-5 py-3.5">
                <span
                  class="text-sm font-mono tabular-nums"
                  :style="{ color: det.falsePositive > 5 ? 'var(--argus-warning)' : 'var(--argus-text-muted)' }"
                >
                  {{ det.falsePositive }}%
                </span>
              </td>
              <td class="px-5 py-3.5">
                <div class="flex items-center gap-3 min-w-36">
                  <div class="flex-1 h-2 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
                    <div
                      class="h-full rounded-full transition-all duration-700"
                      :style="{ width: `${det.accuracy}%`, background: accuracyGradient(det.accuracy) }"
                    />
                  </div>
                  <span class="text-sm font-bold font-mono tabular-nums" style="color: var(--argus-text);">{{ det.accuracy }}%</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
