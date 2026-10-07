<script setup lang="ts">
import { computed, watch, ref } from 'vue'
import { Line } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Filler,
  Tooltip,
  Legend
} from 'chart.js'
import type { LatencyPoint } from '~/stores/useDashboardStore'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Filler, Tooltip, Legend)

const props = defineProps<{
  data: LatencyPoint[]
  darkMode: boolean
}>()

const chartKey = ref(0)
watch(() => props.darkMode, () => {
  chartKey.value++
})

const chartData = computed(() => ({
  labels: props.data.map(d => d.time),
  datasets: [
    {
      label: 'Avg',
      data: props.data.map(d => d.avg),
      borderColor: props.darkMode ? '#38BDF8' : '#2563EB',
      backgroundColor: props.darkMode ? 'rgba(56, 189, 248, 0.08)' : 'rgba(37, 99, 235, 0.06)',
      fill: true,
      tension: 0.4,
      borderWidth: 2,
      pointRadius: 0,
      pointHoverRadius: 4,
      pointHoverBackgroundColor: props.darkMode ? '#38BDF8' : '#2563EB'
    },
    {
      label: 'p95',
      data: props.data.map(d => d.p95),
      borderColor: props.darkMode ? '#FBBF24' : '#E67E22',
      backgroundColor: 'transparent',
      borderDash: [4, 4],
      tension: 0.4,
      borderWidth: 1.5,
      pointRadius: 0,
      pointHoverRadius: 3,
      pointHoverBackgroundColor: props.darkMode ? '#FBBF24' : '#E67E22'
    },
    {
      label: 'p99',
      data: props.data.map(d => d.p99),
      borderColor: props.darkMode ? '#F87171' : '#E03E3E',
      backgroundColor: 'transparent',
      borderDash: [2, 2],
      tension: 0.4,
      borderWidth: 1.5,
      pointRadius: 0,
      pointHoverRadius: 3,
      pointHoverBackgroundColor: props.darkMode ? '#F87171' : '#E03E3E'
    }
  ]
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: {
    duration: 800,
    easing: 'easeInOutQuart' as const
  },
  interaction: {
    mode: 'index' as const,
    intersect: false
  },
  plugins: {
    legend: {
      display: true,
      position: 'bottom' as const,
      labels: {
        color: props.darkMode ? '#94A3B8' : '#3B4963',
        font: {
          family: 'DM Sans',
          size: 11
        },
        usePointStyle: true,
        pointStyle: 'line',
        padding: 16,
        boxWidth: 20,
        boxHeight: 0
      }
    },
    tooltip: {
      backgroundColor: props.darkMode ? '#1E293B' : '#FFFFFF',
      titleColor: props.darkMode ? '#E2E8F0' : '#0C1425',
      bodyColor: props.darkMode ? '#94A3B8' : '#3B4963',
      borderColor: props.darkMode ? '#334155' : '#D1DAE8',
      borderWidth: 1,
      cornerRadius: 8,
      padding: 12,
      titleFont: {
        family: 'DM Sans',
        size: 13,
        weight: 600 as const
      },
      bodyFont: {
        family: 'DM Sans',
        size: 12
      },
      callbacks: {
        label: (ctx: { dataset: { label?: string }, parsed: { y: number | null } }) => {
          return `${ctx.dataset.label ?? ''}: ${ctx.parsed.y ?? 0}ms`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        display: false
      },
      ticks: {
        color: props.darkMode ? '#64748B' : '#6B7D99',
        font: {
          family: 'DM Sans',
          size: 10
        },
        maxRotation: 0
      },
      border: {
        display: false
      }
    },
    y: {
      grid: {
        color: props.darkMode ? 'rgba(30, 41, 59, 0.5)' : 'rgba(193, 207, 232, 0.5)',
        lineWidth: 1
      },
      ticks: {
        color: props.darkMode ? '#64748B' : '#6B7D99',
        font: {
          family: 'DM Sans',
          size: 10
        },
        callback: (value: string | number) => `${value}ms`
      },
      border: {
        display: false
      }
    }
  }
}))
</script>

<template>
  <div class="w-full h-56">
    <Line
      :key="chartKey"
      :data="chartData"
      :options="chartOptions"
    />
  </div>
</template>
