<script setup lang="ts">
// =============================================================================
// PreExamCheck — Production-Grade Hard Gate Modal
// =============================================================================
//
// Blocks exam content until 4 real system checks pass:
//   1. Media Permissions  — getUserMedia({ video, audio })
//   2. Face Enrollment    — useVisionEngine quality gate + canvas capture
//   3. Storage Audit      — navigator.storage.estimate() ≥ 500 MB
//   4. Network Probe      — 3× /healthz ping, RTT + jitter measurement
//
// This modal is UNCLOSABLE by design (no X, no escape, no backdrop click).
// The "Start Exam" button is disabled until all checks pass.
//
// After verification, emits 'verified' with the reference face Blob and the
// live MediaStream, which the parent reuses for the FloatingCamera PIP.
// =============================================================================

import { ref, computed, watch, onMounted, onUnmounted, shallowRef } from 'vue'
import { useVisionEngine } from '~/composables/useVisionEngine'
import { registerDebugVisionEngine } from '~/composables/useDebugBridge'
import type { VisionFrame } from '~/composables/useVisionEngine'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type PreExamPhase = 'idle' | 'media' | 'face' | 'storage' | 'network' | 'ready' | 'transitioning' | 'done'
type CheckStatus = 'pending' | 'running' | 'passed' | 'warning' | 'failed'

interface CheckResult {
  status: CheckStatus
  label: string
  detail: string
  icon: string
  errorInfo?: { title: string; instructions: string[] }
}

interface NetworkProbeResult {
  avgRttMs: number
  jitter: number
  packetLoss: number
  mode: 'online' | 'degraded' | 'offline'
}

interface FaceCapture {
  blob: Blob
  dataUrl: string
  frameQuality: number
  capturedAt: number
}

// ---------------------------------------------------------------------------
// Props / Emits
// ---------------------------------------------------------------------------

defineProps<{
  sessionId: string
  examId: string
  orgId: string
  studentId: string
}>()

const emit = defineEmits<{
  (e: 'verified', payload: {
    referenceFace: Blob
    mediaStream: MediaStream
    networkMode: 'online' | 'degraded' | 'offline'
  }): void
}>()

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

const phase = ref<PreExamPhase>('idle')
const mediaStream = shallowRef<MediaStream | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)
const faceCapture = ref<FaceCapture | null>(null)
const networkResult = ref<NetworkProbeResult | null>(null)
const faceStableSeconds = ref(0)
const isTransitioning = ref(false)

const FACE_STABLE_THRESHOLD = 2 // seconds of consistent good quality
const STORAGE_MIN_MB = 500

// Check results
const mediaCheck = ref<CheckResult>({
  status: 'pending', label: 'Камера и микрофон', detail: 'Ожидание', icon: 'i-lucide-video'
})
const faceCheck = ref<CheckResult>({
  status: 'pending', label: 'Регистрация лица', detail: 'Ожидание', icon: 'i-lucide-scan-face'
})
const storageCheck = ref<CheckResult>({
  status: 'pending', label: 'Хранилище', detail: 'Ожидание', icon: 'i-lucide-hard-drive'
})
const networkCheck = ref<CheckResult>({
  status: 'pending', label: 'Сеть', detail: 'Ожидание', icon: 'i-lucide-wifi'
})

const checks = computed(() => [mediaCheck.value, faceCheck.value, storageCheck.value, networkCheck.value])

const allPassed = computed(() =>
  mediaCheck.value.status === 'passed' &&
  faceCheck.value.status === 'passed' &&
  storageCheck.value.status === 'passed' &&
  (networkCheck.value.status === 'passed' || networkCheck.value.status === 'warning')
)

const canStartExam = computed(() => allPassed.value && phase.value === 'ready')
const showModal = computed(() => phase.value !== 'done')

// Current error (first failed check)
const currentError = computed(() => {
  for (const c of checks.value) {
    if (c.status === 'failed' && c.errorInfo) return c.errorInfo
  }
  return null
})

