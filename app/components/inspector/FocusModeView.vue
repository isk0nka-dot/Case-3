<script setup lang="ts">
// =============================================================================
// FocusModeView — Expanded session view with vertical filmstrip
// =============================================================================
// When a session is focused (double-click or Enter), this replaces the grid
// with a large main view + left filmstrip of other sessions + right sidebar.
// =============================================================================

import type { ActiveSession } from '~/composables/useAdminAPI'
import { useInspectorStore } from '~/stores/useInspectorStore'
import { useEvidenceCapture } from '~/composables/useEvidenceCapture'
import { useAdminAPI } from '~/composables/useAdminAPI'

const props = defineProps<{
  session: ActiveSession
  allSessions: ActiveSession[]
}>()

const inspectorStore = useInspectorStore()
const { captureEvidence } = useEvidenceCapture()
const api = useAdminAPI()
const { isDark, errorBg, warningBg, successBg } = useColors()

const mainPlayerRef = ref<{ videoRef: HTMLVideoElement | null } | null>(null)
const capturing = ref(false)
const actionLoading = ref<Record<string, boolean>>({})

// Other sessions for filmstrip (excluding focused)
const filmstripSessions = computed(() =>
  props.allSessions.filter(s => s.sessionId !== props.session.sessionId)
)

// Current session risk score
const riskScore = computed(() => inspectorStore.getRiskScore(props.session.sessionId))
const riskTrend = computed(() => riskScore.value?.trend ?? [])

// Evidence for this session
const evidence = computed(() => inspectorStore.getEvidence(props.session.sessionId))

// Risk color
const riskColorValue = computed(() => {
  if (riskScore.value.composite >= 60) return 'var(--argus-error)'
  if (riskScore.value.composite >= 30) return 'var(--argus-warning)'
  return 'var(--argus-success)'
})

async function handleCapture() {
  if (capturing.value) return
  capturing.value = true
  try {
    const video = mainPlayerRef.value?.videoRef ?? null
    await captureEvidence(video, props.session.sessionId)
  } finally {
    capturing.value = false
  }
}

function switchFocus(sessionId: string) {
  inspectorStore.setFocus(sessionId)
}

function exitFocus() {
  inspectorStore.setFocus(null)
}

async function handleWarn() {
  actionLoading.value['warn'] = true
  try {
    await api.warnSession(props.session.sessionId)
  } finally {
    actionLoading.value['warn'] = false
  }
}

async function handleTerminate() {
  actionLoading.value['terminate'] = true
  try {
    await api.terminateSession(props.session.sessionId)
  } finally {
    actionLoading.value['terminate'] = false
  }
}

function formatStudentName(studentId: string): string {
  const match = studentId.match(/student-(\d+)/)
  if (match) return `Студент #${match[1]}`
  return studentId
}
</script>

