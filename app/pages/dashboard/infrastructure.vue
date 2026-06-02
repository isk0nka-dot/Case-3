<script setup lang="ts">
import { useAdminAPI, type InfraContainerStats, type InfraNetworkMetric, type InfraLatencyPoint } from '~/composables/useAdminAPI'
import { useAuthStore } from '~/stores/useAuthStore'

const adminAPI = useAdminAPI()
const authStore = useAuthStore()
const { demoMode } = useDemoMode()
const { isDark, accentBg, warningBg } = useColors()

// ---------------------------------------------------------------------------
// Reactive state — populated from the real API
// ---------------------------------------------------------------------------
const serverNodes = ref<InfraContainerStats[]>([])
const networkMetrics = ref<InfraNetworkMetric[]>([])
const latencyHistory = ref<InfraLatencyPoint[]>([])
const totalCapacity = ref(10000)
const currentLoad = ref(0)
const loadPercent = ref(0)
const avgLatencyMs = ref(0)
const p99LatencyMs = ref(0)
const loading = ref(true)
const error = ref('')
const lastUpdated = ref('')
const usingDemoData = ref(false)

// ---------------------------------------------------------------------------
// Data fetching with 10s polling
// ---------------------------------------------------------------------------
let pollTimer: ReturnType<typeof setInterval> | null = null

const DEMO_INFRA_NODES: InfraContainerStats[] = [
  { id: 'node-1', name: 'argus-event-collector', region: 'Almaty-1', cpuLoad: 34, memoryUsage: 58, diskUsage: 42, status: 'healthy', latencyMs: 12, uptime: 99.97, connections: 284, memUsedMB: 940, memLimitMB: 2048 },
  { id: 'node-2', name: 'argus-kafka', region: 'Almaty-1', cpuLoad: 22, memoryUsage: 44, diskUsage: 61, status: 'healthy', latencyMs: 8, uptime: 99.99, connections: 147, memUsedMB: 716, memLimitMB: 2048 },
  { id: 'node-3', name: 'argus-clickhouse', region: 'Almaty-2', cpuLoad: 67, memoryUsage: 71, diskUsage: 78, status: 'warning', latencyMs: 28, uptime: 99.91, connections: 92, memUsedMB: 1454, memLimitMB: 4096 },
  { id: 'node-4', name: 'argus-postgres', region: 'Almaty-1', cpuLoad: 18, memoryUsage: 36, diskUsage: 29, status: 'healthy', latencyMs: 5, uptime: 100, connections: 48, memUsedMB: 368, memLimitMB: 1024 },
  { id: 'node-5', name: 'argus-livekit', region: 'Almaty-2', cpuLoad: 81, memoryUsage: 62, diskUsage: 38, status: 'warning', latencyMs: 45, uptime: 99.84, connections: 1240, memUsedMB: 2534, memLimitMB: 8192 },
  { id: 'node-6', name: 'argus-minio', region: 'Almaty-3', cpuLoad: 12, memoryUsage: 28, diskUsage: 84, status: 'healthy', latencyMs: 18, uptime: 99.98, connections: 36, memUsedMB: 574, memLimitMB: 2048 }
]

