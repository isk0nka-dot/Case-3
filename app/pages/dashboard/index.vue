<script setup lang="ts">
import { useDashboardStore, type ActiveExam, type ViolationAlert } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'

const store = useDashboardStore()
const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()
const { monitoringStatusLabel: statusLabel, monitoringStatusColor: statusBadgeColor, monitoringStatusBg: statusBadgeBg } = useStatusHelpers()
const toast = useToast()
const route = useRoute()
const router = useRouter()

// Handle "Permission Denied" redirect from middleware
onMounted(() => {
  if (route.query.denied === '1') {
    toast.add({
      title: 'Доступ запрещён',
      description: 'У вас нет прав для просмотра этой страницы. Требуются права Супер Администратора.',
      icon: 'i-lucide-shield-alert',
      color: 'error'
    })
    // Clean up the URL query parameter
    router.replace({ path: '/dashboard', query: {} })
  }
})

// --- Exam Selector ---
const examSelectorOpen = ref(false)
const examSearchTerm = ref('')

const examOptions = computed(() => {
  const q = examSearchTerm.value.trim().toLowerCase()
  const exams = store.activeExams
  if (!q) return exams
  return exams.filter(e => e.examName.toLowerCase().includes(q))
})

function handleExamSelect(examId: string | null) {
  store.selectExam(examId)
  examSelectorOpen.value = false
  examSearchTerm.value = ''
}

// --- Student Search ---
const searchFocused = ref(false)

function delayedBlur() {
  setTimeout(() => { searchFocused.value = false }, 200)
}

// --- Formatters (delegated to shared composable) ---
const formatTime = formatTimeShort
const formatStartTime = formatTimeShort

// Status helpers delegated to useStatusHelpers composable
</script>

