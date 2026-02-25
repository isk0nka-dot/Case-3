<script setup lang="ts">
// =============================================================================
// Argus AI — Performance Debugger Panel
// =============================================================================
//
// Hidden overlay panel activated via Ctrl+Shift+D. Shows real-time metrics
// from the health governor, vision engine, resilience layer, and session
// event throughput. Glassmorphic styling consistent with the Argus design.
//
// Data flows through the debug bridge (reactive module-scoped refs) so this
// component can live at app root while composables exist in child pages.
// =============================================================================

import { useColors } from '~/composables/useColors'
import {
  debugHealthGovernor,
  debugVisionEngine,
  debugResilience,
  debugSession
} from '~/composables/useDebugBridge'

const emit = defineEmits<{
  close: []
}>()

const { accentBg, errorBg, successBg, warningBg, isDark } = useColors()

// ---------------------------------------------------------------------------
// Session availability
// ---------------------------------------------------------------------------

const hasSession = computed(() => !!debugSession.value)
const hasResilience = computed(() => !!debugResilience.value)
const hasVision = computed(() => !!debugVisionEngine.value)
const hasGovernor = computed(() => !!debugHealthGovernor.value)

// ---------------------------------------------------------------------------
// Tier History Tracking
// ---------------------------------------------------------------------------

interface TierEntry {
  tier: string
  timestamp: number
}

const tierHistory = ref<TierEntry[]>([])
const lastRecordedTier = ref<string | null>(null)

// ---------------------------------------------------------------------------
// Refresh loop — 1 Hz for non-critical data
// ---------------------------------------------------------------------------

let refreshTimer: ReturnType<typeof setInterval> | null = null

const tick = ref(0) // reactive trigger for template re-evaluation

