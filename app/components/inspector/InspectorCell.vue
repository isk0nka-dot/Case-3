<script setup lang="ts">
// =============================================================================
// InspectorCell — Single session card in the Inspector Grid
// =============================================================================
// Renders VideoPlayer + RiskGauge + RiskSparkline + evidence capture button
// + violation counts + LIVE badge per session. Optimized for 25+ simultaneous
// instances on screen.
// =============================================================================

import type { ActiveSession } from '~/composables/useAdminAPI'
import type { RiskScore } from '~/stores/useInspectorStore'
import { useInspectorStore } from '~/stores/useInspectorStore'
import { useEvidenceCapture } from '~/composables/useEvidenceCapture'

const props = defineProps<{
  session: ActiveSession
  riskScore: RiskScore
  isTop3: boolean
  isSelected: boolean
  compact: boolean
}>()

const emit = defineEmits<{
  (e: 'focus', sessionId: string): void
  (e: 'filter', sessionId: string): void
}>()

const inspectorStore = useInspectorStore()
const { captureEvidence } = useEvidenceCapture()
const { isDark } = useColors()
const { errorBg, warningBg } = useColors()

const playerRef = ref<{ videoRef: HTMLVideoElement | null } | null>(null)
const hovered = ref(false)
const capturing = ref(false)

// Evidence count for badge
const evidenceCount = computed(() => inspectorStore.getEvidenceCount(props.session.sessionId))

// Risk color for sparkline
const riskColor = computed(() => {
  if (props.riskScore.composite >= 60) return 'var(--argus-error)'
  if (props.riskScore.composite >= 30) return 'var(--argus-warning)'
  return 'var(--argus-success)'
})

// Border style
const borderStyle = computed(() => {
  if (props.isTop3 && props.riskScore.composite >= 60) {
    return { borderColor: 'var(--argus-error)', borderWidth: '2px' }
  }
  if (props.session.violationLevel === 'critical') {
    return { borderColor: 'var(--argus-error)', borderWidth: '1px' }
  }
  if (props.session.violationLevel === 'warning') {
    return { borderColor: 'var(--argus-warning)', borderWidth: '1px' }
  }
  return { borderColor: 'var(--argus-border)', borderWidth: '1px' }
})

const badgeBg = computed(() => isDark.value ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)')

// Evidence capture handler
async function handleCapture() {
  if (capturing.value) return
  capturing.value = true
  try {
    const video = playerRef.value?.videoRef ?? null
    await captureEvidence(video, props.session.sessionId)
  } finally {
    capturing.value = false
  }
}

function handleClick() {
  emit('filter', props.session.sessionId)
}

function handleDblClick() {
  emit('focus', props.session.sessionId)
}

function formatStudentName(studentId: string): string {
  const match = studentId.match(/student-(\d+)/)
  if (match) return `Студент #${match[1]}`
  return studentId
}
</script>

<template>
  <div
    class="glass-card rounded-xl overflow-hidden video-card-hover transition-all duration-300 relative"
    :class="[
      isTop3 && riskScore.composite >= 60 ? 'pulse-critical' : '',
      session.violationLevel === 'warning' && !isTop3 ? 'pulse-warning' : '',
      isSelected ? 'ring-2 ring-[var(--argus-accent)]' : ''
    ]"
    :style="borderStyle"
    @click="handleClick"
    @dblclick="handleDblClick"
    @mouseenter="hovered = true"
    @mouseleave="hovered = false"
  >
    <!-- Video Feed -->
    <div class="relative aspect-video overflow-hidden" style="background: var(--argus-bg-deep);">
      <VideoPlayer
        ref="playerRef"
        :session-id="session.sessionId"
        :compact="true"
      />

      <!-- LIVE Badge (top-left) -->
      <div class="absolute top-1.5 left-1.5 z-10">
        <div
          v-if="session.status === 'active'"
          class="flex items-center gap-1 px-1.5 py-0.5 rounded-md text-[8px] font-bold uppercase"
          :style="{ background: badgeBg }"
        >
          <span class="relative flex size-1.5">
            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
            <span class="relative inline-flex size-1.5 rounded-full bg-red-500" />
          </span>
          <span style="color: var(--argus-error);">LIVE</span>
        </div>
        <div
          v-else
          class="flex items-center gap-1 px-1.5 py-0.5 rounded-md text-[8px] font-bold"
          :style="{ background: badgeBg, color: 'var(--argus-text-dimmed)' }"
        >
          ЗАВЕРШЕНА
        </div>
      </div>

      <!-- Risk Gauge (top-right) -->
      <div class="absolute top-1.5 right-1.5 z-10">
        <div class="rounded-md p-0.5" :style="{ background: badgeBg }">
          <RiskGauge :score="riskScore.composite" :size="compact ? 28 : 36" />
        </div>
      </div>

      <!-- Violation Counts (bottom-left) -->
      <div class="absolute bottom-1.5 left-1.5 flex items-center gap-1 z-10">
        <div
          v-if="session.criticalCount > 0"
          class="flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[8px] font-bold"
          :style="{ background: badgeBg, color: 'var(--argus-error)' }"
        >
          <UIcon name="i-lucide-shield-alert" class="size-2.5" />
          {{ session.criticalCount }}
        </div>
        <div
          v-if="session.warningCount > 0"
          class="flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[8px] font-bold"
          :style="{ background: badgeBg, color: 'var(--argus-warning)' }"
        >
          <UIcon name="i-lucide-alert-triangle" class="size-2.5" />
          {{ session.warningCount }}
        </div>
      </div>

      <!-- Evidence Capture Button (bottom-right, on hover) -->
      <Transition name="fade">
        <button
          v-if="hovered && session.status === 'active'"
          class="absolute bottom-1.5 right-1.5 z-10 flex items-center gap-1 px-2 py-1 rounded-md text-[9px] font-bold transition-all cursor-pointer"
          :style="{ background: badgeBg, color: 'var(--argus-accent)' }"
          :disabled="capturing"
          @click.stop="handleCapture"
        >
          <UIcon v-if="!capturing" name="i-lucide-camera" class="size-3" />
          <span v-else class="size-3 animate-spin rounded-full border border-t-transparent" style="border-color: var(--argus-accent);" />
          <template v-if="evidenceCount > 0">
            {{ evidenceCount }}
          </template>
        </button>
      </Transition>
    </div>

    <!-- Card Footer (hidden in compact mode) -->
    <div v-if="!compact" class="px-2.5 py-2 space-y-1.5">
      <div class="flex items-center justify-between gap-1.5">
        <div class="min-w-0 flex-1">
          <p class="text-xs font-semibold truncate" style="color: var(--argus-text);">
            {{ formatStudentName(session.studentId) }}
          </p>
          <p class="text-[9px] mt-0.5 truncate" style="color: var(--argus-text-dimmed);">
            {{ session.sessionId.substring(0, 12) }}...
          </p>
        </div>
        <span
          v-if="session.criticalCount + session.warningCount > 0"
          class="text-[9px] font-bold px-1.5 py-0.5 rounded-full shrink-0"
          :style="{
            background: session.violationLevel === 'critical' ? errorBg(0.1) : warningBg(0.1),
            color: session.violationLevel === 'critical' ? 'var(--argus-error)' : 'var(--argus-warning)'
          }"
        >
          {{ session.criticalCount + session.warningCount }} нар.
        </span>
      </div>

      <!-- Sparkline -->
      <RiskSparkline
        v-if="riskScore.trend.length >= 2"
        :data="riskScore.trend"
        :color="riskColor"
        :width="compact ? 80 : 120"
        :height="16"
      />
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.15s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
