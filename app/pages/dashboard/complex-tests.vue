<script setup lang="ts">
import { useDashboardStore, type ComplexTestItem } from '~/stores/useDashboardStore'

const store = useDashboardStore()
const { isDark, accentBg, errorBg, successBg, purpleBg } = useColors()

// --- Search ---
const searchQuery = ref('')
const searchFocused = ref(false)

// --- Filtered ---
const filteredComplexTests = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return store.complexTests
  return store.complexTests.filter(t =>
    t.name.toLowerCase().includes(q)
    || t.subjects.some(s => s.toLowerCase().includes(q))
    || t.author.toLowerCase().includes(q)
  )
})

// --- Stats ---
const totalComplexTests = computed(() => store.complexTests.length)
const activeComplexTests = computed(() => store.complexTests.filter(t => t.status === 'active').length)
const totalSubTests = computed(() => store.complexTests.reduce((s, t) => s + t.testsCount, 0))

// --- Delete modal ---
const deleteModalOpen = ref(false)
const testToDelete = ref<ComplexTestItem | null>(null)

function openDeleteModal(test: ComplexTestItem) {
  testToDelete.value = test
  deleteModalOpen.value = true
}

function confirmDelete() {
  if (testToDelete.value) {
    store.deleteComplexTest(testToDelete.value.id)
  }
  deleteModalOpen.value = false
  testToDelete.value = null
}

function cancelDelete() {
  deleteModalOpen.value = false
  testToDelete.value = null
}

// --- Formatters (delegated to useStatusHelpers composable) ---
const { testStatusLabel: statusLabel, testStatusColor: statusColor, testStatusBg: statusBgFn } = useStatusHelpers()
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
            :style="{ background: purpleBg(0.1), border: `1px solid ${purpleBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-layers"
              class="size-5"
              :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
            />
          </div>
          <div>
            <h1
              class="text-xl font-bold"
              style="color: var(--argus-text);"
            >
              Комплексные тесты
            </h1>
            <p
              class="text-xs mt-0.5"
              style="color: var(--argus-text-dimmed);"
            >
              Составные экзамены из нескольких тестов · Блоковая система
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3">
        <button
          class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
          :style="{ background: isDark ? '#a78bfa' : '#7c3aed', color: '#fff' }"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          Создать комплексный тест
        </button>
      </div>
    </div>

    <!-- ============================== -->
    <!--  KPI CARDS                      -->
    <!-- ============================== -->
    <div class="grid grid-cols-2 sm:grid-cols-3 gap-3">
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: purpleBg(0.2) }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: purpleBg(0.08) }"
        >
          <UIcon
            name="i-lucide-layers"
            class="size-4"
            :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-text);"
          >
            {{ totalComplexTests }}
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
            {{ activeComplexTests }}
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
        :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: accentBg(0.08) }"
        >
          <UIcon
            name="i-lucide-puzzle"
            class="size-4"
            style="color: var(--argus-accent);"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-text);"
          >
            {{ totalSubTests }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Суб-тестов
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
          :style="{ background: 'var(--argus-bg-elevated)', border: `1px solid ${searchFocused ? (isDark ? '#a78bfa' : '#7c3aed') : 'var(--argus-border)'}`, boxShadow: searchFocused ? `0 0 0 3px ${purpleBg(0.1)}` : 'none' }"
        >
          <UIcon
            name="i-lucide-search"
            class="size-3.5 shrink-0"
            style="color: var(--argus-text-dimmed);"
          />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Поиск по названию, предмету или автору..."
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
        >{{ filteredComplexTests.length }} из {{ totalComplexTests }}</span>
      </div>

      <!-- Table header -->
      <div
        class="hidden lg:grid grid-cols-12 gap-2 px-5 py-2.5 text-[9px] font-bold uppercase tracking-wider border-b"
        :style="{ color: 'var(--argus-text-dimmed)', borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
      >
        <div class="col-span-4">
          Название
        </div>
        <div class="col-span-2 text-center">
          Предметы
        </div>
        <div class="col-span-1 text-center">
          Статус
        </div>
        <div class="col-span-1 text-center">
          Тестов
        </div>
        <div class="col-span-1 text-center">
          Вопросов
        </div>
        <div class="col-span-1 text-center">
          Время
        </div>
        <div class="col-span-2 text-right">
          Действия
        </div>
      </div>

      <!-- Table rows -->
      <div v-if="filteredComplexTests.length > 0">
        <div
          v-for="test in filteredComplexTests"
          :key="test.id"
          class="grid grid-cols-1 lg:grid-cols-12 gap-2 lg:gap-2 px-5 py-3.5 items-center transition-all border-b last:border-b-0"
          :style="{ borderColor: 'var(--argus-border-subtle)' }"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
        >
          <!-- Name + Author + Date -->
          <div class="lg:col-span-4 min-w-0">
            <p
              class="text-xs font-semibold truncate"
              style="color: var(--argus-text);"
            >
              {{ test.name }}
            </p>
            <div class="flex items-center gap-2 mt-0.5">
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
              >{{ formatDate(test.createdAt) }}</span>
            </div>
          </div>

          <!-- Subjects -->
          <div class="lg:col-span-2 flex lg:justify-center">
            <div class="flex flex-wrap gap-1">
              <span
                v-for="subj in test.subjects"
                :key="subj"
                class="text-[8px] font-bold px-1.5 py-0.5 rounded"
                :style="{ background: purpleBg(0.08), color: isDark ? '#a78bfa' : '#7c3aed' }"
              >
                {{ subj }}
              </span>
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

          <!-- Tests count -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-xs font-bold tabular-nums"
              style="color: var(--argus-text);"
            >{{ test.testsCount }}</span>
          </div>

          <!-- Total questions -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-[10px] font-medium tabular-nums"
              style="color: var(--argus-text-dimmed);"
            >{{ test.totalQuestions }}</span>
          </div>

          <!-- Duration -->
          <div class="lg:col-span-1 flex lg:justify-center">
            <span
              class="text-[10px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >{{ test.totalDuration }} мин</span>
          </div>

          <!-- Actions: Edit + Delete -->
          <div class="lg:col-span-2 flex items-center justify-end gap-1.5">
            <button
              class="flex items-center justify-center size-8 rounded-lg transition-all cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              title="Редактировать"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = purpleBg(0.1); ($event.currentTarget as HTMLElement).style.color = isDark ? '#a78bfa' : '#7c3aed'"
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
          :style="{ background: purpleBg(0.06) }"
        >
          <UIcon
            name="i-lucide-layers"
            class="size-8"
            :style="{ color: isDark ? '#a78bfa' : '#7c3aed', opacity: 0.35 }"
          />
        </div>
        <p
          class="text-sm font-medium"
          style="color: var(--argus-text-dimmed);"
        >
          Комплексные тесты не найдены
        </p>
        <p
          class="text-[10px] mt-1 text-center max-w-xs"
          style="color: var(--argus-text-dimmed);"
        >
          Попробуйте изменить поисковый запрос или создайте новый комплексный тест
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
                  Удалить комплексный тест?
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
                  >{{ testToDelete?.testsCount }} тестов</span>
                  <span
                    class="text-[8px]"
                    style="color: var(--argus-border);"
                  >·</span>
                  <span
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >{{ testToDelete?.totalQuestions }} вопросов</span>
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
                Вы уверены, что хотите удалить этот комплексный тест? Все включённые суб-тесты и результаты будут удалены безвозвратно.
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
