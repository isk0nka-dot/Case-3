export default defineNuxtRouteMiddleware((to) => {
  // Public pages that don't require auth.
  // The index page (/) is the landing page — always accessible.
  const publicPaths = ['/', '/landing', '/student-guide', '/docs']

  if (publicPaths.includes(to.path)) {
    return // Allow access — no auth required
  }

  // For all other routes, check auth via localStorage
  // (Reading localStorage directly because Pinia may not be hydrated yet in middleware)
  if (import.meta.client) {
    try {
      const stored = localStorage.getItem('argus_auth')
      if (stored) {
        const data = JSON.parse(stored)
        if (data.isAuthenticated) {
          // --- Super Admin Route Protection ---
          // Restrict /dashboard/infrastructure, /api, and /organizations to Super Admin only.
          const superAdminOnlyPaths = ['/dashboard/infrastructure', '/dashboard/executive', '/api', '/organizations']
          if (superAdminOnlyPaths.includes(to.path)) {
            const storedUser = localStorage.getItem('argus_user')
            if (storedUser) {
              const user = JSON.parse(storedUser)
              const isSuperAdmin = user.role === 'super_admin' && user.orgId === '*'
              if (!isSuperAdmin) {
                // Show permission denied notification via query parameter
                // (the dashboard page will pick this up and show a toast)
                return navigateTo('/dashboard?denied=1')
              }
            } else {
              // No user data stored — cannot verify role, block access
              return navigateTo('/dashboard?denied=1')
            }
          }

          // --- Org Admin Route Protection ---
          // Restrict /integrations to org_admin or super_admin.
          const orgAdminPaths = ['/integrations']
          if (orgAdminPaths.includes(to.path)) {
            const storedUser = localStorage.getItem('argus_user')
            if (storedUser) {
              const user = JSON.parse(storedUser)
              const isAdmin = user.role === 'org_admin' || (user.role === 'super_admin' && user.orgId === '*')
              if (!isAdmin) {
                return navigateTo('/dashboard?denied=1')
              }
            } else {
              return navigateTo('/dashboard?denied=1')
            }
          }

          return // Allow access — user is authenticated
        }
      }
    }
    catch {
      // Fall through to redirect
    }
  }

  // Not authenticated — redirect to the index (landing) page
  return navigateTo('/')
})
