<script setup lang="ts">
// =============================================================================
// LivenessChallenge — Active Liveness Challenge Overlay
// =============================================================================
// Renders challenge instructions, countdown timer, and pass/fail result.
// Positioned as a fixed overlay on top of the video feed.
// Triggered by useLivenessChallenge composable state.
// =============================================================================

import type { LivenessChallenge as LivenessChallengeType, ChallengeStatus } from '~/composables/useLivenessChallenge'

const props = defineProps<{
  challenge: LivenessChallengeType | null
}>()

const emit = defineEmits<{
  (e: 'dismissed'): void
}>()

const { isDark, accentBg, errorBg, successBg } = useColors()

// Countdown timer
const remainingMs = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

// Result display
const showResult = ref(false)
const lastResult = ref<'passed' | 'failed' | 'timeout' | null>(null)
let resultTimer: ReturnType<typeof setTimeout> | null = null

// Challenge icon mapping
const challengeIcons: Record<string, string> = {
  turn_head_left: 'i-lucide-arrow-left',
  turn_head_right: 'i-lucide-arrow-right',
  look_up: 'i-lucide-arrow-up',
  look_down: 'i-lucide-arrow-down',
  blink_twice: 'i-lucide-eye',
  nod_yes: 'i-lucide-arrow-down-up',
}

// Track whether a challenge was active before it was cleared
let wasActive = false

// Watch challenge changes
watch(() => props.challenge, (newChallenge, oldChallenge) => {
  // Challenge started
  if (newChallenge && newChallenge.status === 'active') {
    showResult.value = false
    lastResult.value = null
    wasActive = true
    startCountdown(newChallenge.deadline)
  }

  // Challenge cleared after being active → show result
  if (!newChallenge && wasActive) {
    wasActive = false
    stopCountdown()
    // Determine result from old challenge status
    const status = oldChallenge?.status
    lastResult.value = status === 'passed' ? 'passed' : status === 'timeout' ? 'timeout' : 'failed'
    showResult.value = true

    if (resultTimer) clearTimeout(resultTimer)
    resultTimer = setTimeout(() => {
      showResult.value = false
      lastResult.value = null
      emit('dismissed')
    }, 2000)
  }
}, { deep: true })

function startCountdown(deadline: number) {
  stopCountdown()
  updateRemaining(deadline)
  countdownTimer = setInterval(() => updateRemaining(deadline), 100)
}

function updateRemaining(deadline: number) {
  remainingMs.value = Math.max(0, deadline - Date.now())
  if (remainingMs.value <= 0) {
    stopCountdown()
  }
}

function stopCountdown() {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

const remainingSec = computed(() => Math.ceil(remainingMs.value / 1000))
const progress = computed(() => {
  if (!props.challenge) return 0
  const total = props.challenge.deadline - props.challenge.issuedAt
  return Math.max(0, Math.min(1, remainingMs.value / total))
})

const isUrgent = computed(() => remainingSec.value <= 3)

onUnmounted(() => {
  stopCountdown()
  if (resultTimer) clearTimeout(resultTimer)
})
</script>

<template>
  <!-- Active Challenge Overlay -->
  <Transition name="challenge">
    <div
      v-if="challenge && challenge.status === 'active'"
      class="fixed inset-x-0 top-0 z-50 flex items-start justify-center pt-6 pointer-events-none"
    >
      <div
        class="pointer-events-auto rounded-2xl px-6 py-4 shadow-2xl border backdrop-blur-xl max-w-sm w-full mx-4"
        :style="{
          background: isDark ? 'rgba(11, 15, 20, 0.92)' : 'rgba(255, 255, 255, 0.95)',
          borderColor: isUrgent ? 'var(--argus-error)' : 'var(--argus-accent)',
        }"
      >
        <!-- Header -->
        <div class="flex items-center gap-2 mb-3">
          <div
            class="flex items-center justify-center size-8 rounded-lg"
            :style="{ background: accentBg(0.15) }"
          >
            <UIcon
              :name="challengeIcons[challenge.type] ?? 'i-lucide-scan-face'"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div class="flex-1 min-w-0">
            <p class="text-[10px] font-bold uppercase tracking-wider" style="color: var(--argus-accent);">
              Проверка живости
            </p>
          </div>
          <div
            class="text-lg font-mono font-bold tabular-nums"
            :style="{ color: isUrgent ? 'var(--argus-error)' : 'var(--argus-text)' }"
          >
            {{ remainingSec }}с
          </div>
        </div>

        <!-- Instruction -->
        <p class="text-base font-semibold mb-3" style="color: var(--argus-text);">
          {{ challenge.instruction }}
        </p>

        <!-- Progress Bar -->
        <div
          class="h-1.5 rounded-full overflow-hidden"
          :style="{ background: isDark ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.06)' }"
        >
          <div
            class="h-full rounded-full transition-all duration-100"
            :style="{
              width: `${progress * 100}%`,
              background: isUrgent
                ? 'var(--argus-error)'
                : 'var(--argus-accent)',
            }"
          />
        </div>
      </div>
    </div>
  </Transition>

  <!-- Result Toast -->
  <Transition name="result">
    <div
      v-if="showResult && lastResult"
      class="fixed inset-x-0 top-0 z-50 flex items-start justify-center pt-6 pointer-events-none"
    >
      <div
        class="pointer-events-auto rounded-2xl px-6 py-4 shadow-2xl border backdrop-blur-xl max-w-sm w-full mx-4 flex items-center gap-3"
        :style="{
          background: isDark ? 'rgba(11, 15, 20, 0.92)' : 'rgba(255, 255, 255, 0.95)',
          borderColor: lastResult === 'passed' ? 'var(--argus-success)' : 'var(--argus-error)',
        }"
      >
        <div
          class="flex items-center justify-center size-10 rounded-xl"
          :style="{ background: lastResult === 'passed' ? successBg(0.15) : errorBg(0.15) }"
        >
          <UIcon
            :name="lastResult === 'passed' ? 'i-lucide-check-circle' : 'i-lucide-x-circle'"
            class="size-6"
            :style="{ color: lastResult === 'passed' ? 'var(--argus-success)' : 'var(--argus-error)' }"
          />
        </div>
        <div>
          <p class="text-sm font-semibold" style="color: var(--argus-text);">
            {{ lastResult === 'passed' ? 'Проверка пройдена' : lastResult === 'timeout' ? 'Время истекло' : 'Проверка не пройдена' }}
          </p>
          <p class="text-xs mt-0.5" style="color: var(--argus-text-dimmed);">
            {{ lastResult === 'passed' ? 'Продолжайте работу' : 'Следующая проверка будет позже' }}
          </p>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.challenge-enter-active,
.challenge-leave-active {
  transition: all 0.35s cubic-bezier(0.16, 1, 0.3, 1);
}
.challenge-enter-from {
  opacity: 0;
  transform: translateY(-20px) scale(0.95);
}
.challenge-leave-to {
  opacity: 0;
  transform: translateY(-10px) scale(0.98);
}

.result-enter-active,
.result-leave-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.result-enter-from {
  opacity: 0;
  transform: translateY(-16px) scale(0.95);
}
.result-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
