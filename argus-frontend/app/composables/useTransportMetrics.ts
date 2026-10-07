// =============================================================================
// Argus AI — useTransportMetrics Composable
// =============================================================================
//
// Provides reactive transport metrics for the infrastructure health dashboard.
// Updates at 1Hz to show real-time connection quality, throughput, and latency.
//
// Usage:
//   const { metrics, isHealthy, statusLabel } = useTransportMetrics()
//
// =============================================================================

import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { TransportMetrics } from '~/lib/grpc/interceptors'

export function useTransportMetrics() {
  const metrics = ref<TransportMetrics>({
    totalRequests: 0,
    totalSuccesses: 0,
    totalFailures: 0,
    totalRetries: 0,
    avgLatencyMs: 0,
    p95LatencyMs: 0,
    totalBytesSent: 0,
    totalBytesReceived: 0,
    eventsPerSecond: 0
  })

  const isHealthy = ref(true)
  let updateTimer: ReturnType<typeof setInterval> | null = null

  function update() {
    try {
      const { $grpc } = useNuxtApp()
      if ($grpc) {
        metrics.value = $grpc.getMetrics()
        isHealthy.value = $grpc.isHealthy()
      }
    } catch {
      // Plugin not available.
    }
  }

  onMounted(() => {
    update()
    updateTimer = setInterval(update, 1000) // 1Hz
  })

  onUnmounted(() => {
    if (updateTimer) clearInterval(updateTimer)
  })

  const successRate = computed(() => {
    const total = metrics.value.totalRequests
    if (total === 0) return 100
    return Math.round((metrics.value.totalSuccesses / total) * 1000) / 10
  })

  const statusLabel = computed(() => {
    if (!isHealthy.value) return 'Отключено'
    if (metrics.value.totalRequests === 0) return 'Ожидание'
    if (metrics.value.avgLatencyMs > 1000) return 'Высокая задержка'
    return 'Стабильно'
  })

  const statusColor = computed(() => {
    if (!isHealthy.value) return 'text-red-400'
    if (metrics.value.avgLatencyMs > 500) return 'text-amber-400'
    return 'text-green-400'
  })

  function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  function resetMetrics() {
    try {
      const { $grpc } = useNuxtApp()
      $grpc?.resetMetrics()
    } catch {
      // Plugin not available.
    }
  }

  return {
    metrics,
    isHealthy,
    successRate,
    statusLabel,
    statusColor,
    formatBytes,
    resetMetrics
  }
}
