<script setup lang="ts">
// =============================================================================
// LiveEventFeed — Real-time proctoring event feed component
// =============================================================================
//
// Displays a high-velocity stream of proctoring events with:
//   - Severity-coded badges (critical/warning/info)
//   - Category-based icons (video, audio, browser, kernel)
//   - Smooth entry animations (no jank at 10K events/sec)
//   - Auto-scroll to latest events
//   - Click-to-inspect event details
//
// Performance:
//   The component renders from the useEventFeedStore which updates at 4Hz.
//   Only 50 events are rendered at a time (virtualized via displayLimit).
//   Each event row is a frozen object — Vue skips deep reactivity tracking.
//
// =============================================================================

import { useEventFeedStore, type FeedEvent } from '~/stores/useEventFeedStore'
import { Severity } from '~/lib/proto/types'

const feedStore = useEventFeedStore()

// Start feed updates when component mounts.
onMounted(() => {
  feedStore.startUpdates()
})

onUnmounted(() => {
  feedStore.stopUpdates()
})

// Category icons mapping.
const categoryIcons: Record<string, string> = {
  video: 'i-lucide-video',
  object: 'i-lucide-search',
  audio: 'i-lucide-volume-2',
  browser: 'i-lucide-globe',
  network: 'i-lucide-wifi',
  psychometry: 'i-lucide-brain',
  kernel: 'i-lucide-cpu',
  telemetry: 'i-lucide-activity',
  unknown: 'i-lucide-help-circle'
}

// Severity styling.
function getSeverityClasses(severity: Severity): string {
  switch (severity) {
    case Severity.CRITICAL:
      return 'bg-red-500/20 text-red-400 border-red-500/30'
    case Severity.WARNING:
      return 'bg-amber-500/20 text-amber-400 border-amber-500/30'
    case Severity.INFO:
      return 'bg-blue-500/20 text-blue-400 border-blue-500/30'
    default:
      return 'bg-gray-500/20 text-gray-400 border-gray-500/30'
  }
}

function getSeverityDot(severity: Severity): string {
  switch (severity) {
    case Severity.CRITICAL: return 'bg-red-500'
    case Severity.WARNING: return 'bg-amber-500'
    case Severity.INFO: return 'bg-blue-500'
    default: return 'bg-gray-500'
  }
}

// formatTimeAgoMs → centralized in useFormatters() composable
const { formatTimeAgoMs } = useFormatters()

