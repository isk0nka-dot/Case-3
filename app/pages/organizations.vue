<script setup lang="ts">
import { useAuthStore } from '~/stores/useAuthStore'
import { useAdminAPI, type Organization, type User, type APIKey } from '~/composables/useAdminAPI'

// =============================================================================
// Argus AI — Organizations Management (Super Admin Only)
// =============================================================================

const authStore = useAuthStore()
const adminAPI = useAdminAPI()
const router = useRouter()
const toast = useToast()
const { isDark, accentBg, errorBg, successBg, warningBg, purpleBg } = useColors()
const { formatDate, formatDateTime } = useFormatters()

// --- Auth guard ---
onMounted(() => {
  if (!authStore.isSuperAdmin) {
    router.replace('/')
  }
})

// --- Toast helper: persistent for server unavailable, auto-hide for other errors ---
function showErrorToast(err: unknown, fallback = 'Ошибка') {
  const message = err instanceof Error ? err.message : fallback
  const isServerDown = message.includes('Сервер недоступен')
  toast.add({
    title: message,
    icon: isServerDown ? 'i-lucide-wifi-off' : 'i-lucide-alert-circle',
    color: 'error',
    duration: isServerDown ? 0 : undefined // persistent for server errors
  })
}

// --- State ---
const loading = ref(true)
const organizations = ref<Organization[]>([])
const stats = ref({ totalOrganizations: 0, totalUsers: 0, totalApiKeys: 0 })
const searchQuery = ref('')
const searchFocused = ref(false)

// --- Create Org Modal ---
const showCreateOrgModal = ref(false)
const createOrgLoading = ref(false)
const newOrg = reactive({
  orgId: '',
  name: '',
  slug: '',
  orgType: 'university',
  contactEmail: '',
  contactPhone: '',
  city: '',
  region: '',
  plan: 'standard',
  maxSessions: 100,
  maxEventsRps: 500,
  // Primary Admin fields
  adminFullName: '',
  adminPhone: '',
  adminPassword: ''
})

// --- Focus management: auto-focus first input when create modal opens ---
const orgIdInput = ref<HTMLInputElement | null>(null)

watch(showCreateOrgModal, async (isOpen) => {
  if (isOpen) {
    await nextTick()
    orgIdInput.value?.focus()
  }
})

// --- Credential Popup ---
const showCredentialPopup = ref(false)
const createdCredentials = reactive({
  orgName: '',
  orgId: '',
  adminFullName: '',
  adminPhone: '',
  adminPassword: ''
})
const credentialsCopied = ref(false)

function generateSecurePassword(): string {
  const upper = 'ABCDEFGHJKLMNPQRSTUVWXYZ'
  const lower = 'abcdefghjkmnpqrstuvwxyz'
  const digits = '23456789'
  const symbols = '!@#$%&*+-='
  const all = upper + lower + digits + symbols
  const arr = new Uint8Array(16)
  crypto.getRandomValues(arr)
  // Guarantee at least one of each class in first 4 chars.
  let pwd = upper[arr[0]! % upper.length]!
    + lower[arr[1]! % lower.length]!
    + digits[arr[2]! % digits.length]!
    + symbols[arr[3]! % symbols.length]!
  for (let i = 4; i < 16; i++) {
    pwd += all[arr[i]! % all.length]!
  }
  // Shuffle via Fisher-Yates.
  const a = pwd.split('')
  for (let i = a.length - 1; i > 0; i--) {
    const j = arr[i]! % (i + 1)
    ;[a[i]!, a[j]!] = [a[j]!, a[i]!]
  }
  return a.join('')
}

function fillGeneratedPassword() {
  newOrg.adminPassword = generateSecurePassword()
}

function copyCredentials() {
  const text = [
    `Организация: ${createdCredentials.orgName} (${createdCredentials.orgId})`,
    `Админ: ${createdCredentials.adminFullName}`,
    `Логин (телефон): +7${createdCredentials.adminPhone}`,
    `Пароль: ${createdCredentials.adminPassword}`
  ].join('\n')
  navigator.clipboard.writeText(text)
  credentialsCopied.value = true
  setTimeout(() => { credentialsCopied.value = false }, 2500)
}

// --- Delete Org Modal (slug-confirmation safety dialog) ---
const showDeleteOrgModal = ref(false)
const orgToDelete = ref<Organization | null>(null)
const deleteConfirmSlug = ref('')
const deleteOrgLoading = ref(false)

function openDeleteOrgModal(org: Organization) {
  orgToDelete.value = org
  deleteConfirmSlug.value = ''
  deleteOrgLoading.value = false
  showDeleteOrgModal.value = true
}

function cancelDeleteOrg() {
  showDeleteOrgModal.value = false
  orgToDelete.value = null
  deleteConfirmSlug.value = ''
}

const isDeleteConfirmed = computed(() => {
  return orgToDelete.value && deleteConfirmSlug.value === orgToDelete.value.slug
})

async function confirmDeleteOrg() {
  if (!orgToDelete.value || !isDeleteConfirmed.value) return

  deleteOrgLoading.value = true
  try {
    await adminAPI.deleteOrg(orgToDelete.value.orgId)
    toast.add({
      title: `Организация «${orgToDelete.value.name}» удалена`,
      icon: 'i-lucide-check-circle',
      color: 'success'
    })
    // Remove from local state immediately
    organizations.value = organizations.value.filter(o => o.orgId !== orgToDelete.value!.orgId)
    stats.value.totalOrganizations = organizations.value.length
    cancelDeleteOrg()
  } catch (err) {
    showErrorToast(err, 'Ошибка при удалении организации')
  } finally {
    deleteOrgLoading.value = false
  }
}

// --- Org Detail Slide-over ---
const selectedOrg = ref<Organization | null>(null)
const detailTab = ref<'users' | 'keys' | 'settings' | 'features'>('users')
const detailLoading = ref(false)

// --- Users tab state ---
const orgUsers = ref<User[]>([])
const usersLoading = ref(false)
const showCreateUserForm = ref(false)
const createUserLoading = ref(false)
const newUser = reactive({
  phone: '',
  password: '',
  fullName: '',
  email: '',
  role: 'proctor' as string
})

// --- API Keys tab state ---
const orgApiKeys = ref<APIKey[]>([])
const keysLoading = ref(false)
const showCreateKeyForm = ref(false)
const createKeyLoading = ref(false)
const newKey = reactive({
  name: '',
  permissions: [] as string[],
  environment: 'live' as 'live' | 'test'
})
const newlyCreatedSecret = ref<string | null>(null)
const revokeLoadingId = ref<string | null>(null)

// --- Edit org state ---
const editOrgLoading = ref(false)
const editOrgData = reactive({
  name: '',
  slug: '',
  orgType: '',
  contactEmail: '',
  contactPhone: '',
  city: '',
  region: '',
  plan: '',
  maxSessions: 0,
  maxEventsRps: 0
})

// --- Feature Toggles state ---
interface OrgFeatureToggles {
  // AI Rules
  ai_face_verification: boolean
  ai_gaze_tracking: boolean
  ai_object_detection: boolean
  ai_emotion_analysis: boolean
  ai_anti_spoofing: boolean
  ai_voice_detection: boolean
  ai_blink_analysis: boolean
  // Browser Rules
  browser_fullscreen: boolean
  browser_tab_limit: boolean
  browser_copy_paste_block: boolean
  browser_print_screen_block: boolean
  browser_vm_block: boolean
  browser_context_menu_block: boolean
  browser_remote_access_block: boolean
  // Behavioral Analysis Rules
  behavioral_typing_dynamics: boolean
  behavioral_cursor_sync: boolean
  // System Rules
  system_process_scanning: boolean
  system_hardware_detection: boolean
  system_vpn_detection: boolean
  system_network_scan: boolean
  system_multi_monitor: boolean
  system_hardware_id_binding: boolean
}

const featureToggles = reactive<OrgFeatureToggles>({
  ai_face_verification: true,
  ai_gaze_tracking: true,
  ai_object_detection: true,
  ai_emotion_analysis: true,
  ai_anti_spoofing: true,
  ai_voice_detection: true,
  ai_blink_analysis: true,
  browser_fullscreen: true,
  browser_tab_limit: true,
  browser_copy_paste_block: true,
  browser_print_screen_block: true,
  browser_vm_block: true,
  browser_context_menu_block: true,
  browser_remote_access_block: true,
  behavioral_typing_dynamics: true,
  behavioral_cursor_sync: true,
  system_process_scanning: true,
  system_hardware_detection: true,
  system_vpn_detection: true,
  system_network_scan: true,
  system_multi_monitor: true,
  system_hardware_id_binding: true
})

const featureSaving = ref(false)

// Feature toggle groups for display
interface FeatureGroup {
  id: string
  label: string
  icon: string
  color: string
  description?: string
  features: { key: keyof OrgFeatureToggles, label: string, description: string, badge?: string }[]
}

const featureGroups: FeatureGroup[] = [
  {
    id: 'ai', label: 'AI Правила', icon: 'i-lucide-brain', color: 'var(--argus-accent)',
    features: [
      { key: 'ai_face_verification', label: 'Верификация лица', description: 'ID-фото сопоставление перед экзаменом' },
      { key: 'ai_gaze_tracking', label: 'Отслеживание взгляда', description: 'AI-анализ направления взгляда' },
      { key: 'ai_object_detection', label: 'Детекция объектов', description: 'Обнаружение телефона, книг, наушников' },
      { key: 'ai_emotion_analysis', label: 'Анализ эмоций', description: 'Стресс и подозрительные эмоции' },
      { key: 'ai_anti_spoofing', label: 'Anti-spoofing', description: 'Защита от подмены лица (фото/видео)' },
      { key: 'ai_voice_detection', label: 'Голосовая детекция', description: 'Обнаружение речи и шёпота' },
      { key: 'ai_blink_analysis', label: 'Анализ моргания', description: 'Паттерны моргания для liveness' }
    ]
  },
  {
    id: 'browser', label: 'Правила браузера', icon: 'i-lucide-globe', color: isDark.value ? '#fbbf24' : '#d97706',
    features: [
      { key: 'browser_fullscreen', label: 'Полноэкранный режим', description: 'Принудительный fullscreen + детекция выхода' },
      { key: 'browser_tab_limit', label: 'Лимит вкладок', description: 'Ограничение переключений вкладок' },
      { key: 'browser_copy_paste_block', label: 'Блокировка копирования', description: 'Запрет Ctrl+C / Ctrl+V' },
      { key: 'browser_print_screen_block', label: 'Блокировка скриншотов', description: 'Запрет PrintScreen / снимков экрана' },
      { key: 'browser_vm_block', label: 'Блокировка VM', description: 'Обнаружение виртуальных машин' },
      { key: 'browser_context_menu_block', label: 'Блокировка контекстного меню', description: 'Запрет правого клика' },
      { key: 'browser_remote_access_block', label: 'Блокировка удалённого доступа', description: 'TeamViewer, AnyDesk и др.' }
    ]
  },
  {
    id: 'behavioral', label: 'Поведенческий анализ', icon: 'i-lucide-brain-circuit', color: 'var(--argus-accent)',
    features: [
      { key: 'behavioral_typing_dynamics', label: 'Динамика набора текста', description: 'Анализ биометрического почерка клавиатурного ввода: WPM, латентность между нажатиями, ритм набора', badge: 'Kernel-данные' },
      { key: 'behavioral_cursor_sync', label: 'Синхронизация руки и курсора', description: 'Корреляция физических движений руки с перемещением курсора для детекции удалённого управления', badge: 'Kernel-данные' }
    ]
  },
  {
    id: 'system', label: 'Системные правила', icon: 'i-lucide-cpu', color: isDark.value ? '#a78bfa' : '#7c3aed',
    features: [
      { key: 'system_process_scanning', label: 'Сканирование процессов', description: 'Проверка запущенных процессов' },
      { key: 'system_hardware_detection', label: 'Детекция оборудования', description: 'USB-устройства и периферия' },
      { key: 'system_vpn_detection', label: 'VPN/Proxy детекция', description: 'Обнаружение VPN и прокси-серверов' },
      { key: 'system_network_scan', label: 'Сканирование сети', description: 'Анализ локальной сети' },
      { key: 'system_multi_monitor', label: 'Мульти-монитор', description: 'Обнаружение нескольких мониторов' },
      { key: 'system_hardware_id_binding', label: 'Привязка к устройству', description: 'Проверка Hardware ID' }
    ]
  }
]

