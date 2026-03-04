<script setup lang="ts">
// =============================================================================
// SecurityShield — Unified Security Status HUD
// =============================================================================
// Compact floating indicator showing all security subsystem statuses.
// Positioned bottom-right of the proctoring viewport.
// Expands on hover to show subsystem details.
// =============================================================================

import type { BrowserIntegrityState } from '~/composables/useBrowserIntegrity'
import type { SpoofAnalysis } from '~/composables/useAntiSpoofing'
import type { VirtualCameraReport } from '~/composables/useVirtualCameraDetector'
import type { DeviceFingerprint } from '~/composables/useDeviceFingerprint'
import type { LivenessChallenge } from '~/composables/useLivenessChallenge'

const props = defineProps<{
  /** Browser integrity state */
  browserState: BrowserIntegrityState
  /** Anti-spoofing analysis */
  spoofAnalysis: SpoofAnalysis | null
  /** Virtual camera report */
  virtualCameraReport: VirtualCameraReport | null
  /** Device fingerprint */
  deviceFingerprint: DeviceFingerprint | null
  /** Current liveness challenge */
  currentChallenge: LivenessChallenge | null
  /** Liveness pass rate (0-1) */
  livenessPassRate: number
  /** Whether watermark is active */
  watermarkActive: boolean
}>()

const { isDark, errorBg, successBg, warningBg, accentBg } = useColors()

const expanded = ref(false)

// Subsystem statuses
interface SubsystemStatus {
  name: string
  icon: string
  status: 'ok' | 'warning' | 'critical' | 'inactive'
  detail: string
}

const subsystems = computed<SubsystemStatus[]>(() => {
  const list: SubsystemStatus[] = []

  // 1. Browser Integrity
  const bi = props.browserState
  const biIssues = (bi.tabSwitchCount > 3 ? 1 : 0)
    + (bi.devToolsOpen ? 1 : 0)
    + (bi.isSecondScreenDetected ? 1 : 0)
    + (bi.copyAttempts > 0 ? 1 : 0)
  list.push({
    name: 'Браузер',
    icon: 'i-lucide-globe',
    status: biIssues >= 2 ? 'critical' : biIssues >= 1 ? 'warning' : 'ok',
    detail: biIssues === 0
      ? 'Целостность'
      : `${bi.tabSwitchCount} перекл.${bi.devToolsOpen ? ', DevTools' : ''}${bi.isSecondScreenDetected ? ', 2й экран' : ''}`
  })

  // 2. Anti-Spoofing
  const spoof = props.spoofAnalysis
  list.push({
    name: 'Лицо',
    icon: 'i-lucide-scan-face',
    status: !spoof
      ? 'inactive'
      : spoof.isSpoof
        ? 'critical'
        : spoof.confidence > 0.3
          ? 'warning'
          : 'ok',
    detail: !spoof
      ? 'Ожидание'
      : spoof.isSpoof
        ? `Обнаружен ${spoof.spoofType}`
        : 'Живое лицо'
  })

  // 3. Virtual Camera
  const vc = props.virtualCameraReport
  list.push({
    name: 'Камера',
    icon: 'i-lucide-camera',
    status: !vc
      ? 'inactive'
      : vc.isVirtual
        ? 'critical'
        : 'ok',
    detail: !vc
      ? 'Проверка...'
      : vc.isVirtual
        ? `Виртуальная (${(vc.confidence * 100).toFixed(0)}%)`
        : 'Физическая'
  })

  // 4. Device Fingerprint
  const df = props.deviceFingerprint
  list.push({
    name: 'Устройство',
    icon: 'i-lucide-fingerprint',
    status: !df
      ? 'inactive'
      : df.isVirtualMachine
        ? 'critical'
        : 'ok',
    detail: !df
      ? 'Сбор...'
      : df.isVirtualMachine
        ? 'Виртуальная машина'
        : `ID: ${df.deviceId.substring(0, 8)}`
  })

  // 5. Liveness
  list.push({
    name: 'Живость',
    icon: 'i-lucide-heart-pulse',
    status: props.currentChallenge
      ? 'warning'
      : props.livenessPassRate < 0.5
        ? 'critical'
        : props.livenessPassRate < 0.8
          ? 'warning'
          : 'ok',
    detail: props.currentChallenge
      ? 'Активная проверка'
      : `Пройдено ${(props.livenessPassRate * 100).toFixed(0)}%`
  })

  // 6. Watermark
  list.push({
    name: 'Водяной знак',
    icon: 'i-lucide-shield-check',
    status: props.watermarkActive ? 'ok' : 'inactive',
    detail: props.watermarkActive ? 'Активен' : 'Неактивен'
  })

  return list
})

// Overall status
const overallStatus = computed(() => {
  if (subsystems.value.some(s => s.status === 'critical')) return 'critical'
  if (subsystems.value.some(s => s.status === 'warning')) return 'warning'
  return 'ok'
})

