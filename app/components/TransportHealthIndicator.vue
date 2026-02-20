<script setup lang="ts">
// =============================================================================
// TransportHealthIndicator — gRPC transport connection health status
// =============================================================================
//
// Shows the health of the gRPC-Web connection to the Event Collector server.
// Displayed in the dashboard sidebar or header. Updates every 2 seconds.
//
// States:
//   - Connected (green): Transport is healthy, < 3 consecutive failures
//   - Degraded (amber): 1-2 consecutive failures, still retrying
//   - Disconnected (red): 3+ consecutive failures
//   - Idle (gray): No transport plugin available
//
// =============================================================================

const { $grpc } = useNuxtApp()

const health = ref<'connected' | 'degraded' | 'disconnected' | 'idle'>('idle')
const latency = ref(0)
const eventsPerSecond = ref(0)
const totalSent = ref(0)

let healthTimer: ReturnType<typeof setInterval> | null = null

function updateHealth() {
  if (!$grpc) {
    health.value = 'idle'
    return
  }

  const metrics = $grpc.getMetrics()
  const isHealthy = $grpc.isHealthy()

  totalSent.value = metrics.totalRequests
  latency.value = metrics.avgLatencyMs
  eventsPerSecond.value = metrics.eventsPerSecond

  if (isHealthy && metrics.totalRequests > 0) {
    health.value = 'connected'
  } else if (!isHealthy) {
    health.value = metrics.totalFailures > 5 ? 'disconnected' : 'degraded'
  } else {
    health.value = 'idle'
  }
}

onMounted(() => {
  healthTimer = setInterval(updateHealth, 2000)
  updateHealth()
})

onUnmounted(() => {
  if (healthTimer) clearInterval(healthTimer)
})

const statusConfig = computed(() => {
  switch (health.value) {
    case 'connected':
      return { color: 'bg-green-500', label: 'Подключено', textColor: 'text-green-400' }
    case 'degraded':
      return { color: 'bg-amber-500', label: 'Нестабильно', textColor: 'text-amber-400' }
    case 'disconnected':
      return { color: 'bg-red-500', label: 'Отключено', textColor: 'text-red-400' }
    default:
      return { color: 'bg-gray-500', label: 'Ожидание', textColor: 'text-gray-400' }
  }
})
</script>

<template>
  <div class="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-[var(--argus-bg-elevated)] border border-[var(--argus-border-subtle)]">
    <!-- Status dot -->
    <div class="relative">
      <div class="w-2 h-2 rounded-full" :class="statusConfig.color" />
      <div
        v-if="health === 'connected'"
        class="absolute inset-0 w-2 h-2 rounded-full animate-ping opacity-75"
        :class="statusConfig.color"
      />
    </div>

    <!-- Status label -->
    <span class="text-[11px] font-medium" :class="statusConfig.textColor">
      {{ statusConfig.label }}
    </span>

    <!-- Metrics (when connected) -->
    <template v-if="health === 'connected' || health === 'degraded'">
      <span class="text-[10px] text-[var(--argus-text-dimmed)] tabular-nums">
        {{ latency.toFixed(0) }}мс
      </span>
      <span class="text-[10px] text-[var(--argus-text-dimmed)] tabular-nums">
        {{ eventsPerSecond }}/с
      </span>
    </template>
  </div>
</template>