// Phase badge
const phaseLabel = computed(() => {
  switch (phase.value) {
    case 'idle': return 'Подготовка'
    case 'media': return 'Шаг 1/4'
    case 'face': return 'Шаг 2/4'
    case 'storage': return 'Шаг 3/4'
    case 'network': return 'Шаг 4/4'
    case 'ready': return 'Готово'
    case 'transitioning': return 'Запуск...'
    default: return ''
  }
})

// Vision engine
const visionEngine = useVisionEngine({
  videoElement: videoRef,
  maxFaces: 1,
  inferenceHz: 10
})

// Register with debug bridge for PerformanceDebugger overlay
registerDebugVisionEngine(visionEngine)

// ---------------------------------------------------------------------------
// Step 1: Media Permissions
// ---------------------------------------------------------------------------

async function runMediaCheck(): Promise<void> {
  phase.value = 'media'
  mediaCheck.value = {
    status: 'running', label: 'Камера и микрофон',
    detail: 'Запрос разрешений...', icon: 'i-lucide-video'
  }

  try {
    const stream = await navigator.mediaDevices.getUserMedia({
      video: {
        width: { ideal: 1280 },
        height: { ideal: 720 },
        facingMode: 'user'
      },
      audio: true
    })

    mediaStream.value = stream

    if (videoRef.value) {
      videoRef.value.srcObject = stream
      await videoRef.value.play()
    }

    const videoTrack = stream.getVideoTracks()[0]
    const trackLabel = videoTrack ? videoTrack.label : 'Camera'
    mediaCheck.value = {
      status: 'passed', label: 'Камера и микрофон',
      detail: trackLabel, icon: 'i-lucide-video'
    }

    // Proceed to face enrollment
    await runFaceEnrollment()
  } catch (err: unknown) {
    const domErr = err as DOMException | undefined
    const errorName = domErr?.name ?? 'Unknown'
    mediaCheck.value = {
      status: 'failed', label: 'Камера и микрофон',
      detail: 'Доступ запрещён', icon: 'i-lucide-video-off',
      errorInfo: {
        title: errorName === 'NotFoundError'
          ? 'Камера не найдена'
          : 'Камера обязательна для экзамена',
        instructions: getBrowserPermissionInstructions(errorName)
      }
    }
  }
}

function getBrowserPermissionInstructions(errorName: string): string[] {
  if (errorName === 'NotFoundError') {
    return [
      'Подключите веб-камеру к компьютеру',
      'Убедитесь, что камера не используется другим приложением',
      'Перезагрузите страницу и повторите попытку'
    ]
  }

  const ua = navigator.userAgent
  if (ua.includes('Chrome') && !ua.includes('Edg')) {
    return [
      'Нажмите на иконку замка слева от адресной строки',
      'Выберите "Настройки сайтов"',
      'Установите "Камера" и "Микрофон" → "Разрешить"',
      'Обновите страницу'
    ]
  }
  if (ua.includes('Firefox')) {
    return [
      'Нажмите на иконку щита слева от адресной строки',
      'Нажмите "Разрешения" → "Камера" → "Разрешить"',
      'Также разрешите "Микрофон"',
      'Обновите страницу'
    ]
  }
  if (ua.includes('Safari') && !ua.includes('Chrome')) {
    return [
      'Откройте Safari → Настройки → Веб-сайты',
      'Выберите "Камера" и "Микрофон"',
      'Установите "Разрешить" для этого сайта',
      'Обновите страницу'
    ]
  }
  if (ua.includes('Edg')) {
    return [
      'Нажмите на иконку замка слева от адресной строки',
      'Нажмите "Разрешения для этого сайта"',
      'Установите "Камера" и "Микрофон" → "Разрешить"',
      'Обновите страницу'
    ]
  }
  return [
    'Разрешите доступ к камере и микрофону в настройках браузера',
    'Обновите страницу и повторите попытку'
  ]
}