// Computed: count of enabled features per group
function countEnabledInGroup(group: FeatureGroup): number {
  return group.features.filter(f => featureToggles[f.key]).length
}

// Toggle all features in a group
function toggleGroup(group: FeatureGroup, value: boolean) {
  group.features.forEach((f) => {
    featureToggles[f.key] = value
  })
}

// Check if all features in a group are enabled
function isGroupFullyEnabled(group: FeatureGroup): boolean {
  return group.features.every(f => featureToggles[f.key])
}

// Check if any feature in a group is enabled
function isGroupPartiallyEnabled(group: FeatureGroup): boolean {
  return group.features.some(f => featureToggles[f.key]) && !isGroupFullyEnabled(group)
}

// Save feature toggles (mock)
async function saveFeatureToggles() {
  featureSaving.value = true
  try {
    // In real implementation, call adminAPI.updateOrgFeatures(selectedOrg.value.orgId, featureToggles)
    await new Promise(r => setTimeout(r, 800))
    toast.add({ title: 'Feature toggles сохранены', icon: 'i-lucide-check-circle', color: 'success' })
  } catch {
    toast.add({ title: 'Ошибка сохранения', icon: 'i-lucide-alert-circle', color: 'error' })
  } finally {
    featureSaving.value = false
  }
}

// Load feature toggles when opening detail (add to loadTabData for 'features')
function loadFeatureToggles() {
  // In real impl, load from API. For now, use defaults.
  // Reset all to true as default for each org
  Object.keys(featureToggles).forEach((key) => {
    (featureToggles as any)[key] = true
  })
}

// --- Computed ---
const filteredOrgs = computed(() => {
  const q = searchQuery.value.toLowerCase().trim()
  if (!q) return organizations.value
  return organizations.value.filter(org =>
    org.name.toLowerCase().includes(q)
    || org.orgId.toLowerCase().includes(q)
    || org.slug.toLowerCase().includes(q)
    || (org.city && org.city.toLowerCase().includes(q))
    || (org.contactEmail && org.contactEmail.toLowerCase().includes(q))
  )
})

// --- Plan color mapping ---
function planColor(plan: string): string {
  switch (plan) {
    case 'free': return 'neutral'
    case 'standard': return 'info'
    case 'professional': return 'primary'
    case 'enterprise': return 'warning'
    case 'unlimited': return 'success'
    default: return 'neutral'
  }
}

function planBg(plan: string, opacity: number): string {
  switch (plan) {
    case 'free': return isDark.value ? `rgba(148, 163, 184, ${opacity})` : `rgba(107, 114, 128, ${opacity})`
    case 'standard': return accentBg(opacity)
    case 'professional': return purpleBg(opacity)
    case 'enterprise': return warningBg(opacity)
    case 'unlimited': return successBg(opacity)
    default: return isDark.value ? `rgba(148, 163, 184, ${opacity})` : `rgba(107, 114, 128, ${opacity})`
  }
}

function planTextColor(plan: string): string {
  switch (plan) {
    case 'free': return 'var(--argus-text-dimmed)'
    case 'standard': return 'var(--argus-accent)'
    case 'professional': return isDark.value ? '#a78bfa' : '#7c3aed'
    case 'enterprise': return isDark.value ? '#fbbf24' : '#d97706'
    case 'unlimited': return isDark.value ? '#34d399' : '#059669'
    default: return 'var(--argus-text-dimmed)'
  }
}

function planLabel(plan: string): string {
  switch (plan) {
    case 'free': return 'Free'
    case 'standard': return 'Standard'
    case 'professional': return 'Professional'
    case 'enterprise': return 'Enterprise'
    case 'unlimited': return 'Unlimited'
    default: return plan
  }
}

// --- Role badge helpers ---
function roleBg(role: string, opacity: number): string {
  switch (role) {
    case 'super_admin': return errorBg(opacity)
    case 'org_admin': return warningBg(opacity)
    case 'proctor': return accentBg(opacity)
    case 'viewer': return isDark.value ? `rgba(148, 163, 184, ${opacity})` : `rgba(107, 114, 128, ${opacity})`
    default: return isDark.value ? `rgba(148, 163, 184, ${opacity})` : `rgba(107, 114, 128, ${opacity})`
  }
}

function roleTextColor(role: string): string {
  switch (role) {
    case 'super_admin': return isDark.value ? '#f87171' : '#dc2626'
    case 'org_admin': return isDark.value ? '#fbbf24' : '#d97706'
    case 'proctor': return 'var(--argus-accent)'
    case 'viewer': return 'var(--argus-text-dimmed)'
    default: return 'var(--argus-text-dimmed)'
  }
}

function roleLabel(role: string): string {
  switch (role) {
    case 'super_admin': return 'Super Admin'
    case 'org_admin': return 'Орг. Админ'
    case 'proctor': return 'Проктор'
    case 'viewer': return 'Наблюдатель'
    default: return role
  }
}

// --- Org type options ---
const orgTypeOptions = [
  { value: 'university', label: 'Университет' },
  { value: 'college', label: 'Колледж' },
  { value: 'school', label: 'Школа' },
  { value: 'testing_center', label: 'Тестовый центр' },
  { value: 'corporate', label: 'Корпоративный' },
  { value: 'government', label: 'Государственный' }
]

const planOptions = [
  { value: 'free', label: 'Free' },
  { value: 'standard', label: 'Standard' },
  { value: 'professional', label: 'Professional' },
  { value: 'enterprise', label: 'Enterprise' },
  { value: 'unlimited', label: 'Unlimited' }
]

const roleOptions = [
  { value: 'org_admin', label: 'Орг. Админ' },
  { value: 'proctor', label: 'Проктор' },
  { value: 'viewer', label: 'Наблюдатель' }
]

const permissionOptions = [
  { value: 'events:write', label: 'events:write' },
  { value: 'events:read', label: 'events:read' },
  { value: 'sessions:read', label: 'sessions:read' },
  { value: 'sessions:write', label: 'sessions:write' },
  { value: 'analytics:read', label: 'analytics:read' }
]

// --- Data fetching ---
async function fetchOrganizations() {
  loading.value = true
  try {
    const [orgsData, statsData] = await Promise.all([
      adminAPI.listOrgs({ search: searchQuery.value || undefined }),
      adminAPI.getStats()
    ])
    organizations.value = orgsData
    stats.value = statsData
  } catch (err: unknown) {
    showErrorToast(err, 'Ошибка загрузки данных')
  } finally {
    loading.value = false
  }
}

async function refreshAll() {
  await fetchOrganizations()
}

// --- Create Organization ---
function openCreateOrgModal() {
  newOrg.orgId = ''
  newOrg.name = ''
  newOrg.slug = ''
  newOrg.orgType = 'university'
  newOrg.contactEmail = ''
  newOrg.contactPhone = ''
  newOrg.city = ''
  newOrg.region = ''
  newOrg.plan = 'standard'
  newOrg.maxSessions = 100
  newOrg.maxEventsRps = 500
  newOrg.adminFullName = ''
  newOrg.adminPhone = ''
  newOrg.adminPassword = ''
  showCreateOrgModal.value = true
}

// --- Create form validation: all mandatory fields must be filled ---
const isCreateFormValid = computed(() => {
  return !!(
    newOrg.orgId.trim()
    && newOrg.name.trim()
    && newOrg.slug.trim()
    && newOrg.adminFullName.trim()
    && newOrg.adminPhone.trim()
    && newOrg.adminPassword.length >= 6
  )
})

async function submitCreateOrg() {
  if (!newOrg.orgId || !newOrg.name || !newOrg.slug) {
    toast.add({ title: 'Заполните обязательные поля: ID, Название, Slug', icon: 'i-lucide-alert-circle', color: 'warning' })
    return
  }

  // Validate admin fields.
  if (!newOrg.adminFullName || !newOrg.adminPhone || !newOrg.adminPassword) {
    toast.add({ title: 'Заполните данные Primary Admin: ФИО, Телефон, Пароль', icon: 'i-lucide-alert-circle', color: 'warning' })
    return
  }
  if (newOrg.adminPassword.length < 6) {
    toast.add({ title: 'Пароль админа должен быть минимум 6 символов', icon: 'i-lucide-alert-circle', color: 'warning' })
    return
  }

  // Validate JWT token is present before making request.
  if (!authStore.jwtToken) {
    toast.add({ title: 'Ошибка авторизации: JWT токен отсутствует. Перезайдите в систему.', icon: 'i-lucide-shield-alert', color: 'error' })
    return
  }

  createOrgLoading.value = true
  try {
    const payload = {
      orgId: newOrg.orgId,
      name: newOrg.name,
      slug: newOrg.slug,
      orgType: newOrg.orgType,
      contactEmail: newOrg.contactEmail || undefined,
      contactPhone: newOrg.contactPhone || undefined,
      city: newOrg.city || undefined,
      region: newOrg.region || undefined,
      plan: newOrg.plan,
      maxSessions: newOrg.maxSessions,
      maxEventsRps: newOrg.maxEventsRps,
      adminFullName: newOrg.adminFullName,
      adminPhone: newOrg.adminPhone,
      adminPassword: newOrg.adminPassword
    }

    console.log('[Organizations] Creating org+admin with payload (password redacted)')

    const result = await adminAPI.createOrgWithAdmin(payload)
    console.log('[Organizations] Org+admin created successfully:', result.organization.orgId)

    // Populate credential popup BEFORE closing create modal.
    createdCredentials.orgName = result.organization.name
    createdCredentials.orgId = result.organization.orgId
    createdCredentials.adminFullName = result.admin.fullName
    createdCredentials.adminPhone = newOrg.adminPhone
    createdCredentials.adminPassword = newOrg.adminPassword
    credentialsCopied.value = false

    toast.add({ title: 'Организация и администратор созданы', icon: 'i-lucide-check-circle', color: 'success' })
    showCreateOrgModal.value = false
    showCredentialPopup.value = true
    await refreshAll()
  } catch (err: unknown) {
    console.error('[Organizations] Create org+admin failed:', err)
    showErrorToast(err, 'Ошибка создания организации')
  } finally {
    createOrgLoading.value = false
  }
}

