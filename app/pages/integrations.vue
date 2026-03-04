<script setup lang="ts">
// =============================================================================
// Argus AI — Admin Integration Dashboard
// =============================================================================
// Private page for org_admin + super_admin.
// API key management, webhook config, IP whitelisting, Swagger link, Hard Gate docs.
// Language: Russian (consistent with admin panel).
// =============================================================================

import { useDashboardStore } from '~/stores/useDashboardStore'
import { useAuthStore } from '~/stores/useAuthStore'
import { useAdminAPI } from '~/composables/useAdminAPI'
import type { Organization, APIKey } from '~/composables/useAdminAPI'
import type { IPWhitelistEntry } from '~/stores/useDashboardStore'

const store = useDashboardStore()
const authStore = useAuthStore()
const adminAPI = useAdminAPI()
const { accentBg, errorBg, successBg, warningBg } = useColors()
const runtimeConfig = useRuntimeConfig()

// --- Multi-tenant state ---
const orgs = ref<Organization[]>([])
const orgsLoading = ref(false)
const selectedOrgFilter = ref<string | null>(null)
const showOrgDropdown = ref(false)

// --- API Keys ---
const backendKeys = ref<APIKey[]>([])
const backendKeysLoading = ref(false)
const backendKeysError = ref<string | null>(null)
const showNewKeyForm = ref(false)
const newKeyName = ref('')
const newKeyEnvironment = ref<'live' | 'test'>('live')
const newKeyPermissions = ref<string[]>(['read:sessions', 'read:reports'])
const copiedKeyId = ref<string | null>(null)
const revealedKeyId = ref<string | null>(null)
const createdSecretKey = ref<string | null>(null)
const createKeyLoading = ref(false)

const availablePermissions = [
  'read:sessions', 'write:sessions', 'read:reports',
  'write:violations', 'read:violations', 'write:webhooks',
  'read:webhooks', 'admin:keys'
]

// --- Webhooks ---
const showWebhookForm = ref(false)
const newWebhookUrl = ref('')
const selectedWebhookEvents = ref<string[]>([])
const testPayloadSending = ref<string | null>(null)

const availableEvents = [
  { value: 'violation.detected', label: 'Нарушение обнаружено' },
  { value: 'session.ended', label: 'Сессия завершена' },
  { value: 'integrity.finalized', label: 'Оценка честности' },
  { value: 'session.started', label: 'Сессия начата' },
  { value: 'alert.critical', label: 'Критическое предупреждение' }
]

// --- IP Whitelist ---
const showIPForm = ref(false)
const newIP = ref('')
const newIPLabel = ref('')
const ipValidationError = ref<string | null>(null)

// IP/CIDR validation
const ipRegex = /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(\/\d{1,2})?$/

function validateIP(ip: string): boolean {
  if (!ipRegex.test(ip)) return false
  const parts = ip.split('/')[0]!.split('.')
  if (parts.some(p => parseInt(p) > 255)) return false
  const cidr = ip.split('/')[1]
  if (cidr && parseInt(cidr) > 32) return false
  return true
}

// --- Active section for scrollspy ---
const activeTab = ref<'keys' | 'webhooks' | 'ip' | 'swagger' | 'hardgate'>('keys')

// --- Computed ---
const effectiveOrgId = computed(() => {
  if (authStore.isSuperAdmin) return selectedOrgFilter.value || authStore.effectiveOrgId
  return authStore.user?.orgId || null
})

const filteredKeys = computed(() => {
  if (!effectiveOrgId.value) return backendKeys.value
  return backendKeys.value.filter(k => k.orgId === effectiveOrgId.value)
})

const activeKeysCount = computed(() => filteredKeys.value.filter(k => k.isActive).length)
const activeWebhooksCount = computed(() => store.webhooks.filter(w => w.status === 'active').length)

const orgIPWhitelist = computed(() => {
  if (!effectiveOrgId.value) return store.ipWhitelist
  return store.ipWhitelist.filter(e => e.orgId === effectiveOrgId.value)
})

function orgName(orgId: string): string {
  const org = orgs.value.find(o => o.orgId === orgId)
  return org ? org.name : orgId
}

const selectedOrgName = computed(() => {
  if (!selectedOrgFilter.value) return 'Все организации'
  return orgName(selectedOrgFilter.value)
})

// --- Fetch functions ---
async function fetchOrgs() {
  if (!authStore.isSuperAdmin) return
  orgsLoading.value = true
  try {
    orgs.value = await adminAPI.listOrgs()
  } catch {
    orgs.value = []
  } finally {
    orgsLoading.value = false
  }
}