// ---------------------------------------------------------------------------
// Step 2: Face Enrollment
// ---------------------------------------------------------------------------

async function runFaceEnrollment(): Promise<void> {
  phase.value = 'face'
  faceCheck.value = {
    status: 'running', label: 'Регистрация лица',
    detail: 'Загрузка AI модели...', icon: 'i-lucide-scan-face'
  }

  const started = await visionEngine.start()
  if (!started) {
    faceCheck.value = {
      status: 'failed', label: 'Регистрация лица',
      detail: 'Ошибка загрузки модели', icon: 'i-lucide-scan-face',
      errorInfo: {
        title: 'Не удалось загрузить AI модель',
        instructions: [
          'Проверьте подключение к интернету',
          'Попробуйте обновить страницу',
          'Убедитесь, что браузер поддерживает WebGL'
        ]
      }
    }
    return
  }

  faceCheck.value.detail = 'Разместите лицо в овале'
}

let goodFrameStart: number | null = null

watch(() => visionEngine.currentFrame.value, (frame: VisionFrame | null) => {
  if (phase.value !== 'face' || !frame || faceCapture.value) return

  const singleFace = frame.faceCount === 1
  const qualityOk = frame.frameQuality >= 0.7
  const bbox = frame.faceBBox
  const centered = bbox
    ? Math.abs((bbox.x + bbox.w / 2) - 0.5) < 0.2 &&
      Math.abs((bbox.y + bbox.h / 2) - 0.5) < 0.2
    : false

  if (singleFace && qualityOk && centered) {
    if (!goodFrameStart) goodFrameStart = Date.now()
    faceStableSeconds.value = (Date.now() - goodFrameStart) / 1000

    if (faceStableSeconds.value >= FACE_STABLE_THRESHOLD) {
      void captureReferenceFace()
    } else {
      const remaining = Math.ceil(FACE_STABLE_THRESHOLD - faceStableSeconds.value)
      faceCheck.value.detail = `Удерживайте... ${remaining}с`
    }
  } else {
    goodFrameStart = null
    faceStableSeconds.value = 0

    // Dynamic guidance
    if (frame.faceCount === 0) {
      faceCheck.value.detail = 'Лицо не обнаружено'
    } else if (frame.faceCount > 1) {
      faceCheck.value.detail = 'Обнаружено несколько лиц — вы должны быть одни'
    } else if (!centered) {
      faceCheck.value.detail = 'Центрируйте лицо в овале'
    } else if (!qualityOk) {
      faceCheck.value.detail = 'Улучшите освещение'
    }
  }
})

// Oval guide color
const faceOvalColor = computed(() => {
  if (faceCapture.value) return 'var(--argus-success, #34D399)'
  const frame = visionEngine.currentFrame.value
  if (!frame || frame.faceCount === 0) return 'var(--argus-error, #F87171)'
  if (faceStableSeconds.value > 0) return 'var(--argus-success, #34D399)'
  if (frame.faceCount === 1) return 'var(--argus-warning, #FBBF24)'
  return 'var(--argus-error, #F87171)'
})

async function captureReferenceFace(): Promise<void> {
  const video = videoRef.value
  if (!video || faceCapture.value) return

  const canvas = document.createElement('canvas')
  canvas.width = video.videoWidth
  canvas.height = video.videoHeight
  const ctx = canvas.getContext('2d')
  if (!ctx) return

  ctx.drawImage(video, 0, 0, canvas.width, canvas.height)

  const blob = await new Promise<Blob | null>((resolve) => {
    canvas.toBlob(resolve, 'image/jpeg', 0.92)
  })
  if (!blob) return

  const dataUrl = canvas.toDataURL('image/jpeg', 0.92)

  faceCapture.value = {
    blob,
    dataUrl,
    frameQuality: visionEngine.currentFrame.value?.frameQuality ?? 0,
    capturedAt: Date.now()
  }

  faceCheck.value = {
    status: 'passed', label: 'Регистрация лица',
    detail: `Качество: ${Math.round(faceCapture.value.frameQuality * 100)}%`,
    icon: 'i-lucide-scan-face'
  }

  // Stop inference to save CPU; video stream stays live for PIP
  visionEngine.stop()

  // Proceed to storage
  await runStorageAudit()
}

