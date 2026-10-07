<script setup lang="ts">
import { computed, watch, ref } from 'vue'
import { Bar } from 'vue-chartjs'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Tooltip,
  Legend
} from 'chart.js'
import type { ViolationTrendPoint } from '~/stores/useDashboardStore'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip, Legend)

const props = defineProps<{
  data: ViolationTrendPoint[]
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
      label: 'Телефон',
      data: props.data.map(d => d.phoneDetected),
      backgroundColor: props.darkMode ? 'rgba(56, 189, 248, 0.85)' : 'rgba(37, 99, 235, 0.8)',
      borderRadius: 4,
      borderSkipped: false as const,
      barPercentage: 0.7,
      categoryPercentage: 0.8
    },
    {
      label: 'Взгляд',
      data: props.data.map(d => d.gazeDeviation),
      backgroundColor: props.darkMode ? 'rgba(139, 92, 246, 0.75)' : 'rgba(139, 92, 246, 0.7)',
      borderRadius: 4,
      borderSkipped: false as const,
      barPercentage: 0.7,
      categoryPercentage: 0.8
    },
    {
      label: 'Посторонние',
      data: props.data.map(d => d.multiplePersons),
      backgroundColor: props.darkMode ? 'rgba(248, 113, 113, 0.75)' : 'rgba(224, 62, 62, 0.7)',
      borderRadius: 4,
      borderSkipped: false as const,
      barPercentage: 0.7,
      categoryPercentage: 0.8
    },
    {
      label: 'Вкладки',
      data: props.data.map(d => d.tabSwitch),
      backgroundColor: props.darkMode ? 'rgba(52, 211, 153, 0.7)' : 'rgba(16, 163, 74, 0.7)',
      borderRadius: 4,
      borderSkipped: false as const,
      barPercentage: 0.7,
      categoryPercentage: 0.8
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
        pointStyle: 'rectRounded',
        padding: 16,
        boxWidth: 8,
        boxHeight: 8
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
      displayColors: true,
      boxPadding: 4
    }
  },
  scales: {
    x: {
      stacked: true,
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
      stacked: true,
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
        stepSize: 10
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
    <Bar
      :key="chartKey"
      :data="chartData"
      :options="chartOptions"
    />
  </div>
</template>