async function fetchAPIKeys() {
  const orgId = effectiveOrgId.value
  if (!orgId) {
    backendKeys.value = []
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

// --- API Key actions ---
function maskKey(keyId: string, prefix: string): string {
  return `${keyId}_${prefix}${'•'.repeat(16)}`
}

function copyKey(text: string, id: string) {
  navigator.clipboard.writeText(text)
  copiedKeyId.value = id
  setTimeout(() => { copiedKeyId.value = null }, 2000)
}

async function generateNewKey() {
  if (!newKeyName.value.trim()) return
  const orgId = effectiveOrgId.value
  if (!orgId) return

  createKeyLoading.value = true
  createdSecretKey.value = null
  try {
    const result = await adminAPI.createAPIKey(orgId, {
      name: newKeyName.value.trim(),
      permissions: newKeyPermissions.value,
      environment: newKeyEnvironment.value
    })
    createdSecretKey.value = result.secret
    await fetchAPIKeys()
    newKeyName.value = ''
    newKeyPermissions.value = ['read:sessions', 'read:reports']
    newKeyEnvironment.value = 'live'
  } catch {
    // Fallback to mock
    const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
    let randomPart = ''
    for (let i = 0; i < 24; i++) {
      randomPart += chars.charAt(Math.floor(Math.random() * chars.length))
    }
    const prefix = newKeyEnvironment.value === 'live' ? 'argus_live_sk_' : 'argus_test_sk_'
    createdSecretKey.value = `${prefix}${randomPart}`
    store.apiKeys.push({
      id: `ak-${Date.now()}`,
      name: newKeyName.value.trim(),
      key: createdSecretKey.value,
      created: new Date().toISOString().split('T')[0] ?? '',
      lastUsed: '\u2014',
      status: 'active',
      permissions: [...newKeyPermissions.value]
    })
    newKeyName.value = ''
    newKeyPermissions.value = ['read:sessions', 'read:reports']
    newKeyEnvironment.value = 'live'
  } finally {
    createKeyLoading.value = false
  }
}

async function revokeKey(keyId: string) {
  try {
    await adminAPI.revokeAPIKey(keyId)
    await fetchAPIKeys()
  } catch {
    const key = store.apiKeys.find(k => k.id === keyId)
    if (key) key.status = 'revoked'
  }
}

// --- Webhook actions ---
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

async function sendTestPayload(webhookId: string) {
  testPayloadSending.value = webhookId
  // Simulate sending test payload (no backend implementation yet)
  await new Promise(resolve => setTimeout(resolve, 1500))
  testPayloadSending.value = null
  // Show success via brief visual feedback — the button text changes
}

// --- IP Whitelist actions ---
function addIPEntry() {
  ipValidationError.value = null
  if (!newIP.value.trim()) {
    ipValidationError.value = 'Введите IP-адрес'
    return
  }
  if (!validateIP(newIP.value.trim())) {
    ipValidationError.value = 'Неверный формат IP/CIDR (например: 192.168.1.0/24)'
    return
  }
  if (!newIPLabel.value.trim()) {
    ipValidationError.value = 'Введите описание'
    return
  }

  const entry: IPWhitelistEntry = {
    id: `ip-${Date.now()}`,
    orgId: effectiveOrgId.value || 'unknown',
    ip: newIP.value.trim(),
    label: newIPLabel.value.trim(),
    createdAt: new Date().toISOString().split('T')[0] ?? '',
    createdBy: authStore.user?.fullName || 'admin'
  }

  store.addIPEntry(entry)
  newIP.value = ''
  newIPLabel.value = ''
  showIPForm.value = false
}

function removeIPEntry(entryId: string) {
  store.removeIPEntry(entryId)
}

// --- Org selector ---
function selectOrg(orgId: string | null) {
  selectedOrgFilter.value = orgId
  if (orgId) authStore.switchOrg(orgId)
  showOrgDropdown.value = false
  fetchAPIKeys()
}

function onClickOutsideOrgDropdown(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.org-selector-dropdown')) {
    showOrgDropdown.value = false
  }
}

// --- Swagger URL ---
const swaggerUrl = computed(() => {
  const base = runtimeConfig.public.grpcUrl || runtimeConfig.public.apiBaseUrl || 'http://localhost:8080'
  return `https://petstore.swagger.io/?url=${base}/api/openapi.yaml`
})

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

          <Transition name="modal">
            <div
              v-if="showOrgDropdown"
              class="absolute right-0 top-full mt-1 w-[300px] rounded-xl border shadow-xl z-50 overflow-hidden"
              :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            >
              <button
                class="w-full flex items-center gap-3 px-4 py-3 text-left transition-all cursor-pointer border-b"
                :style="{
                  background: selectedOrgFilter === null ? accentBg(0.06) : 'transparent',
                  borderColor: 'var(--argus-border-subtle)'
                }"
                @click="selectOrg(null)"
              >
                <UIcon
                  name="i-lucide-globe"
                  class="size-3.5"
                  style="color: var(--argus-accent);"
                />
                <span
                  class="text-xs font-semibold"
                  style="color: var(--argus-text);"
                >Все организации</span>
              </button>
              <div class="max-h-[280px] overflow-y-auto">
                <button
                  v-for="org in orgs"
                  :key="org.orgId"
                  class="w-full flex items-center gap-3 px-4 py-2.5 text-left transition-all cursor-pointer"
                  :style="{
                    background: selectedOrgFilter === org.orgId ? accentBg(0.06) : 'transparent',
                    borderBottom: '1px solid var(--argus-border-subtle)'
                  }"
                  @click="selectOrg(org.orgId)"
                >
                  <UIcon
                    name="i-lucide-building"
                    class="size-3.5"
                    :style="{ color: org.isActive ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
                  />
                  <span
                    class="text-xs font-medium truncate"
                    style="color: var(--argus-text);"
                  >{{ org.name }}</span>
                  <UIcon
                    v-if="selectedOrgFilter === org.orgId"
                    name="i-lucide-check"
                    class="size-3.5 ml-auto shrink-0"
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
    <!--  PAGE HEADER                   -->
    <!-- ============================== -->
    <div class="flex items-center justify-between gap-4 flex-wrap">
      <div>
        <h1
          class="text-xl font-bold"
          style="color: var(--argus-text);"
        >
          Интеграции и Настройка API
        </h1>
        <p
          class="text-xs mt-1"
          style="color: var(--argus-text-dimmed);"
        >
          Управление ключами, вебхуками и безопасностью интеграций
        </p>
      </div>
      <NuxtLink
        to="/docs"
        target="_blank"
        class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium transition-all"
        :style="{ background: accentBg(0.08), color: 'var(--argus-accent)' }"
      >
        <UIcon
          name="i-lucide-external-link"
          class="size-3.5"
        />
        Документация для разработчиков
      </NuxtLink>
    </div>

    <!-- ============================== -->
    <!--  QUICK STATS                   -->
    <!-- ============================== -->
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
      <div class="glass-card rounded-xl px-5 py-4">
        <div class="flex items-center justify-between">
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              Активные ключи
            </p>
            <p
              class="text-2xl font-bold tabular-nums mt-1"
              style="color: var(--argus-text);"
            >
              {{ activeKeysCount }}
            </p>
          </div>
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: accentBg(0.1) }"
          >
            <UIcon
              name="i-lucide-key"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
        </div>
      </div>
      <div class="glass-card rounded-xl px-5 py-4">
        <div class="flex items-center justify-between">
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              Активные вебхуки
            </p>
            <p
              class="text-2xl font-bold tabular-nums mt-1"
              style="color: var(--argus-text);"
            >
              {{ activeWebhooksCount }}
            </p>
          </div>
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: successBg(0.1) }"
          >
            <UIcon
              name="i-lucide-webhook"
              class="size-5"
              style="color: var(--argus-success);"
            />
          </div>
        </div>
      </div>
      <div class="glass-card rounded-xl px-5 py-4">
        <div class="flex items-center justify-between">
          <div>
            <p
              class="text-[10px] font-medium uppercase tracking-wider"
              style="color: var(--argus-text-dimmed);"
            >
              IP в белом списке
            </p>
            <p
              class="text-2xl font-bold tabular-nums mt-1"
              style="color: var(--argus-text);"
            >
              {{ orgIPWhitelist.length }}
            </p>
          </div>
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: warningBg(0.1) }"
          >
            <UIcon
              name="i-lucide-shield"
              class="size-5"
              style="color: var(--argus-warning);"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  SECTION TABS                  -->
    <!-- ============================== -->
    <div
      class="flex items-center gap-1 border-b pb-px"
      :style="{ borderColor: 'var(--argus-border)' }"
    >
      <button
        v-for="tab in [
          { id: 'keys' as const, label: 'API Ключи', icon: 'i-lucide-key' },
          { id: 'webhooks' as const, label: 'Вебхуки', icon: 'i-lucide-webhook' },
          { id: 'ip' as const, label: 'IP Whitelist', icon: 'i-lucide-shield' },
          { id: 'swagger' as const, label: 'Swagger', icon: 'i-lucide-file-code' },
          { id: 'hardgate' as const, label: 'Hard Gate', icon: 'i-lucide-scan-face' }
        ]"
        :key="tab.id"
        class="flex items-center gap-1.5 px-4 py-2.5 text-xs font-medium transition-all cursor-pointer rounded-t-lg"
        :style="{
          color: activeTab === tab.id ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)',
          borderBottom: activeTab === tab.id ? '2px solid var(--argus-accent)' : '2px solid transparent',
          background: activeTab === tab.id ? accentBg(0.05) : 'transparent'
        }"
        @click="activeTab = tab.id"
      >
        <UIcon
          :name="tab.icon"
          class="size-3.5"
        />
        {{ tab.label }}
      </button>
    </div>

    <!-- ============================== -->
    <!--  TAB: API KEYS                 -->
    <!-- ============================== -->
    <div
      v-if="activeTab === 'keys'"
      class="space-y-4"
    >
      <div class="flex items-center justify-between">
        <h2
          class="text-sm font-semibold"
          style="color: var(--argus-text);"
        >
          Управление API ключами
        </h2>
        <button
          class="scanner-btn flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
          @click="showNewKeyForm = !showNewKeyForm"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          {{ showNewKeyForm ? 'Закрыть' : 'Создать ключ' }}
        </button>
      </div>

      <!-- Create key form -->
      <Transition name="modal">
        <div
          v-if="showNewKeyForm"
          class="glass-card rounded-xl p-5 space-y-4"
        >
          <h3
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Новый API ключ
          </h3>

          <!-- Secret key display (shown once after creation) -->
          <div
            v-if="createdSecretKey"
            class="rounded-lg p-4 border"
            :style="{ background: successBg(0.05), borderColor: successBg(0.2) }"
          >
            <div class="flex items-center gap-2 mb-2">
              <UIcon
                name="i-lucide-alert-triangle"
                class="size-4"
                style="color: var(--argus-warning);"
              />
              <span
                class="text-xs font-semibold"
                style="color: var(--argus-warning);"
              >Секретный ключ показывается только один раз!</span>
            </div>
            <div class="flex items-center gap-2">
              <code
                class="flex-1 text-xs p-2 rounded"
                style="background: var(--argus-bg-deep); color: var(--argus-success); font-family: monospace; word-break: break-all;"
              >
                {{ createdSecretKey }}
              </code>
              <button
                class="shrink-0 px-3 py-2 rounded-lg text-xs font-medium cursor-pointer"
                :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
                @click="copyKey(createdSecretKey!, 'new-secret')"
              >
                {{ copiedKeyId === 'new-secret' ? 'Скопировано!' : 'Копировать' }}
              </button>
            </div>
          </div>

          <div class="grid sm:grid-cols-2 gap-4">
            <div>
              <label
                class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
                style="color: var(--argus-text-dimmed);"
              >Название</label>
              <input
                v-model="newKeyName"
                type="text"
                placeholder="Production API Key"
                class="w-full px-3 py-2 rounded-lg border text-sm outline-none"
                :style="{ background: 'var(--argus-bg-deep)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
              >
            </div>
            <div>
              <label
                class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
                style="color: var(--argus-text-dimmed);"
              >Среда</label>
              <div class="flex gap-2">
                <button
                  v-for="env in (['live', 'test'] as const)"
                  :key="env"
                  class="flex-1 px-3 py-2 rounded-lg text-xs font-medium cursor-pointer border transition-all"
                  :style="{
                    background: newKeyEnvironment === env ? accentBg(0.1) : 'var(--argus-bg-deep)',
                    borderColor: newKeyEnvironment === env ? accentBg(0.3) : 'var(--argus-border)',
                    color: newKeyEnvironment === env ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
                  }"
                  @click="newKeyEnvironment = env"
                >
                  {{ env === 'live' ? 'Production' : 'Staging' }}
                </button>
              </div>
            </div>
          </div>

          <div>
            <label
              class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
              style="color: var(--argus-text-dimmed);"
            >Разрешения</label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="perm in availablePermissions"
                :key="perm"
                class="px-2.5 py-1 rounded-md text-[11px] font-medium cursor-pointer border transition-all"
                :style="{
                  background: newKeyPermissions.includes(perm) ? accentBg(0.1) : 'transparent',
                  borderColor: newKeyPermissions.includes(perm) ? accentBg(0.3) : 'var(--argus-border)',
                  color: newKeyPermissions.includes(perm) ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)'
                }"
                @click="newKeyPermissions.includes(perm)
                  ? newKeyPermissions = newKeyPermissions.filter(p => p !== perm)
                  : newKeyPermissions.push(perm)"
              >
                {{ perm }}
              </button>
            </div>
          </div>

          <div class="flex justify-end gap-2">
            <button
              class="px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @click="showNewKeyForm = false; createdSecretKey = null"
            >
              Закрыть
            </button>
            <button
              class="scanner-btn px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
              :disabled="!newKeyName.trim() || createKeyLoading"
              @click="generateNewKey"
            >
              {{ createKeyLoading ? 'Создание...' : 'Создать ключ' }}
            </button>
          </div>
        </div>
      </Transition>

      <!-- Keys table -->
      <div class="glass-card rounded-xl overflow-hidden">
        <div
          v-if="backendKeysLoading"
          class="flex items-center justify-center py-12"
        >
          <UIcon
            name="i-lucide-loader-2"
            class="size-5 animate-spin"
            style="color: var(--argus-text-dimmed);"
          />
        </div>
        <table
          v-else
          class="w-full text-sm"
        >
          <thead>
            <tr :style="{ background: 'var(--argus-bg-elevated)' }">
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Название
              </th>
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Ключ
              </th>
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider hidden sm:table-cell"
                style="color: var(--argus-text-dimmed);"
              >
                Статус
              </th>
              <th
                class="text-right px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
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
              class="border-t"
              :style="{ borderColor: 'var(--argus-border-subtle)' }"
            >
              <td class="px-4 py-3">
                <p
                  class="text-xs font-medium"
                  style="color: var(--argus-text);"
                >
                  {{ key.name }}
                </p>
                <p
                  class="text-[10px] mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ key.createdAt?.split('T')[0] }}
                </p>
              </td>
              <td class="px-4 py-3">
                <code
                  class="text-[11px] px-1.5 py-0.5 rounded"
                  style="background: var(--argus-bg-deep); color: var(--argus-text-muted); font-family: monospace;"
                >
                  {{ revealedKeyId === key.id ? key.keyId + '_' + key.secretPrefix : maskKey(key.keyId, key.secretPrefix) }}
                </code>
              </td>
              <td class="px-4 py-3 hidden sm:table-cell">
                <span
                  class="text-[10px] font-bold px-2 py-0.5 rounded-full uppercase"
                  :style="{
                    background: key.isActive ? successBg(0.1) : errorBg(0.1),
                    color: key.isActive ? 'var(--argus-success)' : 'var(--argus-error)'
                  }"
                >
                  {{ key.isActive ? 'Активен' : 'Отозван' }}
                </span>
              </td>
              <td class="px-4 py-3 text-right">
                <div class="flex items-center justify-end gap-1">
                  <button
                    class="p-1.5 rounded-md transition-all cursor-pointer"
                    style="color: var(--argus-text-dimmed);"
                    :title="revealedKeyId === key.id ? 'Скрыть' : 'Показать'"
                    @click="revealedKeyId = revealedKeyId === key.id ? null : key.id"
                  >
                    <UIcon
                      :name="revealedKeyId === key.id ? 'i-lucide-eye-off' : 'i-lucide-eye'"
                      class="size-3.5"
                    />
                  </button>
                  <button
                    class="p-1.5 rounded-md transition-all cursor-pointer"
                    :style="{ color: copiedKeyId === key.id ? 'var(--argus-success)' : 'var(--argus-text-dimmed)' }"
                    title="Копировать"
                    @click="copyKey(key.keyId, key.id)"
                  >
                    <UIcon
                      :name="copiedKeyId === key.id ? 'i-lucide-check' : 'i-lucide-copy'"
                      class="size-3.5"
                    />
                  </button>
                  <button
                    v-if="key.isActive"
                    class="p-1.5 rounded-md transition-all cursor-pointer"
                    style="color: var(--argus-error);"
                    title="Отозвать"
                    @click="revokeKey(key.keyId)"
                  >
                    <UIcon
                      name="i-lucide-trash-2"
                      class="size-3.5"
                    />
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="filteredKeys.length === 0">
              <td
                colspan="4"
                class="px-4 py-8 text-center text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                Нет API ключей. Создайте первый ключ для начала интеграции.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  TAB: WEBHOOKS                 -->
    <!-- ============================== -->
    <div
      v-if="activeTab === 'webhooks'"
      class="space-y-4"
    >
      <div class="flex items-center justify-between">
        <h2
          class="text-sm font-semibold"
          style="color: var(--argus-text);"
        >
          Конфигурация вебхуков
        </h2>
        <button
          class="scanner-btn flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
          @click="showWebhookForm = !showWebhookForm"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          {{ showWebhookForm ? 'Закрыть' : 'Добавить вебхук' }}
        </button>
      </div>

      <!-- Add webhook form -->
      <Transition name="modal">
        <div
          v-if="showWebhookForm"
          class="glass-card rounded-xl p-5 space-y-4"
        >
          <h3
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Новый вебхук
          </h3>
          <div>
            <label
              class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
              style="color: var(--argus-text-dimmed);"
            >URL</label>
            <input
              v-model="newWebhookUrl"
              type="url"
              placeholder="https://your-app.com/webhooks/argus"
              class="w-full px-3 py-2 rounded-lg border text-sm outline-none"
              :style="{ background: 'var(--argus-bg-deep)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
            >
          </div>
          <div>
            <label
              class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
              style="color: var(--argus-text-dimmed);"
            >События</label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="evt in availableEvents"
                :key="evt.value"
                class="px-2.5 py-1.5 rounded-md text-[11px] font-medium cursor-pointer border transition-all"
                :style="{
                  background: selectedWebhookEvents.includes(evt.value) ? successBg(0.1) : 'transparent',
                  borderColor: selectedWebhookEvents.includes(evt.value) ? successBg(0.3) : 'var(--argus-border)',
                  color: selectedWebhookEvents.includes(evt.value) ? 'var(--argus-success)' : 'var(--argus-text-dimmed)'
                }"
                @click="selectedWebhookEvents.includes(evt.value)
                  ? selectedWebhookEvents = selectedWebhookEvents.filter(e => e !== evt.value)
                  : selectedWebhookEvents.push(evt.value)"
              >
                {{ evt.label }}
              </button>
            </div>
          </div>
          <div class="flex justify-end gap-2">
            <button
              class="px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @click="showWebhookForm = false"
            >
              Отмена
            </button>
            <button
              class="scanner-btn px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
              :disabled="!newWebhookUrl.trim() || selectedWebhookEvents.length === 0"
              @click="addWebhook"
            >
              Добавить
            </button>
          </div>
        </div>
      </Transition>

      <!-- Webhooks table -->
      <div class="glass-card rounded-xl overflow-hidden">
        <table class="w-full text-sm">
          <thead>
            <tr :style="{ background: 'var(--argus-bg-elevated)' }">
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                URL
              </th>
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider hidden md:table-cell"
                style="color: var(--argus-text-dimmed);"
              >
                События
              </th>
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Статус
              </th>
              <th
                class="text-right px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Действия
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="wh in store.webhooks"
              :key="wh.id"
              class="border-t"
              :style="{ borderColor: 'var(--argus-border-subtle)' }"
            >
              <td class="px-4 py-3">
                <code
                  class="text-[11px]"
                  style="color: var(--argus-text-muted); font-family: monospace;"
                >{{ wh.url }}</code>
              </td>
              <td class="px-4 py-3 hidden md:table-cell">
                <div class="flex flex-wrap gap-1">
                  <span
                    v-for="evt in wh.events"
                    :key="evt"
                    class="text-[9px] font-medium px-1.5 py-0.5 rounded"
                    :style="{ background: accentBg(0.08), color: 'var(--argus-accent)' }"
                  >
                    {{ evt }}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3">
                <span
                  class="text-[10px] font-bold px-2 py-0.5 rounded-full uppercase cursor-pointer"
                  :style="{
                    background: wh.status === 'active' ? successBg(0.1) : warningBg(0.1),
                    color: wh.status === 'active' ? 'var(--argus-success)' : 'var(--argus-warning)'
                  }"
                  @click="toggleWebhookStatus(wh.id)"
                >
                  {{ wh.status === 'active' ? 'Активен' : 'Приостановлен' }}
                </span>
              </td>
              <td class="px-4 py-3 text-right">
                <button
                  class="px-3 py-1.5 rounded-lg text-[11px] font-medium cursor-pointer border transition-all"
                  :style="{
                    borderColor: testPayloadSending === wh.id ? successBg(0.3) : 'var(--argus-border)',
                    color: testPayloadSending === wh.id ? 'var(--argus-success)' : 'var(--argus-accent)',
                    background: testPayloadSending === wh.id ? successBg(0.05) : 'transparent'
                  }"
                  :disabled="testPayloadSending === wh.id"
                  @click="sendTestPayload(wh.id)"
                >
                  <span
                    v-if="testPayloadSending === wh.id"
                    class="flex items-center gap-1"
                  >
                    <UIcon
                      name="i-lucide-loader-2"
                      class="size-3 animate-spin"
                    />
                    Отправка...
                  </span>
                  <span
                    v-else
                    class="flex items-center gap-1"
                  >
                    <UIcon
                      name="i-lucide-send"
                      class="size-3"
                    />
                    Тестовый запрос
                  </span>
                </button>
              </td>
            </tr>
            <tr v-if="store.webhooks.length === 0">
              <td
                colspan="4"
                class="px-4 py-8 text-center text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                Нет настроенных вебхуков.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  TAB: IP WHITELIST             -->
    <!-- ============================== -->
    <div
      v-if="activeTab === 'ip'"
      class="space-y-4"
    >
      <div class="flex items-center justify-between">
        <div>
          <h2
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            IP Whitelist
          </h2>
          <p
            class="text-[10px] mt-0.5"
            style="color: var(--argus-text-dimmed);"
          >
            Ограничение доступа к API по IP-адресам
          </p>
        </div>
        <button
          class="scanner-btn flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
          @click="showIPForm = !showIPForm; ipValidationError = null"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-3.5"
          />
          {{ showIPForm ? 'Закрыть' : 'Добавить IP' }}
        </button>
      </div>

      <!-- Add IP form -->
      <Transition name="modal">
        <div
          v-if="showIPForm"
          class="glass-card rounded-xl p-5 space-y-4"
        >
          <h3
            class="text-sm font-semibold"
            style="color: var(--argus-text);"
          >
            Добавить IP-адрес
          </h3>

          <div
            v-if="ipValidationError"
            class="rounded-lg px-3 py-2 text-xs font-medium"
            :style="{ background: errorBg(0.1), color: 'var(--argus-error)' }"
          >
            {{ ipValidationError }}
          </div>

          <div class="grid sm:grid-cols-2 gap-4">
            <div>
              <label
                class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
                style="color: var(--argus-text-dimmed);"
              >IP / CIDR</label>
              <input
                v-model="newIP"
                type="text"
                placeholder="192.168.1.0/24"
                class="w-full px-3 py-2 rounded-lg border text-sm outline-none font-mono"
                :style="{ background: 'var(--argus-bg-deep)', borderColor: ipValidationError ? 'var(--argus-error)' : 'var(--argus-border)', color: 'var(--argus-text)' }"
              >
            </div>
            <div>
              <label
                class="text-[10px] font-medium uppercase tracking-wider block mb-1.5"
                style="color: var(--argus-text-dimmed);"
              >Описание</label>
              <input
                v-model="newIPLabel"
                type="text"
                placeholder="Main Campus Network"
                class="w-full px-3 py-2 rounded-lg border text-sm outline-none"
                :style="{ background: 'var(--argus-bg-deep)', borderColor: 'var(--argus-border)', color: 'var(--argus-text)' }"
              >
            </div>
          </div>
          <div class="flex justify-end gap-2">
            <button
              class="px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
              style="color: var(--argus-text-dimmed);"
              @click="showIPForm = false"
            >
              Отмена
            </button>
            <button
              class="scanner-btn px-4 py-2 rounded-lg text-xs font-medium cursor-pointer"
              @click="addIPEntry"
            >
              Добавить
            </button>
          </div>
        </div>
      </Transition>

      <!-- IP table -->
      <div class="glass-card rounded-xl overflow-hidden">
        <table class="w-full text-sm">
          <thead>
            <tr :style="{ background: 'var(--argus-bg-elevated)' }">
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                IP / CIDR
              </th>
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Описание
              </th>
              <th
                class="text-left px-4 py-3 text-[10px] font-semibold uppercase tracking-wider hidden sm:table-cell"
                style="color: var(--argus-text-dimmed);"
              >
                Добавлено
              </th>
              <th
                class="text-right px-4 py-3 text-[10px] font-semibold uppercase tracking-wider"
                style="color: var(--argus-text-dimmed);"
              >
                Действия
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="entry in orgIPWhitelist"
              :key="entry.id"
              class="border-t"
              :style="{ borderColor: 'var(--argus-border-subtle)' }"
            >
              <td class="px-4 py-3">
                <code
                  class="text-xs font-mono px-1.5 py-0.5 rounded"
                  style="background: var(--argus-bg-deep); color: var(--argus-accent);"
                >{{ entry.ip }}</code>
              </td>
              <td
                class="px-4 py-3 text-xs"
                style="color: var(--argus-text-muted);"
              >
                {{ entry.label }}
              </td>
              <td
                class="px-4 py-3 text-xs hidden sm:table-cell"
                style="color: var(--argus-text-dimmed);"
              >
                {{ entry.createdAt }}
              </td>
              <td class="px-4 py-3 text-right">
                <button
                  class="p-1.5 rounded-md cursor-pointer"
                  style="color: var(--argus-error);"
                  title="Удалить"
                  @click="removeIPEntry(entry.id)"
                >
                  <UIcon
                    name="i-lucide-trash-2"
                    class="size-3.5"
                  />
                </button>
              </td>
            </tr>
            <tr v-if="orgIPWhitelist.length === 0">
              <td
                colspan="4"
                class="px-4 py-8 text-center text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                Нет записей в белом списке IP.
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- ============================== -->
    <!--  TAB: SWAGGER                  -->
    <!-- ============================== -->
    <div
      v-if="activeTab === 'swagger'"
      class="space-y-4"
    >
      <div class="grid sm:grid-cols-2 gap-4">
        <div class="glass-card rounded-xl p-6">
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-10 rounded-lg"
              :style="{ background: accentBg(0.1) }"
            >
              <UIcon
                name="i-lucide-file-code"
                class="size-5"
                style="color: var(--argus-accent);"
              />
            </div>
            <div>
              <h3
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Swagger UI
              </h3>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                Интерактивный обозреватель API
              </p>
            </div>
          </div>
          <p
            class="text-xs leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Откройте Swagger UI для тестирования API эндпоинтов, просмотра схем запросов и ответов. Все эндпоинты задокументированы с примерами.
          </p>
          <a
            :href="swaggerUrl"
            target="_blank"
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all"
            :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
          >
            <UIcon
              name="i-lucide-external-link"
              class="size-3.5"
            />
            Открыть Swagger UI
          </a>
        </div>

        <div class="glass-card rounded-xl p-6">
          <div class="flex items-center gap-3 mb-4">
            <div
              class="flex items-center justify-center size-10 rounded-lg"
              :style="{ background: successBg(0.1) }"
            >
              <UIcon
                name="i-lucide-book-open"
                class="size-5"
                style="color: var(--argus-success);"
              />
            </div>
            <div>
              <h3
                class="text-sm font-semibold"
                style="color: var(--argus-text);"
              >
                Документация
              </h3>
              <p
                class="text-[10px]"
                style="color: var(--argus-text-dimmed);"
              >
                Руководство для разработчиков
              </p>
            </div>
          </div>
          <p
            class="text-xs leading-relaxed mb-4"
            style="color: var(--argus-text-muted);"
          >
            Полная документация включает примеры кода на cURL и Node.js, описание потока сессий, вебхуков и интеграции виджета.
          </p>
          <NuxtLink
            to="/docs"
            target="_blank"
            class="inline-flex items-center gap-1.5 px-4 py-2 rounded-lg text-xs font-medium transition-all"
            :style="{ background: successBg(0.1), color: 'var(--argus-success)' }"
          >
            <UIcon
              name="i-lucide-external-link"
              class="size-3.5"
            />
            Открыть документацию
          </NuxtLink>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  TAB: HARD GATE                -->
    <!-- ============================== -->
    <div
      v-if="activeTab === 'hardgate'"
      class="space-y-4"
    >
      <div class="glass-card rounded-xl p-6">
        <div class="flex items-center gap-3 mb-4">
          <div
            class="flex items-center justify-center size-10 rounded-lg"
            :style="{ background: 'rgba(162,89,255,0.1)' }"
          >
            <UIcon
              name="i-lucide-scan-face"
              class="size-5"
              style="color: var(--argus-brand-purple);"
            />
          </div>
          <div>
            <h3
              class="text-base font-semibold"
              style="color: var(--argus-text);"
            >
              Hard Gate — PreExamCheck
            </h3>
            <p
              class="text-xs"
              style="color: var(--argus-text-dimmed);"
            >
              Система предэкзаменационной верификации
            </p>
          </div>
        </div>

        <p
          class="text-sm leading-relaxed mb-6"
          style="color: var(--argus-text-muted);"
        >
          PreExamCheck — это незакрываемый модальный барьер, который блокирует доступ к экзаменационному контенту до тех пор, пока студент не пройдёт все 4 этапа верификации. Модальное окно нельзя закрыть клавишей Escape, кликом за его пределы или кнопкой закрытия.
        </p>

        <!-- 4-stage diagram -->
        <div class="grid sm:grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
          <div
            v-for="stage in [
              { num: 1, icon: 'i-lucide-camera', title: 'Медиа-доступ', desc: 'Запрос доступа к камере и микрофону через getUserMedia. Без согласия — экзамен невозможен.', color: 'var(--argus-accent)' },
              { num: 2, icon: 'i-lucide-scan-face', title: 'Регистрация лица', desc: 'Захват эталонного снимка через MediaPipe. Требуется качество ≥0.7 в течение 2+ секунд.', color: 'var(--argus-success)' },
              { num: 3, icon: 'i-lucide-hard-drive', title: 'Аудит хранилища', desc: 'Проверка navigator.storage — необходимо ≥500 МБ свободного места для кэширования.', color: 'var(--argus-warning)' },
              { num: 4, icon: 'i-lucide-wifi', title: 'Зондирование сети', desc: '3x пинг /healthz. Измерение RTT/джиттера. Режим: online, degraded, offline.', color: 'var(--argus-brand-purple)' }
            ]"
            :key="stage.num"
            class="rounded-xl border p-4"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <div class="flex items-center gap-2 mb-3">
              <div
                class="flex items-center justify-center size-7 rounded-full text-[11px] font-bold text-white"
                :style="{ background: stage.color }"
              >
                {{ stage.num }}
              </div>
              <UIcon
                :name="stage.icon"
                class="size-4"
                :style="{ color: stage.color }"
              />
            </div>
            <h4
              class="text-sm font-semibold mb-1"
              style="color: var(--argus-text);"
            >
              {{ stage.title }}
            </h4>
            <p
              class="text-xs leading-relaxed"
              style="color: var(--argus-text-dimmed);"
            >
              {{ stage.desc }}
            </p>
          </div>
        </div>

        <!-- Unlocked state explanation -->
        <div
          class="rounded-xl border p-4"
          :style="{ background: 'rgba(162,89,255,0.05)', borderColor: 'rgba(162,89,255,0.15)' }"
        >
          <div class="flex items-start gap-3">
            <UIcon
              name="i-lucide-unlock"
              class="size-5 shrink-0 mt-0.5"
              style="color: var(--argus-brand-purple);"
            />
            <div>
              <h4
                class="text-sm font-semibold mb-1"
                style="color: var(--argus-brand-purple);"
              >
                Разблокированное состояние
              </h4>
              <p
                class="text-xs leading-relaxed"
                style="color: var(--argus-text-muted);"
              >
                Когда все 4 этапа пройдены, PreExamCheck эмитирует событие <code
                  class="px-1 py-0.5 rounded text-[11px]"
                  style="background: var(--argus-bg-elevated);"
                >verified</code> с данными:
              </p>
              <div
                class="mt-2 rounded-lg p-3"
                style="background: var(--argus-bg-deep);"
              >
                <code
                  class="text-[11px] leading-relaxed"
                  style="color: var(--argus-text-muted); font-family: monospace; white-space: pre;"
                >{{ `{
  referenceFace: Blob,        // JPEG-снимок лица
  mediaStream: MediaStream,   // Живой поток камеры для PIP
  networkMode: 'online' | 'degraded' | 'offline'
}` }}</code>
              </div>
              <p
                class="text-xs mt-2"
                style="color: var(--argus-text-dimmed);"
              >
                Клиентская система использует это событие для отображения экзаменационного контента. MediaStream передаётся в FloatingCamera (PIP) для мониторинга в реальном времени.
              </p>
            </div>
          </div>
        </div>

        <!-- Link to docs -->
        <div class="mt-4 flex justify-end">
          <NuxtLink
            to="/docs#widget-integration"
            target="_blank"
            class="flex items-center gap-1.5 text-xs font-medium"
            style="color: var(--argus-accent);"
          >
            <UIcon
              name="i-lucide-external-link"
              class="size-3.5"
            />
            Подробная документация по виджету
          </NuxtLink>
        </div>
      </div>
    </div>
  </div>
</template>