<template>
  <div class="flex h-full">
    <!-- ===== MAIN SCROLLABLE CONTENT ===== -->
    <div class="flex-1 overflow-y-auto p-6 space-y-6 main-fluid">
      <!-- Page header row -->
      <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1
            class="text-2xl font-bold"
            style="color: var(--argus-text);"
          >
            Дашборд
          </h1>
          <p
            class="text-sm mt-1"
            style="color: var(--argus-text-dimmed);"
          >
            Мониторинг в реальном времени — Интеграция Eduser
          </p>
        </div>

        <div class="flex items-center gap-3">
          <!-- Active filter badge with reset -->
          <Transition name="filter-badge">
            <button
              v-if="store.isFiltered"
              class="flex items-center gap-1.5 px-2.5 py-1.5 rounded-full text-xs font-medium transition-all border"
              :style="{
                background: accentBg(0.06),
                borderColor: accentBg(0.2),
                color: 'var(--argus-accent)'
              }"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.12)"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.06)"
              @click="store.clearFilter()"
            >
              <UIcon
                name="i-lucide-x"
                class="size-3"
              />
              Сбросить
            </button>
          </Transition>

          <div
            class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
            :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
          >
            <span class="relative flex size-1.5">
              <span
                class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
                :class="isDark ? 'bg-emerald-400' : 'bg-emerald-500'"
              />
              <span
                class="relative inline-flex size-1.5 rounded-full"
                :class="isDark ? 'bg-emerald-400' : 'bg-emerald-600'"
              />
            </span>
            Система активна
          </div>
          <!-- Right panel toggle button -->
          <button
            class="hidden xl:flex items-center gap-1.5 text-xs font-medium px-3 py-1.5 rounded-full transition-colors"
            :style="{
              background: store.rightPanelOpen ? errorBg(0.1) : 'var(--argus-bg-hover)',
              color: store.rightPanelOpen ? 'var(--argus-error)' : 'var(--argus-text-dimmed)'
            }"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = store.rightPanelOpen ? errorBg(0.15) : 'var(--argus-border)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = store.rightPanelOpen ? errorBg(0.1) : 'var(--argus-bg-hover)'"
            @click="store.toggleRightPanel()"
          >
            <UIcon
              :name="store.rightPanelOpen ? 'i-lucide-panel-right-close' : 'i-lucide-panel-right-open'"
              class="size-3.5"
            />
            <span
              v-if="!store.rightPanelOpen"
              class="relative"
            >
              Нарушения
              <span class="absolute -top-1.5 -right-4 flex items-center justify-center size-4 rounded-full text-[9px] font-bold text-white bg-red-500">
                {{ store.filteredCriticalAlerts.length }}
              </span>
            </span>
            <span v-else>Скрыть</span>
          </button>
        </div>
      </div>

      <!-- ===== FILTER + SEARCH — SINGLE ROW ===== -->
      <div class="flex items-center gap-4">
        <!-- Exam Filter (~25% width) -->
        <div class="relative w-1/4 min-w-48 shrink-0">
          <button
            class="flex items-center gap-2 w-full px-3.5 py-2.5 rounded-xl text-sm font-medium transition-all border"
            :style="{
              background: store.isFiltered ? accentBg(0.08) : 'var(--argus-bg-card)',
              borderColor: store.isFiltered ? 'var(--argus-accent)' : 'var(--argus-border)',
              color: store.isFiltered ? 'var(--argus-accent)' : 'var(--argus-text-muted)'
            }"
            @click="examSelectorOpen = !examSelectorOpen"
          >
            <UIcon
              name="i-lucide-filter"
              class="size-4 shrink-0"
            />
            <span class="flex-1 text-left truncate">
              {{ store.selectedExam ? store.selectedExam.examName : 'Все экзамены' }}
            </span>
            <UIcon
              name="i-lucide-chevron-down"
              class="size-3.5 shrink-0 transition-transform duration-200"
              :class="examSelectorOpen ? 'rotate-180' : ''"
            />
          </button>

          <!-- Dropdown panel -->
          <Transition name="dropdown">
            <div
              v-if="examSelectorOpen"
              class="absolute z-50 mt-1.5 left-0 w-80 rounded-xl border shadow-xl overflow-hidden"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <!-- Search within exams -->
              <div
                class="p-2 border-b"
                style="border-color: var(--argus-border);"
              >
                <div
                  class="flex items-center gap-2 px-3 py-2 rounded-lg"
                  :style="{ background: 'var(--argus-bg-hover)' }"
                >
                  <UIcon
                    name="i-heroicons-magnifying-glass"
                    class="size-3.5 shrink-0"
                    style="color: var(--argus-text-dimmed);"
                  />
                  <input
                    v-model="examSearchTerm"
                    type="text"
                    placeholder="Поиск экзамена..."
                    class="w-full bg-transparent text-sm outline-none"
                    :style="{ color: 'var(--argus-text)' }"
                  >
                </div>
              </div>

              <!-- Options -->
              <div class="max-h-64 overflow-y-auto py-1">
                <!-- All exams option -->
                <button
                  class="flex items-center gap-3 w-full px-4 py-2.5 text-left text-sm transition-colors"
                  :style="{
                    color: !store.isFiltered ? 'var(--argus-accent)' : 'var(--argus-text-muted)',
                    background: !store.isFiltered ? accentBg(0.06) : 'transparent'
                  }"
                  @mouseenter="!store.isFiltered ? null : (($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)')"
                  @mouseleave="!store.isFiltered ? null : (($event.currentTarget as HTMLElement).style.background = 'transparent')"
                  @click="handleExamSelect(null)"
                >
                  <UIcon
                    name="i-lucide-globe"
                    class="size-4 shrink-0"
                  />
                  <span class="font-medium">Все экзамены</span>
                  <span
                    class="ml-auto text-xs"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ store.totalParticipants }}
                  </span>
                </button>

                <div
                  class="mx-3 my-1 border-t"
                  style="border-color: var(--argus-border-subtle);"
                />

                <!-- Individual exam options -->
                <button
                  v-for="exam in examOptions"
                  :key="exam.id"
                  class="flex items-center gap-3 w-full px-4 py-2.5 text-left text-sm transition-colors"
                  :style="{
                    color: store.selectedExamId === exam.id ? 'var(--argus-accent)' : 'var(--argus-text)',
                    background: store.selectedExamId === exam.id ? accentBg(0.06) : 'transparent'
                  }"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = store.selectedExamId === exam.id ? accentBg(0.06) : 'var(--argus-bg-hover)'"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = store.selectedExamId === exam.id ? accentBg(0.06) : 'transparent'"
                  @click="handleExamSelect(exam.id)"
                >
                  <UIcon
                    name="i-lucide-book-open"
                    class="size-4 shrink-0"
                    style="color: var(--argus-text-dimmed);"
                  />
                  <div class="flex-1 min-w-0">
                    <p class="font-medium truncate">
                      {{ exam.examName }}
                    </p>
                    <p
                      class="text-[10px] mt-0.5"
                      style="color: var(--argus-text-dimmed);"
                    >
                      {{ exam.participants }} участников
                    </p>
                  </div>
                  <span
                    class="text-[10px] font-medium px-1.5 py-0.5 rounded-full shrink-0"
                    :style="{
                      background: exam.violationRate >= 5 ? errorBg(0.1) : exam.violationRate >= 3 ? warningBg(0.1) : successBg(0.1),
                      color: exam.violationRate >= 5 ? 'var(--argus-error)' : exam.violationRate >= 3 ? 'var(--argus-warning)' : 'var(--argus-success)'
                    }"
                  >
                    {{ exam.violationRate }}%
                  </span>
                </button>
              </div>
            </div>
          </Transition>

          <!-- Click-away overlay -->
          <div
            v-if="examSelectorOpen"
            class="fixed inset-0 z-40"
            @click="examSelectorOpen = false"
          />
        </div>

        <!-- Search Bar (~75% width) -->
        <div class="relative flex-1">
          <div
            class="flex items-center gap-3 px-4 py-2.5 rounded-xl border transition-all"
            :style="{
              background: 'var(--argus-bg-card)',
              borderColor: searchFocused ? 'var(--argus-accent)' : 'var(--argus-border)',
              boxShadow: searchFocused ? `0 0 0 3px ${accentBg(0.1)}` : 'none'
            }"
          >
            <UIcon
              name="i-heroicons-magnifying-glass"
              class="size-5 shrink-0"
              :style="{ color: searchFocused ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)' }"
            />
            <input
              v-model="store.studentSearchQuery"
              type="text"
              :placeholder="store.isFiltered
                ? `Поиск в «${store.selectedExam?.examName ?? ''}» — по имени или ИИН...`
                : 'Поиск студента среди 10 000+ участников — по имени или ИИН...'
              "
              class="w-full bg-transparent text-sm outline-none"
              :style="{ color: 'var(--argus-text)' }"
              @focus="searchFocused = true"
              @blur="delayedBlur"
            >
            <span
              v-if="store.studentSearchQuery"
              class="text-[10px] font-medium px-2 py-0.5 rounded-full whitespace-nowrap"
              :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
            >
              {{ store.searchResults.length }} результатов
            </span>
          </div>

          <!-- Search Results Dropdown -->
          <Transition name="dropdown">
            <div
              v-if="store.searchResults.length > 0 && searchFocused"
              class="absolute z-50 mt-2 left-0 right-0 rounded-xl border shadow-xl overflow-hidden"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <div
                class="px-4 py-2.5 border-b flex items-center justify-between"
                style="border-color: var(--argus-border);"
              >
                <span
                  class="text-[11px] font-medium uppercase tracking-wider"
                  style="color: var(--argus-text-dimmed);"
                >
                  Результаты поиска
                </span>
                <span
                  class="text-[10px]"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ store.searchResults.length }} найдено
                </span>
              </div>
              <div class="max-h-80 overflow-y-auto">
                <div
                  v-for="student in store.searchResults"
                  :key="student.id"
                  class="flex items-center gap-3 px-4 py-3 transition-colors cursor-pointer"
                  style="border-bottom: 1px solid var(--argus-border-subtle);"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                >
                  <!-- Status dot -->
                  <div
                    class="size-2 rounded-full shrink-0"
                    :style="{ background: statusBadgeColor(student.status) }"
                  />
                  <!-- Info -->
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <p
                        class="text-sm font-medium truncate"
                        style="color: var(--argus-text);"
                      >
                        {{ student.name }}
                      </p>
                      <span
                        class="text-[9px] font-bold px-1.5 py-0.5 rounded-full uppercase shrink-0"
                        :style="{ background: statusBadgeBg(student.status, 0.1), color: statusBadgeColor(student.status) }"
                      >
                        {{ statusLabel(student.status) }}
                      </span>
                    </div>
                    <p
                      class="text-[11px] mt-0.5"
                      style="color: var(--argus-text-dimmed);"
                    >
                      ИИН: {{ student.iin }} · {{ student.examName }}
                    </p>
                  </div>
                  <!-- Integrity -->
                  <div class="text-right shrink-0">
                    <p
                      class="text-sm font-bold tabular-nums"
                      :style="{ color: student.integrityScore < 50 ? 'var(--argus-error)' : student.integrityScore < 70 ? 'var(--argus-warning)' : 'var(--argus-text)' }"
                    >
                      {{ student.integrityScore }}%
                    </p>
                    <p
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >
                      {{ student.violations }} нарушений
                    </p>
                  </div>
                </div>
              </div>
            </div>
          </Transition>
        </div>
      </div>

      <!-- KPI Cards -->
      <div class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4">
        <!-- Активные сессии -->
        <div class="glass-card rounded-xl p-5">
          <div class="flex items-start justify-between">
            <div>
              <p
                class="text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Активные сессии
              </p>
              <p
                class="text-3xl font-bold mt-2"
                style="color: var(--argus-text);"
              >
                {{ store.filteredActiveSessions }}
              </p>
              <p
                class="text-xs mt-1.5"
                style="color: var(--argus-text-dimmed);"
              >
                {{ store.filteredTotalParticipants }} {{ store.isFiltered ? 'в экзамене' : 'всего участников' }}
              </p>
            </div>
            <div
              class="flex items-center justify-center size-10 rounded-lg"
              :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
            >
              <UIcon
                name="i-lucide-users"
                class="size-5"
                style="color: var(--argus-accent);"
              />
            </div>
          </div>
        </div>

        <!-- Критические нарушения -->
        <div class="glass-card glow-error rounded-xl p-5">
          <div class="flex items-start justify-between">
            <div>
              <p
                class="text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Критические нарушения
              </p>
              <p
                class="text-3xl font-bold mt-2"
                style="color: var(--argus-error);"
              >
                {{ store.filteredCriticalViolations }}
              </p>
              <p
                class="text-xs mt-1.5"
                style="color: var(--argus-text-dimmed);"
              >
                {{ store.filteredCriticalAlerts.length }} нерешённых
              </p>
            </div>
            <div
              class="flex items-center justify-center size-10 rounded-lg"
              :style="{ background: errorBg(0.1), border: `1px solid ${errorBg(0.15)}` }"
            >
              <UIcon
                name="i-lucide-shield-alert"
                class="size-5"
                style="color: var(--argus-error);"
              />
            </div>
          </div>
        </div>

        <!-- Средний балл честности -->
        <div class="glass-card rounded-xl p-5">
          <div class="flex items-start justify-between">
            <div class="w-full">
              <p
                class="text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Средний балл честности
              </p>
              <p
                class="text-3xl font-bold mt-2"
                style="color: var(--argus-text);"
              >
                {{ store.filteredAvgIntegrity }}%
              </p>
              <div
                class="mt-3 w-full h-1.5 rounded-full overflow-hidden"
                style="background: var(--argus-bg-hover);"
              >
                <div
                  class="h-full rounded-full transition-all duration-700"
                  :style="{
                    width: `${store.filteredAvgIntegrity}%`,
                    background: store.filteredAvgIntegrity >= 80
                      ? (isDark ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)')
                      : store.filteredAvgIntegrity >= 60
                        ? (isDark ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)')
                        : (isDark ? 'linear-gradient(90deg, #F87171, #EF4444)' : 'linear-gradient(90deg, #E03E3E, #C92B2B)')
                  }"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- Состояние системы (Super Admin only — global infrastructure metric) -->
        <div
          v-if="authStore.isSuperAdmin"
          class="glass-card rounded-xl p-5"
        >
          <div class="flex items-start justify-between">
            <div>
              <p
                class="text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Состояние системы
              </p>
              <div class="flex items-center gap-2 mt-2">
                <span class="relative flex size-2.5">
                  <span
                    class="absolute inline-flex h-full w-full animate-ping rounded-full opacity-75"
                    :class="store.systemHealth === 'operational' ? 'bg-emerald-400' : store.systemHealth === 'degraded' ? 'bg-yellow-400' : 'bg-red-400'"
                  />
                  <span
                    class="relative inline-flex size-2.5 rounded-full"
                    :class="store.systemHealth === 'operational' ? 'bg-emerald-500' : store.systemHealth === 'degraded' ? 'bg-yellow-500' : 'bg-red-500'"
                  />
                </span>
                <p
                  class="text-lg font-bold"
                  style="color: var(--argus-text);"
                >
                  {{ store.systemHealthLabel }}
                </p>
              </div>
              <p
                class="text-xs mt-1.5"
                style="color: var(--argus-text-dimmed);"
              >
                {{ store.systemHealthUptime }}% аптайм
              </p>
            </div>
            <div
              class="flex items-center justify-center size-10 rounded-lg"
              :style="{ background: successBg(0.1), border: `1px solid ${successBg(0.15)}` }"
            >
              <UIcon
                name="i-lucide-server"
                class="size-5"
                style="color: var(--argus-success);"
              />
            </div>
          </div>
        </div>

        <!-- Экзамены завершённые (Org Admin replacement card when system card is hidden) -->
        <div
          v-else
          class="glass-card rounded-xl p-5"
        >
          <div class="flex items-start justify-between">
            <div>
              <p
                class="text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Экзамены за сегодня
              </p>
              <p
                class="text-3xl font-bold mt-2"
                style="color: var(--argus-text);"
              >
                {{ store.activeExams.length }}
              </p>
              <p
                class="text-xs mt-1.5"
                style="color: var(--argus-text-dimmed);"
              >
                Активных экзаменов
              </p>
            </div>
            <div
              class="flex items-center justify-center size-10 rounded-lg"
              :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
            >
              <UIcon
                name="i-lucide-calendar-check"
                class="size-5"
                style="color: var(--argus-accent);"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Main Grid: Exams Table + Violation Trends -->
      <div class="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <!-- Active Exams Table -->
        <div class="glass-card rounded-xl overflow-hidden">
          <div
            class="flex items-center justify-between px-5 py-4 border-b"
            style="border-color: var(--argus-border);"
          >
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-book-open"
                class="size-4"
                style="color: var(--argus-text-dimmed);"
              />
              <h2
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Активные экзамены
              </h2>
            </div>
            <span
              class="text-[10px] font-medium px-2 py-1 rounded-full"
              :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
            >
              {{ store.activeExams.length }} активных
            </span>
          </div>

          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr style="border-bottom: 1px solid var(--argus-border);">
                  <th
                    class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Название экзамена
                  </th>
                  <th
                    class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Время начала
                  </th>
                  <th
                    class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Участники
                  </th>
                  <th
                    class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Уровень нарушений
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="exam in store.activeExams"
                  :key="exam.id"
                  class="transition-colors"
                  :style="{
                    borderBottom: '1px solid var(--argus-border-subtle)',
                    background: store.selectedExamId === exam.id ? accentBg(0.04) : 'transparent'
                  }"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = store.selectedExamId === exam.id ? accentBg(0.06) : 'var(--argus-bg-hover)'"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = store.selectedExamId === exam.id ? accentBg(0.04) : 'transparent'"
                >
                  <td class="px-5 py-3">
                    <button
                      class="font-medium text-left transition-colors exam-link"
                      :style="{
                        color: store.selectedExamId === exam.id ? 'var(--argus-accent)' : 'var(--argus-text)'
                      }"
                      @click="store.selectExam(store.selectedExamId === exam.id ? null : exam.id)"
                    >
                      {{ exam.examName }}
                    </button>
                  </td>
                  <td
                    class="px-5 py-3"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ formatStartTime(exam.startTime) }}
                  </td>
                  <td class="px-5 py-3">
                    <div class="flex items-center gap-1.5">
                      <UIcon
                        name="i-lucide-users"
                        class="size-3.5"
                        style="color: var(--argus-text-dimmed);"
                      />
                      <span style="color: var(--argus-text-muted);">{{ exam.participants }}</span>
                    </div>
                  </td>
                  <td class="px-5 py-3">
                    <span
                      class="text-xs font-medium px-2 py-1 rounded-full"
                      :style="{
                        background: exam.violationRate >= 5 ? errorBg(0.1) : exam.violationRate >= 3 ? warningBg(0.1) : successBg(0.1),
                        color: exam.violationRate >= 5 ? 'var(--argus-error)' : exam.violationRate >= 3 ? 'var(--argus-warning)' : 'var(--argus-success)'
                      }"
                    >
                      {{ exam.violationRate }}%
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Violation Trends Chart -->
        <div class="glass-card rounded-xl overflow-hidden">
          <div
            class="flex items-center justify-between px-5 py-4 border-b"
            style="border-color: var(--argus-border);"
          >
            <div class="flex items-center gap-2">
              <UIcon
                name="i-lucide-trending-up"
                class="size-4"
                style="color: var(--argus-text-dimmed);"
              />
              <h2
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Тренды нарушений
              </h2>
            </div>
            <span
              class="text-[10px] font-medium px-2 py-1 rounded-full"
              :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }"
            >
              Последние 12 часов
            </span>
          </div>

          <div class="p-5">
            <ViolationChart
              :data="store.violationTrends"
              :dark-mode="isDark"
            />
          </div>
        </div>
      </div>

      <!-- Bottom: Студенты в зоне риска -->
      <div class="glass-card rounded-xl overflow-hidden">
        <div
          class="flex items-center justify-between px-5 py-4 border-b"
          style="border-color: var(--argus-border);"
        >
          <div class="flex items-center gap-2">
            <UIcon
              name="i-lucide-alert-triangle"
              class="size-4"
              style="color: var(--argus-error);"
            />
            <h2
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Студенты в зоне риска
            </h2>
          </div>
          <p
            class="text-[11px]"
            style="color: var(--argus-text-dimmed);"
          >
            {{ store.isFiltered ? store.selectedExam?.examName : 'Наименьшие баллы честности среди активных экзаменов' }}
          </p>
        </div>

        <div v-if="store.filteredAtRiskStudents.length > 0">
          <div
            v-for="(student, index) in store.filteredAtRiskStudents"
            :key="student.id"
            class="flex items-center gap-4 px-5 py-3.5 transition-colors"
            style="border-bottom: 1px solid var(--argus-border-subtle);"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          >
            <div
              class="flex items-center justify-center size-7 rounded-full text-xs font-bold"
              :style="{ background: 'var(--argus-bg-hover)', border: '1px solid var(--argus-border)', color: 'var(--argus-text-dimmed)' }"
            >
              {{ index + 1 }}
            </div>

            <div class="flex-1 min-w-0">
              <p
                class="text-sm font-medium truncate"
                style="color: var(--argus-text);"
              >
                {{ student.name }}
              </p>
              <p
                class="text-xs truncate"
                style="color: var(--argus-text-dimmed);"
              >
                {{ student.examName }}
              </p>
            </div>

            <span
              class="text-[11px] font-medium px-2.5 py-1 rounded-full"
              :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
            >
              {{ student.violations }} нарушений
            </span>

            <div class="w-28 text-right">
              <p
                class="text-sm font-bold"
                :style="{
                  color: student.integrityScore < 40 ? 'var(--argus-error)' : student.integrityScore < 60 ? 'var(--argus-warning)' : 'var(--argus-text)'
                }"
              >
                {{ student.integrityScore }}%
              </p>
              <div
                class="mt-1 w-full h-1 rounded-full overflow-hidden"
                style="background: var(--argus-bg-hover);"
              >
                <div
                  class="h-full rounded-full transition-all duration-500"
                  :style="{
                    width: `${student.integrityScore}%`,
                    background: student.integrityScore < 40
                      ? (isDark ? 'linear-gradient(90deg, #F87171, #EF4444)' : 'linear-gradient(90deg, #E03E3E, #C92B2B)')
                      : student.integrityScore < 60
                        ? (isDark ? 'linear-gradient(90deg, #FBBF24, #F59E0B)' : 'linear-gradient(90deg, #E67E22, #C96E1A)')
                        : (isDark ? 'linear-gradient(90deg, #34D399, #10B981)' : 'linear-gradient(90deg, #10A34A, #0D8A3E)')
                  }"
                />
              </div>
            </div>
          </div>
        </div>

        <!-- Empty state when filtered with no results -->
        <div
          v-else
          class="flex flex-col items-center justify-center py-12 px-6"
        >
          <UIcon
            name="i-lucide-check-circle"
            class="size-10 mb-3"
            style="color: var(--argus-success);"
          />
          <p
            class="text-sm font-medium"
            style="color: var(--argus-text);"
          >
            Нет студентов в зоне риска
          </p>
          <p
            class="text-xs mt-1"
            style="color: var(--argus-text-dimmed);"
          >
            В выбранном экзамене нет студентов с низким баллом честности
          </p>
        </div>
      </div>
    </div>

    <!-- ===== RIGHT PANEL: Нарушения (лента) ===== -->
    <aside
      class="hidden xl:flex flex-col shrink-0 right-panel-transition border-l"
      :style="{
        width: store.rightPanelOpen ? '320px' : '0px',
        borderColor: store.rightPanelOpen ? 'var(--argus-border)' : 'transparent',
        background: 'var(--argus-bg-card)',
        opacity: store.rightPanelOpen ? 1 : 0
      }"
    >
      <div
        v-if="store.rightPanelOpen"
        class="flex flex-col h-full w-80"
      >
        <!-- Header -->
        <div
          class="flex items-center justify-between px-4 h-14 border-b shrink-0"
          style="border-color: var(--argus-border);"
        >
          <div class="flex items-center gap-2">
            <span class="relative flex size-2">
              <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-75" />
              <span class="relative inline-flex size-2 rounded-full bg-red-500" />
            </span>
            <h2
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Нарушения
            </h2>
            <span
              v-if="store.isFiltered"
              class="text-[9px] font-medium px-1.5 py-0.5 rounded-full"
              :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
            >
              Фильтр
            </span>
          </div>
          <div class="flex items-center gap-2">
            <span
              class="text-[10px] font-medium px-2 py-0.5 rounded-full"
              :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
            >
              {{ store.filteredAlerts.length }}
            </span>
            <button
              class="flex items-center justify-center size-6 rounded-md transition-colors"
              style="color: var(--argus-text-dimmed);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              @click="store.toggleRightPanel()"
            >
              <UIcon
                name="i-lucide-x"
                class="size-3.5"
              />
            </button>
          </div>
        </div>

        <!-- Feed -->
        <div class="flex-1 overflow-y-auto">
          <div
            v-for="alert in store.filteredAlerts"
            :key="alert.id"
            class="px-4 py-3 transition-colors cursor-pointer"
            style="border-bottom: 1px solid var(--argus-border-subtle);"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          >
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0 flex-1">
                <p
                  class="text-sm font-medium truncate"
                  style="color: var(--argus-text);"
                >
                  {{ alert.studentName }}
                </p>
                <div class="flex items-center gap-1.5 mt-1">
                  <span
                    class="text-[10px] font-medium px-2 py-0.5 rounded-full"
                    :style="{
                      background: alert.severity === 'critical' ? errorBg(0.1) : alert.severity === 'warning' ? warningBg(0.1) : 'var(--argus-bg-hover)',
                      color: alert.severity === 'critical' ? 'var(--argus-error)' : alert.severity === 'warning' ? 'var(--argus-warning)' : 'var(--argus-text-dimmed)'
                    }"
                  >
                    {{ alert.violationType }}
                  </span>
                </div>
                <p
                  class="text-[10px] mt-1"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ alert.examName }}
                </p>
              </div>
              <div class="flex flex-col items-end gap-1.5 shrink-0">
                <span
                  class="text-[10px]"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ formatTime(alert.timestamp) }}
                </span>
                <button
                  class="text-[11px] font-medium px-2 py-1 rounded-md transition-colors flex items-center gap-1"
                  :style="{ color: 'var(--argus-accent)', background: accentBg(0.08) }"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.15)"
                  @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.08)"
                  @click.stop="store.openMonitoringModalFromAlert(alert)"
                >
                  <UIcon
                    name="i-lucide-eye"
                    class="size-3"
                  />
                  Просмотр
                </button>
              </div>
            </div>
          </div>

          <!-- Empty feed -->
          <div
            v-if="store.filteredAlerts.length === 0"
            class="flex flex-col items-center justify-center py-12 px-4"
          >
            <UIcon
              name="i-lucide-check-circle"
              class="size-8 mb-2"
              style="color: var(--argus-success);"
            />
            <p
              class="text-xs text-center"
              style="color: var(--argus-text-dimmed);"
            >
              Нет нарушений для выбранного экзамена
            </p>
          </div>
        </div>
      </div>
    </aside>

    <!-- Monitoring Modal (shared component) -->
    <MonitoringModal
      :session="store.monitoringModalSession"
      :is-live="store.monitoringModalIsLive"
      @close="store.closeMonitoringModal()"
    />
  </div>
</template>
