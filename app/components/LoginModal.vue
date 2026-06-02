<script setup lang="ts">
import { useAuthStore } from '~/stores/useAuthStore'
import { useAdminAPI } from '~/composables/useAdminAPI'

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'login-success': []
}>()

const authStore = useAuthStore()
const adminAPI = useAdminAPI()
const { demoMode } = useDemoMode()

// Form state
const phoneDigits = ref('')
const password = ref('')
const errorMessage = ref('')
const isSubmitting = ref(false)
const showPassword = ref(false)
const phoneInput = ref<HTMLInputElement | null>(null)

// Auto-focus phone input when modal opens
watch(() => props.modelValue, (val) => {
  if (val) {
    nextTick(() => {
      phoneInput.value?.focus()
    })
    // Reset form on open
    errorMessage.value = ''
  }
})

// Phone input: only allow digits, max 10 chars
function onPhoneInput(event: Event) {
  const input = event.target as HTMLInputElement
  input.value = input.value.replace(/\D/g, '')
  if (input.value.length > 10) {
    input.value = input.value.slice(0, 10)
  }
  phoneDigits.value = input.value
}

async function handleSubmit() {
  errorMessage.value = ''

  if (!phoneDigits.value || phoneDigits.value.length < 10) {
    errorMessage.value = 'Введите 10 цифр номера телефона'
    return
  }

  if (!password.value) {
    errorMessage.value = 'Введите пароль'
    return
  }

  isSubmitting.value = true

  try {
    const fullPhone = `+7${phoneDigits.value}`

    // Try real API login first.
    await adminAPI.login(fullPhone, password.value)

    emit('update:modelValue', false)
    emit('login-success')

    // Reset form
    phoneDigits.value = ''
    password.value = ''
    errorMessage.value = ''

    // Navigate to main dashboard.
    navigateTo('/dashboard')
  } catch (err: unknown) {
    // Demo login is allowed only when the operator explicitly enables demo mode.
    const fullPhone = `+7${phoneDigits.value}`
    const VALID_PHONE = '+77077469966'
    const VALID_PASSWORD = 'Astana01+'

    if (demoMode.value && fullPhone === VALID_PHONE && password.value === VALID_PASSWORD) {
      // Demo fallback — set basic auth state, a demo JWT, and mock user data.
      authStore.login(fullPhone, password.value)
      authStore.setToken('demo-jwt-token', 'demo-session', 'demo-super-admin', '*')
      authStore.setUserData({
        id: 'demo-super-admin',
        orgId: '*',
        phone: fullPhone,
        fullName: 'Argus Super Admin',
        email: 'admin@argus.ai',
        role: 'super_admin',
        isActive: true
      })

      emit('update:modelValue', false)
      emit('login-success')
      phoneDigits.value = ''
      password.value = ''
      errorMessage.value = ''
      navigateTo('/dashboard')
    } else {
      // Show error from API or generic message.
      if (err instanceof Error) {
        errorMessage.value = err.message || 'Неверный номер телефона или пароль'
      } else {
        errorMessage.value = 'Неверный номер телефона или пароль'
      }
    }
  } finally {
    isSubmitting.value = false
  }
}

function close() {
  emit('update:modelValue', false)
  errorMessage.value = ''
}

