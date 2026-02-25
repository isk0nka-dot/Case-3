<script setup lang="ts">
// =============================================================================
// HardBlockerModal — Unclosable Storage Critical Modal (ADR-007)
// =============================================================================
//
// Displayed when browser storage reaches 90% of quota. This modal is UNCLOSABLE
// by design — there is no X button, no backdrop click handler, no Escape key.
// The exam cannot continue until the student frees disk space and storage drops
// below 80% (recovery threshold).
//
// Evidence is NEVER evicted. This modal is the enforcement mechanism for the
// "zero evidence loss" policy defined in ADR-007.
//
// =============================================================================

defineProps<{
  /** Whether the hard blocker is active. */
  isBlocked: boolean
  /** Current storage usage as a fraction (0-1). */
  storagePercent: number
  /** Current storage usage in megabytes. */
  usageMB: number
  /** Total storage quota in megabytes. */
  quotaMB: number
}>()
</script>

<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="isBlocked"
        class="fixed inset-0 z-[300] flex items-center justify-center"
        style="background: rgba(0, 0, 0, 0.85)"
      >
        <!-- NO backdrop click handler — modal is unclosable -->

        <!-- Content -->
        <div
          class="relative w-full max-w-lg mx-4 rounded-2xl border-2 overflow-hidden z-10"
          style="
            background: var(--argus-bg-card, #1a1a2e);
            border-color: var(--argus-error, #ef4444);
          "
        >
          <!-- Header — Red warning -->
          <div
            class="px-6 py-4 flex items-center gap-3"
            style="background: rgba(239, 68, 68, 0.15)"
          >
            <svg
              class="w-8 h-8 flex-shrink-0"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
              style="color: var(--argus-error, #ef4444)"
            >
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
              <path d="M12 8v4" />
              <path d="M12 16h.01" />
            </svg>
            <div>
              <h2
                class="text-lg font-bold tracking-wide"
                style="color: var(--argus-error, #ef4444)"
              >
                КРИТИЧЕСКАЯ ОШИБКА ХРАНЕНИЯ
              </h2>
              <p
                class="text-xs mt-0.5"
                style="color: var(--argus-text-dimmed, #a0a0b0)"
              >
                Экзамен приостановлен
              </p>
            </div>
          </div>

          <!-- Body -->
          <div class="px-6 py-5 space-y-4">
            <!-- Storage usage bar -->
            <div>
              <div class="flex justify-between text-sm mb-1.5">
                <span style="color: var(--argus-text, #e0e0e8)">
                  Использовано
                </span>
                <span
                  class="font-mono font-bold"
                  style="color: var(--argus-error, #ef4444)"
                >
                  {{ Math.round(storagePercent * 100) }}%
                </span>
              </div>
              <div
                class="h-3 rounded-full overflow-hidden"
                style="background: var(--argus-bg-deep, #0d0d1a)"
              >
                <div
                  class="h-full rounded-full transition-all duration-500"
                  :style="{
                    width: Math.min(storagePercent * 100, 100) + '%',
                    background:
                      storagePercent >= 0.9
                        ? 'var(--argus-error, #ef4444)'
                        : 'var(--argus-warning, #f59e0b)'
                  }"
                />
              </div>
              <div
                class="flex justify-between text-xs mt-1"
                style="color: var(--argus-text-muted, #6b6b80)"
              >
                <span>{{ usageMB }} МБ использовано</span>
                <span>{{ quotaMB }} МБ всего</span>
              </div>
            </div>

            <!-- Warning message -->
            <div
              class="p-4 rounded-lg text-sm leading-relaxed"
              style="
                background: rgba(239, 68, 68, 0.08);
                border: 1px solid rgba(239, 68, 68, 0.2);
                color: var(--argus-text, #e0e0e8);
              "
            >
              <p class="font-semibold mb-2" style="color: var(--argus-error, #ef4444)">
                Хранилище браузера почти заполнено.
              </p>
              <p>
                Данные прокторинга не могут быть сохранены. Экзамен приостановлен
                для предотвращения потери доказательств. Для продолжения:
              </p>
              <ol class="list-decimal list-inside mt-2 space-y-1 pl-1">
                <li>Закройте лишние вкладки браузера</li>
                <li>Удалите ненужные загрузки и файлы</li>
                <li>Очистите кэш других сайтов</li>
                <li>Дождитесь автоматического восстановления</li>
              </ol>
            </div>

            <!-- Auto-recovery notice -->
            <div
              class="flex items-center gap-2 text-xs px-1"
              style="color: var(--argus-text-muted, #6b6b80)"
            >
              <svg
                class="w-4 h-4 animate-spin flex-shrink-0"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              >
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
              <span>
                Проверка каждые 30 секунд. Экзамен возобновится автоматически
                когда использование хранилища снизится ниже 80%.
              </span>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.3s ease;
}

.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
