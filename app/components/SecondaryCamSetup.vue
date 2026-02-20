<script setup lang="ts">
// =============================================================================
// SecondaryCamSetup — Mobile Secondary Camera Pairing & Calibration UI
// =============================================================================
// Full-lifecycle component: QR code → pairing → calibration → stream health.
// Integrates with useSecondaryCam composable for backend orchestration.
// =============================================================================

import type { SidecamPolicy } from '~/composables/useAdminAPI'

const props = defineProps<{
  sessionId: string
  studentId: string
  examId: string
  orgId: string
  policy: SidecamPolicy
}>()

const emit = defineEmits<{
  ready: []
  failed: [reason: string]
  disconnected: []
  pairingStarted: []
}>()

const { isDark, errorBg, successBg, warningBg, accentBg } = useColors()

const sidecam = useSecondaryCam()

// Auto-initiate pairing on mount if policy is not disabled
onMounted(async () => {
  if (props.policy === 'disabled') return

  await sidecam.initiatePairing({
    sessionId: props.sessionId,
    studentId: props.studentId,
    examId: props.examId,
    orgId: props.orgId,
    policy: props.policy,
  })
  emit('pairingStarted')
})

// Emit events on phase changes
watch(() => sidecam.phase.value, (phase) => {
  if (phase === 'ready') emit('ready')
  if (phase === 'failed') emit('failed', sidecam.error.value)
  if (phase === 'disconnected') emit('disconnected')
})

// Phase display helpers
const phaseIcon = computed(() => {
  switch (sidecam.phase.value) {
    case 'pairing': return 'i-lucide-qr-code'
    case 'calibrating': return 'i-lucide-scan-eye'
    case 'ready': return 'i-lucide-check-circle'
    case 'disconnected': return 'i-lucide-wifi-off'
    case 'failed': return 'i-lucide-x-circle'
    default: return 'i-lucide-smartphone'
  }
})

const phaseLabel = computed(() => {
  switch (sidecam.phase.value) {
    case 'idle': return 'Ожидание'
    case 'pairing': return 'Сканируйте QR-код'
    case 'calibrating': return 'Калибровка камеры'
    case 'ready': return 'Камера готова'
    case 'disconnected': return 'Связь потеряна'
    case 'failed': return 'Ошибка'
    default: return ''
  }
})

const phaseColor = computed(() => {
  switch (sidecam.phase.value) {
    case 'ready': return 'text-green-400'
    case 'calibrating': return 'text-amber-400'
    case 'disconnected': return 'text-orange-400'
    case 'failed': return 'text-red-400'
    default: return 'text-blue-400'
  }
})

const qualityColor = computed(() => {
  switch (sidecam.streamQuality.value) {
    case 'excellent': return 'text-green-400'
    case 'good': return 'text-blue-400'
    case 'degraded': return 'text-amber-400'
    case 'critical': return 'text-red-400'
    default: return 'text-gray-400'
  }
})

const qualityLabel = computed(() => {
  switch (sidecam.streamQuality.value) {
    case 'excellent': return 'Отлично'
    case 'good': return 'Хорошо'
    case 'degraded': return 'Снижено'
    case 'critical': return 'Критично'
    default: return '—'
  }
})

const batteryPercent = computed(() => Math.round(sidecam.batteryLevel.value * 100))

const batteryIcon = computed(() => {
  if (sidecam.batteryCritical.value) return 'i-lucide-battery-warning'
  if (sidecam.batteryWarning.value) return 'i-lucide-battery-low'
  return 'i-lucide-battery-full'
})

const batteryColor = computed(() => {
  if (sidecam.batteryCritical.value) return 'text-red-400'
  if (sidecam.batteryWarning.value) return 'text-amber-400'
  return 'text-green-400'
})

const thermalIcon = computed(() => {
  switch (sidecam.thermalState.value) {
    case 'critical': return 'i-lucide-flame'
    case 'serious': return 'i-lucide-thermometer'
    case 'fair': return 'i-lucide-thermometer'
    default: return 'i-lucide-snowflake'
  }
})

