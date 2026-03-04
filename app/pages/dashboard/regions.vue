<script setup lang="ts">
import { useDashboardStore } from '~/stores/useDashboardStore'

const store = useDashboardStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()

function violationColor(rate: number): string {
  if (rate >= 4.5) return 'var(--argus-error)'
  if (rate >= 3.5) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

function violationBg(rate: number, opacity: number): string {
  if (rate >= 4.5) return errorBg(opacity)
  if (rate >= 3.5) return warningBg(opacity)
  return successBg(opacity)
}

function integrityGradient(score: number): string {
  if (score >= 85) {
    return isDark.value ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)'
  }
  if (score >= 80) {
    return isDark.value ? 'linear-gradient(90deg, #38BDF8, #0EA5E9)' : 'linear-gradient(90deg, #2563EB, #1D4ED8)'
  }
  return isDark.value ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)'
}

// Sort regions by students (descending) for the bar visualization
const sortedRegions = computed(() =>
  [...store.regions].sort((a, b) => b.students - a.students)
)

const maxStudents = computed(() =>
  Math.max(...store.regions.map(r => r.students))
)

// Top violations across all regions
const topViolationCounts = computed(() => {
  const counts: Record<string, number> = {}
  store.regions.forEach((r) => {
    counts[r.topViolation] = (counts[r.topViolation] ?? 0) + 1
  })
  return Object.entries(counts)
    .sort((a, b) => b[1] - a[1])
    .map(([type, count]) => ({ type, count }))
})
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- Page header -->
    <div>
      <h1
        class="text-2xl font-bold"
        style="color: var(--argus-text);"
      >
        Regional Analysis
      </h1>
      <p
        class="text-sm mt-1"
        style="color: var(--argus-text-dimmed);"
      >
        Распределение студентов по регионам Казахстана и анализ нарушений
      </p>
    </div>

    <!-- Summary KPI cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
      <div class="glass-card rounded-xl p-5">
        <p
          class="text-[11px] font-medium uppercase tracking-wider"
          style="color: var(--argus-text-dimmed);"
        >
          Всего студентов
        </p>
        <p
          class="text-3xl font-bold mt-2"
          style="color: var(--argus-text);"
        >
          {{ store.totalStudentsAllRegions.toLocaleString() }}
        </p>
        <p
          class="text-xs mt-1.5"
          style="color: var(--argus-text-dimmed);"
        >
          {{ store.regions.length }} регионов
        </p>
      </div>

      <div class="glass-card rounded-xl p-5">
        <p
          class="text-[11px] font-medium uppercase tracking-wider"
          style="color: var(--argus-text-dimmed);"
        >
          Средний уровень нарушений
        </p>
        <p
          class="text-3xl font-bold mt-2"
          :style="{ color: Number(store.avgViolationRateAllRegions) >= 3.5 ? 'var(--argus-warning)' : 'var(--argus-success)' }"
        >
          {{ store.avgViolationRateAllRegions }}%
        </p>
        <p
          class="text-xs mt-1.5"
          style="color: var(--argus-text-dimmed);"
        >
          Среднее по всем регионам
        </p>
      </div>

      <div class="glass-card rounded-xl p-5">
        <p
          class="text-[11px] font-medium uppercase tracking-wider"
          style="color: var(--argus-text-dimmed);"
        >
          Активные сессии
        </p>
        <p
          class="text-3xl font-bold mt-2"
          style="color: var(--argus-text);"
        >
          {{ store.regions.reduce((s, r) => s + r.activeSessions, 0) }}
        </p>
        <p
          class="text-xs mt-1.5"
          style="color: var(--argus-text-dimmed);"
        >
          По всем регионам
        </p>
      </div>

      <div class="glass-card rounded-xl p-5">
        <p
          class="text-[11px] font-medium uppercase tracking-wider"
          style="color: var(--argus-text-dimmed);"
        >
          Топ нарушение
        </p>
        <p
          class="text-lg font-bold mt-2"
          style="color: var(--argus-error);"
        >
          {{ topViolationCounts[0]?.type ?? '—' }}
        </p>
        <p
          class="text-xs mt-1.5"
          style="color: var(--argus-text-dimmed);"
        >
          В {{ topViolationCounts[0]?.count ?? 0 }} регионах
        </p>
      </div>
    </div>

    <!-- Main Content -->
    <div class="grid grid-cols-1 xl:grid-cols-3 gap-6">
      <!-- Student Distribution (bar chart representation) -->
      <div class="xl:col-span-2 glass-card rounded-xl overflow-hidden">
        <div
          class="flex items-center justify-between px-5 py-4 border-b"
          style="border-color: var(--argus-border);"
        >
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-bar-chart-3"
              class="size-4"
              style="color: var(--argus-text-dimmed);"
            />
            <h2
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Распределение студентов
            </h2>
          </div>
          <span
            class="text-[10px] font-medium px-2 py-1 rounded-full"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ store.totalStudentsAllRegions.toLocaleString() }} всего
          </span>
        </div>

        <div class="p-5 space-y-3">
          <div
            v-for="region in sortedRegions"
            :key="region.id"
            class="flex items-center gap-3"
          >
            <span
              class="text-xs font-medium w-32 truncate text-right"
              style="color: var(--argus-text-muted);"
            >
              {{ region.name }}
            </span>
            <div
              class="flex-1 h-5 rounded overflow-hidden relative"
              style="background: var(--argus-bg-hover);"
            >
              <div
                class="h-full rounded transition-all duration-700 flex items-center"
                :style="{
                  width: `${(region.students / maxStudents) * 100}%`,
                  background: integrityGradient(region.avgIntegrity),
                  minWidth: '40px'
                }"
              >
                <span class="text-[10px] font-bold text-white pl-2 whitespace-nowrap">
                  {{ region.students.toLocaleString() }}
                </span>
              </div>
            </div>
            <span
              class="text-[10px] font-medium px-2 py-0.5 rounded-full w-14 text-center"
              :style="{ background: violationBg(region.violationRate, 0.1), color: violationColor(region.violationRate) }"
            >
              {{ region.violationRate }}%
            </span>
          </div>
        </div>
      </div>

      <!-- Top Violations by Region -->
      <div class="glass-card rounded-xl overflow-hidden">
        <div
          class="flex items-center justify-between px-5 py-4 border-b"
          style="border-color: var(--argus-border);"
        >
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-shield-alert"
              class="size-4"
              style="color: var(--argus-error);"
            />
            <h2
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Типы нарушений
            </h2>
          </div>
        </div>

        <div class="p-5 space-y-4">
          <div
            v-for="item in topViolationCounts"
            :key="item.type"
            class="flex items-center justify-between"
          >
            <span
              class="text-sm"
              style="color: var(--argus-text);"
            >{{ item.type }}</span>
            <div class="flex items-center gap-2">
              <div class="flex">
                <div
                  v-for="i in item.count"
                  :key="i"
                  class="size-2 rounded-full -ml-0.5 first:ml-0"
                  :style="{ background: 'var(--argus-accent)' }"
                />
              </div>
              <span
                class="text-xs font-medium"
                style="color: var(--argus-text-dimmed);"
              >
                {{ item.count }} рег.
              </span>
            </div>
          </div>
        </div>

        <!-- Integrity score legend -->
        <div
          class="px-5 py-4 border-t"
          style="border-color: var(--argus-border);"
        >
          <p
            class="text-[10px] font-medium uppercase tracking-wider mb-3"
            style="color: var(--argus-text-dimmed);"
          >
            Шкала честности
          </p>
          <div class="space-y-2">
            <div class="flex items-center gap-2">
              <div
                class="w-8 h-1.5 rounded-full"
                :style="{ background: isDark ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)' }"
              />
              <span
                class="text-[11px]"
                style="color: var(--argus-text-dimmed);"
              >85%+ — Отлично</span>
            </div>
            <div class="flex items-center gap-2">
              <div
                class="w-8 h-1.5 rounded-full"
                :style="{ background: isDark ? 'linear-gradient(90deg, #38BDF8, #0EA5E9)' : 'linear-gradient(90deg, #2563EB, #1D4ED8)' }"
              />
              <span
                class="text-[11px]"
                style="color: var(--argus-text-dimmed);"
              >80–84% — Хорошо</span>
            </div>
            <div class="flex items-center gap-2">
              <div
                class="w-8 h-1.5 rounded-full"
                :style="{ background: isDark ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)' }"
              />
              <span
                class="text-[11px]"
                style="color: var(--argus-text-dimmed);"
              >Ниже 80% — Внимание</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Detailed Regions Table -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="flex items-center justify-between px-5 py-4 border-b"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-map"
            class="size-4"
            style="color: var(--argus-text-dimmed);"
          />
          <h2
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Данные по регионам
          </h2>
        </div>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th
                class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Регион
              </th>
              <th
                class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Область
              </th>
              <th
                class="text-right px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Студенты
              </th>
              <th
                class="text-right px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Сессии
              </th>
              <th
                class="text-right px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Нарушения
              </th>
              <th
                class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Честность
              </th>
              <th
                class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Топ нарушение
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="region in sortedRegions"
              :key="region.id"
              class="transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <td
                class="px-5 py-3 font-medium"
                style="color: var(--argus-text);"
              >
                {{ region.name }}
              </td>
              <td
                class="px-5 py-3 text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                {{ region.nameKz }}
              </td>
              <td
                class="px-5 py-3 text-right font-mono tabular-nums"
                style="color: var(--argus-text-muted);"
              >
                {{ region.students.toLocaleString() }}
              </td>
              <td
                class="px-5 py-3 text-right font-mono tabular-nums"
                style="color: var(--argus-text-muted);"
              >
                {{ region.activeSessions }}
              </td>
              <td class="px-5 py-3 text-right">
                <span
                  class="text-xs font-medium px-2 py-1 rounded-full"
                  :style="{ background: violationBg(region.violationRate, 0.1), color: violationColor(region.violationRate) }"
                >
                  {{ region.violationRate }}%
                </span>
              </td>
              <td class="px-5 py-3">
                <div class="flex items-center gap-2 min-w-28">
                  <div
                    class="flex-1 h-1.5 rounded-full overflow-hidden"
                    style="background: var(--argus-bg-hover);"
                  >
                    <div
                      class="h-full rounded-full transition-all duration-500"
                      :style="{ width: `${region.avgIntegrity}%`, background: integrityGradient(region.avgIntegrity) }"
                    />
                  </div>
                  <span
                    class="text-xs font-mono tabular-nums"
                    style="color: var(--argus-text-dimmed);"
                  >{{ region.avgIntegrity }}%</span>
                </div>
              </td>
              <td
                class="px-5 py-3 text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                {{ region.topViolation }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
