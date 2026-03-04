<script setup lang="ts">
// =============================================================================
// OrgSwitcher — Global Organization Switcher (Super Admin only)
// =============================================================================
//
// Allows Super Admins to switch the active organization context globally.
// When an org is selected, all dashboard data filters to that org_id.
// Selecting "Все организации" resets to the aggregate (all-orgs) view.
//
// Placement: sidebar bottom area or top navigation bar.
//
// Visibility: only rendered when authStore.isSuperAdmin === true.
// =============================================================================

import { useAuthStore } from '~/stores/useAuthStore'
import { useAdminAPI, type Organization } from '~/composables/useAdminAPI'

const authStore = useAuthStore()
const adminAPI = useAdminAPI()

// --- State ---
const isOpen = ref(false)
const searchQuery = ref('')
const organizations = ref<Organization[]>([])
const isLoading = ref(false)
const loadError = ref<string | null>(null)

// Reference for the dropdown container (click-outside detection).
const dropdownRef = ref<HTMLElement | null>(null)

// --- Computed ---

/** The currently selected organization object, or null for "All". */
const selectedOrg = computed(() => {
  if (!authStore.selectedOrgId) return null
  return organizations.value.find(o => o.orgId === authStore.selectedOrgId) || null
})

/** Display label for the trigger button. */
const triggerLabel = computed(() => {
  return selectedOrg.value?.name || 'Все организации'
})

/** Filtered organizations based on the search query. */
const filteredOrgs = computed(() => {
  if (!searchQuery.value.trim()) return organizations.value
  const q = searchQuery.value.toLowerCase().trim()
  return organizations.value.filter(org =>
    org.name.toLowerCase().includes(q)
    || org.orgId.toLowerCase().includes(q)
    || (org.city && org.city.toLowerCase().includes(q))
  )
})

// --- Plan badge colors ---
function planBadgeClasses(plan: string): string {
  switch (plan) {
    case 'enterprise':
      return 'bg-purple-500/15 text-purple-400 border-purple-500/25'
    case 'professional':
    case 'pro':
      return 'bg-blue-500/15 text-blue-400 border-blue-500/25'
    case 'starter':
    case 'basic':
      return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/25'
    case 'trial':
      return 'bg-amber-500/15 text-amber-400 border-amber-500/25'
    default:
      return 'bg-[var(--argus-bg-elevated)] text-[var(--argus-text-dimmed)] border-[var(--argus-border)]'
  }
}

// --- Actions ---

async function fetchOrganizations() {
  if (!authStore.isSuperAdmin) return
  isLoading.value = true
  loadError.value = null

  try {
    organizations.value = await adminAPI.listOrgs()
  } catch (err: any) {
    loadError.value = err?.message || 'Ошибка загрузки организаций'
    console.error('[OrgSwitcher] Failed to fetch organizations:', err)
  } finally {
    isLoading.value = false
  }
}

function toggleDropdown() {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    searchQuery.value = ''
    // Refresh the list every time we open, in case new orgs were added.
    fetchOrganizations()
  }
}

function selectOrg(orgId: string | null) {
  authStore.switchOrg(orgId)
  isOpen.value = false
  searchQuery.value = ''
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    isOpen.value = false
  }
}

// --- Lifecycle ---

onMounted(() => {
  if (authStore.isSuperAdmin) {
    fetchOrganizations()
  }
  document.addEventListener('click', handleClickOutside, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside, true)
})
</script>