// --- Org Detail / Slide-over ---
async function openOrgDetail(org: Organization) {
  selectedOrg.value = org
  detailTab.value = 'users'
  newlyCreatedSecret.value = null
  showCreateUserForm.value = false
  showCreateKeyForm.value = false

  // Populate edit form
  editOrgData.name = org.name
  editOrgData.slug = org.slug
  editOrgData.orgType = org.orgType
  editOrgData.contactEmail = org.contactEmail || ''
  editOrgData.contactPhone = org.contactPhone || ''
  editOrgData.city = org.city || ''
  editOrgData.region = org.region || ''
  editOrgData.plan = org.plan
  editOrgData.maxSessions = org.maxSessions
  editOrgData.maxEventsRps = org.maxEventsRps

  await loadTabData('users', org.orgId)
}

function closeOrgDetail() {
  selectedOrg.value = null
  orgUsers.value = []
  orgApiKeys.value = []
  newlyCreatedSecret.value = null
}

async function switchTab(tab: 'users' | 'keys' | 'settings' | 'features') {
  detailTab.value = tab
  if (selectedOrg.value) {
    await loadTabData(tab, selectedOrg.value.orgId)
  }
}

async function loadTabData(tab: string, orgId: string) {
  detailLoading.value = true
  try {
    if (tab === 'users') {
      usersLoading.value = true
      orgUsers.value = await adminAPI.listUsers(orgId)
      usersLoading.value = false
    } else if (tab === 'keys') {
      keysLoading.value = true
      orgApiKeys.value = await adminAPI.listAPIKeys(orgId)
      keysLoading.value = false
    } else if (tab === 'features') {
      loadFeatureToggles()
    }
  } catch (err: unknown) {
    showErrorToast(err, 'Ошибка загрузки данных')
  } finally {
    detailLoading.value = false
  }
}

// --- Create User ---
function openCreateUserForm() {
  newUser.phone = ''
  newUser.password = ''
  newUser.fullName = ''
  newUser.email = ''
  newUser.role = 'proctor'
  showCreateUserForm.value = true
}

async function submitCreateUser() {
  if (!selectedOrg.value) return
  if (!newUser.phone || !newUser.password || !newUser.fullName) {
    toast.add({ title: 'Заполните обязательные поля', icon: 'i-lucide-alert-circle', color: 'warning' })
    return
  }
  createUserLoading.value = true
  try {
    await adminAPI.createUser(selectedOrg.value.orgId, {
      phone: newUser.phone,
      password: newUser.password,
      fullName: newUser.fullName,
      email: newUser.email || undefined,
      role: newUser.role
    })
    toast.add({ title: 'Пользователь создан', icon: 'i-lucide-check-circle', color: 'success' })
    showCreateUserForm.value = false
    orgUsers.value = await adminAPI.listUsers(selectedOrg.value.orgId)
  } catch (err: unknown) {
    showErrorToast(err, 'Ошибка создания пользователя')
  } finally {
    createUserLoading.value = false
  }
}

// --- Create API Key ---
function openCreateKeyForm() {
  newKey.name = ''
  newKey.permissions = []
  newKey.environment = 'live'
  newlyCreatedSecret.value = null
  showCreateKeyForm.value = true
}

async function submitCreateKey() {
  if (!selectedOrg.value) return
  if (!newKey.name) {
    toast.add({ title: 'Укажите название ключа', icon: 'i-lucide-alert-circle', color: 'warning' })
    return
  }
  createKeyLoading.value = true
  try {
    const response = await adminAPI.createAPIKey(selectedOrg.value.orgId, {
      name: newKey.name,
      permissions: newKey.permissions.length > 0 ? newKey.permissions : undefined,
      environment: newKey.environment
    })
    newlyCreatedSecret.value = response.secret
    toast.add({ title: 'API ключ создан', icon: 'i-lucide-check-circle', color: 'success' })
    orgApiKeys.value = await adminAPI.listAPIKeys(selectedOrg.value.orgId)
  } catch (err: unknown) {
    showErrorToast(err, 'Ошибка создания ключа')
  } finally {
    createKeyLoading.value = false
  }
}

// --- Revoke API Key ---
async function revokeKey(keyId: string) {
  revokeLoadingId.value = keyId
  try {
    await adminAPI.revokeAPIKey(keyId)
    toast.add({ title: 'API ключ отозван', icon: 'i-lucide-check-circle', color: 'warning' })
    if (selectedOrg.value) {
      orgApiKeys.value = await adminAPI.listAPIKeys(selectedOrg.value.orgId)
    }
  } catch (err: unknown) {
    showErrorToast(err, 'Ошибка отзыва ключа')
  } finally {
    revokeLoadingId.value = null
  }
}

// --- Update Organization ---
async function submitUpdateOrg() {
  if (!selectedOrg.value) return
  editOrgLoading.value = true
  try {
    const updated = await adminAPI.updateOrg(selectedOrg.value.orgId, {
      name: editOrgData.name,
      slug: editOrgData.slug,
      orgType: editOrgData.orgType,
      contactEmail: editOrgData.contactEmail || undefined,
      contactPhone: editOrgData.contactPhone || undefined,
      city: editOrgData.city || undefined,
      region: editOrgData.region || undefined,
      plan: editOrgData.plan,
      maxSessions: editOrgData.maxSessions,
      maxEventsRps: editOrgData.maxEventsRps
    })
    selectedOrg.value = updated
    toast.add({ title: 'Организация обновлена', icon: 'i-lucide-check-circle', color: 'success' })
    await refreshAll()
  } catch (err: unknown) {
    showErrorToast(err, 'Ошибка обновления')
  } finally {
    editOrgLoading.value = false
  }
}

// --- Copy to clipboard ---
async function copyToClipboard(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.add({ title: 'Скопировано в буфер обмена', icon: 'i-lucide-copy', color: 'success' })
  } catch {
    toast.add({ title: 'Не удалось скопировать', icon: 'i-lucide-alert-circle', color: 'error' })
  }
}

// --- Permission toggle ---
function togglePermission(perm: string) {
  const idx = newKey.permissions.indexOf(perm)
  if (idx >= 0) {
    newKey.permissions.splice(idx, 1)
  } else {
    newKey.permissions.push(perm)
  }
}

// --- Org type label ---
function orgTypeLabel(t: string): string {
  const found = orgTypeOptions.find(o => o.value === t)
  return found ? found.label : t
}

// --- Init ---
onMounted(() => {
  if (authStore.isSuperAdmin) {
    fetchOrganizations()
  }
})
</script>

