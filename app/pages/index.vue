<script setup lang="ts">
import { useAuthStore } from '~/stores/useAuthStore'

const colorMode = useColorMode()
const authStore = useAuthStore()
const showLoginModal = ref(false)

// Force dark mode for landing + restore auth session
onMounted(() => {
  colorMode.preference = 'dark'
  authStore.restoreSession()

  // If already authenticated, skip landing and go to dashboard
  if (authStore.isLoggedIn) {
    navigateTo('/dashboard', { replace: true })
  }
})

function handleLoginSuccess() {
  showLoginModal.value = false
  navigateTo('/dashboard')
}

// --- Scroll reveal ---
const heroRef = ref<HTMLElement | null>(null)
const featuresRef = ref<HTMLElement | null>(null)
const settingsRef = ref<HTMLElement | null>(null)
const archiveRef = ref<HTMLElement | null>(null)
const statsRef = ref<HTMLElement | null>(null)
const ctaRef = ref<HTMLElement | null>(null)

const featuresVisible = ref(false)
const settingsVisible = ref(false)
const archiveVisible = ref(false)
const statsVisible = ref(false)
const ctaVisible = ref(false)

// Animated counters
const counterSessions = ref(0)
const counterCountries = ref(0)
const counterReduction = ref(0)
const counterHours = ref(0)

function animateCounter(target: Ref<number>, end: number, duration: number) {
  const step = end / (duration / 16)
  const interval = setInterval(() => {
    target.value = Math.min(end, target.value + step)
    if (target.value >= end) {
      target.value = end
      clearInterval(interval)
    }
  }, 16)
}

// Cursor follower for hero — multi-color rotation
const cursorX = ref(50)
const cursorY = ref(50)
const cursorColorIndex = ref(0)
const brandColors = ['#4285F4', '#34A853', '#FBBC05', '#EA4335', '#A259FF']

let colorRotationInterval: ReturnType<typeof setInterval> | null = null

function handleMouseMove(e: MouseEvent) {
  const hero = heroRef.value
  if (!hero) return
  const rect = hero.getBoundingClientRect()
  cursorX.value = ((e.clientX - rect.left) / rect.width) * 100
  cursorY.value = ((e.clientY - rect.top) / rect.height) * 100
}

const cursorGradient = computed(() => {
  const c1 = brandColors[cursorColorIndex.value % 5]
  const c2 = brandColors[(cursorColorIndex.value + 1) % 5]
  return `radial-gradient(circle at ${cursorX.value}% ${cursorY.value}%, ${hexToRgba(c1!, 0.07)} 0%, ${hexToRgba(c2!, 0.03)} 30%, transparent 55%)`
})

function hexToRgba(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

// --- Settings Dual-Tab System ---
const settingsTab = ref<'general' | 'exceptions'>('general')
const excSearchQuery = ref('')

// --- Mock Quick Profiles ---
const mockActivePreset = computed<string | null>(() => {
  const s = mockSettings
  // Check Extreme
  if (s.requireSideCamera && s.faceVerification && s.dynamicFaceRecheck && s.antiSpoofing && s.objectDetectionPhone && s.gazeTracking && s.voiceActivityDetection && s.blockCopyPaste && s.blockPrintScreen && s.blockVirtualMachine && s.blockRemoteAccess && s.processScanning && s.hardwareDeviceDetection && s.advancedRemoteBlock && s.hardwareIdBinding && s.deepMultiMonitorCheck && s.gazeSensitivity === 85 && s.voiceDetectionThreshold === 80 && s.gazeDeviationLimitSec === 3 && s.tabSwitchingLimit === 0) return 'extreme'
  // Check Standard
  if (!s.requireSideCamera && s.faceVerification && s.dynamicFaceRecheck && s.antiSpoofing && s.objectDetectionPhone && s.gazeTracking && s.voiceActivityDetection && !s.blockCopyPaste && s.blockPrintScreen && s.blockVirtualMachine && s.blockRemoteAccess && s.processScanning && !s.hardwareDeviceDetection && s.advancedRemoteBlock && !s.hardwareIdBinding && s.deepMultiMonitorCheck && s.gazeSensitivity === 60 && s.voiceDetectionThreshold === 50 && s.gazeDeviationLimitSec === 8 && s.tabSwitchingLimit === 3) return 'standard'
  // Check Light
  if (!s.requireSideCamera && !s.faceVerification && !s.dynamicFaceRecheck && !s.antiSpoofing && s.objectDetectionPhone && !s.gazeTracking && !s.voiceActivityDetection && !s.blockCopyPaste && !s.blockPrintScreen && !s.blockVirtualMachine && !s.blockRemoteAccess && !s.processScanning && !s.hardwareDeviceDetection && !s.advancedRemoteBlock && !s.hardwareIdBinding && !s.deepMultiMonitorCheck && s.gazeSensitivity === 30 && s.voiceDetectionThreshold === 25 && s.gazeDeviationLimitSec === 15 && s.tabSwitchingLimit === 10) return 'light'
  return null
})

function applyMockPreset(preset: string) {
  if (preset === 'extreme') {
    Object.assign(mockSettings, {
      requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, objectDetectionPhone: true, objectDetectionPerson: true, roomScan360: true,
      gazeTracking: true, gazeSensitivity: 85, gazeDeviationLimitSec: 3, voiceDetectionThreshold: 80, voiceActivityDetection: true,
      tabSwitchingLimit: 0, blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: true, blockRemoteAccess: true, typingDynamics: true,
      processScanning: true, hardwareDeviceDetection: true, advancedRemoteBlock: true, hardwareIdBinding: true, deepMultiMonitorCheck: true
    })
  } else if (preset === 'standard') {
    Object.assign(mockSettings, {
      requireSideCamera: false, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true, objectDetectionPhone: true, objectDetectionPerson: true, roomScan360: false,
      gazeTracking: true, gazeSensitivity: 60, gazeDeviationLimitSec: 8, voiceDetectionThreshold: 50, voiceActivityDetection: true,
      tabSwitchingLimit: 3, blockCopyPaste: false, blockPrintScreen: true, blockVirtualMachine: true, blockMultiDesktop: false, blockRemoteAccess: true, typingDynamics: false,
      processScanning: true, hardwareDeviceDetection: false, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true
    })
  } else if (preset === 'light') {
    Object.assign(mockSettings, {
      requireSideCamera: false, faceVerification: false, dynamicFaceRecheck: false, antiSpoofing: false, objectDetectionPhone: true, objectDetectionPerson: false, roomScan360: false,
      gazeTracking: false, gazeSensitivity: 30, gazeDeviationLimitSec: 15, voiceDetectionThreshold: 25, voiceActivityDetection: false,
      tabSwitchingLimit: 10, blockCopyPaste: false, blockPrintScreen: false, blockVirtualMachine: false, blockMultiDesktop: false, blockRemoteAccess: false, typingDynamics: false,
      processScanning: false, hardwareDeviceDetection: false, advancedRemoteBlock: false, hardwareIdBinding: false, deepMultiMonitorCheck: false
    })
  }
}

// --- Mock Individual Exception Selection ---
const mockExcSelectedStudent = ref<{ name: string, studentId: string, reason: string } | null>(null)
const mockExcSettings = reactive({
  requireSideCamera: true, faceVerification: true, dynamicFaceRecheck: true, antiSpoofing: true,
  objectDetectionPhone: true, gazeTracking: true, voiceActivityDetection: true,
  blockCopyPaste: true, blockPrintScreen: true, blockVirtualMachine: true, blockRemoteAccess: true,
  processScanning: true, hardwareDeviceDetection: false, advancedRemoteBlock: true, hardwareIdBinding: false, deepMultiMonitorCheck: true
})

const mockExcDiffCount = computed(() => {
  let count = 0
  const keys = ['requireSideCamera', 'faceVerification', 'dynamicFaceRecheck', 'antiSpoofing', 'objectDetectionPhone', 'gazeTracking', 'voiceActivityDetection', 'blockCopyPaste', 'blockPrintScreen', 'blockVirtualMachine', 'blockRemoteAccess', 'processScanning', 'hardwareDeviceDetection', 'advancedRemoteBlock', 'hardwareIdBinding', 'deepMultiMonitorCheck'] as const
  for (const key of keys) {
    if ((mockExcSettings as any)[key] !== (mockSettings as any)[key]) count++
  }
  return count
})

function selectMockException(exc: { name: string, studentId: string, reason: string, overrides: Record<string, any> }) {
  mockExcSelectedStudent.value = { name: exc.name, studentId: exc.studentId, reason: exc.reason }
  // Reset to global then apply overrides
  Object.assign(mockExcSettings, {
    requireSideCamera: mockSettings.requireSideCamera, faceVerification: mockSettings.faceVerification, dynamicFaceRecheck: mockSettings.dynamicFaceRecheck, antiSpoofing: mockSettings.antiSpoofing,
    objectDetectionPhone: mockSettings.objectDetectionPhone, gazeTracking: mockSettings.gazeTracking, voiceActivityDetection: mockSettings.voiceActivityDetection,
    blockCopyPaste: mockSettings.blockCopyPaste, blockPrintScreen: mockSettings.blockPrintScreen, blockVirtualMachine: mockSettings.blockVirtualMachine, blockRemoteAccess: mockSettings.blockRemoteAccess,
    processScanning: mockSettings.processScanning, hardwareDeviceDetection: mockSettings.hardwareDeviceDetection, advancedRemoteBlock: mockSettings.advancedRemoteBlock, hardwareIdBinding: mockSettings.hardwareIdBinding, deepMultiMonitorCheck: mockSettings.deepMultiMonitorCheck
  })
  // Apply individual overrides
  for (const [key, val] of Object.entries(exc.overrides)) {
    if (key in mockExcSettings) {
      (mockExcSettings as any)[key] = val
    }
  }
}

function clearMockException() {
  mockExcSelectedStudent.value = null
}

function resetMockExcToGlobal() {
  Object.assign(mockExcSettings, {
    requireSideCamera: mockSettings.requireSideCamera, faceVerification: mockSettings.faceVerification, dynamicFaceRecheck: mockSettings.dynamicFaceRecheck, antiSpoofing: mockSettings.antiSpoofing,
    objectDetectionPhone: mockSettings.objectDetectionPhone, gazeTracking: mockSettings.gazeTracking, voiceActivityDetection: mockSettings.voiceActivityDetection,
    blockCopyPaste: mockSettings.blockCopyPaste, blockPrintScreen: mockSettings.blockPrintScreen, blockVirtualMachine: mockSettings.blockVirtualMachine, blockRemoteAccess: mockSettings.blockRemoteAccess,
    processScanning: mockSettings.processScanning, hardwareDeviceDetection: mockSettings.hardwareDeviceDetection, advancedRemoteBlock: mockSettings.advancedRemoteBlock, hardwareIdBinding: mockSettings.hardwareIdBinding, deepMultiMonitorCheck: mockSettings.deepMultiMonitorCheck
  })
}

// Mock student exceptions
const mockExceptions = ref([
  {
    name: 'Асылбек Нурланов',
    studentId: '040215550123',
    reason: 'Нарушение зрения — увеличенный экран',
    overrides: { gazeTracking: false, gazeSensitivity: 30, gazeDeviationLimitSec: 20 }
  },
  {
    name: 'Мадина Сериккызы',
    studentId: '050823450789',
    reason: 'Мобильная камера недоступна',
    overrides: { requireSideCamera: false, roomScan360: false }
  },
  {
    name: 'Дамир Касымов',
    studentId: '030512340456',
    reason: 'Медленный интернет (3G)',
    overrides: { voiceActivityDetection: false, dynamicFaceRecheck: false }
  }
])

const filteredMockExceptions = computed(() => {
  if (!excSearchQuery.value) return mockExceptions.value
  const q = excSearchQuery.value.toLowerCase()
  return mockExceptions.value.filter(e =>
    e.name.toLowerCase().includes(q) || e.studentId.includes(q) || e.reason.toLowerCase().includes(q)
  )
})

const activeSettingsCount = computed(() => {
  let count = 0
  const s = mockSettings
  if (s.requireSideCamera) count++
  if (s.faceVerification) count++
  if (s.dynamicFaceRecheck) count++
  if (s.antiSpoofing) count++
  if (s.objectDetectionPhone) count++
  if (s.gazeTracking) count++
  if (s.voiceActivityDetection) count++
  if (s.blockCopyPaste) count++
  if (s.blockPrintScreen) count++
  if (s.blockVirtualMachine) count++
  if (s.blockRemoteAccess) count++
  return count
})

// "Why Argus AI?" killer features
const killerFeatures = [
  {
    icon: 'i-lucide-video',
    title: 'Двойная камера 360°',
    description: 'Синхронный мониторинг с веб-камеры и телефона.',
    color: '#4285F4'
  },
  {
    icon: 'i-lucide-shield-alert',
    title: 'Deep Lockdown Vision',
    description: 'Блокировка виртуальных машин, удалённого доступа и вторых мониторов.',
    color: '#EA4335'
  },
  {
    icon: 'i-lucide-scan-face',
    title: 'Антиспуфинг-контроль',
    description: 'Детекция подмены лица и неживых объектов в реальном времени.',
    color: '#A259FF'
  },
  {
    icon: 'i-lucide-wifi',
    title: 'Умный интернет',
    description: 'Стабильная работа AI-анализа даже при 52 кб/с.',
    color: '#34A853'
  },
  {
    icon: 'i-lucide-plug-zap',
    title: 'API-Native',
    description: 'Лёгкая интеграция в любую LMS (Eduser, Moodle) за 48 часов.',
    color: '#FBBC05'
  },
  {
    icon: 'i-lucide-user-cog',
    title: 'Индивидуальный подход',
    description: 'Override System: уникальные правила прокторинга для каждого студента без ущерба целостности экзамена.',
    color: '#EA4335'
  }
]

// --- Interactive Proctoring Settings Mock ---
const mockSettings = reactive({
  requireSideCamera: true,
  faceVerification: true,
  dynamicFaceRecheck: true,
  antiSpoofing: true,
  roomScan360: false,
  objectDetectionPhone: true,
  objectDetectionPerson: true,
  gazeTracking: true,
  gazeSensitivity: 65,
  gazeDeviationLimitSec: 8,
  voiceDetectionThreshold: 55,
  voiceActivityDetection: true,
  tabSwitchingLimit: 3,
  blockCopyPaste: true,
  blockPrintScreen: true,
  blockVirtualMachine: true,
  blockMultiDesktop: false,
  blockRemoteAccess: true,
  typingDynamics: false,
  processScanning: true,
  hardwareDeviceDetection: false,
  advancedRemoteBlock: true,
  hardwareIdBinding: false,
  deepMultiMonitorCheck: true
})

// --- Archive Player Mock ---
const archivePlaying = ref(false)
const archiveProgress = ref(32)
const archiveVolume = ref(75)
const archiveShowSide = ref(true)
const archiveCurrentTime = ref('03:42')
const archiveTotalTime = ref('11:15')

let archiveInterval: ReturnType<typeof setInterval> | null = null

function toggleArchivePlay() {
  archivePlaying.value = !archivePlaying.value
  if (archivePlaying.value) {
    archiveInterval = setInterval(() => {
      if (archiveProgress.value < 100) {
        archiveProgress.value += 0.2
        // Update current time display
        const totalSeconds = Math.floor((archiveProgress.value / 100) * 675)
        const mins = Math.floor(totalSeconds / 60)
        const secs = totalSeconds % 60
        archiveCurrentTime.value = `${String(mins).padStart(2, '0')}:${String(secs).padStart(2, '0')}`
      } else {
        archivePlaying.value = false
        if (archiveInterval) clearInterval(archiveInterval)
      }
    }, 100)
  } else {
    if (archiveInterval) clearInterval(archiveInterval)
  }
}

// --- Audio Waveform Animation ---
const waveformBars = ref<number[]>([])
let waveformInterval: ReturnType<typeof setInterval> | null = null

function generateWaveform() {
  const bars: number[] = []
  for (let i = 0; i < 60; i++) {
    bars.push(Math.random() * 80 + 10)
  }
  waveformBars.value = bars
}

// --- Particle Effect ---
interface Particle {
  id: number
  x: number
  y: number
  size: number
  color: string
  duration: number
  delay: number
}

const particles = ref<Particle[]>([])

function generateParticles() {
  const items: Particle[] = []
  for (let i = 0; i < 40; i++) {
    items.push({
      id: i,
      x: Math.random() * 100,
      y: Math.random() * 100,
      size: Math.random() * 3 + 1,
      color: brandColors[Math.floor(Math.random() * 5)]!,
      duration: Math.random() * 15 + 10,
      delay: Math.random() * 10
    })
  }
  particles.value = items
}

// Intersection observer for scroll reveal
onMounted(() => {
  generateParticles()
  generateWaveform()

  // Rotate cursor color every 2s
  colorRotationInterval = setInterval(() => {
    cursorColorIndex.value = (cursorColorIndex.value + 1) % 5
  }, 2000)

  // Animate waveform
  waveformInterval = setInterval(() => {
    generateWaveform()
  }, 300)

  const observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          if (entry.target === featuresRef.value) featuresVisible.value = true
          if (entry.target === settingsRef.value) settingsVisible.value = true
          if (entry.target === archiveRef.value) archiveVisible.value = true

          if (entry.target === comparisonRef.value) comparisonVisible.value = true
          if (entry.target === pricingRef.value) pricingVisible.value = true
          if (entry.target === statsRef.value) {
            statsVisible.value = true
            animateCounter(counterHours, 1000000, 2000)
            animateCounter(counterCountries, 50, 1500)
            animateCounter(counterReduction, 94, 1800)
            animateCounter(counterSessions, 10000, 2200)
          }
          if (entry.target === ctaRef.value) ctaVisible.value = true
        }
      })
    },
    { threshold: 0.15 }
  )

  if (featuresRef.value) observer.observe(featuresRef.value)
  if (settingsRef.value) observer.observe(settingsRef.value)
  if (archiveRef.value) observer.observe(archiveRef.value)

  if (comparisonRef.value) observer.observe(comparisonRef.value)
  if (pricingRef.value) observer.observe(pricingRef.value)
  if (statsRef.value) observer.observe(statsRef.value)
  if (ctaRef.value) observer.observe(ctaRef.value)

  onUnmounted(() => {
    observer.disconnect()
    if (colorRotationInterval) clearInterval(colorRotationInterval)
    if (archiveInterval) clearInterval(archiveInterval)
    if (waveformInterval) clearInterval(waveformInterval)
  })
})

