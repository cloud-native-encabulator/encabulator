import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'console',
    component: () => import('@/views/Console.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/Login.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/admin',
    name: 'admin',
    component: () => import('@/views/admin/AdminLayout.vue'),
    meta: { requiresAuth: true, requiresAdmin: true },
    children: [
      {
        path: '',
        redirect: '/admin/users'
      },
      {
        path: 'users',
        name: 'admin-users',
        component: () => import('@/views/admin/UserManagement.vue')
      },
      {
        path: 'monitoring',
        name: 'admin-monitoring',
        component: () => import('@/views/admin/GrafanaIntegration.vue')
      },
      {
        path: 'config',
        name: 'admin-config',
        component: () => import('@/views/admin/SystemConfig.vue')
      },
      {
        path: 'audit',
        name: 'admin-audit',
        component: () => import('@/views/admin/AuditLog.vue')
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// Navigation guard for authentication
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()
  
  // Initialize session if not already done (creates guest session if needed)
  if (!authStore.user) {
    authStore.initializeSession()
  }
  
  // Check if guest session is expired
  if (authStore.isGuest && !authStore.isGuestSessionValid()) {
    authStore.logout()
    authStore.initializeSession()
  }
  
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    next({ name: 'login', query: { redirect: to.fullPath } })
  } else if (to.meta.requiresAdmin && !authStore.isAdmin) {
    // Guests and non-admins can't access admin portal
    next({ name: 'console' })
  } else {
    next()
  }
})

export default router
