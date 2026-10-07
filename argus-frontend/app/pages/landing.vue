<script setup lang="ts">
import { useAuthStore } from '~/stores/useAuthStore'

const authStore = useAuthStore()
const colorMode = useColorMode()
const showLoginModal = ref(false)

// If the user is already authenticated, redirect to dashboard immediately
if (import.meta.client) {
  authStore.restoreSession()
  if (authStore.isLoggedIn) {
    navigateTo('/dashboard', { replace: true })
  }
}

// Force dark mode for the landing page
onMounted(() => {
  colorMode.preference = 'dark'

  // Double-check after mount (restoreSession may have hydrated by now)
  if (authStore.isLoggedIn) {
    navigateTo('/dashboard', { replace: true })
  }
})

function handleLoginSuccess() {
  showLoginModal.value = false
  navigateTo('/dashboard')
}

function openLogin() {
  showLoginModal.value = true
}

// ---------------------------------------------------------------------------
// Pricing data
// ---------------------------------------------------------------------------
const pricingTiers = [
  {
    name: 'Стартовый',
    nameEn: 'Starter',
    price: '$2.50',
    unit: '/ студент / мес',
    audience: 'До 500 студентов',
    badge: null,
    accent: '#FBBC05',
    features: [
      'AI Face ID верификация',
      'Gaze Tracking (трекинг взгляда)',
      'Browser Lockdown',
      'Детекция телефонов и объектов',
      'Basic REST API',
      'Email-поддержка'
    ]
  },
  {
    name: 'Университет',
    nameEn: 'University',
    price: '$1.20',
    unit: '/ студент / мес',
    audience: '2,000 – 5,000 студентов',
    badge: 'Популярный',
    accent: '#4285F4',
    features: [
      'Всё из Стартового, плюс:',
      'Dual-Cam (веб + боковая камера)',
      'Voice Detection + Speaker Count',
      'Full REST API + Webhooks',
      'Offline-First (OPFS хранилище)',
      'Приоритетная поддержка 24/7'
    ]
  },
  {
    name: 'Enterprise',
    nameEn: 'Enterprise',
    price: 'Индивидуально',
    unit: '',
    audience: '5,000+ студентов',
    badge: 'Максимум',
    accent: '#34A853',
    features: [
      'Всё из Университета, плюс:',
      'SSO / SAML интеграция',
      'SLA 99.95% uptime',
      'Dedicated account manager',
      'Custom retention policies',
      'On-premise deployment опция'
    ]
  }
]

// ---------------------------------------------------------------------------
// Capabilities data
// ---------------------------------------------------------------------------
const capabilities = [
  {
    icon: 'i-lucide-wifi-off',
    color: '#4285F4',
    title: 'Offline-First Прокторинг',
    description: 'Данные экзамена никогда не теряются. IndexedDB + OPFS гибридное хранилище с navigator.storage.persist(). Автоматическая синхронизация после восстановления связи.'
  },
  {
    icon: 'i-lucide-tablet-smartphone',
    color: '#34A853',
    title: 'Safari & iOS Оптимизация',
    description: 'Origin Private File System (OPFS) для устройств Apple. Полная поддержка Safari, Chrome, Edge. Оптимизировано для мобильного прокторинга.'
  },
  {
    icon: 'i-lucide-scan-eye',
    color: '#A259FF',
    title: 'AI Integrity Engine',
    description: 'Face tracking, gaze detection, voice analysis в реальном времени. 40+ типов детекции с задержкой <30мс. MediaPipe on-device inference без отправки видео.'
  },
  {
    icon: 'i-lucide-shield-check',
    color: '#EA4335',
    title: 'Forensic Evidence Chain',
    description: 'SHA-256 хеш-цепочка доказательств. ClickHouse аналитика с дедупликацией. Kafka-backed replay гарантирует 100% аудит-покрытие.'
  }
]

// ---------------------------------------------------------------------------
// Roles data
// ---------------------------------------------------------------------------
const roles = [
  {
    icon: 'i-lucide-graduation-cap',
    color: '#4285F4',
    name: 'Студент',
    description: 'Проходит экзамен, видит свои результаты и может подать апелляцию через личный кабинет.'
  },
  {
    icon: 'i-lucide-monitor',
    color: '#A259FF',
    name: 'Администратор',
    description: 'Управляет экзаменами, мониторит сессии в реальном времени, принимает решения по нарушениям.'
  },
  {
    icon: 'i-lucide-crown',
    color: '#FBBC05',
    name: 'Супер-Админ',
    description: 'Полный доступ ко всем организациям, API-управление, инфраструктурный мониторинг, аналитика.'
  }
]
</script>