// ---------------------------------------------------------------------------
// Step 3: Storage Audit
// ---------------------------------------------------------------------------

async function runStorageAudit(): Promise<void> {
  phase.value = 'storage'
  storageCheck.value = {
    status: 'running', label: 'Хранилище',
    detail: 'Проверка...', icon: 'i-lucide-hard-drive'
  }

  try {
    const estimate = await navigator.storage.estimate()
    const quota = estimate.quota ?? 0
    const usage = estimate.usage ?? 0
    const availableMB = Math.round((quota - usage) / (1024 * 1024))

    // Request persistent storage (ADR-007)
    let persistent = false
    try {
      persistent = await navigator.storage.persist()
    } catch {
      // persist() not available in all browsers — graceful degradation
    }

    if (availableMB >= STORAGE_MIN_MB) {
      storageCheck.value = {
        status: 'passed', label: 'Хранилище',
        detail: `${availableMB} МБ свободно${persistent ? ' · постоянное' : ''}`,
        icon: 'i-lucide-hard-drive'
      }
      // Proceed to network
      await runNetworkProbe()
    } else {
      storageCheck.value = {
        status: 'failed', label: 'Хранилище',
        detail: `${availableMB} МБ — недостаточно`,
        icon: 'i-lucide-hard-drive',
        errorInfo: {
          title: 'Недостаточно места на диске',
          instructions: [
            'Закройте лишние вкладки браузера',
            'Удалите ненужные загрузки',
            'Очистите кэш других сайтов (Настройки → Конфиденциальность → Очистить данные)',
            `Необходимо минимум ${STORAGE_MIN_MB} МБ свободного места`
          ]
        }
      }
    }
  } catch {
    // API unavailable — allow with warning
    storageCheck.value = {
      status: 'warning', label: 'Хранилище',
      detail: 'Не удалось проверить — продолжаем', icon: 'i-lucide-hard-drive'
    }
    await runNetworkProbe()
  }
}

// ---------------------------------------------------------------------------
// Step 4: Network Probe
// ---------------------------------------------------------------------------

async function runNetworkProbe(): Promise<void> {
  phase.value = 'network'
  networkCheck.value = {
    status: 'running', label: 'Сеть',
    detail: 'Ping 1/3...', icon: 'i-lucide-wifi'
  }

  const config = useRuntimeConfig()
  const baseUrl = (config.public.apiBaseUrl as string) || ''
  const healthUrl = baseUrl ? `${baseUrl}/healthz` : '/healthz'

  const rtts: number[] = []
  let failures = 0

  for (let i = 0; i < 3; i++) {
    networkCheck.value.detail = `Ping ${i + 1}/3...`
    try {
      const controller = new AbortController()
      const timeoutId = setTimeout(() => controller.abort(), 5000)
      const start = performance.now()

      await fetch(healthUrl, {
        method: 'GET',
        signal: controller.signal,
        cache: 'no-store'
      })

      clearTimeout(timeoutId)
      rtts.push(performance.now() - start)
    } catch {
      failures++
    }

    if (i < 2) await new Promise<void>(r => setTimeout(r, 1000))
  }

  const validRtts = rtts.filter(r => r > 0)
  const avgRtt = validRtts.length > 0
    ? validRtts.reduce((a, b) => a + b, 0) / validRtts.length
    : Infinity
  const jitter = validRtts.length >= 2
    ? Math.max(...validRtts) - Math.min(...validRtts)
    : 0

  if (failures === 3) {
    networkResult.value = { avgRttMs: Infinity, jitter: 0, packetLoss: 1, mode: 'offline' }
    networkCheck.value = {
      status: 'warning', label: 'Сеть',
      detail: 'Оффлайн — данные синхронизируются позже',
      icon: 'i-lucide-wifi-off'
    }
  } else if (avgRtt > 200 || jitter > 100) {
    networkResult.value = { avgRttMs: avgRtt, jitter, packetLoss: failures / 3, mode: 'degraded' }
    networkCheck.value = {
      status: 'warning', label: 'Сеть',
      detail: `RTT: ${Math.round(avgRtt)}мс · Jitter: ${Math.round(jitter)}мс — режим оффлайн-синхронизации`,
      icon: 'i-lucide-wifi'
    }
  } else {
    networkResult.value = { avgRttMs: avgRtt, jitter, packetLoss: failures / 3, mode: 'online' }
    networkCheck.value = {
      status: 'passed', label: 'Сеть',
      detail: `RTT: ${Math.round(avgRtt)}мс — онлайн`,
      icon: 'i-lucide-wifi'
    }
  }

  phase.value = 'ready'
}

