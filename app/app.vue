<script setup lang="ts">
import { useDashboardStore } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'

useHead({
  meta: [
    { name: 'viewport', content: 'width=device-width, initial-scale=1' }
  ],
  link: [
    { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' },
    { rel: 'icon', href: '/favicon.ico' }
  ],
  htmlAttrs: {
    lang: 'ru'
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

// Dashboard sub-pages (visible to all roles)
const dashboardChildren = computed(() => {
  const items = [
    { label: 'Обзор', icon: 'i-lucide-gauge', to: '/dashboard' },
    { label: 'Регионы', icon: 'i-lucide-map-pin', to: '/dashboard/regions' },
    { label: 'Нарушения', icon: 'i-heroicons-exclamation-triangle', to: '/dashboard/violations' },
  ]
  // Infrastructure and Executive are global/system pages — Super Admin only
  if (authStore.isSuperAdmin) {
    items.splice(1, 0, { label: 'Инфраструктура', icon: 'i-lucide-server', to: '/dashboard/infrastructure' })
    items.splice(2, 0, { label: 'Executive', icon: 'i-lucide-shield-check', to: '/dashboard/executive' })
  }
  return items
})

// Other top-level navigation items (visible to all roles)
const topNavigation = computed(() => {
  const items = [
    { label: 'Экзамены', icon: 'i-lucide-graduation-cap', to: '/dashboard/exams' },
    { label: 'Тесты', icon: 'i-lucide-file-text', to: '/dashboard/tests' },
    { label: 'Комплексные тесты', icon: 'i-lucide-layers', to: '/dashboard/complex-tests' },
    { label: 'Мониторинг', icon: 'i-lucide-video', to: '/monitoring' },
    { label: 'Архив сессий', icon: 'i-lucide-archive', to: '/archive' },
    { label: 'Форензик', icon: 'i-lucide-file-search', to: '/forensic' },
    { label: 'Апелляции', icon: 'i-lucide-scale', to: '/appeals' },
    { label: 'Экспорт', icon: 'i-lucide-hard-drive-download', to: '/dashboard/exports' },
    { label: 'Аналитика', icon: 'i-lucide-bar-chart-3', to: '/analytics' },
  ]
  // API & Integrations is a system page — Super Admin only
  if (authStore.isSuperAdmin) {
    items.push({ label: 'API и Интеграции', icon: 'i-lucide-plug', to: '/api' })
  }
  return items
})

// Super Admin-only navigation items
const adminNavigation = [
  { label: 'Организации', icon: 'i-lucide-building-2', to: '/organizations' },
]

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
const isLandingPage = computed(() => route.path === '/' || route.path === '/landing')

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

// Simulate real-time data
let interval: ReturnType<typeof setInterval> | null = null
onMounted(() => {
  interval = setInterval(() => {
    store.simulateNewViolation()
  }, 8000)
})
onUnmounted(() => {
  if (interval) clearInterval(interval)
})
</script>

<template>
  <UApp :toaster="{ position: 'top-right', expand: true }">
    <!-- Landing page renders without sidebar chrome -->
    <NuxtPage v-if="isLandingPage" />

    <div v-else class="flex h-screen overflow-hidden" style="background: var(--argus-bg-deep);">
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
          <ArgusLogo :size="30" />

          <span
            class="text-[15px] font-semibold tracking-tight sidebar-label"
            :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            style="color: var(--argus-text);"
          >
            Argus AI
          </span>

          <button
            v-if="store.leftSidebarOpen"
            class="ml-auto flex items-center justify-center size-7 rounded-md transition-colors"
            style="color: var(--argus-text-dimmed);"
            title="Свернуть"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            @click="store.toggleLeftSidebar()"
          >
            <UIcon name="i-lucide-panel-left-close" class="size-4" />
          </button>

          <button
            v-else
            class="absolute left-[72px] top-[18px] z-20 flex items-center justify-center size-6 rounded-full border shadow-sm transition-all"
            :style="{
              background: 'var(--argus-bg-card)',
              borderColor: 'var(--argus-border)',
              color: 'var(--argus-text-dimmed)'
            }"
            title="Развернуть"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-card)'"
            @click="store.toggleLeftSidebar()"
          >
            <UIcon name="i-lucide-panel-left-open" class="size-3.5" />
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
              <UIcon name="i-lucide-layout-dashboard" class="size-[18px] shrink-0" />
              <span
                class="sidebar-label flex-1 text-left"
                :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
              >
                Дашборд
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
                Дашборд
              </div>
            </button>

            <!-- Dashboard children (collapsible) -->
            <div
              v-if="store.leftSidebarOpen"
              class="sub-menu-container"
              :class="store.dashboardMenuOpen ? 'sub-menu-open' : 'sub-menu-closed'"
            >
              <div class="ml-4 mt-1 space-y-0.5 border-l" style="border-color: var(--argus-border);">
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
                  <UIcon :name="child.icon" class="size-[15px] shrink-0" />
                  <span>{{ child.label }}</span>
                </NuxtLink>
              </div>
            </div>
          </div>

          <!-- ===== SUPER ADMIN SECTION ===== -->
          <template v-if="authStore.isSuperAdmin">
            <div class="my-2 mx-3 border-t" style="border-color: var(--argus-border-subtle);" />
            <div
              v-if="store.leftSidebarOpen"
              class="px-3 py-1"
            >
              <span class="text-[10px] font-semibold uppercase tracking-widest" style="color: var(--argus-text-muted);">
                Администрирование
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
              <UIcon :name="item.icon" class="size-[18px] shrink-0" />
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
          <div class="my-2 mx-3 border-t" style="border-color: var(--argus-border-subtle);" />

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
            <UIcon :name="item.icon" class="size-[18px] shrink-0" />
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

        <!-- Bottom: Org Switcher + Theme Toggle + User -->
        <div class="px-2 py-3 border-t space-y-2" style="border-color: var(--argus-border);">
          <!-- Global Organization Switcher (Super Admin only) -->
          <OrgSwitcher v-if="store.leftSidebarOpen" />

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
              {{ isDark ? 'Светлая тема' : 'Тёмная тема' }}
            </span>
          </button>

          <!-- User Profile -->
          <div
            class="flex items-center rounded-lg"
            :class="store.leftSidebarOpen ? 'gap-3 px-3 py-2' : 'justify-center py-2'"
          >
            <div class="flex items-center justify-center size-8 rounded-full text-xs font-bold shrink-0" style="background: linear-gradient(135deg, var(--argus-accent), var(--argus-accent-muted)); color: white;">
              {{ authStore.displayName?.charAt(0) || 'A' }}
            </div>
            <div
              class="flex-1 min-w-0 sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            >
              <p class="text-sm font-medium truncate" style="color: var(--argus-text);">{{ authStore.displayName }}</p>
              <p class="text-xs truncate" style="color: var(--argus-text-dimmed);">{{ authStore.displayEmail }}</p>
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
            <UIcon name="i-lucide-log-out" class="size-[18px] shrink-0" />
            <span
              class="text-sm font-medium sidebar-label"
              :class="store.leftSidebarOpen ? 'sidebar-label-visible' : 'sidebar-label-hidden'"
            >
              Выйти
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
              Выйти
            </div>
          </button>
        </div>
      </aside>

      <!-- ===== MAIN + RIGHT PANEL ===== -->
      <div class="flex-1 flex flex-col overflow-hidden main-fluid">
        <!-- Top bar (mobile) -->
        <header class="lg:hidden flex items-center justify-between px-4 h-14 border-b" style="border-color: var(--argus-border); background: var(--argus-bg-card);">
          <ArgusLogo :size="26" :show-text="true" />
          <button
            class="flex items-center justify-center size-8 rounded-lg"
            style="color: var(--argus-text-dimmed);"
            @click="toggleTheme()"
          >
            <UIcon :name="isDark ? 'i-lucide-moon' : 'i-lucide-sun'" class="size-5" />
          </button>
        </header>

        <!-- Page content -->
        <main class="flex-1 overflow-y-auto">
          <NuxtPage />
        </main>
      </div>
    </div>
  </UApp>
</template>