// Close on Escape key
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && props.modelValue) close()
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="login-modal">
      <div
        v-if="modelValue"
        class="fixed inset-0 z-[100] flex items-center justify-center px-4"
        @click.self="close"
      >
        <!-- Backdrop -->
        <div
          class="absolute inset-0"
          style="background: rgba(0, 0, 0, 0.6); backdrop-filter: blur(8px);"
        />

        <!-- Modal Panel -->
        <div
          class="relative w-full max-w-md rounded-2xl p-8"
          style="
            background: linear-gradient(135deg, rgba(17, 24, 34, 0.95), rgba(22, 31, 44, 0.9));
            backdrop-filter: blur(20px);
            border: 1px solid rgba(66, 133, 244, 0.2);
            box-shadow:
              0 0 40px rgba(66, 133, 244, 0.08),
              0 25px 50px rgba(0, 0, 0, 0.5),
              inset 0 1px 0 rgba(255, 255, 255, 0.05);
          "
        >
          <!-- Close button -->
          <button
            class="absolute top-4 right-4 flex items-center justify-center size-8 rounded-lg text-white/40 hover:text-white/80 hover:bg-white/5 transition-all"
            @click="close"
          >
            <UIcon
              name="i-lucide-x"
              class="size-5"
            />
          </button>

          <!-- Header -->
          <div class="text-center mb-8">
            <div class="inline-flex items-center justify-center mb-4">
              <ArgusLogo :size="40" />
            </div>
            <h2 class="text-xl font-bold text-white">
              Вход в систему
            </h2>
            <p class="text-sm text-white/40 mt-1">
              Argus AI — Панель прокторинга
            </p>
            <div
              v-if="demoMode"
              class="inline-flex items-center gap-1.5 mt-3 px-2.5 py-1 rounded-full text-[11px] font-semibold"
              style="background: rgba(251, 191, 36, 0.12); border: 1px solid rgba(251, 191, 36, 0.25); color: #fbbf24;"
            >
              <UIcon
                name="i-lucide-flask-conical"
                class="size-3"
              />
              Demo mode
            </div>
          </div>

          <!-- Form -->
          <form
            class="space-y-5"
            @submit.prevent="handleSubmit"
          >
            <!-- Phone field -->
            <div>
              <label class="block text-sm font-medium text-white/60 mb-2">Номер телефона</label>
              <div
                class="flex items-center rounded-xl overflow-hidden transition-all login-input-group"
              >
                <!-- Fixed +7 prefix -->
                <span
                  class="shrink-0 px-4 py-3 text-sm font-medium text-white/50 select-none"
                  style="background: rgba(255, 255, 255, 0.03); border-right: 1px solid rgba(255, 255, 255, 0.08);"
                >
                  +7
                </span>
                <input
                  ref="phoneInput"
                  type="tel"
                  :value="phoneDigits"
                  placeholder="700 000 00 00"
                  maxlength="10"
                  autocomplete="tel"
                  class="flex-1 bg-transparent px-4 py-3 text-sm text-white placeholder-white/20 outline-none"
                  @input="onPhoneInput"
                >
              </div>
            </div>

            <!-- Password field -->
            <div>
              <label class="block text-sm font-medium text-white/60 mb-2">Пароль</label>
              <div class="relative flex items-center">
                <input
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  placeholder="Введите пароль"
                  autocomplete="current-password"
                  class="login-input w-full rounded-xl px-4 py-3 text-sm text-white placeholder-white/20 outline-none transition-all pr-12"
                >
                <button
                  type="button"
                  class="absolute right-3 top-1/2 flex size-8 -translate-y-1/2 items-center justify-center rounded-lg text-white/40 transition-colors hover:text-white/80"
                  @click="showPassword = !showPassword"
                >
                  <UIcon
                    :name="showPassword ? 'i-lucide-eye-off' : 'i-lucide-eye'"
                    class="size-5"
                  />
                </button>
              </div>
            </div>

            <!-- Error message -->
            <Transition name="login-modal">
              <div
                v-if="errorMessage"
                class="flex items-center gap-2 px-4 py-3 rounded-xl text-sm"
                style="background: rgba(248, 113, 113, 0.1); border: 1px solid rgba(248, 113, 113, 0.2); color: #F87171;"
              >
                <UIcon
                  name="i-lucide-alert-circle"
                  class="size-4 shrink-0"
                />
                <span>{{ errorMessage }}</span>
              </div>
            </Transition>

            <!-- Submit button -->
            <button
              type="submit"
              :disabled="isSubmitting"
              class="scanner-btn w-full px-6 py-3.5 rounded-xl text-sm font-bold text-white transition-all"
              :class="{ 'opacity-60 pointer-events-none': isSubmitting }"
            >
              <span>{{ isSubmitting ? 'Вход...' : 'Войти' }}</span>
            </button>
          </form>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* Modal transition */
.login-modal-enter-active {
  transition: opacity 0.25s ease;
}
.login-modal-leave-active {
  transition: opacity 0.2s ease;
}
.login-modal-enter-from,
.login-modal-leave-to {
  opacity: 0;
}

/* Input base styling */
.login-input,
.login-input-group {
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

/* Focus styling for inputs */
.login-input:focus {
  border-color: rgba(66, 133, 244, 0.4);
  box-shadow: 0 0 0 3px rgba(66, 133, 244, 0.1);
}

.login-input-group:focus-within {
  border-color: rgba(66, 133, 244, 0.4);
  box-shadow: 0 0 0 3px rgba(66, 133, 244, 0.1);
}
</style>
