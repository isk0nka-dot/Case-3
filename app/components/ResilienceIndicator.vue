<script setup lang="ts">
// =============================================================================
// ResilienceIndicator — Adaptive monitoring tier & offline queue status
// =============================================================================
//
// Non-panic UX component showing:
//   - Current resilience tier (A/B/C) with calm colour coding
//   - Offline queue counter when items are pending
//   - Connection loss banner (non-interruptive)
//   - Tier transition messages
//
// Design principles:
//   - No red error states — amber for degraded, calm blue for offline
//   - Messages are informational, not alarming
//   - Auto-dismiss recovery messages after 3 seconds
//   - Compact: fits in header/sidebar without disruption
//
// =============================================================================

import type { ResilienceTier } from '~/composables/useHealthGovernor'
import type { QueueStats } from '~/composables/useOfflineQueue'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface Props {
  /** Current resilience tier (A/B/C). */
  tier: ResilienceTier
  /** Whether the system is currently online. */
  isOnline: boolean
  /** Whether the system is effectively offline. */
  isEffectivelyOffline: boolean
  /** Connection/tier change message (Russian). */
  connectionMessage: string
  /** Offline queue statistics. */
  queueStats: QueueStats
  /** Whether there are pending items in the queue. */
  hasPendingItems: boolean
  /** Health score (0-100). */
  healthScore: number
}

const props = defineProps<Props>()

// ---------------------------------------------------------------------------
// Computed
// ---------------------------------------------------------------------------

/** Tier display configuration. */
const tierConfig = computed(() => {
  switch (props.tier) {
    case 'A':
      return {
        label: 'A',
        description: 'Оптимальный',
        dotColor: 'bg-emerald-500',
        textColor: 'text-emerald-400',
        borderColor: 'border-emerald-500/30',
        bgColor: 'bg-emerald-500/10'
      }
    case 'B':
      return {
        label: 'B',
        description: 'Облегченный',
        dotColor: 'bg-amber-500',
        textColor: 'text-amber-400',
        borderColor: 'border-amber-500/30',
        bgColor: 'bg-amber-500/10'
      }
    case 'C':
      return {
        label: 'C',
        description: 'Автономный',
        dotColor: 'bg-sky-500',
        textColor: 'text-sky-400',
        borderColor: 'border-sky-500/30',
        bgColor: 'bg-sky-500/10'
      }
  }
})

/** Format pending count for display. */
const pendingLabel = computed(() => {
  const total = props.queueStats.pendingEvents + props.queueStats.pendingSnapshots
  if (total === 0) return ''
  if (total === 1) return '1 событие'
  if (total >= 2 && total <= 4) return `${total} события`
  return `${total} событий`
})

/** Format queue size for display. */
const queueSizeLabel = computed(() => {
  const bytes = props.queueStats.totalSizeBytes
  if (bytes < 1024) return `${bytes} Б`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`
  return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`
})

/** Whether to show the offline/degraded banner. */
const showBanner = computed(() => {
  return props.connectionMessage !== '' || props.tier !== 'A'
})

/** Whether to show the queue counter. */
const showQueue = computed(() => {
  return props.hasPendingItems
})
</script>

<template>
  <div class="flex flex-col gap-1">
    <!-- Tier Badge + Queue Counter -->
    <div class="flex items-center gap-2">
      <!-- Tier Badge -->
      <div
        class="flex items-center gap-1.5 px-2 py-1 rounded-md border"
        :class="[tierConfig.bgColor, tierConfig.borderColor]"
      >
        <!-- Status dot -->
        <div class="relative">
          <div class="w-2 h-2 rounded-full" :class="tierConfig.dotColor" />
          <div
            v-if="tier === 'A'"
            class="absolute inset-0 w-2 h-2 rounded-full animate-ping opacity-50"
            :class="tierConfig.dotColor"
          />
        </div>

        <!-- Tier label -->
        <span class="text-[11px] font-semibold" :class="tierConfig.textColor">
          {{ tierConfig.label }}
        </span>
        <span class="text-[10px] text-[var(--argus-text-dimmed)]">
          {{ tierConfig.description }}
        </span>

        <!-- Health score (when degraded) -->
        <span
          v-if="tier !== 'A'"
          class="text-[10px] tabular-nums"
          :class="tierConfig.textColor"
        >
          {{ healthScore }}%
        </span>
      </div>

      <!-- Queue Counter (when items pending) -->
      <div
        v-if="showQueue"
        class="flex items-center gap-1 px-2 py-1 rounded-md bg-sky-500/10 border border-sky-500/30"
      >
        <!-- Queue icon (inbox) -->
        <svg class="w-3 h-3 text-sky-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-2.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
        </svg>
        <span class="text-[10px] text-sky-400 tabular-nums">
          {{ pendingLabel }}
        </span>
        <span class="text-[9px] text-sky-400/60 tabular-nums">
          ({{ queueSizeLabel }})
        </span>
      </div>
    </div>

    <!-- Connection Message Banner (non-interruptive) -->
    <div
      v-if="connectionMessage"
      class="flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px]"
      :class="isEffectivelyOffline
        ? 'bg-sky-500/10 border border-sky-500/20 text-sky-400'
        : 'bg-amber-500/10 border border-amber-500/20 text-amber-400'"
    >
      <!-- Offline icon -->
      <svg
        v-if="isEffectivelyOffline"
        class="w-3.5 h-3.5 flex-shrink-0"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M18.364 5.636a9 9 0 010 12.728M5.636 18.364a9 9 0 010-12.728M8.464 15.536a5 5 0 010-7.072M15.536 8.464a5 5 0 010 7.072" />
        <line x1="4" y1="4" x2="20" y2="20" stroke-linecap="round" />
      </svg>
      <!-- Warning icon -->
      <svg
        v-else
        class="w-3.5 h-3.5 flex-shrink-0"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z" />
      </svg>
      <span>{{ connectionMessage }}</span>
    </div>
  </div>
</template>
