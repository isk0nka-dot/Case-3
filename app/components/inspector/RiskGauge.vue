<script setup lang="ts">
// =============================================================================
// RiskGauge — SVG arc gauge (0-100 risk score)
// =============================================================================
// 36x36 SVG viewBox. Background arc + filled arc via stroke-dashoffset.
// Color: green (<30), amber (30-60), red (>60).
// =============================================================================

const props = withDefaults(defineProps<{
  score: number
  size?: number
}>(), {
  size: 36
})

const radius = 13
const circumference = 2 * Math.PI * radius
const strokeWidth = 3

const dashOffset = computed(() => {
  const pct = Math.max(0, Math.min(100, props.score)) / 100
  return circumference * (1 - pct)
})

const color = computed(() => {
  if (props.score >= 60) return 'var(--argus-error)'
  if (props.score >= 30) return 'var(--argus-warning)'
  return 'var(--argus-success)'
})

const trackColor = computed(() => {
  if (props.score >= 60) return 'rgba(248, 113, 113, 0.15)'
  if (props.score >= 30) return 'rgba(251, 191, 36, 0.15)'
  return 'rgba(52, 211, 153, 0.15)'
})
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 36 36"
    class="risk-gauge"
  >
    <!-- Background arc -->
    <circle
      cx="18" cy="18" :r="radius"
      fill="none"
      :stroke="trackColor"
      :stroke-width="strokeWidth"
      stroke-linecap="round"
      transform="rotate(-90 18 18)"
      :stroke-dasharray="circumference"
      stroke-dashoffset="0"
    />
    <!-- Filled arc -->
    <circle
      cx="18" cy="18" :r="radius"
      fill="none"
      :stroke="color"
      :stroke-width="strokeWidth"
      stroke-linecap="round"
      transform="rotate(-90 18 18)"
      :stroke-dasharray="circumference"
      :stroke-dashoffset="dashOffset"
      class="risk-gauge-arc"
    />
    <!-- Score text -->
    <text
      x="18" y="19"
      text-anchor="middle"
      dominant-baseline="central"
      :fill="color"
      font-size="9"
      font-weight="700"
      font-family="'DM Sans', system-ui, sans-serif"
    >
      {{ Math.round(score) }}
    </text>
  </svg>
</template>

<style scoped>
.risk-gauge-arc {
  transition: stroke-dashoffset 0.6s cubic-bezier(0.4, 0, 0.2, 1),
              stroke 0.4s ease;
}
</style>
