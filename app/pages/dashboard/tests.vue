<script setup lang="ts">
import { useDashboardStore, type TestItem } from '~/stores/useDashboardStore'

const store = useDashboardStore()
const { accentBg, errorBg, successBg, warningBg, isDark } = useColors()

// --- Search ---
const searchQuery = ref('')
const searchFocused = ref(false)

// --- Filtered tests ---
const filteredTests = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return store.tests
  return store.tests.filter(t =>
    t.name.toLowerCase().includes(q)
    || t.subject.toLowerCase().includes(q)
    || t.author.toLowerCase().includes(q)
  )
})

// --- Stats ---
const totalTests = computed(() => store.tests.length)
const activeTests = computed(() => store.tests.filter(t => t.status === 'active').length)
const draftTests = computed(() => store.tests.filter(t => t.status === 'draft').length)
const totalQuestions = computed(() => store.tests.reduce((s, t) => s + t.questionsCount, 0))

// --- Delete modal ---
const deleteModalOpen = ref(false)
const testToDelete = ref<TestItem | null>(null)

function openDeleteModal(test: TestItem) {
  testToDelete.value = test
  deleteModalOpen.value = true
}

function confirmDelete() {
  if (testToDelete.value) {
    store.deleteTest(testToDelete.value.id)
  }
  deleteModalOpen.value = false
  testToDelete.value = null
}

function cancelDelete() {
  deleteModalOpen.value = false
  testToDelete.value = null
}

// Test status helpers delegated to useStatusHelpers composable
const { testStatusLabel: statusLabel, testStatusColor: statusColor, testStatusBg: statusBgFn } = useStatusHelpers()

function difficultyLabel(d: string): string {
  switch (d) {
    case 'easy': return 'Лёгкий'
    case 'medium': return 'Средний'
    case 'hard': return 'Сложный'
    default: return '—'
  }
}

