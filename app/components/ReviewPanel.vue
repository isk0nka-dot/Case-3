<script setup lang="ts">
/**
 * ReviewPanel — mandatory human review workflow for proctoring sessions.
 *
 * Provides three action buttons:
 * - Confirmed (violations are genuine, consequences apply)
 * - Dismissed (false positives, session cleared)
 * - Escalated (requires senior review, ethics committee)
 *
 * Also shows the existing review decision if one exists.
 */
import { useAdminAPI, type ReviewDecision, type SubmitReviewRequest } from '~/composables/useAdminAPI'

const props = defineProps<{
  sessionId: string
  integrityScore: number
  evidenceIds?: string[]
}>()

const emit = defineEmits<{
  (e: 'reviewed', decision: ReviewDecision): void
}>()

const api = useAdminAPI()
const { formatDateTime } = useFormatters()
const { isDark } = useColors()
const { reviewDecisionLabel, reviewDecisionColor, reviewDecisionBg, reviewDecisionIcon } = useStatusHelpers()

// State
const existingReview = ref<ReviewDecision | null>(null)
const reviewPending = ref(true)
const loading = ref(false)
const submitting = ref(false)
const notes = ref('')
const error = ref('')
const success = ref('')

// Fetch existing review on mount
async function fetchReview() {
  loading.value = true
  try {
    const resp = await api.getReview(props.sessionId)
    if ('decision' in resp) {
      existingReview.value = resp as ReviewDecision
      reviewPending.value = false
    } else {
      existingReview.value = null
      reviewPending.value = true
    }
  } catch (err: any) {
    existingReview.value = null
    reviewPending.value = true
  } finally {
    loading.value = false
  }
}

// Submit a review decision
async function submitDecision(decision: 'confirmed' | 'dismissed' | 'escalated') {
  submitting.value = true
  error.value = ''
  success.value = ''

  const req: SubmitReviewRequest = {
    decision,
    notes: notes.value,
    evidenceIds: props.evidenceIds || [],
    integrityScore: props.integrityScore
  }

  try {
    const resp = await api.submitReview(props.sessionId, req)
    existingReview.value = resp
    reviewPending.value = false
    success.value = reviewDecisionLabel(decision) + ' — решение сохранено'
    emit('reviewed', resp)
  } catch (err: any) {
    error.value = err.message || 'Failed to submit review'
  } finally {
    submitting.value = false
  }
}

// Decision helpers centralized in useStatusHelpers composable

onMounted(() => fetchReview())
watch(() => props.sessionId, () => {
  notes.value = ''
  error.value = ''
  success.value = ''
  fetchReview()
})
</script>