async function fetchInfraStats() {
  if (!authStore.isAuthenticated) {
    error.value = 'Unauthorized'
    loading.value = false
    return
  }

  try {
    const stats = await adminAPI.getInfrastructureStats()
    serverNodes.value = stats.serverNodes || []
    networkMetrics.value = stats.networkMetrics || []
    latencyHistory.value = stats.latencyHistory || []
    totalCapacity.value = stats.totalCapacity || 10000
    currentLoad.value = stats.currentLoad || 0
    loadPercent.value = stats.loadPercent || 0
    avgLatencyMs.value = stats.avgLatencyMs || 0
    p99LatencyMs.value = stats.p99LatencyMs || 0
    lastUpdated.value = new Date().toLocaleTimeString('ru-RU')
    error.value = ''
    usingDemoData.value = false
  } catch (e: unknown) {
    const msg = e instanceof Error ? e.message : String(e)
    if (demoMode.value && serverNodes.value.length === 0) {
      console.info('[Infrastructure] Backend unavailable, showing demo data.')
      serverNodes.value = DEMO_INFRA_NODES
      networkMetrics.value = [
        { label: 'Входящий трафик', value: 2840, unit: 'Mbps', status: 'healthy', trend: 'up' },
        { label: 'Исходящий трафик', value: 1620, unit: 'Mbps', status: 'healthy', trend: 'stable' },
        { label: 'Потеря пакетов', value: 0.02, unit: '%', status: 'healthy', trend: 'down' },
        { label: 'Задержка DNS', value: 4.2, unit: 'ms', status: 'healthy', trend: 'stable' }
      ]
      totalCapacity.value = 10000
      currentLoad.value = 3847
      loadPercent.value = 38
      avgLatencyMs.value = 19
      p99LatencyMs.value = 48
      usingDemoData.value = true
      error.value = ''
    } else if (serverNodes.value.length === 0) {
      error.value = msg
    }
    lastUpdated.value = new Date().toLocaleTimeString('ru-RU')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchInfraStats()
  pollTimer = setInterval(fetchInfraStats, 10_000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})

// ---------------------------------------------------------------------------
// Computed
// ---------------------------------------------------------------------------
const healthyCount = computed(() => serverNodes.value.filter(s => s.status === 'healthy').length)
const warningCount = computed(() => serverNodes.value.filter(s => s.status === 'warning').length)
const criticalCount = computed(() => serverNodes.value.filter(s => s.status === 'critical').length)

const overallStatus = computed(() => {
  if (criticalCount.value > 0) return 'critical'
  if (warningCount.value > 0) return 'warning'
  return 'healthy'
})

const overallStatusLabel = computed(() => {
  switch (overallStatus.value) {
    case 'critical': return 'Критично'
    case 'warning': return 'Внимание'
    default: return 'Стабильна'
  }
})

// ---------------------------------------------------------------------------
// UI Helpers (delegated to useStatusHelpers composable)
// ---------------------------------------------------------------------------
const { infraStatusColor: statusColor, infraStatusBg: statusBg, infraStatusLabel: statusLabel } = useStatusHelpers()

function loadGradient(value: number): string {
  if (value >= 85) {
    return isDark.value ? 'linear-gradient(90deg, #F87171, #EF4444)' : 'linear-gradient(90deg, #E03E3E, #C92B2B)'
  }
  if (value >= 70) {
    return isDark.value ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)'
  }
  return isDark.value ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)'
}