<template>
  <div
    class="landing-root min-h-screen relative overflow-hidden"
    style="background: #0a0e17;"
  >
    <!-- Ambient Background -->
    <div class="absolute inset-0 pointer-events-none overflow-hidden">
      <div
        class="absolute top-[-20%] left-[-10%] w-[60%] h-[60%] rounded-full opacity-[0.04]"
        style="background: radial-gradient(circle, #4285F4, transparent 70%);"
      />
      <div
        class="absolute bottom-[-15%] right-[-5%] w-[50%] h-[50%] rounded-full opacity-[0.04]"
        style="background: radial-gradient(circle, #A259FF, transparent 70%);"
      />
      <div
        class="absolute top-[40%] left-[50%] w-[40%] h-[40%] rounded-full opacity-[0.02]"
        style="background: radial-gradient(circle, #34A853, transparent 70%);"
      />
      <!-- Grid pattern -->
      <div
        class="absolute inset-0 opacity-[0.03]"
        style="background-image: linear-gradient(rgba(255,255,255,0.08) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.08) 1px, transparent 1px); background-size: 64px 64px;"
      />
    </div>

    <!-- ===== NAVIGATION BAR ===== -->
    <nav class="relative z-20 flex items-center justify-between px-6 sm:px-8 py-5 max-w-7xl mx-auto">
      <div class="flex items-center gap-3">
        <ArgusLogo :size="32" />
        <span class="text-lg font-bold text-white tracking-tight">Argus AI</span>
      </div>
      <div class="flex items-center gap-3">
        <button
          class="text-sm text-white/50 hover:text-white/80 transition-colors px-4 py-2 hidden sm:block"
          @click="openLogin"
        >
          Войти
        </button>
        <button
          class="scanner-btn px-5 py-2.5 rounded-xl text-sm font-semibold text-white transition-all"
          @click="openLogin"
        >
          Начать работу
        </button>
      </div>
    </nav>

    <!-- ===== HERO SECTION ===== -->
    <section class="relative z-10 flex flex-col items-center text-center px-6 pt-12 sm:pt-16 pb-20 sm:pb-24 max-w-5xl mx-auto">
      <!-- Badge -->
      <div
        class="inline-flex items-center gap-2 px-4 py-1.5 rounded-full text-xs font-medium mb-8"
        style="background: rgba(66, 133, 244, 0.1); border: 1px solid rgba(66, 133, 244, 0.2); color: #93b4f4;"
      >
        <span class="relative flex size-1.5">
          <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-blue-400 opacity-75" />
          <span class="relative inline-flex size-1.5 rounded-full bg-blue-400" />
        </span>
        AI-Powered Proctoring Platform
      </div>

      <!-- Headline -->
      <h1 class="text-4xl sm:text-5xl lg:text-7xl font-extrabold text-white leading-[1.1] tracking-tight">
        <span class="block">Integrity at Scale.</span>
        <span
          class="block mt-2"
          style="background: linear-gradient(135deg, #4285F4, #A259FF); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text;"
        >
          AI-Driven Proctoring.
        </span>
      </h1>

      <p class="mt-6 text-base sm:text-lg lg:text-xl text-white/40 max-w-2xl leading-relaxed">
        Real-time fraud detection, forensic evidence chain, and enterprise-grade exam monitoring for 10,000+ concurrent sessions.
      </p>

      <!-- CTA Buttons -->
      <div class="flex flex-col sm:flex-row items-center gap-4 mt-10">
        <button
          class="scanner-btn w-full sm:w-auto px-8 py-4 rounded-xl text-base font-bold text-white transition-all"
          @click="openLogin"
        >
          Войти в панель управления
        </button>
        <NuxtLink
          to="/student-guide"
          class="w-full sm:w-auto text-center px-6 py-4 rounded-xl text-sm font-medium text-white/60 hover:text-white/90 transition-all border border-white/10 hover:border-white/20 hover:bg-white/5"
        >
          Руководство
        </NuxtLink>
      </div>

      <!-- Stats Row -->
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-6 sm:gap-8 mt-16 sm:mt-20 w-full max-w-3xl">
        <div class="text-center">
          <p class="text-2xl sm:text-3xl font-bold text-white tabular-nums">
            10K+
          </p>
          <p class="text-[10px] sm:text-xs text-white/30 mt-1 uppercase tracking-wider">
            Concurrent Sessions
          </p>
        </div>
        <div class="text-center">
          <p class="text-2xl sm:text-3xl font-bold text-white tabular-nums">
            1.2M+
          </p>
          <p class="text-[10px] sm:text-xs text-white/30 mt-1 uppercase tracking-wider">
            Events Processed
          </p>
        </div>
        <div class="text-center">
          <p class="text-2xl sm:text-3xl font-bold text-white tabular-nums">
            7ms
          </p>
          <p class="text-[10px] sm:text-xs text-white/30 mt-1 uppercase tracking-wider">
            P50 Latency
          </p>
        </div>
        <div class="text-center">
          <p class="text-2xl sm:text-3xl font-bold text-white tabular-nums">
            40+
          </p>
          <p class="text-[10px] sm:text-xs text-white/30 mt-1 uppercase tracking-wider">
            Detection Types
          </p>
        </div>
      </div>
    </section>

    <!-- ===== KEY CAPABILITIES ===== -->
    <section class="relative z-10 max-w-6xl mx-auto px-6 pb-24">
      <div class="text-center mb-12">
        <h2 class="text-2xl sm:text-3xl font-bold text-white">
          Ключевые технологии
        </h2>
        <p class="text-sm text-white/30 mt-3 max-w-xl mx-auto">
          Каждая функция построена на production-grade инфраструктуре — Kafka, ClickHouse, OPFS, MediaPipe
        </p>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
        <div
          v-for="cap in capabilities"
          :key="cap.title"
          class="group rounded-2xl p-6 transition-all duration-200 hover:translate-y-[-2px]"
          style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06);"
        >
          <div
            class="flex items-center justify-center size-12 rounded-xl mb-5 transition-all group-hover:scale-105"
            :style="{ background: cap.color + '15', border: '1px solid ' + cap.color + '25' }"
          >
            <UIcon
              :name="cap.icon"
              class="size-6"
              :style="{ color: cap.color }"
            />
          </div>
          <h3 class="text-base font-bold text-white mb-2">
            {{ cap.title }}
          </h3>
          <p class="text-sm text-white/35 leading-relaxed">
            {{ cap.description }}
          </p>
        </div>
      </div>
    </section>

    <!-- ===== ROLE-BASED ACCESS ===== -->
    <section class="relative z-10 max-w-5xl mx-auto px-6 pb-24">
      <div class="text-center mb-12">
        <h2 class="text-2xl sm:text-3xl font-bold text-white">
          Единая точка входа — три роли
        </h2>
        <p class="text-sm text-white/30 mt-3">
          Один логин. Система определяет вашу роль автоматически.
        </p>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
        <div
          v-for="role in roles"
          :key="role.name"
          class="rounded-2xl p-6 text-center transition-all hover:translate-y-[-2px]"
          style="background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.06);"
        >
          <div
            class="flex items-center justify-center size-14 rounded-2xl mx-auto mb-5"
            :style="{ background: role.color + '12', border: '1px solid ' + role.color + '20' }"
          >
            <UIcon
              :name="role.icon"
              class="size-7"
              :style="{ color: role.color }"
            />
          </div>
          <h3 class="text-base font-bold text-white mb-2">
            {{ role.name }}
          </h3>
          <p class="text-sm text-white/35 leading-relaxed">
            {{ role.description }}
          </p>
        </div>
      </div>

      <div class="flex justify-center mt-8">
        <button
          class="scanner-btn px-8 py-3.5 rounded-xl text-sm font-bold text-white transition-all"
          @click="openLogin"
        >
          <span class="flex items-center gap-2">
            <UIcon
              name="i-lucide-log-in"
              class="size-4"
            />
            Войти
          </span>
        </button>
      </div>
    </section>

    <!-- ===== PRICING SECTION ===== -->
    <section class="relative z-10 max-w-6xl mx-auto px-6 pb-24">
      <div class="text-center mb-12">
        <h2 class="text-2xl sm:text-3xl font-bold text-white">
          Тарифные планы
        </h2>
        <p class="text-sm text-white/30 mt-3 max-w-lg mx-auto">
          Прозрачная стоимость. Все функции AI включены в каждый план. Скидки за объём.
        </p>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-3 gap-5 items-start">
        <div
          v-for="(tier, idx) in pricingTiers"
          :key="tier.name"
          class="relative rounded-2xl p-6 transition-all hover:translate-y-[-2px] flex flex-col"
          :style="{
            background: idx === 1 ? 'rgba(66, 133, 244, 0.04)' : 'rgba(255,255,255,0.02)',
            border: idx === 1 ? '1px solid rgba(66, 133, 244, 0.25)' : '1px solid rgba(255,255,255,0.06)'
          }"
        >
          <!-- Badge -->
          <div
            v-if="tier.badge"
            class="absolute -top-3 left-1/2 -translate-x-1/2 px-4 py-1 rounded-full text-[10px] font-bold uppercase tracking-wider"
            :style="{ background: tier.accent, color: idx === 2 ? '#fff' : '#0a0e17' }"
          >
            {{ tier.badge }}
          </div>

          <!-- Tier Header -->
          <div
            class="mb-6"
            :class="{ 'pt-2': tier.badge }"
          >
            <div class="flex items-center gap-2 mb-1">
              <span
                class="text-xs font-medium uppercase tracking-wider"
                :style="{ color: tier.accent }"
              >{{ tier.nameEn }}</span>
            </div>
            <h3 class="text-xl font-bold text-white">
              {{ tier.name }}
            </h3>
            <p class="text-xs text-white/30 mt-1">
              {{ tier.audience }}
            </p>
          </div>

          <!-- Price -->
          <div class="mb-6">
            <span class="text-3xl font-extrabold text-white">{{ tier.price }}</span>
            <span
              v-if="tier.unit"
              class="text-sm text-white/30 ml-1"
            >{{ tier.unit }}</span>
          </div>

          <!-- Features List -->
          <ul class="space-y-3 mb-8 flex-1">
            <li
              v-for="feature in tier.features"
              :key="feature"
              class="flex items-start gap-2.5 text-sm"
            >
              <UIcon
                :name="feature.startsWith('Всё из') ? 'i-lucide-arrow-up-right' : 'i-lucide-check'"
                class="size-4 shrink-0 mt-0.5"
                :style="{ color: feature.startsWith('Всё из') ? tier.accent : 'rgba(255,255,255,0.25)' }"
              />
              <span
                :class="feature.startsWith('Всё из') ? 'font-medium' : ''"
                :style="{ color: feature.startsWith('Всё из') ? tier.accent : 'rgba(255,255,255,0.5)' }"
              >
                {{ feature }}
              </span>
            </li>
          </ul>

          <!-- CTA -->
          <button
            class="w-full py-3 rounded-xl text-sm font-bold transition-all cursor-pointer"
            :class="idx === 1 ? 'scanner-btn text-white' : 'text-white/70 hover:text-white'"
            :style="idx !== 1 ? { background: 'rgba(255,255,255,0.05)', border: '1px solid rgba(255,255,255,0.1)' } : {}"
            @click="openLogin"
          >
            {{ idx === 2 ? 'Связаться с нами' : 'Начать бесплатно' }}
          </button>
        </div>
      </div>
    </section>

    <!-- ===== FINAL CTA ===== -->
    <section class="relative z-10 max-w-4xl mx-auto px-6 pb-20">
      <div
        class="rounded-3xl p-10 sm:p-14 text-center"
        style="background: linear-gradient(135deg, rgba(66, 133, 244, 0.08), rgba(162, 89, 255, 0.08)); border: 1px solid rgba(255,255,255,0.06);"
      >
        <h2 class="text-2xl sm:text-3xl font-bold text-white mb-3">
          Готовы к трансформации?
        </h2>
        <p class="text-sm text-white/35 mb-8 max-w-md mx-auto">
          Запустите AI-прокторинг за 15 минут. Без установки ПО, без сложной интеграции.
        </p>
        <button
          class="scanner-btn px-10 py-4 rounded-xl text-base font-bold text-white transition-all"
          @click="openLogin"
        >
          Начать бесплатный пробный период
        </button>
        <p class="text-xs text-white/20 mt-5">
          14 дней бесплатно · Без привязки карты · Полный доступ
        </p>
      </div>
    </section>

    <!-- ===== FOOTER ===== -->
    <footer
      class="relative z-10 border-t py-8 px-6"
      style="border-color: rgba(255,255,255,0.06);"
    >
      <div class="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-4">
        <div class="flex items-center gap-2">
          <ArgusLogo :size="20" />
          <span class="text-xs text-white/20">Argus AI Proctoring Platform · v2.5.0</span>
        </div>
        <p class="text-xs text-white/20">
          &copy; {{ new Date().getFullYear() }} Argus AI. Все права защищены.
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
.scanner-btn {
  background: linear-gradient(135deg, #4285F4, #3b6fd4);
  box-shadow: 0 4px 20px rgba(66, 133, 244, 0.25), inset 0 1px 0 rgba(255, 255, 255, 0.1);
}
.scanner-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 30px rgba(66, 133, 244, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.15);
}
</style>
