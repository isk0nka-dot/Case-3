<script setup lang="ts">
// =============================================================================
// IncidentSidebar — Filtered event stream with silence mode
// =============================================================================
// 320px right sidebar showing real-time incidents from useEventFeedStore.
// Filter by session when sidebarFilterSessionId is set. Silence mode dims
// non-critical events. AI summary tooltips on hover.
// =============================================================================

import { useEventFeedStore, type FeedEvent } from '~/stores/useEventFeedStore'
import { useInspectorStore } from '~/stores/useInspectorStore'
import { Severity } from '~/lib/proto/types'

const feedStore = useEventFeedStore()
const inspectorStore = useInspectorStore()

// Filtered events based on sidebar filter
const events = computed(() => {
  const filterSessionId = inspectorStore.sidebarFilterSessionId
  if (filterSessionId) {
    return feedStore.visibleEvents.filter(e => e.sessionId === filterSessionId)
  }
  return feedStore.visibleEvents
})

// Category icons
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

function getSeverityDot(severity: Severity): string {
  switch (severity) {
    case Severity.CRITICAL: return 'bg-red-500'
    case Severity.WARNING: return 'bg-amber-500'
    case Severity.INFO: return 'bg-blue-500'
    default: return 'bg-gray-500'
  }
}

function getSeverityClasses(severity: Severity): string {
  switch (severity) {
    case Severity.CRITICAL: return 'bg-red-500/20 text-red-400 border-red-500/30'
    case Severity.WARNING: return 'bg-amber-500/20 text-amber-400 border-amber-500/30'
    case Severity.INFO: return 'bg-blue-500/20 text-blue-400 border-blue-500/30'
    default: return 'bg-gray-500/20 text-gray-400 border-gray-500/30'
  }
}

// formatTimeAgoMs → centralized in useFormatters() composable
const { formatTimeAgoMs } = useFormatters()

function isSilenced(event: FeedEvent): boolean {
  return inspectorStore.silenceMode && event.severity !== Severity.CRITICAL
}

function clearFilter() {
  inspectorStore.setSidebarFilter(null)
}
</script>

<template>
  <div class="flex flex-col h-full border-l border-[var(--argus-border)]" style="width: 320px; min-width: 320px;">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-[var(--argus-border)]">
      <div class="flex items-center gap-2">
        <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse" />
        <h3 class="text-sm font-semibold" style="color: var(--argus-text);">
          Инциденты
        </h3>
        <span class="text-xs tabular-nums" style="color: var(--argus-text-dimmed);">
          {{ feedStore.stats.eventsPerSecond }}/сек
        </span>
      </div>

      <div class="flex items-center gap-2">
        <!-- Silence mode toggle -->
        <button
          class="flex items-center gap-1 px-2 py-1 rounded-md text-[10px] font-medium transition-all cursor-pointer"
          :style="{
            background: inspectorStore.silenceMode ? 'rgba(251, 191, 36, 0.15)' : 'var(--argus-bg-elevated)',
            color: inspectorStore.silenceMode ? 'var(--argus-warning)' : 'var(--argus-text-dimmed)',
            border: `1px solid ${inspectorStore.silenceMode ? 'rgba(251, 191, 36, 0.3)' : 'var(--argus-border)'}`
          }"
          @click="inspectorStore.toggleSilence()"
        >
          <UIcon :name="inspectorStore.silenceMode ? 'i-lucide-bell-off' : 'i-lucide-bell'" class="size-3" />
          {{ inspectorStore.silenceMode ? 'Тишина' : 'Звук' }}
        </button>
      </div>
    </div>

    <!-- Session filter chip -->
    <div
      v-if="inspectorStore.sidebarFilterSessionId"
      class="px-4 py-2 border-b border-[var(--argus-border)] flex items-center gap-2"
      style="background: var(--argus-bg-elevated);"
    >
      <UIcon name="i-lucide-filter" class="size-3" style="color: var(--argus-accent);" />
      <span class="text-[10px] font-medium truncate" style="color: var(--argus-accent);">
        {{ inspectorStore.sidebarFilterSessionId.substring(0, 12) }}...
      </span>
      <button
        class="ml-auto text-[10px] px-1.5 py-0.5 rounded cursor-pointer"
        style="color: var(--argus-text-dimmed); background: var(--argus-bg-hover);"
        @click="clearFilter"
      >
        Очистить
      </button>
    </div>

    <!-- Severity counters -->
    <div class="flex items-center gap-3 px-4 py-2 text-xs border-b border-[var(--argus-border-subtle)]">
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

    <!-- Event list -->
    <div class="flex-1 overflow-y-auto">
      <div
        v-if="events.length === 0"
        class="flex flex-col items-center justify-center h-full"
        style="color: var(--argus-text-dimmed);"
      >
        <UIcon name="i-lucide-radio" class="w-8 h-8 mb-2 opacity-40" />
        <span class="text-sm">Ожидание событий...</span>
      </div>

      <div v-else class="divide-y divide-[var(--argus-border-subtle)]">
        <div
          v-for="event in events"
          :key="event.id"
          class="px-3 py-2 hover:bg-[var(--argus-bg-hover)] transition-colors duration-150 cursor-default"
          :class="{ 'bg-red-500/5': event.severity === 3 }"
          :style="{ opacity: isSilenced(event) ? 0.3 : 1 }"
        >
          <div class="flex items-start gap-2">
            <!-- Severity dot -->
            <div class="mt-1.5 flex-shrink-0">
              <div
                class="w-1.5 h-1.5 rounded-full"
                :class="[getSeverityDot(event.severity), event.severity === 3 ? 'animate-pulse' : '']"
              />
            </div>

            <!-- Category icon -->
            <div class="mt-0.5 flex-shrink-0">
              <UIcon
                :name="categoryIcons[event.category] || categoryIcons.unknown"
                class="w-3.5 h-3.5"
                style="color: var(--argus-text-dimmed);"
              />
            </div>

            <!-- Content -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-1.5">
                <span
                  class="inline-flex items-center px-1 py-px rounded text-[9px] font-medium border"
                  :class="getSeverityClasses(event.severity)"
                >
                  {{ event.eventTypeLabel }}
                </span>
                <span class="text-[9px] tabular-nums" style="color: var(--argus-text-dimmed);">
                  {{ formatTimeAgoMs(event.receivedAt) }}
                </span>
              </div>

              <p
                v-if="event.label"
                class="text-[10px] mt-0.5 truncate"
                style="color: var(--argus-text-muted);"
              >
                {{ event.label }}
              </p>

              <div class="flex items-center gap-1.5 mt-0.5 text-[9px]" style="color: var(--argus-text-dimmed);">
                <span class="truncate max-w-[80px]" :title="event.sessionId">
                  {{ event.sessionId.substring(0, 8) }}
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

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-[var(--argus-border)] flex items-center justify-between text-[10px]" style="color: var(--argus-text-dimmed);">
      <span>{{ feedStore.stats.totalDisplayed.toLocaleString() }} событий</span>
      <span>{{ feedStore.stats.totalTelemetryAggregated.toLocaleString() }} тел.</span>
    </div>
  </div>
</template>