<template>
  <div
    v-if="authStore.isSuperAdmin"
    class="min-h-screen p-4 md:p-6 lg:p-8 space-y-6"
    :style="{ background: 'var(--argus-bg-deep)' }"
  >
    <!-- ============================== -->
    <!--  HEADER                        -->
    <!-- ============================== -->
    <div class="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-3">
          <div
            class="flex items-center justify-center size-10 rounded-xl"
            :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
          >
            <UIcon
              name="i-lucide-building-2"
              class="size-5"
              style="color: var(--argus-accent);"
            />
          </div>
          <div>
            <h1
              class="text-xl font-bold"
              style="color: var(--argus-text);"
            >
              Организации
            </h1>
            <p
              class="text-xs mt-0.5"
              style="color: var(--argus-text-dimmed);"
            >
              Управление организациями и пользователями платформы
              <span
                class="ml-1 font-semibold"
                style="color: var(--argus-text-muted);"
              >
                ({{ organizations.length }})
              </span>
            </p>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-3 flex-wrap">
        <!-- Refresh -->
        <button
          class="flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-all cursor-pointer"
          :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text-dimmed)' }"
          @click="refreshAll"
        >
          <UIcon
            name="i-lucide-refresh-cw"
            class="size-3.5"
            :class="{ 'animate-spin': loading }"
          />
          Обновить
        </button>

        <!-- Create Org Button -->
        <button
          class="flex items-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold transition-all cursor-pointer"
          :style="{ background: 'var(--argus-accent)', color: '#fff' }"
          @click="openCreateOrgModal"
        >
          <UIcon
            name="i-lucide-plus"
            class="size-4"
          />
          Создать организацию
        </button>
      </div>
    </div>

    <!-- ============================== -->
    <!--  STATS CARDS                   -->
    <!-- ============================== -->
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
      <!-- Total Orgs -->
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: accentBg(0.2) }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: accentBg(0.08) }"
        >
          <UIcon
            name="i-lucide-building-2"
            class="size-4"
            style="color: var(--argus-accent);"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            style="color: var(--argus-accent);"
          >
            {{ stats.totalOrganizations }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Организации
          </p>
        </div>
      </div>

      <!-- Total Users -->
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: successBg(0.2) }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: successBg(0.08) }"
        >
          <UIcon
            name="i-lucide-users"
            class="size-4"
            :style="{ color: isDark ? '#34d399' : '#059669' }"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            :style="{ color: isDark ? '#34d399' : '#059669' }"
          >
            {{ stats.totalUsers }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            Пользователи
          </p>
        </div>
      </div>

      <!-- Total API Keys -->
      <div
        class="flex items-center gap-3 px-4 py-3 rounded-xl border"
        :style="{ background: 'var(--argus-bg-card)', borderColor: purpleBg(0.2) }"
      >
        <div
          class="flex items-center justify-center size-9 rounded-lg shrink-0"
          :style="{ background: purpleBg(0.08) }"
        >
          <UIcon
            name="i-lucide-key-round"
            class="size-4"
            :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
          />
        </div>
        <div class="min-w-0">
          <p
            class="text-lg font-bold tabular-nums leading-tight"
            :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
          >
            {{ stats.totalApiKeys }}
          </p>
          <p
            class="text-[9px] font-medium uppercase tracking-wider"
            style="color: var(--argus-text-dimmed);"
          >
            API Ключи
          </p>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  SEARCH BAR                    -->
    <!-- ============================== -->
    <div
      class="rounded-xl border overflow-hidden"
      :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
    >
      <div class="flex items-center gap-3 px-5 py-3">
        <div
          class="flex items-center gap-2 flex-1 px-3 py-2 rounded-lg transition-all"
          :style="{
            background: 'var(--argus-bg-elevated)',
            border: `1px solid ${searchFocused ? 'var(--argus-accent)' : 'var(--argus-border)'}`,
            boxShadow: searchFocused ? `0 0 0 3px ${accentBg(0.1)}` : 'none'
          }"
        >
          <UIcon
            name="i-lucide-search"
            class="size-3.5 shrink-0"
            style="color: var(--argus-text-dimmed);"
          />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Поиск по названию, городу, email, ID..."
            class="flex-1 bg-transparent text-xs font-medium outline-none placeholder:text-[var(--argus-text-dimmed)]"
            style="color: var(--argus-text);"
            @focus="searchFocused = true"
            @blur="searchFocused = false"
          >
          <button
            v-if="searchQuery"
            class="cursor-pointer"
            @click="searchQuery = ''"
          >
            <UIcon
              name="i-lucide-x"
              class="size-3.5"
              style="color: var(--argus-text-dimmed);"
            />
          </button>
        </div>

        <div
          class="flex items-center gap-2 text-xs"
          style="color: var(--argus-text-dimmed);"
        >
          <span class="font-medium">{{ filteredOrgs.length }}</span>
          <span>из</span>
          <span class="font-medium">{{ organizations.length }}</span>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  LOADING STATE                 -->
    <!-- ============================== -->
    <div
      v-if="loading"
      class="flex flex-col items-center justify-center py-20 gap-4"
    >
      <UIcon
        name="i-lucide-loader-2"
        class="size-8 animate-spin"
        style="color: var(--argus-accent);"
      />
      <p
        class="text-sm font-medium"
        style="color: var(--argus-text-dimmed);"
      >
        Загрузка организаций...
      </p>
    </div>

    <!-- ============================== -->
    <!--  EMPTY STATE                   -->
    <!-- ============================== -->
    <div
      v-else-if="filteredOrgs.length === 0 && !loading"
      class="flex flex-col items-center justify-center py-20 gap-4 rounded-xl border"
      :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
    >
      <UIcon
        name="i-lucide-building-2"
        class="size-12"
        style="color: var(--argus-text-dimmed); opacity: 0.4;"
      />
      <p
        class="text-sm font-medium"
        style="color: var(--argus-text-dimmed);"
      >
        {{ searchQuery ? 'Ничего не найдено' : 'Нет организаций' }}
      </p>
      <button
        v-if="!searchQuery"
        class="flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-bold cursor-pointer"
        :style="{ background: 'var(--argus-accent)', color: '#fff' }"
        @click="openCreateOrgModal"
      >
        <UIcon
          name="i-lucide-plus"
          class="size-3.5"
        />
        Создать первую организацию
      </button>
    </div>

    <!-- ============================== -->
    <!--  ORGANIZATION CARDS GRID       -->
    <!-- ============================== -->
    <div
      v-else
      class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4"
    >
      <div
        v-for="org in filteredOrgs"
        :key="org.id"
        class="rounded-xl border transition-all cursor-pointer group"
        :style="{
          background: 'var(--argus-bg-card)',
          borderColor: 'var(--argus-border)'
        }"
        @click="openOrgDetail(org)"
      >
        <!-- Card Header -->
        <div class="px-5 pt-4 pb-3">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <h3
                  class="text-sm font-bold truncate"
                  style="color: var(--argus-text);"
                >
                  {{ org.name }}
                </h3>
                <!-- Status Dot -->
                <span
                  class="size-2 rounded-full shrink-0"
                  :style="{ background: org.isActive ? (isDark ? '#34d399' : '#059669') : (isDark ? '#f87171' : '#dc2626') }"
                  :title="org.isActive ? 'Активна' : 'Неактивна'"
                />
              </div>
              <div class="flex items-center gap-2 mt-1">
                <span
                  class="text-[10px] font-mono"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ org.orgId }}
                </span>
                <span
                  class="text-[10px]"
                  style="color: var(--argus-border);"
                >|</span>
                <span
                  class="text-[10px] font-mono"
                  style="color: var(--argus-text-dimmed);"
                >
                  {{ org.slug }}
                </span>
              </div>
            </div>

            <!-- Plan Badge -->
            <span
              class="px-2.5 py-1 rounded-md text-[10px] font-bold uppercase tracking-wider shrink-0"
              :style="{
                background: planBg(org.plan, 0.1),
                color: planTextColor(org.plan),
                border: `1px solid ${planBg(org.plan, 0.2)}`
              }"
            >
              {{ planLabel(org.plan) }}
            </span>
          </div>
        </div>

        <!-- Card Body -->
        <div class="px-5 pb-3 space-y-2">
          <!-- City + Email row -->
          <div class="flex items-center gap-4 flex-wrap">
            <div
              v-if="org.city"
              class="flex items-center gap-1.5"
            >
              <UIcon
                name="i-lucide-map-pin"
                class="size-3"
                style="color: var(--argus-text-dimmed);"
              />
              <span
                class="text-[11px]"
                style="color: var(--argus-text-muted);"
              >{{ org.city }}</span>
            </div>
            <div
              v-if="org.contactEmail"
              class="flex items-center gap-1.5 min-w-0"
            >
              <UIcon
                name="i-lucide-mail"
                class="size-3 shrink-0"
                style="color: var(--argus-text-dimmed);"
              />
              <span
                class="text-[11px] truncate"
                style="color: var(--argus-text-muted);"
              >{{ org.contactEmail }}</span>
            </div>
          </div>

          <!-- Org type + limits -->
          <div class="flex items-center gap-3 flex-wrap">
            <span
              class="px-2 py-0.5 rounded text-[10px] font-medium"
              :style="{ background: 'var(--argus-bg-elevated)', color: 'var(--argus-text-dimmed)' }"
            >
              {{ orgTypeLabel(org.orgType) }}
            </span>
            <div class="flex items-center gap-1.5">
              <UIcon
                name="i-lucide-monitor"
                class="size-3"
                style="color: var(--argus-text-dimmed);"
              />
              <span
                class="text-[10px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >
                {{ org.maxSessions }} сессий
              </span>
            </div>
            <div class="flex items-center gap-1.5">
              <UIcon
                name="i-lucide-zap"
                class="size-3"
                style="color: var(--argus-text-dimmed);"
              />
              <span
                class="text-[10px] font-medium"
                style="color: var(--argus-text-dimmed);"
              >
                {{ org.maxEventsRps }} ev/s
              </span>
            </div>
          </div>
        </div>

        <!-- Card Footer -->
        <div
          class="flex items-center justify-between px-5 py-2.5 border-t"
          :style="{ borderColor: 'var(--argus-border-subtle)' }"
        >
          <span
            class="text-[10px]"
            style="color: var(--argus-text-dimmed);"
          >
            Создана: {{ formatDate(org.createdAt) }}
          </span>
          <div class="flex items-center gap-1">
            <button
              class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-all cursor-pointer"
              :style="{ color: 'var(--argus-accent)' }"
              title="Пользователи"
              @click.stop="openOrgDetail(org); switchTab('users')"
            >
              <UIcon
                name="i-lucide-users"
                class="size-3"
              />
            </button>
            <button
              class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-all cursor-pointer"
              :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
              title="API Ключи"
              @click.stop="openOrgDetail(org); switchTab('keys')"
            >
              <UIcon
                name="i-lucide-key-round"
                class="size-3"
              />
            </button>
            <button
              class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-all cursor-pointer"
              :style="{ color: 'var(--argus-text-dimmed)' }"
              title="Настройки"
              @click.stop="openOrgDetail(org); switchTab('settings')"
            >
              <UIcon
                name="i-lucide-settings"
                class="size-3"
              />
            </button>
            <button
              v-if="org.orgId !== '*'"
              class="flex items-center gap-1 px-2 py-1 rounded text-[10px] font-medium transition-all cursor-pointer hover:opacity-80"
              :style="{ color: isDark ? '#f87171' : '#dc2626' }"
              title="Удалить организацию"
              @click.stop="openDeleteOrgModal(org)"
            >
              <UIcon
                name="i-lucide-trash-2"
                class="size-3"
              />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- ============================== -->
    <!--  CREATE ORG MODAL              -->
    <!-- ============================== -->
    <UModal
      v-model:open="showCreateOrgModal"
      :close="false"
      :ui="{ overlay: 'z-[100]', content: 'z-[100]' }"
    >
      <template #content>
        <div
          class="p-6 max-h-[85vh] overflow-y-auto"
          :style="{ background: 'var(--argus-bg-card)' }"
          @click.stop
        >
          <div class="flex items-center gap-3 mb-6">
            <div
              class="flex items-center justify-center size-10 rounded-xl"
              :style="{ background: accentBg(0.1), border: `1px solid ${accentBg(0.15)}` }"
            >
              <UIcon
                name="i-lucide-building-2"
                class="size-5"
                style="color: var(--argus-accent);"
              />
            </div>
            <div>
              <h2
                class="text-lg font-bold"
                style="color: var(--argus-text);"
              >
                Создать организацию
              </h2>
              <p
                class="text-xs"
                style="color: var(--argus-text-dimmed);"
              >
                Заполните данные новой организации
              </p>
            </div>
          </div>

          <div class="space-y-4">
            <!-- Row: orgId + slug -->
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  ID организации *
                </label>
                <input
                  ref="orgIdInput"
                  v-model="newOrg.orgId"
                  type="text"
                  placeholder="org_kaznu"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Slug *
                </label>
                <input
                  v-model="newOrg.slug"
                  type="text"
                  placeholder="kaznu"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
            </div>

            <!-- Name -->
            <div>
              <label
                class="block text-[11px] font-medium mb-1.5"
                style="color: var(--argus-text-dimmed);"
              >
                Название организации *
              </label>
              <input
                v-model="newOrg.name"
                type="text"
                placeholder="Казахский Национальный Университет"
                class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                :style="{
                  background: 'var(--argus-bg-elevated)',
                  border: '1px solid var(--argus-border)',
                  color: 'var(--argus-text)'
                }"
              >
            </div>

            <!-- Row: orgType + plan -->
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Тип организации
                </label>
                <select
                  v-model="newOrg.orgType"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all cursor-pointer"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
                  <option
                    v-for="opt in orgTypeOptions"
                    :key="opt.value"
                    :value="opt.value"
                  >
                    {{ opt.label }}
                  </option>
                </select>
              </div>
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Тарифный план
                </label>
                <select
                  v-model="newOrg.plan"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all cursor-pointer"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
                  <option
                    v-for="opt in planOptions"
                    :key="opt.value"
                    :value="opt.value"
                  >
                    {{ opt.label }}
                  </option>
                </select>
              </div>
            </div>

            <!-- Row: contactEmail + contactPhone -->
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Email контакт
                </label>
                <input
                  v-model="newOrg.contactEmail"
                  type="email"
                  placeholder="admin@university.kz"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Телефон
                </label>
                <input
                  v-model="newOrg.contactPhone"
                  type="tel"
                  placeholder="+7 700 123 4567"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
            </div>

            <!-- Row: city + region -->
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Город
                </label>
                <input
                  v-model="newOrg.city"
                  type="text"
                  placeholder="Алматы"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Регион
                </label>
                <input
                  v-model="newOrg.region"
                  type="text"
                  placeholder="Алматинская область"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
            </div>

            <!-- Row: maxSessions + maxEventsRps -->
            <div class="grid grid-cols-2 gap-3">
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Макс. сессий
                </label>
                <input
                  v-model.number="newOrg.maxSessions"
                  type="number"
                  min="1"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
              <div>
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Макс. событий/сек
                </label>
                <input
                  v-model.number="newOrg.maxEventsRps"
                  type="number"
                  min="1"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>
            </div>

            <!-- ═══ PRIMARY ADMIN SECTION ═══ -->
            <div
              class="pt-4 mt-2 border-t"
              :style="{ borderColor: 'var(--argus-border)' }"
            >
              <div class="flex items-center gap-2 mb-3">
                <div
                  class="flex items-center justify-center size-7 rounded-lg"
                  :style="{ background: successBg(0.1), border: `1px solid ${successBg(0.15)}` }"
                >
                  <UIcon
                    name="i-lucide-user-plus"
                    class="size-3.5"
                    :style="{ color: isDark ? '#34d399' : '#059669' }"
                  />
                </div>
                <div>
                  <p
                    class="text-xs font-bold"
                    style="color: var(--argus-text);"
                  >
                    Primary Admin
                  </p>
                  <p
                    class="text-[10px]"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Администратор организации (org_admin)
                  </p>
                </div>
              </div>

              <!-- Admin Full Name -->
              <div class="mb-3">
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  ФИО администратора *
                </label>
                <input
                  v-model="newOrg.adminFullName"
                  type="text"
                  placeholder="Иванов Иван Иванович"
                  class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: '1px solid var(--argus-border)',
                    color: 'var(--argus-text)'
                  }"
                >
              </div>

              <!-- Admin Phone -->
              <div class="mb-3">
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Телефон (логин) *
                </label>
                <div class="flex items-center gap-0">
                  <span
                    class="px-3 py-2 rounded-l-lg text-xs font-bold"
                    :style="{
                      background: 'var(--argus-bg-hover)',
                      border: '1px solid var(--argus-border)',
                      borderRight: 'none',
                      color: 'var(--argus-text-dimmed)'
                    }"
                  >+7</span>
                  <input
                    v-model="newOrg.adminPhone"
                    type="tel"
                    placeholder="700 123 4567"
                    maxlength="10"
                    class="w-full px-3 py-2 rounded-r-lg text-xs font-medium outline-none transition-all"
                    :style="{
                      background: 'var(--argus-bg-elevated)',
                      border: '1px solid var(--argus-border)',
                      color: 'var(--argus-text)'
                    }"
                  >
                </div>
              </div>

              <!-- Admin Password -->
              <div class="mb-1">
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Пароль *
                </label>
                <div class="flex items-center gap-2">
                  <input
                    v-model="newOrg.adminPassword"
                    type="text"
                    placeholder="Минимум 6 символов"
                    class="flex-1 px-3 py-2 rounded-lg text-xs font-mono font-medium outline-none transition-all"
                    :style="{
                      background: 'var(--argus-bg-elevated)',
                      border: '1px solid var(--argus-border)',
                      color: 'var(--argus-text)'
                    }"
                  >
                  <button
                    type="button"
                    class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-[11px] font-bold cursor-pointer transition-all shrink-0"
                    :style="{
                      background: successBg(0.1),
                      border: `1px solid ${successBg(0.2)}`,
                      color: isDark ? '#34d399' : '#059669'
                    }"
                    @click="fillGeneratedPassword"
                  >
                    <UIcon
                      name="i-lucide-key-round"
                      class="size-3.5"
                    />
                    Сгенерировать
                  </button>
                </div>
              </div>
            </div>

            <!-- Actions -->
            <div
              class="flex items-center justify-end gap-3 pt-4 border-t"
              :style="{ borderColor: 'var(--argus-border)' }"
            >
              <button
                class="px-4 py-2 rounded-lg text-xs font-medium cursor-pointer transition-all"
                :style="{ background: 'var(--argus-bg-elevated)', color: 'var(--argus-text-dimmed)', border: '1px solid var(--argus-border)' }"
                @click="showCreateOrgModal = false"
              >
                Отмена
              </button>
              <button
                class="flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-bold cursor-pointer transition-all"
                :style="{
                  background: 'var(--argus-accent)',
                  color: '#fff',
                  opacity: (createOrgLoading || !isCreateFormValid) ? 0.5 : 1
                }"
                :disabled="createOrgLoading || !isCreateFormValid"
                @click="submitCreateOrg"
              >
                <UIcon
                  v-if="createOrgLoading"
                  name="i-lucide-loader-2"
                  class="size-3.5 animate-spin"
                />
                <UIcon
                  v-else
                  name="i-lucide-plus"
                  class="size-3.5"
                />
                + Создать
              </button>
            </div>
          </div>
        </div>
      </template>
    </UModal>

    <!-- ============================== -->
    <!--  CREDENTIAL POPUP              -->
    <!-- ============================== -->
    <UModal
      v-model:open="showCredentialPopup"
      :close="false"
      :dismissible="false"
    >
      <template #content>
        <div
          class="p-6"
          :style="{ background: 'var(--argus-bg-card)' }"
        >
          <!-- Header -->
          <div class="flex items-center gap-3 mb-5">
            <div
              class="flex items-center justify-center size-10 rounded-xl"
              :style="{ background: isDark ? 'rgba(52,211,153,0.12)' : 'rgba(5,150,105,0.08)' }"
            >
              <UIcon
                name="i-lucide-check-circle"
                class="size-5"
                :style="{ color: isDark ? '#34d399' : '#059669' }"
              />
            </div>
            <div>
              <h3
                class="text-sm font-bold"
                style="color: var(--argus-text);"
              >
                Организация создана
              </h3>
              <p
                class="text-[11px] mt-0.5"
                style="color: var(--argus-text-dimmed);"
              >
                Сохраните учётные данные администратора
              </p>
            </div>
          </div>

          <!-- Credential Card -->
          <div
            class="rounded-xl p-4 space-y-3 border"
            :style="{
              background: isDark ? 'rgba(255,255,255,0.03)' : 'rgba(0,0,0,0.02)',
              borderColor: 'var(--argus-border)'
            }"
          >
            <!-- Org Name -->
            <div>
              <div
                class="text-[10px] uppercase tracking-wider font-semibold mb-1"
                style="color: var(--argus-text-dimmed);"
              >
                Организация
              </div>
              <div
                class="text-xs font-bold"
                style="color: var(--argus-text);"
              >
                {{ createdCredentials.orgName }}
                <span
                  class="font-mono text-[10px] ml-1"
                  style="color: var(--argus-text-dimmed);"
                >
                  ({{ createdCredentials.orgId }})
                </span>
              </div>
            </div>

            <!-- Admin Name -->
            <div>
              <div
                class="text-[10px] uppercase tracking-wider font-semibold mb-1"
                style="color: var(--argus-text-dimmed);"
              >
                Администратор
              </div>
              <div
                class="text-xs font-bold"
                style="color: var(--argus-text);"
              >
                {{ createdCredentials.adminFullName }}
              </div>
            </div>

            <!-- Phone / Login -->
            <div>
              <div
                class="text-[10px] uppercase tracking-wider font-semibold mb-1"
                style="color: var(--argus-text-dimmed);"
              >
                Логин (телефон)
              </div>
              <div
                class="text-xs font-mono font-bold"
                style="color: var(--argus-text);"
              >
                +7{{ createdCredentials.adminPhone }}
              </div>
            </div>

            <!-- Password -->
            <div>
              <div
                class="text-[10px] uppercase tracking-wider font-semibold mb-1"
                style="color: var(--argus-text-dimmed);"
              >
                Пароль
              </div>
              <div
                class="text-xs font-mono font-bold px-2 py-1.5 rounded-lg border select-all"
                :style="{
                  background: isDark ? 'rgba(139,92,246,0.08)' : 'rgba(139,92,246,0.05)',
                  borderColor: isDark ? 'rgba(139,92,246,0.2)' : 'rgba(139,92,246,0.15)',
                  color: isDark ? '#c4b5fd' : '#7c3aed'
                }"
              >
                {{ createdCredentials.adminPassword }}
              </div>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex items-center gap-3 mt-5">
            <button
              class="flex-1 flex items-center justify-center gap-2 px-4 py-2.5 rounded-lg text-xs font-bold cursor-pointer transition-all"
              :style="{
                background: credentialsCopied
                  ? (isDark ? 'rgba(52,211,153,0.15)' : 'rgba(5,150,105,0.1)')
                  : 'var(--argus-accent)',
                color: credentialsCopied
                  ? (isDark ? '#34d399' : '#059669')
                  : '#fff'
              }"
              @click="copyCredentials"
            >
              <UIcon
                :name="credentialsCopied ? 'i-lucide-check' : 'i-lucide-copy'"
                class="size-3.5"
              />
              {{ credentialsCopied ? 'Скопировано!' : 'Скопировать всё' }}
            </button>
            <button
              class="px-4 py-2.5 rounded-lg text-xs font-semibold cursor-pointer transition-all border"
              :style="{
                background: 'transparent',
                color: 'var(--argus-text-dimmed)',
                borderColor: 'var(--argus-border)'
              }"
              @click="showCredentialPopup = false"
            >
              Закрыть
            </button>
          </div>

          <!-- Warning -->
          <div
            class="flex items-start gap-2 mt-4 p-3 rounded-lg"
            :style="{
              background: isDark ? 'rgba(251,191,36,0.08)' : 'rgba(245,158,11,0.06)',
              border: `1px solid ${isDark ? 'rgba(251,191,36,0.15)' : 'rgba(245,158,11,0.12)'}`
            }"
          >
            <UIcon
              name="i-lucide-alert-triangle"
              class="size-3.5 shrink-0 mt-0.5"
              :style="{ color: isDark ? '#fbbf24' : '#d97706' }"
            />
            <span
              class="text-[11px] leading-relaxed"
              :style="{ color: isDark ? '#fbbf24' : '#d97706' }"
            >
              Пароль показывается только один раз. Убедитесь, что вы его сохранили.
            </span>
          </div>
        </div>
      </template>
    </UModal>

    <!-- ============================== -->
    <!--  DELETE ORG CONFIRMATION MODAL -->
    <!-- ============================== -->
    <Teleport to="body">
      <Transition name="fade">
        <div
          v-if="showDeleteOrgModal && orgToDelete"
          class="fixed inset-0 z-[200] flex items-center justify-center p-4"
        >
          <!-- Backdrop -->
          <div
            class="absolute inset-0"
            :style="{ background: 'rgba(0, 0, 0, 0.6)', backdropFilter: 'blur(4px)' }"
            @click="cancelDeleteOrg"
          />

          <!-- Modal Card -->
          <div
            class="relative z-10 w-full max-w-md rounded-2xl border overflow-hidden"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
            @click.stop
          >
            <!-- Header -->
            <div class="flex items-center gap-3 px-6 py-5">
              <div
                class="flex items-center justify-center size-11 rounded-xl shrink-0"
                :style="{ background: errorBg(0.1), border: `1px solid ${errorBg(0.15)}` }"
              >
                <UIcon
                  name="i-lucide-alert-triangle"
                  class="size-5"
                  :style="{ color: isDark ? '#f87171' : '#dc2626' }"
                />
              </div>
              <div>
                <h3
                  class="text-sm font-bold"
                  style="color: var(--argus-text);"
                >
                  Удалить организацию?
                </h3>
                <p
                  class="text-[11px] mt-0.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Это действие скроет организацию и заблокирует доступ
                </p>
              </div>
            </div>

            <!-- Content: Org preview card -->
            <div class="px-6 pb-4">
              <div
                class="p-4 rounded-xl border"
                :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border-subtle)' }"
              >
                <div class="flex items-center justify-between">
                  <div>
                    <p
                      class="text-xs font-bold"
                      style="color: var(--argus-text);"
                    >
                      {{ orgToDelete.name }}
                    </p>
                    <div class="flex items-center gap-2 mt-1">
                      <span
                        class="text-[10px] font-mono"
                        style="color: var(--argus-text-dimmed);"
                      >{{ orgToDelete.orgId }}</span>
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-border);"
                      >|</span>
                      <span
                        class="text-[10px] font-mono"
                        style="color: var(--argus-text-dimmed);"
                      >{{ orgToDelete.slug }}</span>
                    </div>
                  </div>
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-bold uppercase"
                    :style="{ background: errorBg(0.1), color: isDark ? '#f87171' : '#dc2626' }"
                  >
                    Soft Delete
                  </span>
                </div>
                <div
                  v-if="orgToDelete.city || orgToDelete.contactEmail"
                  class="flex items-center gap-3 mt-2"
                >
                  <div
                    v-if="orgToDelete.city"
                    class="flex items-center gap-1"
                  >
                    <UIcon
                      name="i-lucide-map-pin"
                      class="size-2.5"
                      style="color: var(--argus-text-dimmed);"
                    />
                    <span
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >{{ orgToDelete.city }}</span>
                  </div>
                  <div
                    v-if="orgToDelete.contactEmail"
                    class="flex items-center gap-1"
                  >
                    <UIcon
                      name="i-lucide-mail"
                      class="size-2.5"
                      style="color: var(--argus-text-dimmed);"
                    />
                    <span
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >{{ orgToDelete.contactEmail }}</span>
                  </div>
                </div>
              </div>

              <p
                class="text-[11px] mt-3 leading-relaxed"
                style="color: var(--argus-text-muted);"
              >
                Организация будет деактивирована (soft-delete). Все активные сессии и API-ключи будут заблокированы.
                Исторические данные и evidence сохранятся для юридической отчётности.
              </p>

              <!-- Slug confirmation input -->
              <div class="mt-4">
                <label
                  class="block text-[11px] font-medium mb-1.5"
                  style="color: var(--argus-text-dimmed);"
                >
                  Введите slug организации <span
                    class="font-bold font-mono"
                    :style="{ color: isDark ? '#f87171' : '#dc2626' }"
                  >{{ orgToDelete.slug }}</span> для подтверждения:
                </label>
                <input
                  v-model="deleteConfirmSlug"
                  type="text"
                  :placeholder="orgToDelete.slug"
                  class="w-full px-3 py-2 rounded-lg text-xs font-mono font-medium outline-none transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    border: `1px solid ${isDeleteConfirmed ? (isDark ? '#f87171' : '#dc2626') : 'var(--argus-border)'}`,
                    color: 'var(--argus-text)'
                  }"
                  @keydown.enter="isDeleteConfirmed && confirmDeleteOrg()"
                >
              </div>
            </div>

            <!-- Actions -->
            <div
              class="flex items-center justify-end gap-3 px-6 py-4 border-t"
              :style="{ borderColor: 'var(--argus-border)' }"
            >
              <button
                class="px-4 py-2 rounded-lg text-xs font-medium cursor-pointer transition-all"
                :style="{ background: 'var(--argus-bg-elevated)', color: 'var(--argus-text-dimmed)', border: '1px solid var(--argus-border)' }"
                @click="cancelDeleteOrg"
              >
                Отмена
              </button>
              <button
                class="flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-bold transition-all"
                :style="{
                  background: isDark ? '#dc2626' : '#dc2626',
                  color: '#fff',
                  opacity: (!isDeleteConfirmed || deleteOrgLoading) ? 0.4 : 1,
                  cursor: (!isDeleteConfirmed || deleteOrgLoading) ? 'not-allowed' : 'pointer'
                }"
                :disabled="!isDeleteConfirmed || deleteOrgLoading"
                @click="confirmDeleteOrg"
              >
                <UIcon
                  v-if="deleteOrgLoading"
                  name="i-lucide-loader-2"
                  class="size-3.5 animate-spin"
                />
                <UIcon
                  v-else
                  name="i-lucide-trash-2"
                  class="size-3.5"
                />
                Удалить навсегда
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- ============================== -->
    <!--  ORG DETAIL MODAL              -->
    <!-- ============================== -->
    <UModal
      :open="!!selectedOrg"
      :close="false"
      @update:open="(v: boolean) => { if (!v) selectedOrg = null }"
    >
      <template #content>
        <div
          v-if="selectedOrg"
          class="max-h-[85vh] overflow-y-auto"
          :style="{ background: 'var(--argus-bg-card)' }"
        >
          <!-- Detail Header -->
          <div
            class="sticky top-0 z-10 px-6 py-4 border-b"
            :style="{ background: 'var(--argus-bg-card)', borderColor: 'var(--argus-border)' }"
          >
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-3 min-w-0">
                <div
                  class="flex items-center justify-center size-10 rounded-xl shrink-0"
                  :style="{ background: planBg(selectedOrg.plan, 0.1), border: `1px solid ${planBg(selectedOrg.plan, 0.2)}` }"
                >
                  <UIcon
                    name="i-lucide-building-2"
                    class="size-5"
                    :style="{ color: planTextColor(selectedOrg.plan) }"
                  />
                </div>
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <h2
                      class="text-base font-bold truncate"
                      style="color: var(--argus-text);"
                    >
                      {{ selectedOrg.name }}
                    </h2>
                    <span
                      class="size-2 rounded-full shrink-0"
                      :style="{ background: selectedOrg.isActive ? (isDark ? '#34d399' : '#059669') : (isDark ? '#f87171' : '#dc2626') }"
                    />
                    <span
                      class="px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider shrink-0"
                      :style="{
                        background: planBg(selectedOrg.plan, 0.1),
                        color: planTextColor(selectedOrg.plan),
                        border: `1px solid ${planBg(selectedOrg.plan, 0.2)}`
                      }"
                    >
                      {{ planLabel(selectedOrg.plan) }}
                    </span>
                  </div>
                  <div class="flex items-center gap-2 mt-0.5">
                    <span
                      class="text-[10px] font-mono"
                      style="color: var(--argus-text-dimmed);"
                    >{{ selectedOrg.orgId }}</span>
                    <span
                      class="text-[10px]"
                      style="color: var(--argus-border);"
                    >|</span>
                    <span
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >{{ selectedOrg.city || '---' }}</span>
                    <span
                      class="text-[10px]"
                      style="color: var(--argus-border);"
                    >|</span>
                    <span
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >{{ selectedOrg.contactEmail || '---' }}</span>
                  </div>
                </div>
              </div>
              <button
                class="flex items-center justify-center size-8 rounded-lg cursor-pointer transition-all"
                :style="{ color: 'var(--argus-text-dimmed)' }"
                @click="closeOrgDetail"
              >
                <UIcon
                  name="i-lucide-x"
                  class="size-4"
                />
              </button>
            </div>

            <!-- Tabs -->
            <div class="flex items-center gap-1 mt-4">
              <button
                v-for="tab in ([
                  { id: 'users' as const, label: 'Пользователи', icon: 'i-lucide-users' },
                  { id: 'keys' as const, label: 'API Ключи', icon: 'i-lucide-key-round' },
                  { id: 'settings' as const, label: 'Настройки', icon: 'i-lucide-settings' },
                  { id: 'features' as const, label: 'Возможности', icon: 'i-lucide-toggle-right' }
                ])"
                :key="tab.id"
                class="flex items-center gap-1.5 px-3 py-2 rounded-lg text-xs font-medium transition-all cursor-pointer"
                :style="{
                  background: detailTab === tab.id ? accentBg(0.1) : 'transparent',
                  color: detailTab === tab.id ? 'var(--argus-accent)' : 'var(--argus-text-dimmed)',
                  border: detailTab === tab.id ? `1px solid ${accentBg(0.2)}` : '1px solid transparent'
                }"
                @click="switchTab(tab.id)"
              >
                <UIcon
                  :name="tab.icon"
                  class="size-3.5"
                />
                {{ tab.label }}
              </button>
            </div>
          </div>

          <!-- Tab Content -->
          <div class="p-6">
            <!-- ======================== -->
            <!--  TAB: USERS              -->
            <!-- ======================== -->
            <div v-if="detailTab === 'users'">
              <!-- Create user toggle -->
              <div class="flex items-center justify-between mb-4">
                <h3
                  class="text-sm font-bold"
                  style="color: var(--argus-text);"
                >
                  Пользователи организации
                  <span
                    class="font-normal text-xs ml-1"
                    style="color: var(--argus-text-dimmed);"
                  >({{ orgUsers.length }})</span>
                </h3>
                <button
                  class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[11px] font-bold cursor-pointer transition-all"
                  :style="{ background: accentBg(0.1), color: 'var(--argus-accent)', border: `1px solid ${accentBg(0.2)}` }"
                  @click="openCreateUserForm"
                >
                  <UIcon
                    name="i-lucide-user-plus"
                    class="size-3.5"
                  />
                  Добавить
                </button>
              </div>

              <!-- Create User Form -->
              <div
                v-if="showCreateUserForm"
                class="mb-4 p-4 rounded-xl border space-y-3"
                :style="{ background: 'var(--argus-bg-elevated)', borderColor: accentBg(0.2) }"
              >
                <div class="flex items-center gap-2 mb-2">
                  <UIcon
                    name="i-lucide-user-plus"
                    class="size-4"
                    style="color: var(--argus-accent);"
                  />
                  <span
                    class="text-xs font-bold"
                    style="color: var(--argus-text);"
                  >Новый пользователь</span>
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >ФИО *</label>
                    <input
                      v-model="newUser.fullName"
                      type="text"
                      placeholder="Иванов Иван Иванович"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Роль *</label>
                    <select
                      v-model="newUser.role"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none cursor-pointer"
                      :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                      <option
                        v-for="opt in roleOptions"
                        :key="opt.value"
                        :value="opt.value"
                      >
                        {{ opt.label }}
                      </option>
                    </select>
                  </div>
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Телефон *</label>
                    <input
                      v-model="newUser.phone"
                      type="tel"
                      placeholder="+7 700 123 4567"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Пароль *</label>
                    <input
                      v-model="newUser.password"
                      type="password"
                      placeholder="Минимум 8 символов"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                </div>

                <div>
                  <label
                    class="block text-[10px] font-medium mb-1"
                    style="color: var(--argus-text-dimmed);"
                  >Email</label>
                  <input
                    v-model="newUser.email"
                    type="email"
                    placeholder="user@example.com"
                    class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                    :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                  >
                </div>

                <div class="flex items-center justify-end gap-2 pt-2">
                  <button
                    class="px-3 py-1.5 rounded-lg text-[11px] font-medium cursor-pointer"
                    :style="{ color: 'var(--argus-text-dimmed)' }"
                    @click="showCreateUserForm = false"
                  >
                    Отмена
                  </button>
                  <button
                    class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[11px] font-bold cursor-pointer"
                    :style="{ background: 'var(--argus-accent)', color: '#fff', opacity: createUserLoading ? 0.7 : 1 }"
                    :disabled="createUserLoading"
                    @click="submitCreateUser"
                  >
                    <UIcon
                      v-if="createUserLoading"
                      name="i-lucide-loader-2"
                      class="size-3 animate-spin"
                    />
                    Создать
                  </button>
                </div>
              </div>

              <!-- Users Loading -->
              <div
                v-if="usersLoading"
                class="flex items-center justify-center py-10"
              >
                <UIcon
                  name="i-lucide-loader-2"
                  class="size-5 animate-spin"
                  style="color: var(--argus-accent);"
                />
              </div>

              <!-- Users List -->
              <div
                v-else-if="orgUsers.length > 0"
                class="space-y-2"
              >
                <div
                  v-for="u in orgUsers"
                  :key="u.id"
                  class="flex items-center justify-between px-4 py-3 rounded-xl border transition-all"
                  :style="{ background: 'var(--argus-bg-elevated)', borderColor: 'var(--argus-border-subtle)' }"
                >
                  <div class="flex items-center gap-3 min-w-0">
                    <div
                      class="flex items-center justify-center size-8 rounded-lg shrink-0"
                      :style="{ background: roleBg(u.role, 0.1) }"
                    >
                      <UIcon
                        name="i-lucide-user"
                        class="size-3.5"
                        :style="{ color: roleTextColor(u.role) }"
                      />
                    </div>
                    <div class="min-w-0">
                      <div class="flex items-center gap-2">
                        <span
                          class="text-xs font-bold truncate"
                          style="color: var(--argus-text);"
                        >{{ u.fullName }}</span>
                        <span
                          class="px-1.5 py-0.5 rounded text-[9px] font-bold uppercase tracking-wider shrink-0"
                          :style="{
                            background: roleBg(u.role, 0.1),
                            color: roleTextColor(u.role),
                            border: `1px solid ${roleBg(u.role, 0.2)}`
                          }"
                        >
                          {{ roleLabel(u.role) }}
                        </span>
                        <span
                          class="size-1.5 rounded-full shrink-0"
                          :style="{ background: u.isActive ? (isDark ? '#34d399' : '#059669') : (isDark ? '#f87171' : '#dc2626') }"
                        />
                      </div>
                      <div class="flex items-center gap-2 mt-0.5">
                        <span
                          class="text-[10px]"
                          style="color: var(--argus-text-dimmed);"
                        >{{ u.phone }}</span>
                        <span
                          v-if="u.email"
                          class="text-[10px]"
                          style="color: var(--argus-text-dimmed);"
                        >{{ u.email }}</span>
                      </div>
                    </div>
                  </div>

                  <div class="text-right shrink-0 ml-3">
                    <div
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >
                      Вход: {{ u.lastLoginAt ? formatDateTime(u.lastLoginAt) : '—' }}
                    </div>
                    <div
                      class="text-[10px]"
                      style="color: var(--argus-text-dimmed);"
                    >
                      Создан: {{ formatDate(u.createdAt) }}
                    </div>
                  </div>
                </div>
              </div>

              <!-- No Users -->
              <div
                v-else
                class="flex flex-col items-center justify-center py-10 gap-3"
              >
                <UIcon
                  name="i-lucide-users"
                  class="size-8"
                  style="color: var(--argus-text-dimmed); opacity: 0.3;"
                />
                <p
                  class="text-xs"
                  style="color: var(--argus-text-dimmed);"
                >
                  Нет пользователей
                </p>
              </div>
            </div>

            <!-- ======================== -->
            <!--  TAB: API KEYS           -->
            <!-- ======================== -->
            <div v-if="detailTab === 'keys'">
              <!-- Create key toggle -->
              <div class="flex items-center justify-between mb-4">
                <h3
                  class="text-sm font-bold"
                  style="color: var(--argus-text);"
                >
                  API Ключи
                  <span
                    class="font-normal text-xs ml-1"
                    style="color: var(--argus-text-dimmed);"
                  >({{ orgApiKeys.length }})</span>
                </h3>
                <button
                  class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[11px] font-bold cursor-pointer transition-all"
                  :style="{ background: purpleBg(0.1), color: isDark ? '#a78bfa' : '#7c3aed', border: `1px solid ${purpleBg(0.2)}` }"
                  @click="openCreateKeyForm"
                >
                  <UIcon
                    name="i-lucide-key-round"
                    class="size-3.5"
                  />
                  Создать ключ
                </button>
              </div>

              <!-- Newly Created Secret Banner -->
              <div
                v-if="newlyCreatedSecret"
                class="mb-4 p-4 rounded-xl border space-y-2"
                :style="{ background: warningBg(0.05), borderColor: warningBg(0.3) }"
              >
                <div class="flex items-center gap-2">
                  <UIcon
                    name="i-lucide-alert-triangle"
                    class="size-4"
                    :style="{ color: isDark ? '#fbbf24' : '#d97706' }"
                  />
                  <span
                    class="text-xs font-bold"
                    :style="{ color: isDark ? '#fbbf24' : '#d97706' }"
                  >
                    Секретный ключ создан! Сохраните его сейчас.
                  </span>
                </div>
                <p
                  class="text-[10px]"
                  style="color: var(--argus-text-dimmed);"
                >
                  Этот секрет отображается только один раз. Скопируйте и сохраните его в безопасном месте.
                </p>
                <div
                  class="flex items-center gap-2 p-3 rounded-lg font-mono text-xs break-all"
                  :style="{ background: 'var(--argus-bg-deep)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                >
                  <span class="flex-1 select-all">{{ newlyCreatedSecret }}</span>
                  <button
                    class="flex items-center justify-center size-7 rounded-md shrink-0 cursor-pointer transition-all"
                    :style="{ background: accentBg(0.1), color: 'var(--argus-accent)' }"
                    title="Скопировать"
                    @click="copyToClipboard(newlyCreatedSecret!)"
                  >
                    <UIcon
                      name="i-lucide-copy"
                      class="size-3.5"
                    />
                  </button>
                </div>
                <button
                  class="text-[10px] font-medium cursor-pointer mt-1"
                  :style="{ color: 'var(--argus-text-dimmed)' }"
                  @click="newlyCreatedSecret = null"
                >
                  Закрыть
                </button>
              </div>

              <!-- Create Key Form -->
              <div
                v-if="showCreateKeyForm && !newlyCreatedSecret"
                class="mb-4 p-4 rounded-xl border space-y-3"
                :style="{ background: 'var(--argus-bg-elevated)', borderColor: purpleBg(0.2) }"
              >
                <div class="flex items-center gap-2 mb-2">
                  <UIcon
                    name="i-lucide-key-round"
                    class="size-4"
                    :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
                  />
                  <span
                    class="text-xs font-bold"
                    style="color: var(--argus-text);"
                  >Новый API ключ</span>
                </div>

                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Название *</label>
                    <input
                      v-model="newKey.name"
                      type="text"
                      placeholder="Production API Key"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Среда</label>
                    <select
                      v-model="newKey.environment"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none cursor-pointer"
                      :style="{ background: 'var(--argus-bg-card)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                      <option value="live">
                        Live
                      </option>
                      <option value="test">
                        Test
                      </option>
                    </select>
                  </div>
                </div>

                <!-- Permissions -->
                <div>
                  <label
                    class="block text-[10px] font-medium mb-2"
                    style="color: var(--argus-text-dimmed);"
                  >Разрешения</label>
                  <div class="flex flex-wrap gap-2">
                    <button
                      v-for="perm in permissionOptions"
                      :key="perm.value"
                      class="px-2.5 py-1 rounded-md text-[10px] font-medium cursor-pointer transition-all"
                      :style="{
                        background: newKey.permissions.includes(perm.value) ? purpleBg(0.15) : 'var(--argus-bg-card)',
                        color: newKey.permissions.includes(perm.value) ? (isDark ? '#a78bfa' : '#7c3aed') : 'var(--argus-text-dimmed)',
                        border: `1px solid ${newKey.permissions.includes(perm.value) ? purpleBg(0.3) : 'var(--argus-border)'}`
                      }"
                      @click="togglePermission(perm.value)"
                    >
                      {{ perm.label }}
                    </button>
                  </div>
                </div>

                <div class="flex items-center justify-end gap-2 pt-2">
                  <button
                    class="px-3 py-1.5 rounded-lg text-[11px] font-medium cursor-pointer"
                    :style="{ color: 'var(--argus-text-dimmed)' }"
                    @click="showCreateKeyForm = false"
                  >
                    Отмена
                  </button>
                  <button
                    class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[11px] font-bold cursor-pointer"
                    :style="{ background: isDark ? '#a78bfa' : '#7c3aed', color: '#fff', opacity: createKeyLoading ? 0.7 : 1 }"
                    :disabled="createKeyLoading"
                    @click="submitCreateKey"
                  >
                    <UIcon
                      v-if="createKeyLoading"
                      name="i-lucide-loader-2"
                      class="size-3 animate-spin"
                    />
                    Создать
                  </button>
                </div>
              </div>

              <!-- Keys Loading -->
              <div
                v-if="keysLoading"
                class="flex items-center justify-center py-10"
              >
                <UIcon
                  name="i-lucide-loader-2"
                  class="size-5 animate-spin"
                  :style="{ color: isDark ? '#a78bfa' : '#7c3aed' }"
                />
              </div>

              <!-- Keys List -->
              <div
                v-else-if="orgApiKeys.length > 0"
                class="space-y-2"
              >
                <div
                  v-for="k in orgApiKeys"
                  :key="k.id"
                  class="flex items-center justify-between px-4 py-3 rounded-xl border transition-all"
                  :style="{
                    background: 'var(--argus-bg-elevated)',
                    borderColor: k.isActive ? 'var(--argus-border-subtle)' : errorBg(0.2),
                    opacity: k.isActive ? 1 : 0.6
                  }"
                >
                  <div class="flex items-center gap-3 min-w-0">
                    <div
                      class="flex items-center justify-center size-8 rounded-lg shrink-0"
                      :style="{ background: k.isActive ? purpleBg(0.1) : errorBg(0.1) }"
                    >
                      <UIcon
                        :name="k.isActive ? 'i-lucide-key-round' : 'i-lucide-key-round'"
                        class="size-3.5"
                        :style="{ color: k.isActive ? (isDark ? '#a78bfa' : '#7c3aed') : (isDark ? '#f87171' : '#dc2626') }"
                      />
                    </div>
                    <div class="min-w-0">
                      <div class="flex items-center gap-2">
                        <span
                          class="text-xs font-bold truncate"
                          style="color: var(--argus-text);"
                        >{{ k.name }}</span>
                        <span
                          v-if="!k.isActive"
                          class="px-1.5 py-0.5 rounded text-[9px] font-bold uppercase"
                          :style="{ background: errorBg(0.1), color: isDark ? '#f87171' : '#dc2626' }"
                        >
                          Отозван
                        </span>
                      </div>
                      <div class="flex items-center gap-2 mt-0.5 flex-wrap">
                        <span
                          class="text-[10px] font-mono"
                          style="color: var(--argus-text-dimmed);"
                        >
                          {{ k.keyId }}
                        </span>
                        <span
                          class="text-[10px]"
                          style="color: var(--argus-border);"
                        >|</span>
                        <span
                          class="text-[10px] font-mono"
                          style="color: var(--argus-text-dimmed);"
                        >
                          {{ k.secretPrefix }}***
                        </span>
                        <span
                          v-if="k.permissions.length > 0"
                          class="text-[10px]"
                          style="color: var(--argus-border);"
                        >|</span>
                        <span
                          v-for="perm in k.permissions"
                          :key="perm"
                          class="px-1 py-0 rounded text-[9px]"
                          :style="{ background: 'var(--argus-bg-card)', color: 'var(--argus-text-dimmed)' }"
                        >
                          {{ perm }}
                        </span>
                      </div>
                    </div>
                  </div>

                  <div class="flex items-center gap-3 shrink-0 ml-3">
                    <div class="text-right">
                      <div
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >
                        Исп.: {{ k.lastUsedAt ? formatDateTime(k.lastUsedAt) : '—' }}
                      </div>
                      <div
                        v-if="k.expiresAt"
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >
                        Истекает: {{ formatDate(k.expiresAt) }}
                      </div>
                    </div>
                    <button
                      v-if="k.isActive"
                      class="flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-[10px] font-bold cursor-pointer transition-all"
                      :style="{
                        background: errorBg(0.1),
                        color: isDark ? '#f87171' : '#dc2626',
                        border: `1px solid ${errorBg(0.2)}`,
                        opacity: revokeLoadingId === k.id ? 0.7 : 1
                      }"
                      :disabled="revokeLoadingId === k.id"
                      @click="revokeKey(k.id)"
                    >
                      <UIcon
                        v-if="revokeLoadingId === k.id"
                        name="i-lucide-loader-2"
                        class="size-3 animate-spin"
                      />
                      <UIcon
                        v-else
                        name="i-lucide-shield-off"
                        class="size-3"
                      />
                      Отозвать
                    </button>
                  </div>
                </div>
              </div>

              <!-- No Keys -->
              <div
                v-else
                class="flex flex-col items-center justify-center py-10 gap-3"
              >
                <UIcon
                  name="i-lucide-key-round"
                  class="size-8"
                  style="color: var(--argus-text-dimmed); opacity: 0.3;"
                />
                <p
                  class="text-xs"
                  style="color: var(--argus-text-dimmed);"
                >
                  Нет API ключей
                </p>
              </div>
            </div>

            <!-- ======================== -->
            <!--  TAB: SETTINGS           -->
            <!-- ======================== -->
            <div v-if="detailTab === 'settings'">
              <h3
                class="text-sm font-bold mb-4"
                style="color: var(--argus-text);"
              >
                Настройки организации
              </h3>

              <div class="space-y-4">
                <!-- Name + Slug -->
                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Название</label>
                    <input
                      v-model="editOrgData.name"
                      type="text"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Slug</label>
                    <input
                      v-model="editOrgData.slug"
                      type="text"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                </div>

                <!-- Type + Plan -->
                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Тип организации</label>
                    <select
                      v-model="editOrgData.orgType"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none cursor-pointer"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                      <option
                        v-for="opt in orgTypeOptions"
                        :key="opt.value"
                        :value="opt.value"
                      >
                        {{ opt.label }}
                      </option>
                    </select>
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Тарифный план</label>
                    <select
                      v-model="editOrgData.plan"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none cursor-pointer"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                      <option
                        v-for="opt in planOptions"
                        :key="opt.value"
                        :value="opt.value"
                      >
                        {{ opt.label }}
                      </option>
                    </select>
                  </div>
                </div>

                <!-- Email + Phone -->
                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Email контакт</label>
                    <input
                      v-model="editOrgData.contactEmail"
                      type="email"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Телефон</label>
                    <input
                      v-model="editOrgData.contactPhone"
                      type="tel"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                </div>

                <!-- City + Region -->
                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Город</label>
                    <input
                      v-model="editOrgData.city"
                      type="text"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Регион</label>
                    <input
                      v-model="editOrgData.region"
                      type="text"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                </div>

                <!-- Limits -->
                <div class="grid grid-cols-2 gap-3">
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Макс. сессий</label>
                    <input
                      v-model.number="editOrgData.maxSessions"
                      type="number"
                      min="1"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                  <div>
                    <label
                      class="block text-[10px] font-medium mb-1"
                      style="color: var(--argus-text-dimmed);"
                    >Макс. событий/сек</label>
                    <input
                      v-model.number="editOrgData.maxEventsRps"
                      type="number"
                      min="1"
                      class="w-full px-3 py-2 rounded-lg text-xs font-medium outline-none"
                      :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border)', color: 'var(--argus-text)' }"
                    >
                  </div>
                </div>

                <!-- Org Info (read-only) -->
                <div
                  class="p-3 rounded-lg space-y-1.5"
                  :style="{ background: 'var(--argus-bg-elevated)', border: '1px solid var(--argus-border-subtle)' }"
                >
                  <p
                    class="text-[10px] font-bold uppercase tracking-wider mb-2"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Информация
                  </p>
                  <div class="grid grid-cols-2 gap-x-4 gap-y-1">
                    <div class="flex items-center justify-between">
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >ID:</span>
                      <span
                        class="text-[10px] font-mono"
                        style="color: var(--argus-text-muted);"
                      >{{ selectedOrg.orgId }}</span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Статус:</span>
                      <span
                        class="text-[10px] font-bold"
                        :style="{ color: selectedOrg.isActive ? (isDark ? '#34d399' : '#059669') : (isDark ? '#f87171' : '#dc2626') }"
                      >
                        {{ selectedOrg.isActive ? 'Активна' : 'Неактивна' }}
                      </span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Создана:</span>
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-muted);"
                      >{{ formatDateTime(selectedOrg.createdAt) }}</span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Обновлена:</span>
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-muted);"
                      >{{ formatDateTime(selectedOrg.updatedAt) }}</span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >Хранение данных:</span>
                      <span
                        class="text-[10px]"
                        style="color: var(--argus-text-muted);"
                      >{{ selectedOrg.retentionDays }} дней</span>
                    </div>
                  </div>
                </div>

                <!-- Save Button -->
                <div
                  class="flex items-center justify-end gap-3 pt-4 border-t"
                  :style="{ borderColor: 'var(--argus-border)' }"
                >
                  <button
                    class="flex items-center gap-2 px-4 py-2 rounded-lg text-xs font-bold cursor-pointer transition-all"
                    :style="{ background: 'var(--argus-accent)', color: '#fff', opacity: editOrgLoading ? 0.7 : 1 }"
                    :disabled="editOrgLoading"
                    @click="submitUpdateOrg"
                  >
                    <UIcon
                      v-if="editOrgLoading"
                      name="i-lucide-loader-2"
                      class="size-3.5 animate-spin"
                    />
                    <UIcon
                      v-else
                      name="i-lucide-save"
                      class="size-3.5"
                    />
                    Сохранить изменения
                  </button>
                </div>
              </div>
            </div>

            <!-- ======================== -->
            <!--  TAB: FEATURES           -->
            <!-- ======================== -->
            <div
              v-if="detailTab === 'features'"
              class="space-y-4"
            >
              <!-- Header -->
              <div class="flex items-center justify-between">
                <div>
                  <h4
                    class="text-sm font-bold"
                    style="color: var(--argus-text);"
                  >
                    Feature Toggles
                  </h4>
                  <p
                    class="text-[11px] mt-0.5"
                    style="color: var(--argus-text-dimmed);"
                  >
                    Управление доступными правилами прокторинга для организации
                  </p>
                </div>
                <button
                  class="flex items-center gap-2 px-3 py-1.5 rounded-lg text-[11px] font-bold transition-all cursor-pointer"
                  :style="{ background: 'var(--argus-accent)', color: '#fff' }"
                  :disabled="featureSaving"
                  @click="saveFeatureToggles"
                >
                  <UIcon
                    :name="featureSaving ? 'i-lucide-loader-2' : 'i-lucide-save'"
                    class="size-3"
                    :class="{ 'animate-spin': featureSaving }"
                  />
                  {{ featureSaving ? 'Сохранение...' : 'Сохранить' }}
                </button>
              </div>

              <!-- Feature Groups -->
              <div
                v-for="group in featureGroups"
                :key="group.id"
                class="rounded-xl border overflow-hidden"
                :style="{ borderColor: 'var(--argus-border)', background: 'var(--argus-bg-elevated)' }"
              >
                <!-- Group Header -->
                <div
                  class="flex items-center justify-between px-4 py-3 cursor-pointer"
                  :style="{ background: 'var(--argus-bg-card)' }"
                >
                  <div class="flex items-center gap-3">
                    <div
                      class="flex items-center justify-center size-8 rounded-lg"
                      :style="{ background: `color-mix(in srgb, ${group.color} 10%, transparent)` }"
                    >
                      <UIcon
                        :name="group.icon"
                        class="size-4"
                        :style="{ color: group.color }"
                      />
                    </div>
                    <div>
                      <p
                        class="text-xs font-bold"
                        style="color: var(--argus-text);"
                      >
                        {{ group.label }}
                      </p>
                      <p
                        v-if="group.description"
                        class="text-[9px] max-w-xs leading-tight"
                        :style="{ color: group.color }"
                      >
                        {{ group.description }}
                      </p>
                      <p
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >
                        {{ countEnabledInGroup(group) }} из {{ group.features.length }} включено
                      </p>
                    </div>
                  </div>

                  <!-- Group toggle -->
                  <button
                    class="relative w-10 h-5 rounded-full transition-all cursor-pointer"
                    :style="{
                      background: isGroupFullyEnabled(group) ? group.color : isGroupPartiallyEnabled(group) ? `color-mix(in srgb, ${group.color} 40%, var(--argus-bg-elevated))` : 'var(--argus-bg-elevated)',
                      border: `1px solid ${isGroupFullyEnabled(group) ? group.color : 'var(--argus-border)'}`
                    }"
                    @click="toggleGroup(group, !isGroupFullyEnabled(group))"
                  >
                    <span
                      class="absolute top-0.5 w-3.5 h-3.5 rounded-full transition-all"
                      :style="{
                        left: isGroupFullyEnabled(group) ? '22px' : '2px',
                        background: isGroupFullyEnabled(group) ? '#fff' : 'var(--argus-text-dimmed)'
                      }"
                    />
                  </button>
                </div>

                <!-- Individual Features -->
                <div
                  class="divide-y"
                  :style="{ borderColor: 'var(--argus-border)' }"
                >
                  <div
                    v-for="feature in group.features"
                    :key="feature.key"
                    class="flex items-center justify-between px-4 py-2.5"
                  >
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center gap-1.5">
                        <p
                          class="text-[11px] font-medium"
                          style="color: var(--argus-text);"
                        >
                          {{ feature.label }}
                        </p>
                        <span
                          v-if="feature.badge"
                          class="shrink-0 px-1.5 py-px rounded text-[8px] font-bold uppercase tracking-wide"
                          :style="{ background: `color-mix(in srgb, ${group.color} 12%, transparent)`, color: group.color, border: `1px solid color-mix(in srgb, ${group.color} 20%, transparent)` }"
                        >{{ feature.badge }}</span>
                      </div>
                      <p
                        class="text-[10px]"
                        style="color: var(--argus-text-dimmed);"
                      >
                        {{ feature.description }}
                      </p>
                    </div>

                    <!-- Individual toggle -->
                    <button
                      class="relative w-9 h-4.5 rounded-full transition-all cursor-pointer shrink-0 ml-3"
                      :style="{
                        background: featureToggles[feature.key] ? group.color : 'var(--argus-bg-elevated)',
                        border: `1px solid ${featureToggles[feature.key] ? group.color : 'var(--argus-border)'}`
                      }"
                      @click="featureToggles[feature.key] = !featureToggles[feature.key]"
                    >
                      <span
                        class="absolute top-0.5 w-3 h-3 rounded-full transition-all"
                        :style="{
                          left: featureToggles[feature.key] ? '18px' : '2px',
                          background: featureToggles[feature.key] ? '#fff' : 'var(--argus-text-dimmed)'
                        }"
                      />
                    </button>
                  </div>
                </div>
              </div>

              <!-- Quick Actions -->
              <div class="flex items-center gap-3 pt-2">
                <button
                  class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[10px] font-medium transition-all cursor-pointer"
                  :style="{ background: successBg(0.08), color: isDark ? '#34d399' : '#059669', border: `1px solid ${successBg(0.15)}` }"
                  @click="Object.keys(featureToggles).forEach(k => (featureToggles as any)[k] = true)"
                >
                  <UIcon
                    name="i-lucide-check-circle"
                    class="size-3"
                  />
                  Включить все
                </button>
                <button
                  class="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-[10px] font-medium transition-all cursor-pointer"
                  :style="{ background: errorBg(0.08), color: isDark ? '#f87171' : '#dc2626', border: `1px solid ${errorBg(0.15)}` }"
                  @click="Object.keys(featureToggles).forEach(k => (featureToggles as any)[k] = false)"
                >
                  <UIcon
                    name="i-lucide-x-circle"
                    class="size-3"
                  />
                  Отключить все
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </UModal>
  </div>

  <!-- Not Super Admin fallback -->
  <div
    v-else
    class="min-h-screen flex items-center justify-center"
    :style="{ background: 'var(--argus-bg-deep)' }"
  >
    <div class="text-center space-y-3">
      <UIcon
        name="i-lucide-shield-x"
        class="size-12 mx-auto"
        style="color: var(--argus-text-dimmed); opacity: 0.4;"
      />
      <p
        class="text-sm font-medium"
        style="color: var(--argus-text-dimmed);"
      >
        Доступ запрещён
      </p>
      <p
        class="text-xs"
        style="color: var(--argus-text-dimmed);"
      >
        Эта страница доступна только для Super Admin
      </p>
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
