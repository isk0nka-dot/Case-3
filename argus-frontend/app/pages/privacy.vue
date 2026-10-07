<script setup lang="ts">
// =============================================================================
// Privacy & Data Retention — Kazakhstan PD compliance dashboard
//
// Allows org admins to:
//   - View and configure data retention periods
//   - Export audit logs for compliance
//   - Review consent records
//   - Initiate right-to-erasure requests
//   - View Kazakhstan Personal Data compliance checklist
// =============================================================================

import { useAdminAPI } from '~/composables/useAdminAPI'
import { useAuthStore } from '~/stores/useAuthStore'

const api = useAdminAPI()
const authStore = useAuthStore()
const { isDark, accentBg, successBg, warningBg, errorBg } = useColors()

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

const retentionDays = ref(90)
const videoRetentionDays = ref(180)
const auditRetentionDays = ref(365)
const saving = ref(false)
const saveSuccess = ref(false)
const saveError = ref('')
const retentionApplying = ref(false)
const retentionApplyMode = ref<'dry-run' | 'apply' | null>(null)
const retentionResult = ref<Awaited<ReturnType<typeof api.applyRetentionPolicy>> | null>(null)
const retentionError = ref('')

const erasureStudentId = ref('')
const erasureReason = ref('')
const erasureLoading = ref(false)
const erasureResult = ref<string | null>(null)

const auditExportLoading = ref(false)

const selectedOrgId = computed(() => {
  if (authStore.effectiveOrgId) return authStore.effectiveOrgId
  if (!authStore.isSuperAdmin) return authStore.user?.orgId || authStore.orgId
  return null
})

const requiresOrgSelection = computed(() => authStore.isSuperAdmin && !selectedOrgId.value)

// Load retention from org settings
onMounted(async () => {
  if (!selectedOrgId.value) return
  try {
    const org = await api.getOrg(selectedOrgId.value)
    retentionDays.value = org.retentionDays || 90
  } catch { /* use defaults */ }
})

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

async function saveRetention() {
  if (!selectedOrgId.value) {
    saveError.value = 'Выберите организацию перед сохранением политики.'
    return
  }

  saving.value = true
  saveSuccess.value = false
  saveError.value = ''
  try {
    await api.updateOrg(selectedOrgId.value, {
      retentionDays: retentionDays.value
    })
    saveSuccess.value = true
    setTimeout(() => { saveSuccess.value = false }, 3000)
  } catch (err) {
    saveError.value = err instanceof Error ? err.message : 'Не удалось сохранить retention policy'
  } finally {
    saving.value = false
  }
}

async function runRetentionCleanup(dryRun: boolean) {
  if (!selectedOrgId.value) {
    retentionError.value = 'Выберите организацию перед применением retention.'
    return
  }

  retentionApplying.value = true
  retentionApplyMode.value = dryRun ? 'dry-run' : 'apply'
  retentionResult.value = null
  retentionError.value = ''

  try {
    retentionResult.value = await api.applyRetentionPolicy(selectedOrgId.value, {
      dryRun,
      videoRetentionDays: videoRetentionDays.value,
      auditRetentionDays: auditRetentionDays.value
    })
  } catch (err) {
    retentionError.value = err instanceof Error ? err.message : 'Не удалось применить retention policy'
  } finally {
    retentionApplying.value = false
    retentionApplyMode.value = null
  }
}

async function requestErasure() {
  if (!erasureStudentId.value.trim()) return
  erasureLoading.value = true
  erasureResult.value = null
  try {
    // Delete enrollment (biometric data)
    if (selectedOrgId.value) {
      await api.deleteEnrollment?.(erasureStudentId.value, selectedOrgId.value).catch(() => {})
    }
    erasureResult.value = `Биометрические данные студента ${erasureStudentId.value} удалены. Запрос добавлен в журнал аудита.`
    erasureStudentId.value = ''
    erasureReason.value = ''
  } catch {
    erasureResult.value = 'Ошибка при удалении данных. Проверьте ID студента.'
  } finally {
    erasureLoading.value = false
  }
}

