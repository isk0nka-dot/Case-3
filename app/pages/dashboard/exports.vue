<script setup lang="ts">
import { useAuthStore } from '~/stores/useAuthStore'

const authStore = useAuthStore()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()
const { formatDateTime, formatTimeAgo, formatFileSize } = useFormatters()

// --- Types ---
interface ExportJob {
  id: string
  orgId: string
  requestedBy: string
  sessionIds: string[]
  status: 'pending' | 'processing' | 'completed' | 'failed' | 'expired'
  archiveUri?: string
  manifestUri?: string
  sha256Archive?: string
  presignTtlSec: number
  downloadUrl?: string
  errorMessage?: string
  createdAt: string
  completedAt?: string
  expiresAt?: string
}

// --- State ---
const exports = ref<ExportJob[]>([])
const loading = ref(false)
const creating = ref(false)
const error = ref('')

// --- Create Export Modal ---
const showCreateModal = ref(false)
const newExportSessionIds = ref('')

// --- Helpers (delegated to useStatusHelpers composable) ---
const { exportStatusLabel: statusLabel, exportStatusColor: statusColor, exportStatusIcon: statusIcon, exportStatusBg: statusBgColor } = useStatusHelpers()

// formatDate/formatRelative/formatFileSize → replaced by useFormatters() composable

// --- Stats ---
const stats = computed(() => {
  const total = exports.value.length
  const completed = exports.value.filter(e => e.status === 'completed').length
  const processing = exports.value.filter(e => e.status === 'processing' || e.status === 'pending').length
  const failed = exports.value.filter(e => e.status === 'failed').length
  return { total, completed, processing, failed }
})

// --- API calls ---
async function fetchExports() {
  loading.value = true
  error.value = ''
  try {
    const { useAdminAPI } = await import('~/composables/useAdminAPI')
    const api = useAdminAPI()
    const data = await api.listExportJobs()
    exports.value = data
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Failed to fetch exports'
    // Use demo data for development
    exports.value = getDemoExports()
  } finally {
    loading.value = false
  }
}

async function createExport() {
  const sessionIds = newExportSessionIds.value
    .split(/[\n,]+/)
    .map(s => s.trim())
    .filter(Boolean)

  if (sessionIds.length === 0) {
    error.value = 'Укажите хотя бы один ID сессии'
    return
  }

  creating.value = true
  error.value = ''
  try {
    const { useAdminAPI } = await import('~/composables/useAdminAPI')
    const api = useAdminAPI()
    await api.createExportJob(sessionIds)
    showCreateModal.value = false
    newExportSessionIds.value = ''
    await fetchExports()
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Failed to create export'
  } finally {
    creating.value = false
  }
}

async function cancelExport(exportId: string) {
  try {
    const { useAdminAPI } = await import('~/composables/useAdminAPI')
    const api = useAdminAPI()
    await api.cancelExportJob(exportId)
    await fetchExports()
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : 'Failed to cancel export'
  }
}

function getDemoExports(): ExportJob[] {
  return [
    {
      id: 'exp-001',
      orgId: 'org-1',
      requestedBy: 'admin@example.com',
      sessionIds: ['sess-101', 'sess-102', 'sess-103'],
      status: 'completed',
      sha256Archive: 'a1b2c3d4e5f6...',
      presignTtlSec: 86400,
      downloadUrl: '#',
      createdAt: new Date(Date.now() - 3600000).toISOString(),
      completedAt: new Date(Date.now() - 3000000).toISOString(),
      expiresAt: new Date(Date.now() + 83400000).toISOString()
    },
    {
      id: 'exp-002',
      orgId: 'org-1',
      requestedBy: 'admin@example.com',
      sessionIds: ['sess-201', 'sess-202'],
      status: 'processing',
      presignTtlSec: 86400,
      createdAt: new Date(Date.now() - 300000).toISOString()
    },
    {
      id: 'exp-003',
      orgId: 'org-1',
      requestedBy: 'admin@example.com',
      sessionIds: ['sess-301'],
      status: 'failed',
      errorMessage: 'Evidence fragments not found for session sess-301',
      presignTtlSec: 86400,
      createdAt: new Date(Date.now() - 7200000).toISOString()
    },
    {
      id: 'exp-004',
      orgId: 'org-1',
      requestedBy: 'admin@example.com',
      sessionIds: ['sess-401', 'sess-402', 'sess-403', 'sess-404', 'sess-405'],
      status: 'pending',
      presignTtlSec: 86400,
      createdAt: new Date(Date.now() - 120000).toISOString()
    }
  ]
}