function difficultyColor(d: string): string {
  switch (d) {
    case 'easy': return 'var(--argus-success)'
    case 'medium': return 'var(--argus-warning)'
    case 'hard': return 'var(--argus-error)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function difficultyBg(d: string, opacity: number): string {
  switch (d) {
    case 'easy': return successBg(opacity)
    case 'medium': return warningBg(opacity)
    case 'hard': return errorBg(opacity)
    default: return 'transparent'
  }
}
</script>

<template>
  <div class="p-6 space-y-5">
    <!-- ============================== -->
    <!--  HEADER                         -->
    <!-- ============================== -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-xl"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-file-text"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <h1
              class="text-xl font-bold"
              style="color: var(--argus-text);"
            >
              Тесты
            </h1>
            <p
              class="text-xs mt-0.5"
              style="color: var(--argus-text-dimmed);"
            >
              Управление тестовыми заданиями · Банк вопросов
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <button
          class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
          :style="{ background: 'var(--argus-accent)', color: '#fff' }"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          Создать тест
        </button>
      </div>
    </div>

    <!-- ============================== -->
    <!--  KPI CARDS                      -->
    <!-- ============================== -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: accentBg(0.08) }"
        >
          <UIcon
            name="i-lucide-file-text"
            class="size-4"
            style="color: var(--argus-accent);"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-text);"
          >
            {{ totalTests }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Всего
          </p>
        </div>
      </div>
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: successBg(0.2) }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: successBg(0.08) }"
        >
          <UIcon
            name="i-lucide-check-circle-2"
            class="size-4"
            style="color: var(--argus-success);"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-success);"
          >
            {{ activeTests }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Активные
          </p>
        </div>
      </div>
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: warningBg(0.2) }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: warningBg(0.08) }"
        >
          <UIcon
            name="i-lucide-pencil-line"
            class="size-4"
            style="color: var(--argus-warning);"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-warning);"
          >
            {{ draftTests }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Черновики
          </p>
        </div>
      </div>
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: isDark ? 'rgba(167, 139, 250, 0.08)' : 'rgba(139, 92, 246, 0.06)' }"
        >
          <UIcon
            name="i-lucide-help-circle"
            class="size-4"
            :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-text);"
          >
            {{ totalQuestions }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Вопросов
          </p>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  TABLE                          -->
    <!-- ============================== -->
    <div
      class="rounded-xl border overflow-hidden"
      :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
    >
      <!-- Search bar -->
      <div
        class="flex items-center gap-3 px-5 py-3 border-b"
        :style="{ borderColor: 'var(--argus-border)' }"
      >
        <div
          class="flex items-center gap-2 flex-1 px-3 py-2 rounded-lg transition-all"
          :style="{ background: 'var(--argus-bg-elevated)', border: `1px solid ${searchFocused ? 'var(--argus-accent)' : 'var(--argus-border)'}`, boxShadow: searchFocused ? `0 0 0 3px ${accentBg(0.1)}` : 'none' }"
        >
          <UIcon
            name="i-lucide-search"
            class="size-3.5 shrink-0"
            style="color: var(--argus-text-dimmed);"
          />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Поиск теста по названию, предмету или автору..."
            class="flex-1 bg-transparent text-xs font-medium outline-none placeholder:text-[var(--argus-text-dimmed)]"
            :style="{ color: 'var(--argus-text)' }"
            @focus="searchFocused = true"
            @blur="searchFocused = false"
          >
          <button
            v-if="searchQuery"
            class="flex items-center justify-center size-4 rounded-full cursor-pointer"
            style="color: var(--argus-text-dimmed);"
            @click="searchQuery = ''"
          >
            <UIcon
              name="i-lucide-x"
              class="size-3"
            />
          </button>
        </div>
        <span
          class="text-[10px] font-medium shrink-0"
          style="color: var(--argus-text-dimmed);"
        >{{ filteredTests.length }} из {{ totalTests }}</span>
      </div>

      <!-- Table header -->
      <div
        class="hidden lg:grid grid-cols-12 gap-2 px-5 py-2.5 text-[9px] font-bold uppercase tracking-wider border-b"
        :style="{ color: 'var(--argus-text-dimmed)', borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
      >
        <div class="col-span-4">
          Название теста
        </div>
        <div class="col-span-1 text-center">
          Статус
        </div>
        <div class="col-span-1 text-center">
          Сложность
        </div>
        <div class="col-span-1 text-center">
          Вопросы
        </div>
        <div class="col-span-1 text-center">
          Время
        </div>
        <div class="col-span-1 text-center">
          Проходной
        </div>
        <div class="col-span-1 text-center">
          Попытки
        </div>
        <div class="col-span-2 text-right">
          Действия
        </div>
      </div>

      <!-- Table rows -->
      <div v-if="filteredTests.length > 0">
        <div
          v-for="test in filteredTests"
          :key="test.id"
          class="grid grid-cols-1 lg:grid-cols-12 gap-2 lg:gap-2 px-5 py-3.5 items-center transition-all border-b last:border-b-0"
          :style="{ borderColor: 'var(--argus-border-subtle)' }"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
        >
          <!-- Name + Subject + Author -->
          <div class="lg:col-span-4 min-w-0">
            <p
              class="text-xs font-semibold truncate"
              style="color: var(--argus-text);"
            >
              {{ test.name }}
            </p>
            <div class="flex items-center gap-2 mt-0.5">
              <span
                class="text-[9px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >{{ test.subject }}</span>
              <span
                class="text-[8px]"
                style="color: var(--argus-border);"
              >·</span>
              <span
                class="text-[9px]"
                style="color: var(--argus-text-dimmed);"
              >{{ test.author }}</span>
              <span
                class="text-[8px]"
                style="color: var(--argus-border);"
              >·</span>
              <span
                class="text-[9px] font-mono"
                style="color: var(--argus-text-dimmed);"
              >{{ formatDate(test.updatedAt) }}</span>
            </div>
          </div>

          <!-- Status -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="inline-flex items-center gap-1 text-[9px] font-bold px-2 py-1 rounded-full"
              :style="{ background: statusBgFn(test.status, 0.1), color: statusColor(test.status) }"
            >
              <span
                class="size-1.5 rounded-full"
                :style="{ background: statusColor(test.status) }"
              />
              {{ statusLabel(test.status) }}
            </span>
          </div>

          <!-- Difficulty -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-[9px] font-bold px-2 py-1 rounded-full"
              :style="{ background: difficultyBg(test.difficulty, 0.1), color: difficultyColor(test.difficulty) }"
            >
              {{ difficultyLabel(test.difficulty) }}
            </span>
          </div>

          <!-- Questions count -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-xs font-bold tabular-nums"
              style="color: var(--argus-text);"
            >{{ test.questionsCount }}</span>
          </div>

          <!-- Duration -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-[10px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >{{ test.duration }} мин</span>
          </div>

          <!-- Pass score -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-[10px] font-bold tabular-nums"
              style="color: var(--argus-text);"
            >{{ test.passScore }}%</span>
          </div>

          <!-- Attempts -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-[10px] font-medium tabular-nums"
              style="color: var(--argus-text-dimmed);"
            >{{ test.attemptsAllowed }}</span>
          </div>

          <!-- Actions: Edit + Delete -->
          <div class="lg:col-span-2 flex items-center justify-end gap-1.5">
            <button
              class="flex items-center justify-center size-8 rounded-lg transition-all cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              title="Редактировать"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.1); ($event.currentTarget as HTMLElement).style.color = 'var(--argus-accent)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'; ($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-dimmed)'"
            >
              <UIcon
                name="i-lucide-pencil"
                class="size-3.5"
              />
            </button>
            <button
              class="flex items-center justify-center size-8 rounded-lg transition-all cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              title="Удалить"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.1); ($event.currentTarget as HTMLElement).style.color = 'var(--argus-error)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'; ($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-dimmed)'"
              @click="openDeleteModal(test)"
            >
              <UIcon
                name="i-lucide-trash-2"
                class="size-3.5"
              />
            </button>
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div
        v-else
        class="flex flex-col items-center justify-center py-16 px-4"
      >
        <div
          class="flex items-center justify-center size-16 rounded-2xl mb-4"
          :style="{ background: accentBg(0.06) }"
        >
          <UIcon
            name="i-lucide-file-search"
            class="size-8"
            style="color: var(--argus-accent); opacity: 0.35;"
          />
        </div>
        <p
          class="text-sm font-medium"
          style="color: var(--argus-text-dimmed);"
        >
          Тесты не найдены
        </p>
        <p
          class="text-[10px] mt-1 text-center max-w-xs"
          style="color: var(--argus-text-dimmed);"
        >
          Попробуйте изменить поисковый запрос или создайте новый тест
        </p>
      </div>
    </div>

    <!-- ============================== -->
    <!--  DELETE CONFIRMATION MODAL       -->
    <!-- ============================== -->
    <Teleport to="body">
      <Transition name="fade">
        <div
          v-if="deleteModalOpen"
          class="fixed inset-0 z-[100] flex items-center justify-center"
        >
          <!-- Overlay -->
          <div
            class="absolute inset-0"
            style="background: rgba(0, 0, 0, 0.6); backdrop-filter: blur(4px);"
            @click="cancelDelete"
          />

          <!-- Modal card -->
          <div
            class="relative z-10 w-full max-w-md mx-4 rounded-2xl border overflow-hidden"
            :style="{
              background: 'var(--argus-bg-card)',
              borderColor: 'var(--argus-border)',
              boxShadow: '0 24px 64px rgba(0, 0, 0, 0.4)'
            }"
          >
            <!-- Header -->
            <div class="flex items-center gap-3 px-6 py-5">
              <div
                class="flex items-center justify-center size-11 rounded-xl shrink-0"
                :style="{ background: errorBg(0.1), border: `1px solid ${errorBg(0.15)}` }"
              >
                <UIcon
                  name="i-lucide-alert-triangle"
                  class="size-5"
                  style="color: var(--argus-error);"
                />
              </div>
              <div>
                <h3
                  class="text-sm font-bold"
                  style="color: var(--argus-text);"
                >
                  Удалить тест?
                </h3>
                <p
                  class="text-[11px] mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Это действие нельзя отменить
                </p>
              </div>
            </div>

            <!-- Content -->
            <div class="px-6 pb-4">
              <div
                class="p-4 rounded-xl border"
                :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border-subtle)' }"
              >
                <p
                  class="text-xs font-semibold"
                  style="color: var(--argus-text);"
                >
                  {{ testToDelete?.name }}
                </p>
                <div class="flex items-center gap-2 mt-1.5">
                  <span
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >{{ testToDelete?.subject }}</span>
                  <span
                    class="text-[8px]"
                    style="color: var(--argus-border);"
                  >·</span>
                  <span
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >{{ testToDelete?.questionsCount }} вопросов</span>
                  <span
                    class="text-[8px]"
                    style="color: var(--argus-border);"
                  >·</span>
                  <span
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >{{ testToDelete?.author }}</span>
                </div>
              </div>
              <p
                class="text-[11px] mt-3 leading-relaxed"
                style="color: var(--argus-text-muted);"
              >
                Вы уверены, что хотите удалить этот тест? Все связанные данные, включая историю прохождений и результаты, будут удалены безвозвратно.
              </p>
            </div>

            <!-- Actions -->
            <div
              class="flex items-center justify-end gap-3 px-6 py-4 border-t"
              :style="{ borderColor: 'var(--argus-border)' }"
            >
              <button
                class="px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
                :style="{ background: 'var(--argus-bg-elevated)', color: 'var(--argus-text-dimmed)', border: '1px solid var(--argus-border)' }"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-elevated)'"
                @click="cancelDelete"
              >
                Отмена
              </button>
              <button
                class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold text-white transition-all cursor-pointer"
                :style="{ background: 'var(--argus-error)' }"
                @mouseenter="($event.currentTarget as HTMLElement).style.opacity = '0.9'"
                @mouseleave="($event.currentTarget as HTMLElement).style.opacity = '1'"
                @click="confirmDelete"
              >
                <UIcon
                  name="i-lucide-trash-2"
                  class="size-3.5"
                />
                Удалить
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
