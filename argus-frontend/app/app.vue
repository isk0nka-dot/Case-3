<script setup lang="ts">
import { useDashboardStore } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'
import { LOCALES } from '~/i18n/messages'

const { t, locale, setLocale } = useArgusI18n()

const currentLocaleLabel = computed(
  () => LOCALES.find(l => l.code === locale.value)?.label ?? 'RU'
)
function cycleLocale() {
  const idx = LOCALES.findIndex(l => l.code === locale.value)
  setLocale(LOCALES[(idx + 1) % LOCALES.length]!.code)
}

useHead({
  meta: [
    { name: 'viewport', content: 'width=device-width, initial-scale=1' }
  ],
  link: [
    { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' },
    { rel: 'icon', href: '/favicon.ico' }
  ],
  htmlAttrs: {
    lang: locale
  }
})

useSeoMeta({
  title: 'Argus AI — Панель прокторинга',
  description: 'AI-система прокторинга экзаменов'
})

const store = useDashboardStore()
const authStore = useAuthStore()
const colorMode = useColorMode()

const isDark = computed(() => colorMode.value === 'dark')

function toggleTheme() {
  colorMode.preference = isDark.value ? 'light' : 'dark'
}

// Dashboard sub-pages
const dashboardChildren = computed(() => {
  return [
    { label: t('nav.overview'), icon: 'i-lucide-gauge', to: '/dashboard' }
  ]
})

// Other top-level navigation items
const topNavigation = computed(() => {
  return [
    { label: t('nav.monitoring'), icon: 'i-lucide-video', to: '/monitoring' },
    { label: t('nav.archive'), icon: 'i-lucide-archive', to: '/archive' }
  ]
})

// Super Admin-only navigation items
const adminNavigation = computed(() => [
  { label: t('nav.organizations'), icon: 'i-lucide-building-2', to: '/organizations' }
])

const route = useRoute()

// Check if current route is under dashboard (exclude exams — now top-level)
const isDashboardRoute = computed(() => {
  if (route.path === '/dashboard/exams') return false
  if (route.path === '/dashboard/tests') return false
  if (route.path === '/dashboard/complex-tests') return false
  if (route.path === '/dashboard/exports') return false
  return route.path === '/dashboard' || route.path.startsWith('/dashboard/')
})

// Landing page should render without sidebar chrome
// Landing pages render without sidebar chrome
const isLandingPage = computed(() => route.path === '/' || route.path === '/landing' || route.path.startsWith('/docs'))

// Restore auth session eagerly during setup (not onMounted) so that
// child pages' onMounted hooks already have the JWT token available.
// In Vue 3, child onMounted fires *before* parent onMounted, so
// deferring to onMounted causes a race: the child fetches with a null
// token → 401 → authStore.logout() wipes everything.
if (import.meta.client) {
  authStore.restoreSession()
}

function handleLogout() {
  authStore.logout()
  navigateTo('/')
}

// Performance Debugger (Ctrl+Shift+D)
const showDebugger = ref(false)

onMounted(() => {
  // Ctrl+Shift+D toggles PerformanceDebugger overlay
  window.addEventListener('keydown', (e: KeyboardEvent) => {
    if (e.ctrlKey && e.shiftKey && e.key === 'D') {
      e.preventDefault()
      showDebugger.value = !showDebugger.value
    }
  })
})
</script>

<template>
  <UApp :toaster="{ position: 'top-right', expand: true }">
    <!-- Landing page renders without sidebar chrome -->
    <NuxtPage v-if="isLandingPage" />

    <div
      v-else
      class="flex h-screen overflow-hidden"
      style="background: var(--argus-bg-deep);"
    >
      <!-- ===== LEFT SIDEBAR ===== -->
      <aside
        class="hidden lg:flex flex-col shrink-0 sidebar-transition border-r"
        :style="{
          width: store.leftSidebarOpen ? '256px' : '72px',
          borderColor: 'var(--argus-border)',
          background: 'var(--argus-bg-card)'
        }"
      >
        <!-- Logo + Collapse Toggle -->
        <div
          class="flex items-center h-16 border-b shrink-0"
          :class="store.leftSidebarOpen ? 'px-5 gap-2.5' : 'px-0 justify-center'"
          style="border-color: var(--argus-border);"
        >
          <NuxtLink
            to="/"
            class="flex items-center gap-2.5 cursor-pointer hover:opacity-80 transition-opacity"
          >
            <ArgusLogo :size="30" />

            <span
              class="text-[15px] font-semibold tracking-tight sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
              style="color: var(--argus-text);"
            >
              Argus AI
            </span>
          </NuxtLink>

          <button
            v-if="store.leftSidebarOpen"
            class="ml-auto flex items-center justify-center size-7 rounded-md transition-colors"
            style="color: var(--argus-text-dimmed);"
            :title="t('common.collapse')"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            @click="store.toggleLeftSidebar()"
          >
            <UIcon
              name="i-lucide-panel-left-close"
              class="size-4"
            />
          </button>

          <button
            v-else
            class="absolute left-[72px] top-[18px] z-20 flex items-center justify-center size-6 rounded-full border shadow-sm transition-all"
            :style="{
              background: 'var(--argus-bg-card)',
              borderColor: 'var(--argus-border)',
              color: 'var(--argus-text-dimmed)'
            }"
            :title="t('common.expand')"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-card)'"
            @click="store.toggleLeftSidebar()"
          >
            <UIcon
              name="i-lucide-panel-left-open"
              class="size-3.5"
            />
          </button>
        </div>

        <!-- Nav items -->
        <nav class="flex-1 px-2 py-4 space-y-1 overflow-y-auto">
          <!-- ===== DASHBOARD PARENT (collapsible) ===== -->
          <div>
            <!-- Dashboard parent button -->
            <button
              class="flex items-center w-full rounded-lg text-sm font-medium transition-all duration-200 group relative"
              :class="[
                store.leftSidebarOpen ? 'gap-3 px-3 py-2.5' : 'justify-center px-0 py-2.5',
                isDashboardRoute ? 'glow-accent' : ''
              ]"
              :style="isDashboardRoute
                ? `background: ${isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)'}; color: var(--argus-accent);`
                : 'color: var(--argus-text-dimmed);'
              "
              @mouseenter="($event.currentTarget as HTMLElement).style.background = isDashboardRoute ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)') : 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = isDashboardRoute ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)') : 'transparent'"
              @click="store.leftSidebarOpen ? store.toggleDashboardMenu() : store.toggleLeftSidebar()"
            >
              <UIcon
                name="i-lucide-layout-dashboard"
                class="size-[18px] shrink-0"
              />
              <span
                class="sidebar-label flex-1 text-left"
                :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
              >
                {{ t('nav.dashboard') }}
              </span>
              <!-- Chevron indicator -->
              <UIcon
                v-if="store.leftSidebarOpen"
                name="i-lucide-chevron-down"
                class="size-3.5 shrink-0 chevron-icon"
                :class="store.dashboardMenuOpen ? 'chevron-open' : 'chevron-closed'"
              />

              <!-- Tooltip when collapsed -->
              <div
                v-if="!store.leftSidebarOpen"
                class="absolute left-full ml-2 px-2.5 py-1.5 rounded-md text-xs font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-50"
                :style="{
                  background: 'var(--argus-bg-elevated)',
                  color: 'var(--argus-text)',
                  border: '1px solid var(--argus-border)',
                  boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
                }"
              >
                {{ t('nav.dashboard') }}
              </div>
            </button>

            <!-- Dashboard children (collapsible) -->
            <div
              v-if="store.leftSidebarOpen"
              class="sub-menu-container"
              :class="store.dashboardMenuOpen ? 'sub-menu-open' : 'sub-menu-closed'"
            >
              <div
                class="ml-4 mt-1 space-y-0.5 border-l"
                style="border-color: var(--argus-border);"
              >
                <NuxtLink
                  v-for="child in dashboardChildren"
                  :key="child.label"
                  :to="child.to"
                  class="flex items-center gap-2.5 pl-4 pr-3 py-2 rounded-r-lg text-[13px] font-medium transition-all duration-200 group/child relative"
                  :style="route.path === child.to
                    ? `background: ${isDark ? 'rgba(56, 189, 248, 0.08)' : 'rgba(37, 99, 235, 0.08)'}; color: var(--argus-accent); border-left: 2px solid var(--argus-accent); margin-left: -1px;`
                    : 'color: var(--argus-text-dimmed);'
                  "
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = route.path === child.to ? (isDark ? 'rgba(56, 189, 248, 0.08)' : 'rgba(37, 99, 235, 0.08)') : 'var(--argus-bg-hover)'"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = route.path === child.to ? (isDark ? 'rgba(56, 189, 248, 0.08)' : 'rgba(37, 99, 235, 0.08)') : 'transparent'"
                >
                  <UIcon
                    :name="child.icon"
                    class="size-[15px] shrink-0"
                  />
                  <span>{{ child.label }}</span>
                </NuxtLink>
              </div>
            </div>
          </div>

          <!-- ===== SUPER ADMIN SECTION ===== -->
          <template v-if="authStore.isSuperAdmin">
            <div
              class="my-2 mx-3 border-t"
              style="border-color: var(--argus-border-subtle);"
            />
            <div
              v-if="store.leftSidebarOpen"
              class="px-3 py-1"
            >
              <span
                class="text-[10px] font-semibold uppercase tracking-widest"
                style="color: var(--argus-text-muted);"
              >
                {{ t('common.administration') }}
              </span>
            </div>
            <NuxtLink
              v-for="item in adminNavigation"
              :key="item.label"
              :to="item.to"
              class="flex items-center rounded-lg text-sm font-medium transition-all duration-200 group relative"
              :class="[
                store.leftSidebarOpen ? 'gap-3 px-3 py-2.5' : 'justify-center px-0 py-2.5',
                route.path === item.to ? 'glow-accent' : ''
              ]"
              :style="route.path === item.to
                ? `background: ${isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)'}; color: var(--argus-accent);`
                : 'color: var(--argus-text-dimmed);'
              "
              @mouseenter="($event.currentTarget as HTMLElement).style.background = route.path === item.to ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)') : 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = route.path === item.to ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)') : 'transparent'"
            >
              <UIcon
                :name="item.icon"
                class="size-[18px] shrink-0"
              />
              <span
                class="sidebar-label"
                :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
              >
                {{ item.label }}
              </span>

              <!-- Tooltip when collapsed -->
              <div
                v-if="!store.leftSidebarOpen"
                class="absolute left-full ml-2 px-2.5 py-1.5 rounded-md text-xs font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-50"
                :style="{
                  background: 'var(--argus-bg-elevated)',
                  color: 'var(--argus-text)',
                  border: '1px solid var(--argus-border)',
                  boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
                }"
              >
                {{ item.label }}
              </div>
            </NuxtLink>
          </template>

          <!-- Separator -->
          <div
            class="my-2 mx-3 border-t"
            style="border-color: var(--argus-border-subtle);"
          />

          <!-- ===== OTHER NAV ITEMS ===== -->
          <NuxtLink
            v-for="item in topNavigation"
            :key="item.label"
            :to="item.to"
            class="flex items-center rounded-lg text-sm font-medium transition-all duration-200 group relative"
            :class="[
              store.leftSidebarOpen ? 'gap-3 px-3 py-2.5' : 'justify-center px-0 py-2.5',
              route.path === item.to ? 'glow-accent' : ''
            ]"
            :style="route.path === item.to
              ? `background: ${isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)'}; color: var(--argus-accent);`
              : 'color: var(--argus-text-dimmed);'
            "
            @mouseenter="($event.currentTarget as HTMLElement).style.background = route.path === item.to ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)') : 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = route.path === item.to ? (isDark ? 'rgba(56, 189, 248, 0.1)' : 'rgba(37, 99, 235, 0.1)') : 'transparent'"
          >
            <UIcon
              :name="item.icon"
              class="size-[18px] shrink-0"
            />
            <span
              class="sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            >
              {{ item.label }}
            </span>

            <!-- Tooltip when collapsed -->
            <div
              v-if="!store.leftSidebarOpen"
              class="absolute left-full ml-2 px-2.5 py-1.5 rounded-md text-xs font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-50"
              :style="{
                background: 'var(--argus-bg-elevated)',
                color: 'var(--argus-text)',
                border: '1px solid var(--argus-border)',
                boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
              }"
            >
              {{ item.label }}
            </div>
          </NuxtLink>
        </nav>

        <!-- Bottom: Language + Theme Toggle + User -->
        <div
          class="px-2 py-3 border-t space-y-2"
          style="border-color: var(--argus-border);"
        >
          <!-- Language switcher (KZ / RU / EN) -->
          <div
            v-if="store.leftSidebarOpen"
            class="flex items-center gap-1 px-1"
          >
            <button
              v-for="l in LOCALES"
              :key="l.code"
              class="flex-1 text-[11px] font-semibold py-1.5 rounded-md transition-colors"
              :style="locale === l.code
                ? 'background: var(--argus-accent); color: #fff;'
                : 'color: var(--argus-text-dimmed);'"
              :title="l.name"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = locale === l.code ? 'var(--argus-accent)' : 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = locale === l.code ? 'var(--argus-accent)' : 'transparent'"
              @click="setLocale(l.code)"
            >
              {{ l.label }}
            </button>
          </div>
          <button
            v-else
            class="flex items-center justify-center w-full py-2 rounded-lg text-[11px] font-semibold transition-colors"
            style="color: var(--argus-text-dimmed);"
            :title="t('common.language')"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            @click="cycleLocale()"
          >
            {{ currentLocaleLabel }}
          </button>

          <!-- Theme Toggle -->
          <button
            class="flex items-center w-full rounded-lg transition-all duration-200"
            :class="store.leftSidebarOpen ? 'gap-3 px-3 py-2.5' : 'justify-center py-2.5'"
            style="color: var(--argus-text-dimmed);"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            @click="toggleTheme()"
          >
            <!-- Sun/Moon animated icon -->
            <div class="relative size-[18px] shrink-0">
              <UIcon
                name="i-lucide-moon"
                class="size-[18px] absolute inset-0 theme-toggle-icon"
                :style="{ opacity: isDark ? 1 : 0, transform: isDark ? 'rotate(0deg) scale(1)' : 'rotate(-90deg) scale(0.5)' }"
              />
              <UIcon
                name="i-lucide-sun"
                class="size-[18px] absolute inset-0 theme-toggle-icon"
                :style="{ opacity: isDark ? 0 : 1, transform: isDark ? 'rotate(90deg) scale(0.5)' : 'rotate(0deg) scale(1)' }"
              />
            </div>
            <span
              class="text-sm font-medium sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            >
              {{ isDark ? t('common.lightTheme') : t('common.darkTheme') }}
            </span>
          </button>

          <!-- User Profile -->
          <div
            class="flex items-center rounded-lg"
            :class="store.leftSidebarOpen ? 'gap-3 px-3 py-2' : 'justify-center py-2'"
          >
            <div
              class="flex items-center justify-center size-8 rounded-full text-xs font-bold shrink-0"
              style="background: linear-gradient(135deg, var(--argus-accent), var(--argus-accent-muted)); color: white;"
            >
              {{ authStore.displayName?.charAt(0) || 'A' }}
            </div>
            <div
              class="flex-1 min-w-0 sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            >
              <p
                class="text-sm font-medium truncate"
                style="color: var(--argus-text);"
              >
                {{ authStore.displayName }}
              </p>
              <p
                class="text-xs truncate"
                style="color: var(--argus-text-dimmed);"
              >
                {{ authStore.displayEmail }}
              </p>
            </div>
          </div>

          <!-- Logout Button -->
          <button
            class="flex items-center w-full rounded-lg transition-all duration-200 group relative"
            :class="store.leftSidebarOpen ? 'gap-3 px-3 py-2.5' : 'justify-center py-2.5'"
            style="color: var(--argus-text-dimmed);"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            @click="handleLogout"
          >
            <UIcon
              name="i-lucide-log-out"
              class="size-[18px] shrink-0"
            />
            <span
              class="text-sm font-medium sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            >
              {{ t('common.logout') }}
            </span>

            <!-- Tooltip when collapsed -->
            <div
              v-if="!store.leftSidebarOpen"
              class="absolute left-full ml-2 px-2.5 py-1.5 rounded-md text-xs font-medium whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-50"
              :style="{
                background: 'var(--argus-bg-elevated)',
                color: 'var(--argus-text)',
                border: '1px solid var(--argus-border)',
                boxShadow: '0 4px 12px rgba(0,0,0,0.15)'
              }"
            >
              {{ t('common.logout') }}
            </div>
          </button>
        </div>
      </aside>

      <!-- ===== MAIN + RIGHT PANEL ===== -->
      <div class="flex-1 flex flex-col overflow-hidden main-fluid">
        <!-- Top bar (mobile) -->
        <header
          class="lg:hidden flex items-center justify-between px-4 h-14 border-b"
          style="border-color: var(--argus-border); background: var(--argus-bg-card);"
        >
          <ArgusLogo
            :size="26"
            :show-text="true"
          />
          <button
            class="flex items-center justify-center size-8 rounded-lg"
            style="color: var(--argus-text-dimmed);"
            @click="toggleTheme()"
          >
            <UIcon
              :name="isDark ? 'i-lucide-moon' : 'i-lucide-sun'"
              class="size-5"
            />
          </button>
        </header>

        <!-- Page content -->
        <main class="flex-1 overflow-y-auto">
          <NuxtPage />
        </main>
      </div>
    </div>

    <!-- Performance Debugger Overlay (Ctrl+Shift+D) -->
    <PerformanceDebugger
      v-if="showDebugger"
      @close="showDebugger = false"
    />
  </UApp>
</template>


