<script setup lang="ts">
// =============================================================================
// CriticalAlertsBanner — Persistent critical alert notification
// =============================================================================
//
// Displays a fixed banner at the top of the dashboard when there are
// unacknowledged critical alerts. Features:
//   - Pulsing red glow for visual urgency
//   - Alert count badge
//   - Click to expand/collapse alert list
//   - Individual and bulk acknowledgment
//   - Auto-dismiss after 30 minutes
//
// =============================================================================

import { useProctoringAlerts, type CriticalAlert } from '~/composables/useProctoringAlerts'
import { Severity } from '~/lib/proto/types'

const {
  alerts,
  unreadCount,
  hasCritical,
  recentAlerts,
  acknowledgeAlert,
  acknowledgeAll
} = useProctoringAlerts({
  minSeverity: Severity.WARNING,
  maxAlerts: 200,
  enableSound: true
})

const isExpanded = ref(false)

function getAlertIcon(alert: CriticalAlert): string {
  const category = alert.eventTypeLabel.toLowerCase()
  if (category.includes('лицо') || category.includes('взгляд')) return 'i-lucide-eye'
  if (category.includes('телефон') || category.includes('объект')) return 'i-lucide-smartphone'
  if (category.includes('доступ') || category.includes('удалён')) return 'i-lucide-shield-alert'
  if (category.includes('виртуал')) return 'i-lucide-monitor'
  if (category.includes('vpn') || category.includes('прокси')) return 'i-lucide-wifi-off'
  if (category.includes('вкладк')) return 'i-lucide-external-link'
  if (category.includes('процесс')) return 'i-lucide-terminal'
  return 'i-lucide-alert-triangle'
}
</script>

<template>
  <!-- Only show when there are unread alerts -->
  <Transition
    enter-active-class="transition-all duration-300 ease-out"
    enter-from-class="opacity-0 -translate-y-full"
    enter-to-class="opacity-100 translate-y-0"
    leave-active-class="transition-all duration-200 ease-in"
    leave-from-class="opacity-100 translate-y-0"
    leave-to-class="opacity-0 -translate-y-full"
  >
    <div
      v-if="unreadCount > 0"
      class="relative z-50"
    >
      <!-- Alert bar -->
      <div
        class="flex items-center gap-3 px-4 py-2 cursor-pointer transition-colors"
        :class="hasCritical
          ? 'bg-red-500/15 border-b border-red-500/30 hover:bg-red-500/20'
          : 'bg-amber-500/10 border-b border-amber-500/20 hover:bg-amber-500/15'"
        @click="isExpanded = !isExpanded"
      >
        <!-- Pulsing icon -->
        <div class="relative flex-shrink-0">
          <UIcon
            name="i-lucide-alert-triangle"
            class="w-5 h-5"
            :class="hasCritical ? 'text-red-500' : 'text-amber-500'"
          />
          <span
            v-if="hasCritical"
            class="absolute -top-0.5 -right-0.5 w-2 h-2 rounded-full bg-red-500 animate-ping"
          />
        </div>

        <!-- Alert message -->
        <div class="flex-1 min-w-0">
          <span class="text-sm font-medium" :class="hasCritical ? 'text-red-400' : 'text-amber-400'">
            {{ unreadCount }} {{ unreadCount === 1 ? 'новое оповещение' : 'новых оповещений' }}
          </span>
          <span
            v-if="recentAlerts[0]"
            class="text-xs ml-2"
            :class="hasCritical ? 'text-red-400/60' : 'text-amber-400/60'"
          >
            — {{ recentAlerts[0].eventTypeLabel }}
          </span>
        </div>

        <!-- Actions -->
        <div class="flex items-center gap-2 flex-shrink-0">
          <button
            class="text-xs px-2 py-1 rounded transition-colors"
            :class="hasCritical
              ? 'text-red-400/80 hover:text-red-300 hover:bg-red-500/20'
              : 'text-amber-400/80 hover:text-amber-300 hover:bg-amber-500/20'"
            @click.stop="acknowledgeAll()"
          >
            Прочитать все
          </button>

          <UIcon
            :name="isExpanded ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'"
            class="w-4 h-4"
            :class="hasCritical ? 'text-red-400/60' : 'text-amber-400/60'"
          />
        </div>
      </div>

      <!-- Expanded alert list -->
      <Transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="opacity-0 max-h-0"
        enter-to-class="opacity-100 max-h-[300px]"
        leave-active-class="transition-all duration-150 ease-in"
        leave-from-class="opacity-100 max-h-[300px]"
        leave-to-class="opacity-0 max-h-0"
      >
        <div
          v-if="isExpanded"
          class="overflow-y-auto max-h-[300px] border-b"
          :class="hasCritical ? 'bg-red-500/5 border-red-500/20' : 'bg-amber-500/5 border-amber-500/15'"
        >
          <div
            v-for="alert in recentAlerts"
            :key="alert.id"
            class="flex items-center gap-3 px-4 py-2 border-b last:border-0 transition-colors"
            :class="[
              alert.acknowledged
                ? 'opacity-50 border-[var(--argus-border-subtle)]'
                : alert.severity === 3
                  ? 'border-red-500/10 hover:bg-red-500/10'
                  : 'border-amber-500/10 hover:bg-amber-500/10'
            ]"
          >
            <!-- Icon -->
            <UIcon
              :name="getAlertIcon(alert)"
              class="w-4 h-4 flex-shrink-0"
              :class="alert.severity === 3 ? 'text-red-400' : 'text-amber-400'"
            />

            <!-- Content -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="text-xs font-medium" :class="alert.severity === 3 ? 'text-red-400' : 'text-amber-400'">
                  {{ alert.eventTypeLabel }}
                </span>
                <span class="text-[10px] text-[var(--argus-text-dimmed)] tabular-nums">
                  {{ formatTime(alert.timestamp) }}
                </span>
              </div>
              <p class="text-[11px] text-[var(--argus-text-muted)] truncate">
                {{ alert.label || 'Нет описания' }}
              </p>
            </div>

            <!-- Confidence -->
            <span class="text-[10px] tabular-nums text-[var(--argus-text-dimmed)] flex-shrink-0">
              {{ (alert.confidence * 100).toFixed(0) }}%
            </span>

            <!-- Acknowledge button -->
            <button
              v-if="!alert.acknowledged"
              class="p-1 rounded transition-colors"
              :class="alert.severity === 3 ? 'hover:bg-red-500/20 text-red-400/60 hover:text-red-400' : 'hover:bg-amber-500/20 text-amber-400/60 hover:text-amber-400'"
              title="Прочитано"
              @click.stop="acknowledgeAlert(alert.id)"
            >
              <UIcon name="i-lucide-check" class="w-3.5 h-3.5" />
            </button>
            <UIcon
              v-else
              name="i-lucide-check-check"
              class="w-3.5 h-3.5 text-green-500/40 flex-shrink-0"
            />
          </div>
        </div>
      </Transition>
    </div>
  </Transition>
</template>