onMounted(() => {
  refreshTimer = setInterval(() => {
    tick.value++

    // Track tier transitions
    if (hasGovernor.value) {
      const currentTier = debugHealthGovernor.value!.currentTier.value
      if (currentTier !== lastRecordedTier.value) {
        tierHistory.value.push({ tier: currentTier, timestamp: Date.now() })
        if (tierHistory.value.length > 50) tierHistory.value.shift()
        lastRecordedTier.value = currentTier
      }
    }
  }, 1000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatMs(ms: number): string {
  if (ms < 1) return '<1ms'
  if (ms > 10000) return `${(ms / 1000).toFixed(1)}s`
  return `${Math.round(ms)}ms`
}

function formatDuration(startMs: number): string {
  const elapsed = Date.now() - startMs
  const sec = Math.floor(elapsed / 1000)
  const min = Math.floor(sec / 60)
  const hr = Math.floor(min / 60)
  if (hr > 0) return `${hr}h ${min % 60}m`
  if (min > 0) return `${min}m ${sec % 60}s`
  return `${sec}s`
}

function tierColor(tier: string): string {
  switch (tier) {
    case 'A': return 'var(--argus-success)'
    case 'B': return 'var(--argus-warning)'
    case 'C': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function scoreColor(score: number): string {
  if (score >= 70) return 'var(--argus-success)'
  if (score >= 40) return 'var(--argus-warning)'
  return 'var(--argus-error)'
}

function stateColor(state: string): string {
  switch (state) {
    case 'active': return 'var(--argus-success)'
    case 'starting':
    case 'stopping': return 'var(--argus-warning)'
    default: return 'var(--argus-text-dimmed)'
  }
}

// Last tier entry duration
const currentTierDuration = computed(() => {
  if (tierHistory.value.length === 0) return 'N/A'
  const last = tierHistory.value[tierHistory.value.length - 1]!
  return formatDuration(last.timestamp)
})

// Force template to read tick so it re-renders
const _tick = computed(() => tick.value)
</script>

<template>
  <Teleport to="body">
    <div
      class="fixed bottom-4 right-4 z-[9999] w-[400px] max-h-[80vh] overflow-y-auto rounded-2xl border shadow-2xl"
      :style="{
        background: isDark ? 'rgba(15, 23, 42, 0.92)' : 'rgba(255, 255, 255, 0.92)',
        backdropFilter: 'blur(12px)',
        borderColor: 'var(--argus-border)',
        color: 'var(--argus-text)'
      }"
    >
      <!-- Header -->
      <div
        class="flex items-center justify-between px-4 py-3 border-b sticky top-0 z-10"
        :style="{
          borderColor: 'var(--argus-border)',
          background: isDark ? 'rgba(15, 23, 42, 0.95)' : 'rgba(255, 255, 255, 0.95)'
        }"
      >
        <div class="flex items-center gap-2">
          <UIcon name="i-lucide-activity" class="size-4" style="color: var(--argus-accent);" />
          <span class="text-sm font-bold tracking-tight">Performance Debugger</span>
          <span class="text-[10px] font-mono px-1.5 py-0.5 rounded" :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }">
            LIVE
          </span>
        </div>
        <button
          class="flex items-center justify-center size-6 rounded-md transition-colors cursor-pointer"
          style="color: var(--argus-text-dimmed);"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          @click="emit('close')"
        >
          <UIcon name="i-lucide-x" class="size-4" />
        </button>
      </div>

      <!-- No Session State -->
      <div v-if="!hasSession" class="px-4 py-8 text-center">
        <UIcon name="i-lucide-monitor-off" class="size-10 mx-auto mb-3" style="color: var(--argus-text-muted);" />
        <p class="text-sm font-medium" style="color: var(--argus-text-dimmed);">No Active Proctoring Session</p>
        <p class="text-xs mt-1" style="color: var(--argus-text-muted);">Start a session to see performance metrics.</p>
      </div>

      <!-- Metrics Sections -->
      <div v-else class="divide-y" :style="{ borderColor: 'var(--argus-border)' }" :data-tick="_tick">
        <!-- Section 1: Health Governor -->
        <div v-if="hasGovernor" class="px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <UIcon name="i-lucide-heart-pulse" class="size-3.5" style="color: var(--argus-accent);" />
            <span class="text-[11px] font-bold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
              Health Governor
            </span>
          </div>

          <!-- Score bar -->
          <div class="flex items-center gap-3 mb-2">
            <div class="flex-1 h-2 rounded-full overflow-hidden" :style="{ background: isDark ? 'rgba(255,255,255,0.05)' : 'rgba(0,0,0,0.05)' }">
              <div
                class="h-full rounded-full transition-all duration-500"
                :style="{
                  width: `${debugHealthGovernor!.healthScore.value}%`,
                  background: scoreColor(debugHealthGovernor!.healthScore.value)
                }"
              />
            </div>
            <span
              class="text-sm font-bold font-mono w-12 text-right"
              :style="{ color: scoreColor(debugHealthGovernor!.healthScore.value) }"
            >
              {{ debugHealthGovernor!.healthScore.value }}
            </span>
            <span
              class="text-xs font-bold px-1.5 py-0.5 rounded"
              :style="{
                color: tierColor(debugHealthGovernor!.currentTier.value),
                background: `${tierColor(debugHealthGovernor!.currentTier.value)}15`
              }"
            >
              Tier {{ debugHealthGovernor!.currentTier.value }}
            </span>
          </div>

          <!-- Metric rows -->
          <div class="grid grid-cols-3 gap-x-3 gap-y-1 text-xs font-mono">
            <div>
              <span style="color: var(--argus-text-muted);">FPS</span>
              <span class="ml-1 font-semibold">{{ debugHealthGovernor!.metrics.value.fps }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">RTT</span>
              <span class="ml-1 font-semibold">{{ formatMs(debugHealthGovernor!.metrics.value.rttMs) }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Loss</span>
              <span class="ml-1 font-semibold">{{ (debugHealthGovernor!.metrics.value.packetLoss * 100).toFixed(1) }}%</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">CPU</span>
              <span class="ml-1 font-semibold">{{ (debugHealthGovernor!.metrics.value.cpuPressure * 100).toFixed(0) }}%</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Inf.</span>
              <span class="ml-1 font-semibold">{{ formatMs(debugHealthGovernor!.inferenceLatencyMs.value) }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">BW</span>
              <span class="ml-1 font-semibold">{{ Math.round(debugHealthGovernor!.metrics.value.bandwidthKbps) }}k</span>
            </div>
          </div>

          <!-- Hysteresis -->
          <div class="flex gap-4 mt-1.5 text-[10px] font-mono" style="color: var(--argus-text-muted);">
            <span>Consecutive ↓ {{ debugHealthGovernor!.consecutiveDowngradeSamples.value }}</span>
            <span>Consecutive ↑ {{ debugHealthGovernor!.consecutiveUpgradeSamples.value }}</span>
          </div>
        </div>

        <!-- Section 2: Vision Engine -->
        <div v-if="hasVision" class="px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <UIcon name="i-lucide-eye" class="size-3.5" style="color: var(--argus-brand-purple, var(--argus-accent));" />
            <span class="text-[11px] font-bold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
              Vision Engine
            </span>
          </div>

          <div class="grid grid-cols-3 gap-x-3 gap-y-1 text-xs font-mono">
            <div>
              <span style="color: var(--argus-text-muted);">Model</span>
              <span class="ml-1 font-semibold" :style="{ color: debugVisionEngine!.isModelLoaded.value ? 'var(--argus-success)' : 'var(--argus-error)' }">
                {{ debugVisionEngine!.isModelLoaded.value ? '✓' : '✗' }}
              </span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">FPS</span>
              <span class="ml-1 font-semibold">{{ debugVisionEngine!.fps.value }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Faces</span>
              <span class="ml-1 font-semibold">{{ debugVisionEngine!.currentFrame.value?.faceCount ?? 0 }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Avg Inf.</span>
              <span class="ml-1 font-semibold">{{ formatMs(debugVisionEngine!.stats.value.avgInferenceMs) }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">No-face</span>
              <span class="ml-1 font-semibold">{{ debugVisionEngine!.consecutiveNoFace.value }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Frames</span>
              <span class="ml-1 font-semibold">{{ debugVisionEngine!.stats.value.totalFrames }}</span>
            </div>
          </div>

          <!-- Vision stats detail -->
          <div class="flex gap-4 mt-1.5 text-[10px] font-mono" style="color: var(--argus-text-muted);">
            <span>Gaze dev: {{ debugVisionEngine!.stats.value.totalGazeDeviations }}</span>
            <span>Head anom: {{ debugVisionEngine!.stats.value.totalHeadPoseAnomalies }}</span>
            <span>Blinks: {{ debugVisionEngine!.stats.value.totalBlinks }}</span>
          </div>
        </div>

        <!-- Section 3: Tier History -->
        <div v-if="hasGovernor" class="px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <UIcon name="i-lucide-git-branch" class="size-3.5" style="color: var(--argus-warning);" />
            <span class="text-[11px] font-bold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
              Tier History
            </span>
          </div>

          <div v-if="tierHistory.length === 0" class="text-xs font-mono" style="color: var(--argus-text-muted);">
            No transitions recorded yet
          </div>
          <div v-else class="space-y-1">
            <div
              v-for="(entry, i) in tierHistory.slice(-5).reverse()"
              :key="i"
              class="flex items-center gap-2 text-xs font-mono"
            >
              <span
                class="w-5 text-center font-bold"
                :style="{ color: tierColor(entry.tier) }"
              >
                {{ entry.tier }}
              </span>
              <span style="color: var(--argus-text-muted);">
                {{ new Date(entry.timestamp).toLocaleTimeString() }}
              </span>
              <span v-if="i === 0" class="text-[10px]" style="color: var(--argus-text-dimmed);">
                ({{ currentTierDuration }})
              </span>
            </div>
          </div>
        </div>

        <!-- Section 4: Connection & Queue -->
        <div v-if="hasResilience" class="px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <UIcon name="i-lucide-wifi" class="size-3.5" style="color: var(--argus-success);" />
            <span class="text-[11px] font-bold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
              Connection & Queue
            </span>
          </div>

          <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-xs font-mono">
            <div>
              <span style="color: var(--argus-text-muted);">Status</span>
              <span
                class="ml-1 font-semibold"
                :style="{ color: debugResilience!.isOnline.value ? 'var(--argus-success)' : 'var(--argus-error)' }"
              >
                {{ debugResilience!.isOnline.value ? '● Online' : '● Offline' }}
              </span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Clock Δ</span>
              <span class="ml-1 font-semibold">{{ debugResilience!.clockOffsetMs.value > 0 ? '+' : '' }}{{ debugResilience!.clockOffsetMs.value }}ms</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Events Q</span>
              <span class="ml-1 font-semibold">{{ debugResilience!.queueStats.value.pendingEvents ?? 0 }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Snaps Q</span>
              <span class="ml-1 font-semibold">{{ debugResilience!.queueStats.value.pendingSnapshots ?? 0 }}</span>
            </div>
          </div>

          <!-- Connection message -->
          <div
            v-if="debugResilience!.connectionMessage.value"
            class="mt-1.5 text-[10px] font-mono px-2 py-1 rounded"
            :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
          >
            {{ debugResilience!.connectionMessage.value }}
          </div>

          <!-- Server directive -->
          <div class="flex gap-4 mt-1.5 text-[10px] font-mono" style="color: var(--argus-text-muted);">
            <span>Directive: {{ debugResilience!.serverDirective.value?.telemetryMode ?? 'NORMAL' }}</span>
            <span v-if="debugResilience!.isDegraded.value" style="color: var(--argus-warning);">DEGRADED</span>
          </div>
        </div>

        <!-- Section 5: Session Throughput -->
        <div v-if="hasSession" class="px-4 py-3">
          <div class="flex items-center gap-2 mb-2">
            <UIcon name="i-lucide-send" class="size-3.5" style="color: var(--argus-accent);" />
            <span class="text-[11px] font-bold uppercase tracking-wider" style="color: var(--argus-text-dimmed);">
              Event Throughput
            </span>
          </div>

          <div class="grid grid-cols-3 gap-x-3 gap-y-1 text-xs font-mono">
            <div>
              <span style="color: var(--argus-text-muted);">Status</span>
              <span
                class="ml-1 font-semibold"
                :style="{ color: stateColor(debugSession!.status.value) }"
              >
                {{ debugSession!.status.value }}
              </span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Sent</span>
              <span class="ml-1 font-semibold">{{ debugSession!.metrics.value.totalEventsSent }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Dropped</span>
              <span
                class="ml-1 font-semibold"
                :style="{ color: debugSession!.metrics.value.totalEventsDropped > 0 ? 'var(--argus-error)' : 'var(--argus-text)' }"
              >
                {{ debugSession!.metrics.value.totalEventsDropped }}
              </span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Viols</span>
              <span class="ml-1 font-semibold">{{ debugSession!.metrics.value.violationCount }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Evt/s</span>
              <span class="ml-1 font-semibold">{{ debugSession!.metrics.value.eventsPerSecond }}</span>
            </div>
            <div>
              <span style="color: var(--argus-text-muted);">Focus</span>
              <span class="ml-1 font-semibold">{{ (debugSession!.metrics.value.currentFocusScore * 100).toFixed(0) }}%</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div
        class="px-4 py-2 border-t text-center"
        :style="{ borderColor: 'var(--argus-border)' }"
      >
        <span class="text-[10px] font-mono" style="color: var(--argus-text-muted);">
          Ctrl+Shift+D to toggle • 1Hz refresh
        </span>
      </div>
    </div>
  </Teleport>
</template>
