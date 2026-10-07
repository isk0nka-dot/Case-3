<script setup lang="ts">
/**
 * ConsentStatus — Displays student consent record for a proctoring session.
 *
 * Features:
 * - Shows consent status (accepted / not recorded)
 * - Consent version and timestamp
 * - Visual indicator for compliance
 */
import { useAdminAPI, type ConsentRecord } from '~/composables/useAdminAPI'
import { useColors } from '~/composables/useColors'
import { formatDateTimeFull as formatDateTime } from '~/composables/useFormatters'

const props = defineProps<{
  sessionId: string
}>()

const api = useAdminAPI()
const { successBg, warningBg } = useColors()

// State
const consent = ref<ConsentRecord | null>(null)
const loading = ref(false)
const notFound = ref(false)

async function fetchConsent() {
  loading.value = true
  notFound.value = false
  try {
    consent.value = await api.getConsent(props.sessionId)
  } catch {
    consent.value = null
    notFound.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => fetchConsent())
watch(() => props.sessionId, () => fetchConsent())
</script>

<template>
  <div class="space-y-2">
    <!-- Header -->
    <div class="flex items-center gap-2">
      <UIcon
        name="i-lucide-file-check"
        class="size-4"
        style="color: var(--argus-accent);"
      />
      <span
        class="text-xs font-semibold"
        style="color: var(--argus-text);"
      >Согласие студента</span>
    </div>

    <!-- Loading -->
    <div
      v-if="loading"
      class="flex items-center gap-2 py-2"
    >
      <div
        class="animate-spin rounded-full size-3.5 border border-t-transparent"
        style="border-color: var(--argus-accent); border-top-color: transparent;"
      />
      <span
        class="text-[10px]"
        style="color: var(--argus-text-dimmed);"
      >Загрузка...</span>
    </div>

    <!-- Consent Found -->
    <div
      v-else-if="consent"
      class="rounded-lg border p-3"
      :style="{
        borderColor: consent.accepted ? 'rgba(52, 211, 153, 0.2)' : 'rgba(251, 191, 36, 0.2)',
        background: consent.accepted ? successBg(0.04) : warningBg(0.04)
      }"
    >
      <div class="flex items-center justify-between mb-2">
        <span
          class="inline-flex items-center gap-1 text-[9px] font-bold px-2 py-0.5 rounded-full uppercase"
          :style="{
            background: consent.accepted ? successBg(0.1) : warningBg(0.1),
            color: consent.accepted ? 'var(--argus-success)' : 'var(--argus-warning)'
          }"
        >
          <UIcon
            :name="consent.accepted ? 'i-lucide-check-circle' : 'i-lucide-alert-circle'"
            class="size-2.5"
          />
          {{ consent.accepted ? 'Принято' : 'Отклонено' }}
        </span>
        <span
          class="text-[8px] font-mono"
          style="color: var(--argus-text-dimmed);"
        >
          v{{ consent.consentVersion || '1.0' }}
        </span>
      </div>

      <div class="space-y-1">
        <div class="flex items-center gap-2">
          <span
            class="text-[9px]"
            style="color: var(--argus-text-dimmed);"
          >Студент:</span>
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-text);"
          >{{ consent.studentId }}</span>
        </div>
        <div class="flex items-center gap-2">
          <span
            class="text-[9px]"
            style="color: var(--argus-text-dimmed);"
          >Дата:</span>
          <span
            class="text-[9px] font-medium"
            style="color: var(--argus-text);"
          >{{ formatDateTime(consent.createdAt) }}</span>
        </div>
      </div>
    </div>

    <!-- No Consent Record -->
    <div
      v-else-if="notFound"
      class="flex items-center gap-2 px-3 py-2.5 rounded-lg border"
      :style="{ borderColor: 'rgba(251, 191, 36, 0.2)', background: warningBg(0.04) }"
    >
      <UIcon
        name="i-lucide-file-question"
        class="size-3.5"
        style="color: var(--argus-warning);"
      />
      <span
        class="text-[10px] font-medium"
        style="color: var(--argus-warning);"
      >
        Запись согласия не найдена для данной сессии
      </span>
    </div>
  </div>
</template>
