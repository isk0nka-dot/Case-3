import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

// =============================================================================
// Argus AI — Auth Store (Multi-Tenant RBAC)
// =============================================================================
//
// Manages authentication state for the Argus SaaS platform.
// Supports both the demo login (hardcoded) and real JWT-based authentication
// against the admin API (/api/v1/auth/login).
//
// Role hierarchy: super_admin > org_admin > proctor > viewer
// Super Admin has org_id = '*' — sees all organizations.
// =============================================================================

export type UserRole = 'super_admin' | 'org_admin' | 'proctor' | 'viewer'

export interface AuthUser {
  id: string
  orgId: string
  phone: string
  fullName: string
  email?: string
  role: UserRole
  isActive: boolean
}

export const useAuthStore = defineStore('auth', () => {
  // --- State ---
  const isAuthenticated = ref(false)
  const userPhone = ref<string | null>(null)

  // JWT token from admin API authentication.
  const jwtToken = ref<string | null>(null)

  // Session metadata.
  const sessionId = ref<string | null>(null)
  const userId = ref<string | null>(null)
  const orgId = ref<string | null>(null)

  // Full user object from API.
  const user = ref<AuthUser | null>(null)

  // Currently selected org context for Super Admin global switcher.
  // null = "All Organizations" (aggregate view).
  // '*' is the super admin's own org_id.
  const selectedOrgId = ref<string | null>(null)

  // --- Computed ---
  const isLoggedIn = computed(() => isAuthenticated.value)
  const hasValidToken = computed(() => !!jwtToken.value)

  const userRole = computed((): UserRole => {
    return user.value?.role || 'viewer'
  })

  const isSuperAdmin = computed(() => {
    return user.value?.role === 'super_admin' && user.value?.orgId === '*'
  })

  const isOrgAdmin = computed(() => {
    return user.value?.role === 'org_admin' || isSuperAdmin.value
  })

  const canManageOrgs = computed(() => isSuperAdmin.value)
  const canManageUsers = computed(() => isOrgAdmin.value)

  // The effective org_id for data filtering:
  // - Super Admin with no selection → null (show all)
  // - Super Admin with specific org → that org_id
  // - Regular user → their own org_id
  const effectiveOrgId = computed((): string | null => {
    if (isSuperAdmin.value) {
      return selectedOrgId.value // null = all orgs, or specific org_id
    }
    return user.value?.orgId || orgId.value || null
  })

  // Display name for the user.
  const displayName = computed(() => {
    return user.value?.fullName || 'Пользователь'
  })

  const displayEmail = computed(() => {
    return user.value?.email || user.value?.phone || ''
  })

  // --- Actions ---

  /**
   * Login via admin API. Called from useAdminAPI().login() after successful auth.
   * The API client calls setToken() and setUserData() after the fetch.
   *
   * For backward compatibility, also supports the demo login flow.
   */
  function login(phone: string, _password: string): { success: boolean, error?: string } {
    // This is the legacy demo-only flow. Real login goes through useAdminAPI.
    // Keep for backward compat during transition.
    const normalizedPhone = phone.startsWith('+7') ? phone : `+7${phone}`

    // Mark as authenticated — the real auth state comes from API response.
    isAuthenticated.value = true
    userPhone.value = normalizedPhone

    if (import.meta.client) {
      localStorage.setItem('argus_auth', JSON.stringify({
        isAuthenticated: true,
        userPhone: normalizedPhone
      }))
    }

    return { success: true }
  }

  /**
   * Set JWT token from API authentication.
   */
  function setToken(token: string, session?: string, uid?: string, org?: string) {
    jwtToken.value = token
    isAuthenticated.value = true
    if (session) sessionId.value = session
    if (uid) userId.value = uid
    if (org) orgId.value = org

    if (import.meta.client) {
      localStorage.setItem('argus_jwt', token)
      // Persist auth state so the global middleware recognizes the session
      // across page navigations and full reloads.
      localStorage.setItem('argus_auth', JSON.stringify({
        isAuthenticated: true,
        userPhone: userPhone.value
      }))
    }
  }

  /**
   * Set full user data from API response.
   */
  function setUserData(userData: AuthUser) {
    user.value = userData
    userPhone.value = userData.phone
    orgId.value = userData.orgId
    userId.value = userData.id

    if (import.meta.client) {
      localStorage.setItem('argus_user', JSON.stringify(userData))
    }
  }

  /**
   * Clear JWT token (logout or token expiry).
   */
  function clearToken() {
    jwtToken.value = null
    sessionId.value = null
    userId.value = null
    orgId.value = null

    if (import.meta.client) {
      localStorage.removeItem('argus_jwt')
    }
  }

  /**
   * Switch the active organization context (Super Admin only).
   * @param orgIdValue - The org_id to switch to, or null for "All Organizations"
   */
  function switchOrg(orgIdValue: string | null) {
    if (!isSuperAdmin.value) return
    selectedOrgId.value = orgIdValue

    if (import.meta.client) {
      if (orgIdValue) {
        localStorage.setItem('argus_selected_org', orgIdValue)
      } else {
        localStorage.removeItem('argus_selected_org')
      }
    }
  }

  function logout() {
    isAuthenticated.value = false
    userPhone.value = null
    user.value = null
    selectedOrgId.value = null
    clearToken()
    if (import.meta.client) {
      localStorage.removeItem('argus_auth')
      localStorage.removeItem('argus_user')
      localStorage.removeItem('argus_selected_org')
    }
  }

  function restoreSession() {
    if (import.meta.client) {
      try {
        // Restore basic auth state.
        const stored = localStorage.getItem('argus_auth')
        if (stored) {
          const data = JSON.parse(stored)
          if (data.isAuthenticated) {
            isAuthenticated.value = true
            userPhone.value = data.userPhone
          }
        }

        // Restore JWT token.
        const storedToken = localStorage.getItem('argus_jwt')
        if (storedToken) {
          jwtToken.value = storedToken
        }

        // Restore user data.
        const storedUser = localStorage.getItem('argus_user')
        if (storedUser) {
          const userData = JSON.parse(storedUser) as AuthUser
          user.value = userData
          orgId.value = userData.orgId
          userId.value = userData.id
        }

        // Restore selected org context.
        const storedOrg = localStorage.getItem('argus_selected_org')
        if (storedOrg) {
          selectedOrgId.value = storedOrg
        }
      } catch {
        // Corrupted storage — clear everything.
        localStorage.removeItem('argus_auth')
        localStorage.removeItem('argus_jwt')
        localStorage.removeItem('argus_user')
        localStorage.removeItem('argus_selected_org')
      }
    }
  }

  return {
    // State
    isAuthenticated,
    userPhone,
    jwtToken,
    sessionId,
    userId,
    orgId,
    user,
    selectedOrgId,

    // Computed
    isLoggedIn,
    hasValidToken,
    userRole,
    isSuperAdmin,
    isOrgAdmin,
    canManageOrgs,
    canManageUsers,
    effectiveOrgId,
    displayName,
    displayEmail,

    // Actions
    login,
    logout,
    setToken,
    setUserData,
    clearToken,
    switchOrg,
    restoreSession
  }
})