// Features data — 360° Control
const features = [
  {
    icon: 'i-lucide-video',
    title: 'Видео контроль 360°',
    description: 'Дуальная камера: веб-камера + мобильная. Верификация лица, сканирование комнаты, непрерывный мониторинг рабочего пространства в реальном времени.',
    accent: '#4285F4'
  },
  {
    icon: 'i-lucide-brain',
    title: 'AI Аналитика Real-Time',
    description: 'Трекинг взгляда, аудио-криминалистика, анализ поведения. Нейросеть распознаёт аномалии за миллисекунды с точностью 99.9%.',
    accent: '#A259FF'
  },
  {
    icon: 'i-lucide-lock',
    title: 'System Lockdown',
    description: 'Блокировка браузера, перехват буфера обмена, детекция виртуальных машин и удалённого доступа. Полная изоляция среды тестирования.',
    accent: '#EA4335'
  }
]

// --- Comparison Section ---
const comparisonRef = ref<HTMLElement | null>(null)
const comparisonVisible = ref(false)

const comparisonData = [
  {
    category: 'Камеры',
    icon: 'i-lucide-video',
    standard: { label: 'Только веб-камера', level: 1, hasFeature: false },
    argus: { label: 'Dual-Cam 360° (веб + мобильная)', level: 3, hasFeature: true }
  },
  {
    category: 'AI Детекция',
    icon: 'i-lucide-brain',
    standard: { label: 'Базовое распознавание лица', level: 1, hasFeature: false },
    argus: { label: 'Face ID + Gaze + Voice + Объекты', level: 3, hasFeature: true }
  },
  {
    category: 'Kernel-Level контроль',
    icon: 'i-lucide-cpu',
    standard: { label: 'Нет доступа к ядру ОС', level: 0, hasFeature: false },
    argus: { label: 'Полный Kernel-Level System Control', level: 3, hasFeature: true },
    highlight: '#FBBC05'
  },
  {
    category: 'Блокировка браузера',
    icon: 'i-lucide-lock',
    standard: { label: 'Только на уровне браузера', level: 1, hasFeature: false },
    argus: { label: 'Deep Lockdown: VM, Remote, Clipboard, Print', level: 3, hasFeature: true }
  },
  {
    category: 'Индивидуальные исключения',
    icon: 'i-lucide-user-cog',
    standard: { label: 'Единые правила для всех', level: 0, hasFeature: false },
    argus: { label: 'Персональные настройки для каждого студента', level: 3, hasFeature: true },
    highlight: '#A259FF'
  },
  {
    category: 'Пропускная способность',
    icon: 'i-lucide-wifi',
    standard: { label: '500+ kb/s (часто зависает)', level: 1, hasFeature: false },
    argus: { label: '52 kb/s — стабильно даже на мобильном', level: 3, hasFeature: true }
  },
  {
    category: 'Правила экзамена',
    icon: 'i-lucide-settings-2',
    standard: { label: 'Фиксированные шаблоны', level: 1, hasFeature: false },
    argus: { label: '24 параметра + Quick Profiles', level: 3, hasFeature: true }
  },
  {
    category: 'Масштабируемость',
    icon: 'i-lucide-scale',
    standard: { label: 'До 500 одновременно', level: 1, hasFeature: false },
    argus: { label: '10,000+ одновременных сессий', level: 3, hasFeature: true }
  },
  {
    category: 'Архив записей',
    icon: 'i-lucide-film',
    standard: { label: 'Только видео (30 дней)', level: 1, hasFeature: false },
    argus: { label: 'Видео + Аудио + Timeline + AI-отчёт', level: 3, hasFeature: true }
  },
  {
    category: 'API интеграция',
    icon: 'i-lucide-plug-zap',
    standard: { label: 'Нет или ограниченный', level: 0, hasFeature: false },
    argus: { label: 'Full REST API + Webhooks + SSO', level: 3, hasFeature: true }
  }
]

// --- Pricing Calculator ---
const pricingRef = ref<HTMLElement | null>(null)
const pricingVisible = ref(false)
const studentCount = ref(1000)

const pricePerStudent = computed(() => {
  if (studentCount.value > 5000) return 1.00
  if (studentCount.value > 2000) return 1.20
  if (studentCount.value > 500) return 0.60
  return 2.50
})

const totalCost = computed(() => {
  return (studentCount.value * pricePerStudent.value).toLocaleString('en-US', { minimumFractionDigits: 0, maximumFractionDigits: 0 })
})

const pricingTierLabel = computed(() => {
  if (studentCount.value > 5000) return 'Enterprise'
  if (studentCount.value > 2000) return 'Университет'
  if (studentCount.value > 500) return 'Институт'
  return 'Колледж'
})

const pricingTierColor = computed(() => {
  if (studentCount.value > 5000) return '#34A853'
  if (studentCount.value > 2000) return '#4285F4'
  if (studentCount.value > 500) return '#A259FF'
  return '#FBBC05'
})

const pricingFeatures = [
  {
    category: 'AI Core',
    icon: 'i-lucide-brain',
    color: '#A259FF',
    items: ['Face ID верификация', 'Gaze Tracking (трекинг взгляда)', 'Voice Detection (детекция голоса)']
  },
  {
    category: 'Device Control',
    icon: 'i-lucide-monitor-smartphone',
    color: '#4285F4',
    items: ['Dual-Cam (веб + боковая)', 'Browser Lockdown', 'VM Detection']
  },
  {
    category: 'Content Protection',
    icon: 'i-lucide-shield-check',
    color: '#EA4335',
    items: ['Clipboard блокировка', 'PrintScreen защита', 'Anti-Spoofing']
  },
  {
    category: 'Integration',
    icon: 'i-lucide-plug-zap',
    color: '#34A853',
    items: ['Full REST API', 'Webhook уведомления', 'SSO / SAML']
  }
]

// Archive session events — extended log
const archiveEvents = [
  { time: '00:15', label: 'Сессия начата, Face ID подтверждён', severity: 'info', icon: 'i-lucide-scan-face', tag: 'ИНФО', source: 'system' },
  { time: '00:42', label: 'Боковая камера подключена (720p)', severity: 'info', icon: 'i-lucide-smartphone', tag: 'ИНФО', source: 'side' },
  { time: '01:24', label: 'Отклонение взгляда вправо (3.2с)', severity: 'warning', icon: 'i-lucide-eye', tag: 'ВНИМАНИЕ', source: 'webcam' },
  { time: '02:10', label: 'Голос в фоне — пассивный шум', severity: 'info', icon: 'i-lucide-volume-1', tag: 'ИНФО', source: 'system' },
  { time: '03:15', label: 'Телефон обнаружен в кадре', severity: 'critical', icon: 'i-lucide-smartphone', tag: 'КРИТИЧНО', source: 'webcam' },
  { time: '03:48', label: 'Dynamic Face ID — повторная проверка OK', severity: 'info', icon: 'i-lucide-scan-face', tag: 'ИНФО', source: 'webcam' },
  { time: '04:33', label: 'Попытка переключения вкладки (1/3)', severity: 'warning', icon: 'i-lucide-app-window', tag: 'ВНИМАНИЕ', source: 'system' },
  { time: '05:42', label: 'Шёпот обнаружен (55dB)', severity: 'warning', icon: 'i-lucide-volume-2', tag: 'ВНИМАНИЕ', source: 'system' },
  { time: '06:15', label: 'Фоновые голоса обнаружены', severity: 'critical', icon: 'i-lucide-users', tag: 'КРИТИЧНО', source: 'webcam' },
  { time: '07:02', label: 'Gaze Tracking: норма восстановлена', severity: 'info', icon: 'i-lucide-eye', tag: 'ИНФО', source: 'webcam' },
  { time: '08:31', label: '2 лица в кадре (боковая камера)', severity: 'critical', icon: 'i-lucide-users', tag: 'КРИТИЧНО', source: 'side' },
  { time: '09:44', label: 'Clipboard paste blocked', severity: 'warning', icon: 'i-lucide-clipboard', tag: 'ВНИМАНИЕ', source: 'system' },
  { time: '10:08', label: 'Сессия завершена', severity: 'info', icon: 'i-lucide-check-circle', tag: 'ИНФО', source: 'system' }
]

// --- Enhanced Archive Demo ---
const archiveSelectedSession = ref<number | null>(null)
const archiveDetailTab = ref<'video' | 'kernel'>('video')
const archiveCamerasSwapped = ref(false)
const archiveMuted = ref(true)
const archiveNoiseLevel = ref(38)
const archiveHighlightedCamera = ref<'webcam' | 'side' | null>(null)

const archiveMockSessions = [
  { id: 0, name: 'Айгерим Тулебаева', iin: '040512500123', exam: 'ЕНТ Математика', date: '15.01.2026', duration: '11:15', integrity: 67, violations: 7, status: 'pending' as const, hasException: true, exceptionCount: 3 },
  { id: 1, name: 'Даурен Касымов', iin: '030815400456', exam: 'ЕНТ Физика', date: '15.01.2026', duration: '09:42', integrity: 92, violations: 1, status: 'reviewed' as const, hasException: false, exceptionCount: 0 },
  { id: 2, name: 'Мадина Серикова', iin: '050220300789', exam: 'ЕНТ Информатика', date: '15.01.2026', duration: '10:30', integrity: 45, violations: 12, status: 'voided' as const, hasException: true, exceptionCount: 5 },
  { id: 3, name: 'Арман Жумабеков', iin: '040918600234', exam: 'ЕНТ История', date: '15.01.2026', duration: '08:55', integrity: 88, violations: 2, status: 'reviewed' as const, hasException: false, exceptionCount: 0 }
]

const { sessionStatusLabel, sessionStatusColor, integrityColor, sourceLabel, sourceIcon, sourceColor } = useStatusHelpers()

const archiveKernelStatus = reactive({
  processScanning: true,
  hardwareDeviceDetection: false,
  advancedRemoteBlock: true,
  hardwareIdBinding: false,
  deepMultiMonitorCheck: true
})

const archiveSelectedSessionData = computed(() => {
  if (archiveSelectedSession.value === null) return archiveMockSessions[0]
  return archiveMockSessions.find(s => s.id === archiveSelectedSession.value) ?? archiveMockSessions[0]
})

const archiveWebcamEvents = computed(() => archiveEvents.filter(e => e.source === 'webcam').length)
const archiveSideEvents = computed(() => archiveEvents.filter(e => e.source === 'side').length)
const archiveSystemEvents = computed(() => archiveEvents.filter(e => e.source === 'system').length)

function archiveSelectSession(id: number) {
  archiveSelectedSession.value = id
  archiveProgress.value = 0
  archivePlaying.value = false
  archiveCurrentTime.value = '00:00'
  archiveCamerasSwapped.value = false
  archiveHighlightedCamera.value = null
}

function archiveSeekToEvent(timeStr: string, source?: string) {
  const [mins, secs] = timeStr.split(':').map(Number)
  const totalSec = (mins ?? 0) * 60 + (secs ?? 0)
  const totalDuration = 675
  archiveProgress.value = Math.min(100, (totalSec / totalDuration) * 100)
  archiveCurrentTime.value = timeStr
  if (source === 'side' || source === 'webcam') {
    archiveHighlightedCamera.value = source
    setTimeout(() => { archiveHighlightedCamera.value = null }, 2500)
  }
}

const archiveKernelLabels: Record<string, string> = {
  processScanning: 'Process Scan',
  hardwareDeviceDetection: 'HW Detect',
  advancedRemoteBlock: 'Remote Block',
  hardwareIdBinding: 'HW ID Bind',
  deepMultiMonitorCheck: 'Multi-Mon'
}
</script>