<template>
  <div class="flex h-full focus-mode-container">
    <!-- Left Filmstrip -->
    <div
      class="w-20 shrink-0 border-r border-[var(--argus-border)] overflow-y-auto filmstrip-scroll"
      style="background: var(--argus-bg-deep);"
    >
      <div class="p-1.5 space-y-1.5">
        <div
          v-for="fs in filmstripSessions"
          :key="fs.sessionId"
          class="rounded-lg overflow-hidden cursor-pointer transition-all border"
          :class="[
            fs.violationLevel === 'critical' ? 'border-red-500/50' : fs.violationLevel === 'warning' ? 'border-amber-500/30' : 'border-transparent'
          ]"
          :style="{ background: 'var(--argus-bg-card)' }"
          @click="switchFocus(fs.sessionId)"
        >
          <div
            class="aspect-video overflow-hidden"
            style="background: var(--argus-bg-deep);"
          >
            <VideoPlayer
              :session-id="fs.sessionId"
              :compact="true"
            />
          </div>
          <div class="px-1 py-0.5">
            <p
              class="text-[7px] font-medium truncate"
              style="color: var(--argus-text-dimmed);"
            >
              {{ fs.sessionId.substring(0, 8) }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- Main Expanded View -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- Header bar -->
      <div
        class="flex items-center justify-between px-4 py-2 border-b border-[var(--argus-border)]"
        style="background: var(--argus-bg-card);"
      >
        <div class="flex items-center gap-3">
          <button
            class="flex items-center gap-1 px-2 py-1 rounded-md text-xs transition-all cursor-pointer"
            style="color: var(--argus-text-dimmed); background: var(--argus-bg-elevated);"
            @click="exitFocus"
          >
            <UIcon
              name="i-lucide-arrow-left"
              class="size-3.5"
            />
            Назад
          </button>
          <div>
            <h3
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              {{ formatStudentName(session.studentId) }}
            </h3>
            <p
              class="text-[10px]"
              style="color: var(--argus-text-dimmed);"
            >
              {{ session.sessionId }} · {{ session.examId }}
            </p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <!-- Risk gauge -->
          <RiskGauge
            :score="riskScore.composite"
            :size="32"
          />

          <!-- Evidence capture -->
          <button
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer"
            style="background: var(--argus-bg-elevated); color: var(--argus-accent); border: 1px solid var(--argus-border);"
            :disabled="capturing"
            @click="handleCapture"
          >
            <UIcon
              v-if="!capturing"
              name="i-lucide-camera"
              class="size-3.5"
            />
            <span
              v-else
              class="size-3.5 animate-spin rounded-full border border-t-transparent"
              style="border-color: var(--argus-accent);"
            />
            Захват ({{ inspectorStore.getEvidenceCount(session.sessionId) }})
          </button>

          <!-- Actions -->
          <button
            v-if="session.status === 'active'"
            class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer"
            :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
            :disabled="actionLoading['warn']"
            @click="handleWarn"
          >
            <UIcon
              name="i-lucide-alert-triangle"
              class="size-3.5"
            />
            Предупредить
          </button>
          <button
            v-if="session.status === 'active'"
            class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-xs font-medium transition-all cursor-pointer"
            :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
            :disabled="actionLoading['terminate']"
            @click="handleTerminate"
          >
            <UIcon
              name="i-lucide-ban"
              class="size-3.5"
            />
            Завершить
          </button>
        </div>
      </div>

      <!-- Main content area -->
      <div class="flex-1 flex gap-4 p-4 overflow-hidden">
        <!-- Large video player -->
        <div class="flex-1 flex flex-col gap-3">
          <div
            class="relative rounded-xl overflow-hidden flex-1"
            style="background: var(--argus-bg-deep);"
          >
            <VideoPlayer
              ref="mainPlayerRef"
              :session-id="session.sessionId"
              :compact="false"
            />
            <!-- AI Overlay (read-only mode — no local inference in monitoring) -->
            <AIOverlay
              :vision-frame="null"
              :audio-frame="null"
              :is-vision-active="false"
              :is-audio-active="false"
              class="absolute inset-0 pointer-events-none"
            />
          </div>

          <!-- Sparkline + stats row -->
          <div class="flex items-center gap-4">
            <div class="glass-card rounded-lg px-3 py-2 flex items-center gap-3">
              <span
                class="text-[10px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >Тренд риска</span>
              <RiskSparkline
                v-if="riskTrend.length >= 2"
                :data="riskTrend"
                :color="riskColorValue"
                :width="160"
                :height="24"
              />
            </div>

            <div class="flex items-center gap-3">
              <div class="glass-card rounded-lg px-3 py-2 text-center">
                <div
                  class="text-sm font-bold tabular-nums"
                  style="color: var(--argus-error);"
                >
                  {{ session.criticalCount }}
                </div>
                <div
                  class="text-[9px]"
                  style="color: var(--argus-text-dimmed);"
                >
                  Критических
                </div>
              </div>
              <div class="glass-card rounded-lg px-3 py-2 text-center">
                <div
                  class="text-sm font-bold tabular-nums"
                  style="color: var(--argus-warning);"
                >
                  {{ session.warningCount }}
                </div>
                <div
                  class="text-[9px]"
                  style="color: var(--argus-text-dimmed);"
                >
                  Предупр.
                </div>
              </div>
              <div class="glass-card rounded-lg px-3 py-2 text-center">
                <div
                  class="text-sm font-bold tabular-nums"
                  style="color: var(--argus-text);"
                >
                  {{ session.totalEvents }}
                </div>
                <div
                  class="text-[9px]"
                  style="color: var(--argus-text-dimmed);"
                >
                  Событий
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Evidence history panel -->
        <div
          v-if="evidence.length > 0"
          class="w-48 shrink-0 overflow-y-auto space-y-2"
        >
          <h4
            class="text-[10px] font-semibold uppercase tracking-wider px-1"
            style="color: var(--argus-text-dimmed);"
          >
            Доказательства ({{ evidence.length }})
          </h4>
          <div
            v-for="ev in evidence"
            :key="ev.id"
            class="glass-card rounded-lg overflow-hidden"
          >
            <img
              :src="ev.frameDataUrl"
              :alt="`Evidence ${ev.id}`"
              class="w-full aspect-video object-cover"
            >
            <div class="px-2 py-1.5 space-y-0.5">
              <div class="flex items-center justify-between">
                <span
                  class="text-[8px] font-bold tabular-nums"
                  :style="{ color: riskColorValue }"
                >
                  Риск: {{ ev.aiMetadata.riskScore }}
                </span>
                <span
                  class="text-[8px] tabular-nums"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ new Date(ev.timestamp).toLocaleTimeString('ru-RU') }}
                </span>
              </div>
              <div
                class="text-[7px] font-mono truncate"
                style="color: var(--argus-text-dimmed);"
                :title="ev.frameSha256"
              >
                SHA: {{ ev.frameSha256.substring(0, 16) }}...
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.filmstrip-scroll::-webkit-scrollbar {
  width: 3px;
}
.filmstrip-scroll::-webkit-scrollbar-track {
  background: transparent;
}
.filmstrip-scroll::-webkit-scrollbar-thumb {
  background: var(--argus-scrollbar);
  border-radius: 3px;
}
</style>
