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
</script>

<template>
  <div class="landing-root min-h-screen relative overflow-hidden" style="background: #0a0e17;">
    <!-- Ambient Background -->
    <div class="absolute inset-0 pointer-events-none overflow-hidden">
      <div class="absolute top-[-20%] left-[-10%] w-[60%] h-[60%] rounded-full opacity-[0.03]"
        style="background: radial-gradient(circle, #4285F4, transparent 70%);" />
      <div class="absolute bottom-[-15%] right-[-5%] w-[50%] h-[50%] rounded-full opacity-[0.03]"
        style="background: radial-gradient(circle, #A259FF, transparent 70%);" />
      <!-- Grid pattern -->
      <div class="absolute inset-0 opacity-[0.03]"
        style="background-image: linear-gradient(rgba(255,255,255,0.08) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.08) 1px, transparent 1px); background-size: 64px 64px;" />
    </div>

    <!-- ===== NAVIGATION BAR ===== -->
    <nav class="relative z-20 flex items-center justify-between px-8 py-5 max-w-7xl mx-auto">
      <div class="flex items-center gap-3">
        <ArgusLogo :size="32" />
        <span class="text-lg font-bold text-white tracking-tight">Argus AI</span>
      </div>
      <div class="flex items-center gap-4">
        <button
          class="text-sm text-white/50 hover:text-white/80 transition-colors px-4 py-2"
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
    <section class="relative z-10 flex flex-col items-center text-center px-6 pt-16 pb-24 max-w-5xl mx-auto">
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
      <h1 class="text-5xl sm:text-6xl lg:text-7xl font-extrabold text-white leading-[1.1] tracking-tight">
        <span class="block">Integrity at Scale.</span>
        <span class="block mt-2" style="background: linear-gradient(135deg, #4285F4, #A259FF); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text;">
          AI-Driven Proctoring.
        </span>
      </h1>

      <p class="mt-6 text-lg sm:text-xl text-white/40 max-w-2xl leading-relaxed">
        Real-time fraud detection, forensic evidence chain, and enterprise-grade exam monitoring for 10,000+ concurrent sessions.
      </p>

      <!-- CTA Buttons -->
      <div class="flex items-center gap-4 mt-10">
        <button
          class="scanner-btn px-8 py-4 rounded-xl text-base font-bold text-white transition-all"
          @click="openLogin"
        >
          Войти в панель управления
        </button>
        <NuxtLink
          to="/student-guide"
          class="px-6 py-4 rounded-xl text-sm font-medium text-white/60 hover:text-white/90 transition-all border border-white/10 hover:border-white/20 hover:bg-white/5"
        >
          Руководство
        </NuxtLink>
      </div>

      <!-- Stats Row -->
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-8 mt-20 w-full max-w-3xl">
        <div class="text-center">
          <p class="text-3xl font-bold text-white tabular-nums">10K+</p>
          <p class="text-xs text-white/30 mt-1 uppercase tracking-wider">Concurrent Sessions</p>
        </div>
        <div class="text-center">
          <p class="text-3xl font-bold text-white tabular-nums">1.2M+</p>
          <p class="text-xs text-white/30 mt-1 uppercase tracking-wider">Events Processed</p>
        </div>
        <div class="text-center">
          <p class="text-3xl font-bold text-white tabular-nums">7ms</p>
          <p class="text-xs text-white/30 mt-1 uppercase tracking-wider">P50 Latency</p>
        </div>
        <div class="text-center">
          <p class="text-3xl font-bold text-white tabular-nums">40+</p>
          <p class="text-xs text-white/30 mt-1 uppercase tracking-wider">Detection Types</p>
        </div>
      </div>
    </section>

    <!-- ===== FEATURES GRID ===== -->
    <section class="relative z-10 max-w-6xl mx-auto px-6 pb-24">
      <div class="grid grid-cols-1 md:grid-cols-3 gap-5">
        <!-- Feature Card 1 -->
        <div class="rounded-2xl p-6 transition-all hover:translate-y-[-2px]"
          style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06);">
          <div class="flex items-center justify-center size-12 rounded-xl mb-5"
            style="background: rgba(66, 133, 244, 0.1); border: 1px solid rgba(66, 133, 244, 0.15);">
            <UIcon name="i-lucide-scan-eye" class="size-6" style="color: #4285F4;" />
          </div>
          <h3 class="text-base font-bold text-white mb-2">Real-time AI Detection</h3>
          <p class="text-sm text-white/35 leading-relaxed">
            Gaze tracking, object detection, audio analysis, and behavioral patterns — 40+ event types with sub-30ms latency.
          </p>
        </div>
        <!-- Feature Card 2 -->
        <div class="rounded-2xl p-6 transition-all hover:translate-y-[-2px]"
          style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06);">
          <div class="flex items-center justify-center size-12 rounded-xl mb-5"
            style="background: rgba(162, 89, 255, 0.1); border: 1px solid rgba(162, 89, 255, 0.15);">
            <UIcon name="i-lucide-shield-check" class="size-6" style="color: #A259FF;" />
          </div>
          <h3 class="text-base font-bold text-white mb-2">Forensic Ledger</h3>
          <p class="text-sm text-white/35 leading-relaxed">
            SHA-256 hash-chained evidence with 2-year WORM retention. Tamper-proof chain of custody for legal proceedings.
          </p>
        </div>
        <!-- Feature Card 3 -->
        <div class="rounded-2xl p-6 transition-all hover:translate-y-[-2px]"
          style="background: rgba(255,255,255,0.03); border: 1px solid rgba(255,255,255,0.06);">
          <div class="flex items-center justify-center size-12 rounded-xl mb-5"
            style="background: rgba(52, 168, 83, 0.1); border: 1px solid rgba(52, 168, 83, 0.15);">
            <UIcon name="i-lucide-building-2" class="size-6" style="color: #34A853;" />
          </div>
          <h3 class="text-base font-bold text-white mb-2">Multi-Tenant SaaS</h3>
          <p class="text-sm text-white/35 leading-relaxed">
            Organization isolation with per-tenant rate limits, role-based access, and configurable retention policies.
          </p>
        </div>
      </div>
    </section>

    <!-- ===== FOOTER ===== -->
    <footer class="relative z-10 border-t py-8 px-6 text-center" style="border-color: rgba(255,255,255,0.06);">
      <p class="text-xs text-white/20">
        Argus AI Proctoring Platform &copy; {{ new Date().getFullYear() }}
      </p>
    </footer>

    <!-- Login Modal -->
    <LoginModal v-model="showLoginModal" @login-success="handleLoginSuccess" />
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