const overallColor = computed(() => {
  switch (overallStatus.value) {
    case 'critical': return 'var(--argus-error)'
    case 'warning': return 'var(--argus-warning)'
    default: return 'var(--argus-success)'
  }
})

const criticalCount = computed(() => subsystems.value.filter(s => s.status === 'critical').length)
const warningCount = computed(() => subsystems.value.filter(s => s.status === 'warning').length)

function statusColor(status: SubsystemStatus['status']): string {
  switch (status) {
    case 'critical': return 'var(--argus-error)'
    case 'warning': return 'var(--argus-warning)'
    case 'ok': return 'var(--argus-success)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function statusBg(status: SubsystemStatus['status']): string {
  switch (status) {
    case 'critical': return errorBg(0.12)
    case 'warning': return warningBg(0.12)
    case 'ok': return successBg(0.12)
    default: return accentBg(0.08)
  }
}
</script>

<template>
  <div
    class="fixed bottom-4 right-4 z-40 transition-all duration-300"
    @mouseenter="expanded = true"
    @mouseleave="expanded = false"
  >
    <!-- Expanded Panel -->
    <Transition name="shield-expand">
      <div
        v-if="expanded"
        class="rounded-xl shadow-2xl border backdrop-blur-xl mb-2 overflow-hidden"
        :style="{
          background: isDark ? 'rgba(11, 15, 20, 0.95)' : 'rgba(255, 255, 255, 0.97)',
          borderColor: 'var(--argus-border)',
          width: '260px'
        }"
      >
        <!-- Header -->
        <div
          class="px-3 py-2 border-b"
          :style="{ borderColor: 'var(--argus-border)' }"
        >
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-shield"
              class="size-4"
              :style="{ color: overallColor }"
            />
            <span
              class="text-xs font-bold"
              style="color: var(--argus-text);"
            >Щит безопасности</span>
            <div class="flex-1" />
            <span
              v-if="criticalCount > 0"
              class="text-[9px] font-bold px-1.5 py-0.5 rounded"
              :style="{ background: errorBg(0.15), color: 'var(--argus-error)' }"
            >
              {{ criticalCount }} крит.
            </span>
            <span
              v-if="warningCount > 0"
              class="text-[9px] font-bold px-1.5 py-0.5 rounded"
              :style="{ background: warningBg(0.15), color: 'var(--argus-warning)' }"
            >
              {{ warningCount }} пред.
            </span>
          </div>
        </div>

        <!-- Subsystem List -->
        <div class="py-1">
          <div
            v-for="sub in subsystems"
            :key="sub.name"
            class="flex items-center gap-2 px-3 py-1.5"
          >
            <!-- Status dot -->
            <span
              class="size-1.5 rounded-full shrink-0"
              :style="{ background: statusColor(sub.status) }"
            />

            <!-- Icon -->
            <div
              class="flex items-center justify-center size-6 rounded shrink-0"
              :style="{ background: statusBg(sub.status) }"
            >
              <UIcon
                :name="sub.icon"
                class="size-3.5"
                :style="{ color: statusColor(sub.status) }"
              />
            </div>

            <!-- Info -->
            <div class="flex-1 min-w-0">
              <p
                class="text-[10px] font-semibold truncate"
                style="color: var(--argus-text);"
              >
                {{ sub.name }}
              </p>
              <p
                class="text-[9px] truncate"
                style="color: var(--argus-text-dimmed);"
              >
                {{ sub.detail }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- Compact Badge (always visible) -->
    <button
      class="flex items-center gap-1.5 px-3 py-2 rounded-xl shadow-lg border backdrop-blur-xl cursor-default transition-all"
      :style="{
        background: isDark ? 'rgba(11, 15, 20, 0.9)' : 'rgba(255, 255, 255, 0.95)',
        borderColor: overallColor
      }"
    >
      <span class="relative flex size-2.5">
        <span
          v-if="overallStatus === 'critical'"
          class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
          :style="{ background: overallColor }"
        />
        <span
          class="relative inline-flex size-2.5 rounded-full"
          :style="{ background: overallColor }"
        />
      </span>
      <UIcon
        name="i-lucide-shield"
        class="size-4"
        :style="{ color: overallColor }"
      />
      <span
        class="text-[10px] font-bold"
        :style="{ color: overallColor }"
      >
        {{ overallStatus === 'ok' ? 'Защита' : overallStatus === 'warning' ? 'Внимание' : 'Угроза' }}
      </span>
    </button>
  </div>
</template>

<style scoped>
.shield-expand-enter-active,
.shield-expand-leave-active {
  transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.shield-expand-enter-from,
.shield-expand-leave-to {
  opacity: 0;
  transform: translateY(8px) scale(0.95);
}
</style>