function trendIcon(trend: string): string {
  switch (trend) {
    case 'up': return 'i-lucide-trending-up'
    case 'down': return 'i-lucide-trending-down'
    default: return 'i-lucide-minus'
  }
}
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- Page header -->
    <div class="flex items-center justify-between">
      <div>
        <h1
          class="text-2xl font-bold"
          style="color: var(--argus-text);"
        >
          Infrastructure Health
        </h1>
        <span
          v-if="usingDemoData"
          class="inline-flex items-center gap-1 mt-2 px-2 py-1 rounded-full text-[10px] font-bold"
          :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
        >
          <UIcon
            name="i-lucide-flask-conical"
            class="size-3"
          />
          Demo data
        </span>
        <p
          class="text-sm mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Мониторинг серверной инфраструктуры Docker Compose кластера
        </p>
      </div>
      <div class="flex items-center gap-3">
        <span
          v-if="lastUpdated"
          class="text-[10px] font-mono"
          style="color: var(--argus-text-dimmed);"
        >
          Обновлено: {{ lastUpdated }}
        </span>
        <span class="relative flex size-2">
          <span
            class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
            :style="{ background: statusColor(overallStatus) }"
          />
          <span
            class="relative inline-flex size-2 rounded-full"
            :style="{ background: statusColor(overallStatus) }"
          />
        </span>
      </div>
    </div>

    <!-- Loading / Error state -->
    <div
      v-if="loading && serverNodes.length === 0"
      class="glass-card rounded-xl p-8 text-center"
    >
      <UIcon
        name="i-lucide-loader-2"
        class="size-6 animate-spin mx-auto"
        style="color: var(--argus-accent);"
      />
      <p
        class="text-sm mt-3"
        style="color: var(--argus-text-dimmed);"
      >
        Загрузка метрик инфраструктуры...
      </p>
    </div>

    <div
      v-else-if="error && serverNodes.length === 0"
      class="glass-card rounded-xl p-8 text-center"
    >
      <UIcon
        name="i-lucide-alert-triangle"
        class="size-6 mx-auto"
        style="color: var(--argus-error);"
      />
      <p
        class="text-sm mt-3"
        style="color: var(--argus-error);"
      >
        {{ error }}
      </p>
      <button
        class="mt-3 px-4 py-2 rounded-lg text-xs font-semibold cursor-pointer"
        :style="{ background: 'var(--argus-accent)', color: '#fff' }"
        @click="fetchInfraStats"
      >
        Повторить
      </button>
    </div>

    <template v-else>
      <!-- Top KPI row -->
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <!-- Capacity -->
        <div class="glass-card rounded-xl p-5">
          <p
            class="text-[11px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Загрузка платформы (CPU)
          </p>
          <div class="flex items-baseline gap-1.5 mt-2">
            <span
              class="text-3xl font-bold"
              style="color: var(--argus-text);"
            >{{ Math.round(loadPercent) }}%</span>
            <span
              class="text-xs"
              style="color: var(--argus-text-dimmed);"
            >средняя CPU</span>
          </div>
          <div
            class="mt-3 w-full h-2 rounded-full overflow-hidden"
            style="background: var(--argus-bg-hover);"
          >
            <div
              class="h-full rounded-full transition-all duration-700"
              :style="{ width: `${Math.min(loadPercent, 100)}%`, background: loadGradient(loadPercent) }"
            />
          </div>
          <p
            class="text-[10px] mt-2"
            style="color: var(--argus-text-dimmed);"
          >
            {{ serverNodes.length }} контейнеров Docker
          </p>
        </div>

        <!-- Server Status Summary -->
        <div class="glass-card rounded-xl p-5">
          <p
            class="text-[11px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Статус контейнеров
          </p>
          <div class="flex items-center gap-4 mt-3">
            <div class="flex items-center gap-1.5">
              <span
                class="size-2 rounded-full"
                style="background: var(--argus-success);"
              />
              <span
                class="text-lg font-bold"
                style="color: var(--argus-success);"
              >{{ healthyCount }}</span>
            </div>
            <div class="flex items-center gap-1.5">
              <span
                class="size-2 rounded-full"
                style="background: var(--argus-warning);"
              />
              <span
                class="text-lg font-bold"
                style="color: var(--argus-warning);"
              >{{ warningCount }}</span>
            </div>
            <div class="flex items-center gap-1.5">
              <span
                class="size-2 rounded-full"
                style="background: var(--argus-error);"
              />
              <span
                class="text-lg font-bold"
                style="color: var(--argus-error);"
              >{{ criticalCount }}</span>
            </div>
          </div>
          <p
            class="text-xs mt-2"
            style="color: var(--argus-text-dimmed);"
          >
            {{ serverNodes.length }} нод всего
          </p>
        </div>

        <!-- Avg Latency -->
        <div class="glass-card rounded-xl p-5">
          <p
            class="text-[11px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Средняя задержка
          </p>
          <p
            class="text-3xl font-bold mt-2"
            style="color: var(--argus-text);"
          >
            {{ Math.round(avgLatencyMs) || '—' }}<span
              class="text-base font-normal ml-0.5"
              style="color: var(--argus-text-dimmed);"
            >мс</span>
          </p>
          <p
            class="text-xs mt-1.5"
            style="color: var(--argus-text-dimmed);"
          >
            p99: {{ Math.round(p99LatencyMs) || '—' }}мс
          </p>
        </div>

        <!-- Network Stability -->
        <div class="glass-card rounded-xl p-5">
          <p
            class="text-[11px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Состояние кластера
          </p>
          <div class="flex items-center gap-2 mt-2">
            <span class="relative flex size-2.5">
              <span
                class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
                :style="{ background: statusColor(overallStatus) }"
              />
              <span
                class="relative inline-flex size-2.5 rounded-full"
                :style="{ background: statusColor(overallStatus) }"
              />
            </span>
            <p
              class="text-lg font-bold"
              style="color: var(--argus-text);"
            >
              {{ overallStatusLabel }}
            </p>
          </div>
          <p
            class="text-xs mt-1.5"
            style="color: var(--argus-text-dimmed);"
          >
            Обновление каждые 10 сек
          </p>
        </div>
      </div>

      <!-- Charts Row -->
      <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <!-- Latency Chart -->
        <div class="glass-card rounded-xl overflow-hidden">
          <div
            class="flex items-center justify-between px-5 py-4 border-b"
            style="border-color: var(--argus-border);"
          >
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-activity"
                class="size-4"
                style="color: var(--argus-text-dimmed);"
              />
              <h2
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Задержка (Latency)
              </h2>
            </div>
            <span
              class="text-[10px] font-medium px-2 py-1 rounded-full"
              :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }"
            >
              {{ latencyHistory.length > 0 ? `${latencyHistory.length} точек` : 'Нет данных' }}
            </span>
          </div>
          <div class="p-5">
            <LatencyChart
              v-if="latencyHistory.length > 0"
              :data="latencyHistory"
              :dark-mode="isDark"
            />
            <div
              v-else
              class="text-center py-8"
            >
              <UIcon
                name="i-lucide-bar-chart-3"
                class="size-8 mx-auto"
                style="color: var(--argus-text-dimmed); opacity: 0.3;"
              />
              <p
                class="text-xs mt-2"
                style="color: var(--argus-text-dimmed);"
              >
                Данные о задержках появятся после нескольких циклов сбора
              </p>
            </div>
          </div>
        </div>

        <!-- Network Metrics -->
        <div class="glass-card rounded-xl overflow-hidden">
          <div
            class="flex items-center justify-between px-5 py-4 border-b"
            style="border-color: var(--argus-border);"
          >
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-network"
                class="size-4"
                style="color: var(--argus-text-dimmed);"
              />
              <h2
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Системные метрики
              </h2>
            </div>
          </div>
          <div
            class="divide-y"
            style="border-color: var(--argus-border-subtle);"
          >
            <div
              v-for="metric in networkMetrics"
              :key="metric.label"
              class="flex items-center justify-between px-5 py-3.5 transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <div class="flex items-center gap-3">
                <div
                  class="size-2 rounded-full"
                  :style="{ background: statusColor(metric.status) }"
                />
                <span
                  class="text-sm"
                  style="color: var(--argus-text);"
                >{{ metric.label }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span
                  class="text-sm font-semibold"
                  style="color: var(--argus-text);"
                >
                  {{ typeof metric.value === 'number' ? metric.value.toLocaleString() : metric.value }}{{ metric.unit ? ` ${metric.unit}` : '' }}
                </span>
                <UIcon
                  :name="trendIcon(metric.trend)"
                  class="size-3.5"
                  :style="{ color: metric.trend === 'up' ? 'var(--argus-warning)' : metric.trend === 'down' ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
                />
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Server Nodes Table -->
      <div class="glass-card rounded-xl overflow-hidden">
        <div
          class="flex items-center justify-between px-5 py-4 border-b"
          style="border-color: var(--argus-border);"
        >
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-hard-drive"
              class="size-4"
              style="color: var(--argus-text-dimmed);"
            />
            <h2
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Docker Контейнеры
            </h2>
          </div>
          <span
            class="text-[10px] font-medium px-2 py-1 rounded-full"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ serverNodes.length }} контейнеров
          </span>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr style="border-bottom: 1px solid var(--argus-border);">
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  Сервис
                </th>
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  Хост
                </th>
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  CPU
                </th>
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  RAM
                </th>
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  RAM (MB)
                </th>
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  Диск
                </th>
                <th
                  class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  Статус
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="node in serverNodes"
                :key="node.id"
                class="transition-colors"
                style="border-bottom: 1px solid var(--argus-border-subtle);"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              >
                <td class="px-5 py-3">
                  <div>
                    <p
                      class="font-medium"
                      style="color: var(--argus-text);"
                    >
                      {{ node.name }}
                    </p>
                    <p
                      class="text-[10px] font-mono"
                      style="color: var(--argus-text-dimmed);"
                    >
                      {{ node.id }}
                    </p>
                  </div>
                </td>
                <td
                  class="px-5 py-3"
                  style="color: var(--argus-text-muted);"
                >
                  {{ node.region }}
                </td>
                <td class="px-5 py-3">
                  <div class="flex items-center gap-2 min-w-24">
                    <div
                      class="flex-1 h-1.5 rounded-full overflow-hidden"
                      style="background: var(--argus-bg-hover);"
                    >
                      <div
                        class="h-full rounded-full transition-all duration-500"
                        :style="{ width: `${Math.min(node.cpuLoad, 100)}%`, background: loadGradient(node.cpuLoad) }"
                      />
                    </div>
                    <span
                      class="text-xs font-mono tabular-nums"
                      style="color: var(--argus-text-dimmed);"
                    >{{ node.cpuLoad }}%</span>
                  </div>
                </td>
                <td class="px-5 py-3">
                  <div class="flex items-center gap-2 min-w-24">
                    <div
                      class="flex-1 h-1.5 rounded-full overflow-hidden"
                      style="background: var(--argus-bg-hover);"
                    >
                      <div
                        class="h-full rounded-full transition-all duration-500"
                        :style="{ width: `${Math.min(node.memoryUsage, 100)}%`, background: loadGradient(node.memoryUsage) }"
                      />
                    </div>
                    <span
                      class="text-xs font-mono tabular-nums"
                      style="color: var(--argus-text-dimmed);"
                    >{{ node.memoryUsage }}%</span>
                  </div>
                </td>
                <td class="px-5 py-3">
                  <span
                    class="text-xs font-mono tabular-nums"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ Math.round(node.memUsedMB) }} / {{ Math.round(node.memLimitMB) }}
                  </span>
                </td>
                <td class="px-5 py-3">
                  <div class="flex items-center gap-2 min-w-24">
                    <div
                      class="flex-1 h-1.5 rounded-full overflow-hidden"
                      style="background: var(--argus-bg-hover);"
                    >
                      <div
                        class="h-full rounded-full transition-all duration-500"
                        :style="{ width: `${Math.min(node.diskUsage, 100)}%`, background: loadGradient(node.diskUsage) }"
                      />
                    </div>
                    <span
                      class="text-xs font-mono tabular-nums"
                      style="color: var(--argus-text-dimmed);"
                    >{{ node.diskUsage }}%</span>
                  </div>
                </td>
                <td class="px-5 py-3">
                  <span
                    class="text-[11px] font-medium px-2.5 py-1 rounded-full"
                    :style="{ background: statusBg(node.status, 0.1), color: statusColor(node.status) }"
                  >
                    {{ statusLabel(node.status) }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </template>
  </div>
</template>