<template>
  <div
    class="landing-page min-h-screen overflow-x-hidden"
    style="background: #121820;"
  >
    <!-- ============================== -->
    <!--  NAVIGATION                    -->
    <!-- ============================== -->
    <nav
      class="fixed top-0 left-0 right-0 z-50 px-6 py-4"
      style="background: rgba(18, 24, 32, 0.75); backdrop-filter: blur(20px); border-bottom: 1px solid rgba(255,255,255,0.06);"
    >
      <div class="max-w-7xl mx-auto flex items-center justify-between">
        <div class="flex items-center gap-3">
          <ArgusLogo
            :size="36"
            :animated="true"
          />
          <span class="text-lg font-bold text-white tracking-tight">Argus AI</span>
        </div>

        <div class="hidden md:flex items-center gap-8">
          <a
            href="#features"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Возможности</a>
          <a
            href="#settings-demo"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Настройки</a>
          <a
            href="#archive-demo"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Архив</a>
          <a
            href="#stats"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Результаты</a>
          <a
            href="#comparison"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Сравнение</a>
          <a
            href="#pricing"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Стоимость</a>
          <NuxtLink
            v-if="authStore.isLoggedIn"
            to="/"
            class="text-sm text-white/50 hover:text-white transition-colors"
          >Дашборд</NuxtLink>
        </div>

        <div class="flex items-center gap-3">
          <!-- Not logged in: show "Войти" -->
          <template v-if="!authStore.isLoggedIn">
            <button
              class="px-4 py-2 rounded-lg text-sm font-medium text-white/70 hover:text-white transition-colors cursor-pointer"
              @click="showLoginModal = true"
            >
              Войти
            </button>
          </template>

          <!-- Logged in: show "Админ панель" -->
          <template v-else>
            <NuxtLink
              to="/"
              class="scanner-btn scanner-btn-nav px-5 py-2.5 rounded-lg text-sm font-bold text-white inline-flex items-center"
            >
              <span>Админ панель</span>
            </NuxtLink>
          </template>

        </div>
      </div>
    </nav>

    <!-- ============================== -->
    <!--  HERO SECTION                  -->
    <!-- ============================== -->
    <section
      ref="heroRef"
      class="relative min-h-screen flex items-center justify-center px-6 pt-20 overflow-hidden"
      @mousemove="handleMouseMove"
    >
      <!-- Animated background -->
      <div class="absolute inset-0 overflow-hidden">
        <!-- Grid pattern -->
        <div
          class="absolute inset-0 opacity-[0.03]"
          style="background-image: linear-gradient(rgba(255,255,255,0.3) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.3) 1px, transparent 1px); background-size: 80px 80px;"
        />
        <!-- Radial gradient following cursor — multi-color -->
        <div
          class="absolute inset-0 transition-all duration-700 ease-out"
          :style="{ background: cursorGradient }"
        />
        <!-- Floating particles -->
        <div
          v-for="p in particles"
          :key="p.id"
          class="particle absolute rounded-full"
          :style="{
            left: `${p.x}%`,
            top: `${p.y}%`,
            width: `${p.size}px`,
            height: `${p.size}px`,
            background: p.color,
            opacity: 0.3,
            animationDuration: `${p.duration}s`,
            animationDelay: `${p.delay}s`
          }"
        />
        <!-- Static glow — brand gradient -->
        <div
          class="absolute top-1/4 left-1/2 -translate-x-1/2 w-[800px] h-[800px] rounded-full"
          style="background: radial-gradient(circle, rgba(66, 133, 244, 0.05) 0%, rgba(162, 89, 255, 0.03) 30%, transparent 70%);"
        />
      </div>

      <div class="relative z-10 max-w-5xl mx-auto text-center">
        <!-- Pre-headline badge -->
        <div
          class="inline-flex items-center gap-2 px-4 py-2 rounded-full mb-8"
          style="background: rgba(66, 133, 244, 0.08); border: 1px solid rgba(66, 133, 244, 0.15);"
        >
          <div class="size-2 rounded-full bg-green-400 animate-pulse" />
          <span class="text-xs font-semibold text-white/70">AI-powered vigilance · 99.97% uptime</span>
        </div>

        <!-- Main headline with scanline effect -->
        <h1 class="hero-headline text-4xl sm:text-5xl md:text-7xl font-bold text-white leading-[1.1] tracking-tight">
          <span class="block">AI НА СТРАЖЕ ЧЕСТНОСТИ:</span>
          <span class="hero-gradient-text block mt-2">БЕСКОМПРОМИССНЫЙ ПРОКТОРИНГ</span>
          <span class="block mt-2 text-3xl sm:text-4xl md:text-5xl text-white/60">НОВОГО ПОКОЛЕНИЯ</span>
        </h1>

        <!-- Sub-headline -->
        <p class="mt-8 text-lg md:text-xl text-white/50 max-w-3xl mx-auto leading-relaxed">
          Контроль в реальном времени: <span class="text-white/80 font-semibold">10,000+</span> одновременных сессий,
          <span class="text-white/80 font-semibold">360°</span> видео-мониторинг и точность AI-детекции
          <span class="text-white/80 font-semibold">99.9%</span>.
        </p>


        <!-- Scroll indicator -->
        <div class="mt-20 flex flex-col items-center gap-2 animate-bounce">
          <span class="text-[10px] text-white/30 uppercase tracking-widest">Узнать больше</span>
          <UIcon
            name="i-lucide-chevron-down"
            class="size-5 text-white/20"
          />
        </div>
      </div>
    </section>

    <!-- ============================== -->
    <!--  FEATURES — 360° CONTROL       -->
    <!-- ============================== -->
    <section
      id="features"
      ref="featuresRef"
      class="relative py-32 px-6"
    >
      <div class="max-w-7xl mx-auto">
        <div class="text-center mb-20">
          <span
            class="text-xs font-bold uppercase tracking-widest"
            style="color: #4285F4;"
          >360° Контроль</span>
          <h2 class="text-4xl md:text-5xl font-bold text-white mt-4 tracking-tight">
            Интеллект на каждом уровне
          </h2>
          <p class="text-lg text-white/40 mt-4 max-w-2xl mx-auto">
            Три столпа безопасности Argus AI
          </p>
        </div>

        <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div
            v-for="(feature, idx) in features"
            :key="feature.title"
            class="feature-card relative group rounded-2xl p-8 border transition-all duration-700"
            :class="featuresVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-12'"
            :style="{
              transitionDelay: `${idx * 150}ms`,
              background: '#1A2130',
              borderColor: 'rgba(255,255,255,0.06)',
              borderTop: `3px solid ${feature.accent}`
            }"
          >
            <div
              class="absolute inset-0 rounded-2xl opacity-0 group-hover:opacity-100 transition-opacity duration-500"
              :style="{ background: `radial-gradient(circle at 50% 0%, ${hexToRgba(feature.accent, 0.08)} 0%, transparent 70%)` }"
            />

            <div class="relative z-10">
              <div
                class="flex items-center justify-center size-14 rounded-xl mb-6"
                :style="{
                  background: hexToRgba(feature.accent, 0.08),
                  border: `1px solid ${hexToRgba(feature.accent, 0.15)}`
                }"
              >
                <UIcon
                  :name="feature.icon"
                  class="size-7"
                  :style="{ color: feature.accent }"
                />
              </div>

              <h3 class="text-lg font-bold text-white">
                {{ feature.title }}
              </h3>
              <p class="text-sm text-white/50 mt-4 leading-relaxed">
                {{ feature.description }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ============================================ -->
    <!--  INTERACTIVE PROCTORING SETTINGS SHOWCASE    -->
    <!--  Dual-Tab Modal + Killer Features Sidebar    -->
    <!-- ============================================ -->
    <section
      id="settings-demo"
      ref="settingsRef"
      class="relative py-32 px-6"
      style="background: #0F151D;"
    >
      <div class="max-w-7xl mx-auto">
        <div class="text-center mb-16">
          <span
            class="text-xs font-bold uppercase tracking-widest"
            style="color: #A259FF;"
          >Настройки прокторинга</span>
          <h2 class="text-4xl md:text-5xl font-bold text-white mt-4 tracking-tight">
            Полный контроль над каждым параметром
          </h2>
          <p class="text-lg text-white/40 mt-4 max-w-2xl mx-auto">
            Попробуйте интерактивную настройку прямо сейчас — точная копия дашборда
          </p>
        </div>

        <!-- === MAIN LAYOUT: Settings Modal (left) + Killer Features (right) === -->
        <div class="grid grid-cols-1 lg:grid-cols-12 gap-6 items-start">
          <!-- ========== LEFT: Settings Modal (8 cols) ========== -->
          <div
            class="lg:col-span-8 relative rounded-2xl border overflow-hidden transition-all duration-1000"
            :class="settingsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-16'"
            style="background: #1A2130; border-color: rgba(162, 89, 255, 0.15);"
          >
            <!-- Modal header -->
            <div
              class="flex items-center justify-between px-6 py-4 border-b"
              style="border-color: rgba(255,255,255,0.06);"
            >
              <div class="flex items-center gap-3">
                <div
                  class="flex items-center justify-center size-8 rounded-lg"
                  style="background: rgba(162, 89, 255, 0.1);"
                >
                  <UIcon
                    name="i-lucide-settings-2"
                    class="size-4"
                    style="color: #A259FF;"
                  />
                </div>
                <div>
                  <span class="text-sm font-bold text-white">Настройки прокторинга</span>
                  <span
                    class="text-[9px] font-bold px-2 py-0.5 rounded-full ml-2"
                    style="background: rgba(52, 168, 83, 0.15); color: #34A853;"
                  >ARGUS PROTECTED</span>
                </div>
              </div>
              <div class="flex items-center gap-2">
                <span
                  class="text-[9px] font-bold px-2 py-0.5 rounded-full"
                  style="background: rgba(66, 133, 244, 0.1); color: #4285F4;"
                >{{ activeSettingsCount }}/11 активно</span>
                <span class="text-[10px] text-white/30 font-mono">Демо</span>
              </div>
            </div>

            <!-- ===== DUAL TAB BAR ===== -->
            <div
              class="flex items-center border-b px-4"
              style="border-color: rgba(255,255,255,0.06);"
            >
              <button
                class="flex items-center gap-2 px-4 py-3 text-xs font-bold cursor-pointer transition-all relative"
                :style="{ color: settingsTab === 'general' ? '#A259FF' : 'rgba(255,255,255,0.4)' }"
                @click="settingsTab = 'general'"
              >
                <UIcon
                  name="i-lucide-sliders-horizontal"
                  class="size-3.5"
                />
                Общие настройки
                <div
                  v-if="settingsTab === 'general'"
                  class="absolute bottom-0 left-0 right-0 h-0.5"
                  style="background: #A259FF;"
                />
              </button>
              <button
                class="flex items-center gap-2 px-4 py-3 text-xs font-bold cursor-pointer transition-all relative"
                :style="{ color: settingsTab === 'exceptions' ? '#A259FF' : 'rgba(255,255,255,0.4)' }"
                @click="settingsTab = 'exceptions'"
              >
                <UIcon
                  name="i-lucide-user-cog"
                  class="size-3.5"
                />
                Индивидуальные исключения
                <span
                  class="text-[8px] font-bold px-1.5 py-0.5 rounded-full ml-1"
                  style="background: rgba(162, 89, 255, 0.12); color: #A259FF;"
                >{{ mockExceptions.length }}</span>
                <div
                  v-if="settingsTab === 'exceptions'"
                  class="absolute bottom-0 left-0 right-0 h-0.5"
                  style="background: #A259FF;"
                />
              </button>
            </div>

            <!-- ===== TAB 1: GENERAL SETTINGS ===== -->
            <div
              v-if="settingsTab === 'general'"
              class="p-6 space-y-6 max-h-[560px] overflow-y-auto custom-scrollbar"
            >
              <!-- QUICK PROFILES -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon
                    name="i-lucide-zap"
                    class="size-4"
                    style="color: #FBBC05;"
                  />
                  <h3 class="text-xs font-bold text-white uppercase tracking-wider">
                    Быстрые профили
                  </h3>
                </div>
                <div class="grid grid-cols-3 gap-2">
                  <button
                    class="flex flex-col items-center gap-1.5 p-3 rounded-xl border-2 transition-all cursor-pointer"
                    :style="{
                      background: mockActivePreset === 'extreme' ? 'rgba(234, 67, 53, 0.1)' : 'rgba(255,255,255,0.02)',
                      borderColor: mockActivePreset === 'extreme' ? 'rgba(234, 67, 53, 0.4)' : 'rgba(255,255,255,0.06)',
                      boxShadow: mockActivePreset === 'extreme' ? '0 0 16px rgba(234, 67, 53, 0.15)' : 'none'
                    }"
                    @click="applyMockPreset('extreme')"
                  >
                    <UIcon
                      name="i-lucide-shield-alert"
                      class="size-5"
                      :style="{ color: mockActivePreset === 'extreme' ? '#EA4335' : 'rgba(255,255,255,0.4)' }"
                    />
                    <span
                      class="text-[10px] font-bold"
                      :style="{ color: mockActivePreset === 'extreme' ? '#EA4335' : 'rgba(255,255,255,0.5)' }"
                    >Экстремальная</span>
                  </button>
                  <button
                    class="flex flex-col items-center gap-1.5 p-3 rounded-xl border-2 transition-all cursor-pointer"
                    :style="{
                      background: mockActivePreset === 'standard' ? 'rgba(66, 133, 244, 0.1)' : 'rgba(255,255,255,0.02)',
                      borderColor: mockActivePreset === 'standard' ? 'rgba(66, 133, 244, 0.4)' : 'rgba(255,255,255,0.06)',
                      boxShadow: mockActivePreset === 'standard' ? '0 0 16px rgba(66, 133, 244, 0.15)' : 'none'
                    }"
                    @click="applyMockPreset('standard')"
                  >
                    <UIcon
                      name="i-lucide-shield-check"
                      class="size-5"
                      :style="{ color: mockActivePreset === 'standard' ? '#4285F4' : 'rgba(255,255,255,0.4)' }"
                    />
                    <span
                      class="text-[10px] font-bold"
                      :style="{ color: mockActivePreset === 'standard' ? '#4285F4' : 'rgba(255,255,255,0.5)' }"
                    >Стандарт</span>
                  </button>
                  <button
                    class="flex flex-col items-center gap-1.5 p-3 rounded-xl border-2 transition-all cursor-pointer"
                    :style="{
                      background: mockActivePreset === 'light' ? 'rgba(52, 168, 83, 0.1)' : 'rgba(255,255,255,0.02)',
                      borderColor: mockActivePreset === 'light' ? 'rgba(52, 168, 83, 0.4)' : 'rgba(255,255,255,0.06)',
                      boxShadow: mockActivePreset === 'light' ? '0 0 16px rgba(52, 168, 83, 0.15)' : 'none'
                    }"
                    @click="applyMockPreset('light')"
                  >
                    <UIcon
                      name="i-lucide-shield-off"
                      class="size-5"
                      :style="{ color: mockActivePreset === 'light' ? '#34A853' : 'rgba(255,255,255,0.4)' }"
                    />
                    <span
                      class="text-[10px] font-bold"
                      :style="{ color: mockActivePreset === 'light' ? '#34A853' : 'rgba(255,255,255,0.5)' }"
                    >Лёгкая</span>
                  </button>
                </div>
              </div>

              <!-- VIDEO RULES -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon
                    name="i-lucide-video"
                    class="size-4"
                    style="color: #4285F4;"
                  />
                  <h3 class="text-xs font-bold text-white uppercase tracking-wider">
                    Видео-правила
                  </h3>
                </div>
                <div class="space-y-0">
                  <div
                    class="flex items-center justify-between py-3 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <div>
                      <p class="text-xs font-medium text-white/80">
                        Требовать боковую камеру
                      </p>
                      <p class="text-[10px] text-white/30">
                        Мобильный телефон как боковая камера
                      </p>
                    </div>
                    <button
                      class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                      :style="{ background: mockSettings.requireSideCamera ? '#4285F4' : 'rgba(255,255,255,0.15)' }"
                      @click="mockSettings.requireSideCamera = !mockSettings.requireSideCamera"
                    >
                      <div
                        class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                        :style="{ left: mockSettings.requireSideCamera ? '22px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div
                    class="flex items-center justify-between py-3 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <div>
                      <p class="text-xs font-medium text-white/80">
                        Верификация лица (Face ID)
                      </p>
                      <p class="text-[10px] text-white/30">
                        Проверка личности перед началом
                      </p>
                    </div>
                    <button
                      class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                      :style="{ background: mockSettings.faceVerification ? '#4285F4' : 'rgba(255,255,255,0.15)' }"
                      @click="mockSettings.faceVerification = !mockSettings.faceVerification"
                    >
                      <div
                        class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                        :style="{ left: mockSettings.faceVerification ? '22px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div
                    class="flex items-center justify-between py-3 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <div>
                      <p class="text-xs font-medium text-white/80">
                        Dynamic Face ID
                      </p>
                      <p class="text-[10px] text-white/30">
                        Периодическая повторная проверка
                      </p>
                    </div>
                    <button
                      class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                      :style="{ background: mockSettings.dynamicFaceRecheck ? '#4285F4' : 'rgba(255,255,255,0.15)' }"
                      @click="mockSettings.dynamicFaceRecheck = !mockSettings.dynamicFaceRecheck"
                    >
                      <div
                        class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                        :style="{ left: mockSettings.dynamicFaceRecheck ? '22px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div
                    class="flex items-center justify-between py-3 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <div>
                      <p class="text-xs font-medium text-white/80">
                        Детекция телефона
                      </p>
                      <p class="text-[10px] text-white/30">
                        AI-обнаружение мобильных устройств
                      </p>
                    </div>
                    <button
                      class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                      :style="{ background: mockSettings.objectDetectionPhone ? '#4285F4' : 'rgba(255,255,255,0.15)' }"
                      @click="mockSettings.objectDetectionPhone = !mockSettings.objectDetectionPhone"
                    >
                      <div
                        class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                        :style="{ left: mockSettings.objectDetectionPhone ? '22px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div class="flex items-center justify-between py-3">
                    <div>
                      <p class="text-xs font-medium text-white/80">
                        Anti-Spoofing
                      </p>
                      <p class="text-[10px] text-white/30">
                        Защита от подмены лица
                      </p>
                    </div>
                    <button
                      class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                      :style="{ background: mockSettings.antiSpoofing ? '#4285F4' : 'rgba(255,255,255,0.15)' }"
                      @click="mockSettings.antiSpoofing = !mockSettings.antiSpoofing"
                    >
                      <div
                        class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                        :style="{ left: mockSettings.antiSpoofing ? '22px' : '2px' }"
                      />
                    </button>
                  </div>
                </div>
              </div>

              <!-- AI SENSITIVITY -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon
                    name="i-lucide-brain"
                    class="size-4"
                    style="color: #A259FF;"
                  />
                  <h3 class="text-xs font-bold text-white uppercase tracking-wider">
                    Чувствительность ИИ
                  </h3>
                </div>
                <div
                  class="py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div class="flex items-center justify-between mb-2">
                    <p class="text-xs font-medium text-white/80">
                      Порог детекции голоса
                    </p>
                    <span
                      class="text-xs font-bold tabular-nums"
                      style="color: #A259FF;"
                    >{{ mockSettings.voiceDetectionThreshold }}%</span>
                  </div>
                  <input
                    v-model.number="mockSettings.voiceDetectionThreshold"
                    type="range"
                    min="0"
                    max="100"
                    class="w-full h-1.5 rounded-full appearance-none cursor-pointer"
                    :style="{ background: `linear-gradient(to right, #A259FF ${mockSettings.voiceDetectionThreshold}%, rgba(255,255,255,0.1) ${mockSettings.voiceDetectionThreshold}%)` }"
                  >
                </div>
                <div
                  class="py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div class="flex items-center justify-between mb-2">
                    <p class="text-xs font-medium text-white/80">
                      Чувствительность взгляда
                    </p>
                    <span
                      class="text-xs font-bold tabular-nums"
                      style="color: #A259FF;"
                    >{{ mockSettings.gazeSensitivity }}%</span>
                  </div>
                  <input
                    v-model.number="mockSettings.gazeSensitivity"
                    type="range"
                    min="10"
                    max="95"
                    class="w-full h-1.5 rounded-full appearance-none cursor-pointer"
                    :style="{ background: `linear-gradient(to right, #A259FF ${(mockSettings.gazeSensitivity - 10) / 85 * 100}%, rgba(255,255,255,0.1) ${(mockSettings.gazeSensitivity - 10) / 85 * 100}%)` }"
                  >
                </div>
                <div class="py-3">
                  <div class="flex items-center justify-between mb-2">
                    <p class="text-xs font-medium text-white/80">
                      Лимит отклонения взгляда
                    </p>
                    <span
                      class="text-xs font-bold tabular-nums px-2 py-0.5 rounded"
                      :style="{ background: mockSettings.gazeDeviationLimitSec <= 5 ? 'rgba(234,67,53,0.15)' : 'rgba(255,255,255,0.06)', color: mockSettings.gazeDeviationLimitSec <= 5 ? '#EA4335' : 'white' }"
                    >
                      {{ mockSettings.gazeDeviationLimitSec }} сек
                    </span>
                  </div>
                  <input
                    v-model.number="mockSettings.gazeDeviationLimitSec"
                    type="range"
                    min="3"
                    max="30"
                    class="w-full h-1.5 rounded-full appearance-none cursor-pointer"
                    :style="{ background: `linear-gradient(to right, #A259FF ${(mockSettings.gazeDeviationLimitSec - 3) / 27 * 100}%, rgba(255,255,255,0.1) ${(mockSettings.gazeDeviationLimitSec - 3) / 27 * 100}%)` }"
                  >
                </div>
              </div>

              <!-- BROWSER LOCKDOWN -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon
                    name="i-lucide-monitor"
                    class="size-4"
                    style="color: #EA4335;"
                  />
                  <h3 class="text-xs font-bold text-white uppercase tracking-wider">
                    Ограничения браузера
                  </h3>
                </div>
                <div
                  class="py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div class="flex items-center justify-between mb-2">
                    <p class="text-xs font-medium text-white/80">
                      Лимит переключения вкладок
                    </p>
                    <span
                      class="text-xs font-bold tabular-nums px-2 py-0.5 rounded"
                      :style="{ background: mockSettings.tabSwitchingLimit === 0 ? 'rgba(234,67,53,0.15)' : 'rgba(255,255,255,0.06)', color: mockSettings.tabSwitchingLimit === 0 ? '#EA4335' : 'white' }"
                    >
                      {{ mockSettings.tabSwitchingLimit === 0 ? 'Запрещено' : mockSettings.tabSwitchingLimit }}
                    </span>
                  </div>
                  <input
                    v-model.number="mockSettings.tabSwitchingLimit"
                    type="range"
                    min="0"
                    max="10"
                    class="w-full h-1.5 rounded-full appearance-none cursor-pointer"
                    :style="{ background: `linear-gradient(to right, #EA4335 ${mockSettings.tabSwitchingLimit / 10 * 100}%, rgba(255,255,255,0.1) ${mockSettings.tabSwitchingLimit / 10 * 100}%)` }"
                  >
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <p class="text-xs font-medium text-white/80">
                    Блокировка буфера обмена
                  </p>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.blockCopyPaste ? '#EA4335' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.blockCopyPaste = !mockSettings.blockCopyPaste"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.blockCopyPaste ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <p class="text-xs font-medium text-white/80">
                    Блокировка PrintScreen
                  </p>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.blockPrintScreen ? '#EA4335' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.blockPrintScreen = !mockSettings.blockPrintScreen"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.blockPrintScreen ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <p class="text-xs font-medium text-white/80">
                    Блокировка виртуальных машин
                  </p>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.blockVirtualMachine ? '#EA4335' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.blockVirtualMachine = !mockSettings.blockVirtualMachine"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.blockVirtualMachine ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div class="flex items-center justify-between py-3">
                  <p class="text-xs font-medium text-white/80">
                    Блокировка удалённого доступа
                  </p>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.blockRemoteAccess ? '#EA4335' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.blockRemoteAccess = !mockSettings.blockRemoteAccess"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.blockRemoteAccess ? '22px' : '2px' }"
                    />
                  </button>
                </div>
              </div>

              <!-- KERNEL-LEVEL SYSTEM CONTROL -->
              <div>
                <div class="flex items-center gap-2 mb-3">
                  <UIcon
                    name="i-lucide-cpu"
                    class="size-4"
                    style="color: #FBBC05;"
                  />
                  <h3 class="text-xs font-bold text-white uppercase tracking-wider">
                    Системный контроль (Kernel-Level)
                  </h3>
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div>
                    <p class="text-xs font-medium text-white/80">
                      Контроль процессов
                    </p>
                    <p class="text-[10px] text-white/30">
                      AI-сканирование всех процессов ОС
                    </p>
                  </div>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.processScanning ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.processScanning = !mockSettings.processScanning"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.processScanning ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div>
                    <p class="text-xs font-medium text-white/80">
                      Детекция внешних устройств
                    </p>
                    <p class="text-[10px] text-white/30">
                      Блокировка USB-захвата, HDMI-карт
                    </p>
                  </div>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.hardwareDeviceDetection ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.hardwareDeviceDetection = !mockSettings.hardwareDeviceDetection"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.hardwareDeviceDetection ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div>
                    <p class="text-xs font-medium text-white/80">
                      Удалённый доступ (Advanced)
                    </p>
                    <p class="text-[10px] text-white/30">
                      TeamViewer, AnyDesk, VNC + скрипты
                    </p>
                  </div>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.advancedRemoteBlock ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.advancedRemoteBlock = !mockSettings.advancedRemoteBlock"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.advancedRemoteBlock ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div
                  class="flex items-center justify-between py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div>
                    <p class="text-xs font-medium text-white/80">
                      Hardware ID Binding
                    </p>
                    <p class="text-[10px] text-white/30">
                      Привязка к CPU/Motherboard ID
                    </p>
                  </div>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.hardwareIdBinding ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.hardwareIdBinding = !mockSettings.hardwareIdBinding"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.hardwareIdBinding ? '22px' : '2px' }"
                    />
                  </button>
                </div>
                <div class="flex items-center justify-between py-3">
                  <div>
                    <p class="text-xs font-medium text-white/80">
                      Deep Multi-Monitor Check
                    </p>
                    <p class="text-[10px] text-white/30">
                      100% детекция всех мониторов
                    </p>
                  </div>
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{ background: mockSettings.deepMultiMonitorCheck ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                    @click="mockSettings.deepMultiMonitorCheck = !mockSettings.deepMultiMonitorCheck"
                  >
                    <div
                      class="absolute top-0.5 size-4 rounded-full bg-white shadow transition-all"
                      :style="{ left: mockSettings.deepMultiMonitorCheck ? '22px' : '2px' }"
                    />
                  </button>
                </div>
              </div>
            </div>

            <!-- ===== TAB 2: INDIVIDUAL EXCEPTIONS ===== -->
            <div
              v-if="settingsTab === 'exceptions'"
              class="max-h-[560px] overflow-y-auto custom-scrollbar"
            >
              <!-- Search bar -->
              <div class="px-6 pt-5 pb-3">
                <div
                  v-if="!mockExcSelectedStudent"
                  class="relative"
                >
                  <UIcon
                    name="i-lucide-search"
                    class="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-white/30"
                  />
                  <input
                    v-model="excSearchQuery"
                    type="text"
                    placeholder="Поиск по ФИО, ИИН или причине..."
                    class="w-full pl-10 pr-4 py-2.5 rounded-xl text-xs text-white placeholder-white/30 outline-none"
                    style="background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.08);"
                  >
                </div>

                <!-- Selected student chip -->
                <div
                  v-else
                  class="rounded-xl border overflow-hidden"
                  style="border-color: rgba(162, 89, 255, 0.2);"
                >
                  <div
                    class="flex items-center justify-between gap-3 px-4 py-3"
                    style="background: rgba(162, 89, 255, 0.06);"
                  >
                    <div class="flex items-center gap-3">
                      <div
                        class="flex items-center justify-center size-9 rounded-lg"
                        style="background: rgba(162, 89, 255, 0.12);"
                      >
                        <UIcon
                          name="i-lucide-user"
                          class="size-4"
                          style="color: #A259FF;"
                        />
                      </div>
                      <div>
                        <div class="flex items-center gap-2">
                          <p class="text-xs font-bold text-white/90">
                            {{ mockExcSelectedStudent.name }}
                          </p>
                          <span
                            class="inline-flex items-center gap-1 text-[7px] font-bold px-1.5 py-0.5 rounded"
                            style="background: rgba(230, 126, 34, 0.12); color: #E67E22;"
                          >
                            <UIcon
                              name="i-lucide-pen-line"
                              class="size-2.5"
                            />
                            Manual Override
                          </span>
                        </div>
                        <p class="text-[10px] font-mono text-white/30">
                          {{ mockExcSelectedStudent.studentId }}
                        </p>
                      </div>
                    </div>
                    <div class="flex items-center gap-2">
                      <span
                        v-if="mockExcDiffCount > 0"
                        class="text-[9px] font-bold px-2 py-0.5 rounded-full"
                        style="background: rgba(162, 89, 255, 0.12); color: #A259FF;"
                      >{{ mockExcDiffCount }} отличий</span>
                      <button
                        class="flex items-center justify-center size-7 rounded-lg cursor-pointer text-white/30 hover:text-white/60 hover:bg-white/5 transition-colors"
                        @click="clearMockException"
                      >
                        <UIcon
                          name="i-lucide-x"
                          class="size-3.5"
                        />
                      </button>
                    </div>
                  </div>
                  <!-- Custom Settings + Reset to Global -->
                  <div
                    class="flex items-center justify-between px-4 py-2 border-t"
                    style="border-color: rgba(162, 89, 255, 0.1); background: rgba(162, 89, 255, 0.03);"
                  >
                    <div class="flex items-center gap-1.5">
                      <UIcon
                        name="i-lucide-sliders-horizontal"
                        class="size-3"
                        style="color: #A259FF;"
                      />
                      <span
                        class="text-[10px] font-semibold"
                        style="color: #A259FF;"
                      >Custom Settings</span>
                      <span
                        v-if="mockExcDiffCount > 0"
                        class="text-[9px] text-white/30"
                      >— {{ mockExcDiffCount }} параметров отличаются</span>
                    </div>
                    <button
                      v-if="mockExcDiffCount > 0"
                      class="flex items-center gap-1 px-2 py-1 rounded-md text-[9px] font-bold text-white/30 cursor-pointer hover:text-white/60 hover:bg-white/5 transition-all"
                      @click="resetMockExcToGlobal"
                    >
                      <UIcon
                        name="i-lucide-rotate-ccw"
                        class="size-3"
                      />
                      Reset to Global
                    </button>
                  </div>
                </div>
              </div>

              <!-- When student selected: Show mirrored toggles -->
              <div
                v-if="mockExcSelectedStudent"
                class="px-6 pb-5 space-y-4"
              >
                <div>
                  <div class="flex items-center gap-2 mb-2">
                    <UIcon
                      name="i-lucide-video"
                      class="size-3.5"
                      style="color: #A259FF;"
                    />
                    <h4 class="text-[10px] font-bold text-white/50 uppercase tracking-wider">
                      Видео-правила
                    </h4>
                  </div>
                  <div
                    class="flex items-center justify-between py-2.5 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <p class="text-[11px] font-medium text-white/70">
                      Боковая камера
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.requireSideCamera ? '#A259FF' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.requireSideCamera = !mockExcSettings.requireSideCamera"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.requireSideCamera ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div
                    class="flex items-center justify-between py-2.5 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <p class="text-[11px] font-medium text-white/70">
                      Face ID
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.faceVerification ? '#A259FF' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.faceVerification = !mockExcSettings.faceVerification"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.faceVerification ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div
                    class="flex items-center justify-between py-2.5 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <p class="text-[11px] font-medium text-white/70">
                      Dynamic Face ID
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.dynamicFaceRecheck ? '#A259FF' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.dynamicFaceRecheck = !mockExcSettings.dynamicFaceRecheck"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.dynamicFaceRecheck ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div class="flex items-center justify-between py-2.5">
                    <p class="text-[11px] font-medium text-white/70">
                      Anti-Spoofing
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.antiSpoofing ? '#A259FF' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.antiSpoofing = !mockExcSettings.antiSpoofing"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.antiSpoofing ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                </div>

                <div>
                  <div class="flex items-center gap-2 mb-2">
                    <UIcon
                      name="i-lucide-cpu"
                      class="size-3.5"
                      style="color: #FBBC05;"
                    />
                    <h4 class="text-[10px] font-bold text-white/50 uppercase tracking-wider">
                      Kernel-Level
                    </h4>
                  </div>
                  <div
                    class="flex items-center justify-between py-2.5 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <p class="text-[11px] font-medium text-white/70">
                      Скан. процессов
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.processScanning ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.processScanning = !mockExcSettings.processScanning"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.processScanning ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div
                    class="flex items-center justify-between py-2.5 border-b"
                    style="border-color: rgba(255,255,255,0.04);"
                  >
                    <p class="text-[11px] font-medium text-white/70">
                      Hardware ID
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.hardwareIdBinding ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.hardwareIdBinding = !mockExcSettings.hardwareIdBinding"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.hardwareIdBinding ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                  <div class="flex items-center justify-between py-2.5">
                    <p class="text-[11px] font-medium text-white/70">
                      Deep Multi-Monitor
                    </p>
                    <button
                      class="relative w-9 h-[18px] rounded-full transition-all cursor-pointer"
                      :style="{ background: mockExcSettings.deepMultiMonitorCheck ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                      @click="mockExcSettings.deepMultiMonitorCheck = !mockExcSettings.deepMultiMonitorCheck"
                    >
                      <div
                        class="absolute top-[2px] size-[14px] rounded-full bg-white shadow transition-all"
                        :style="{ left: mockExcSettings.deepMultiMonitorCheck ? '19px' : '2px' }"
                      />
                    </button>
                  </div>
                </div>
              </div>

              <!-- When no student selected: Exception list -->
              <template v-if="!mockExcSelectedStudent">
                <div class="px-6 py-2 flex items-center justify-between">
                  <span class="text-[10px] text-white/30 uppercase tracking-wider">Студенты с исключениями</span>
                  <span
                    class="text-[10px] font-bold"
                    style="color: #A259FF;"
                  >{{ filteredMockExceptions.length }} из {{ mockExceptions.length }}</span>
                </div>

                <div class="px-6 pb-4 space-y-3">
                  <div
                    v-for="(exc, idx) in filteredMockExceptions"
                    :key="idx"
                    class="rounded-xl border transition-all cursor-pointer overflow-hidden"
                    style="border-color: rgba(255,255,255,0.06);"
                    @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'rgba(162,89,255,0.25)'"
                    @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'rgba(255,255,255,0.06)'"
                    @click="selectMockException(exc)"
                  >
                    <div
                      class="flex items-start justify-between p-4"
                      style="background: rgba(255,255,255,0.02);"
                    >
                      <div class="flex items-center gap-3">
                        <div
                          class="flex items-center justify-center size-9 rounded-lg shrink-0"
                          style="background: rgba(162, 89, 255, 0.08);"
                        >
                          <UIcon
                            name="i-lucide-user-cog"
                            class="size-4"
                            style="color: #A259FF;"
                          />
                        </div>
                        <div>
                          <div class="flex items-center gap-2">
                            <p class="text-xs font-bold text-white/80">
                              {{ exc.name }}
                            </p>
                            <span
                              class="inline-flex items-center gap-1 text-[7px] font-bold px-1.5 py-0.5 rounded"
                              style="background: rgba(230, 126, 34, 0.12); color: #E67E22;"
                            >
                              <UIcon
                                name="i-lucide-pen-line"
                                class="size-2"
                              />
                              Manual Override
                            </span>
                          </div>
                          <p class="text-[10px] font-mono text-white/25">
                            ИИН: {{ exc.studentId }}
                          </p>
                        </div>
                      </div>
                      <span
                        class="text-[8px] font-bold px-2 py-0.5 rounded-full shrink-0"
                        style="background: rgba(162, 89, 255, 0.1); color: #A259FF;"
                      >
                        {{ Object.keys(exc.overrides).length }} переопр.
                      </span>
                    </div>
                    <!-- Custom Settings summary row -->
                    <div
                      class="flex items-center gap-2 px-4 py-2 border-t"
                      style="border-color: rgba(255,255,255,0.04); background: rgba(162, 89, 255, 0.02);"
                    >
                      <span
                        class="text-[8px] font-bold uppercase tracking-wider shrink-0"
                        style="color: #A259FF;"
                      >Custom Settings</span>
                      <div class="flex flex-wrap gap-1">
                        <span
                          v-for="(val, key) in exc.overrides"
                          :key="String(key)"
                          class="inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-[7px] font-bold"
                          :style="{
                            background: typeof val === 'boolean' && !val ? 'rgba(234, 67, 53, 0.08)' : 'rgba(66, 133, 244, 0.08)',
                            color: typeof val === 'boolean' && !val ? '#EA4335' : '#4285F4'
                          }"
                        >
                          {{ String(key) }}: {{ typeof val === 'boolean' ? (val ? 'ВКЛ' : 'ВЫКЛ') : val }}
                        </span>
                      </div>
                    </div>
                  </div>

                  <div
                    v-if="filteredMockExceptions.length === 0"
                    class="text-center py-8"
                  >
                    <UIcon
                      name="i-lucide-search-x"
                      class="size-8 text-white/15 mx-auto mb-2"
                    />
                    <p class="text-xs text-white/30">
                      Исключений не найдено
                    </p>
                  </div>

                  <button
                    class="w-full py-3 rounded-xl border-2 border-dashed text-xs font-medium text-white/25 cursor-pointer transition-all hover:border-[rgba(162,89,255,0.3)] hover:text-white/40"
                    style="border-color: rgba(255,255,255,0.06);"
                  >
                    <UIcon
                      name="i-lucide-plus"
                      class="size-4 inline-block mr-1"
                    />
                    Добавить исключение
                  </button>
                </div>
              </template>
            </div>

            <!-- Settings footer -->
            <div
              class="px-6 py-4 border-t"
              style="border-color: rgba(255,255,255,0.06);"
            >
              <p class="text-[11px] text-white/40 text-center leading-relaxed">
                <UIcon
                  name="i-lucide-shield-check"
                  class="size-3.5 inline-block mr-1"
                  style="color: #34A853;"
                />
                Политика вашего экзамена: Полный контроль над AI-детекцией и безопасностью браузера. Гибкая настройка для любой дисциплины.
              </p>
            </div>
          </div>

          <!-- ========== RIGHT: "Why Argus AI?" Killer Features (4 cols) ========== -->
          <div
            class="lg:col-span-4 space-y-4 transition-all duration-1000"
            :class="settingsVisible ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-8'"
            style="transition-delay: 300ms;"
          >
            <!-- Section header -->
            <div class="mb-2">
              <h3 class="text-lg font-bold text-white flex items-center gap-2">
                <UIcon
                  name="i-lucide-sparkles"
                  class="size-5"
                  style="color: #FBBC05;"
                />
                Почему Argus AI?
              </h3>
              <p class="text-[11px] text-white/30 mt-1">
                Шесть причин выбрать Argus AI
              </p>
            </div>

            <!-- Killer feature cards -->
            <div
              v-for="(feat, idx) in killerFeatures"
              :key="feat.title"
              class="group p-4 rounded-xl border transition-all duration-700 hover:scale-[1.02]"
              :class="settingsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-6'"
              :style="{
                transitionDelay: `${500 + idx * 120}ms`,
                background: '#1A2130',
                borderColor: 'rgba(255,255,255,0.06)',
                borderLeft: `3px solid ${feat.color}`
              }"
            >
              <div class="flex items-start gap-3">
                <div
                  class="flex items-center justify-center size-9 rounded-lg shrink-0 transition-all group-hover:scale-110"
                  :style="{ background: hexToRgba(feat.color, 0.08), border: `1px solid ${hexToRgba(feat.color, 0.15)}` }"
                >
                  <UIcon
                    :name="feat.icon"
                    class="size-4"
                    :style="{ color: feat.color }"
                  />
                </div>
                <div>
                  <h4 class="text-xs font-bold text-white/80">
                    {{ feat.title }}
                  </h4>
                  <p class="text-[10px] text-white/40 mt-1 leading-relaxed">
                    {{ feat.description }}
                  </p>
                </div>
              </div>
            </div>

            <!-- Enterprise pricing callout -->
            <div
              class="p-4 rounded-xl border transition-all duration-1000"
              :class="settingsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-6'"
              style="background: linear-gradient(135deg, rgba(52, 168, 83, 0.04), rgba(66, 133, 244, 0.04)); border-color: rgba(52, 168, 83, 0.15); transition-delay: 1100ms;"
            >
              <div class="flex items-center gap-3 mb-3">
                <div
                  class="flex items-center justify-center size-9 rounded-lg shrink-0"
                  style="background: rgba(52, 168, 83, 0.1);"
                >
                  <UIcon
                    name="i-lucide-badge-dollar-sign"
                    class="size-4"
                    style="color: #34A853;"
                  />
                </div>
                <div>
                  <p class="text-xs font-bold text-white/80">
                    Enterprise: от <span style="color: #34A853;">$1.00</span>
                  </p>
                  <p class="text-[9px] text-white/30">
                    за студента / экзаменационный цикл
                  </p>
                </div>
              </div>
              <a
                href="#pricing"
                class="flex items-center gap-1.5 text-[10px] font-bold cursor-pointer transition-colors hover:opacity-80"
                style="color: #4285F4;"
              >
                Рассчитать стоимость
                <UIcon
                  name="i-lucide-arrow-right"
                  class="size-3"
                />
              </a>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ============================================ -->
    <!--  INTERACTIVE ARCHIVE SESSION SHOWCASE        -->
    <!-- ============================================ -->
    <section
      id="archive-demo"
      ref="archiveRef"
      class="relative py-32 px-6"
    >
      <div class="max-w-7xl mx-auto">
        <div class="text-center mb-16">
          <span
            class="text-xs font-bold uppercase tracking-widest"
            style="color: #34A853;"
          >Архив сессий</span>
          <h2 class="text-4xl md:text-5xl font-bold text-white mt-4 tracking-tight">
            Полная прозрачность каждой сессии
          </h2>
          <p class="text-lg text-white/40 mt-4 max-w-2xl mx-auto">
            Просматривайте записи, анализируйте события, слушайте аудио — в любой момент
          </p>
        </div>

        <!-- ========== MINI SESSION TABLE ========== -->
        <div
          class="relative rounded-2xl border overflow-hidden transition-all duration-1000 mb-6"
          :class="archiveVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-16'"
          style="background: #1A2130; border-color: rgba(52, 168, 83, 0.15);"
        >
          <!-- Table header -->
          <div
            class="grid grid-cols-12 gap-0 px-5 py-3 border-b"
            style="border-color: rgba(255,255,255,0.06); background: rgba(0,0,0,0.15);"
          >
            <div class="col-span-3">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider">Студент</span>
            </div>
            <div class="col-span-2">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider">Экзамен</span>
            </div>
            <div class="col-span-2 text-center">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider">Integrity</span>
            </div>
            <div class="col-span-1 text-center">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider">Наруш.</span>
            </div>
            <div class="col-span-2 text-center">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider">Статус</span>
            </div>
            <div class="col-span-1 text-center">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider">Длит.</span>
            </div>
            <div class="col-span-1 text-right">
              <span class="text-[9px] font-bold text-white/30 uppercase tracking-wider" />
            </div>
          </div>

          <!-- Table rows -->
          <div
            v-for="(session, idx) in archiveMockSessions"
            :key="session.id"
            class="grid grid-cols-12 gap-0 px-5 py-3 border-b items-center cursor-pointer transition-all duration-300 hover:bg-white/[0.02]"
            :class="[
              archiveVisible ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-4',
              (archiveSelectedSession === session.id || (archiveSelectedSession === null && session.id === 0)) ? 'bg-white/[0.03]' : ''
            ]"
            :style="{
              borderColor: 'rgba(255,255,255,0.04)',
              transitionDelay: `${200 + idx * 100}ms`,
              borderLeft: (archiveSelectedSession === session.id || (archiveSelectedSession === null && session.id === 0)) ? '2px solid #4285F4' : '2px solid transparent'
            }"
            @click="archiveSelectSession(session.id)"
          >
            <!-- Name + IIN -->
            <div class="col-span-3">
              <div class="flex items-center gap-2">
                <div
                  class="size-7 rounded-full flex items-center justify-center shrink-0"
                  style="background: rgba(66, 133, 244, 0.1);"
                >
                  <span
                    class="text-[9px] font-bold"
                    style="color: #4285F4;"
                  >{{ session.name.charAt(0) }}</span>
                </div>
                <div>
                  <p class="text-[11px] font-semibold text-white/80 leading-tight">
                    {{ session.name }}
                  </p>
                  <p class="text-[8px] font-mono text-white/20">
                    {{ session.iin }}
                  </p>
                </div>
              </div>
            </div>
            <!-- Exam -->
            <div class="col-span-2">
              <span class="text-[10px] text-white/50">{{ session.exam }}</span>
            </div>
            <!-- Integrity with mini bar -->
            <div class="col-span-2 flex flex-col items-center gap-1">
              <span
                class="text-[11px] font-bold tabular-nums"
                :style="{ color: integrityColor(session.integrity) }"
              >{{ session.integrity }}%</span>
              <div
                class="w-full max-w-[60px] h-1 rounded-full overflow-hidden"
                style="background: rgba(255,255,255,0.06);"
              >
                <div
                  class="h-full rounded-full transition-all duration-700"
                  :style="{ width: `${session.integrity}%`, background: integrityColor(session.integrity) }"
                />
              </div>
            </div>
            <!-- Violations -->
            <div class="col-span-1 text-center">
              <span
                class="text-[10px] font-bold px-2 py-0.5 rounded-full"
                :style="{
                  background: session.violations > 5 ? 'rgba(234, 67, 53, 0.12)' : session.violations > 2 ? 'rgba(251, 188, 5, 0.12)' : 'rgba(52, 168, 83, 0.1)',
                  color: session.violations > 5 ? '#EA4335' : session.violations > 2 ? '#FBBC05' : '#34A853'
                }"
              >{{ session.violations }}</span>
            </div>
            <!-- Status -->
            <div class="col-span-2 text-center">
              <span
                class="text-[9px] font-bold px-2.5 py-1 rounded-full inline-flex items-center gap-1"
                :style="{ background: `${sessionStatusColor(session.status)}15`, color: sessionStatusColor(session.status) }"
              >
                <UIcon
                  :name="session.status === 'reviewed' ? 'i-lucide-check-circle' : session.status === 'pending' ? 'i-lucide-clock' : 'i-lucide-x-circle'"
                  class="size-2.5"
                />
                {{ sessionStatusLabel(session.status) }}
              </span>
            </div>
            <!-- Duration -->
            <div class="col-span-1 text-center">
              <span class="text-[10px] font-mono text-white/30">{{ session.duration }}</span>
            </div>
            <!-- Action -->
            <div class="col-span-1 text-right">
              <button
                class="size-7 rounded-lg flex items-center justify-center cursor-pointer transition-all hover:scale-110"
                style="background: rgba(66, 133, 244, 0.1);"
              >
                <UIcon
                  name="i-lucide-play"
                  class="size-3.5"
                  style="color: #4285F4;"
                />
              </button>
            </div>
          </div>
        </div>

        <!-- ========== SESSION DETAIL PANEL ========== -->
        <div
          class="relative rounded-2xl border overflow-hidden transition-all duration-1000"
          :class="archiveVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-16'"
          style="background: #1A2130; border-color: rgba(52, 168, 83, 0.15); transition-delay: 300ms;"
        >
          <!-- Detail header bar -->
          <div
            class="flex items-center justify-between px-6 py-4 border-b"
            style="border-color: rgba(255,255,255,0.06);"
          >
            <div class="flex items-center gap-3">
              <div
                class="flex items-center justify-center size-8 rounded-lg"
                style="background: rgba(52, 168, 83, 0.1);"
              >
                <UIcon
                  name="i-lucide-film"
                  class="size-4"
                  style="color: #34A853;"
                />
              </div>
              <div class="flex items-center gap-2">
                <span
                  class="text-[9px] font-bold px-2 py-0.5 rounded-full inline-flex items-center gap-1"
                  :style="{ background: `${sessionStatusColor(archiveSelectedSessionData!.status)}15`, color: sessionStatusColor(archiveSelectedSessionData!.status) }"
                >
                  <UIcon
                    :name="archiveSelectedSessionData!.status === 'reviewed' ? 'i-lucide-check-circle' : archiveSelectedSessionData!.status === 'pending' ? 'i-lucide-clock' : 'i-lucide-x-circle'"
                    class="size-2.5"
                  />
                  {{ sessionStatusLabel(archiveSelectedSessionData!.status) }}
                </span>
                <span class="text-sm font-bold text-white">{{ archiveSelectedSessionData!.name }}</span>
                <span class="text-[10px] text-white/30">{{ archiveSelectedSessionData!.exam }}</span>
              </div>
              <!-- Manual Override badge -->
              <span
                v-if="archiveSelectedSessionData!.hasException"
                class="text-[8px] font-bold px-2 py-0.5 rounded-full inline-flex items-center gap-1"
                style="background: rgba(49, 130, 206, 0.12); color: #3182CE;"
              >
                <UIcon
                  name="i-lucide-shield-alert"
                  class="size-2.5"
                />
                Manual Override ({{ archiveSelectedSessionData!.exceptionCount }})
              </span>
            </div>
            <div class="flex items-center gap-2">
              <span
                class="text-[9px] font-bold px-2 py-0.5 rounded-full"
                style="background: rgba(234, 67, 53, 0.15); color: #EA4335;"
              >{{ archiveEvents.filter(e => e.severity === 'critical').length }} критич.</span>
              <span
                class="text-[9px] font-bold px-2 py-0.5 rounded-full"
                style="background: rgba(251, 188, 5, 0.15); color: #FBBC05;"
              >{{ archiveEvents.filter(e => e.severity === 'warning').length }} предупр.</span>
              <span class="text-[10px] text-white/30 font-mono">{{ archiveSelectedSessionData!.date }}</span>
            </div>
          </div>

          <!-- Detail tabs -->
          <div
            class="flex items-center gap-0 border-b"
            style="border-color: rgba(255,255,255,0.06);"
          >
            <button
              class="px-5 py-2.5 text-[10px] font-bold uppercase tracking-wider transition-all cursor-pointer"
              :style="{
                color: archiveDetailTab === 'video' ? '#4285F4' : 'rgba(255,255,255,0.3)',
                borderBottom: archiveDetailTab === 'video' ? '2px solid #4285F4' : '2px solid transparent',
                background: archiveDetailTab === 'video' ? 'rgba(66, 133, 244, 0.04)' : 'transparent'
              }"
              @click="archiveDetailTab = 'video'"
            >
              <UIcon
                name="i-lucide-video"
                class="size-3 inline-block mr-1"
              />
              Воспроизведение
            </button>
            <button
              class="px-5 py-2.5 text-[10px] font-bold uppercase tracking-wider transition-all cursor-pointer"
              :style="{
                color: archiveDetailTab === 'kernel' ? '#FBBC05' : 'rgba(255,255,255,0.3)',
                borderBottom: archiveDetailTab === 'kernel' ? '2px solid #FBBC05' : '2px solid transparent',
                background: archiveDetailTab === 'kernel' ? 'rgba(251, 188, 5, 0.04)' : 'transparent'
              }"
              @click="archiveDetailTab = 'kernel'"
            >
              <UIcon
                name="i-lucide-cpu"
                class="size-3 inline-block mr-1"
              />
              Kernel-Level
            </button>
          </div>

          <!-- ===== VIDEO TAB ===== -->
          <div v-show="archiveDetailTab === 'video'">
            <!-- 7-col grid: 5 left (video+audio), 2 right (event log) -->
            <div
              class="grid grid-cols-1 md:grid-cols-7 gap-0"
              style="min-height: 380px;"
            >
              <!-- LEFT: 5 cols — Video + Audio + Kernel mini -->
              <div
                class="md:col-span-5 flex flex-col"
                style="border-right: 1px solid rgba(255,255,255,0.04);"
              >
                <!-- Dual camera row -->
                <div
                  class="flex gap-0 flex-1"
                  style="min-height: 260px;"
                >
                  <!-- Main camera (flex-3) -->
                  <div
                    class="relative transition-all duration-500"
                    :class="archiveCamerasSwapped ? 'flex-1' : 'flex-[3]'"
                    :style="{
                      background: '#0D1117',
                      borderRight: '1px solid rgba(255,255,255,0.04)',
                      boxShadow: archiveHighlightedCamera === (archiveCamerasSwapped ? 'side' : 'webcam') ? 'inset 0 0 30px rgba(66, 133, 244, 0.15)' : 'none'
                    }"
                  >
                    <div class="absolute inset-0 flex items-center justify-center">
                      <div class="flex flex-col items-center gap-3">
                        <div
                          class="scanning-circle size-20 rounded-full border-2 flex items-center justify-center"
                          :style="{ borderColor: archiveCamerasSwapped ? 'rgba(162, 89, 255, 0.3)' : 'rgba(66, 133, 244, 0.3)' }"
                        >
                          <UIcon
                            :name="archiveCamerasSwapped ? 'i-lucide-smartphone' : 'i-lucide-video'"
                            class="size-8 text-white/20"
                          />
                        </div>
                        <span class="text-[9px] font-mono text-white/30">{{ archiveCamerasSwapped ? 'SIDE CAM \u00b7 720p' : 'WEBCAM \u00b7 1080p \u00b7 30fps' }}</span>
                      </div>
                    </div>
                    <!-- Overlays -->
                    <div
                      class="absolute top-3 left-3 flex items-center gap-1.5 px-2 py-1 rounded"
                      style="background: rgba(0,0,0,0.6);"
                    >
                      <div
                        class="size-2 rounded-full animate-pulse"
                        style="background: #EA4335;"
                      />
                      <span class="text-[8px] font-mono text-white/70">REC</span>
                    </div>
                    <div
                      class="absolute top-3 right-3 flex items-center gap-1 px-2 py-1 rounded"
                      style="background: rgba(0,0,0,0.6);"
                    >
                      <UIcon
                        name="i-lucide-shield-check"
                        class="size-2.5"
                        style="color: #34A853;"
                      />
                      <span class="text-[8px] font-bold text-white/60">AI ACTIVE</span>
                    </div>
                    <!-- Integrity badge on video -->
                    <div
                      class="absolute top-3 left-1/2 -translate-x-1/2 flex items-center gap-1.5 px-2.5 py-1 rounded-full"
                      style="background: rgba(0,0,0,0.7);"
                    >
                      <span
                        class="text-[9px] font-bold"
                        :style="{ color: integrityColor(archiveSelectedSessionData!.integrity) }"
                      >{{ archiveSelectedSessionData!.integrity }}%</span>
                      <span class="text-[7px] text-white/30">Integrity</span>
                    </div>
                    <div class="absolute bottom-3 left-3 flex items-center gap-2">
                      <span
                        class="text-[8px] font-bold px-1.5 py-0.5 rounded"
                        :style="{ background: archiveCamerasSwapped ? 'rgba(162, 89, 255, 0.2)' : 'rgba(66, 133, 244, 0.2)', color: archiveCamerasSwapped ? '#A259FF' : '#4285F4' }"
                      >{{ archiveCamerasSwapped ? '\u0411\u041e\u041a\u041e\u0412\u0410\u042f' : '\u041e\u0421\u041d\u041e\u0412\u041d\u0410\u042f' }}</span>
                      <span
                        class="text-[7px] font-mono px-1.5 py-0.5 rounded text-white/30"
                        style="background: rgba(0,0,0,0.6);"
                      >{{ archiveCurrentTime }}</span>
                    </div>
                    <!-- Face bounding box overlay (main cam only) -->
                    <div
                      v-if="!archiveCamerasSwapped"
                      class="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-24 h-28 border border-dashed rounded-lg"
                      style="border-color: rgba(66, 133, 244, 0.25);"
                    >
                      <div
                        class="absolute -top-1 -left-1 w-2.5 h-2.5 border-l-2 border-t-2"
                        style="border-color: #4285F4;"
                      />
                      <div
                        class="absolute -top-1 -right-1 w-2.5 h-2.5 border-r-2 border-t-2"
                        style="border-color: #4285F4;"
                      />
                      <div
                        class="absolute -bottom-1 -left-1 w-2.5 h-2.5 border-l-2 border-b-2"
                        style="border-color: #4285F4;"
                      />
                      <div
                        class="absolute -bottom-1 -right-1 w-2.5 h-2.5 border-r-2 border-b-2"
                        style="border-color: #4285F4;"
                      />
                    </div>
                  </div>

                  <!-- Side camera (flex-1) -->
                  <div
                    class="relative transition-all duration-500"
                    :class="archiveCamerasSwapped ? 'flex-[3]' : 'flex-1'"
                    :style="{
                      background: archiveShowSide ? '#0D1117' : '#080B10',
                      boxShadow: archiveHighlightedCamera === (archiveCamerasSwapped ? 'webcam' : 'side') ? 'inset 0 0 30px rgba(162, 89, 255, 0.15)' : 'none'
                    }"
                  >
                    <div
                      v-if="archiveShowSide"
                      class="absolute inset-0 flex items-center justify-center"
                    >
                      <div class="flex flex-col items-center gap-2">
                        <div
                          class="scanning-circle-delayed size-14 rounded-full border-2 flex items-center justify-center"
                          :style="{ borderColor: archiveCamerasSwapped ? 'rgba(66, 133, 244, 0.3)' : 'rgba(162, 89, 255, 0.3)' }"
                        >
                          <UIcon
                            :name="archiveCamerasSwapped ? 'i-lucide-video' : 'i-lucide-smartphone'"
                            class="size-6 text-white/20"
                          />
                        </div>
                        <span class="text-[8px] font-mono text-white/30">{{ archiveCamerasSwapped ? 'WEBCAM \u00b7 1080p' : 'SIDE CAM \u00b7 720p' }}</span>
                      </div>
                    </div>
                    <div
                      v-else
                      class="absolute inset-0 flex items-center justify-center"
                    >
                      <div class="flex flex-col items-center gap-1 opacity-30">
                        <UIcon
                          name="i-lucide-camera-off"
                          class="size-8 text-white"
                        />
                        <span class="text-[8px] text-white/60">Отключена</span>
                      </div>
                    </div>
                    <!-- Overlays -->
                    <div
                      class="absolute top-2 left-2 flex items-center gap-1 px-1.5 py-0.5 rounded"
                      style="background: rgba(0,0,0,0.6);"
                    >
                      <div
                        class="size-1.5 rounded-full"
                        :style="{ background: archiveCamerasSwapped ? '#4285F4' : '#A259FF' }"
                      />
                      <span class="text-[7px] font-mono text-white/70">REC</span>
                    </div>
                    <div class="absolute bottom-2 left-2">
                      <span
                        class="text-[7px] font-bold px-1.5 py-0.5 rounded"
                        :style="{ background: archiveCamerasSwapped ? 'rgba(66, 133, 244, 0.2)' : 'rgba(162, 89, 255, 0.2)', color: archiveCamerasSwapped ? '#4285F4' : '#A259FF' }"
                      >{{ archiveCamerasSwapped ? '\u041e\u0421\u041d\u041e\u0412\u041d\u0410\u042f' : '\u0411\u041e\u041a\u041e\u0412\u0410\u042f' }}</span>
                    </div>
                    <button
                      class="absolute top-2 right-2 flex items-center gap-1 px-1.5 py-0.5 rounded cursor-pointer transition-all hover:opacity-80"
                      style="background: rgba(0,0,0,0.6);"
                      @click="archiveShowSide = !archiveShowSide"
                    >
                      <UIcon
                        :name="archiveShowSide ? 'i-lucide-eye' : 'i-lucide-eye-off'"
                        class="size-2.5 text-white/60"
                      />
                    </button>
                  </div>
                </div>

                <!-- Camera swap button -->
                <div
                  class="px-4 py-2 border-t flex items-center justify-between"
                  style="border-color: rgba(255,255,255,0.04); background: rgba(0,0,0,0.1);"
                >
                  <button
                    class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg cursor-pointer transition-all hover:scale-105"
                    style="background: rgba(255,255,255,0.04); border: 1px solid rgba(255,255,255,0.06);"
                    @click="archiveCamerasSwapped = !archiveCamerasSwapped"
                  >
                    <UIcon
                      name="i-lucide-arrow-left-right"
                      class="size-3 text-white/40"
                    />
                    <span class="text-[9px] font-bold text-white/40">Поменять камеры</span>
                  </button>
                  <div class="flex items-center gap-3">
                    <span class="text-[8px] text-white/20 font-mono">{{ archiveSelectedSessionData!.date }}</span>
                    <span class="text-[8px] text-white/20 font-mono">{{ archiveSelectedSessionData!.duration }}</span>
                  </div>
                </div>

                <!-- Audio analytics section -->
                <div
                  class="px-4 py-3 border-t"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div class="flex items-center gap-3">
                    <UIcon
                      name="i-lucide-audio-waveform"
                      class="size-3.5 text-white/40 shrink-0"
                    />
                    <div class="flex-1 h-8 flex items-end gap-[2px]">
                      <div
                        v-for="(bar, i) in waveformBars"
                        :key="i"
                        class="flex-1 rounded-t transition-all duration-200"
                        :style="{
                          height: `${archivePlaying ? bar : bar * 0.3}%`,
                          background: i < waveformBars.length * (archiveProgress / 100)
                            ? 'linear-gradient(to top, #4285F4, #A259FF)'
                            : 'rgba(255,255,255,0.1)',
                          minHeight: '2px'
                        }"
                      />
                    </div>
                    <!-- Noise level -->
                    <div class="flex flex-col items-center gap-0.5 shrink-0 px-2">
                      <span class="text-[7px] text-white/25 uppercase">Шум</span>
                      <span
                        class="text-[10px] font-bold tabular-nums"
                        :style="{ color: archiveNoiseLevel > 60 ? '#EA4335' : archiveNoiseLevel > 40 ? '#FBBC05' : '#34A853' }"
                      >{{ archiveNoiseLevel }}dB</span>
                    </div>
                    <!-- Mute toggle -->
                    <button
                      class="size-7 rounded-lg flex items-center justify-center cursor-pointer transition-all shrink-0"
                      :style="{ background: archiveMuted ? 'rgba(234, 67, 53, 0.1)' : 'rgba(66, 133, 244, 0.1)' }"
                      @click="archiveMuted = !archiveMuted"
                    >
                      <UIcon
                        :name="archiveMuted ? 'i-lucide-volume-x' : 'i-lucide-volume-2'"
                        class="size-3.5"
                        :style="{ color: archiveMuted ? '#EA4335' : '#4285F4' }"
                      />
                    </button>
                    <!-- Volume slider -->
                    <div class="flex items-center gap-1.5 shrink-0">
                      <input
                        v-model.number="archiveVolume"
                        type="range"
                        min="0"
                        max="100"
                        class="w-16 h-1 rounded-full appearance-none cursor-pointer"
                        :style="{ background: `linear-gradient(to right, #4285F4 ${archiveVolume}%, rgba(255,255,255,0.1) ${archiveVolume}%)` }"
                      >
                    </div>
                  </div>
                </div>

                <!-- Kernel-Level Status mini row -->
                <div
                  class="px-4 py-2.5 border-t"
                  style="border-color: rgba(255,255,255,0.04); background: rgba(251, 188, 5, 0.02);"
                >
                  <div class="flex items-center gap-2 mb-2">
                    <UIcon
                      name="i-lucide-cpu"
                      class="size-3"
                      style="color: #FBBC05;"
                    />
                    <span
                      class="text-[8px] font-bold uppercase tracking-wider"
                      style="color: rgba(251, 188, 5, 0.6);"
                    >Kernel-Level</span>
                  </div>
                  <div class="flex items-center gap-3 flex-wrap">
                    <div
                      v-for="(val, key) in archiveKernelStatus"
                      :key="key"
                      class="flex items-center gap-1.5 px-2 py-1 rounded-md cursor-pointer transition-all"
                      :style="{
                        background: val ? 'rgba(251, 188, 5, 0.08)' : 'rgba(255,255,255,0.02)',
                        border: `1px solid ${val ? 'rgba(251, 188, 5, 0.15)' : 'rgba(255,255,255,0.04)'}`
                      }"
                      @click="(archiveKernelStatus as any)[key] = !(archiveKernelStatus as any)[key]"
                    >
                      <div
                        class="size-1.5 rounded-full"
                        :style="{ background: val ? '#FBBC05' : 'rgba(255,255,255,0.15)' }"
                      />
                      <span
                        class="text-[8px] font-bold"
                        :style="{ color: val ? '#FBBC05' : 'rgba(255,255,255,0.25)' }"
                      >{{ archiveKernelLabels[key] || key }}</span>
                    </div>
                  </div>
                </div>

                <!-- Individual Exception indicator -->
                <div
                  v-if="archiveSelectedSessionData!.hasException"
                  class="px-4 py-2.5 border-t"
                  style="border-color: rgba(255,255,255,0.04); background: rgba(49, 130, 206, 0.02);"
                >
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <UIcon
                        name="i-lucide-shield-alert"
                        class="size-3"
                        style="color: #3182CE;"
                      />
                      <span
                        class="text-[9px] font-bold"
                        style="color: #3182CE;"
                      >Индивидуальные исключения</span>
                    </div>
                    <div class="flex items-center gap-2">
                      <span
                        class="text-[8px] font-bold px-2 py-0.5 rounded-full"
                        style="background: rgba(49, 130, 206, 0.12); color: #3182CE;"
                      >Manual Override</span>
                      <span class="text-[8px] text-white/30">{{ archiveSelectedSessionData!.exceptionCount }} переопред.</span>
                    </div>
                  </div>
                </div>
              </div>

              <!-- RIGHT: 2 cols — Event Log -->
              <div
                class="md:col-span-2 flex flex-col"
                style="background: rgba(0,0,0,0.15);"
              >
                <!-- Event log header -->
                <div
                  class="px-4 py-3 border-b"
                  style="border-color: rgba(255,255,255,0.04);"
                >
                  <div class="flex items-center justify-between mb-2">
                    <div class="flex items-center gap-2">
                      <UIcon
                        name="i-lucide-activity"
                        class="size-3.5"
                        style="color: #34A853;"
                      />
                      <span class="text-[10px] font-bold text-white/50 uppercase tracking-wider">Журнал</span>
                    </div>
                    <span
                      class="text-[8px] font-bold px-1.5 py-0.5 rounded-full"
                      style="background: rgba(52, 168, 83, 0.1); color: #34A853;"
                    >{{ archiveEvents.length }}</span>
                  </div>
                  <!-- Source summary -->
                  <div class="flex items-center gap-2">
                    <span
                      class="text-[7px] font-bold px-1.5 py-0.5 rounded-full inline-flex items-center gap-0.5"
                      style="background: rgba(66, 133, 244, 0.08); color: #4285F4;"
                    >
                      <UIcon
                        name="i-lucide-video"
                        class="size-2"
                      />
                      {{ archiveWebcamEvents }} веб
                    </span>
                    <span
                      class="text-[7px] font-bold px-1.5 py-0.5 rounded-full inline-flex items-center gap-0.5"
                      style="background: rgba(162, 89, 255, 0.08); color: #A259FF;"
                    >
                      <UIcon
                        name="i-lucide-camera"
                        class="size-2"
                      />
                      {{ archiveSideEvents }} бок.
                    </span>
                    <span
                      class="text-[7px] font-bold px-1.5 py-0.5 rounded-full inline-flex items-center gap-0.5"
                      style="background: rgba(52, 168, 83, 0.08); color: #34A853;"
                    >
                      <UIcon
                        name="i-lucide-monitor"
                        class="size-2"
                      />
                      {{ archiveSystemEvents }} сист.
                    </span>
                  </div>
                </div>

                <!-- Timeline events -->
                <div
                  class="flex-1 overflow-y-auto custom-scrollbar"
                  style="max-height: 380px;"
                >
                  <div
                    v-for="(evt, idx) in archiveEvents"
                    :key="idx"
                    class="relative pl-7 pr-3 py-2.5 border-b transition-all duration-500 hover:bg-white/[0.02] group"
                    :class="archiveVisible ? 'opacity-100 translate-x-0' : 'opacity-0 translate-x-4'"
                    :style="{
                      borderColor: 'rgba(255,255,255,0.03)',
                      transitionDelay: `${600 + idx * 80}ms`
                    }"
                  >
                    <!-- Timeline connector -->
                    <div
                      class="absolute left-3.5 top-0 bottom-0 w-px"
                      style="background: rgba(255,255,255,0.06);"
                    />
                    <!-- Timeline dot -->
                    <div
                      class="absolute left-2.5 top-3.5 size-2.5 rounded-full border-2 z-10"
                      :class="{ 'animate-pulse': evt.severity === 'critical' }"
                      :style="{
                        background: '#1A2130',
                        borderColor: evt.severity === 'critical' ? '#EA4335' : evt.severity === 'warning' ? '#FBBC05' : '#4285F4'
                      }"
                    />

                    <div class="flex-1 min-w-0">
                      <!-- Time + severity tag + source badge -->
                      <div class="flex items-center gap-1.5 mb-1 flex-wrap">
                        <span class="text-[8px] font-mono text-white/25">{{ evt.time }}</span>
                        <span
                          class="text-[6px] font-black px-1 py-0.5 rounded-sm uppercase"
                          :style="{
                            background: evt.severity === 'critical' ? 'rgba(234, 67, 53, 0.15)' : evt.severity === 'warning' ? 'rgba(251, 188, 5, 0.15)' : 'rgba(66, 133, 244, 0.08)',
                            color: evt.severity === 'critical' ? '#EA4335' : evt.severity === 'warning' ? '#FBBC05' : '#4285F4'
                          }"
                        >{{ evt.tag }}</span>
                        <span
                          class="text-[6px] font-bold px-1 py-0.5 rounded-sm inline-flex items-center gap-0.5"
                          :style="{
                            background: `${sourceColor(evt.source)}10`,
                            color: sourceColor(evt.source)
                          }"
                        >
                          <UIcon
                            :name="sourceIcon(evt.source)"
                            class="size-1.5"
                          />
                          {{ sourceLabel(evt.source) }}
                        </span>
                      </div>
                      <!-- Event label -->
                      <div class="flex items-center gap-1.5">
                        <UIcon
                          :name="evt.icon"
                          class="size-3 shrink-0"
                          :style="{ color: evt.severity === 'critical' ? '#EA4335' : evt.severity === 'warning' ? '#FBBC05' : 'rgba(255,255,255,0.3)' }"
                        />
                        <span class="text-[9px] text-white/50 leading-tight">{{ evt.label }}</span>
                      </div>
                      <!-- Seek-to button -->
                      <button
                        class="mt-1 text-[7px] font-bold px-1.5 py-0.5 rounded cursor-pointer transition-all opacity-0 group-hover:opacity-100 inline-flex items-center gap-0.5"
                        style="background: rgba(66, 133, 244, 0.08); color: #4285F4;"
                        @click.stop="archiveSeekToEvent(evt.time, evt.source)"
                      >
                        <UIcon
                          name="i-lucide-skip-forward"
                          class="size-2"
                        />
                        Перейти к
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- ===== KERNEL TAB ===== -->
          <div
            v-show="archiveDetailTab === 'kernel'"
            class="p-6"
          >
            <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
              <!-- Kernel-Level System Control -->
              <div
                class="rounded-xl border p-5"
                style="background: rgba(251, 188, 5, 0.02); border-color: rgba(251, 188, 5, 0.1);"
              >
                <div class="flex items-center gap-2 mb-4">
                  <div
                    class="size-8 rounded-lg flex items-center justify-center"
                    style="background: rgba(251, 188, 5, 0.1);"
                  >
                    <UIcon
                      name="i-lucide-cpu"
                      class="size-4"
                      style="color: #FBBC05;"
                    />
                  </div>
                  <div>
                    <h4 class="text-sm font-bold text-white">
                      Kernel-Level System Control
                    </h4>
                    <p class="text-[10px] text-white/30">
                      Низкоуровневый контроль системы
                    </p>
                  </div>
                </div>
                <div class="space-y-3">
                  <div
                    v-for="(val, key) in archiveKernelStatus"
                    :key="key"
                    class="flex items-center justify-between p-3 rounded-lg cursor-pointer transition-all"
                    :style="{
                      background: val ? 'rgba(251, 188, 5, 0.06)' : 'rgba(255,255,255,0.02)',
                      border: `1px solid ${val ? 'rgba(251, 188, 5, 0.12)' : 'rgba(255,255,255,0.04)'}`
                    }"
                    @click="(archiveKernelStatus as any)[key] = !(archiveKernelStatus as any)[key]"
                  >
                    <div class="flex items-center gap-2">
                      <UIcon
                        name="i-lucide-shield"
                        class="size-3.5"
                        :style="{ color: val ? '#FBBC05' : 'rgba(255,255,255,0.2)' }"
                      />
                      <span
                        class="text-[11px] font-semibold"
                        :style="{ color: val ? 'rgba(255,255,255,0.8)' : 'rgba(255,255,255,0.3)' }"
                      >{{ archiveKernelLabels[key] || key }}</span>
                    </div>
                    <div
                      class="w-8 h-4 rounded-full relative transition-all cursor-pointer"
                      :style="{ background: val ? '#FBBC05' : 'rgba(255,255,255,0.1)' }"
                    >
                      <div
                        class="absolute top-0.5 size-3 rounded-full transition-all"
                        :style="{ left: val ? '18px' : '2px', background: val ? '#1A2130' : 'rgba(255,255,255,0.3)' }"
                      />
                    </div>
                  </div>
                </div>
              </div>

              <!-- Individual Exceptions Detail -->
              <div
                class="rounded-xl border p-5"
                style="background: rgba(49, 130, 206, 0.02); border-color: rgba(49, 130, 206, 0.1);"
              >
                <div class="flex items-center gap-2 mb-4">
                  <div
                    class="size-8 rounded-lg flex items-center justify-center"
                    style="background: rgba(49, 130, 206, 0.1);"
                  >
                    <UIcon
                      name="i-lucide-shield-alert"
                      class="size-4"
                      style="color: #3182CE;"
                    />
                  </div>
                  <div>
                    <h4 class="text-sm font-bold text-white">
                      Индивидуальные исключения
                    </h4>
                    <p class="text-[10px] text-white/30">
                      Для выбранного студента
                    </p>
                  </div>
                </div>

                <div
                  v-if="archiveSelectedSessionData!.hasException"
                  class="space-y-3"
                >
                  <div
                    class="flex items-center justify-between p-3 rounded-lg"
                    style="background: rgba(49, 130, 206, 0.06); border: 1px solid rgba(49, 130, 206, 0.12);"
                  >
                    <div class="flex items-center gap-2">
                      <UIcon
                        name="i-lucide-user"
                        class="size-3.5"
                        style="color: #3182CE;"
                      />
                      <span class="text-[11px] font-semibold text-white/80">{{ archiveSelectedSessionData!.name }}</span>
                    </div>
                    <span
                      class="text-[9px] font-bold px-2 py-0.5 rounded-full"
                      style="background: rgba(49, 130, 206, 0.12); color: #3182CE;"
                    >Manual Override</span>
                  </div>
                  <div
                    class="p-3 rounded-lg"
                    style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.04);"
                  >
                    <div class="flex items-center justify-between mb-2">
                      <span class="text-[9px] text-white/40">Переопределённые параметры</span>
                      <span
                        class="text-[10px] font-bold"
                        style="color: #3182CE;"
                      >{{ archiveSelectedSessionData!.exceptionCount }}</span>
                    </div>
                    <div
                      class="w-full h-1.5 rounded-full overflow-hidden"
                      style="background: rgba(255,255,255,0.06);"
                    >
                      <div
                        class="h-full rounded-full"
                        :style="{ width: `${(archiveSelectedSessionData!.exceptionCount / 19) * 100}%`, background: '#3182CE' }"
                      />
                    </div>
                    <p class="text-[8px] text-white/20 mt-1">
                      {{ archiveSelectedSessionData!.exceptionCount }} из 19 параметров изменены
                    </p>
                  </div>
                </div>
                <div
                  v-else
                  class="flex flex-col items-center justify-center py-8 opacity-30"
                >
                  <UIcon
                    name="i-lucide-check-circle"
                    class="size-8 text-white mb-2"
                  />
                  <span class="text-[10px] text-white/60">Без исключений</span>
                  <span class="text-[8px] text-white/30">Используются глобальные настройки</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Playback Controls with event markers -->
          <div
            class="px-6 py-4 border-t"
            style="border-color: rgba(255,255,255,0.06);"
          >
            <div class="flex items-center gap-4">
              <!-- Play/Pause -->
              <button
                class="flex items-center justify-center size-10 rounded-full cursor-pointer transition-all hover:scale-110"
                style="background: linear-gradient(135deg, #4285F4, #A259FF);"
                @click="toggleArchivePlay"
              >
                <UIcon
                  :name="archivePlaying ? 'i-lucide-pause' : 'i-lucide-play'"
                  class="size-5 text-white"
                />
              </button>

              <!-- Progress bar with event markers -->
              <div class="flex-1 relative">
                <!-- Event markers on timeline -->
                <div class="absolute top-0 left-0 right-0 h-1.5 pointer-events-none z-10">
                  <div
                    v-for="(evt, idx) in archiveEvents"
                    :key="'marker-' + idx"
                    class="absolute top-0 w-0.5 h-full rounded-full"
                    :style="{
                      left: `${((parseInt(evt.time.split(':')[0]!) * 60 + parseInt(evt.time.split(':')[1]!)) / 675) * 100}%`,
                      background: evt.severity === 'critical' ? '#EA4335' : evt.severity === 'warning' ? '#FBBC05' : 'rgba(66, 133, 244, 0.4)',
                      opacity: evt.severity === 'critical' ? 0.8 : 0.4
                    }"
                  />
                </div>
                <input
                  v-model.number="archiveProgress"
                  type="range"
                  min="0"
                  max="100"
                  step="0.1"
                  class="w-full h-1.5 rounded-full appearance-none cursor-pointer relative z-20"
                  :style="{ background: `linear-gradient(to right, #4285F4 ${archiveProgress}%, rgba(255,255,255,0.1) ${archiveProgress}%)` }"
                >
                <div class="flex items-center justify-between mt-1">
                  <span class="text-[10px] font-mono text-white/30">{{ archiveCurrentTime }}</span>
                  <span class="text-[10px] font-mono text-white/30">{{ archiveTotalTime }}</span>
                </div>
              </div>

              <!-- Speed -->
              <div
                class="flex items-center gap-1 px-2 py-1 rounded"
                style="background: rgba(255,255,255,0.06);"
              >
                <span class="text-[10px] font-bold text-white/50">1.0x</span>
              </div>

              <!-- Fullscreen toggle -->
              <button
                class="flex items-center justify-center size-8 rounded-lg cursor-pointer transition-all hover:scale-105"
                style="background: rgba(255,255,255,0.06);"
              >
                <UIcon
                  name="i-lucide-maximize"
                  class="size-4 text-white/50"
                />
              </button>
            </div>
          </div>
        </div>

        <!-- Caption -->
        <p class="text-center text-[11px] text-white/30 mt-6 leading-relaxed max-w-xl mx-auto">
          <UIcon
            name="i-lucide-eye"
            class="size-3.5 inline-block mr-1"
            style="color: #34A853;"
          />
          Полная прозрачность каждой сессии: Просматривайте записи, анализируйте события, слушайте аудио — в любой момент.
        </p>
      </div>
    </section>

    <!-- ============================== -->
    <!--  SOCIAL PROOF / STATS          -->
    <!-- ============================== -->
    <section
      id="stats"
      ref="statsRef"
      class="relative py-32 px-6"
    >
      <div class="max-w-7xl mx-auto">
        <div class="text-center mb-16">
          <span
            class="text-xs font-bold uppercase tracking-widest"
            style="color: #4285F4;"
          >Результаты</span>
          <h2 class="text-4xl md:text-5xl font-bold text-white mt-4 tracking-tight">
            Цифры, которые говорят сами за себя
          </h2>
        </div>

        <div class="grid grid-cols-2 md:grid-cols-4 gap-6">
          <div
            class="text-center p-8 rounded-2xl border transition-all duration-700"
            :class="statsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'"
            style="background: #1A2130; border-color: rgba(255,255,255,0.06); transition-delay: 0ms;"
          >
            <p class="text-4xl md:text-5xl font-bold text-white tabular-nums">
              {{ Math.round(counterHours).toLocaleString() }}+
            </p>
            <p class="text-sm text-white/40 mt-2">
              проверенных часов
            </p>
          </div>

          <div
            class="text-center p-8 rounded-2xl border transition-all duration-700"
            :class="statsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'"
            style="background: #1A2130; border-color: rgba(255,255,255,0.06); transition-delay: 150ms;"
          >
            <p
              class="text-4xl md:text-5xl font-bold tabular-nums"
              style="color: #4285F4;"
            >
              {{ Math.round(counterCountries) }}+
            </p>
            <p class="text-sm text-white/40 mt-2">
              стран
            </p>
          </div>

          <div
            class="text-center p-8 rounded-2xl border transition-all duration-700"
            :class="statsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'"
            style="background: #1A2130; border-color: rgba(255,255,255,0.06); transition-delay: 300ms;"
          >
            <p class="text-4xl md:text-5xl font-bold text-white tabular-nums">
              {{ Math.round(counterReduction) }}%
            </p>
            <p class="text-sm text-white/40 mt-2">
              снижение нарушений
            </p>
          </div>

          <div
            class="text-center p-8 rounded-2xl border transition-all duration-700"
            :class="statsVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'"
            style="background: #1A2130; border-color: rgba(255,255,255,0.06); transition-delay: 450ms;"
          >
            <p
              class="text-4xl md:text-5xl font-bold tabular-nums"
              style="color: #34A853;"
            >
              {{ Math.round(counterSessions).toLocaleString() }}+
            </p>
            <p class="text-sm text-white/40 mt-2">
              одновременных сессий
            </p>
          </div>
        </div>

        <!-- Integration logos -->
        <div class="mt-20 text-center">
          <p class="text-xs text-white/30 uppercase tracking-widest mb-8">
            Интеграции
          </p>
          <div class="flex items-center justify-center gap-12 opacity-30">
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-graduation-cap"
                class="size-6 text-white"
              />
              <span class="text-sm font-semibold text-white">Moodle</span>
            </div>
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-palette"
                class="size-6 text-white"
              />
              <span class="text-sm font-semibold text-white">Canvas</span>
            </div>
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-shield-check"
                class="size-6 text-white"
              />
              <span class="text-sm font-semibold text-white">Eduser</span>
            </div>
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-book-open"
                class="size-6 text-white"
              />
              <span class="text-sm font-semibold text-white">Google Classroom</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ============================================ -->
    <!--  ARGUS AI VS STANDARD — COMPARISON MATRIX   -->
    <!-- ============================================ -->
    <section
      id="comparison"
      ref="comparisonRef"
      class="relative py-32 px-6"
    >
      <div class="max-w-6xl mx-auto">
        <div class="text-center mb-16">
          <span
            class="text-xs font-bold uppercase tracking-widest"
            style="color: #EA4335;"
          >Опережая конкурентов</span>
          <h2 class="text-3xl md:text-4xl lg:text-[2.75rem] font-bold text-white mt-4 tracking-tight leading-tight">
            Argus AI vs <span class="comparison-gradient-text">Стандартный прокторинг</span>
          </h2>
          <p class="text-lg text-white/40 mt-4 max-w-2xl mx-auto">
            Не все решения равны. Посмотрите, что отличает Argus AI от стандартных систем.
          </p>
        </div>

        <!-- Comparison Table -->
        <div
          class="relative rounded-2xl border overflow-hidden transition-all duration-1000"
          :class="comparisonVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-16'"
          style="background: #1A2130; border-color: rgba(234, 67, 53, 0.12);"
        >
          <!-- Table header -->
          <div
            class="grid grid-cols-12 gap-0 border-b"
            style="border-color: rgba(255,255,255,0.06);"
          >
            <div class="col-span-4 px-6 py-4 flex items-center">
              <span class="text-xs font-bold text-white/40 uppercase tracking-wider">Параметр</span>
            </div>
            <div
              class="col-span-4 px-6 py-4 text-center"
              style="background: rgba(255,255,255,0.02);"
            >
              <div class="flex items-center justify-center gap-2">
                <UIcon
                  name="i-lucide-monitor"
                  class="size-4 text-white/30"
                />
                <span class="text-xs font-bold text-white/40 uppercase tracking-wider">Стандартный</span>
              </div>
            </div>
            <div
              class="col-span-4 px-6 py-4 text-center"
              style="background: rgba(66, 133, 244, 0.04); border-left: 2px solid rgba(66, 133, 244, 0.2);"
            >
              <div class="flex items-center justify-center gap-2">
                <ArgusLogo :size="18" />
                <span
                  class="text-xs font-bold uppercase tracking-wider"
                  style="color: #4285F4;"
                >Argus AI</span>
                <span
                  class="text-[7px] font-black px-1.5 py-0.5 rounded-full"
                  style="background: rgba(52, 168, 83, 0.15); color: #34A853;"
                >WINNER</span>
              </div>
            </div>
          </div>

          <!-- Comparison rows -->
          <div
            v-for="(row, idx) in comparisonData"
            :key="row.category"
            class="grid grid-cols-12 gap-0 border-b transition-all duration-700"
            :class="comparisonVisible ? 'opacity-100 translate-x-0' : 'opacity-0 -translate-x-8'"
            :style="{
              borderColor: 'rgba(255,255,255,0.04)',
              transitionDelay: `${300 + idx * 100}ms`,
              background: row.highlight ? `${row.highlight}04` : 'transparent'
            }"
          >
            <!-- Category -->
            <div class="col-span-4 px-6 py-4 flex items-center gap-3">
              <div
                class="flex items-center justify-center size-7 rounded-lg shrink-0"
                :style="{ background: row.highlight ? `${row.highlight}12` : 'rgba(255,255,255,0.04)' }"
              >
                <UIcon
                  :name="row.icon"
                  class="size-3.5"
                  :style="{ color: row.highlight || 'rgba(255,255,255,0.4)' }"
                />
              </div>
              <div class="min-w-0">
                <span class="text-sm font-semibold text-white/70">{{ row.category }}</span>
                <span
                  v-if="row.highlight"
                  class="ml-2 text-[7px] font-black px-1.5 py-0.5 rounded-full uppercase"
                  :style="{ background: `${row.highlight}15`, color: row.highlight }"
                >NEW</span>
              </div>
            </div>

            <!-- Standard -->
            <div
              class="col-span-4 px-6 py-4 flex items-center justify-center"
              style="background: rgba(255,255,255,0.02);"
            >
              <div class="flex items-center gap-2.5">
                <div
                  class="flex items-center justify-center size-5 rounded-full shrink-0"
                  :style="{ background: row.standard.level > 0 ? 'rgba(255,255,255,0.06)' : 'rgba(234, 67, 53, 0.1)' }"
                >
                  <UIcon
                    :name="row.standard.level > 0 ? 'i-lucide-minus' : 'i-lucide-x'"
                    class="size-3"
                    :style="{ color: row.standard.level > 0 ? 'rgba(255,255,255,0.25)' : '#EA4335' }"
                  />
                </div>
                <span class="text-sm text-white/40 leading-tight">{{ row.standard.label }}</span>
              </div>
            </div>

            <!-- Argus AI -->
            <div
              class="col-span-4 px-6 py-4 flex items-center justify-center"
              :style="{
                background: row.highlight ? `${row.highlight}06` : 'rgba(66, 133, 244, 0.04)',
                borderLeft: `2px solid ${row.highlight ? `${row.highlight}30` : 'rgba(66, 133, 244, 0.1)'}`
              }"
            >
              <div class="flex items-center gap-2.5">
                <div
                  class="flex items-center justify-center size-5 rounded-full shrink-0"
                  :style="{ background: 'rgba(52, 168, 83, 0.15)' }"
                >
                  <UIcon
                    name="i-lucide-check"
                    class="size-3"
                    style="color: #34A853;"
                  />
                </div>
                <span class="text-sm font-medium text-white/70 leading-tight">{{ row.argus.label }}</span>
              </div>
            </div>
          </div>

          <!-- Bottom highlight — Low Latency callout -->
          <div
            class="px-6 py-5 flex items-center justify-between"
            style="background: linear-gradient(135deg, rgba(66, 133, 244, 0.06), rgba(52, 168, 83, 0.06));"
          >
            <div class="flex items-center gap-4">
              <div
                class="flex items-center justify-center size-12 rounded-xl"
                style="background: rgba(52, 168, 83, 0.1); border: 1px solid rgba(52, 168, 83, 0.2);"
              >
                <UIcon
                  name="i-lucide-zap"
                  class="size-6"
                  style="color: #34A853;"
                />
              </div>
              <div>
                <p class="text-sm font-bold text-white">
                  Ultra Low-Latency: <span style="color: #34A853;">52 kb/s</span>
                </p>
                <p class="text-[11px] text-white/40 mt-0.5">
                  Стабильная работа даже на мобильном интернете 3G. В 10 раз экономичнее стандартных решений.
                </p>
              </div>
            </div>
            <div class="hidden md:flex items-center gap-3">
              <div
                class="flex flex-col items-center px-4 py-2 rounded-lg"
                style="background: rgba(255,255,255,0.04);"
              >
                <span class="text-[9px] text-white/30 uppercase">Стандарт</span>
                <span class="text-sm font-bold text-white/40 line-through">500+ kb/s</span>
              </div>
              <UIcon
                name="i-lucide-arrow-right"
                class="size-4 text-white/20"
              />
              <div
                class="flex flex-col items-center px-4 py-2 rounded-lg"
                style="background: rgba(52, 168, 83, 0.08); border: 1px solid rgba(52, 168, 83, 0.2);"
              >
                <span
                  class="text-[9px] uppercase"
                  style="color: #34A853;"
                >Argus AI</span>
                <span
                  class="text-sm font-bold"
                  style="color: #34A853;"
                >52 kb/s</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Exclusive features highlight cards -->
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6">
          <!-- Kernel-Level System Control -->
          <div
            class="rounded-2xl border p-5 transition-all duration-1000"
            :class="comparisonVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'"
            style="background: rgba(251, 188, 5, 0.03); border-color: rgba(251, 188, 5, 0.12); transition-delay: 1100ms;"
          >
            <div class="flex items-center gap-3 mb-3">
              <div
                class="flex items-center justify-center size-10 rounded-xl shrink-0"
                style="background: rgba(251, 188, 5, 0.08); border: 1px solid rgba(251, 188, 5, 0.15);"
              >
                <UIcon
                  name="i-lucide-cpu"
                  class="size-5"
                  style="color: #FBBC05;"
                />
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-bold text-white">
                    Kernel-Level System Control
                  </p>
                  <span
                    class="text-[7px] font-black px-1.5 py-0.5 rounded-full"
                    style="background: rgba(251, 188, 5, 0.15); color: #FBBC05;"
                  >ЭКСКЛЮЗИВ</span>
                </div>
              </div>
            </div>
            <p class="text-[11px] text-white/40 leading-relaxed">
              Глубокий контроль на уровне ядра ОС: <span class="text-white/60 font-medium">сканирование процессов</span>,
              обнаружение аппаратных устройств, блокировка удалённого доступа, привязка к оборудованию и детекция мульти-мониторов.
            </p>
            <div class="flex items-center gap-2 mt-3 flex-wrap">
              <span
                v-for="tag in ['Процессы ОС', 'Устройства', 'Remote Block', 'Hardware ID', 'Мониторы']"
                :key="tag"
                class="text-[8px] font-bold px-2 py-1 rounded-md"
                style="background: rgba(251, 188, 5, 0.06); color: rgba(251, 188, 5, 0.7); border: 1px solid rgba(251, 188, 5, 0.1);"
              >{{ tag }}</span>
            </div>
          </div>

          <!-- Individual Exceptions -->
          <div
            class="rounded-2xl border p-5 transition-all duration-1000"
            :class="comparisonVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-8'"
            style="background: rgba(162, 89, 255, 0.03); border-color: rgba(162, 89, 255, 0.12); transition-delay: 1200ms;"
          >
            <div class="flex items-center gap-3 mb-3">
              <div
                class="flex items-center justify-center size-10 rounded-xl shrink-0"
                style="background: rgba(162, 89, 255, 0.08); border: 1px solid rgba(162, 89, 255, 0.15);"
              >
                <UIcon
                  name="i-lucide-user-cog"
                  class="size-5"
                  style="color: #A259FF;"
                />
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <p class="text-sm font-bold text-white">
                    Индивидуальные исключения
                  </p>
                  <span
                    class="text-[7px] font-black px-1.5 py-0.5 rounded-full"
                    style="background: rgba(162, 89, 255, 0.15); color: #A259FF;"
                  >ЭКСКЛЮЗИВ</span>
                </div>
              </div>
            </div>
            <p class="text-[11px] text-white/40 leading-relaxed">
              Создавайте <span class="text-white/60 font-medium">персональные настройки</span> для каждого студента —
              24 параметра AI-детекции, видео и блокировки могут быть переопределены для студентов с особыми потребностями.
            </p>
            <div class="flex items-center gap-2 mt-3 flex-wrap">
              <span
                v-for="tag in ['Manual Override', 'Per-Student', '24 параметра', 'Reset to Global']"
                :key="tag"
                class="text-[8px] font-bold px-2 py-1 rounded-md"
                style="background: rgba(162, 89, 255, 0.06); color: rgba(162, 89, 255, 0.7); border: 1px solid rgba(162, 89, 255, 0.1);"
              >{{ tag }}</span>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ============================== -->
    <!--  PRICING SECTION               -->
    <!-- ============================== -->
    <section
      id="pricing"
      ref="pricingRef"
      class="relative py-32 px-6"
      style="background: #0F151D;"
    >
      <div class="max-w-5xl mx-auto">
        <div class="text-center mb-16">
          <span
            class="text-xs font-bold uppercase tracking-widest"
            style="color: #FBBC05;"
          >Стоимость</span>
          <h2 class="text-4xl md:text-5xl font-bold text-white mt-4 tracking-tight">
            Масштабируемая честность.<br>
            <span class="pricing-gradient-text">Прозрачные цены.</span>
          </h2>
          <p class="text-lg text-white/40 mt-4 max-w-2xl mx-auto">
            Платите только за реальных студентов. Чем больше — тем дешевле.
          </p>
        </div>

        <!-- Price Calculator Card -->
        <div
          class="relative rounded-2xl border overflow-hidden transition-all duration-1000"
          :class="pricingVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-16'"
          style="background: #1A2130; border-color: rgba(251, 188, 5, 0.15);"
        >
          <!-- Calculator header -->
          <div
            class="flex items-center justify-between px-6 py-4 border-b"
            style="border-color: rgba(255,255,255,0.06);"
          >
            <div class="flex items-center gap-3">
              <div
                class="flex items-center justify-center size-8 rounded-lg"
                style="background: rgba(251, 188, 5, 0.1);"
              >
                <UIcon
                  name="i-lucide-calculator"
                  class="size-4"
                  style="color: #FBBC05;"
                />
              </div>
              <span class="text-sm font-bold text-white">Калькулятор стоимости</span>
            </div>
            <span
              class="text-[9px] font-bold px-2 py-0.5 rounded-full"
              :style="{ background: hexToRgba(pricingTierColor, 0.15), color: pricingTierColor }"
            >
              {{ pricingTierLabel }}
            </span>
          </div>

          <div class="p-6">
            <!-- Student count slider -->
            <div class="mb-8">
              <div class="flex items-center justify-between mb-3">
                <p class="text-sm font-medium text-white/80">
                  Количество студентов
                </p>
                <span class="text-2xl font-bold tabular-nums text-white">{{ studentCount.toLocaleString() }}</span>
              </div>
              <input
                v-model.number="studentCount"
                type="range"
                min="100"
                max="10000"
                step="100"
                class="w-full h-2 rounded-full appearance-none cursor-pointer"
                :style="{ background: `linear-gradient(to right, #FBBC05 ${(studentCount - 100) / 9900 * 100}%, rgba(255,255,255,0.1) ${(studentCount - 100) / 9900 * 100}%)` }"
              >
              <div class="flex items-center justify-between mt-2">
                <span class="text-[10px] text-white/30">100</span>
                <span class="text-[10px] text-white/30">2,500</span>
                <span class="text-[10px] text-white/30">5,000</span>
                <span class="text-[10px] text-white/30">7,500</span>
                <span class="text-[10px] text-white/30">10,000+</span>
              </div>
            </div>

            <!-- Price display -->
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-8">
              <div
                class="text-center p-5 rounded-xl"
                style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06);"
              >
                <p class="text-[10px] text-white/40 uppercase tracking-wider mb-1">
                  Цена за студента
                </p>
                <p class="text-3xl font-bold text-white">
                  $<span class="tabular-nums">{{ pricePerStudent.toFixed(2) }}</span>
                </p>
                <p class="text-[10px] text-white/30 mt-1">
                  за экзаменационный цикл
                </p>
              </div>
              <div
                class="text-center p-5 rounded-xl"
                style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06);"
              >
                <p class="text-[10px] text-white/40 uppercase tracking-wider mb-1">
                  Итого
                </p>
                <p
                  class="text-3xl font-bold tabular-nums"
                  :style="{ color: pricingTierColor }"
                >
                  ${{ totalCost }}
                </p>
                <p class="text-[10px] text-white/30 mt-1">
                  за экзаменационный цикл
                </p>
              </div>
              <div
                class="text-center p-5 rounded-xl relative overflow-hidden"
                :style="{ background: studentCount > 5000 ? hexToRgba('#34A853', 0.06) : 'rgba(255,255,255,0.03)', border: studentCount > 5000 ? '1px solid rgba(52, 168, 83, 0.2)' : '1px solid rgba(255,255,255,0.06)' }"
              >
                <div
                  v-if="studentCount > 5000"
                  class="absolute top-0 right-0 px-2 py-0.5 text-[7px] font-bold rounded-bl"
                  style="background: #34A853; color: white;"
                >
                  BEST VALUE
                </div>
                <p class="text-[10px] text-white/40 uppercase tracking-wider mb-1">
                  Экономия
                </p>
                <p
                  class="text-3xl font-bold tabular-nums"
                  style="color: #34A853;"
                >
                  {{ Math.round((1 - pricePerStudent / 2.50) * 100) }}%
                </p>
                <p class="text-[10px] text-white/30 mt-1">
                  от базовой цены
                </p>
              </div>
            </div>

            <!-- Tier indicators -->
            <div class="grid grid-cols-4 gap-2 mb-8">
              <div
                class="px-3 py-2 rounded-lg text-center"
                :style="{ background: studentCount <= 500 ? 'rgba(251,188,5,0.08)' : 'rgba(255,255,255,0.02)', border: studentCount <= 500 ? '1px solid rgba(251,188,5,0.2)' : '1px solid rgba(255,255,255,0.04)' }"
              >
                <p
                  class="text-[9px] font-bold"
                  :style="{ color: studentCount <= 500 ? '#FBBC05' : 'rgba(255,255,255,0.3)' }"
                >
                  100–500
                </p>
                <p
                  class="text-xs font-bold mt-0.5"
                  :style="{ color: studentCount <= 500 ? 'white' : 'rgba(255,255,255,0.2)' }"
                >
                  $2.50
                </p>
              </div>
              <div
                class="px-3 py-2 rounded-lg text-center"
                :style="{ background: studentCount > 500 && studentCount <= 2000 ? 'rgba(162,89,255,0.08)' : 'rgba(255,255,255,0.02)', border: studentCount > 500 && studentCount <= 2000 ? '1px solid rgba(162,89,255,0.2)' : '1px solid rgba(255,255,255,0.04)' }"
              >
                <p
                  class="text-[9px] font-bold"
                  :style="{ color: studentCount > 500 && studentCount <= 2000 ? '#A259FF' : 'rgba(255,255,255,0.3)' }"
                >
                  501–2,000
                </p>
                <p
                  class="text-xs font-bold mt-0.5"
                  :style="{ color: studentCount > 500 && studentCount <= 2000 ? 'white' : 'rgba(255,255,255,0.2)' }"
                >
                  $0.60
                </p>
              </div>
              <div
                class="px-3 py-2 rounded-lg text-center"
                :style="{ background: studentCount > 2000 && studentCount <= 5000 ? 'rgba(66,133,244,0.08)' : 'rgba(255,255,255,0.02)', border: studentCount > 2000 && studentCount <= 5000 ? '1px solid rgba(66,133,244,0.2)' : '1px solid rgba(255,255,255,0.04)' }"
              >
                <p
                  class="text-[9px] font-bold"
                  :style="{ color: studentCount > 2000 && studentCount <= 5000 ? '#4285F4' : 'rgba(255,255,255,0.3)' }"
                >
                  2,001–5,000
                </p>
                <p
                  class="text-xs font-bold mt-0.5"
                  :style="{ color: studentCount > 2000 && studentCount <= 5000 ? 'white' : 'rgba(255,255,255,0.2)' }"
                >
                  $1.20
                </p>
              </div>
              <div
                class="px-3 py-2 rounded-lg text-center"
                :style="{ background: studentCount > 5000 ? 'rgba(52,168,83,0.08)' : 'rgba(255,255,255,0.02)', border: studentCount > 5000 ? '1px solid rgba(52,168,83,0.2)' : '1px solid rgba(255,255,255,0.04)' }"
              >
                <p
                  class="text-[9px] font-bold"
                  :style="{ color: studentCount > 5000 ? '#34A853' : 'rgba(255,255,255,0.3)' }"
                >
                  5,000+
                </p>
                <p
                  class="text-xs font-bold mt-0.5"
                  :style="{ color: studentCount > 5000 ? 'white' : 'rgba(255,255,255,0.2)' }"
                >
                  $1.00
                </p>
              </div>
            </div>

            <!-- Feature grid -->
            <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
              <div
                v-for="feat in pricingFeatures"
                :key="feat.category"
                class="p-4 rounded-xl"
                style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);"
              >
                <div class="flex items-center gap-2 mb-3">
                  <UIcon
                    :name="feat.icon"
                    class="size-4"
                    :style="{ color: feat.color }"
                  />
                  <span
                    class="text-[10px] font-bold uppercase tracking-wider"
                    :style="{ color: feat.color }"
                  >{{ feat.category }}</span>
                </div>
                <ul class="space-y-1.5">
                  <li
                    v-for="item in feat.items"
                    :key="item"
                    class="flex items-start gap-1.5"
                  >
                    <UIcon
                      name="i-lucide-check"
                      class="size-3 mt-0.5 shrink-0"
                      style="color: #34A853;"
                    />
                    <span class="text-[10px] text-white/60 leading-tight">{{ item }}</span>
                  </li>
                </ul>
              </div>
            </div>

          </div>

          <!-- API callout -->
          <div
            class="px-6 py-4 border-t"
            style="border-color: rgba(255,255,255,0.06); background: rgba(66, 133, 244, 0.03);"
          >
            <div class="flex items-center gap-3">
              <div
                class="flex items-center justify-center size-8 rounded-lg shrink-0"
                style="background: rgba(66, 133, 244, 0.1);"
              >
                <UIcon
                  name="i-lucide-code-2"
                  class="size-4"
                  style="color: #4285F4;"
                />
              </div>
              <div>
                <p class="text-xs font-semibold text-white/80">
                  Developer API
                </p>
                <p class="text-[10px] text-white/40">
                  Интегрируйте Argus AI в свою платформу по API. Гибкая тарификация: платите только за реальные сессии.
                </p>
              </div>
              <button
                class="scanner-btn scanner-btn-inline shrink-0 px-3 py-1.5 rounded-lg text-[10px] font-bold"
                style="color: #4285F4;"
              >
                <span class="flex items-center gap-1.5">
                  <UIcon
                    name="i-lucide-plug-zap"
                    class="size-3"
                  />
                  Интегрировать по API
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- ============================== -->
    <!--  FINAL CTA                     -->
    <!-- ============================== -->
    <section
      ref="ctaRef"
      class="relative py-32 px-6"
      style="background: #121820;"
    >
      <div
        class="max-w-4xl mx-auto text-center transition-all duration-1000"
        :class="ctaVisible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-12'"
      >
        <h2 class="text-4xl md:text-6xl font-bold text-white tracking-tight">
          Готовы к
          <span class="cta-gradient-text">
            трансформации?
          </span>
        </h2>
        <p class="text-lg text-white/40 mt-6 max-w-2xl mx-auto">
          Присоединяйтесь к сотням учебных заведений, которые уже используют Argus AI для обеспечения честности экзаменов.
        </p>

        <div class="flex items-center justify-center gap-4 mt-12">
          <a
            href="https://wa.me/77073057755"
            target="_blank"
            class="scanner-btn scanner-btn-hero px-10 py-5 rounded-xl text-lg font-bold text-white inline-flex items-center gap-3"
          >
            <span>Связаться с отделом продаж</span>
          </a>
        </div>
      </div>
    </section>

    <!-- ============================== -->
    <!--  FOOTER                        -->
    <!-- ============================== -->
    <footer
      class="border-t px-6 py-12"
      style="border-color: rgba(255,255,255,0.06); background: #0A0E14;"
    >
      <div class="max-w-7xl mx-auto flex flex-col md:flex-row items-center justify-between gap-6">
        <div class="flex items-center gap-3">
          <ArgusLogo :size="28" />
          <span class="text-sm font-bold text-white/70">Argus AI</span>
          <span class="text-xs text-white/30">v2.4.1</span>
        </div>

        <div class="flex items-center gap-6">
          <a class="text-xs text-white/30 hover:text-white/60 transition-colors cursor-pointer">Политика конфиденциальности</a>
          <a class="text-xs text-white/30 hover:text-white/60 transition-colors cursor-pointer">Условия использования</a>
          <a class="text-xs text-white/30 hover:text-white/60 transition-colors cursor-pointer">Статус системы</a>
          <NuxtLink
            v-if="authStore.isLoggedIn"
            to="/"
            class="text-xs text-white/30 hover:text-white/60 transition-colors"
          >Дашборд</NuxtLink>
        </div>

        <p class="text-xs text-white/20">
          &copy; 2026 Argus AI. Все права защищены.
        </p>
      </div>
    </footer>

    <!-- Login Modal -->
    <LoginModal
      v-model="showLoginModal"
      @login-success="handleLoginSuccess"
    />
  </div>
