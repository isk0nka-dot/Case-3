<script setup lang="ts">
const colorMode = useColorMode()

// Force dark mode
onMounted(() => {
  colorMode.preference = 'dark'
})

// --- Steps / Navigation ---
type GuideStep = 'system-check' | 'dual-cam' | 'honor-code' | 'tutorial'
const currentStep = ref<GuideStep>('system-check')
const steps: { id: GuideStep; label: string; icon: string; color: string }[] = [
  { id: 'system-check', label: 'Проверка системы', icon: 'i-lucide-monitor-check', color: '#4285F4' },
  { id: 'dual-cam', label: 'Настройка камер', icon: 'i-lucide-smartphone', color: '#A259FF' },
  { id: 'honor-code', label: 'Правила экзамена', icon: 'i-lucide-shield-check', color: '#34A853' },
  { id: 'tutorial', label: 'Пробный запуск', icon: 'i-lucide-play-circle', color: '#FBBC05' }
]

const currentStepIndex = computed(() => steps.findIndex(s => s.id === currentStep.value))
const canGoNext = computed(() => currentStepIndex.value < steps.length - 1)
const canGoPrev = computed(() => currentStepIndex.value > 0)

function goNext() {
  if (canGoNext.value) {
    currentStep.value = steps[currentStepIndex.value + 1]!.id
  }
}

function goPrev() {
  if (canGoPrev.value) {
    currentStep.value = steps[currentStepIndex.value - 1]!.id
  }
}

// --- System Check ---
interface CheckItem {
  id: string
  label: string
  description: string
  icon: string
  status: 'pending' | 'checking' | 'passed' | 'warning' | 'failed'
  detail: string
}

const systemChecks = ref<CheckItem[]>([
  { id: 'browser', label: 'Браузер', description: 'Chrome 90+ или Edge 90+', icon: 'i-lucide-globe', status: 'pending', detail: '' },
  { id: 'webcam', label: 'Веб-камера', description: 'Доступ к камере', icon: 'i-lucide-video', status: 'pending', detail: '' },
  { id: 'microphone', label: 'Микрофон', description: 'Доступ к аудио', icon: 'i-lucide-mic', status: 'pending', detail: '' },
  { id: 'screen', label: 'Разрешение экрана', description: 'Минимум 1280x720', icon: 'i-lucide-monitor', status: 'pending', detail: '' },
  { id: 'internet', label: 'Скорость интернета', description: 'Минимум 52 kb/s', icon: 'i-lucide-wifi', status: 'pending', detail: '' },
  { id: 'extensions', label: 'Расширения браузера', description: 'Проверка конфликтов', icon: 'i-lucide-puzzle', status: 'pending', detail: '' }
])

const isRunningCheck = ref(false)
const allChecksDone = computed(() => systemChecks.value.every(c => c.status !== 'pending' && c.status !== 'checking'))
const allChecksPassed = computed(() => systemChecks.value.every(c => c.status === 'passed'))

async function runSystemCheck() {
  isRunningCheck.value = true

  for (const check of systemChecks.value) {
    check.status = 'checking'
    check.detail = ''
    await delay(800 + Math.random() * 600)

    // Simulate check results
    switch (check.id) {
      case 'browser':
        check.status = 'passed'
        check.detail = 'Google Chrome 121.0.6167'
        break
      case 'webcam':
        check.status = 'passed'
        check.detail = 'HD Webcam (1920x1080 @ 30fps)'
        break
      case 'microphone':
        check.status = 'passed'
        check.detail = 'Встроенный микрофон (44.1kHz)'
        break
      case 'screen':
        check.status = 'passed'
        check.detail = `${window.innerWidth}x${window.innerHeight}`
        break
      case 'internet':
        check.status = 'passed'
        check.detail = '12.4 Mb/s (стабильно)'
        break
      case 'extensions':
        check.status = 'warning'
        check.detail = 'AdBlock обнаружен — рекомендуем отключить'
        break
    }
  }

  isRunningCheck.value = false
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function checkStatusColor(status: string): string {
  switch (status) {
    case 'passed': return '#34A853'
    case 'warning': return '#FBBC05'
    case 'failed': return '#EA4335'
    case 'checking': return '#4285F4'
    default: return 'rgba(255,255,255,0.2)'
  }
}

function checkStatusIcon(status: string): string {
  switch (status) {
    case 'passed': return 'i-lucide-check-circle'
    case 'warning': return 'i-lucide-alert-triangle'
    case 'failed': return 'i-lucide-x-circle'
    case 'checking': return 'i-lucide-loader-2'
    default: return 'i-lucide-circle'
  }
}

// --- Dual Cam Setup ---
const dualCamStep = ref(1) // 1=intro, 2=QR scan, 3=placement
const qrScanning = ref(false)
const qrConnected = ref(false)

function startQrScan() {
  qrScanning.value = true
  setTimeout(() => {
    qrScanning.value = false
    qrConnected.value = true
  }, 3000)
}

// --- Honor Code ---
const honorAgreed = ref(false)
const honorRules = [
  { icon: 'i-lucide-video', text: 'Я понимаю, что мой экзамен будет записываться двумя камерами (веб-камера + боковая).' },
  { icon: 'i-lucide-eye', text: 'Я понимаю, что AI-система будет отслеживать мой взгляд, голос и поведение.' },
  { icon: 'i-lucide-lock', text: 'Я не буду использовать запрещённые устройства, приложения или материалы.' },
  { icon: 'i-lucide-users', text: 'Я буду находиться один(а) в комнате на протяжении всего экзамена.' },
  { icon: 'i-lucide-monitor', text: 'Я не буду переключаться между вкладками и приложениями.' },
  { icon: 'i-lucide-clipboard', text: 'Я не буду копировать, вставлять или делать скриншоты экзаменационных материалов.' },
  { icon: 'i-lucide-shield-check', text: 'Я подтверждаю свою личность и обязуюсь проходить экзамен честно.' }
]

// --- Tutorial / Practice ---
const tutorialPhase = ref<'intro' | 'face-check' | 'gaze-test' | 'audio-test' | 'complete'>('intro')
const tutorialProgress = computed(() => {
  const phases = ['intro', 'face-check', 'gaze-test', 'audio-test', 'complete']
  return (phases.indexOf(tutorialPhase.value) / (phases.length - 1)) * 100
})

const faceDetected = ref(false)
const gazeCalibrated = ref(false)
const audioDetected = ref(false)

function startTutorial() {
  tutorialPhase.value = 'face-check'
  setTimeout(() => { faceDetected.value = true }, 2500)
}

function nextTutorialPhase() {
  switch (tutorialPhase.value) {
    case 'face-check':
      tutorialPhase.value = 'gaze-test'
      setTimeout(() => { gazeCalibrated.value = true }, 3000)
      break
    case 'gaze-test':
      tutorialPhase.value = 'audio-test'
      setTimeout(() => { audioDetected.value = true }, 2000)
      break
    case 'audio-test':
      tutorialPhase.value = 'complete'
      break
  }
}

// --- Audio level bars for tutorial ---
const audioLevels = ref<number[]>(Array.from({ length: 20 }, () => Math.random() * 30 + 5))
let audioInterval: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  audioInterval = setInterval(() => {
    audioLevels.value = Array.from({ length: 20 }, () => Math.random() * 80 + 10)
  }, 200)
})