// Selected event for detail view.
const selectedEvent = ref<FeedEvent | null>(null)
const isEventModalOpen = computed({
  get: () => !!selectedEvent.value,
  set: (val: boolean) => { if (!val) selectedEvent.value = null }
})
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Header with stats -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-[var(--argus-border)]">
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
        <h3 class="text-sm font-semibold text-[var(--argus-text)]">
          Поток событий
        </h3>
        <span class="text-xs text-[var(--argus-text-dimmed)] tabular-nums">
          {{ feedStore.stats.eventsPerSecond }}/сек
        </span>
      </div>

      <div class="flex items-center gap-3 text-xs">
        <span class="flex items-center gap-1 text-red-400">
          <span class="w-1.5 h-1.5 rounded-full bg-red-500" />
          {{ feedStore.stats.criticalCount }}
        </span>
        <span class="flex items-center gap-1 text-amber-400">
          <span class="w-1.5 h-1.5 rounded-full bg-amber-500" />
          {{ feedStore.stats.warningCount }}
        </span>
        <span class="flex items-center gap-1 text-blue-400">
          <span class="w-1.5 h-1.5 rounded-full bg-blue-500" />
          {{ feedStore.stats.infoCount }}
        </span>
      </div>
    </div>

    <!-- Critical alert banner -->
    <div
      v-if="feedStore.lastCriticalEvent"
      class="px-4 py-2 bg-red-500/10 border-b border-red-500/20 flex items-center gap-2"
    >
      <UIcon name="i-lucide-alert-triangle" class="text-red-500 w-4 h-4 flex-shrink-0" />
      <span class="text-xs text-red-400 truncate">
        {{ feedStore.lastCriticalEvent.eventTypeLabel }} — {{ feedStore.lastCriticalEvent.label }}
      </span>
      <span class="text-xs text-red-500/60 ml-auto flex-shrink-0 tabular-nums">
        {{ formatTimeAgoMs(feedStore.lastCriticalEvent.receivedAt) }}
      </span>
    </div>

    <!-- Event list -->
    <div class="flex-1 overflow-y-auto">
      <div
        v-if="feedStore.visibleEvents.length === 0"
        class="flex flex-col items-center justify-center h-full text-[var(--argus-text-dimmed)]"
      >
        <UIcon name="i-lucide-radio" class="w-8 h-8 mb-2 opacity-40" />
        <span class="text-sm">Ожидание событий...</span>
        <span class="text-xs mt-1 opacity-60">Подключитесь к серверу для получения данных</span>
      </div>

      <div v-else class="divide-y divide-[var(--argus-border-subtle)]">
        <div
          v-for="event in feedStore.visibleEvents"
          :key="event.id"
          class="px-4 py-2.5 hover:bg-[var(--argus-bg-hover)] cursor-pointer transition-colors duration-150"
          :class="{ 'bg-red-500/5': event.severity === 3 }"
          @click="selectedEvent = event"
        >
          <div class="flex items-start gap-2.5">
            <!-- Severity indicator -->
            <div class="mt-1.5 flex-shrink-0">
              <div
                class="w-2 h-2 rounded-full"
                :class="[getSeverityDot(event.severity), event.severity === 3 ? 'animate-pulse' : '']"
              />
            </div>

            <!-- Category icon -->
            <div class="mt-0.5 flex-shrink-0">
              <UIcon
                :name="categoryIcons[event.category] || categoryIcons.unknown"
                class="w-4 h-4 text-[var(--argus-text-dimmed)]"
              />
            </div>

            <!-- Event content -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span
                  class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium border"
                  :class="getSeverityClasses(event.severity)"
                >
                  {{ event.eventTypeLabel }}
                </span>
                <span class="text-[10px] text-[var(--argus-text-dimmed)] tabular-nums">
                  {{ formatTime(event.timestamp) }}
                </span>
              </div>

              <p
                v-if="event.label"
                class="text-xs text-[var(--argus-text-muted)] mt-0.5 truncate"
              >
                {{ event.label }}
              </p>

              <div class="flex items-center gap-2 mt-1 text-[10px] text-[var(--argus-text-dimmed)]">
                <span class="truncate max-w-[100px]" :title="event.sessionId">
                  {{ event.sessionId.substring(0, 8) }}...
                </span>
                <span v-if="event.confidence < 1" class="tabular-nums">
                  {{ (event.confidence * 100).toFixed(0) }}%
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Footer with totals -->
    <div class="px-4 py-2 border-t border-[var(--argus-border)] flex items-center justify-between text-[10px] text-[var(--argus-text-dimmed)]">
      <span>{{ feedStore.stats.totalDisplayed.toLocaleString() }} событий</span>
      <span>{{ feedStore.stats.totalTelemetryAggregated.toLocaleString() }} телеметрия</span>
    </div>

    <!-- Event Detail Modal -->
    <UModal v-model:open="isEventModalOpen">
      <template #default>
        <div v-if="selectedEvent" class="p-6">
          <div class="flex items-center gap-3 mb-4">
            <span
              class="px-2 py-1 rounded text-xs font-medium border"
              :class="getSeverityClasses(selectedEvent.severity)"
            >
              {{ selectedEvent.severityLabel }}
            </span>
            <h3 class="text-lg font-semibold text-[var(--argus-text)]">
              {{ selectedEvent.eventTypeLabel }}
            </h3>
          </div>

          <div class="space-y-3 text-sm">
            <div class="grid grid-cols-2 gap-3">
              <div>
                <span class="text-[var(--argus-text-dimmed)]">ID события</span>
                <p class="text-[var(--argus-text)] font-mono text-xs mt-0.5">{{ selectedEvent.id }}</p>
              </div>
              <div>
                <span class="text-[var(--argus-text-dimmed)]">Время</span>
                <p class="text-[var(--argus-text)] mt-0.5">{{ formatTime(selectedEvent.timestamp) }}</p>
              </div>
              <div>
                <span class="text-[var(--argus-text-dimmed)]">Сессия</span>
                <p class="text-[var(--argus-text)] font-mono text-xs mt-0.5">{{ selectedEvent.sessionId }}</p>
              </div>
              <div>
                <span class="text-[var(--argus-text-dimmed)]">Студент</span>
                <p class="text-[var(--argus-text)] mt-0.5">{{ selectedEvent.studentId }}</p>
              </div>
              <div>
                <span class="text-[var(--argus-text-dimmed)]">Категория</span>
                <p class="text-[var(--argus-text)] mt-0.5 capitalize">{{ selectedEvent.category }}</p>
              </div>
              <div>
                <span class="text-[var(--argus-text-dimmed)]">Точность</span>
                <p class="text-[var(--argus-text)] mt-0.5">{{ (selectedEvent.confidence * 100).toFixed(1) }}%</p>
              </div>
            </div>

            <div v-if="selectedEvent.label">
              <span class="text-[var(--argus-text-dimmed)]">Описание</span>
              <p class="text-[var(--argus-text)] mt-0.5">{{ selectedEvent.label }}</p>
            </div>
          </div>
        </div>
      </template>
    </UModal>
  </div>
</template>