</template>

<style scoped>
/* =============================================
   HERO HEADLINE — SCANLINE / GLITCH EFFECT
   ============================================= */
.hero-headline {
  position: relative;
  overflow: hidden;
}

.hero-headline::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(
    90deg,
    transparent 0%,
    rgba(66, 133, 244, 0.4) 15%,
    rgba(52, 168, 83, 0.5) 30%,
    rgba(251, 188, 5, 0.4) 50%,
    rgba(234, 67, 53, 0.5) 70%,
    rgba(162, 89, 255, 0.4) 85%,
    transparent 100%
  );
  animation: scanline 4s linear infinite;
  pointer-events: none;
  filter: blur(1px);
  opacity: 0.7;
  z-index: 2;
}

@keyframes scanline {
  0% {
    top: -4px;
    opacity: 0;
  }
  5% {
    opacity: 0.7;
  }
  50% {
    opacity: 0.9;
  }
  95% {
    opacity: 0.7;
  }
  100% {
    top: 100%;
    opacity: 0;
  }
}

/* Gradient text for the second hero line */
.hero-gradient-text {
  background: linear-gradient(135deg, #4285F4, #34A853, #FBBC05, #EA4335, #A259FF);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  text-shadow: none;
  position: relative;
}

.hero-gradient-text::before {
  content: 'БЕСКОМПРОМИССНЫЙ ПРОКТОРИНГ';
  position: absolute;
  left: 0;
  top: 0;
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #4285F4, #34A853, #FBBC05, #EA4335, #A259FF);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  filter: blur(20px);
  opacity: 0.4;
  z-index: -1;
  pointer-events: none;
}

/* Pricing section gradient text */
.pricing-gradient-text {
  background: linear-gradient(135deg, #FBBC05, #EA4335, #A259FF);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

/* Comparison section gradient text */
.comparison-gradient-text {
  background: linear-gradient(135deg, #EA4335, #FBBC05, #4285F4);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

/* CTA section gradient text */
.cta-gradient-text {
  background: linear-gradient(135deg, #4285F4, #34A853, #A259FF);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

/* =============================================
   SCANNER CTA BUTTON SYSTEM
   Corporate blue + glassmorphism + laser sweep
   ============================================= */

/* --- BASE: All scanner buttons --- */
/* Feature card hover glow */
.feature-card {
  transition: border-color 0.3s ease, box-shadow 0.3s ease, opacity 0.7s ease, transform 0.7s ease;
}

.feature-card:hover {
  box-shadow: 0 0 30px rgba(66, 133, 244, 0.06);
}

/* Floating particles */
.particle {
  animation: particle-float linear infinite;
  pointer-events: none;
}

@keyframes particle-float {
  0% {
    transform: translateY(0) translateX(0);
    opacity: 0;
  }
  10% {
    opacity: 0.3;
  }
  50% {
    opacity: 0.15;
    transform: translateY(-30px) translateX(15px);
  }
  90% {
    opacity: 0.3;
  }
  100% {
    transform: translateY(-60px) translateX(-10px);
    opacity: 0;
  }
}

/* Scanning circle animation for video placeholders */
.scanning-circle {
  animation: scan-pulse 3s ease-in-out infinite;
}

.scanning-circle-delayed {
  animation: scan-pulse 3s ease-in-out infinite 1.5s;
}

@keyframes scan-pulse {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(66, 133, 244, 0.2);
    transform: scale(1);
  }
  50% {
    box-shadow: 0 0 20px 5px rgba(66, 133, 244, 0.1);
    transform: scale(1.05);
  }
}

/* Custom scrollbar for settings panel */
.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 4px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.2);
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
</style>