onUnmounted(() => {
  if (audioInterval) clearInterval(audioInterval)
})

// Hexadecimal to rgba
function hexToRgba(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}
</script>

<template>
  <div class="student-guide min-h-screen" style="background: #121820;">
    <!-- ============================== -->
    <!--  HEADER / NAVIGATION           -->
    <!-- ============================== -->
    <header class="sticky top-0 z-50 px-6 py-4" style="background: rgba(18, 24, 32, 0.9); backdrop-filter: blur(20px); border-bottom: 1px solid rgba(255,255,255,0.06);">
      <div class="max-w-5xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <NuxtLink to="/landing" class="flex items-center gap-2 hover:opacity-80 transition-opacity">
            <ArgusLogo :size="32" />
            <span class="text-base font-bold text-white">Argus AI</span>
          </NuxtLink>
          <div class="w-px h-6 mx-2" style="background: rgba(255,255,255,0.1);" />
          <span class="text-sm font-semibold text-white/60">Руководство студента</span>
        </div>

        <div class="hidden md:flex items-center gap-1">
          <button
            v-for="(step, idx) in steps"
            :key="step.id"
            class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all cursor-pointer"
            :style="{
              background: currentStep === step.id ? hexToRgba(step.color, 0.1) : 'transparent',
              color: currentStep === step.id ? step.color : 'rgba(255,255,255,0.4)',
              border: currentStep === step.id ? `1px solid ${hexToRgba(step.color, 0.2)}` : '1px solid transparent'
            }"
            @click="currentStep = step.id"
          >
            <span class="flex items-center justify-center size-5 rounded-full text-[9px] font-bold" :style="{ background: currentStep === step.id || idx < currentStepIndex ? hexToRgba(step.color, 0.15) : 'rgba(255,255,255,0.06)', color: currentStep === step.id || idx < currentStepIndex ? step.color : 'rgba(255,255,255,0.3)' }">
              {{ idx + 1 }}
            </span>
            <span class="hidden lg:inline">{{ step.label }}</span>
          </button>
        </div>
      </div>
    </header>

    <!-- ============================== -->
    <!--  PROGRESS BAR                  -->
    <!-- ============================== -->
    <div class="w-full h-1" style="background: rgba(255,255,255,0.04);">
      <div
        class="h-full transition-all duration-500 ease-out"
        :style="{
          width: `${((currentStepIndex + 1) / steps.length) * 100}%`,
          background: `linear-gradient(90deg, #4285F4, #A259FF, #34A853, #FBBC05)`
        }"
      />
    </div>

    <!-- ============================== -->
    <!--  MAIN CONTENT                  -->
    <!-- ============================== -->
    <main class="max-w-5xl mx-auto px-6 py-12">

      <!-- ======================================= -->
      <!--  STEP 1: SYSTEM CHECK                   -->
      <!-- ======================================= -->
      <div v-if="currentStep === 'system-check'" class="space-y-8">
        <div class="text-center mb-12">
          <div class="inline-flex items-center justify-center size-16 rounded-2xl mb-6" style="background: rgba(66, 133, 244, 0.08); border: 1px solid rgba(66, 133, 244, 0.15);">
            <UIcon name="i-lucide-monitor-check" class="size-8" style="color: #4285F4;" />
          </div>
          <h1 class="text-3xl md:text-4xl font-bold text-white tracking-tight">
            Проверка системы
          </h1>
          <p class="text-base text-white/40 mt-3 max-w-lg mx-auto">
            Убедитесь, что ваше устройство готово к прокторингу. Автоматическая проверка займёт ~10 секунд.
          </p>
        </div>

        <!-- Check items -->
        <div class="rounded-2xl border overflow-hidden" style="background: #1A2130; border-color: rgba(66, 133, 244, 0.12);">
          <div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: rgba(255,255,255,0.06);">
            <div class="flex items-center gap-3">
              <UIcon name="i-lucide-scan-line" class="size-4" style="color: #4285F4;" />
              <span class="text-sm font-bold text-white">Диагностика оборудования</span>
            </div>
            <span v-if="allChecksDone" class="text-[9px] font-bold px-2 py-0.5 rounded-full" :style="{ background: allChecksPassed ? 'rgba(52,168,83,0.15)' : 'rgba(251,188,5,0.15)', color: allChecksPassed ? '#34A853' : '#FBBC05' }">
              {{ allChecksPassed ? 'ВСЕ ПРОВЕРКИ ПРОЙДЕНЫ' : 'ЕСТЬ ПРЕДУПРЕЖДЕНИЯ' }}
            </span>
          </div>

          <div class="divide-y" style="border-color: rgba(255,255,255,0.04);">
            <div
              v-for="check in systemChecks"
              :key="check.id"
              class="px-6 py-4 flex items-center gap-4 transition-all duration-300"
              style="border-color: rgba(255,255,255,0.04);"
            >
              <!-- Icon -->
              <div class="flex items-center justify-center size-10 rounded-xl shrink-0" :style="{ background: hexToRgba(checkStatusColor(check.status), 0.08), border: `1px solid ${hexToRgba(checkStatusColor(check.status), 0.15)}` }">
                <UIcon :name="check.icon" class="size-5" :style="{ color: checkStatusColor(check.status) }" />
              </div>

              <!-- Text -->
              <div class="flex-1 min-w-0">
                <p class="text-sm font-semibold text-white/80">{{ check.label }}</p>
                <p class="text-[11px] text-white/30 mt-0.5">{{ check.description }}</p>
                <p v-if="check.detail" class="text-[10px] font-mono mt-1" :style="{ color: checkStatusColor(check.status) }">
                  {{ check.detail }}
                </p>
              </div>

              <!-- Status indicator -->
              <div class="shrink-0">
                <UIcon
                  :name="checkStatusIcon(check.status)"
                  class="size-5"
                  :class="{ 'animate-spin': check.status === 'checking' }"
                  :style="{ color: checkStatusColor(check.status) }"
                />
              </div>
            </div>
          </div>

          <!-- Run check button -->
          <div class="px-6 py-4 border-t text-center" style="border-color: rgba(255,255,255,0.06);">
            <button
              v-if="!allChecksDone"
              class="px-8 py-3 rounded-xl text-sm font-bold text-white cursor-pointer transition-all hover:scale-105 disabled:opacity-50 disabled:cursor-not-allowed"
              :disabled="isRunningCheck"
              style="background: linear-gradient(135deg, #4285F4, #34A853); box-shadow: 0 0 20px rgba(66, 133, 244, 0.15);"
              @click="runSystemCheck"
            >
              <span v-if="isRunningCheck" class="flex items-center gap-2">
                <UIcon name="i-lucide-loader-2" class="size-4 animate-spin" />
                Проверка...
              </span>
              <span v-else class="flex items-center gap-2">
                <UIcon name="i-lucide-play" class="size-4" />
                Запустить проверку
              </span>
            </button>
            <div v-else class="flex items-center justify-center gap-3">
              <UIcon name="i-lucide-check-circle" class="size-5" style="color: #34A853;" />
              <span class="text-sm font-semibold text-white/60">Проверка завершена — ваше устройство готово</span>
            </div>
          </div>
        </div>

        <!-- System Requirements info -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
          <div class="p-4 rounded-xl" style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
            <div class="flex items-center gap-2 mb-2">
              <UIcon name="i-lucide-chrome" class="size-4" style="color: #4285F4;" />
              <span class="text-[10px] font-bold text-white/50 uppercase">Браузер</span>
            </div>
            <p class="text-xs text-white/40">Chrome 90+, Edge 90+, Firefox 90+. Рекомендуем Chrome.</p>
          </div>
          <div class="p-4 rounded-xl" style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
            <div class="flex items-center gap-2 mb-2">
              <UIcon name="i-lucide-wifi" class="size-4" style="color: #34A853;" />
              <span class="text-[10px] font-bold text-white/50 uppercase">Интернет</span>
            </div>
            <p class="text-xs text-white/40">Минимум 52 kb/s. Argus AI оптимизирован для медленных сетей.</p>
          </div>
          <div class="p-4 rounded-xl" style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
            <div class="flex items-center gap-2 mb-2">
              <UIcon name="i-lucide-monitor" class="size-4" style="color: #A259FF;" />
              <span class="text-[10px] font-bold text-white/50 uppercase">Экран</span>
            </div>
            <p class="text-xs text-white/40">Разрешение 1280x720 или выше. Планшеты не поддерживаются.</p>
          </div>
        </div>
      </div>

      <!-- ======================================= -->
      <!--  STEP 2: DUAL-CAM SETUP                -->
      <!-- ======================================= -->
      <div v-if="currentStep === 'dual-cam'" class="space-y-8">
        <div class="text-center mb-12">
          <div class="inline-flex items-center justify-center size-16 rounded-2xl mb-6" style="background: rgba(162, 89, 255, 0.08); border: 1px solid rgba(162, 89, 255, 0.15);">
            <UIcon name="i-lucide-smartphone" class="size-8" style="color: #A259FF;" />
          </div>
          <h1 class="text-3xl md:text-4xl font-bold text-white tracking-tight">
            Настройка камер 360°
          </h1>
          <p class="text-base text-white/40 mt-3 max-w-lg mx-auto">
            Подключите мобильный телефон как боковую камеру для полного контроля рабочего пространства.
          </p>
        </div>

        <!-- Sub-steps -->
        <div class="flex items-center justify-center gap-4 mb-8">
          <button
            v-for="s in 3"
            :key="s"
            class="flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-medium cursor-pointer transition-all"
            :style="{
              background: dualCamStep === s ? 'rgba(162, 89, 255, 0.1)' : 'rgba(255,255,255,0.02)',
              border: dualCamStep === s ? '1px solid rgba(162, 89, 255, 0.2)' : '1px solid rgba(255,255,255,0.06)',
              color: dualCamStep === s ? '#A259FF' : 'rgba(255,255,255,0.4)'
            }"
            @click="dualCamStep = s"
          >
            {{ s === 1 ? 'Подготовка' : s === 2 ? 'QR-подключение' : 'Размещение' }}
          </button>
        </div>

        <!-- Sub-step 1: Intro -->
        <div v-if="dualCamStep === 1" class="rounded-2xl border overflow-hidden" style="background: #1A2130; border-color: rgba(162, 89, 255, 0.12);">
          <div class="p-8">
            <h3 class="text-lg font-bold text-white mb-6">Что вам понадобится:</h3>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div class="flex items-start gap-4 p-4 rounded-xl" style="background: rgba(255,255,255,0.02);">
                <div class="flex items-center justify-center size-10 rounded-xl shrink-0" style="background: rgba(162, 89, 255, 0.08);">
                  <UIcon name="i-lucide-smartphone" class="size-5" style="color: #A259FF;" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-white/80">Мобильный телефон</p>
                  <p class="text-xs text-white/40 mt-1">Android 8+ или iOS 14+. С камерой не ниже 720p.</p>
                </div>
              </div>
              <div class="flex items-start gap-4 p-4 rounded-xl" style="background: rgba(255,255,255,0.02);">
                <div class="flex items-center justify-center size-10 rounded-xl shrink-0" style="background: rgba(66, 133, 244, 0.08);">
                  <UIcon name="i-lucide-wifi" class="size-5" style="color: #4285F4;" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-white/80">Wi-Fi подключение</p>
                  <p class="text-xs text-white/40 mt-1">Телефон должен быть подключён к той же сети Wi-Fi или мобильному интернету.</p>
                </div>
              </div>
              <div class="flex items-start gap-4 p-4 rounded-xl" style="background: rgba(255,255,255,0.02);">
                <div class="flex items-center justify-center size-10 rounded-xl shrink-0" style="background: rgba(52, 168, 83, 0.08);">
                  <UIcon name="i-lucide-battery-full" class="size-5" style="color: #34A853;" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-white/80">Заряд батареи 50%+</p>
                  <p class="text-xs text-white/40 mt-1">Или подключите к зарядке. Экзамен может длиться до 3 часов.</p>
                </div>
              </div>
              <div class="flex items-start gap-4 p-4 rounded-xl" style="background: rgba(255,255,255,0.02);">
                <div class="flex items-center justify-center size-10 rounded-xl shrink-0" style="background: rgba(251, 188, 5, 0.08);">
                  <UIcon name="i-lucide-phone-off" class="size-5" style="color: #FBBC05;" />
                </div>
                <div>
                  <p class="text-sm font-semibold text-white/80">Беззвучный режим</p>
                  <p class="text-xs text-white/40 mt-1">Включите режим «Не беспокоить» — звонки и уведомления прервут запись.</p>
                </div>
              </div>
            </div>

            <div class="text-center mt-8">
              <button
                class="px-6 py-3 rounded-xl text-sm font-bold text-white cursor-pointer transition-all hover:scale-105"
                style="background: linear-gradient(135deg, #A259FF, #4285F4);"
                @click="dualCamStep = 2"
              >
                Далее: QR-подключение
              </button>
            </div>
          </div>
        </div>

        <!-- Sub-step 2: QR Code Scan -->
        <div v-if="dualCamStep === 2" class="rounded-2xl border overflow-hidden" style="background: #1A2130; border-color: rgba(162, 89, 255, 0.12);">
          <div class="p-8">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-8">
              <!-- QR Code -->
              <div class="flex flex-col items-center">
                <h3 class="text-lg font-bold text-white mb-4">Отсканируйте QR-код</h3>
                <div
                  class="relative size-56 rounded-2xl flex items-center justify-center mb-4"
                  :style="{
                    background: qrConnected ? 'rgba(52, 168, 83, 0.05)' : '#0D1117',
                    border: qrConnected ? '2px solid rgba(52, 168, 83, 0.3)' : '2px solid rgba(162, 89, 255, 0.2)'
                  }"
                >
                  <div v-if="!qrConnected">
                    <!-- Mock QR code pattern -->
                    <div class="grid grid-cols-7 gap-1">
                      <div
                        v-for="i in 49"
                        :key="i"
                        class="size-5 rounded-sm"
                        :style="{
                          background: [1,2,3,5,6,7,8,14,15,21,22,28,29,35,36,43,44,45,47,48,49,
                            10,11,12,17,18,24,25,31,32,38,39,40,4,13,16,19,23,26,30,34,37,41,46].includes(i)
                            ? '#A259FF'
                            : 'rgba(255,255,255,0.05)'
                        }"
                      />
                    </div>
                    <!-- Scanning animation overlay -->
                    <div
                      v-if="qrScanning"
                      class="absolute inset-0 rounded-2xl overflow-hidden"
                    >
                      <div class="qr-scan-line absolute left-0 right-0 h-0.5" style="background: #A259FF; box-shadow: 0 0 10px #A259FF;" />
                    </div>
                  </div>
                  <div v-else class="flex flex-col items-center gap-3">
                    <UIcon name="i-lucide-check-circle" class="size-16" style="color: #34A853;" />
                    <span class="text-sm font-bold" style="color: #34A853;">Подключено!</span>
                  </div>

                  <!-- Corner brackets -->
                  <div class="absolute top-2 left-2 w-4 h-4 border-l-2 border-t-2 rounded-tl" style="border-color: rgba(162, 89, 255, 0.4);" />
                  <div class="absolute top-2 right-2 w-4 h-4 border-r-2 border-t-2 rounded-tr" style="border-color: rgba(162, 89, 255, 0.4);" />
                  <div class="absolute bottom-2 left-2 w-4 h-4 border-l-2 border-b-2 rounded-bl" style="border-color: rgba(162, 89, 255, 0.4);" />
                  <div class="absolute bottom-2 right-2 w-4 h-4 border-r-2 border-b-2 rounded-br" style="border-color: rgba(162, 89, 255, 0.4);" />
                </div>

                <button
                  v-if="!qrConnected"
                  class="px-6 py-2 rounded-lg text-xs font-bold cursor-pointer transition-all"
                  :style="{ background: qrScanning ? 'rgba(162, 89, 255, 0.1)' : 'rgba(162, 89, 255, 0.15)', color: '#A259FF', border: '1px solid rgba(162, 89, 255, 0.2)' }"
                  :disabled="qrScanning"
                  @click="startQrScan"
                >
                  {{ qrScanning ? 'Ожидание...' : 'Симулировать сканирование' }}
                </button>
              </div>

              <!-- Instructions -->
              <div>
                <h3 class="text-lg font-bold text-white mb-4">Инструкция:</h3>
                <div class="space-y-4">
                  <div class="flex items-start gap-3">
                    <span class="flex items-center justify-center size-6 rounded-full text-[10px] font-bold shrink-0" style="background: rgba(162, 89, 255, 0.15); color: #A259FF;">1</span>
                    <p class="text-sm text-white/60">Откройте камеру на вашем телефоне</p>
                  </div>
                  <div class="flex items-start gap-3">
                    <span class="flex items-center justify-center size-6 rounded-full text-[10px] font-bold shrink-0" style="background: rgba(162, 89, 255, 0.15); color: #A259FF;">2</span>
                    <p class="text-sm text-white/60">Наведите камеру на QR-код на экране</p>
                  </div>
                  <div class="flex items-start gap-3">
                    <span class="flex items-center justify-center size-6 rounded-full text-[10px] font-bold shrink-0" style="background: rgba(162, 89, 255, 0.15); color: #A259FF;">3</span>
                    <p class="text-sm text-white/60">Перейдите по ссылке — откроется страница передачи видео</p>
                  </div>
                  <div class="flex items-start gap-3">
                    <span class="flex items-center justify-center size-6 rounded-full text-[10px] font-bold shrink-0" style="background: rgba(162, 89, 255, 0.15); color: #A259FF;">4</span>
                    <p class="text-sm text-white/60">Разрешите доступ к камере и микрофону</p>
                  </div>
                  <div class="flex items-start gap-3">
                    <span class="flex items-center justify-center size-6 rounded-full text-[10px] font-bold shrink-0" style="background: rgba(52, 168, 83, 0.15); color: #34A853;">✓</span>
                    <p class="text-sm text-white/60">Подключение установлено автоматически!</p>
                  </div>
                </div>

                <div class="mt-6 p-3 rounded-lg" style="background: rgba(66, 133, 244, 0.06); border: 1px solid rgba(66, 133, 244, 0.1);">
                  <p class="text-[11px] text-white/50">
                    <UIcon name="i-lucide-info" class="size-3 inline-block mr-1" style="color: #4285F4;" />
                    Argus AI использует WebRTC — не требуется установка приложений. Всё работает в браузере.
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Sub-step 3: Camera Placement Guide -->
        <div v-if="dualCamStep === 3" class="rounded-2xl border overflow-hidden" style="background: #1A2130; border-color: rgba(162, 89, 255, 0.12);">
          <div class="p-8">
            <h3 class="text-lg font-bold text-white mb-6 text-center">Схема размещения камер</h3>

            <!-- Placement Diagram -->
            <div class="relative mx-auto max-w-lg aspect-square rounded-2xl p-8" style="background: #0D1117; border: 1px solid rgba(255,255,255,0.06);">
              <!-- Desk -->
              <div class="absolute bottom-16 left-1/2 -translate-x-1/2 w-3/4 h-20 rounded-lg flex items-center justify-center" style="background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08);">
                <span class="text-[10px] text-white/20 uppercase tracking-wider">Рабочий стол</span>
              </div>

              <!-- Laptop/Monitor -->
              <div class="absolute bottom-28 left-1/2 -translate-x-1/2 w-32 h-20 rounded-t-lg flex flex-col items-center justify-center" style="background: rgba(66, 133, 244, 0.06); border: 1px solid rgba(66, 133, 244, 0.15);">
                <UIcon name="i-lucide-monitor" class="size-6" style="color: #4285F4;" />
                <span class="text-[8px] text-white/30 mt-1">Компьютер</span>
                <!-- Webcam indicator -->
                <div class="absolute -top-3 left-1/2 -translate-x-1/2 px-2 py-0.5 rounded-full" style="background: rgba(66, 133, 244, 0.15); border: 1px solid rgba(66, 133, 244, 0.3);">
                  <span class="text-[7px] font-bold" style="color: #4285F4;">WEBCAM</span>
                </div>
              </div>

              <!-- Person -->
              <div class="absolute bottom-40 left-1/2 -translate-x-1/2 flex flex-col items-center">
                <div class="size-8 rounded-full flex items-center justify-center" style="background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.15);">
                  <UIcon name="i-lucide-user" class="size-4 text-white/40" />
                </div>
                <span class="text-[8px] text-white/30 mt-1">ВЫ</span>
              </div>

              <!-- Side camera (phone) -->
              <div class="absolute top-1/3 right-8 flex flex-col items-center">
                <div class="relative">
                  <div class="w-10 h-16 rounded-lg flex items-center justify-center" style="background: rgba(162, 89, 255, 0.08); border: 1px solid rgba(162, 89, 255, 0.3);">
                    <UIcon name="i-lucide-smartphone" class="size-5" style="color: #A259FF;" />
                  </div>
                  <div class="absolute -top-2 -left-2 px-1.5 py-0.5 rounded-full" style="background: rgba(162, 89, 255, 0.15); border: 1px solid rgba(162, 89, 255, 0.3);">
                    <span class="text-[6px] font-bold" style="color: #A259FF;">SIDE CAM</span>
                  </div>
                </div>
                <span class="text-[8px] text-white/30 mt-1">Сбоку 45°</span>
              </div>

              <!-- View angles -->
              <svg class="absolute inset-0 w-full h-full pointer-events-none" viewBox="0 0 400 400" fill="none" xmlns="http://www.w3.org/2000/svg">
                <!-- Webcam view cone -->
                <path d="M200 95 L140 180 L260 180 Z" fill="rgba(66, 133, 244, 0.05)" stroke="rgba(66, 133, 244, 0.2)" stroke-width="1" stroke-dasharray="4 4" />
                <!-- Side cam view cone -->
                <path d="M330 160 L230 120 L230 220 Z" fill="rgba(162, 89, 255, 0.05)" stroke="rgba(162, 89, 255, 0.2)" stroke-width="1" stroke-dasharray="4 4" />
              </svg>

              <!-- Legend -->
              <div class="absolute bottom-3 left-3 flex items-center gap-4">
                <div class="flex items-center gap-1.5">
                  <div class="size-2 rounded-full" style="background: #4285F4;" />
                  <span class="text-[8px] text-white/30">Веб-камера (лицо)</span>
                </div>
                <div class="flex items-center gap-1.5">
                  <div class="size-2 rounded-full" style="background: #A259FF;" />
                  <span class="text-[8px] text-white/30">Боковая камера (пространство)</span>
                </div>
              </div>
            </div>

            <!-- Tips -->
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mt-8">
              <div class="p-4 rounded-xl text-center" style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
                <UIcon name="i-lucide-ruler" class="size-5 mx-auto mb-2" style="color: #A259FF;" />
                <p class="text-xs font-semibold text-white/70">Расстояние</p>
                <p class="text-[10px] text-white/40 mt-1">1–1.5 м от стола, угол 45°</p>
              </div>
              <div class="p-4 rounded-xl text-center" style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
                <UIcon name="i-lucide-sun" class="size-5 mx-auto mb-2" style="color: #FBBC05;" />
                <p class="text-xs font-semibold text-white/70">Освещение</p>
                <p class="text-[10px] text-white/40 mt-1">Лицо должно быть хорошо освещено</p>
              </div>
              <div class="p-4 rounded-xl text-center" style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);">
                <UIcon name="i-lucide-lock" class="size-5 mx-auto mb-2" style="color: #EA4335;" />
                <p class="text-xs font-semibold text-white/70">Фиксация</p>
                <p class="text-[10px] text-white/40 mt-1">Используйте подставку или стопку книг</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- ======================================= -->
      <!--  STEP 3: HONOR CODE                    -->
      <!-- ======================================= -->
      <div v-if="currentStep === 'honor-code'" class="space-y-8">
        <div class="text-center mb-12">
          <div class="inline-flex items-center justify-center size-16 rounded-2xl mb-6" style="background: rgba(52, 168, 83, 0.08); border: 1px solid rgba(52, 168, 83, 0.15);">
            <UIcon name="i-lucide-shield-check" class="size-8" style="color: #34A853;" />
          </div>
          <h1 class="text-3xl md:text-4xl font-bold text-white tracking-tight">
            Кодекс честности
          </h1>
          <p class="text-base text-white/40 mt-3 max-w-lg mx-auto">
            Ознакомьтесь с правилами прохождения экзамена и подтвердите своё согласие.
          </p>
        </div>

        <div class="rounded-2xl border overflow-hidden" style="background: #1A2130; border-color: rgba(52, 168, 83, 0.12);">
          <div class="px-6 py-4 border-b flex items-center gap-3" style="border-color: rgba(255,255,255,0.06);">
            <UIcon name="i-lucide-scroll-text" class="size-4" style="color: #34A853;" />
            <span class="text-sm font-bold text-white">Правила и обязательства</span>
          </div>

          <div class="p-6 space-y-4">
            <div
              v-for="(rule, idx) in honorRules"
              :key="idx"
              class="flex items-start gap-4 p-4 rounded-xl transition-all"
              style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.04);"
            >
              <div class="flex items-center justify-center size-9 rounded-lg shrink-0" style="background: rgba(52, 168, 83, 0.08);">
                <UIcon :name="rule.icon" class="size-4" style="color: #34A853;" />
              </div>
              <p class="text-sm text-white/60 leading-relaxed pt-1.5">{{ rule.text }}</p>
            </div>

            <!-- Warning box -->
            <div class="p-4 rounded-xl" style="background: rgba(234, 67, 53, 0.04); border: 1px solid rgba(234, 67, 53, 0.12);">
              <div class="flex items-start gap-3">
                <UIcon name="i-lucide-alert-triangle" class="size-5 shrink-0 mt-0.5" style="color: #EA4335;" />
                <div>
                  <p class="text-xs font-bold text-white/80">Внимание:</p>
                  <p class="text-xs text-white/50 mt-1">Нарушение любого из правил может привести к аннулированию результатов экзамена. Все действия записываются и могут быть проверены преподавателем.</p>
                </div>
              </div>
            </div>
          </div>

          <!-- Agreement -->
          <div class="px-6 py-5 border-t" style="border-color: rgba(255,255,255,0.06); background: rgba(52, 168, 83, 0.02);">
            <div class="flex items-center gap-4">
              <button
                class="relative size-6 rounded-lg border-2 cursor-pointer transition-all shrink-0 flex items-center justify-center"
                :style="{
                  background: honorAgreed ? '#34A853' : 'transparent',
                  borderColor: honorAgreed ? '#34A853' : 'rgba(255,255,255,0.2)'
                }"
                @click="honorAgreed = !honorAgreed"
              >
                <UIcon v-if="honorAgreed" name="i-lucide-check" class="size-4 text-white" />
              </button>
              <p class="text-sm text-white/60">
                Я прочитал(а) и принимаю все правила прохождения экзамена с использованием AI-прокторинга Argus AI.
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- ======================================= -->
      <!--  STEP 4: INTERACTIVE TUTORIAL           -->
      <!-- ======================================= -->
      <div v-if="currentStep === 'tutorial'" class="space-y-8">
        <div class="text-center mb-12">
          <div class="inline-flex items-center justify-center size-16 rounded-2xl mb-6" style="background: rgba(251, 188, 5, 0.08); border: 1px solid rgba(251, 188, 5, 0.15);">
            <UIcon name="i-lucide-play-circle" class="size-8" style="color: #FBBC05;" />
          </div>
          <h1 class="text-3xl md:text-4xl font-bold text-white tracking-tight">
            Пробный запуск
          </h1>
          <p class="text-base text-white/40 mt-3 max-w-lg mx-auto">
            Протестируйте все модули AI-прокторинга перед реальным экзаменом.
          </p>
        </div>

        <!-- Tutorial progress bar -->
        <div class="w-full h-2 rounded-full overflow-hidden mb-8" style="background: rgba(255,255,255,0.06);">
          <div
            class="h-full rounded-full transition-all duration-700"
            :style="{ width: `${tutorialProgress}%`, background: 'linear-gradient(90deg, #FBBC05, #34A853)' }"
          />
        </div>

        <!-- Tutorial phases -->
        <div class="rounded-2xl border overflow-hidden" style="background: #1A2130; border-color: rgba(251, 188, 5, 0.12);">

          <!-- INTRO -->
          <div v-if="tutorialPhase === 'intro'" class="p-8 text-center">
            <div class="max-w-md mx-auto">
              <div class="flex items-center justify-center gap-6 mb-8">
                <div class="flex flex-col items-center gap-2 p-4 rounded-xl" style="background: rgba(66, 133, 244, 0.06);">
                  <UIcon name="i-lucide-scan-face" class="size-8" style="color: #4285F4;" />
                  <span class="text-[9px] text-white/40">Face ID</span>
                </div>
                <div class="flex flex-col items-center gap-2 p-4 rounded-xl" style="background: rgba(162, 89, 255, 0.06);">
                  <UIcon name="i-lucide-eye" class="size-8" style="color: #A259FF;" />
                  <span class="text-[9px] text-white/40">Gaze Track</span>
                </div>
                <div class="flex flex-col items-center gap-2 p-4 rounded-xl" style="background: rgba(52, 168, 83, 0.06);">
                  <UIcon name="i-lucide-mic" class="size-8" style="color: #34A853;" />
                  <span class="text-[9px] text-white/40">Audio</span>
                </div>
              </div>

              <h3 class="text-lg font-bold text-white mb-3">Пробная калибровка</h3>
              <p class="text-sm text-white/40 mb-8">
                Мы проверим распознавание лица, калибровку взгляда и детекцию звука. Это займёт ~30 секунд.
              </p>

              <button
                class="px-8 py-3 rounded-xl text-sm font-bold text-white cursor-pointer transition-all hover:scale-105"
                style="background: linear-gradient(135deg, #FBBC05, #EA4335); box-shadow: 0 0 20px rgba(251, 188, 5, 0.15);"
                @click="startTutorial"
              >
                <span class="flex items-center gap-2">
                  <UIcon name="i-lucide-play" class="size-4" />
                  Начать пробный запуск
                </span>
              </button>
            </div>
          </div>

          <!-- FACE CHECK -->
          <div v-if="tutorialPhase === 'face-check'" class="p-8">
            <div class="flex items-center gap-3 mb-6">
              <div class="flex items-center justify-center size-8 rounded-lg" style="background: rgba(66, 133, 244, 0.1);">
                <UIcon name="i-lucide-scan-face" class="size-4" style="color: #4285F4;" />
              </div>
              <div>
                <span class="text-sm font-bold text-white">Верификация лица (Face ID)</span>
                <span class="text-[9px] text-white/30 ml-2">Шаг 1 из 3</span>
              </div>
            </div>

            <div class="relative mx-auto w-64 h-64 rounded-2xl overflow-hidden mb-6" style="background: #0D1117; border: 2px solid rgba(66, 133, 244, 0.2);">
              <!-- Face outline overlay -->
              <div class="absolute inset-0 flex items-center justify-center">
                <div
                  class="size-36 rounded-full border-2 border-dashed transition-all duration-1000"
                  :style="{
                    borderColor: faceDetected ? '#34A853' : 'rgba(66, 133, 244, 0.3)',
                    boxShadow: faceDetected ? '0 0 20px rgba(52, 168, 83, 0.2)' : 'none'
                  }"
                >
                  <div class="w-full h-full flex items-center justify-center">
                    <UIcon
                      :name="faceDetected ? 'i-lucide-check-circle' : 'i-lucide-user'"
                      class="size-12 transition-all duration-500"
                      :style="{ color: faceDetected ? '#34A853' : 'rgba(255,255,255,0.15)' }"
                    />
                  </div>
                </div>
              </div>

              <!-- Scanning overlay -->
              <div v-if="!faceDetected" class="absolute inset-0">
                <div class="face-scan-line absolute left-0 right-0 h-0.5" style="background: rgba(66, 133, 244, 0.6); box-shadow: 0 0 10px rgba(66, 133, 244, 0.3);" />
              </div>

              <!-- Status badge -->
              <div class="absolute bottom-3 left-1/2 -translate-x-1/2 px-3 py-1 rounded-full" :style="{ background: faceDetected ? 'rgba(52, 168, 83, 0.15)' : 'rgba(66, 133, 244, 0.1)', border: `1px solid ${faceDetected ? 'rgba(52, 168, 83, 0.3)' : 'rgba(66, 133, 244, 0.2)'}` }">
                <span class="text-[10px] font-bold" :style="{ color: faceDetected ? '#34A853' : '#4285F4' }">
                  {{ faceDetected ? 'Лицо распознано ✓' : 'Сканирование...' }}
                </span>
              </div>
            </div>

            <p class="text-xs text-white/40 text-center mb-6">Расположите лицо в овальной рамке. Убедитесь, что лицо хорошо освещено.</p>

            <div class="text-center">
              <button
                v-if="faceDetected"
                class="px-6 py-2.5 rounded-lg text-sm font-bold text-white cursor-pointer transition-all hover:scale-105"
                style="background: #34A853;"
                @click="nextTutorialPhase"
              >
                Далее: Калибровка взгляда →
              </button>
            </div>
          </div>

          <!-- GAZE TEST -->
          <div v-if="tutorialPhase === 'gaze-test'" class="p-8">
            <div class="flex items-center gap-3 mb-6">
              <div class="flex items-center justify-center size-8 rounded-lg" style="background: rgba(162, 89, 255, 0.1);">
                <UIcon name="i-lucide-eye" class="size-4" style="color: #A259FF;" />
              </div>
              <div>
                <span class="text-sm font-bold text-white">Калибровка взгляда (Gaze Tracking)</span>
                <span class="text-[9px] text-white/30 ml-2">Шаг 2 из 3</span>
              </div>
            </div>

            <!-- Gaze calibration grid -->
            <div class="relative mx-auto w-80 h-56 rounded-2xl mb-6" style="background: #0D1117; border: 2px solid rgba(162, 89, 255, 0.2);">
              <!-- Calibration dots -->
              <div
                v-for="pos in [
                  { x: '10%', y: '15%' }, { x: '50%', y: '15%' }, { x: '90%', y: '15%' },
                  { x: '10%', y: '50%' }, { x: '50%', y: '50%' }, { x: '90%', y: '50%' },
                  { x: '10%', y: '85%' }, { x: '50%', y: '85%' }, { x: '90%', y: '85%' }
                ]"
                :key="`${pos.x}-${pos.y}`"
                class="absolute size-3 rounded-full transition-all duration-500"
                :style="{
                  left: pos.x,
                  top: pos.y,
                  transform: 'translate(-50%, -50%)',
                  background: gazeCalibrated ? '#34A853' : '#A259FF',
                  boxShadow: gazeCalibrated ? '0 0 8px rgba(52, 168, 83, 0.4)' : '0 0 8px rgba(162, 89, 255, 0.3)',
                  animation: gazeCalibrated ? 'none' : 'pulse 2s ease-in-out infinite'
                }"
              />

              <!-- Center instruction -->
              <div v-if="!gazeCalibrated" class="absolute inset-0 flex items-center justify-center">
                <span class="text-[10px] text-white/30 bg-black/60 px-3 py-1 rounded">Смотрите на точки</span>
              </div>
              <div v-else class="absolute inset-0 flex items-center justify-center">
                <div class="px-4 py-2 rounded-lg" style="background: rgba(52, 168, 83, 0.1);">
                  <span class="text-xs font-bold" style="color: #34A853;">Калибровка завершена ✓</span>
                </div>
              </div>
            </div>

            <p class="text-xs text-white/40 text-center mb-6">Следите взглядом за фиолетовыми точками. Не двигайте головой.</p>

            <div class="text-center">
              <button
                v-if="gazeCalibrated"
                class="px-6 py-2.5 rounded-lg text-sm font-bold text-white cursor-pointer transition-all hover:scale-105"
                style="background: #34A853;"
                @click="nextTutorialPhase"
              >
                Далее: Проверка звука →
              </button>
            </div>
          </div>

          <!-- AUDIO TEST -->
          <div v-if="tutorialPhase === 'audio-test'" class="p-8">
            <div class="flex items-center gap-3 mb-6">
              <div class="flex items-center justify-center size-8 rounded-lg" style="background: rgba(52, 168, 83, 0.1);">
                <UIcon name="i-lucide-mic" class="size-4" style="color: #34A853;" />
              </div>
              <div>
                <span class="text-sm font-bold text-white">Детекция звука (Voice Activity)</span>
                <span class="text-[9px] text-white/30 ml-2">Шаг 3 из 3</span>
              </div>
            </div>

            <!-- Audio level visualization -->
            <div class="mx-auto w-80 h-32 rounded-2xl flex items-end justify-center gap-[3px] p-4 mb-6" style="background: #0D1117; border: 2px solid rgba(52, 168, 83, 0.2);">
              <div
                v-for="(level, i) in audioLevels"
                :key="i"
                class="flex-1 rounded-t transition-all duration-150"
                :style="{
                  height: `${audioDetected ? level : level * 0.3}%`,
                  background: audioDetected
                    ? `linear-gradient(to top, #34A853, #4285F4)`
                    : 'rgba(255,255,255,0.1)',
                  minHeight: '3px'
                }"
              />
            </div>

            <div class="text-center mb-6">
              <p v-if="!audioDetected" class="text-xs text-white/40">Произнесите что-нибудь для проверки микрофона...</p>
              <div v-else class="inline-flex items-center gap-2 px-4 py-2 rounded-lg" style="background: rgba(52, 168, 83, 0.1); border: 1px solid rgba(52, 168, 83, 0.2);">
                <UIcon name="i-lucide-check-circle" class="size-4" style="color: #34A853;" />
                <span class="text-xs font-bold" style="color: #34A853;">Микрофон работает корректно!</span>
              </div>
            </div>

            <div class="text-center">
              <button
                v-if="audioDetected"
                class="px-6 py-2.5 rounded-lg text-sm font-bold text-white cursor-pointer transition-all hover:scale-105"
                style="background: #34A853;"
                @click="nextTutorialPhase"
              >
                Завершить тест →
              </button>
            </div>
          </div>

          <!-- COMPLETE -->
          <div v-if="tutorialPhase === 'complete'" class="p-8 text-center">
            <div class="relative inline-flex items-center justify-center mb-6">
              <div class="absolute w-32 h-32 rounded-full" style="background: radial-gradient(circle, rgba(52, 168, 83, 0.15) 0%, transparent 70%); filter: blur(15px);" />
              <div class="relative flex items-center justify-center size-20 rounded-full" style="background: rgba(52, 168, 83, 0.1); border: 2px solid rgba(52, 168, 83, 0.3);">
                <UIcon name="i-lucide-check-circle" class="size-10" style="color: #34A853;" />
              </div>
            </div>

            <h3 class="text-2xl font-bold text-white mb-3">Все тесты пройдены!</h3>
            <p class="text-sm text-white/40 max-w-md mx-auto mb-8">
              Ваше устройство полностью готово к прокторингу. Все модули AI-детекции настроены и работают корректно.
            </p>

            <!-- Results summary -->
            <div class="grid grid-cols-3 gap-4 max-w-md mx-auto mb-8">
              <div class="p-3 rounded-xl text-center" style="background: rgba(52, 168, 83, 0.06); border: 1px solid rgba(52, 168, 83, 0.15);">
                <UIcon name="i-lucide-scan-face" class="size-5 mx-auto mb-1" style="color: #34A853;" />
                <p class="text-[9px] font-bold" style="color: #34A853;">Face ID ✓</p>
              </div>
              <div class="p-3 rounded-xl text-center" style="background: rgba(52, 168, 83, 0.06); border: 1px solid rgba(52, 168, 83, 0.15);">
                <UIcon name="i-lucide-eye" class="size-5 mx-auto mb-1" style="color: #34A853;" />
                <p class="text-[9px] font-bold" style="color: #34A853;">Gaze ✓</p>
              </div>
              <div class="p-3 rounded-xl text-center" style="background: rgba(52, 168, 83, 0.06); border: 1px solid rgba(52, 168, 83, 0.15);">
                <UIcon name="i-lucide-mic" class="size-5 mx-auto mb-1" style="color: #34A853;" />
                <p class="text-[9px] font-bold" style="color: #34A853;">Audio ✓</p>
              </div>
            </div>

            <NuxtLink
              to="/"
              class="inline-flex items-center gap-2 px-8 py-3 rounded-xl text-sm font-bold text-white transition-all hover:scale-105"
              style="background: linear-gradient(135deg, #34A853, #4285F4); box-shadow: 0 0 20px rgba(52, 168, 83, 0.15);"
            >
              <UIcon name="i-lucide-arrow-right" class="size-4" />
              Перейти к экзамену
            </NuxtLink>
          </div>
        </div>
      </div>

      <!-- ============================== -->
      <!--  STEP NAVIGATION BUTTONS       -->
      <!-- ============================== -->
      <div class="flex items-center justify-between mt-12 pt-8 border-t" style="border-color: rgba(255,255,255,0.06);">
        <button
          v-if="canGoPrev"
          class="flex items-center gap-2 px-5 py-2.5 rounded-lg text-sm font-medium text-white/50 hover:text-white cursor-pointer transition-all"
          style="background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08);"
          @click="goPrev"
        >
          <UIcon name="i-lucide-arrow-left" class="size-4" />
          {{ steps[currentStepIndex - 1]?.label }}
        </button>
        <div v-else />

        <div class="flex items-center gap-2">
          <div
            v-for="(step, idx) in steps"
            :key="step.id"
            class="size-2 rounded-full transition-all"
            :style="{
              background: idx === currentStepIndex ? step.color : idx < currentStepIndex ? 'rgba(52, 168, 83, 0.4)' : 'rgba(255,255,255,0.1)'
            }"
          />
        </div>

        <button
          v-if="canGoNext"
          class="flex items-center gap-2 px-5 py-2.5 rounded-lg text-sm font-bold text-white cursor-pointer transition-all hover:scale-105"
          :style="{
            background: `linear-gradient(135deg, ${steps[currentStepIndex + 1]?.color || '#4285F4'}, ${steps[currentStepIndex]?.color || '#4285F4'})`,
            boxShadow: `0 0 15px ${hexToRgba(steps[currentStepIndex + 1]?.color || '#4285F4', 0.15)}`
          }"
          @click="goNext"
        >
          {{ steps[currentStepIndex + 1]?.label }}
          <UIcon name="i-lucide-arrow-right" class="size-4" />
        </button>
        <div v-else />
      </div>
    </main>

    <!-- ============================== -->
    <!--  FOOTER                        -->
    <!-- ============================== -->
    <footer class="border-t px-6 py-8 mt-12" style="border-color: rgba(255,255,255,0.06); background: #0A0E14;">
      <div class="max-w-5xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <ArgusLogo :size="24" />
          <span class="text-xs font-bold text-white/40">Argus AI · Руководство студента</span>
        </div>
        <p class="text-[10px] text-white/20">&copy; 2026 Argus AI</p>
      </div>
    </footer>
  </div>