<template>
  <!-- Only visible to Super Admin users -->
  <div
    v-if="authStore.isSuperAdmin"
    ref="dropdownRef"
    class="org-switcher relative"
  >
    <!-- Trigger Button -->
    <button
      class="org-switcher-trigger flex items-center gap-2 w-full px-3 py-2 rounded-lg text-left transition-all duration-150"
      :class="isOpen ? 'org-switcher-trigger--active' : ''"
      @click="toggleDropdown"
    >
      <!-- Org icon -->
      <div class="org-switcher-icon flex-shrink-0 flex items-center justify-center w-7 h-7 rounded-md">
        <UIcon
          name="i-lucide-building-2"
          class="w-4 h-4"
          :class="authStore.selectedOrgId ? 'text-[var(--argus-accent)]' : 'text-[var(--argus-text-dimmed)]'"
        />
      </div>

      <!-- Label -->
      <div class="flex-1 min-w-0">
        <p
          class="text-xs font-medium truncate"
          style="color: var(--argus-text);"
        >
          {{ triggerLabel }}
        </p>
        <p
          class="text-[10px] truncate"
          style="color: var(--argus-text-dimmed);"
        >
          {{ authStore.selectedOrgId ? authStore.selectedOrgId : 'Глобальный контекст' }}
        </p>
      </div>

      <!-- Chevron -->
      <UIcon
        :name="isOpen ? 'i-lucide-chevron-up' : 'i-lucide-chevron-down'"
        class="w-3.5 h-3.5 flex-shrink-0 transition-transform duration-150"
        style="color: var(--argus-text-dimmed);"
      />
    </button>

    <!-- Dropdown Panel -->
    <Transition
      enter-active-class="transition-all duration-150 ease-out"
      enter-from-class="opacity-0 translate-y-1 scale-[0.98]"
      enter-to-class="opacity-100 translate-y-0 scale-100"
      leave-active-class="transition-all duration-100 ease-in"
      leave-from-class="opacity-100 translate-y-0 scale-100"
      leave-to-class="opacity-0 translate-y-1 scale-[0.98]"
    >
      <div
        v-if="isOpen"
        class="org-switcher-dropdown absolute bottom-full left-0 right-0 mb-1 z-50 rounded-xl overflow-hidden"
      >
        <!-- Search input -->
        <div class="org-switcher-search px-3 py-2">
          <div class="flex items-center gap-2 px-2.5 py-1.5 rounded-lg org-switcher-search-input">
            <UIcon
              name="i-lucide-search"
              class="w-3.5 h-3.5 flex-shrink-0"
              style="color: var(--argus-text-dimmed);"
            />
            <input
              v-model="searchQuery"
              type="text"
              placeholder="Поиск организации..."
              class="flex-1 bg-transparent text-xs outline-none placeholder:text-[var(--argus-text-dimmed)]"
              style="color: var(--argus-text);"
            >
          </div>
        </div>

        <!-- Divider -->
        <div
          class="h-px"
          style="background: var(--argus-border);"
        />

        <!-- "All Organizations" option -->
        <button
          class="org-switcher-item flex items-center gap-2.5 w-full px-3 py-2 text-left transition-colors duration-100"
          :class="!authStore.selectedOrgId ? 'org-switcher-item--selected' : ''"
          @click="selectOrg(null)"
        >
          <div
            class="flex items-center justify-center w-6 h-6 rounded-md"
            style="background: var(--argus-bg-elevated);"
          >
            <UIcon
              name="i-lucide-globe"
              class="w-3.5 h-3.5"
              :class="!authStore.selectedOrgId ? 'text-[var(--argus-accent)]' : 'text-[var(--argus-text-dimmed)]'"
            />
          </div>
          <div class="flex-1 min-w-0">
            <p
              class="text-xs font-medium"
              style="color: var(--argus-text);"
            >
              Все организации
            </p>
            <p
              class="text-[10px]"
              style="color: var(--argus-text-dimmed);"
            >
              Агрегированный просмотр
            </p>
          </div>
          <UIcon
            v-if="!authStore.selectedOrgId"
            name="i-lucide-check"
            class="w-3.5 h-3.5 flex-shrink-0 text-[var(--argus-accent)]"
          />
        </button>

        <!-- Divider -->
        <div
          class="h-px"
          style="background: var(--argus-border);"
        />

        <!-- Organizations list -->
        <div
          class="org-switcher-list overflow-y-auto"
          style="max-height: 240px;"
        >
          <!-- Loading state -->
          <div
            v-if="isLoading"
            class="flex items-center justify-center gap-2 px-3 py-4"
          >
            <UIcon
              name="i-lucide-loader-2"
              class="w-4 h-4 animate-spin"
              style="color: var(--argus-text-dimmed);"
            />
            <span
              class="text-xs"
              style="color: var(--argus-text-dimmed);"
            >Загрузка...</span>
          </div>

          <!-- Error state -->
          <div
            v-else-if="loadError"
            class="flex items-center gap-2 px-3 py-3"
          >
            <UIcon
              name="i-lucide-alert-circle"
              class="w-4 h-4 flex-shrink-0 text-red-400"
            />
            <span class="text-xs text-red-400">{{ loadError }}</span>
          </div>

          <!-- Empty search results -->
          <div
            v-else-if="filteredOrgs.length === 0 && searchQuery"
            class="px-3 py-4 text-center"
          >
            <UIcon
              name="i-lucide-search-x"
              class="w-5 h-5 mx-auto mb-1"
              style="color: var(--argus-text-dimmed);"
            />
            <p
              class="text-xs"
              style="color: var(--argus-text-dimmed);"
            >
              Ничего не найдено
            </p>
          </div>

          <!-- Empty list -->
          <div
            v-else-if="filteredOrgs.length === 0"
            class="px-3 py-4 text-center"
          >
            <p
              class="text-xs"
              style="color: var(--argus-text-dimmed);"
            >
              Нет организаций
            </p>
          </div>

          <!-- Org items -->
          <button
            v-for="org in filteredOrgs"
            :key="org.orgId"
            class="org-switcher-item flex items-center gap-2.5 w-full px-3 py-2 text-left transition-colors duration-100"
            :class="authStore.selectedOrgId === org.orgId ? 'org-switcher-item--selected' : ''"
            @click="selectOrg(org.orgId)"
          >
            <!-- Org avatar -->
            <div
              class="flex items-center justify-center w-6 h-6 rounded-md text-[10px] font-bold flex-shrink-0"
              style="background: var(--argus-bg-elevated); color: var(--argus-accent);"
            >
              {{ org.name.charAt(0).toUpperCase() }}
            </div>

            <!-- Org info -->
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-1.5">
                <p
                  class="text-xs font-medium truncate"
                  style="color: var(--argus-text);"
                >
                  {{ org.name }}
                </p>
                <!-- Plan badge -->
                <span
                  class="inline-flex items-center px-1.5 py-0.5 rounded text-[9px] font-medium border flex-shrink-0"
                  :class="planBadgeClasses(org.plan)"
                >
                  {{ org.plan }}
                </span>
              </div>
              <p
                class="text-[10px] truncate"
                style="color: var(--argus-text-dimmed);"
              >
                {{ org.orgId }}
                <template v-if="org.city">
                  &middot; {{ org.city }}
                </template>
              </p>
            </div>

            <!-- Active indicator -->
            <div class="flex items-center gap-1.5 flex-shrink-0">
              <span
                v-if="!org.isActive"
                class="w-1.5 h-1.5 rounded-full bg-red-500/60"
                title="Неактивна"
              />
              <UIcon
                v-if="authStore.selectedOrgId === org.orgId"
                name="i-lucide-check"
                class="w-3.5 h-3.5 text-[var(--argus-accent)]"
              />
            </div>
          </button>
        </div>

        <!-- Footer with org count -->
        <div
          class="h-px"
          style="background: var(--argus-border);"
        />
        <div class="px-3 py-1.5">
          <p
            class="text-[10px] tabular-nums"
            style="color: var(--argus-text-dimmed);"
          >
            {{ organizations.length }} {{ organizations.length === 1 ? 'организация' : 'организаций' }}
          </p>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
