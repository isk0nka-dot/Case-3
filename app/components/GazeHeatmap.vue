<script setup lang="ts">
// =============================================================================
// GazeHeatmap — Gaze tracking heatmap visualization
// =============================================================================
//
// Renders a 10×10 grid heatmap showing where a student has been looking
// during the proctoring session. Uses CSS grid with opacity-based intensity.
//
// Data source: useTelemetryStore → sessions.get(sessionId).heatmap
//
// Performance:
//   The heatmap grid is 100 cells (10×10). Each cell is a simple div with
//   an inline background-color opacity. At 2Hz update rate, this is trivial
//   for the browser to render.
//
// =============================================================================

import { useTelemetryStore, type HeatmapCell } from '~/stores/useTelemetryStore'

const props = defineProps<{
  sessionId: string
  width?: number
  height?: number
}>()

const telemetryStore = useTelemetryStore()

const heatmap = computed((): HeatmapCell[] => {
  const session = telemetryStore.getSessionTelemetry(props.sessionId)
  return session?.heatmap ?? []
})

const currentGaze = computed(() => {
  const session = telemetryStore.getSessionTelemetry(props.sessionId)
  return session?.currentGaze
})

function getHeatColor(intensity: number): string {
  if (intensity === 0) return 'transparent'
  // Blue → Cyan → Green → Yellow → Red
  if (intensity < 0.25) return `rgba(59, 130, 246, ${intensity * 4 * 0.6})`
  if (intensity < 0.5) return `rgba(34, 197, 94, ${0.4 + intensity * 0.4})`
  if (intensity < 0.75) return `rgba(234, 179, 8, ${0.5 + intensity * 0.3})`
  return `rgba(239, 68, 68, ${0.6 + intensity * 0.4})`
}
</script>

<template>
  <div
    class="relative rounded-lg overflow-hidden bg-[var(--argus-bg-deep)] border border-[var(--argus-border-subtle)]"
    :style="{ width: `${width ?? 200}px`, height: `${height ?? 200}px` }"
  >
    <!-- Heatmap grid -->
    <div class="absolute inset-0 grid grid-cols-10 grid-rows-10">
      <div
        v-for="cell in heatmap"
        :key="`${cell.gridX}-${cell.gridY}`"
        class="transition-colors duration-500"
        :style="{ backgroundColor: getHeatColor(cell.intensity) }"
      />
    </div>

    <!-- Current gaze position indicator -->
    <div
      v-if="currentGaze"
      class="absolute w-3 h-3 rounded-full bg-sky-400 border-2 border-white/50 shadow-lg shadow-sky-400/50 transition-all duration-100"
      :style="{
        left: `${currentGaze.x * 100}%`,
        top: `${currentGaze.y * 100}%`,
        transform: 'translate(-50%, -50%)'
      }"
    />

    <!-- Grid overlay (subtle lines) -->
    <div class="absolute inset-0 grid grid-cols-10 grid-rows-10 pointer-events-none">
      <div
        v-for="i in 100"
        :key="i"
        class="border-r border-b border-white/5"
      />
    </div>

    <!-- Label -->
    <div class="absolute bottom-1 left-1 text-[9px] text-white/30">
      Тепловая карта взгляда
    </div>
  </div>
</template>
