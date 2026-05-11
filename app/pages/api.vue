<script setup lang="ts">
import { useDashboardStore } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'
import { useAdminAPI } from '~/composables/useAdminAPI'
import type { Organization, APIKey } from '~/composables/useAdminAPI'

const store = useDashboardStore()
const authStore = useAuthStore()
const adminAPI = useAdminAPI()
const { isDark, accentBg, errorBg, successBg, warningBg } = useColors()

// --- Multi-tenant state ---
const orgs = ref<Organization[]>([])
const orgsLoading = ref(false)
const selectedOrgFilter = ref<string | null>(null) // null = all orgs
const showOrgDropdown = ref(false)

// API keys from backend (real data)
const backendKeys = ref<APIKey[]>([])
const backendKeysLoading = ref(false)
const backendKeysError = ref<string | null>(null)

// --- Fetch organizations (Super Admin only) ---
async function fetchOrgs() {
  if (!authStore.isSuperAdmin) return
  orgsLoading.value = true
  try {
    orgs.value = await adminAPI.listOrgs()
  } catch {
    // Fallback: keep empty, UI will degrade gracefully
    orgs.value = []
  } finally {
    orgsLoading.value = false
  }
}

// --- Fetch API keys from backend ---
async function fetchAPIKeys() {
  const orgId = authStore.isSuperAdmin ? selectedOrgFilter.value : authStore.user?.orgId
  if (!orgId) {
    // "All orgs" mode for super admin — fetch from all orgs
    if (authStore.isSuperAdmin && orgs.value.length > 0) {
      backendKeysLoading.value = true
      backendKeysError.value = null
      try {
        const allKeys: APIKey[] = []
        const results = await Promise.allSettled(
          orgs.value.map(org => adminAPI.listAPIKeys(org.orgId))
        )
        for (const result of results) {
          if (result.status === 'fulfilled') {
            allKeys.push(...result.value)
          }
        }
        backendKeys.value = allKeys
      } catch {
        backendKeysError.value = 'Не удалось загрузить API-ключи'
        backendKeys.value = []
      } finally {
        backendKeysLoading.value = false
      }
    } else {
      backendKeys.value = []
    }
    return
  }

  backendKeysLoading.value = true
  backendKeysError.value = null
  try {
    backendKeys.value = await adminAPI.listAPIKeys(orgId)
  } catch {
    backendKeysError.value = 'Не удалось загрузить API-ключи'
    backendKeys.value = []
  } finally {
    backendKeysLoading.value = false
  }
}

// --- Determine data source: backend or mock fallback ---
const useBackendData = computed(() => backendKeys.value.length > 0 || backendKeysLoading.value)

// Unified keys list — backend keys mapped to display format, or store mock data
const displayKeys = computed(() => {
  if (useBackendData.value && backendKeys.value.length > 0) {
    return backendKeys.value.map(k => ({
      id: k.id,
      orgId: k.orgId,
      name: k.name,
      key: `${k.keyId}_${k.secretPrefix}••••••••••••`,
      keyId: k.keyId,
      secretPrefix: k.secretPrefix,
      created: k.createdAt,
      lastUsed: k.lastUsedAt || '\u2014',
      status: k.isActive ? 'active' as const : 'revoked' as const,
      permissions: k.permissions,
      rateLimitRps: k.rateLimitRps
    }))
  }
  // Fallback to store mock data
  return store.apiKeys.map(k => ({
    id: k.id,
    orgId: authStore.user?.orgId || 'unknown',
    name: k.name,
    key: k.key,
    keyId: '',
    secretPrefix: '',
    created: k.created,
    lastUsed: k.lastUsed,
    status: k.status,
    permissions: k.permissions,
    rateLimitRps: 0
  }))
})

// Filter keys by selected org (Super Admin)
const filteredKeys = computed(() => {
  if (!authStore.isSuperAdmin || !selectedOrgFilter.value) {
    return displayKeys.value
  }
  return displayKeys.value.filter(k => k.orgId === selectedOrgFilter.value)
})

// Org name resolver
function orgName(orgId: string): string {
  const org = orgs.value.find(o => o.orgId === orgId)
  return org ? org.name : orgId
}

// Selected org display name
const selectedOrgName = computed(() => {
  if (!selectedOrgFilter.value) return 'Все организации'
  return orgName(selectedOrgFilter.value)
})

// --- API Key management ---
const showNewKeyForm = ref(false)
const newKeyName = ref('')
const newKeyOrgId = ref<string | null>(null) // For super admin: which org to create key for
const newKeyEnvironment = ref<'live' | 'test'>('live')
const newKeyPermissions = ref<string[]>(['read:sessions', 'read:reports'])
const copiedKeyId = ref<string | null>(null)
const revealedKeyId = ref<string | null>(null)
const createdSecretKey = ref<string | null>(null) // Shown once after creation
const createKeyLoading = ref(false)

const availablePermissions = [
  'read:sessions',
  'write:sessions',
  'read:reports',
  'write:violations',
  'read:violations',
  'write:webhooks',
  'read:webhooks',
  'admin:keys'
]

function maskKey(key: string): string {
  if (key.length <= 16) return key
  return key.substring(0, 12) + '\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022'
}

function copyKey(key: string, keyId: string) {
  navigator.clipboard.writeText(key)
  copiedKeyId.value = keyId
  setTimeout(() => { copiedKeyId.value = null }, 2000)
}

async function revokeKey(keyId: string) {
  // Try backend first
  try {
    await adminAPI.revokeAPIKey(keyId)
    // Refresh keys
    await fetchAPIKeys()
  } catch {
    // Fallback to mock store
    const key = store.apiKeys.find(k => k.id === keyId)
    if (key) key.status = 'revoked'
  }
}

