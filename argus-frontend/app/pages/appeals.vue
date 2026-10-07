<script setup lang="ts">
/**
 * Appeals Page — Full appeal lifecycle management.
 *
 * Features:
 * - List all appeals with status filters
 * - Click appeal to view detail (GET /api/v1/appeals/{appealId})
 * - Review/update appeal status (PUT /api/v1/appeals/{appealId}/review)
 * - Status badge colors and icons for all 5 states
 * - Linked session navigation
 */
import { useAdminAPI, type AppealDetail } from '~/composables/useAdminAPI'
import { useAuthStore } from '~/stores/useAuthStore'
import { useColors } from '~/composables/useColors'
import { formatDateTime, formatDate } from '~/composables/useFormatters'
import {
  appealStatusLabel as statusLabel,
  appealStatusColor as statusColor,
  appealStatusIcon as statusIcon,
  appealStatusBg,
  isAppealTerminal as isTerminal
} from '~/composables/useStatusHelpers'

const api = useAdminAPI()
const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()

// Wrap appealStatusBg with our colors instance
function statusBgColor(status: string, opacity: number): string {
  return appealStatusBg(status, opacity, { accentBg, warningBg, successBg, errorBg, infoBg: (o: number) => isDark.value ? `rgba(100, 116, 139, ${o})` : `rgba(148, 163, 184, ${o})` })
}

// ---------------------------------------------------------------------------
// Reactive state
// ---------------------------------------------------------------------------
const loading = ref(false)
const error = ref('')
const statusFilter = ref('all')
const appeals = ref<AppealDetail[]>([])
const selectedAppeal = ref<AppealDetail | null>(null)
const reviewNotes = ref('')
const reviewError = ref('')
const reviewSuccess = ref('')
const reviewSubmitting = ref(false)
const detailLoading = ref(false)
type ReviewStatus = 'under_review' | 'upheld' | 'overturned' | 'withdrawn'
const reviewStatus = ref<ReviewStatus>('upheld')

// Fetch all appeals
async function fetchAppeals() {
  loading.value = true
  error.value = ''
  try {
    const params: Record<string, string> = {}
    if (statusFilter.value !== 'all') {
      params.status = statusFilter.value
    }
    appeals.value = await api.listAppeals(params) as AppealDetail[]
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Failed to load appeals'
    appeals.value = []
  } finally {
    loading.value = false
  }
}

// Open appeal detail
async function openAppeal(appeal: AppealDetail) {
  selectedAppeal.value = appeal
  reviewNotes.value = ''
  reviewError.value = ''
  reviewSuccess.value = ''

  // Fetch fresh detail from API
  detailLoading.value = true
  try {
    selectedAppeal.value = await api.getAppeal(appeal.id)
  } catch {
    // Keep the list version if detail fetch fails
  } finally {
    detailLoading.value = false
  }
}

function closeDetail() {
  selectedAppeal.value = null
}

// Submit review decision
async function submitReview() {
  if (!selectedAppeal.value) return
  reviewSubmitting.value = true
  reviewError.value = ''
  reviewSuccess.value = ''

  try {
    await api.reviewAppeal(selectedAppeal.value.id, {
      status: reviewStatus.value,
      reviewerNotes: reviewNotes.value
    })
    reviewSuccess.value = `${statusLabel(reviewStatus.value)} — решение сохранено`

    // Refresh the appeal detail
    selectedAppeal.value = await api.getAppeal(selectedAppeal.value.id)

    // Refresh the list
    await fetchAppeals()
  } catch (err: unknown) {
    reviewError.value = err instanceof Error ? err.message : 'Failed to submit review'
  } finally {
    reviewSubmitting.value = false
  }
}

// Stats
const appealStats = computed(() => {
  const all = appeals.value
  return {
    total: all.length,
    submitted: all.filter(a => a.status === 'submitted').length,
    under_review: all.filter(a => a.status === 'under_review').length,
    upheld: all.filter(a => a.status === 'upheld').length,
    overturned: all.filter(a => a.status === 'overturned').length,
    withdrawn: all.filter(a => a.status === 'withdrawn').length
  }
})

// Filtered appeals
const filteredAppeals = computed(() => {
  if (statusFilter.value === 'all') return appeals.value
  return appeals.value.filter(a => a.status === statusFilter.value)
})