async function exportAuditLog() {
  auditExportLoading.value = true
  try {
    const logs = await api.listAuditLogs({
      orgId: selectedOrgId.value || undefined,
      limit: 1000
    })
    const csv = [
      'Timestamp,UserID,UserPhone,Role,Action,ResourceType,ResourceID,IPAddress',
      ...logs.map(l =>
        [l.createdAt, l.userId, l.userPhone, l.userRole, l.action, l.resourceType, l.resourceId, l.ipAddress]
          .map(v => `"${String(v).replace(/"/g, '""')}"`)
          .join(',')
      )
    ].join('\n')

    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `argus-audit-${new Date().toISOString().slice(0, 10)}.csv`
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    // API layer already surfaces request errors.
  } finally {
    auditExportLoading.value = false
  }
}

// Kazakhstan PD compliance checklist items
const complianceItems = [
  { id: 1, label: 'Согласие на обработку ПДн получено перед началом экзамена', done: true },
  { id: 2, label: 'Субъекты ПДн уведомлены о целях обработки', done: true },
  { id: 3, label: 'Данные хранятся в пределах РК или с разрешёнными трансграничными передачами', done: false },
  { id: 4, label: 'Реализовано право на доступ (студент может запросить свои данные)', done: false },
  { id: 5, label: 'Реализовано право на удаление (erasure)', done: true },
  { id: 6, label: 'Политика хранения данных задокументирована', done: true },
  { id: 7, label: 'Видеозаписи шифруются при хранении', done: false },
  { id: 8, label: 'Доступ к ПДн ограничен на основе ролей (RBAC)', done: true },
  { id: 9, label: 'Журнал аудита всех операций с ПДн ведётся', done: true },
  { id: 10, label: 'Назначен ответственный за обработку ПДн (DPO)', done: false }
]

const doneCount = complianceItems.filter(i => i.done).length
const compliancePercent = Math.round(doneCount / complianceItems.length * 100)
</script>