async function generateNewKey() {
  if (!newKeyName.value.trim()) return

  const targetOrgId = authStore.isSuperAdmin
    ? (newKeyOrgId.value || selectedOrgFilter.value)
    : authStore.user?.orgId

  if (!targetOrgId) {
    useToast().add({
      title: 'Выберите организацию',
      description: 'Укажите организацию в поле "Организация" перед созданием ключа',
      color: 'error',
      duration: 4000
    })
    return
  }

  createKeyLoading.value = true
  createdSecretKey.value = null

  try {
    const result = await adminAPI.createAPIKey(targetOrgId, {
      name: newKeyName.value.trim(),
      permissions: newKeyPermissions.value,
      environment: newKeyEnvironment.value
    })
    createdSecretKey.value = result.secret
    await fetchAPIKeys()
    newKeyName.value = ''
    newKeyPermissions.value = ['read:sessions', 'read:reports']
    newKeyEnvironment.value = 'live'
    newKeyOrgId.value = null
    // Keep form open to show secret
  } catch {
    // Fallback to local mock generation
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
    let randomPart = ''
    for (let i = 0; i < 24; i++) {
      randomPart += chars.charAt(Math.floor(Math.random() * chars.length))
    }
    const prefix = newKeyEnvironment.value === 'live' ? 'argus_live_sk_' : 'argus_test_sk_'
    const fullKey = `${prefix}${randomPart}`
    store.apiKeys.push({
      id: `ak-${Date.now()}`,
      name: newKeyName.value.trim(),
      key: fullKey,
      created: new Date().toISOString().split('T')[0] ?? '',
      lastUsed: '\u2014',
      status: 'active',
      permissions: [...newKeyPermissions.value]
    })
    createdSecretKey.value = fullKey
    newKeyName.value = ''
    newKeyPermissions.value = ['read:sessions', 'read:reports']
    newKeyEnvironment.value = 'live'
    newKeyOrgId.value = null
  } finally {
    createKeyLoading.value = false
  }
}

function closeNewKeyForm() {
  showNewKeyForm.value = false
  createdSecretKey.value = null
}

// --- Webhook management ---
const showWebhookForm = ref(false)
const newWebhookUrl = ref('')
const selectedWebhookEvents = ref<string[]>([])

const availableEvents = [
  { value: 'violation.detected', label: 'Нарушение обнаружено' },
  { value: 'session.ended', label: 'Сессия завершена' },
  { value: 'integrity.finalized', label: 'Оценка честности' },
  { value: 'session.started', label: 'Сессия начата' },
  { value: 'alert.critical', label: 'Критическое предупреждение' }
]

function addWebhook() {
  if (!newWebhookUrl.value.trim() || selectedWebhookEvents.value.length === 0) return
  store.webhooks.push({
    id: `wh-${Date.now()}`,
    url: newWebhookUrl.value.trim(),
    events: [...selectedWebhookEvents.value],
    status: 'active',
    lastDelivery: '\u2014',
    successRate: 0
  })
  newWebhookUrl.value = ''
  selectedWebhookEvents.value = []
  showWebhookForm.value = false
}

function toggleWebhookStatus(webhookId: string) {
  const wh = store.webhooks.find(w => w.id === webhookId)
  if (wh) wh.status = wh.status === 'active' ? 'paused' : 'active'
}

// --- Formatters (auto-imported from useFormatters) ---
const formatDatetime = formatDateTime

function eventLabel(eventType: string): string {
  const found = availableEvents.find(e => e.value === eventType)
  return found ? found.label : eventType
}

// --- LMS integrations ---
const lmsIntegrations = [
  { id: 'lms-moodle', name: 'Moodle', icon: 'i-lucide-graduation-cap', description: 'LTI-интеграция для Moodle LMS', status: 'connected' },
  { id: 'lms-canvas', name: 'Canvas', icon: 'i-lucide-palette', description: 'REST API коннектор для Canvas', status: 'available' },
  { id: 'lms-classroom', name: 'Google Classroom', icon: 'i-lucide-book-open', description: 'OAuth2 интеграция с Google', status: 'available' },
  { id: 'lms-eduser', name: 'Eduser', icon: 'i-lucide-shield-check', description: 'Нативная интеграция с Eduser ҰБТ', status: 'connected' }
]

function handleLMSClick(lms: typeof lmsIntegrations[0]) {
  if (lms.status === 'connected') {
    navigateTo('/integrations')
  } else {
    useToast().add({
      title: `Интеграция ${lms.name}`,
      description: 'Скоро доступно. Обратитесь к документации для ручной настройки.',
      color: 'info',
      duration: 4000
    })
  }
}

// --- Stats ---
const deliverySuccessRate = computed(() => {
  const logs = store.webhookDeliveryLogs
  const success = logs.filter(l => l.status === 'success').length
  return logs.length > 0 ? ((success / logs.length) * 100).toFixed(1) : '0'
})

const activeKeysCount = computed(() =>
  filteredKeys.value.filter(k => k.status === 'active').length
)

const activeWebhooksCount = computed(() =>
  store.webhooks.filter(w => w.status === 'active').length
)

// --- Global Webhook Health (Super Admin cross-org) ---
const globalWebhookHealth = computed(() => {
  const logs = store.webhookDeliveryLogs
  const total = logs.length
  if (total === 0) return { rate: 0, status: 'unknown' as const, totalDeliveries: 0, failedCount: 0, avgLatency: 0 }
  const success = logs.filter(l => l.status === 'success').length
  const failed = total - success
  const rate = (success / total) * 100
  const avgLatency = Math.round(logs.reduce((sum, l) => sum + l.duration, 0) / total)
  let status: 'healthy' | 'degraded' | 'critical' | 'unknown' = 'healthy'
  if (rate < 90) status = 'critical'
  else if (rate < 98) status = 'degraded'
  return { rate: parseFloat(rate.toFixed(1)), status, totalDeliveries: total, failedCount: failed, avgLatency }
})

// --- Quotas / Limits (Super Admin) ---
const keyQuotas = computed(() => {
  const keys = filteredKeys.value.filter(k => k.status === 'active')
  const totalRps = keys.reduce((sum, k) => sum + (k.rateLimitRps || 0), 0)
  return {
    totalActive: keys.length,
    totalRps,
    avgRps: keys.length > 0 ? Math.round(totalRps / keys.length) : 0
  }
})

// --- Org selector handler ---
function selectOrg(orgId: string | null) {
  selectedOrgFilter.value = orgId
  authStore.switchOrg(orgId)
  showOrgDropdown.value = false
  // Refetch keys for selected org
  fetchAPIKeys()
}

// --- Click outside handler for org dropdown ---
function onClickOutsideOrgDropdown(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.org-selector-dropdown')) {
    showOrgDropdown.value = false
  }
}

// --- Init ---
onMounted(async () => {
  if (authStore.isSuperAdmin) {
    selectedOrgFilter.value = authStore.effectiveOrgId
    await fetchOrgs()
  }
  await fetchAPIKeys()
  document.addEventListener('click', onClickOutsideOrgDropdown)
})

onUnmounted(() => {
  document.removeEventListener('click', onClickOutsideOrgDropdown)
})

function copyToClipboard(text: string) {
  window.navigator.clipboard.writeText(text)
}
</script>