// ---------------------------------------------------------------------------
// Retry Handler
// ---------------------------------------------------------------------------

async function retryFailedCheck(): Promise<void> {
  if (mediaCheck.value.status === 'failed') {
    await runMediaCheck()
  } else if (faceCheck.value.status === 'failed') {
    await runFaceEnrollment()
  } else if (storageCheck.value.status === 'failed') {
    await runStorageAudit()
  }
}

// ---------------------------------------------------------------------------
// Start Exam
// ---------------------------------------------------------------------------

async function handleStartExam(): Promise<void> {
  if (!canStartExam.value || !faceCapture.value || !mediaStream.value) return

  phase.value = 'transitioning'
  isTransitioning.value = true

  await new Promise<void>(r => setTimeout(r, 600))

  phase.value = 'done'

  emit('verified', {
    referenceFace: faceCapture.value.blob,
    mediaStream: mediaStream.value,
    networkMode: networkResult.value?.mode ?? 'offline'
  })
}

// ---------------------------------------------------------------------------
// Check Status Helpers
// ---------------------------------------------------------------------------

function statusColor(status: CheckStatus): string {
  switch (status) {
    case 'pending': return 'var(--argus-text-dimmed, #64748B)'
    case 'running': return 'var(--argus-accent, #38BDF8)'
    case 'passed': return 'var(--argus-success, #34D399)'
    case 'warning': return 'var(--argus-warning, #FBBF24)'
    case 'failed': return 'var(--argus-error, #F87171)'
  }
}

