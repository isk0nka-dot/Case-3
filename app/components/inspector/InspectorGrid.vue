<script setup lang="ts">
// =============================================================================
// InspectorGrid — CSS Grid with TransitionGroup reorder
// =============================================================================
// Renders InspectorCell components in a responsive CSS grid that auto-reorders
// by risk score with smooth GPU-composited transitions.
// =============================================================================

import type { ActiveSession } from '~/composables/useAdminAPI'
import { useInspectorStore, type GridDensity } from '~/stores/useInspectorStore'

const props = defineProps<{
  sessions: ActiveSession[]
}>()

const emit = defineEmits<{
  (e: 'focus', sessionId: string): void
}>()

const inspectorStore = useInspectorStore()

// Sorted sessions using risk-based ordering
const sortedSessions = computed(() => {
  return inspectorStore.sortSessionsByRisk(props.sessions)
})

// Grid columns based on density setting
const gridStyle = computed(() => {
  const cols = inspectorStore.gridCols()
  const gap = cols >= 5 ? '6px' : cols >= 4 ? '8px' : '12px'
  return {
    display: 'grid',
    gridTemplateColumns: `repeat(${cols}, 1fr)`,
    gap
  }
})

// Whether cells should use compact mode
const isCompact = computed(() => inspectorStore.compactMode)

// Top-3 session IDs for pulse effect
const top3Set = computed(() => new Set(inspectorStore.top3SessionIds))

function handleFocus(sessionId: string) {
  inspectorStore.setFocus(sessionId)
  emit('focus', sessionId)
}

function handleFilter(sessionId: string) {
  inspectorStore.setSidebarFilter(sessionId)
}
</script>

<template>
  <div
    :style="gridStyle"
    class="inspector-grid"
  >
    <TransitionGroup name="grid-reorder">
      <InspectorCell
        v-for="(session, index) in sortedSessions"
        :key="session.sessionId"
        :session="session"
        :risk-score="inspectorStore.getRiskScore(session.sessionId)"
        :is-top3="top3Set.has(session.sessionId)"
        :is-selected="inspectorStore.selectedCellIndex === index"
        :compact="isCompact"
        @focus="handleFocus"
        @filter="handleFilter"
      />
    </TransitionGroup>

    <!-- Empty state -->
    <div
      v-if="sortedSessions.length === 0"
      class="col-span-full flex flex-col items-center justify-center py-16"
    >
      <UIcon
        name="i-lucide-video-off"
        class="size-12 mb-3"
        style="color: var(--argus-text-dimmed);"
      />
      <p
        class="text-sm font-medium"
        style="color: var(--argus-text);"
      >
        Нет активных сессий
      </p>
      <p
        class="text-xs mt-1"
        style="color: var(--argus-text-dimmed);"
      >
        Измените критерии поиска или сбросьте фильтр
      </p>
    </div>
  </div>
</template>