// --- Polling for status updates ---
let pollTimer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  fetchExports()
  // Poll every 10s for status updates (processing jobs)
  pollTimer = setInterval(() => {
    const hasActive = exports.value.some(e => e.status === 'pending' || e.status === 'processing')
    if (hasActive) fetchExports()
  }, 10000)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
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
          Экспорт доказательств
        </h1>
        <p
          class="text-sm mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Массовый экспорт видеозаписей и манифестов с криптографической верификацией
        </p>
      </div>

      <!-- Create Export Button -->
      <button
        class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-sm font-semibold transition-all"
        :style="{
          background: accentBg(0.15),
          color: 'var(--argus-accent)',
          border: `1px solid ${accentBg(0.3)}`
        }"
        @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.25)"
        @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.15)"
        @click="showCreateModal = true"
      >
        <UIcon
          name="i-lucide-package-plus"
          class="size-4"
        />
        Создать экспорт
      </button>
    </div>

    <!-- Stats Badges -->
    <div class="flex items-center gap-3 flex-wrap">
      <div
        class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
        :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
      >
        <UIcon
          name="i-lucide-package"
          class="size-3.5"
        />
        {{ stats.total }} всего
      </div>
      <div
        class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
        :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
      >
        <UIcon
          name="i-lucide-check-circle"
          class="size-3.5"
        />
        {{ stats.completed }} готово
      </div>
      <div
        v-if="stats.processing > 0"
        class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
        :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
      >
        <UIcon
          name="i-lucide-loader"
          class="size-3.5 animate-spin"
        />
        {{ stats.processing }} в процессе
      </div>
      <div
        v-if="stats.failed > 0"
        class="flex items-center gap-2 text-xs font-medium px-3 py-1.5 rounded-full"
        :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
      >
        <UIcon
          name="i-lucide-x-circle"
          class="size-3.5"
        />
        {{ stats.failed }} ошибок
      </div>
    </div>

    <!-- Error Banner -->
    <div
      v-if="error"
      class="flex items-center gap-3 px-4 py-3 rounded-xl border"
      :style="{ background: errorBg(0.06), borderColor: errorBg(0.2), color: 'var(--argus-error)' }"
    >
      <UIcon
        name="i-lucide-alert-triangle"
        class="size-4 shrink-0"
      />
      <span class="text-sm">{{ error }}</span>
      <button
        class="ml-auto"
        @click="error = ''"
      >
        <UIcon
          name="i-lucide-x"
          class="size-4"
        />
      </button>
    </div>

    <!-- Export Jobs Table -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="px-5 py-3.5 border-b flex items-center justify-between"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-hard-drive-download"
            class="size-4"
            style="color: var(--argus-text-dimmed);"
          />
          <h3
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Задания экспорта
          </h3>
        </div>
        <button
          class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all"
          :style="{ color: 'var(--argus-text-dimmed)' }"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
          @click="fetchExports"
        >
          <UIcon
            name="i-lucide-refresh-cw"
            class="size-3.5"
            :class="loading ? 'animate-spin' : ''"
          />
          Обновить
        </button>
      </div>

      <!-- Loading -->
      <div
        v-if="loading && exports.length === 0"
        class="flex flex-col items-center justify-center py-16"
      >
        <div
          class="animate-spin rounded-full size-8 border-2 border-t-transparent mb-3"
          style="border-color: var(--argus-accent); border-top-color: transparent;"
        />
        <p
          class="text-xs font-medium"
          style="color: var(--argus-text-dimmed);"
        >
          Загрузка экспортов...
        </p>
      </div>

      <!-- Table -->
      <div
        v-else-if="exports.length > 0"
        class="overflow-x-auto"
      >
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
                Сессии
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
                Создан
              </th>
              <th
                class="px-5 py-3 text-left text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                SHA-256
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
              v-for="job in exports"
              :key="job.id"
              :style="{ borderBottom: '1px solid var(--argus-border-subtle)' }"
            >
              <!-- ID -->
              <td class="px-5 py-3.5">
                <p
                  class="text-xs font-mono font-medium"
                  style="color: var(--argus-text);"
                >
                  {{ job.id }}
                </p>
                <p
                  class="text-[10px] mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ job.requestedBy }}
                </p>
              </td>

              <!-- Sessions -->
              <td class="px-5 py-3.5">
                <div class="flex items-center gap-1.5">
                  <span
                    class="text-xs font-bold"
                    style="color: var(--argus-text);"
                  >{{ job.sessionIds.length }}</span>
                  <span
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ job.sessionIds.length === 1 ? 'сессия' : job.sessionIds.length < 5 ? 'сессии' : 'сессий' }}
                  </span>
                </div>
                <p
                  class="text-[9px] font-mono mt-0.5 truncate max-w-48"
                  style="color: var(--argus-text-muted);"
                >
                  {{ job.sessionIds.slice(0, 3).join(', ') }}{{ job.sessionIds.length > 3 ? '...' : '' }}
                </p>
              </td>

              <!-- Status -->
              <td class="px-5 py-3.5 text-center">
                <span
                  class="inline-flex items-center gap-1 text-[10px] font-semibold px-2 py-1 rounded-full"
                  :style="{ background: statusBgColor(job.status, 0.1), color: statusColor(job.status) }"
                >
                  <UIcon
                    :name="statusIcon(job.status)"
                    class="size-3"
                    :class="job.status === 'processing' ? 'animate-spin' : ''"
                  />
                  {{ statusLabel(job.status) }}
                </span>
                <p
                  v-if="job.errorMessage"
                  class="text-[9px] mt-1 max-w-48 truncate"
                  style="color: var(--argus-error);"
                >
                  {{ job.errorMessage }}
                </p>
              </td>

              <!-- Created -->
              <td class="px-5 py-3.5">
                <p
                  class="text-xs"
                  style="color: var(--argus-text-muted);"
                >
                  {{ formatTimeAgo(job.createdAt) }}
                </p>
                <p
                  class="text-[10px] mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ formatDateTime(job.createdAt) }}
                </p>
              </td>

              <!-- SHA-256 -->
              <td class="px-5 py-3.5">
                <p
                  v-if="job.sha256Archive"
                  class="text-[10px] font-mono truncate max-w-32"
                  style="color: var(--argus-text-muted);"
                >
                  {{ job.sha256Archive }}
                </p>
                <span
                  v-else
                  class="text-[10px]"
                  style="color: var(--argus-text-dimmed);"
                >—</span>
              </td>

              <!-- Actions -->
              <td class="px-5 py-3.5 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <!-- Download -->
                  <button
                    v-if="job.status === 'completed' && job.downloadUrl"
                    class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all"
                    :style="{ background: successBg(0.08), color: 'var(--argus-success)' }"
                    @mouseenter="($event.currentTarget as HTMLElement).style.background = successBg(0.15)"
                    @mouseleave="($event.currentTarget as HTMLElement).style.background = successBg(0.08)"
                  >
                    <UIcon
                      name="i-lucide-download"
                      class="size-3"
                    />
                    Скачать
                  </button>

                  <!-- Cancel (only pending) -->
                  <button
                    v-if="job.status === 'pending'"
                    class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all"
                    :style="{ background: errorBg(0.08), color: 'var(--argus-error)' }"
                    @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.15)"
                    @mouseleave="($event.currentTarget as HTMLElement).style.background = errorBg(0.08)"
                    @click="cancelExport(job.id)"
                  >
                    <UIcon
                      name="i-lucide-x"
                      class="size-3"
                    />
                    Отменить
                  </button>

                  <!-- Expiry info -->
                  <span
                    v-if="job.status === 'completed' && job.expiresAt"
                    class="text-[9px] px-2 py-1 rounded-md"
                    :style="{ background: 'var(--argus-bg-elevated)', color: 'var(--argus-text-dimmed)' }"
                  >
                    Истекает: {{ formatTimeAgo(job.expiresAt) }}
                  </span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Empty State -->
      <div
        v-else
        class="flex flex-col items-center justify-center py-16"
      >
        <UIcon
          name="i-lucide-package-x"
          class="size-12 mb-3"
          style="color: var(--argus-text-dimmed);"
        />
        <p
          class="text-sm font-medium"
          style="color: var(--argus-text);"
        >
          Нет экспортов
        </p>
        <p
          class="text-xs mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Создайте экспорт для скачивания доказательств с криптографическим манифестом
        </p>
        <button
          class="mt-4 flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-medium transition-all"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          @click="showCreateModal = true"
        >
          <UIcon
            name="i-lucide-package-plus"
            class="size-3.5"
          />
          Создать экспорт
        </button>
      </div>
    </div>

    <!-- Info Card: What's in an export -->
    <div class="glass-card rounded-xl p-5">
      <div class="flex items-start gap-3">
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: accentBg(0.1) }"
        >
          <UIcon
            name="i-lucide-shield-check"
            class="size-5"
            style="color: var(--argus-accent);"
          />
        </div>
        <div>
          <h4
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Содержимое архива
          </h4>
          <p
            class="text-xs mt-1 leading-relaxed"
            style="color: var(--argus-text-dimmed);"
          >
            Каждый экспорт содержит TAR.GZ архив с видеозаписями, аудиозаписями, снимками экрана
            и криптографический манифест (manifest.json) с SHA-256 хэшами каждого файла
            и привязкой нарушений к временным меткам видео. Архив защищён Object Lock (WORM)
            для обеспечения неизменности доказательной базы.
          </p>
          <div class="flex items-center gap-4 mt-3">
            <div
              class="flex items-center gap-1.5 text-[10px]"
              style="color: var(--argus-text-muted);"
            >
              <UIcon
                name="i-lucide-file-archive"
                class="size-3"
              />
              TAR.GZ архив
            </div>
            <div
              class="flex items-center gap-1.5 text-[10px]"
              style="color: var(--argus-text-muted);"
            >
              <UIcon
                name="i-lucide-fingerprint"
                class="size-3"
              />
              SHA-256 верификация
            </div>
            <div
              class="flex items-center gap-1.5 text-[10px]"
              style="color: var(--argus-text-muted);"
            >
              <UIcon
                name="i-lucide-lock"
                class="size-3"
              />
              Object Lock (WORM)
            </div>
            <div
              class="flex items-center gap-1.5 text-[10px]"
              style="color: var(--argus-text-muted);"
            >
              <UIcon
                name="i-lucide-clock"
                class="size-3"
              />
              Ссылка на 24 часа
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ===== CREATE EXPORT MODAL ===== -->
    <Teleport to="body">
      <Transition name="modal">
        <div
          v-if="showCreateModal"
          class="fixed inset-0 z-[100] flex items-center justify-center p-4"
        >
          <!-- Backdrop -->
          <div
            class="absolute inset-0"
            :style="{ background: isDark ? 'rgba(0, 0, 0, 0.7)' : 'rgba(0, 0, 0, 0.4)' }"
            @click="showCreateModal = false"
          />

          <!-- Modal -->
          <div
            class="relative w-full max-w-lg rounded-2xl border overflow-hidden z-10"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <!-- Header -->
            <div
              class="flex items-center justify-between px-6 py-4 border-b"
              style="border-color: var(--argus-border);"
            >
              <div class="flex items-center gap-2">
                <UIcon
                  name="i-lucide-package-plus"
                  class="size-5"
                  style="color: var(--argus-accent);"
                />
                <h2
                  class="text-lg font-bold"
                  style="color: var(--argus-text);"
                >
                  Новый экспорт
                </h2>
              </div>
              <button
                class="flex items-center justify-center size-8 rounded-lg transition-colors"
                style="color: var(--argus-text-dimmed);"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                @click="showCreateModal = false"
              >
                <UIcon
                  name="i-lucide-x"
                  class="size-5"
                />
              </button>
            </div>

            <!-- Body -->
            <div class="px-6 py-5 space-y-4">
              <div>
                <label
                  class="text-xs font-semibold mb-2 block"
                  style="color: var(--argus-text-muted);"
                >
                  ID сессий (по одному на строку или через запятую)
                </label>
                <textarea
                  v-model="newExportSessionIds"
                  rows="5"
                  class="w-full px-3 py-2 rounded-lg border text-sm resize-none outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    borderColor: 'var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                  placeholder="sess-101&#10;sess-102&#10;sess-103"
                  @focus="($event.target as HTMLElement).style.borderColor = 'var(--argus-accent)'"
                  @blur="($event.target as HTMLElement).style.borderColor = 'var(--argus-border)'"
                />
                <p
                  class="text-[10px] mt-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Максимум 50 сессий на один экспорт. Архив будет готов в течение нескольких минут.
                </p>
              </div>
            </div>

            <!-- Footer -->
            <div
              class="flex items-center justify-end gap-3 px-6 py-4 border-t"
              style="border-color: var(--argus-border);"
            >
              <button
                class="px-4 py-2 rounded-lg text-sm font-medium transition-all"
                :style="{ color: 'var(--argus-text-dimmed)' }"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
                @click="showCreateModal = false"
              >
                Отмена
              </button>
              <button
                class="flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-semibold transition-all"
                :disabled="creating"
                :style="{
                  background: accentBg(0.15),
                  color: 'var(--argus-accent)',
                  opacity: creating ? 0.6 : 1
                }"
                @mouseenter="!creating && (($event.currentTarget as HTMLElement).style.background = accentBg(0.25))"
                @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.15)"
                @click="createExport"
              >
                <UIcon
                  v-if="creating"
                  name="i-lucide-loader"
                  class="size-4 animate-spin"
                />
                <UIcon
                  v-else
                  name="i-lucide-package-plus"
                  class="size-4"
                />
                {{ creating ? 'Создание...' : 'Создать экспорт' }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>
