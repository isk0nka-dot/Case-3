<script setup lang="ts">
// =============================================================================
// RiskSparkline — Canvas2D 60-second trend line
// =============================================================================
// Draws a polyline of the last 12 risk score data points (5s intervals = 60s).
// No Chart.js — pure Canvas2D for minimal overhead across 25+ instances.
// =============================================================================

const props = withDefaults(defineProps<{
  data: number[]
  color?: string
  width?: number
  height?: number
}>(), {
  color: 'var(--argus-accent)',
  width: 120,
  height: 20
})

const canvasRef = ref<HTMLCanvasElement | null>(null)

function draw() {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const w = props.width
  const h = props.height

  canvas.width = w * dpr
  canvas.height = h * dpr
  canvas.style.width = `${w}px`
  canvas.style.height = `${h}px`
  ctx.scale(dpr, dpr)

  ctx.clearRect(0, 0, w, h)

  const points = props.data
  if (points.length < 2) return

  const maxVal = 100
  const padding = 1
  const drawW = w - padding * 2
  const drawH = h - padding * 2
  const step = drawW / (Math.max(points.length - 1, 1))

  // Gradient fill under the line
  const gradient = ctx.createLinearGradient(0, 0, 0, h)

  // Parse CSS variable color — fallback to accent blue
  const resolvedColor = getComputedStyle(canvas).getPropertyValue('--sparkline-color').trim() || '#38BDF8'

  gradient.addColorStop(0, `${resolvedColor}30`)
  gradient.addColorStop(1, `${resolvedColor}05`)

  // Build path
  ctx.beginPath()
  for (let i = 0; i < points.length; i++) {
    const x = padding + i * step
    const y = padding + drawH - (points[i]! / maxVal) * drawH
    if (i === 0) ctx.moveTo(x, y)
    else ctx.lineTo(x, y)
  }

  // Stroke line
  ctx.strokeStyle = resolvedColor
  ctx.lineWidth = 1.5
  ctx.lineJoin = 'round'
  ctx.lineCap = 'round'
  ctx.stroke()

  // Fill area under the line
  const lastX = padding + (points.length - 1) * step
  ctx.lineTo(lastX, h)
  ctx.lineTo(padding, h)
  ctx.closePath()
  ctx.fillStyle = gradient
  ctx.fill()
}

// Redraw on data or color changes
watch(() => [props.data, props.color], draw, { deep: true })

onMounted(draw)
</script>

<template>
  <canvas
    ref="canvasRef"
    :style="{
      '--sparkline-color': color,
      width: `${width}px`,
      height: `${height}px`
    }"
    class="block"
  />
</template>