</template>

<style scoped>
/* QR scan line animation */
.qr-scan-line {
  animation: qr-scan 2s ease-in-out infinite;
}

@keyframes qr-scan {
  0% { top: 0; }
  50% { top: 100%; }
  100% { top: 0; }
}

/* Face scan line animation */
.face-scan-line {
  animation: face-scan 2.5s ease-in-out infinite;
}

@keyframes face-scan {
  0% { top: 5%; opacity: 0; }
  10% { opacity: 0.8; }
  50% { top: 90%; opacity: 0.8; }
  90% { opacity: 0.8; }
  100% { top: 5%; opacity: 0; }
}

/* Range input styling */
input[type="range"] {
  -webkit-appearance: none;
  appearance: none;
  height: 6px;
  border-radius: 3px;
  outline: none;
}

input[type="range"]::-webkit-slider-thumb {
  -webkit-appearance: none;
  appearance: none;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: white;
  cursor: pointer;
  box-shadow: 0 0 6px rgba(0, 0, 0, 0.3);
}

input[type="range"]::-moz-range-thumb {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  background: white;
  cursor: pointer;
  border: none;
  box-shadow: 0 0 6px rgba(0, 0, 0, 0.3);
}

/* Pulse animation for gaze dots */
@keyframes pulse {
  0%, 100% { opacity: 0.6; transform: translate(-50%, -50%) scale(1); }
  50% { opacity: 1; transform: translate(-50%, -50%) scale(1.3); }
}

/* Divide border color for system checks */
.divide-y > * + * {
  border-top: 1px solid rgba(255, 255, 255, 0.04);
}
</style>
