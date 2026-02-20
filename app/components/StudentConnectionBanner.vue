<script setup lang="ts">
// =============================================================================
// StudentConnectionBanner — Non-interruptive connection indicator for students
// =============================================================================
//
// Lightweight component for the student exam view showing:
//   - Persistent status dot (green/amber/red) in the corner
//   - Expandable tooltip with connection details on hover
//   - Non-interruptive toast on tier change with reassuring message
//
// Design principles:
//   - Non-alarming: Green = good, amber = reduced quality, blue = offline
//   - Reassuring: Messages emphasize exam safety, not connection problems
//   - Minimal footprint: Small dot + toast, no modal or blocking overlay
//   - Auto-dismiss: Toasts disappear after 4 seconds
//   - Uses existing useResilience() composable for all data
//
// =============================================================================

import { ref, watch, computed, onBeforeUnmount } from 'vue'
import type { ResilienceTier } from '~/composables/useHealthGovernor'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface Props {
  /** Current resilience tier (A/B/C). */
  tier: ResilienceTier
  /** Health score (0-100). */
  healthScore: number
  /** Whether there are pending items in the offline queue. */
  hasPendingItems: boolean
  /** Total pending items count. */
  pendingCount: number
  /** Whether the system is effectively offline. */
  isOffline: boolean
}

const props = defineProps<Props>()

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

const showTooltip = ref(false)
const toastMessage = ref('')
const showToast = ref(false)

let toastTimer: ReturnType<typeof setTimeout> | null = null

// ---------------------------------------------------------------------------
// Tier Configuration
// ---------------------------------------------------------------------------

const tierDisplay = computed(() => {
  switch (props.tier) {
    case 'A':
      return {
        dotClass: 'bg-emerald-500',
        pulseClass: 'bg-emerald-400',
        label: 'Подключение стабильно',
        sublabel: 'Все данные передаются в реальном времени',
        ringClass: 'ring-emerald-500/30'
      }
    case 'B':
      return {
        dotClass: 'bg-amber-500',
        pulseClass: 'bg-amber-400',
        label: 'Подключение нестабильно',
        sublabel: 'Экзамен продолжается в облегчённом режиме',
        ringClass: 'ring-amber-500/30'
      }
    case 'C':
      return {
        dotClass: 'bg-sky-500',
        pulseClass: 'bg-sky-400',
        label: 'Автономный режим',
        sublabel: 'Данные сохраняются локально и будут отправлены при восстановлении связи',
        ringClass: 'ring-sky-500/30'
      }
  }
})

// ---------------------------------------------------------------------------
// Toast on tier change
// ---------------------------------------------------------------------------

watch(
  () => props.tier,
  (newTier, oldTier) => {
    if (!oldTier || newTier === oldTier) return

    if (newTier === 'B') {
      showToastMessage('Качество связи снизилось. Ваш экзамен продолжается безопасно.')
    } else if (newTier === 'C') {
      showToastMessage('Связь потеряна. Данные сохраняются — экзамен не прерывается.')
    } else if (newTier === 'A') {
      showToastMessage('Связь восстановлена.')
    }
  }
)

function showToastMessage(msg: string): void {
  toastMessage.value = msg
  showToast.value = true

  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    showToast.value = false
  }, 4000)
}

onBeforeUnmount(() => {
  if (toastTimer) clearTimeout(toastTimer)
})

// ---------------------------------------------------------------------------
// Pending label
// ---------------------------------------------------------------------------

const pendingLabel = computed(() => {
  if (props.pendingCount === 0) return ''
  return `${props.pendingCount} в очереди`
})
</script>

<template>
  <div class="fixed top-4 right-4 z-50 flex flex-col items-end gap-2">
    <!-- Status Dot (always visible) -->
    <div
      class="relative cursor-pointer"
      @mouseenter="showTooltip = true"
      @mouseleave="showTooltip = false"
    >
      <!-- Dot -->
      <div
        class="w-3 h-3 rounded-full ring-2 ring-offset-1 ring-offset-transparent transition-colors duration-300"
        :class="[tierDisplay.dotClass, tierDisplay.ringClass]"
      />
      <!-- Pulse animation (only for Tier A = healthy) -->
      <div
        v-if="tier === 'A' && !hasPendingItems"
        class="absolute inset-0 w-3 h-3 rounded-full animate-ping opacity-40"
        :class="tierDisplay.pulseClass"
      />

      <!-- Tooltip (on hover) -->
      <Transition name="tooltip">
        <div
          v-if="showTooltip"
          class="absolute top-full right-0 mt-2 w-56 p-3 rounded-lg shadow-lg border"
          style="background: var(--argus-bg-card); border-color: var(--argus-border);"
        >
          <div class="flex items-center gap-2 mb-1.5">
            <div class="w-2 h-2 rounded-full" :class="tierDisplay.dotClass" />
            <span class="text-xs font-semibold" style="color: var(--argus-text);">
              {{ tierDisplay.label }}
            </span>
          </div>
          <p class="text-[10px] leading-relaxed" style="color: var(--argus-text-dimmed);">
            {{ tierDisplay.sublabel }}
          </p>

          <!-- Queue info (when items pending) -->
          <div
            v-if="hasPendingItems"
            class="mt-2 pt-2 border-t flex items-center gap-1.5"
            style="border-color: var(--argus-border);"
          >
            <svg class="w-3 h-3 text-sky-400 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-2.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4" />
            </svg>
            <span class="text-[10px] text-sky-400 tabular-nums">{{ pendingLabel }}</span>
          </div>

          <!-- Health score -->
          <div
            v-if="tier !== 'A'"
            class="mt-1 text-[9px] tabular-nums"
            style="color: var(--argus-text-dimmed);"
          >
            Здоровье: {{ healthScore }}%
          </div>
        </div>
      </Transition>
    </div>

    <!-- Toast (tier change notification) -->
    <Transition name="toast">
      <div
        v-if="showToast"
        class="px-3 py-2 rounded-lg shadow-lg border text-xs max-w-64"
        style="background: var(--argus-bg-card); border-color: var(--argus-border); color: var(--argus-text);"
      >
        <div class="flex items-start gap-2">
          <div class="w-2 h-2 rounded-full mt-1 flex-shrink-0" :class="tierDisplay.dotClass" />
          <span>{{ toastMessage }}</span>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
/* Tooltip transition */
.tooltip-enter-active {
  transition: opacity 0.15s ease-out, transform 0.15s ease-out;
}
.tooltip-leave-active {
  transition: opacity 0.1s ease-in, transform 0.1s ease-in;
}
.tooltip-enter-from,
.tooltip-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

/* Toast transition */
.toast-enter-active {
  transition: opacity 0.2s ease-out, transform 0.2s ease-out;
}
.toast-leave-active {
  transition: opacity 0.3s ease-in, transform 0.3s ease-in;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(16px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(8px);
}
</style>