<template>
  <div class="space-y-3">
    <!-- Header -->
    <div class="flex items-center gap-2">
      <UIcon
        name="i-lucide-clipboard-check"
        class="size-4"
        style="color: var(--argus-accent);"
      />
      <span
        class="text-xs font-semibold"
        style="color: var(--argus-text);"
      >Рецензия проктора</span>
      <span
        class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
        :style="{
          background: reviewPending
            ? (isDark ? 'rgba(251, 191, 36, 0.1)' : 'rgba(230, 126, 34, 0.08)')
            : (isDark ? 'rgba(52, 211, 153, 0.1)' : 'rgba(16, 163, 74, 0.08)'),
          color: reviewPending ? 'var(--argus-warning)' : 'var(--argus-success)'
        }"
      >
        {{ reviewPending ? 'Ожидает' : 'Решение принято' }}
      </span>
    </div>

    <!-- Loading -->
    <div
      v-if="loading"
      class="flex items-center justify-center py-4"
    >
      <div
        class="animate-spin rounded-full size-5 border-2 border-t-transparent"
        style="border-color: var(--argus-accent); border-top-color: transparent;"
      />
    </div>

    <!-- Existing Review Display -->
    <div
      v-else-if="existingReview"
      class="rounded-lg border p-3 space-y-2"
      :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
    >
      <!-- Decision badge -->
      <div class="flex items-center justify-between">
        <span
          class="inline-flex items-center gap-1.5 text-[10px] font-bold px-2.5 py-1 rounded-full uppercase"
          :style="{ background: reviewDecisionBg(existingReview.decision), color: reviewDecisionColor(existingReview.decision) }"
        >
          <UIcon
            :name="reviewDecisionIcon(existingReview.decision)"
            class="size-3"
          />
          {{ reviewDecisionLabel(existingReview.decision) }}
        </span>
        <span
          class="text-[9px]"
          style="color: var(--argus-text-dimmed);"
        >
          {{ formatDateTime(existingReview.reviewedAt) }}
        </span>
      </div>

      <!-- Reviewer info -->
      <div class="flex items-center gap-2">
        <UIcon
          name="i-lucide-user"
          class="size-3"
          style="color: var(--argus-text-dimmed);"
        />
        <span
          class="text-[10px] font-medium"
          style="color: var(--argus-text-muted);"
        >
          {{ existingReview.reviewerName }}
        </span>
      </div>

      <!-- Notes -->
      <div
        v-if="existingReview.notes"
        class="mt-1"
      >
        <p
          class="text-[10px]"
          style="color: var(--argus-text-dimmed);"
        >
          Комментарий:
        </p>
        <p
          class="text-[10px] mt-0.5"
          style="color: var(--argus-text);"
        >
          {{ existingReview.notes }}
        </p>
      </div>

      <!-- Override button -->
      <button
        class="text-[9px] font-medium px-2 py-1 rounded transition-all mt-2"
        :style="{ color: 'var(--argus-text-dimmed)', background: 'var(--argus-bg-hover)' }"
        @click="reviewPending = true; existingReview = null"
      >
        Изменить решение
      </button>
    </div>

    <!-- Review Form (when pending) -->
    <div
      v-else
      class="space-y-3"
    >
      <!-- Notes textarea -->
      <div>
        <label
          class="text-[9px] font-medium block mb-1"
          style="color: var(--argus-text-dimmed);"
        >
          Комментарий проктора
        </label>
        <textarea
          v-model="notes"
          rows="3"
          placeholder="Описание нарушений, замечания..."
          class="w-full px-3 py-2 rounded-lg border text-xs resize-none outline-none transition-colors"
          :style="{
            background: 'var(--argus-bg-elevated)',
            borderColor: 'var(--argus-border)',
            color: 'var(--argus-text)'
          }"
          @focus="($event.target as HTMLTextAreaElement).style.borderColor = 'var(--argus-accent)'"
          @blur="($event.target as HTMLTextAreaElement).style.borderColor = 'var(--argus-border)'"
        />
      </div>

      <!-- Action buttons -->
      <div class="flex gap-2">
        <!-- Confirmed (violations genuine) -->
        <button
          class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg text-[10px] font-bold transition-all border"
          :style="{
            background: isDark ? 'rgba(248, 113, 113, 0.08)' : 'rgba(224, 62, 62, 0.05)',
            borderColor: isDark ? 'rgba(248, 113, 113, 0.2)' : 'rgba(224, 62, 62, 0.15)',
            color: 'var(--argus-error)'
          }"
          :disabled="submitting"
          @click="submitDecision('confirmed')"
        >
          <UIcon
            name="i-lucide-alert-triangle"
            class="size-3"
          />
          Подтвердить
        </button>

        <!-- Dismissed (false positives) -->
        <button
          class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg text-[10px] font-bold transition-all border"
          :style="{
            background: isDark ? 'rgba(52, 211, 153, 0.08)' : 'rgba(16, 163, 74, 0.05)',
            borderColor: isDark ? 'rgba(52, 211, 153, 0.2)' : 'rgba(16, 163, 74, 0.15)',
            color: 'var(--argus-success)'
          }"
          :disabled="submitting"
          @click="submitDecision('dismissed')"
        >
          <UIcon
            name="i-lucide-check-circle"
            class="size-3"
          />
          Отклонить
        </button>

        <!-- Escalated -->
        <button
          class="flex-1 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg text-[10px] font-bold transition-all border"
          :style="{
            background: isDark ? 'rgba(251, 191, 36, 0.08)' : 'rgba(230, 126, 34, 0.05)',
            borderColor: isDark ? 'rgba(251, 191, 36, 0.2)' : 'rgba(230, 126, 34, 0.15)',
            color: 'var(--argus-warning)'
          }"
          :disabled="submitting"
          @click="submitDecision('escalated')"
        >
          <UIcon
            name="i-lucide-arrow-up-circle"
            class="size-3"
          />
          Эскалировать
        </button>
      </div>

      <!-- Submitting indicator -->
      <div
        v-if="submitting"
        class="flex items-center justify-center gap-2 py-2"
      >
        <div
          class="animate-spin rounded-full size-4 border-2 border-t-transparent"
          style="border-color: var(--argus-accent); border-top-color: transparent;"
        />
        <span
          class="text-[10px]"
          style="color: var(--argus-text-dimmed);"
        >Сохранение решения...</span>
      </div>

      <!-- Error -->
      <div
        v-if="error"
        class="flex items-center gap-2 px-3 py-2 rounded-lg text-[10px]"
        :style="{ background: isDark ? 'rgba(248, 113, 113, 0.1)' : 'rgba(224, 62, 62, 0.08)', color: 'var(--argus-error)' }"
      >
        <UIcon
          name="i-lucide-alert-circle"
          class="size-3 shrink-0"
        />
        {{ error }}
      </div>

      <!-- Success -->
      <div
        v-if="success"
        class="flex items-center gap-2 px-3 py-2 rounded-lg text-[10px]"
        :style="{ background: isDark ? 'rgba(52, 211, 153, 0.1)' : 'rgba(16, 163, 74, 0.08)', color: 'var(--argus-success)' }"
      >
        <UIcon
          name="i-lucide-check-circle"
          class="size-3 shrink-0"
        />
        {{ success }}
      </div>
    </div>
  </div>
</template>