function statusIcon(status: CheckStatus): string {
  switch (status) {
    case 'pending': return 'i-lucide-circle'
    case 'running': return 'i-lucide-loader-2'
    case 'passed': return 'i-lucide-check-circle'
    case 'warning': return 'i-lucide-alert-triangle'
    case 'failed': return 'i-lucide-x-circle'
  }
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

onMounted(() => {
  void runMediaCheck()
})

onUnmounted(() => {
  visionEngine.stop()
  // Do NOT stop MediaStream — it's passed to the parent for FloatingCamera
})
</script>

<template>
  <Teleport to="body">
    <Transition name="preexam">
      <div
        v-if="showModal"
        class="fixed inset-0 z-[250] flex items-center justify-center p-4"
        style="background: rgba(0, 0, 0, 0.88)"
      >
        <!-- NO backdrop click, NO X button, NO Escape — hard gate -->

        <div
          class="relative w-full max-w-2xl rounded-2xl border overflow-hidden transition-all duration-500"
          :class="{ 'scale-0 opacity-0': isTransitioning }"
          :style="{
            background: 'var(--argus-bg-card, #111822)',
            borderColor: allPassed ? 'var(--argus-success, #34D399)' : 'var(--argus-border, #1E293B)'
          }"
        >
          <!-- ========== HEADER ========== -->
          <div
            class="px-6 py-4 flex items-center gap-3"
            :style="{
              background: allPassed
                ? 'rgba(52, 211, 153, 0.06)'
                : 'rgba(56, 189, 248, 0.06)'
            }"
          >
            <div
              class="flex items-center justify-center size-10 rounded-xl"
              :style="{
                background: allPassed
                  ? 'rgba(52, 211, 153, 0.12)'
                  : 'rgba(56, 189, 248, 0.12)'
              }"
            >
              <UIcon
                name="i-lucide-shield-check"
                class="size-5"
                :style="{ color: allPassed ? 'var(--argus-success)' : 'var(--argus-accent)' }"
              />
            </div>
            <div class="flex-1 min-w-0">
              <h2
                class="text-lg font-bold"
                style="color: var(--argus-text, #E2E8F0)"
              >
                Проверка перед экзаменом
              </h2>
              <p
                class="text-xs mt-0.5"
                style="color: var(--argus-text-dimmed, #64748B)"
              >
                Все проверки должны быть пройдены
              </p>
            </div>
            <div
              class="text-[10px] font-bold px-2.5 py-1 rounded-full whitespace-nowrap"
              :style="{
                background: allPassed
                  ? 'rgba(52, 211, 153, 0.15)'
                  : 'rgba(56, 189, 248, 0.15)',
                color: allPassed
                  ? 'var(--argus-success)'
                  : 'var(--argus-accent)'
              }"
            >
              {{ phaseLabel }}
            </div>
          </div>

          <!-- ========== CONTENT: 2-column ========== -->
          <div class="grid grid-cols-1 md:grid-cols-2 gap-0">

            <!-- LEFT: Check List -->
            <div
              class="p-5 space-y-2.5 md:border-r"
              style="border-color: var(--argus-border-subtle, #162032)"
            >
              <div
                v-for="check in checks"
                :key="check.label"
                class="flex items-center gap-3 px-3 py-2.5 rounded-xl transition-colors duration-200"
                :style="{
                  background: check.status === 'running'
                    ? 'rgba(56, 189, 248, 0.04)'
                    : check.status === 'failed'
                      ? 'rgba(248, 113, 113, 0.04)'
                      : 'transparent'
                }"
              >
                <!-- Status icon -->
                <div
                  class="flex items-center justify-center size-8 rounded-lg shrink-0"
                  :style="{
                    background: `color-mix(in srgb, ${statusColor(check.status)} 12%, transparent)`
                  }"
                >
                  <UIcon
                    :name="statusIcon(check.status)"
                    class="size-4"
                    :class="{ 'animate-spin': check.status === 'running' }"
                    :style="{ color: statusColor(check.status) }"
                  />
                </div>
                <!-- Label + detail -->
                <div class="flex-1 min-w-0">
                  <p
                    class="text-sm font-semibold"
                    :style="{ color: statusColor(check.status) }"
                  >
                    {{ check.label }}
                  </p>
                  <p
                    class="text-[11px] mt-0.5 truncate"
                    style="color: var(--argus-text-dimmed, #64748B)"
                  >
                    {{ check.detail }}
                  </p>
                </div>
                <!-- Check icon -->
                <UIcon
                  :name="check.icon"
                  class="size-4 shrink-0"
                  :style="{ color: statusColor(check.status) }"
                />
              </div>
            </div>

            <!-- RIGHT: Live Preview / Context -->
            <div class="p-5 flex flex-col items-center justify-center min-h-[300px]">

              <!-- Phase: media — permission prompt -->
              <div v-if="phase === 'media' && mediaCheck.status !== 'failed'" class="text-center">
                <div
                  class="size-20 mx-auto rounded-2xl flex items-center justify-center mb-4"
                  style="background: rgba(56, 189, 248, 0.08)"
                >
                  <UIcon name="i-lucide-video" class="size-10" style="color: var(--argus-accent)" />
                </div>
                <p class="text-sm" style="color: var(--argus-text-muted, #94A3B8)">
                  Разрешите доступ к камере и микрофону
                </p>
                <p class="text-xs mt-2" style="color: var(--argus-text-dimmed)">
                  Браузер запросит разрешение
                </p>
              </div>

              <!-- Phase: face — live camera with oval guide -->
              <div
                v-else-if="phase === 'face' || (phase !== 'media' && phase !== 'idle' && videoRef)"
                class="relative w-full aspect-[4/3] rounded-xl overflow-hidden"
                style="background: var(--argus-bg-deep, #0B0F14)"
              >
                <video
                  ref="videoRef"
                  autoplay
                  playsinline
                  muted
                  class="w-full h-full object-cover mirror"
                />
                <!-- Oval face guide (only during face phase) -->
                <svg
                  v-if="phase === 'face'"
                  class="absolute inset-0 w-full h-full pointer-events-none"
                  viewBox="0 0 400 300"
                  preserveAspectRatio="xMidYMid slice"
                >
                  <defs>
                    <mask id="face-oval-mask">
                      <rect width="400" height="300" fill="white" />
                      <ellipse cx="200" cy="140" rx="80" ry="105" fill="black" />
                    </mask>
                  </defs>
                  <rect
                    width="400" height="300"
                    fill="rgba(0,0,0,0.45)"
                    mask="url(#face-oval-mask)"
                  />
                  <ellipse
                    cx="200" cy="140" rx="80" ry="105"
                    fill="none"
                    :stroke="faceOvalColor"
                    stroke-width="2.5"
                    stroke-dasharray="8 4"
                    :class="{ 'oval-pulse': faceStableSeconds > 0 }"
                  />
                </svg>
                <!-- Stable progress bar -->
                <div
                  v-if="phase === 'face' && faceStableSeconds > 0 && !faceCapture"
                  class="absolute bottom-3 left-4 right-4"
                >
                  <div
                    class="h-1.5 rounded-full overflow-hidden"
                    style="background: rgba(255,255,255,0.1)"
                  >
                    <div
                      class="h-full rounded-full transition-all duration-200"
                      :style="{
                        width: `${Math.min(100, (faceStableSeconds / FACE_STABLE_THRESHOLD) * 100)}%`,
                        background: 'var(--argus-success, #34D399)'
                      }"
                    />
                  </div>
                </div>
                <!-- Captured thumbnail badge -->
                <div
                  v-if="faceCapture"
                  class="absolute bottom-3 right-3 flex items-center gap-2 px-2 py-1 rounded-lg"
                  style="background: rgba(0,0,0,0.6); backdrop-filter: blur(4px)"
                >
                  <img
                    :src="faceCapture.dataUrl"
                    class="size-8 rounded-md border"
                    :style="{ borderColor: 'var(--argus-success)' }"
                    alt="Reference face"
                  >
                  <span class="text-[10px] font-bold" style="color: var(--argus-success)">
                    Сохранено
                  </span>
                </div>
              </div>

              <!-- Phase: storage -->
              <div v-else-if="phase === 'storage'" class="text-center w-full px-4">
                <div
                  class="size-16 mx-auto rounded-2xl flex items-center justify-center mb-3"
                  style="background: rgba(56, 189, 248, 0.08)"
                >
                  <UIcon
                    name="i-lucide-hard-drive"
                    class="size-8"
                    :class="{ 'animate-pulse': storageCheck.status === 'running' }"
                    style="color: var(--argus-accent)"
                  />
                </div>
                <p class="text-sm" style="color: var(--argus-text-muted)">
                  Проверка доступного хранилища
                </p>
              </div>

              <!-- Phase: network -->
              <div v-else-if="phase === 'network'" class="text-center">
                <div class="flex items-center justify-center gap-3 mb-4">
                  <div
                    v-for="i in 3"
                    :key="i"
                    class="size-4 rounded-full transition-all duration-300"
                    :style="{
                      background: networkCheck.detail.includes(`${i}/3`) || networkCheck.detail.includes('Ping')
                        ? 'var(--argus-accent)'
                        : 'rgba(255,255,255,0.1)',
                      transform: networkCheck.detail.includes(`${i}/3`) ? 'scale(1.3)' : 'scale(1)'
                    }"
                  />
                </div>
                <p class="text-sm" style="color: var(--argus-text-muted)">
                  Измерение задержки сети
                </p>
              </div>

              <!-- Phase: ready — summary -->
              <div v-else-if="phase === 'ready'" class="text-center">
                <div v-if="faceCapture" class="mb-4">
                  <img
                    :src="faceCapture.dataUrl"
                    class="size-20 rounded-2xl mx-auto border-2"
                    :style="{ borderColor: 'var(--argus-success)' }"
                    alt="Reference face"
                  >
                </div>
                <p class="text-sm font-semibold" style="color: var(--argus-success)">
                  Все проверки пройдены
                </p>
                <p class="text-xs mt-1" style="color: var(--argus-text-dimmed)">
                  Нажмите «Начать экзамен» для продолжения
                </p>
              </div>

            </div>
          </div>

          <!-- ========== ERROR PANEL ========== -->
          <div
            v-if="currentError"
            class="px-6 py-4 border-t"
            :style="{
              borderColor: 'var(--argus-border, #1E293B)',
              background: 'rgba(248, 113, 113, 0.04)'
            }"
          >
            <div class="flex items-start gap-3">
              <UIcon
                name="i-lucide-alert-triangle"
                class="size-5 shrink-0 mt-0.5"
                style="color: var(--argus-error, #F87171)"
              />
              <div class="flex-1">
                <p class="text-sm font-bold" style="color: var(--argus-error, #F87171)">
                  {{ currentError.title }}
                </p>
                <ol
                  class="list-decimal list-inside mt-2 space-y-1 text-xs"
                  style="color: var(--argus-text-muted, #94A3B8)"
                >
                  <li v-for="(instruction, idx) in currentError.instructions" :key="idx">
                    {{ instruction }}
                  </li>
                </ol>
                <button
                  class="mt-3 inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-bold text-white scanner-btn"
                  @click="retryFailedCheck"
                >
                  <UIcon name="i-lucide-refresh-cw" class="size-3.5" />
                  Повторить проверку
                </button>
              </div>
            </div>
          </div>

          <!-- ========== FOOTER ========== -->
          <div
            class="px-6 py-5 border-t text-center"
            :style="{ borderColor: 'var(--argus-border, #1E293B)' }"
          >
            <button
              class="inline-flex items-center gap-2 px-10 py-3.5 rounded-xl text-sm font-bold text-white transition-all duration-300"
              :class="canStartExam ? 'scanner-btn cursor-pointer' : 'cursor-not-allowed'"
              :disabled="!canStartExam"
              :style="{
                background: canStartExam
                  ? 'linear-gradient(135deg, var(--argus-brand-green, #34A853), var(--argus-brand-blue, #4285F4))'
                  : 'rgba(255,255,255,0.06)',
                opacity: canStartExam ? 1 : 0.4,
                boxShadow: canStartExam ? '0 4px 20px rgba(52, 211, 153, 0.25)' : 'none'
              }"
              @click="handleStartExam"
            >
              <UIcon name="i-lucide-play" class="size-4" />
              Начать экзамен
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.mirror {
  transform: scaleX(-1);
}

.preexam-enter-active {
  transition: opacity 0.3s ease, transform 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.preexam-leave-active {
  transition: opacity 0.5s ease, transform 0.5s cubic-bezier(0.4, 0, 0.2, 1);
}
.preexam-enter-from {
  opacity: 0;
  transform: scale(0.95);
}
.preexam-leave-to {
  opacity: 0;
  transform: scale(0.9);
}

@keyframes oval-glow {
  0%, 100% { opacity: 0.7; }
  50% { opacity: 1; }
}
.oval-pulse {
  animation: oval-glow 1.5s ease-in-out infinite;
}
</style>