/* ---- Trigger button ---- */
.org-switcher-trigger {
  background: var(--argus-bg-card);
  border: 1px solid var(--argus-border);
}

.org-switcher-trigger:hover {
  background: var(--argus-bg-hover);
  border-color: var(--argus-accent);
}

.org-switcher-trigger--active {
  background: var(--argus-bg-hover);
  border-color: var(--argus-accent);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--argus-accent) 15%, transparent);
}

/* ---- Icon container ---- */
.org-switcher-icon {
  background: var(--argus-bg-elevated);
  border: 1px solid var(--argus-border);
}

/* ---- Dropdown panel ---- */
.org-switcher-dropdown {
  background: var(--argus-bg-card);
  border: 1px solid var(--argus-border);
  box-shadow:
    0 8px 24px rgba(0, 0, 0, 0.35),
    0 0 0 1px color-mix(in srgb, var(--argus-border) 50%, transparent);
}

/* ---- Search area ---- */
.org-switcher-search {
  background: var(--argus-bg-card);
}

.org-switcher-search-input {
  background: var(--argus-bg-elevated);
  border: 1px solid var(--argus-border);
  transition: border-color 0.15s ease;
}

.org-switcher-search-input:focus-within {
  border-color: var(--argus-accent);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--argus-accent) 12%, transparent);
}

/* ---- List items ---- */
.org-switcher-item:hover {
  background: var(--argus-bg-hover);
}

.org-switcher-item--selected {
  background: color-mix(in srgb, var(--argus-accent) 8%, transparent);
}

.org-switcher-item--selected:hover {
  background: color-mix(in srgb, var(--argus-accent) 12%, transparent);
}

/* ---- Scrollbar styling ---- */
.org-switcher-list::-webkit-scrollbar {
  width: 4px;
}

.org-switcher-list::-webkit-scrollbar-track {
  background: transparent;
}

.org-switcher-list::-webkit-scrollbar-thumb {
  background: var(--argus-border);
  border-radius: 4px;
}

.org-switcher-list::-webkit-scrollbar-thumb:hover {
  background: var(--argus-text-dimmed);
}
</style>