<template>
  <div class="p-6 space-y-6">
    <!-- ============================== -->
    <!--  SUPER ADMIN: ORG CONTEXT BAR  -->
    <!-- ============================== -->
    <div
      v-if="authStore.isSuperAdmin"
      class="glass-card rounded-xl px-5 py-3.5"
    >
      <div class="flex items-center justify-between gap-4 flex-wrap">
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-8 rounded-lg"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-building-2"
              class="size-4"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              Контекст организации
            </p>
            <p
              class="text-xs font-semibold"
              style="color: var(--argus-text);"
            >
              Super Admin — мультитенантный режим
            </p>
          </div>
        </div>

        <!-- Org selector dropdown -->
        <div class="relative org-selector-dropdown">
          <button
            class="flex items-center gap-2 px-4 py-2.5 rounded-lg border text-sm font-medium transition-all cursor-pointer min-w-[240px] justify-between"
            :style="{
              background: showOrgDropdown ? accentBg(0.08) : 'var(--argus-bg-card)',
              borderColor: showOrgDropdown ? accentBg(0.3) : 'var(--argus-border)',
              color: 'var(--argus-text)'
            }"
            @click.stop="showOrgDropdown = !showOrgDropdown"
          >
            <div class="flex items-center gap-2">
              <UIcon
                :name="selectedOrgFilter ? 'i-lucide-building' : 'i-lucide-globe'"
                class="size-4"
                :style="{ color: selectedOrgFilter ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)' }"
              />
              <span class="truncate max-w-[180px]">{{ selectedOrgName }}</span>
            </div>
            <UIcon
              name="i-lucide-chevron-down"
              class="size-3.5 transition-transform shrink-0"
              :class="showOrgDropdown ? 'rotate-180' : ''"
              style="color: var(--argus-text-dimmed);"
            />
          </button>

          <!-- Dropdown menu -->
          <Transition name="modal">
            <div
              v-if="showOrgDropdown"
              class="absolute right-0 top-full mt-1 w-[300px] rounded-xl border shadow-xl z-50 overflow-hidden"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <!-- All Organizations option -->
              <button
                class="w-full flex items-center gap-3 px-4 py-3 text-left transition-all cursor-pointer border-b"
                :style="{
                  background: selectedOrgFilter === null ? accentBg(0.06) : 'transparent',
                  borderColor: 'var(--argus-border-subtle)'
                }"
                @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.06)"
                @mouseleave="selectedOrgFilter !== null ? ($event.currentTarget as HTMLElement).style.background = 'transparent' : undefined"
                @click="selectOrg(null)"
              >
                <div
                  class="flex items-center justify-center size-7 rounded-md shrink-0"
                  :style="{ background: accentBg(0.1) }"
                >
                  <UIcon
                    name="i-lucide-globe"
                    class="size-3.5"
                    style="color: var(--argus-accent);"
                  />
                </div>
                <div class="flex-1 min-w-0">
                  <p
                    class="text-xs font-semibold"
                    style="color: var(--argus-text);"
                  >
                    Все организации
                  </p>
                  <p
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Агрегированные данные по всем орг.
                  </p>
                </div>
                <UIcon
                  v-if="selectedOrgFilter === null"
                  name="i-lucide-check"
                  class="size-4 shrink-0"
                  style="color: var(--argus-accent);"
                />
              </button>

              <!-- Org list -->
              <div class="max-h-[280px] overflow-y-auto">
                <div
                  v-if="orgsLoading"
                  class="flex items-center justify-center py-6"
                >
                  <UIcon
                    name="i-lucide-loader-2"
                    class="size-4 animate-spin"
                    style="color: var(--argus-text-dimmed);"
                  />
                  <span
                    class="text-xs ml-2"
                    style="color: var(--argus-text-dimmed);"
                  >Загрузка...</span>
                </div>
                <button
                  v-for="org in orgs"
                  v-else
                  :key="org.orgId"
                  class="w-full flex items-center gap-3 px-4 py-2.5 text-left transition-all cursor-pointer"
                  :style="{
                    background: selectedOrgFilter === org.orgId ? accentBg(0.06) : 'transparent',
                    borderBottom: '1px solid var(--argus-border-subtle)'
                  }"
                  @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.06)"
                  @mouseleave="selectedOrgFilter !== org.orgId ? ($event.currentTarget as HTMLElement).style.background = 'transparent' : undefined"
                  @click="selectOrg(org.orgId)"
                >
                  <div
                    class="flex items-center justify-center size-7 rounded-md shrink-0"
                    :style="{ background: org.isActive ? successBg(0.08) : 'var(--argus-bg-hover)' }"
                  >
                    <UIcon
                      name="i-lucide-building"
                      class="size-3.5"
                      :style="{ color: org.isActive ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
                    />
                  </div>
                  <div class="flex-1 min-w-0">
                    <p
                      class="text-xs font-medium truncate"
                      style="color: var(--argus-text);"
                    >
                      {{ org.name }}
                    </p>
                    <div class="flex items-center gap-2 mt-0.5">
                      <span
                        class="text-[9px]"
                        style="color: var(--argus-text-dimmed);"
                      >{{ org.slug }}</span>
                      <span
                        class="text-[8px] font-bold px-1 py-0.5 rounded uppercase"
                        :style="{
                          background: org.plan === 'enterprise' ? accentBg(0.1) : org.plan === 'pro' ? successBg(0.1) : 'var(--argus-bg-hover)',
                          color: org.plan === 'enterprise' ? 'var(--argus-accent)' : org.plan === 'pro' ? 'var(--argus-success)' : 'var(--argus-text-dimmed)'
                        }"
                      >
                        {{ org.plan }}
                      </span>
                    </div>
                  </div>
                  <UIcon
                    v-if="selectedOrgFilter === org.orgId"
                    name="i-lucide-check"
                    class="size-4 shrink-0"
                    style="color: var(--argus-accent);"
                  />
                </button>
              </div>
            </div>
          </Transition>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  SUPER ADMIN: GLOBAL WEBHOOK   -->
    <!--  DELIVERY HEALTH BANNER        -->
    <!-- ============================== -->
    <div
      v-if="authStore.isSuperAdmin"
      class="glass-card rounded-xl px-5 py-4"
    >
      <div class="flex items-center justify-between gap-4 flex-wrap">
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-9 rounded-lg"
            :style="{
              background: globalWebhookHealth.status === 'healthy' ? successBg(0.1) : globalWebhookHealth.status === 'degraded' ? warningBg(0.1) : errorBg(0.1),
              border: `1px solid ${globalWebhookHealth.status === 'healthy' ? successBg(0.2) : globalWebhookHealth.status === 'degraded' ? warningBg(0.2) : errorBg(0.2)}`
            }"
          >
            <UIcon
              name="i-lucide-activity"
              class="size-5"
              :style="{
                color: globalWebhookHealth.status === 'healthy' ? 'var(--argus-success)' : globalWebhookHealth.status === 'degraded' ? 'var(--argus-warning)' : 'var(--argus-error)'
              }"
            />
          </div>
          <div>
            <div class="flex items-center gap-2">
              <p
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Глобальная доставка вебхуков
              </p>
              <span
                class="text-[9px] font-bold px-2 py-0.5 rounded-full uppercase"
                :style="{
                  background: globalWebhookHealth.status === 'healthy' ? successBg(0.1) : globalWebhookHealth.status === 'degraded' ? warningBg(0.1) : errorBg(0.1),
                  color: globalWebhookHealth.status === 'healthy' ? 'var(--argus-success)' : globalWebhookHealth.status === 'degraded' ? 'var(--argus-warning)' : 'var(--argus-error)'
                }"
              >
                {{ globalWebhookHealth.status === 'healthy' ? 'Здоровый' : globalWebhookHealth.status === 'degraded' ? 'Деградация' : globalWebhookHealth.status === 'critical' ? 'Критический' : 'Неизвестно' }}
              </span>
            </div>
            <p
              class="text-[10px] mt-0.5"
              style="color: var(--argus-text-dimmed);"
            >
              Кросс-организационный мониторинг webhook доставки
            </p>
          </div>
        </div>

        <div class="flex items-center gap-5">
          <div class="text-center">
            <p
              class="text-lg font-bold tabular-nums"
              :style="{
                color: globalWebhookHealth.rate >= 98 ? 'var(--argus-success)' : globalWebhookHealth.rate >= 90 ? 'var(--argus-warning)' : 'var(--argus-error)'
              }"
            >
              {{ globalWebhookHealth.rate }}%
            </p>
            <p
              class="text-[9px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Успех доставки
            </p>
          </div>
          <div
            class="h-8 w-px"
            style="background: var(--argus-border);"
          />
          <div class="text-center">
            <p
              class="text-lg font-bold tabular-nums"
              style="color: var(--argus-text);"
            >
              {{ globalWebhookHealth.totalDeliveries }}
            </p>
            <p
              class="text-[9px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Всего попыток
            </p>
          </div>
          <div
            class="h-8 w-px"
            style="background: var(--argus-border);"
          />
          <div class="text-center">
            <p
              class="text-lg font-bold tabular-nums"
              :style="{ color: globalWebhookHealth.failedCount > 0 ? 'var(--argus-error)' : 'var(--argus-success)' }"
            >
              {{ globalWebhookHealth.failedCount }}
            </p>
            <p
              class="text-[9px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Ошибки
            </p>
          </div>
          <div
            class="h-8 w-px"
            style="background: var(--argus-border);"
          />
          <div class="text-center">
            <p
              class="text-lg font-bold tabular-nums"
              style="color: var(--argus-text);"
            >
              {{ globalWebhookHealth.avgLatency }}мс
            </p>
            <p
              class="text-[9px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Сред. задержка
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  HEADER + SYSTEM STATUS        -->
    <!-- ============================== -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <h1
          class="text-2xl font-bold"
          style="color: var(--argus-text);"
        >
          API и Интеграции
        </h1>
        <p
          class="text-sm mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Управление API-ключами, вебхуками и интеграциями с внешними системами
        </p>
      </div>

      <div class="flex items-center gap-3">
        <!-- System Secure Badge -->
        <div
          class="flex items-center gap-2 px-4 py-2 rounded-lg border"
          :style="{
            background: store.apiSystemStatus === 'operational' ? successBg(0.08) : store.apiSystemStatus === 'degraded' ? warningBg(0.08) : errorBg(0.08),
            borderColor: store.apiSystemStatus === 'operational' ? successBg(0.2) : store.apiSystemStatus === 'degraded' ? warningBg(0.2) : errorBg(0.2)
          }"
        >
          <div
            class="size-2 rounded-full"
            :class="store.apiSystemStatus === 'operational' ? 'animate-pulse' : ''"
            :style="{ background: store.apiSystemStatus === 'operational' ? 'var(--argus-success)' : store.apiSystemStatus === 'degraded' ? 'var(--argus-warning)' : 'var(--argus-error)' }"
          />
          <span
            class="text-xs font-bold"
            :style="{ color: store.apiSystemStatus === 'operational' ? 'var(--argus-success)' : store.apiSystemStatus === 'degraded' ? 'var(--argus-warning)' : 'var(--argus-error)' }"
          >
            {{ store.apiSystemStatus === 'operational' ? 'Система защищена' : store.apiSystemStatus === 'degraded' ? 'Частичная деградация' : 'Система недоступна' }}
          </span>
          <UIcon
            name="i-lucide-shield-check"
            class="size-4"
            :style="{ color: store.apiSystemStatus === 'operational' ? 'var(--argus-success)' : 'var(--argus-warning)' }"
          />
        </div>

        <!-- Quick stats -->
        <div
          class="flex items-center gap-2 px-3 py-2 rounded-lg"
          :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)' }"
        >
          <span
            class="text-[10px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >Ключей:</span>
          <span
            class="text-xs font-bold"
            style="color: var(--argus-text);"
          >{{ activeKeysCount }}</span>
          <span
            class="text-[10px] mx-1"
            style="color: var(--argus-border);"
          >·</span>
          <span
            class="text-[10px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >Хуков:</span>
          <span
            class="text-xs font-bold"
            style="color: var(--argus-text);"
          >{{ activeWebhooksCount }}</span>
          <span
            class="text-[10px] mx-1"
            style="color: var(--argus-border);"
          >·</span>
          <span
            class="text-[10px] font-medium"
            style="color: var(--argus-text-dimmed);"
          >Доставка:</span>
          <span
            class="text-xs font-bold"
            style="color: var(--argus-success);"
          >{{ deliverySuccessRate }}%</span>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  SUPER ADMIN: KEY QUOTAS       -->
    <!-- ============================== -->
    <div
      v-if="authStore.isSuperAdmin"
      class="grid grid-cols-1 md:grid-cols-3 gap-4"
    >
      <div class="glass-card rounded-xl p-5">
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-key-round"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <p
              class="text-2xl font-bold tabular-nums"
              style="color: var(--argus-text);"
            >
              {{ keyQuotas.totalActive }}
            </p>
            <p
              class="text-[10px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Активных ключей{{ selectedOrgFilter ? '' : ' (все орг.)' }}
            </p>
          </div>
        </div>
      </div>
      <div class="glass-card rounded-xl p-5">
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: successBg(0.1), border: `1px solid ${successBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-gauge"
              class="size-5"
              style="color: var(--argus-success);"
            />
          </div>
          <div>
            <p
              class="text-2xl font-bold tabular-nums"
              style="color: var(--argus-text);"
            >
              {{ keyQuotas.totalRps }}
            </p>
            <p
              class="text-[10px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Общий лимит RPS
            </p>
          </div>
        </div>
      </div>
      <div class="glass-card rounded-xl p-5">
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: warningBg(0.1), border: `1px solid ${warningBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-zap"
              class="size-5"
              style="color: var(--argus-warning);"
            />
          </div>
          <div>
            <p
              class="text-2xl font-bold tabular-nums"
              style="color: var(--argus-text);"
            >
              {{ keyQuotas.avgRps }}
            </p>
            <p
              class="text-[10px] font-medium"
              style="color: var(--argus-text-dimmed);"
            >
              Средний RPS / ключ
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  API KEYS MANAGEMENT           -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="flex items-center justify-between px-5 py-4 border-b"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-key-round"
            class="size-4"
            style="color: var(--argus-accent);"
          />
          <h2
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            API Ключи
          </h2>
          <span
            class="text-[10px] font-medium px-2 py-0.5 rounded-full"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ activeKeysCount }} активных
          </span>
          <span
            v-if="backendKeysLoading"
            class="text-[10px] font-medium px-2 py-0.5 rounded-full flex items-center gap-1"
            :style="{ background: warningBg(0.1), color: 'var(--argus-warning)' }"
          >
            <UIcon
              name="i-lucide-loader-2"
              class="size-3 animate-spin"
            />
            Загрузка...
          </span>
        </div>
        <button
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all cursor-pointer"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.2)"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.1)"
          @click="showNewKeyForm = !showNewKeyForm; createdSecretKey = null"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          Создать новый ключ
        </button>
      </div>

      <!-- New key form -->
      <Transition name="modal">
        <div
          v-if="showNewKeyForm"
          class="px-5 py-4 border-b space-y-3"
          style="border-color: var(--argus-border); background: var(--argus-bg-elevated);"
        >
          <!-- Created secret banner -->
          <div
            v-if="createdSecretKey"
            class="flex items-start gap-3 px-4 py-3 rounded-lg border"
            :style="{ background: successBg(0.06), borderColor: successBg(0.2) }"
          >
            <UIcon
              name="i-lucide-shield-alert"
              class="size-5 shrink-0 mt-0.5"
              style="color: var(--argus-success);"
            />
            <div class="flex-1 min-w-0">
              <p
                class="text-xs font-bold"
                style="color: var(--argus-success);"
              >
                Ключ успешно создан!
              </p>
              <p
                class="text-[10px] mt-0.5"
                style="color: var(--argus-text-dimmed);"
              >
                Скопируйте секрет сейчас. Он больше не будет показан.
              </p>
              <div class="flex items-center gap-2 mt-2">
                <code
                  class="text-xs font-mono px-3 py-1.5 rounded-md flex-1 break-all"
                  :style="{ background: 'var(--argus-bg-card)', color: 'var(--argus-text)', border: '1px solid var(--argus-border)' }"
                >
                  {{ createdSecretKey }}
                </code>
                <button
                  class="flex items-center gap-1 px-3 py-1.5 rounded-md text-xs font-bold transition-all cursor-pointer shrink-0"
                  :style="{ background: 'var(--argus-accent)', color: '#fff' }"
                  @click="copyToClipboard(createdSecretKey!)"
                >
                  <UIcon
                    name="i-lucide-copy"
                    class="size-3"
                  />
                  Копировать
                </button>
              </div>
            </div>
            <button
              class="flex items-center justify-center size-7 rounded-md transition-all cursor-pointer shrink-0"
              style="color: var(--argus-text-dimmed);"
              @click="createdSecretKey = null"
            >
              <UIcon
                name="i-lucide-x"
                class="size-3.5"
              />
            </button>
          </div>

          <div class="flex items-center gap-3 flex-wrap">
            <!-- Key name -->
            <div
              class="flex items-center gap-2.5 px-3.5 py-2.5 rounded-lg border flex-1 min-w-[200px]"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <UIcon
                name="i-lucide-tag"
                class="size-4 shrink-0"
                style="color: var(--argus-text-dimmed);"
              />
              <input
                v-model="newKeyName"
                type="text"
                placeholder="Название ключа (напр.: Production API)"
                class="w-full bg-transparent text-sm outline-none"
                :style="{ color: 'var(--argus-text)' }"
                @keyup.enter="generateNewKey"
              >
            </div>

            <!-- Super Admin: Org selector for new key -->
            <div
              v-if="authStore.isSuperAdmin"
              class="flex items-center gap-2.5 px-3.5 py-2.5 rounded-lg border min-w-[200px]"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <UIcon
                name="i-lucide-building"
                class="size-4 shrink-0"
                style="color: var(--argus-text-dimmed);"
              />
              <select
                v-model="newKeyOrgId"
                class="w-full bg-transparent text-sm outline-none cursor-pointer appearance-none"
                :style="{ color: 'var(--argus-text)' }"
              >
                <option
                  :value="null"
                  disabled
                >
                  Организация
                </option>
                <option
                  v-for="org in orgs"
                  :key="org.orgId"
                  :value="org.orgId"
                >
                  {{ org.name }}
                </option>
              </select>
            </div>

            <!-- Environment selector -->
            <div class="flex items-center gap-1.5">
              <button
                class="px-3 py-2.5 rounded-lg text-xs font-medium border transition-all cursor-pointer"
                :style="{
                  background: newKeyEnvironment === 'live' ? accentBg(0.1) : 'transparent',
                  borderColor: newKeyEnvironment === 'live' ? accentBg(0.3) : 'var(--argus-border)',
                  color: newKeyEnvironment === 'live' ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
                }"
                @click="newKeyEnvironment = 'live'"
              >
                Live
              </button>
              <button
                class="px-3 py-2.5 rounded-lg text-xs font-medium border transition-all cursor-pointer"
                :style="{
                  background: newKeyEnvironment === 'test' ? warningBg(0.1) : 'transparent',
                  borderColor: newKeyEnvironment === 'test' ? warningBg(0.3) : 'var(--argus-border)',
                  color: newKeyEnvironment === 'test' ? 'var(--argus-warning)' : 'var(--argus-text-dimmed)'
                }"
                @click="newKeyEnvironment = 'test'"
              >
                Test
              </button>
            </div>
          </div>

          <!-- Permissions selector -->
          <div class="flex items-center gap-2 flex-wrap">
            <span
              class="text-xs font-medium"
              style="color: var(--argus-text-dimmed);"
            >Разрешения:</span>
            <button
              v-for="perm in availablePermissions"
              :key="perm"
              class="flex items-center gap-1 px-2 py-1 rounded-md text-[10px] font-medium transition-all border cursor-pointer"
              :style="{
                background: newKeyPermissions.includes(perm) ? accentBg(0.1) : 'transparent',
                borderColor: newKeyPermissions.includes(perm) ? accentBg(0.3) : 'var(--argus-border)',
                color: newKeyPermissions.includes(perm) ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
              }"
              @click="newKeyPermissions.includes(perm)
                ? newKeyPermissions.splice(newKeyPermissions.indexOf(perm), 1)
                : newKeyPermissions.push(perm)"
            >
              <UIcon
                :name="newKeyPermissions.includes(perm) ? 'i-lucide-check' : 'i-lucide-circle'"
                class="size-2.5"
              />
              {{ perm }}
            </button>
          </div>

          <!-- Actions -->
          <div class="flex items-center gap-2">
            <button
              class="flex items-center gap-1.5 px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
              :style="{ background: 'var(--argus-accent)', color: '#fff', opacity: createKeyLoading ? 0.7 : 1 }"
              :disabled="createKeyLoading"
              @click="generateNewKey"
            >
              <UIcon
                :name="createKeyLoading ? 'i-lucide-loader-2' : 'i-lucide-plus-circle'"
                class="size-3.5"
                :class="createKeyLoading ? 'animate-spin' : ''"
              />
              {{ createKeyLoading ? 'Генерация...' : 'Сгенерировать' }}
            </button>
            <button
              class="flex items-center justify-center size-9 rounded-lg transition-all cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
              @click="closeNewKeyForm"
            >
              <UIcon
                name="i-lucide-x"
                class="size-4"
              />
            </button>
          </div>
        </div>
      </Transition>

      <!-- Keys table -->
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th
                class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Название
              </th>
              <th
                v-if="authStore.isSuperAdmin"
                class="text-left px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Организация
              </th>
              <th
                class="text-left px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Ключ
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Создан
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Посл. использование
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Статус
              </th>
              <th
                class="text-right px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Действия
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="key in filteredKeys"
              :key="key.id"
              class="transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <td class="px-5 py-3.5">
                <div>
                  <p
                    class="text-sm font-medium"
                    style="color: var(--argus-text);"
                  >
                    {{ key.name }}
                  </p>
                  <div class="flex items-center gap-1 mt-0.5">
                    <span
                      v-for="perm in key.permissions"
                      :key="perm"
                      class="text-[8px] font-medium px-1.5 py-0.5 rounded"
                      :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-dimmed)' }"
                    >
                      {{ perm }}
                    </span>
                  </div>
                </div>
              </td>
              <!-- Super Admin: Org column -->
              <td
                v-if="authStore.isSuperAdmin"
                class="px-4 py-3.5"
              >
                <div class="flex items-center gap-1.5">
                  <UIcon
                    name="i-lucide-building"
                    class="size-3 shrink-0"
                    style="color: var(--argus-text-dimmed);"
                  />
                  <span
                    class="text-xs font-medium truncate max-w-[140px]"
                    style="color: var(--argus-text-muted);"
                  >
                    {{ orgName(key.orgId) }}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3.5">
                <div class="flex items-center gap-2">
                  <code
                    class="text-xs font-mono px-2 py-1 rounded"
                    :style="{ background: 'var(--argus-bg-hover)', color: 'var(--argus-text-muted)' }"
                  >
                    {{ revealedKeyId === key.id ? key.key : maskKey(key.key) }}
                  </code>
                  <button
                    class="flex items-center justify-center size-6 rounded-md transition-all cursor-pointer"
                    :style="{ color: 'var(--argus-text-dimmed)' }"
                    :title="revealedKeyId === key.id ? 'Скрыть' : 'Показать'"
                    @mouseenter="($event.currentTarget as HTMLElement).style.color = 'var(--argus-accent)'"
                    @mouseleave="($event.currentTarget as HTMLElement).style.color = 'var(--argus-text-dimmed)'"
                    @click="revealedKeyId = revealedKeyId === key.id ? null : key.id"
                  >
                    <UIcon
                      :name="revealedKeyId === key.id ? 'i-lucide-eye-off' : 'i-lucide-eye'"
                      class="size-3.5"
                    />
                  </button>
                </div>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span
                  class="text-xs"
                  style="color: var(--argus-text-muted);"
                >{{ formatDate(key.created) }}</span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span
                  class="text-xs font-mono"
                  style="color: var(--argus-text-muted);"
                >{{ formatDatetime(key.lastUsed) }}</span>
              </td>
              <td class="px-4 py-3.5 text-center">
                <span
                  class="inline-flex items-center gap-1 text-[10px] font-bold px-2 py-1 rounded-full"
                  :style="{
                    background: key.status === 'active' ? successBg(0.1) : errorBg(0.1),
                    color: key.status === 'active' ? 'var(--argus-success)' : 'var(--argus-error)'
                  }"
                >
                  <div
                    class="size-1.5 rounded-full"
                    :style="{ background: key.status === 'active' ? 'var(--argus-success)' : 'var(--argus-error)' }"
                  />
                  {{ key.status === 'active' ? 'Активен' : 'Отозван' }}
                </span>
              </td>
              <td class="px-5 py-3.5 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <button
                    class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all cursor-pointer"
                    :style="{
                      background: copiedKeyId === key.id ? successBg(0.1) : accentBg(0.08),
                      color: copiedKeyId === key.id ? 'var(--argus-success)' : 'var(--argus-accent)'
                    }"
                    @click="copyKey(key.key, key.id)"
                  >
                    <UIcon
                      :name="copiedKeyId === key.id ? 'i-lucide-check' : 'i-lucide-copy'"
                      class="size-3"
                    />
                    {{ copiedKeyId === key.id ? 'Скопировано!' : 'Копировать' }}
                  </button>
                  <button
                    v-if="key.status === 'active'"
                    class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all cursor-pointer"
                    :style="{ background: errorBg(0.08), color: 'var(--argus-error)' }"
                    @mouseenter="($event.currentTarget as HTMLElement).style.background = errorBg(0.15)"
                    @mouseleave="($event.currentTarget as HTMLElement).style.background = errorBg(0.08)"
                    @click="revokeKey(key.id)"
                  >
                    <UIcon
                      name="i-lucide-ban"
                      class="size-3"
                    />
                    Отозвать
                  </button>
                </div>
              </td>
            </tr>

            <!-- Empty state -->
            <tr v-if="filteredKeys.length === 0 && !backendKeysLoading">
              <td
                :colspan="authStore.isSuperAdmin ? 7 : 6"
                class="px-5 py-8 text-center"
              >
                <div class="flex flex-col items-center gap-2">
                  <UIcon
                    name="i-lucide-key-round"
                    class="size-8"
                    style="color: var(--argus-text-dimmed); opacity: 0.4;"
                  />
                  <p
                    class="text-sm"
                    style="color: var(--argus-text-dimmed);"
                  >
                    {{ selectedOrgFilter ? 'Нет API-ключей для выбранной организации' : 'API-ключи не найдены' }}
                  </p>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  WEBHOOKS SETUP                -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="flex items-center justify-between px-5 py-4 border-b"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-webhook"
            class="size-4"
            style="color: var(--argus-accent);"
          />
          <h2
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Вебхуки
          </h2>
          <span
            class="text-[10px] font-medium px-2 py-0.5 rounded-full"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            {{ activeWebhooksCount }} активных
          </span>
        </div>
        <button
          class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-semibold transition-all cursor-pointer"
          :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = accentBg(0.2)"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = accentBg(0.1)"
          @click="showWebhookForm = !showWebhookForm"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          Добавить вебхук
        </button>
      </div>

      <!-- New webhook form -->
      <Transition name="modal">
        <div
          v-if="showWebhookForm"
          class="px-5 py-4 border-b space-y-3"
          style="border-color: var(--argus-border); background: var(--argus-bg-elevated);"
        >
          <div
            class="flex items-center gap-2.5 px-3.5 py-2.5 rounded-lg border"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <UIcon
              name="i-lucide-link"
              class="size-4 shrink-0"
              style="color: var(--argus-text-dimmed);"
            />
            <input
              v-model="newWebhookUrl"
              type="url"
              placeholder="https://your-domain.com/webhook/endpoint"
              class="w-full bg-transparent text-sm outline-none font-mono"
              :style="{ color: 'var(--argus-text)' }"
            >
          </div>

          <div class="flex items-center gap-2 flex-wrap">
            <span
              class="text-xs font-medium"
              style="color: var(--argus-text-dimmed);"
            >Подписка на события:</span>
            <button
              v-for="event in availableEvents"
              :key="event.value"
              class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all border cursor-pointer"
              :style="{
                background: selectedWebhookEvents.includes(event.value) ? accentBg(0.1) : 'transparent',
                borderColor: selectedWebhookEvents.includes(event.value) ? accentBg(0.3) : 'var(--argus-border)',
                color: selectedWebhookEvents.includes(event.value) ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
              }"
              @click="selectedWebhookEvents.includes(event.value)
                ? selectedWebhookEvents.splice(selectedWebhookEvents.indexOf(event.value), 1)
                : selectedWebhookEvents.push(event.value)"
            >
              <UIcon
                :name="selectedWebhookEvents.includes(event.value) ? 'i-lucide-check' : 'i-lucide-circle'"
                class="size-3"
              />
              {{ event.label }}
            </button>
          </div>

          <div class="flex items-center gap-2">
            <button
              class="flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-bold transition-all cursor-pointer"
              :style="{ background: 'var(--argus-accent)', color: '#fff' }"
              @click="addWebhook"
            >
              <UIcon
                name="i-lucide-check"
                class="size-3.5"
              />
              Сохранить
            </button>
            <button
              class="px-4 py-2 rounded-lg text-xs font-medium transition-all cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @click="showWebhookForm = false"
            >
              Отмена
            </button>
          </div>
        </div>
      </Transition>

      <!-- Webhooks list -->
      <div
        class="divide-y"
        style="border-color: var(--argus-border-subtle);"
      >
        <div
          v-for="wh in store.webhooks"
          :key="wh.id"
          class="px-5 py-4 transition-colors"
          @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
          @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
        >
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <code
                  class="text-xs font-mono truncate max-w-lg"
                  style="color: var(--argus-text);"
                >{{ wh.url }}</code>
                <span
                  class="text-[9px] font-bold px-1.5 py-0.5 rounded-full"
                  :style="{
                    background: wh.status === 'active' ? successBg(0.1) : warningBg(0.1),
                    color: wh.status === 'active' ? 'var(--argus-success)' : 'var(--argus-warning)'
                  }"
                >
                  {{ wh.status === 'active' ? 'Активен' : 'Приостановлен' }}
                </span>
              </div>
              <div class="flex items-center gap-2 mt-2 flex-wrap">
                <span
                  v-for="ev in wh.events"
                  :key="ev"
                  class="text-[9px] font-medium px-2 py-0.5 rounded"
                  :style="{ background: accentBg(0.06), color: 'var(--argus-accent)' }"
                >
                  {{ eventLabel(ev) }}
                </span>
              </div>
              <div class="flex items-center gap-4 mt-2">
                <span
                  class="text-[10px]"
                  style="color: var(--argus-text-dimmed);"
                >Последняя доставка: {{ formatDatetime(wh.lastDelivery) }}</span>
                <span
                  class="text-[10px] font-bold"
                  :style="{ color: wh.successRate >= 95 ? 'var(--argus-success)' : wh.successRate >= 85 ? 'var(--argus-warning)' : 'var(--argus-error)' }"
                >
                  {{ wh.successRate }}% успешных
                </span>
              </div>
            </div>

            <div class="flex items-center gap-1.5 shrink-0">
              <button
                class="flex items-center gap-1 px-2.5 py-1.5 rounded-md text-[10px] font-medium transition-all border cursor-pointer"
                :style="{
                  borderColor: wh.status === 'active' ? warningBg(0.3) : successBg(0.3),
                  color: wh.status === 'active' ? 'var(--argus-warning)' : 'var(--argus-success)'
                }"
                @click="toggleWebhookStatus(wh.id)"
              >
                <UIcon
                  :name="wh.status === 'active' ? 'i-lucide-pause' : 'i-lucide-play'"
                  class="size-3"
                />
                {{ wh.status === 'active' ? 'Пауза' : 'Возобновить' }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  DELIVERY LOGS (API Status)    -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="flex items-center justify-between px-5 py-4 border-b"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-activity"
            class="size-4"
            style="color: var(--argus-text-dimmed);"
          />
          <h2
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Статус API — Последние доставки
          </h2>
        </div>
        <span
          class="text-[10px] font-medium px-2 py-0.5 rounded-full"
          :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
        >
          {{ deliverySuccessRate }}% доставка
        </span>
      </div>

      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--argus-border);">
              <th
                class="text-left px-5 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Событие
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Статус
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Код
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Время
              </th>
              <th
                class="text-center px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Задержка
              </th>
              <th
                class="text-left px-4 py-3 text-[11px] font-medium uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Получатель
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="log in store.webhookDeliveryLogs"
              :key="log.id"
              class="transition-colors"
              style="border-bottom: 1px solid var(--argus-border-subtle);"
              @mouseenter="($event.currentTarget as HTMLElement).style.background = 'var(--argus-bg-hover)'"
              @mouseleave="($event.currentTarget as HTMLElement).style.background = 'transparent'"
            >
              <td class="px-5 py-3">
                <div class="flex items-center gap-2">
                  <UIcon
                    :name="log.event === 'violation.detected' ? 'i-lucide-alert-triangle' : log.event === 'session.ended' ? 'i-lucide-log-out' : 'i-lucide-shield-check'"
                    class="size-3.5"
                    style="color: var(--argus-text-dimmed);"
                  />
                  <span
                    class="text-xs font-medium"
                    style="color: var(--argus-text);"
                  >{{ eventLabel(log.event) }}</span>
                </div>
              </td>
              <td class="px-4 py-3 text-center">
                <span
                  class="inline-flex items-center gap-1 text-[10px] font-bold px-2 py-0.5 rounded-full"
                  :style="{
                    background: log.status === 'success' ? successBg(0.1) : errorBg(0.1),
                    color: log.status === 'success' ? 'var(--argus-success)' : 'var(--argus-error)'
                  }"
                >
                  <UIcon
                    :name="log.status === 'success' ? 'i-lucide-check' : 'i-lucide-x'"
                    class="size-2.5"
                  />
                  {{ log.status === 'success' ? 'Успешно' : 'Ошибка' }}
                </span>
              </td>
              <td class="px-4 py-3 text-center">
                <span
                  class="text-xs font-mono font-bold tabular-nums"
                  :style="{ color: log.responseCode === 200 ? 'var(--argus-success)' : 'var(--argus-error)' }"
                >
                  {{ log.responseCode }}
                </span>
              </td>
              <td class="px-4 py-3 text-center">
                <span
                  class="text-xs font-mono"
                  style="color: var(--argus-text-muted);"
                >{{ formatDatetime(log.timestamp) }}</span>
              </td>
              <td class="px-4 py-3 text-center">
                <span
                  class="text-xs font-mono tabular-nums"
                  :style="{ color: log.duration > 1000 ? 'var(--argus-error)' : log.duration > 200 ? 'var(--argus-warning)' : 'var(--argus-text-muted)' }"
                >
                  {{ log.duration }}мс
                </span>
              </td>
              <td class="px-4 py-3">
                <span
                  class="text-[10px] font-mono truncate max-w-44 block"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ store.webhooks.find(w => w.id === log.webhookId)?.url || '\u2014' }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  LMS INTEGRATIONS              -->
    <!-- ============================== -->
    <div class="glass-card rounded-xl overflow-hidden">
      <div
        class="flex items-center justify-between px-5 py-4 border-b"
        style="border-color: var(--argus-border);"
      >
        <div class="flex items-center gap-2">
          <UIcon
            name="i-lucide-puzzle"
            class="size-4"
            style="color: var(--argus-accent);"
          />
          <h2
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Интеграции с LMS
          </h2>
        </div>
        <a
          class="text-[10px] font-medium px-2 py-1 rounded-full cursor-pointer"
          :style="{ background: accentBg(0.06), color: 'var(--argus-accent)' }"
        >
          Документация →
        </a>
      </div>

      <div
        class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-px"
        style="background: var(--argus-border-subtle);"
      >
        <div
          v-for="lms in lmsIntegrations"
          :key="lms.id"
          class="p-5"
          style="background: var(--argus-card-bg);"
        >
          <div class="flex items-start gap-3">
            <div
              class="flex items-center justify-center size-10 rounded-lg shrink-0"
              :style="{
                background: lms.status === 'connected' ? successBg(0.1) : 'var(--argus-bg-hover)',
                border: `1px solid ${lms.status === 'connected' ? successBg(0.2) : 'var(--argus-border)'}`
              }"
            >
              <UIcon
                :name="lms.icon"
                class="size-5"
                :style="{ color: lms.status === 'connected' ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
              />
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <p
                  class="text-sm font-semibold"
                  style="color: var(--argus-text);"
                >
                  {{ lms.name }}
                </p>
                <span
                  v-if="lms.status === 'connected'"
                  class="text-[8px] font-bold px-1.5 py-0.5 rounded-full"
                  :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
                >
                  Подключено
                </span>
              </div>
              <p
                class="text-[10px] mt-0.5"
                style="color: var(--argus-text-dimmed);"
              >
                {{ lms.description }}
              </p>
            </div>
          </div>

          <button
            class="w-full mt-4 flex items-center justify-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium transition-all border cursor-pointer"
            :style="{
              borderColor: lms.status === 'connected' ? successBg(0.2) : 'var(--argus-border)',
              color: lms.status === 'connected' ? 'var(--argus-success)' : 'var(--argus-accent)',
              background: lms.status === 'connected' ? successBg(0.05) : 'transparent'
            }"
            @mouseenter="($event.currentTarget as HTMLElement).style.background = lms.status === 'connected' ? successBg(0.1) : 'var(--argus-bg-hover)'"
            @mouseleave="($event.currentTarget as HTMLElement).style.background = lms.status === 'connected' ? successBg(0.05) : 'transparent'"
            @click="handleLMSClick(lms)"
          >
            <UIcon
              :name="lms.status === 'connected' ? 'i-lucide-settings' : 'i-lucide-plug'"
              class="size-3.5"
            />
            {{ lms.status === 'connected' ? 'Настроить' : 'Подключить' }}
          </button>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  DOCUMENTATION LINKS           -->
    <!-- ============================== -->
    <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
      <div
        class="glass-card rounded-xl p-5 cursor-pointer video-card-hover"
        @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-accent)'"
        @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-border)'"
        @click="navigateTo('/integrations')"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-book-open"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <p
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Документация API
            </p>
            <p
              class="text-[10px]"
              style="color: var(--argus-text-dimmed);"
            >
              REST API, WebSocket, Batch операции
            </p>
          </div>
        </div>
      </div>

      <div
        class="glass-card rounded-xl p-5 cursor-pointer video-card-hover"
        @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-accent)'"
        @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-border)'"
        @click="navigateTo('/integrations')"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-code-xml"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <p
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Примеры кода
            </p>
            <p
              class="text-[10px]"
              style="color: var(--argus-text-dimmed);"
            >
              Python, Node.js, PHP SDK
            </p>
          </div>
        </div>
      </div>

      <div
        class="glass-card rounded-xl p-5 cursor-pointer video-card-hover"
        @mouseenter="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-accent)'"
        @mouseleave="($event.currentTarget as HTMLElement).style.borderColor = 'var(--argus-border)'"
        @click="useToast().add({ title: 'Поддержка', description: 'Обратитесь по email: support@argus.ai', color: 'info', duration: 4000 })"
      >
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-message-square-text"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <p
              class="text-sm font-semibold"
              style="color: var(--argus-text);"
            >
              Поддержка
            </p>
            <p
              class="text-[10px]"
              style="color: var(--argus-text-dimmed);"
            >
              Чат с разработчиками, статус-страница
            </p>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