const thermalColor = computed(() => {
  switch (sidecam.thermalState.value) {
    case 'critical': return 'text-red-400'
    case 'serious': return 'text-orange-400'
    case 'fair': return 'text-amber-400'
    default: return 'text-blue-400'
  }
})

// Retry pairing
async function retryPairing() {
  await sidecam.initiatePairing({
    sessionId: props.sessionId,
    studentId: props.studentId,
    examId: props.examId,
    orgId: props.orgId,
    policy: props.policy,
  })
}

onUnmounted(() => {
  sidecam.stopPolling()
})
</script>

<template>
  <div
    class="glass-card rounded-xl p-4 space-y-4"
    :class="isDark ? 'bg-white/5 border-white/10' : 'bg-gray-50 border-gray-200'"
  >
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <div :class="[phaseIcon, phaseColor, 'w-5 h-5']" />
        <span class="font-semibold text-sm" :class="phaseColor">{{ phaseLabel }}</span>
      </div>
      <span
        v-if="props.policy === 'mandatory'"
        class="text-[10px] px-2 py-0.5 rounded-full font-medium"
        :class="isDark ? 'bg-red-500/20 text-red-400' : 'bg-red-100 text-red-600'"
      >
        Обязательно
      </span>
      <span
        v-else-if="props.policy === 'optional'"
        class="text-[10px] px-2 py-0.5 rounded-full font-medium"
        :class="isDark ? 'bg-blue-500/20 text-blue-400' : 'bg-blue-100 text-blue-600'"
      >
        Опционально
      </span>
    </div>

    <!-- Loading -->
    <div v-if="sidecam.isLoading.value" class="flex items-center justify-center py-6">
      <div class="w-6 h-6 border-2 border-blue-400 border-t-transparent rounded-full animate-spin" />
      <span class="ml-2 text-xs opacity-60">Инициализация...</span>
    </div>

    <!-- QR Code Phase -->
    <div v-else-if="sidecam.phase.value === 'pairing'" class="space-y-3">
      <div
        class="mx-auto w-48 h-48 rounded-lg flex items-center justify-center"
        :class="isDark ? 'bg-white' : 'bg-white border border-gray-200'"
      >
        <!-- QR code rendered via data URI or canvas -->
        <div class="text-center p-3">
          <div class="i-lucide-qr-code w-16 h-16 mx-auto text-gray-800 mb-2" />
          <p class="text-[10px] text-gray-600 break-all font-mono leading-tight">
            {{ sidecam.qrData.value?.slice(0, 60) }}...
          </p>
        </div>
      </div>
      <p class="text-xs text-center opacity-60">
        Откройте приложение Argus на мобильном устройстве и отсканируйте QR-код
      </p>
      <div class="text-center">
        <span class="text-[10px] opacity-40">
          Код истекает через 5 минут
        </span>
      </div>
    </div>

    <!-- Calibrating Phase -->
    <div v-else-if="sidecam.phase.value === 'calibrating'" class="space-y-3">
      <div
        class="rounded-lg p-3"
        :class="isDark ? 'bg-amber-500/10 border border-amber-500/20' : 'bg-amber-50 border border-amber-200'"
      >
        <div class="flex items-start gap-2">
          <div class="i-lucide-scan-eye w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
          <div class="space-y-1">
            <p class="text-xs font-medium">Калибровка камеры</p>
            <p class="text-[10px] opacity-70">
              {{ sidecam.calibration.value?.message || 'Расположите камеру под углом 45-60° к монитору' }}
            </p>
          </div>
        </div>
      </div>

      <!-- Calibration checks -->
      <div class="grid grid-cols-2 gap-2">
        <div
          class="flex items-center gap-1.5 p-2 rounded-lg text-xs"
          :class="sidecam.calibration.value?.handsVisible
            ? (isDark ? 'bg-green-500/10 text-green-400' : 'bg-green-50 text-green-600')
            : (isDark ? 'bg-white/5 opacity-40' : 'bg-gray-50 opacity-50')"
        >
          <div class="i-lucide-hand w-3.5 h-3.5" />
          Руки видны
        </div>
        <div
          class="flex items-center gap-1.5 p-2 rounded-lg text-xs"
          :class="sidecam.calibration.value?.keyboardVisible
            ? (isDark ? 'bg-green-500/10 text-green-400' : 'bg-green-50 text-green-600')
            : (isDark ? 'bg-white/5 opacity-40' : 'bg-gray-50 opacity-50')"
        >
          <div class="i-lucide-keyboard w-3.5 h-3.5" />
          Клавиатура
        </div>
        <div
          class="flex items-center gap-1.5 p-2 rounded-lg text-xs"
          :class="sidecam.calibration.value?.screenEdge
            ? (isDark ? 'bg-green-500/10 text-green-400' : 'bg-green-50 text-green-600')
            : (isDark ? 'bg-white/5 opacity-40' : 'bg-gray-50 opacity-50')"
        >
          <div class="i-lucide-monitor w-3.5 h-3.5" />
          Экран
        </div>
        <div
          class="flex items-center gap-1.5 p-2 rounded-lg text-xs"
          :class="(sidecam.calibration.value?.viewAngle ?? 0) >= 35 && (sidecam.calibration.value?.viewAngle ?? 0) <= 70
            ? (isDark ? 'bg-green-500/10 text-green-400' : 'bg-green-50 text-green-600')
            : (isDark ? 'bg-white/5 opacity-40' : 'bg-gray-50 opacity-50')"
        >
          <div class="i-lucide-rotate-3d w-3.5 h-3.5" />
          {{ sidecam.calibration.value?.viewAngle ? `${Math.round(sidecam.calibration.value.viewAngle)}°` : '—°' }}
        </div>
      </div>

      <p class="text-[10px] text-center opacity-40">
        Попытка {{ sidecam.calibration.value?.attemptCount ?? 0 }}
      </p>
    </div>

    <!-- Ready Phase -->
    <div v-else-if="sidecam.phase.value === 'ready'" class="space-y-3">
      <!-- Stream Health Bar -->
      <div
        class="flex items-center justify-between p-2 rounded-lg"
        :class="isDark ? 'bg-green-500/10 border border-green-500/20' : 'bg-green-50 border border-green-200'"
      >
        <div class="flex items-center gap-2">
          <div class="w-2 h-2 rounded-full bg-green-400 animate-pulse" />
          <span class="text-xs font-medium text-green-400">Трансляция</span>
        </div>
        <span class="text-[10px]" :class="qualityColor">{{ qualityLabel }}</span>
      </div>

      <!-- Device Stats Grid -->
      <div class="grid grid-cols-3 gap-2">
        <!-- Battery -->
        <div class="text-center p-2 rounded-lg" :class="isDark ? 'bg-white/5' : 'bg-gray-50'">
          <div :class="[batteryIcon, batteryColor, 'w-4 h-4 mx-auto mb-1']" />
          <span class="text-xs font-mono" :class="batteryColor">{{ batteryPercent }}%</span>
        </div>
        <!-- Thermal -->
        <div class="text-center p-2 rounded-lg" :class="isDark ? 'bg-white/5' : 'bg-gray-50'">
          <div :class="[thermalIcon, thermalColor, 'w-4 h-4 mx-auto mb-1']" />
          <span class="text-xs font-mono" :class="thermalColor">
            {{ sidecam.thermalState.value === 'nominal' ? 'ОК' : sidecam.thermalState.value }}
          </span>
        </div>
        <!-- Latency -->
        <div class="text-center p-2 rounded-lg" :class="isDark ? 'bg-white/5' : 'bg-gray-50'">
          <div class="i-lucide-activity w-4 h-4 mx-auto mb-1" :class="qualityColor" />
          <span class="text-xs font-mono" :class="qualityColor">
            {{ sidecam.pairingSession.value?.health?.latencyMs ?? '—' }}ms
          </span>
        </div>
      </div>

      <!-- Anomaly Alerts -->
      <div v-if="!sidecam.handsOnDesk.value" class="flex items-center gap-2 p-2 rounded-lg" :class="isDark ? 'bg-red-500/10 border border-red-500/20' : 'bg-red-50 border border-red-200'">
        <div class="i-lucide-alert-triangle w-4 h-4 text-red-400" />
        <span class="text-xs text-red-400">Руки не на столе</span>
      </div>

      <div v-if="sidecam.displacementAlert.value" class="flex items-center gap-2 p-2 rounded-lg" :class="isDark ? 'bg-red-500/10 border border-red-500/20' : 'bg-red-50 border border-red-200'">
        <div class="i-lucide-move w-4 h-4 text-red-400" />
        <span class="text-xs text-red-400">Устройство перемещено</span>
      </div>

      <!-- Device Directive Alerts -->
      <div
        v-for="alert in sidecam.alerts.value"
        :key="alert.type"
        class="flex items-center gap-2 p-2 rounded-lg"
        :class="alert.severity === 'critical'
          ? (isDark ? 'bg-red-500/10 border border-red-500/20' : 'bg-red-50 border border-red-200')
          : (isDark ? 'bg-amber-500/10 border border-amber-500/20' : 'bg-amber-50 border border-amber-200')"
      >
        <div
          class="w-4 h-4"
          :class="alert.severity === 'critical' ? 'i-lucide-alert-octagon text-red-400' : 'i-lucide-alert-triangle text-amber-400'"
        />
        <span class="text-xs" :class="alert.severity === 'critical' ? 'text-red-400' : 'text-amber-400'">
          {{ alert.message }}
        </span>
      </div>
    </div>

    <!-- Disconnected Phase -->
    <div v-else-if="sidecam.phase.value === 'disconnected'" class="space-y-3">
      <div
        class="flex items-center gap-2 p-3 rounded-lg"
        :class="isDark ? 'bg-orange-500/10 border border-orange-500/20' : 'bg-orange-50 border border-orange-200'"
      >
        <div class="i-lucide-wifi-off w-5 h-5 text-orange-400" />
        <div>
          <p class="text-xs font-medium text-orange-400">Связь потеряна</p>
          <p class="text-[10px] opacity-60">Ожидание восстановления...</p>
        </div>
      </div>
      <p v-if="props.policy === 'mandatory'" class="text-[10px] text-red-400 text-center">
        Экзамен будет приостановлен до восстановления связи
      </p>
    </div>

    <!-- Failed Phase -->
    <div v-else-if="sidecam.phase.value === 'failed'" class="space-y-3">
      <div
        class="flex items-center gap-2 p-3 rounded-lg"
        :class="isDark ? 'bg-red-500/10 border border-red-500/20' : 'bg-red-50 border border-red-200'"
      >
        <div class="i-lucide-x-circle w-5 h-5 text-red-400" />
        <div>
          <p class="text-xs font-medium text-red-400">Ошибка сопряжения</p>
          <p class="text-[10px] opacity-60">{{ sidecam.error.value }}</p>
        </div>
      </div>
      <button
        class="w-full px-3 py-2 text-xs font-medium rounded-lg transition-colors"
        :class="isDark ? 'bg-blue-500/20 text-blue-400 hover:bg-blue-500/30' : 'bg-blue-100 text-blue-600 hover:bg-blue-200'"
        @click="retryPairing"
      >
        Повторить сопряжение
      </button>
    </div>

    <!-- Device Info (shown when connected) -->
    <div
      v-if="sidecam.deviceInfo.value && sidecam.phase.value !== 'failed'"
      class="pt-2 border-t"
      :class="isDark ? 'border-white/5' : 'border-gray-200'"
    >
      <div class="flex items-center justify-between text-[10px] opacity-40">
        <span>{{ sidecam.deviceInfo.value.deviceModel }}</span>
        <span>{{ sidecam.deviceInfo.value.osVersion }}</span>
      </div>
    </div>
  </div>
</template>