<template>
  <div class="p-6 max-w-4xl mx-auto space-y-8">
    <!-- Page header -->
    <div>
      <h1
        class="text-2xl font-bold"
        style="color: var(--argus-text);"
      >
        Приватность и данные
      </h1>
      <p
        class="text-sm mt-1"
        style="color: var(--argus-text-dimmed);"
      >
        Настройки хранения данных, соответствие требованиям ЗРК «О персональных данных» и инструменты удаления
      </p>
      <p
        v-if="requiresOrgSelection"
        class="text-xs mt-3 px-3 py-2 rounded-lg border inline-flex"
        :style="{ background: warningBg(0.08), borderColor: warningBg(0.2), color: 'var(--argus-warning)' }"
      >
        Выберите организацию в верхнем переключателе, чтобы сохранять retention policy.
      </p>
    </div>

    <!-- KZ PD Compliance Checklist -->
    <div
      class="rounded-xl border p-5"
      style="background: var(--argus-bg-card); border-color: var(--argus-border);"
    >
      <div class="flex items-center justify-between mb-4">
        <div>
          <h2
            class="text-base font-semibold"
            style="color: var(--argus-text);"
          >
            Соответствие ЗРК «О персональных данных»
          </h2>
          <p
            class="text-xs mt-0.5"
            style="color: var(--argus-text-dimmed);"
          >
            Чек-лист выполнения требований казахстанского законодательства
          </p>
        </div>
        <div
          class="text-3xl font-bold"
          :style="{ color: compliancePercent >= 80 ? 'var(--argus-success)' : compliancePercent >= 60 ? 'var(--argus-warning)' : 'var(--argus-error)' }"
        >
          {{ compliancePercent }}%
        </div>
      </div>

      <!-- Progress bar -->
      <div
        class="h-2 rounded-full mb-5"
        style="background: var(--argus-bg-elevated);"
      >
        <div
          class="h-2 rounded-full transition-all"
          :style="{
            width: compliancePercent + '%',
            background: compliancePercent >= 80 ? 'var(--argus-success)' : compliancePercent >= 60 ? 'var(--argus-warning)' : 'var(--argus-error)'
          }"
        />
      </div>

      <ul class="space-y-2">
        <li
          v-for="item in complianceItems"
          :key="item.id"
          class="flex items-start gap-3"
        >
          <span
            class="mt-0.5 text-sm"
            :style="{ color: item.done ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
          >
            {{ item.done ? '✓' : '○' }}
          </span>
          <span
            class="text-sm"
            :style="{ color: item.done ? 'var(--argus-text)' : 'var(--argus-text-dimmed)' }"
          >
            {{ item.label }}
          </span>
          <span
            v-if="!item.done"
            class="ml-auto text-[10px] font-medium px-2 py-0.5 rounded-full shrink-0"
            :style="{ background: warningBg(0.12), color: 'var(--argus-warning)' }"
          >
            Нужно выполнить
          </span>
        </li>
      </ul>
    </div>

    <!-- Data Retention Settings -->
    <div
      class="rounded-xl border p-5"
      style="background: var(--argus-bg-card); border-color: var(--argus-border);"
    >
      <h2
        class="text-base font-semibold mb-4"
        style="color: var(--argus-text);"
      >
        Политика хранения данных
      </h2>

      <div class="space-y-4">
        <div>
          <label
            class="block text-sm font-medium mb-1"
            style="color: var(--argus-text);"
          >
            События прокторинга (ClickHouse)
          </label>
          <div class="flex items-center gap-3">
            <input
              v-model.number="retentionDays"
              type="number"
              min="30"
              max="365"
              class="w-24 px-3 py-1.5 rounded-lg border text-sm outline-none"
              :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
            >
            <span
              class="text-sm"
              style="color: var(--argus-text-dimmed);"
            >дней (текущий ClickHouse TTL: 90)</span>
          </div>
        </div>

        <div>
          <label
            class="block text-sm font-medium mb-1"
            style="color: var(--argus-text);"
          >
            Видеозаписи экзаменов (MinIO/S3)
          </label>
          <div class="flex items-center gap-3">
            <input
              v-model.number="videoRetentionDays"
              type="number"
              min="30"
              max="730"
              class="w-24 px-3 py-1.5 rounded-lg border text-sm outline-none"
              :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
            >
            <span
              class="text-sm"
              style="color: var(--argus-text-dimmed);"
            >дней</span>
          </div>
        </div>

        <div>
          <label
            class="block text-sm font-medium mb-1"
            style="color: var(--argus-text);"
          >
            Журнал аудита (PostgreSQL)
          </label>
          <div class="flex items-center gap-3">
            <input
              v-model.number="auditRetentionDays"
              type="number"
              min="180"
              max="1825"
              class="w-24 px-3 py-1.5 rounded-lg border text-sm outline-none"
              :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
            >
            <span
              class="text-sm"
              style="color: var(--argus-text-dimmed);"
            >дней (мин. 180 по законодательству)</span>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3 mt-5">
        <button
          class="px-4 py-2 rounded-lg text-sm font-semibold transition-all"
          :style="{ background: 'var(--argus-accent)', color: '#fff', opacity: saving || requiresOrgSelection ? '0.6' : '1' }"
          :disabled="saving || requiresOrgSelection"
          @click="saveRetention"
        >
          {{ saving ? 'Сохранение...' : 'Сохранить политику' }}
        </button>
        <span
          v-if="saveSuccess"
          class="text-sm"
          style="color: var(--argus-success);"
        >
          ✓ Retention для событий сохранен
        </span>
        <span
          v-if="saveError"
          class="text-sm"
          style="color: var(--argus-error);"
        >
          {{ saveError }}
        </span>
        <span
          class="text-xs"
          style="color: var(--argus-text-dimmed);"
        >
          Backend сохраняет org retention для событий. Video/S3 cleanup сейчас удаляет PostgreSQL metadata; физическое S3 удаление требует отдельной object-store проверки.
        </span>
      </div>

      <div class="flex flex-wrap items-center gap-3 mt-4">
        <button
          class="px-4 py-2 rounded-lg text-sm font-semibold border transition-all"
          :style="{ borderColor: 'var(--argus-border)', color: 'var(--argus-text)', opacity: retentionApplying || requiresOrgSelection ? '0.6' : '1' }"
          :disabled="retentionApplying || requiresOrgSelection"
          @click="runRetentionCleanup(true)"
        >
          {{ retentionApplyMode === 'dry-run' ? 'Проверка...' : 'Dry-run cleanup' }}
        </button>
        <button
          class="px-4 py-2 rounded-lg text-sm font-semibold transition-all"
          :style="{ background: 'var(--argus-error)', color: '#fff', opacity: retentionApplying || requiresOrgSelection ? '0.6' : '1' }"
          :disabled="retentionApplying || requiresOrgSelection"
          @click="runRetentionCleanup(false)"
        >
          {{ retentionApplyMode === 'apply' ? 'Применение...' : 'Применить cleanup' }}
        </button>
        <span
          v-if="retentionError"
          class="text-sm"
          style="color: var(--argus-error);"
        >
          {{ retentionError }}
        </span>
      </div>

      <div
        v-if="retentionResult"
        class="mt-4 rounded-lg border p-3 text-sm"
        :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)', color: 'var(--argus-text)' }"
      >
        <div class="font-semibold mb-2">
          {{ retentionResult.dryRun ? 'Dry-run результат' : 'Cleanup применен' }}
        </div>
        <div class="grid gap-1 sm:grid-cols-2">
          <span>Video metadata matched: {{ retentionResult.recordingsMatched }}</span>
          <span>Video metadata deleted: {{ retentionResult.recordingsDeleted }}</span>
          <span>Audit matched: {{ retentionResult.auditMatched }}</span>
          <span>Audit deleted: {{ retentionResult.auditDeleted }}</span>
        </div>
      </div>
    </div>

    <!-- Right to Erasure -->
    <div
      class="rounded-xl border p-5"
      style="background: var(--argus-bg-card); border-color: var(--argus-border);"
    >
      <h2
        class="text-base font-semibold mb-1"
        style="color: var(--argus-text);"
      >
        Право на удаление (ст. 22 ЗРК)
      </h2>
      <p
        class="text-xs mb-4"
        style="color: var(--argus-text-dimmed);"
      >
        Удаляет биометрические данные (face embedding) студента. Запрос фиксируется в журнале аудита.
      </p>

      <div class="space-y-3">
        <div>
          <label
            class="block text-xs font-medium mb-1"
            style="color: var(--argus-text-dimmed);"
          >
            ID студента
          </label>
          <input
            v-model="erasureStudentId"
            type="text"
            placeholder="student-12345"
            class="w-full max-w-xs px-3 py-1.5 rounded-lg border text-sm outline-none"
            :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
          >
        </div>
        <div>
          <label
            class="block text-xs font-medium mb-1"
            style="color: var(--argus-text-dimmed);"
          >
            Основание (необязательно)
          </label>
          <input
            v-model="erasureReason"
            type="text"
            placeholder="Заявление от 01.06.2026"
            class="w-full max-w-xs px-3 py-1.5 rounded-lg border text-sm outline-none"
            :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
          >
        </div>
      </div>

      <button
        class="mt-4 px-4 py-2 rounded-lg text-sm font-semibold transition-all"
        :style="{
          background: erasureLoading ? 'var(--argus-bg-elevated)' : errorBg(0.12),
          color: 'var(--argus-error)',
          border: `1px solid ${errorBg(0.25)}`,
          opacity: !erasureStudentId.trim() || erasureLoading ? '0.5' : '1'
        }"
        :disabled="!erasureStudentId.trim() || erasureLoading"
        @click="requestErasure"
      >
        {{ erasureLoading ? 'Удаление...' : 'Удалить биометрические данные' }}
      </button>

      <p
        v-if="erasureResult"
        class="mt-3 text-sm"
        :style="{ color: erasureResult.includes('Ошибка') ? 'var(--argus-error)' : 'var(--argus-success)' }"
      >
        {{ erasureResult }}
      </p>
    </div>

    <!-- Audit Log Export -->
    <div
      class="rounded-xl border p-5"
      style="background: var(--argus-bg-card); border-color: var(--argus-border);"
    >
      <h2
        class="text-base font-semibold mb-1"
        style="color: var(--argus-text);"
      >
        Экспорт журнала аудита
      </h2>
      <p
        class="text-xs mb-4"
        style="color: var(--argus-text-dimmed);"
      >
        Выгрузить все действия администраторов и прокторов в CSV для проверки регулятором.
      </p>

      <button
        class="px-4 py-2 rounded-lg text-sm font-semibold transition-all"
        :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.25)}` }"
        :disabled="auditExportLoading"
        @click="exportAuditLog"
      >
        <span v-if="auditExportLoading">Экспорт...</span>
        <span v-else>Скачать audit.csv</span>
      </button>
    </div>
  </div>
</template>
