<script setup lang="ts">
import { useDashboardStore, type MonitoringSession } from '~/stores/useDashboardStore'

const props = defineProps<{
  session: MonitoringSession | null
  /** Whether this session is live (true) or from archive/violation review (false) */
  isLive?: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const store = useDashboardStore()
const { isDark, accentBg, errorBg, successBg, warningBg, purpleBg } = useColors()
const toast = useToast()

// --- Exception lookup ---
function hasException(session: { studentName: string, iin: string, examName: string }): boolean {
  return store.getStudentExceptionInfo(session.studentName, session.iin, session.examName) !== null
}
function getExceptionLabel(session: { studentName: string, iin: string, examName: string }): string {
  const exc = store.getStudentExceptionInfo(session.studentName, session.iin, session.examName)
  return exc?.profileLabel ?? ''
}

// --- AI Status helpers ---
function gazeColor(status: string): string {
  switch (status) {
    case 'normal': return 'var(--argus-success)'
    case 'warning': return 'var(--argus-warning)'
    case 'critical': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function gazeLabel(status: string): string {
  switch (status) {
    case 'normal': return 'Норма'
    case 'warning': return 'Внимание'
    case 'critical': return 'Критично'
    default: return '—'
  }
}

function faceColor(status: string): string {
  switch (status) {
    case 'verified': return 'var(--argus-success)'
    case 'mismatch': return 'var(--argus-error)'
    case 'unavailable': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function faceLabel(status: string): string {
  switch (status) {
    case 'verified': return 'Подтв.'
    case 'mismatch': return 'Несовп.'
    case 'unavailable': return 'Н/Д'
    default: return '—'
  }
}

function objectColor(status: string): string {
  switch (status) {
    case 'clear': return 'var(--argus-success)'
    case 'phone': return 'var(--argus-error)'
    case 'book': return 'var(--argus-warning)'
    case 'earbuds': return 'var(--argus-warning)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function objectLabel(status: string): string {
  switch (status) {
    case 'clear': return 'Чисто'
    case 'phone': return 'Телефон'
    case 'book': return 'Книга'
    case 'earbuds': return 'Наушники'
    default: return '—'
  }
}

function objectIcon(status: string): string {
  switch (status) {
    case 'clear': return 'i-lucide-check-circle'
    case 'phone': return 'i-lucide-smartphone'
    case 'book': return 'i-lucide-book-open'
    case 'earbuds': return 'i-lucide-headphones'
    default: return 'i-lucide-circle'
  }
}

// integrityColor + integrityGradient are auto-imported from useStatusHelpers
function integrityGradientLocal(score: number): string {
  return integrityGradient(score, isDark.value)
}

function severityBgFn(severity: string, opacity: number): string {
  switch (severity) {
    case 'critical': return errorBg(opacity)
    case 'warning': return warningBg(opacity)
    case 'info': return isDark.value ? `rgba(100, 116, 139, ${opacity})` : `rgba(148, 163, 184, ${opacity})`
    default: return 'transparent'
  }
}

// --- Modal close ---
function closeSession() {
  if (document.fullscreenElement) {
    document.exitFullscreen()
  }
  camerasSwapped.value = false
  emit('close')
}

// --- Warn/Terminate interaction ---
const showTerminateConfirm = ref(false)
const showStudentWarning = ref(false)
const warningViolationType = ref('')
const sessionTerminated = ref(false)

// Reset internal state when session changes
watch(() => props.session, () => {
  camerasSwapped.value = false
  isFullscreen.value = false
  isMuted.value = true
  audioVolume.value = 75
  showStudentWarning.value = false
  showTerminateConfirm.value = false
  sessionTerminated.value = false
})

function getLastViolationType(session: MonitoringSession): string {
  if (session.aiStatus.objectDetection === 'phone') return 'Обнаружен телефон'
  if (session.aiStatus.objectDetection === 'book') return 'Обнаружена книга'
  if (session.aiStatus.objectDetection === 'earbuds') return 'Обнаружены наушники'
  if (session.aiStatus.gazeTracking === 'critical') return 'Критическое отклонение взгляда'
  if (session.aiStatus.gazeTracking === 'warning') return 'Отклонение взгляда от экрана'
  if (session.aiStatus.faceIdMatch === 'mismatch') return 'Несовпадение лица'
  return 'Подозрительная активность'
}

function handleWarn() {
  if (!props.session) return
  warningViolationType.value = getLastViolationType(props.session)
  showStudentWarning.value = true

  props.session.events.unshift({
    id: `evt-warn-${Date.now()}`,
    type: 'audio_anomaly',
    label: 'Отправлено предупреждение проктором',
    severity: 'warning',
    timestamp: new Date().toISOString()
  })
  props.session.violationCount += 1

  toast.add({
    title: 'Предупреждение отправлено студенту',
    icon: 'i-lucide-alert-triangle',
    color: 'warning' as const
  })
}

function dismissStudentWarning() {
  showStudentWarning.value = false
}

function handleTerminate() {
  showTerminateConfirm.value = true
}

function confirmTerminate() {
  if (!props.session) return
  props.session.events.unshift({
    id: `evt-term-${Date.now()}`,
    type: 'face_mismatch',
    label: 'Экзамен принудительно завершён проктором',
    severity: 'critical',
    timestamp: new Date().toISOString()
  })
  sessionTerminated.value = true
  showTerminateConfirm.value = false
}

function cancelTerminate() {
  showTerminateConfirm.value = false
}

// --- Camera swap ---
const camerasSwapped = ref(false)

function swapCameras() {
  camerasSwapped.value = !camerasSwapped.value
}

// --- Dual fullscreen mode ---
const webcamRef = ref<HTMLElement | null>(null)
const sideCamRef = ref<HTMLElement | null>(null)
const isFullscreen = ref(false)
const fullscreenTarget = ref<'webcam' | 'side' | null>(null)

async function toggleFullscreenWebcam() {
  const target = camerasSwapped.value ? sideCamRef.value : webcamRef.value
  if (!target) return

  if (document.fullscreenElement) {
    await document.exitFullscreen()
    isFullscreen.value = false
    fullscreenTarget.value = null
  } else {
    try {
      await target.requestFullscreen()
      isFullscreen.value = true
      fullscreenTarget.value = 'webcam'
    } catch (_e) {
      // Fullscreen not available
    }
  }
}

async function toggleFullscreenSide() {
  const target = camerasSwapped.value ? webcamRef.value : sideCamRef.value
  if (!target) return

  if (document.fullscreenElement) {
    await document.exitFullscreen()
    isFullscreen.value = false
    fullscreenTarget.value = null
  } else {
    try {
      await target.requestFullscreen()
      isFullscreen.value = true
      fullscreenTarget.value = 'side'
    } catch (_e) {
      // Fullscreen not available
    }
  }
}

function handleFullscreenChange() {
  isFullscreen.value = !!document.fullscreenElement
  if (!document.fullscreenElement) {
    fullscreenTarget.value = null
  }
}

// --- Audio analytics ---
const isMuted = ref(true)
const audioVolume = ref(75)

const waveformBars = 32
const waveformHeights = ref<number[]>([])

function generateWaveform() {
  waveformHeights.value = Array.from({ length: waveformBars }, () =>
    Math.random() * 80 + 20
  )
}

generateWaveform()

let waveformInterval: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  waveformInterval = setInterval(generateWaveform, 250)
})

onUnmounted(() => {
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  if (waveformInterval) clearInterval(waveformInterval)
})

const noiseLevel = computed(() => {
  if (!props.session) return 0
  const base = props.session.violationLevel === 'critical' ? 65
    : props.session.violationLevel === 'warning' ? 45
    : 25
  return base + Math.floor(Math.random() * 10)
})

function noiseLevelColor(level: number): string {
  if (level > 60) return 'var(--argus-error)'
  if (level > 40) return 'var(--argus-warning)'
  return 'var(--argus-success)'
}

function noiseLevelLabel(level: number): string {
  if (level > 60) return 'Высокий'
  if (level > 40) return 'Средний'
  return 'Тихо'
}

// Determine live or recorded status
const sessionIsLive = computed(() => props.isLive ?? props.session?.isOnline ?? true)
</script>

<template>
  <Teleport to="body">
    <!-- ===== MAIN SESSION MODAL ===== -->
    <Transition name="modal">
      <div
        v-if="session"
        class="fixed inset-0 z-[100] flex items-center justify-center p-4"
      >
        <!-- Backdrop -->
        <div
          class="absolute inset-0"
          :style="{ background: isDark ? 'rgba(0, 0, 0, 0.7)' : 'rgba(0, 0, 0, 0.4)' }"
          @click="closeSession"
        />

        <!-- Modal Content -->
        <div
          class="relative w-full max-w-7xl max-h-[92vh] rounded-2xl border overflow-hidden flex flex-col z-10"
          :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
        >
          <!-- Modal Header -->
          <div class="flex items-center justify-between px-6 py-4 border-b shrink-0" style="border-color: var(--argus-border);">
            <div class="flex items-center gap-3">
              <div
                class="flex items-center gap-1.5 px-2.5 py-1 rounded-full"
                :style="{
                  background: session.violationLevel === 'critical' ? errorBg(0.1) : session.violationLevel === 'warning' ? warningBg(0.1) : successBg(0.1),
                  color: session.violationLevel === 'critical' ? 'var(--argus-error)' : session.violationLevel === 'warning' ? 'var(--argus-warning)' : 'var(--argus-success)'
                }"
              >
                <span class="relative flex size-1.5">
                  <span
                    v-if="sessionIsLive"
                    class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
                    :style="{ background: session.violationLevel === 'critical' ? 'var(--argus-error)' : session.violationLevel === 'warning' ? 'var(--argus-warning)' : 'var(--argus-success)' }"
                  />
                  <span
                    class="relative inline-flex size-1.5 rounded-full"
                    :style="{ background: session.violationLevel === 'critical' ? 'var(--argus-error)' : session.violationLevel === 'warning' ? 'var(--argus-warning)' : 'var(--argus-success)' }"
                  />
                </span>
                <span class="text-[10px] font-bold uppercase">{{ sessionIsLive ? 'LIVE' : 'ЗАПИСЬ' }}</span>
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <h2 class="text-lg font-bold" style="color: var(--argus-text);">{{ session.studentName }}</h2>
                  <!-- Exception badge in modal -->
                  <span
                    v-if="hasException(session)"
                    class="inline-flex items-center gap-1 text-[9px] font-bold px-2 py-0.5 rounded-full"
                    :style="{ background: purpleBg(0.12), color: isDark ? '#a78bfa' : '#7c3aed', border: `1px solid ${purpleBg(0.2)}` }"
                  >
                    <UIcon name="i-lucide-shield-check" class="size-3" />
                    {{ getExceptionLabel(session) }}
                  </span>
                </div>
                <p class="text-xs" style="color: var(--argus-text-dimmed);">
                  ИИН: {{ session.iin }} · {{ session.phone }} · {{ session.examName }}
                </p>
              </div>
            </div>
            <button
              class="flex items-center justify-center size-8 rounded-lg transition-colors"
              style="color: var(--argus-text-dimmed);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              @click="closeSession"
            >
              <UIcon name="i-lucide-x" class="size-5" />
            </button>
          </div>

          <!-- Modal Body -->
          <div class="flex-1 overflow-y-auto">
            <div class="grid grid-cols-1 lg:grid-cols-7 gap-0">
              <!-- Left: Video Feeds (5 cols) -->
              <div class="lg:col-span-5 p-6 space-y-4">
                <!-- Dual Camera Layout -->
                <div class="flex gap-3" :class="camerasSwapped ? 'flex-row-reverse' : ''">
                  <!-- PRIMARY FEED (Webcam) -->
                  <div
                    ref="webcamRef"
                    class="relative flex-[3] aspect-video rounded-xl overflow-hidden"
                    :class="{ 'rounded-none': isFullscreen && fullscreenTarget === 'webcam' }"
                    :style="{ background: 'var(--argus-bg-deep)' }"
                  >
                    <!-- LiveKit Video Player (replaces static placeholder) -->
                    <VideoPlayer
                      :session-id="session.sessionId"
                      :compact="false"
                    />

                    <!-- AI overlays -->
                    <div class="absolute top-3 left-3 flex items-center gap-1.5 flex-wrap">
                      <div
                        class="flex items-center gap-1.5 px-2 py-1 rounded-lg ai-badge"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)' }"
                      >
                        <UIcon name="i-lucide-eye" class="size-3" :style="{ color: gazeColor(session.aiStatus.gazeTracking) }" />
                        <span class="text-[9px] font-bold" :style="{ color: gazeColor(session.aiStatus.gazeTracking) }">
                          Взгляд: {{ gazeLabel(session.aiStatus.gazeTracking) }}
                        </span>
                      </div>
                      <div
                        class="flex items-center gap-1.5 px-2 py-1 rounded-lg ai-badge"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)' }"
                      >
                        <UIcon name="i-lucide-scan-face" class="size-3" :style="{ color: faceColor(session.aiStatus.faceIdMatch) }" />
                        <span class="text-[9px] font-bold" :style="{ color: faceColor(session.aiStatus.faceIdMatch) }">
                          Лицо: {{ faceLabel(session.aiStatus.faceIdMatch) }}
                        </span>
                      </div>
                      <div
                        class="flex items-center gap-1.5 px-2 py-1 rounded-lg ai-badge"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)' }"
                      >
                        <UIcon :name="objectIcon(session.aiStatus.objectDetection)" class="size-3" :style="{ color: objectColor(session.aiStatus.objectDetection) }" />
                        <span class="text-[9px] font-bold" :style="{ color: objectColor(session.aiStatus.objectDetection) }">
                          Предметы: {{ objectLabel(session.aiStatus.objectDetection) }}
                        </span>
                      </div>
                    </div>

                    <!-- Top-right: Integrity + Fullscreen -->
                    <div class="absolute top-3 right-3 flex items-center gap-1.5">
                      <div
                        class="px-2.5 py-1.5 rounded-lg ai-badge"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)' }"
                      >
                        <span class="text-base font-bold tabular-nums" :style="{ color: integrityColor(session.integrityScore) }">
                          {{ session.integrityScore }}%
                        </span>
                      </div>
                      <button
                        class="flex items-center justify-center size-8 rounded-lg ai-badge transition-all"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)', color: 'var(--argus-text-muted)' }"
                        :title="isFullscreen && fullscreenTarget === 'webcam' ? 'Выйти из полноэкранного режима' : camerasSwapped ? 'Во весь экран (Боковая камера)' : 'Во весь экран (Веб-камера)'"
                        @mouseenter="($event.currentTarget as HTMLElement).style.color = 'var(--argus-accent)'"
                        @mouseleave="($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-muted)'"
                        @click.stop="toggleFullscreenWebcam"
                      >
                        <UIcon :name="isFullscreen && fullscreenTarget === 'webcam' ? 'i-lucide-minimize' : 'i-lucide-maximize'" class="size-4" />
                      </button>
                    </div>
                  </div>

                  <!-- SECONDARY FEED (Side Camera) -->
                  <div
                    ref="sideCamRef"
                    class="relative flex-[1] min-w-[160px] rounded-xl overflow-hidden border"
                    :class="{ 'rounded-none': isFullscreen && fullscreenTarget === 'side' }"
                    :style="{ background: 'var(--argus-bg-deep)', borderColor: 'var(--argus-border)' }"
                  >
                    <div class="absolute inset-0 flex items-center justify-center">
                      <div class="flex flex-col items-center gap-2 opacity-25">
                        <UIcon name="i-lucide-camera" class="size-8" style="color: var(--argus-text-dimmed);" />
                        <span class="text-[9px] font-medium text-center px-2" style="color: var(--argus-text-dimmed);">{{ camerasSwapped ? 'ВЕБ-КАМЕРА' : 'БОКОВАЯ КАМЕРА' }}</span>
                      </div>
                    </div>

                    <div class="absolute top-2 right-2 flex items-center gap-1">
                      <button
                        class="flex items-center justify-center size-7 rounded-md ai-badge transition-all"
                        :style="{ background: isDark ? 'rgba(11, 15, 20, 0.8)' : 'rgba(255, 255, 255, 0.9)', color: 'var(--argus-text-muted)' }"
                        :title="isFullscreen && fullscreenTarget === 'side' ? 'Выйти из полноэкранного режима' : camerasSwapped ? 'Во весь экран (Веб-камера)' : 'Во весь экран (Боковая камера)'"
                        @mouseenter="($event.currentTarget as HTMLElement).style.color = 'var(--argus-accent)'"
                        @mouseleave="($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-muted)'"
                        @click.stop="toggleFullscreenSide"
                      >
                        <UIcon :name="isFullscreen && fullscreenTarget === 'side' ? 'i-lucide-minimize' : 'i-lucide-maximize'" class="size-3.5" />
                      </button>
                    </div>

                    <!-- Live/Recorded badge on side camera -->
                    <div
                      class="absolute top-2 left-2 flex items-center gap-1 px-1.5 py-0.5 rounded ai-badge"
                      :style="{ background: isDark ? 'rgba(11, 15, 20, 0.75)' : 'rgba(255, 255, 255, 0.85)' }"
                    >
                      <template v-if="sessionIsLive">
                        <span class="relative flex size-1">
                          <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
                          <span class="relative inline-flex size-1 rounded-full bg-red-500" />
                        </span>
                        <span class="text-[7px] font-bold uppercase" style="color: var(--argus-error);">LIVE</span>
                      </template>
                      <template v-else>
                        <UIcon name="i-lucide-circle-dot" class="size-2.5" style="color: var(--argus-text-dimmed);" />
                        <span class="text-[7px] font-bold uppercase" style="color: var(--argus-text-dimmed);">REC</span>
                      </template>
                    </div>
                  </div>
                </div>

                <!-- ===== AUDIO ANALYTICS SECTION ===== -->
                <div
                  class="rounded-xl border overflow-hidden"
                  :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)' }"
                >
                  <div class="flex items-center justify-between px-4 py-2.5 border-b" style="border-color: var(--argus-border);">
                    <div class="flex items-center gap-2">
                      <UIcon name="i-lucide-audio-waveform" class="size-4" style="color: var(--argus-accent);" />
                      <span class="text-xs font-semibold" style="color: var(--argus-text);">Аудио аналитика</span>
                      <span
                        class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
                        :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
                      >
                        Детекция голоса
                      </span>
                    </div>

                    <div class="flex items-center gap-3">
                      <div class="flex items-center gap-1.5">
                        <span class="text-[9px] font-medium" style="color: var(--argus-text-dimmed);">Уровень шума:</span>
                        <span class="text-[10px] font-bold tabular-nums" :style="{ color: noiseLevelColor(noiseLevel) }">
                          {{ noiseLevel }} дБ
                        </span>
                        <span
                          class="text-[8px] font-semibold px-1.5 py-0.5 rounded"
                          :style="{
                            background: noiseLevel > 60 ? errorBg(0.1) : noiseLevel > 40 ? warningBg(0.1) : successBg(0.1),
                            color: noiseLevelColor(noiseLevel)
                          }"
                        >
                          {{ noiseLevelLabel(noiseLevel) }}
                        </span>
                      </div>
                    </div>
                  </div>

                  <div class="flex items-center gap-4 px-4 py-3">
                    <div class="flex-1 flex items-end gap-[2px] h-10">
                      <div
                        v-for="(h, i) in waveformHeights"
                        :key="i"
                        class="flex-1 rounded-t-sm transition-[height] duration-200"
                        :style="{
                          height: `${isMuted ? 10 : h}%`,
                          background: isMuted
                            ? 'var(--argus-text-dimmed)'
                            : h > 75
                              ? 'var(--argus-error)'
                              : h > 50
                                ? 'var(--argus-warning)'
                                : 'var(--argus-accent)',
                          opacity: isMuted ? 0.25 : 0.7
                        }"
                      />
                    </div>

                    <div class="flex items-center gap-2 shrink-0">
                      <button
                        class="flex items-center justify-center size-8 rounded-lg transition-all border"
                        :style="{
                          background: isMuted ? 'transparent' : accentBg(0.1),
                          borderColor: isMuted ? 'var(--argus-border)' : accentBg(0.25),
                          color: isMuted ? 'var(--argus-text-dimmed)' : 'var(--argus-accent)'
                        }"
                        :title="isMuted ? 'Включить звук' : 'Выключить звук'"
                        @mouseenter="($event.currentTarget as HTMLElement).style.background = isMuted ? 'var(--argus-bg-hover)' : accentBg(0.15)"
                        @mouseleave="($event.currentTarget as HTMLElement).style.background = isMuted ? 'transparent' : accentBg(0.1)"
                        @click="isMuted = !isMuted"
                      >
                        <UIcon :name="isMuted ? 'i-lucide-volume-x' : audioVolume > 50 ? 'i-lucide-volume-2' : 'i-lucide-volume-1'" class="size-4" />
                      </button>

                      <div class="flex items-center gap-2 w-24">
                        <input
                          v-model.number="audioVolume"
                          type="range"
                          min="0"
                          max="100"
                          class="w-full h-1 rounded-full appearance-none cursor-pointer"
                          :style="{
                            background: `linear-gradient(to right, var(--argus-accent) ${audioVolume}%, var(--argus-border) ${audioVolume}%)`,
                            opacity: isMuted ? 0.35 : 1
                          }"
                          :disabled="isMuted"
                        >
                        <span class="text-[9px] font-bold tabular-nums w-7 text-right" :style="{ color: isMuted ? 'var(--argus-text-dimmed)' : 'var(--argus-text-muted)' }">
                          {{ isMuted ? '—' : `${audioVolume}%` }}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Camera Controls + Action Buttons -->
                <div class="flex items-center justify-between gap-3">
                  <div class="flex items-center gap-3">
                    <button
                      class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-semibold transition-all"
                      :class="{ 'opacity-50 pointer-events-none': sessionTerminated }"
                      :style="{ background: warningBg(0.1), color: 'var(--argus-warning)', border: `1px solid ${warningBg(0.2)}` }"
                      @mouseenter="($event.currentTarget as HTMLElement).style.background = warningBg(0.2)"
                      @mouseleave="($event.currentTarget as HTMLElement).style.background = warningBg(0.1)"
                      @click="handleWarn"
                    >
                      <UIcon name="i-lucide-alert-triangle" class="size-4" />
                      Предупредить
                    </button>
                    <button
                      class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-semibold transition-all"
                      :class="{ 'opacity-50 pointer-events-none': sessionTerminated }"
                      :style="{ background: errorBg(0.1), color: 'var(--argus-error)', border: `1px solid ${errorBg(0.2)}` }"
                      @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.2)"
                      @mouseleave="($event.currentTarget as HTMLElement).style.background = errorBg(0.1)"
                      @click="handleTerminate"
                    >
                      <UIcon name="i-lucide-ban" class="size-4" />
                      Завершить экзамен
                    </button>

                    <span
                      v-if="sessionTerminated"
                      class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-bold"
                      :style="{ background: errorBg(0.1), color: 'var(--argus-error)', border: `1px solid ${errorBg(0.25)}` }"
                    >
                      <UIcon name="i-lucide-octagon-x" class="size-4" />
                      Экзамен завершён
                    </span>
                  </div>

                  <button
                    class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all border"
                    :style="{
                      background: camerasSwapped ? accentBg(0.1) : 'transparent',
                      borderColor: camerasSwapped ? accentBg(0.25) : 'var(--argus-border)',
                      color: camerasSwapped ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
                    }"
                    title="Поменять камеры местами"
                    @mouseenter="($event.currentTarget as HTMLElement).style.background = camerasSwapped ? accentBg(0.15) : 'var(--argus-bg-hover)'"
                    @mouseleave="($event.currentTarget as HTMLElement).style.background = camerasSwapped ? accentBg(0.1) : 'transparent'"
                    @click="swapCameras"
                  >
                    <UIcon name="i-lucide-arrow-left-right" class="size-3.5" />
                    Поменять камеры
                  </button>
                </div>
              </div>

              <!-- Right: AI Event Timeline (2 cols) -->
              <div class="lg:col-span-2 border-l flex flex-col" style="border-color: var(--argus-border);">
                <div class="px-5 py-4 border-b shrink-0" style="border-color: var(--argus-border);">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <UIcon name="i-lucide-activity" class="size-4" style="color: var(--argus-text-dimmed);" />
                      <h3 class="text-sm font-semibold" style="color: var(--argus-text);">AI Хронология</h3>
                    </div>
                    <span class="text-[10px] font-medium px-2 py-0.5 rounded-full" :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }">
                      {{ session.events.length }} событий
                    </span>
                  </div>
                </div>

                <!-- Session Info Summary -->
                <div class="px-5 py-3 border-b grid grid-cols-3 gap-3" style="border-color: var(--argus-border);">
                  <div>
                    <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Начало</p>
                    <p class="text-xs font-bold mt-0.5" style="color: var(--argus-text);">{{ formatTimeShort(session.startedAt) }}</p>
                  </div>
                  <div>
                    <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Нарушения</p>
                    <p class="text-xs font-bold mt-0.5" :style="{ color: session.violationCount > 3 ? 'var(--argus-error)' : 'var(--argus-text)' }">{{ session.violationCount }}</p>
                  </div>
                  <div>
                    <p class="text-[9px] font-medium uppercase tracking-wider" style="color: var(--argus-text-dimmed);">Честность</p>
                    <div class="flex items-center gap-1.5 mt-0.5">
                      <p class="text-xs font-bold" :style="{ color: integrityColor(session.integrityScore) }">{{ session.integrityScore }}%</p>
                      <div class="flex-1 h-1 rounded-full overflow-hidden" style="background: var(--argus-bg-hover);">
                        <div
                          class="h-full rounded-full"
                          :style="{ width: `${session.integrityScore}%`, background: integrityGradientLocal(session.integrityScore) }"
                        />
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Timeline -->
                <div class="flex-1 overflow-y-auto">
                  <div v-if="session.events.length > 0" class="px-5 py-3 space-y-0">
                    <div
                      v-for="(event, idx) in session.events"
                      :key="event.id"
                      class="relative flex gap-3 pb-4"
                    >
                      <div class="flex flex-col items-center shrink-0">
                        <div
                          class="flex items-center justify-center size-7 rounded-full border"
                          :style="{
                            background: severityBgFn(event.severity, 0.1),
                            borderColor: severityBgFn(event.severity, 0.3),
                            color: severityColor(event.severity)
                          }"
                        >
                          <UIcon :name="eventIcon(event.type)" class="size-3.5" />
                        </div>
                        <div
                          v-if="idx < session.events.length - 1"
                          class="w-px flex-1 mt-1"
                          style="background: var(--argus-border);"
                        />
                      </div>
                      <div class="flex-1 min-w-0 pt-0.5">
                        <div class="flex items-start justify-between gap-2">
                          <p class="text-xs font-medium" style="color: var(--argus-text);">{{ event.label }}</p>
                          <span
                            class="text-[9px] font-bold px-1.5 py-0.5 rounded-full shrink-0 uppercase"
                            :style="{ background: severityBgFn(event.severity, 0.1), color: severityColor(event.severity) }"
                          >
                            {{ event.severity === 'critical' ? 'КРИТ' : event.severity === 'warning' ? 'ВНИМАНИЕ' : 'ИНФО' }}
                          </span>
                        </div>
                        <p class="text-[10px] mt-0.5" style="color: var(--argus-text-dimmed);">{{ formatTime(event.timestamp) }}</p>
                        <div
                          v-if="event.type === 'audio_anomaly' || event.type === 'background_voices' || event.type === 'whispering'"
                          class="flex items-center gap-1 mt-1"
                        >
                          <UIcon name="i-lucide-audio-waveform" class="size-2.5" style="color: var(--argus-accent);" />
                          <span class="text-[8px] font-medium" style="color: var(--argus-accent);">Аудио событие</span>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div v-else class="flex flex-col items-center justify-center py-12 px-4">
                    <UIcon name="i-lucide-check-circle" class="size-10 mb-3" style="color: var(--argus-success);" />
                    <p class="text-sm font-medium" style="color: var(--argus-text);">Нет AI-событий</p>
                    <p class="text-xs mt-1 text-center" style="color: var(--argus-text-dimmed);">Сессия проходит без нарушений</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ===== TERMINATE CONFIRMATION MODAL ===== -->
    <Transition name="modal">
      <div
        v-if="showTerminateConfirm"
        class="fixed inset-0 z-[200] flex items-center justify-center p-4"
      >
        <div
          class="absolute inset-0"
          :style="{ background: isDark ? 'rgba(0, 0, 0, 0.6)' : 'rgba(0, 0, 0, 0.35)' }"
          @click="cancelTerminate"
        />
        <div
          class="relative w-full max-w-md rounded-2xl border overflow-hidden z-10"
          :style="{ background: 'var(--argus-bg-card)', borderColor: errorBg(0.3) }"
        >
          <div class="p-6 pb-0">
            <div class="flex items-center gap-3 mb-4">
              <div
                class="flex items-center justify-center size-10 rounded-full"
                :style="{ background: errorBg(0.1) }"
              >
                <UIcon name="i-lucide-octagon-x" class="size-5" style="color: var(--argus-error);" />
              </div>
              <div>
                <h3 class="text-base font-bold" style="color: var(--argus-text);">Завершить экзамен?</h3>
                <p v-if="session" class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
                  Студент: {{ session.studentName }}
                </p>
              </div>
            </div>

            <p class="text-sm leading-relaxed" style="color: var(--argus-text-muted);">
              Вы уверены, что хотите принудительно завершить экзамен для этого студента? Это действие нельзя отменить.
            </p>
          </div>

          <div class="flex items-center justify-end gap-3 p-6">
            <button
              class="px-4 py-2.5 rounded-lg text-sm font-medium transition-all border"
              :style="{ background: 'transparent', borderColor: 'var(--argus-border)', color: 'var(--argus-text-muted)' }"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              @click="cancelTerminate"
            >
              Отмена
            </button>
            <button
              class="px-4 py-2.5 rounded-lg text-sm font-semibold transition-all"
              :style="{ background: errorBg(0.15), color: 'var(--argus-error)', border: `1px solid ${errorBg(0.3)}` }"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.25)"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = errorBg(0.15)"
              @click="confirmTerminate"
            >
              Да, завершить
            </button>
          </div>
        </div>
      </div>
    </Transition>

    <!-- ===== STUDENT WARNING OVERLAY ===== -->
    <Transition name="modal">
      <div
        v-if="showStudentWarning"
        class="fixed inset-0 z-[200] flex items-center justify-center p-4"
      >
        <div
          class="absolute inset-0"
          :style="{ background: isDark ? 'rgba(0, 0, 0, 0.75)' : 'rgba(0, 0, 0, 0.5)' }"
          @click="dismissStudentWarning"
        />
        <div
          class="relative w-full max-w-lg rounded-2xl overflow-hidden z-10"
          :style="{
            background: isDark ? 'rgba(17, 24, 34, 0.95)' : 'rgba(255, 255, 255, 0.97)',
            border: `2px solid var(--argus-warning)`,
            boxShadow: isDark ? '0 0 40px rgba(251, 191, 36, 0.15), 0 0 80px rgba(251, 191, 36, 0.05)' : '0 0 30px rgba(230, 126, 34, 0.12)'
          }"
        >
          <div
            class="flex items-center gap-2 px-6 py-3"
            :style="{ background: warningBg(0.15) }"
          >
            <UIcon name="i-lucide-alert-triangle" class="size-5" style="color: var(--argus-warning);" />
            <span class="text-sm font-bold uppercase tracking-wider" style="color: var(--argus-warning);">
              Предупреждение проктора
            </span>
            <span class="text-[9px] font-medium ml-auto px-2 py-0.5 rounded-full" :style="{ background: warningBg(0.15), color: 'var(--argus-warning)' }">
              Вид студента
            </span>
          </div>

          <div class="p-6 space-y-5">
            <div class="flex items-start gap-4">
              <div
                class="flex items-center justify-center size-14 rounded-xl shrink-0"
                :style="{ background: warningBg(0.1) }"
              >
                <UIcon name="i-lucide-shield-alert" class="size-7" style="color: var(--argus-warning);" />
              </div>
              <div class="flex-1">
                <h3 class="text-lg font-bold" style="color: var(--argus-text);">
                  ВНИМАНИЕ!
                </h3>
                <p class="text-sm mt-2 leading-relaxed" style="color: var(--argus-text-muted);">
                  Зафиксировано нарушение: <strong :style="{ color: 'var(--argus-warning)' }">{{ warningViolationType }}</strong>.
                  Пожалуйста, вернитесь к правилам экзамена.
                </p>
              </div>
            </div>

            <div
              class="rounded-lg p-3 text-xs leading-relaxed"
              :style="{ background: warningBg(0.05), color: 'var(--argus-text-dimmed)', border: `1px solid ${warningBg(0.15)}` }"
            >
              Данное предупреждение зафиксировано системой прокторинга. Повторные нарушения могут привести к принудительному завершению экзамена.
            </div>

            <div class="flex justify-center pt-1">
              <button
                class="px-6 py-3 rounded-xl text-sm font-bold transition-all"
                :style="{ background: warningBg(0.12), color: 'var(--argus-warning)', border: `1px solid ${warningBg(0.25)}` }"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = warningBg(0.2)"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = warningBg(0.12)"
                @click="dismissStudentWarning"
              >
                Я понял(а)
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