onMounted(() => fetchAppeals())
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- Page Header -->
    <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1
          class="text-2xl font-bold"
          style="color: var(--argus-text);"
        >
          Апелляции
        </h1>
        <p
          class="text-sm mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Управление апелляциями студентов — {{ appealStats.total }} заявок
          <span
            v-if="loading"
            class="inline-flex items-center gap-1 ml-2 text-xs"
            style="color: var(--argus-accent);"
          >
            <span
              class="animate-spin inline-block size-3 border border-t-transparent rounded-full"
              style="border-color: var(--argus-accent); border-top-color: transparent;"
            />
            Загрузка...
          </span>
        </p>
      </div>

      <!-- Stats Badges -->
      <div class="flex items-center gap-2 flex-wrap">
        <div
          class="flex items-center gap-1.5 text-[10px] font-medium px-2.5 py-1 rounded-full"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
        >
          <UIcon
            name="i-lucide-inbox"
            class="size-3"
          />
          {{ appealStats.submitted }} новых
        </div>
        <div
          class="flex items-center gap-1.5 text-[10px] font-medium px-2.5 py-1 rounded-full"
          :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
        >
          <UIcon
            name="i-lucide-search"
            class="size-3"
          />
          {{ appealStats.under_review }} на рассмотрении
        </div>
        <div
          class="flex items-center gap-1.5 text-[10px] font-medium px-2.5 py-1 rounded-full"
          :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
        >
          <UIcon
            name="i-lucide-check-circle"
            class="size-3"
          />
          {{ appealStats.upheld }} удовлетворено
        </div>
        <div
          class="flex items-center gap-1.5 text-[10px] font-medium px-2.5 py-1 rounded-full"
          :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
        >
          <UIcon
            name="i-lucide-x-circle"
            class="size-3"
          />
          {{ appealStats.overturned }} отклонено
        </div>
      </div>
    </div>

    <!-- Status Filter -->
    <div class="glass-card rounded-xl px-4 py-3">
      <div class="flex items-center gap-2">
        <button
          v-for="filter in ['all', 'submitted', 'under_review', 'upheld', 'overturned', 'withdrawn']"
          :key="filter"
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all border"
          :style="{
            background: statusFilter === filter ? statusBgColor(filter === 'all' ? 'submitted' : filter, 0.1) : 'transparent',
            borderColor: statusFilter === filter ? statusBgColor(filter === 'all' ? 'submitted' : filter, 0.25) : 'var(--argus-border)',
            color: statusFilter === filter ? (filter === 'all' ? 'var(--argus-accent)' : statusColor(filter)) : 'var(--argus-text-dimmed)'
          }"
          @click="statusFilter = filter; fetchAppeals()"
        >
          <UIcon
            :name="filter === 'all' ? 'i-lucide-layout-grid' : statusIcon(filter)"
            class="size-3.5"
          />
          {{ filter === 'all' ? 'Все' : statusLabel(filter) }}
        </button>
      </div>
    </div>

    <!-- Error -->
    <div
      v-if="error"
      class="flex items-center gap-2 px-4 py-3 rounded-xl text-sm"
      :style="{ background: errorBg(0.08), color: 'var(--argus-error)' }"
    >
      <UIcon
        name="i-lucide-alert-circle"
        class="size-4 shrink-0"
      />
      {{ error }}
    </div>

    <!-- Appeals Table -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="px-5 py-3.5 border-b flex items-center justify-between"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-scale"
            class="size-4"
            style="color: var(--argus-text-dimmed);"
          />
          <h3
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Список апелляций
          </h3>
          <span
            class="text-[10px] font-medium px-2 py-0.5 rounded-full"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ filteredAppeals.length }}
          </span>
        </div>
        <button
          class="text-[10px] font-medium px-2 py-1 rounded transition-all"
          :style="{ color: 'var(--argus-accent)' }"
          @click="fetchAppeals"
        >
          Обновить
        </button>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                ID
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Студент
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Сессия
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Причина
              </th>
              <th
                class="px-5 py-3 text-center text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Статус
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Дата
              </th>
              <th
                class="px-5 py-3 text-right text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Действия
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="appeal in filteredAppeals"
              :key="appeal.id"
              class="transition-colors cursor-pointer"
              :style="{ borderBottom: '1px solid var(--argus-border-subtle)' }"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              @click="openAppeal(appeal)"
            >
              <td class="px-5 py-3.5">
                <span
                  class="text-[10px] font-mono"
                  style="color: var(--argus-text-muted);"
                >{{ appeal.id.slice(0, 12) }}...</span>
              </td>
              <td class="px-5 py-3.5">
                <span
                  class="text-xs font-medium"
                  style="color: var(--argus-text);"
                >{{ appeal.studentId }}</span>
              </td>
              <td class="px-5 py-3.5">
                <span
                  class="text-[10px] font-mono"
                  style="color: var(--argus-text-muted);"
                >{{ appeal.sessionId.slice(0, 12) }}...</span>
              </td>
              <td class="px-5 py-3.5">
                <p
                  class="text-xs truncate max-w-xs"
                  style="color: var(--argus-text-muted);"
                >
                  {{ appeal.reason }}
                </p>
              </td>
              <td class="px-5 py-3.5 text-center">
                <span
                  class="inline-flex items-center gap-1 text-[9px] font-bold px-2 py-1 rounded-full uppercase"
                  :style="{ background: statusBgColor(appeal.status, 0.1), color: statusColor(appeal.status) }"
                >
                  <UIcon
                    :name="statusIcon(appeal.status)"
                    class="size-3"
                  />
                  {{ statusLabel(appeal.status) }}
                </span>
              </td>
              <td class="px-5 py-3.5">
                <span
                  class="text-xs"
                  style="color: var(--argus-text-muted);"
                >{{ formatDate(appeal.createdAt) }}</span>
              </td>
              <td class="px-5 py-3.5 text-right">
                <button
                  class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all ml-auto"
                  :style="{ background: accentBg(0.08), color: 'var(--argus-accent)' }"
                  @click.stop="openAppeal(appeal)"
                >
                  <UIcon
                    name="i-lucide-eye"
                    class="size-3"
                  />
                  Подробнее
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Empty state -->
      <div
        v-if="!loading && filteredAppeals.length === 0"
        class="flex flex-col items-center justify-center py-16"
      >
        <UIcon
          name="i-lucide-inbox"
          class="size-12 mb-3"
          style="color: var(--argus-text-dimmed);"
        />
        <p
          class="text-sm font-medium"
          style="color: var(--argus-text);"
        >
          Нет апелляций
        </p>
        <p
          class="text-xs mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          {{ statusFilter === 'all' ? 'Апелляции ещё не были поданы' : 'Нет апелляций с данным статусом' }}
        </p>
      </div>
    </div>

    <!-- ===== APPEAL DETAIL MODAL ===== -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="selectedAppeal"
          class="fixed inset-0 z-[100] flex items-center justify-center p-4"
        >
          <!-- Backdrop -->
          <div
            class="absolute inset-0"
            :style="{ background: isDark ? 'rgba(0, 0, 0, 0.7)' : 'rgba(0, 0, 0, 0.4)' }"
            @click="closeDetail"
          />

          <!-- Modal -->
          <div
            class="relative w-full max-w-2xl max-h-[85vh] rounded-2xl border overflow-hidden flex flex-col z-10"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <!-- Header -->
            <div
              class="flex items-center justify-between px-6 py-4 border-b shrink-0"
              style="border-color: var(--argus-border);"
            >
              <div class="flex items-center gap-3">
                <span
                  class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full"
                  :style="{ background: statusBgColor(selectedAppeal.status, 0.1), color: statusColor(selectedAppeal.status) }"
                >
                  <UIcon
                    :name="statusIcon(selectedAppeal.status)"
                    class="size-3.5"
                  />
                  <span class="text-[10px] font-bold uppercase">{{ statusLabel(selectedAppeal.status) }}</span>
                </span>
                <div>
                  <h2
                    class="text-lg font-bold"
                    style="color: var(--argus-text);"
                  >
                    Апелляция
                  </h2>
                  <p
                    class="text-[10px] font-mono"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ selectedAppeal.id }}
                  </p>
                </div>
              </div>
              <button
                class="flex items-center justify-center size-8 rounded-lg transition-colors"
                style="color: var(--argus-text-dimmed);"
                @click="closeDetail"
              >
                <UIcon
                  name="i-lucide-x"
                  class="size-5"
                />
              </button>
            </div>

            <!-- Body -->
            <div class="flex-1 overflow-y-auto px-6 py-5 space-y-5">
              <!-- Loading -->
              <div
                v-if="detailLoading"
                class="flex items-center justify-center py-8"
              >
                <div
                  class="animate-spin rounded-full size-6 border-2 border-t-transparent"
                  style="border-color: var(--argus-accent); border-top-color: transparent;"
                />
              </div>

              <template v-else>
                <!-- Appeal Info Cards -->
                <div class="grid grid-cols-2 gap-3">
                  <div
                    class="rounded-lg border p-3"
                    :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
                  >
                    <span
                      class="text-[9px] font-semibold uppercase tracking-wider"
                      style="color: var(--argus-text-dimmed);"
                    >Студент</span>
                    <p
                      class="text-sm font-bold mt-1"
                      style="color: var(--argus-text);"
                    >
                      {{ selectedAppeal.studentId }}
                    </p>
                  </div>
                  <div
                    class="rounded-lg border p-3"
                    :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
                  >
                    <span
                      class="text-[9px] font-semibold uppercase tracking-wider"
                      style="color: var(--argus-text-dimmed);"
                    >Экзамен</span>
                    <p
                      class="text-sm font-bold mt-1"
                      style="color: var(--argus-text);"
                    >
                      {{ selectedAppeal.examId }}
                    </p>
                  </div>
                  <div
                    class="rounded-lg border p-3"
                    :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
                  >
                    <span
                      class="text-[9px] font-semibold uppercase tracking-wider"
                      style="color: var(--argus-text-dimmed);"
                    >Сессия</span>
                    <p
                      class="text-xs font-mono mt-1"
                      style="color: var(--argus-text-muted);"
                    >
                      {{ selectedAppeal.sessionId }}
                    </p>
                  </div>
                  <div
                    class="rounded-lg border p-3"
                    :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
                  >
                    <span
                      class="text-[9px] font-semibold uppercase tracking-wider"
                      style="color: var(--argus-text-dimmed);"
                    >Подана</span>
                    <p
                      class="text-xs font-medium mt-1"
                      style="color: var(--argus-text);"
                    >
                      {{ formatDateTime(selectedAppeal.createdAt) }}
                    </p>
                  </div>
                </div>

                <!-- Reason -->
                <div
                  class="rounded-lg border p-4"
                  :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
                >
                  <div class="flex items-center gap-2 mb-2">
                    <UIcon
                      name="i-lucide-message-square"
                      class="size-3.5"
                      style="color: var(--argus-accent);"
                    />
                    <span
                      class="text-[10px] font-semibold"
                      style="color: var(--argus-text-dimmed);"
                    >Причина апелляции</span>
                  </div>
                  <p
                    class="text-sm leading-relaxed"
                    style="color: var(--argus-text);"
                  >
                    {{ selectedAppeal.reason }}
                  </p>
                </div>

                <!-- Existing Review (if reviewed) -->
                <div
                  v-if="selectedAppeal.reviewedBy || selectedAppeal.reviewNotes"
                  class="rounded-lg border p-4"
                  :style="{
                    borderColor: statusBgColor(selectedAppeal.status, 0.2),
                    background: statusBgColor(selectedAppeal.status, 0.04)
                  }"
                >
                  <div class="flex items-center gap-2 mb-2">
                    <UIcon
                      name="i-lucide-clipboard-check"
                      class="size-3.5"
                      :style="{ color: statusColor(selectedAppeal.status) }"
                    />
                    <span
                      class="text-[10px] font-semibold"
                      :style="{ color: statusColor(selectedAppeal.status) }"
                    >Решение рецензента</span>
                  </div>
                  <div class="space-y-1.5">
                    <div
                      v-if="selectedAppeal.reviewedBy"
                      class="flex items-center gap-2"
                    >
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Рецензент:</span>
                      <span
                        class="text-[10px] font-medium"
                        style="color: var(--argus-text);"
                      >{{ selectedAppeal.reviewedBy }}</span>
                    </div>
                    <div
                      v-if="selectedAppeal.reviewedAt"
                      class="flex items-center gap-2"
                    >
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Дата:</span>
                      <span
                        class="text-[10px] font-medium"
                        style="color: var(--argus-text);"
                      >{{ formatDateTime(selectedAppeal.reviewedAt) }}</span>
                    </div>
                    <div v-if="selectedAppeal.reviewNotes">
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Комментарий:</span>
                      <p
                        class="text-xs mt-0.5 leading-relaxed"
                        style="color: var(--argus-text);"
                      >
                        {{ selectedAppeal.reviewNotes }}
                      </p>
                    </div>
                  </div>
                </div>

                <!-- Review Form (only for non-terminal states) -->
                <div
                  v-if="!isTerminal(selectedAppeal.status)"
                  class="rounded-lg border p-4 space-y-3"
                  :style="{ borderColor: accentBg(0.2), background: accentBg(0.03) }"
                >
                  <div class="flex items-center gap-2">
                    <UIcon
                      name="i-lucide-gavel"
                      class="size-3.5"
                      style="color: var(--argus-accent);"
                    />
                    <span
                      class="text-xs font-semibold"
                      style="color: var(--argus-text);"
                    >Вынести решение</span>
                  </div>

                  <!-- Decision selector -->
                  <div class="flex gap-2">
                    <button
                      v-for="option in (['under_review', 'upheld', 'overturned', 'withdrawn'] as const)"
                      :key="option"
                      class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2.5 rounded-lg text-[10px] font-bold transition-all border"
                      :style="{
                        background: reviewStatus === option ? statusBgColor(option, 0.1) : 'transparent',
                        borderColor: reviewStatus === option ? statusBgColor(option, 0.3) : 'var(--argus-border)',
                        color: reviewStatus === option ? statusColor(option) : 'var(--argus-text-dimmed)'
                      }"
                      @click="reviewStatus = option"
                    >
                      <UIcon
                        :name="statusIcon(option)"
                        class="size-3"
                      />
                      {{ statusLabel(option) }}
                    </button>
                  </div>

                  <!-- Notes -->
                  <textarea
                    v-model="reviewNotes"
                    rows="3"
                    placeholder="Комментарий рецензента..."
                    class="w-full px-3 py-2 rounded-lg border text-xs resize-none outline-none transition-colors"
                    :style="{
                      background: 'var(--argus-bg-elevated)',
                      borderColor: 'var(--argus-border)',
                      color: 'var(--argus-text)'
                    }"
                  />

                  <!-- Submit -->
                  <div class="flex items-center gap-3">
                    <button
                      class="flex items-center gap-1.5 px-4 py-2.5 rounded-lg text-xs font-bold transition-all"
                      :style="{
                        background: statusBgColor(reviewStatus, 0.15),
                        color: statusColor(reviewStatus),
                        opacity: reviewSubmitting ? 0.6 : 1
                      }"
                      :disabled="reviewSubmitting"
                      @click="submitReview"
                    >
                      <div
                        v-if="reviewSubmitting"
                        class="animate-spin rounded-full size-3 border border-t-transparent"
                        :style="{ borderColor: statusColor(reviewStatus), borderTopColor: 'transparent' }"
                      />
                      <UIcon
                        v-else
                        name="i-lucide-send"
                        class="size-3.5"
                      />
                      {{ reviewSubmitting ? 'Сохранение...' : 'Сохранить решение' }}
                    </button>
                  </div>

                  <!-- Error / Success -->
                  <div
                    v-if="reviewError"
                    class="flex items-center gap-2 px-3 py-2 rounded-lg text-[10px]"
                    :style="{ background: errorBg(0.08), color: 'var(--argus-error)' }"
                  >
                    <UIcon
                      name="i-lucide-alert-circle"
                      class="size-3 shrink-0"
                    />
                    {{ reviewError }}
                  </div>
                  <div
                    v-if="reviewSuccess"
                    class="flex items-center gap-2 px-3 py-2 rounded-lg text-[10px]"
                    :style="{ background: successBg(0.08), color: 'var(--argus-success)' }"
                  >
                    <UIcon
                      name="i-lucide-check-circle"
                      class="size-3 shrink-0"
                    />
                    {{ reviewSuccess }}
                  </div>
                </div>

                <!-- Terminal state info -->
                <div
                  v-else
                  class="flex items-center gap-2 px-4 py-3 rounded-lg border"
                  :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
                >
                  <UIcon
                    name="i-lucide-lock"
                    class="size-3.5"
                    style="color: var(--argus-text-dimmed);"
                  />
                  <span
                    class="text-[10px] font-medium"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Решение по апелляции является окончательным и не может быть изменено
                  </span>
                </div>
              </template>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.modal-enter-active { transition: opacity 0.2s ease; }
.modal-leave-active { transition: opacity 0.15s ease; }
.modal-enter-from, .modal-leave-to { opacity: 0; }
</style>
