import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { User } from '@/types'

const GUEST_SESSION_DURATION = 24 * 60 * 60 * 1000 // 24 hours in milliseconds

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string | null>(localStorage.getItem('auth_token'))
  const guestSessionExpiry = ref<number | null>(
    parseInt(localStorage.getItem('guest_session_expiry') || '0') || null
  )

  const isAuthenticated = computed(() => !!token.value && !!user.value)
  const isGuest = computed(() => user.value?.role === 'guest')
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isOperator = computed(() => user.value?.role === 'operator' || isAdmin.value)

  function setUser(userData: User) {
    user.value = userData
  }

  function setToken(authToken: string) {
    token.value = authToken
    localStorage.setItem('auth_token', authToken)
  }

  function logout() {
    user.value = null
    token.value = null
    localStorage.removeItem('auth_token')
  }

  async function login(username: string, password: string) {
    try {
      // Call the unilateral-phase-detractor API
      const response = await fetch('http://unilateral-phase-detractor.encabulator-support.svc.cluster.local:8080/api/v1/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password })
      })
      
      if (!response.ok) {
        throw new Error('Login failed')
      }
      
      const data = await response.json()
      setToken(data.access_token)
      
      // Fetch user details
      await fetchCurrentUser()
      
      return user.value
    } catch (error) {
      console.error('Login error:', error)
      throw error
    }
  }

  async function fetchCurrentUser() {
    if (!token.value) return null
    
    try {
      const response = await fetch('http://unilateral-phase-detractor.encabulator-support.svc.cluster.local:8080/api/v1/users/me', {
        headers: { 'Authorization': `Bearer ${token.value}` }
      })
      
      if (!response.ok) {
        logout()
        return null
      }
      
      const userData = await response.json()
      
      // Map roles to single role (use highest privilege)
      let role: 'admin' | 'operator' | 'viewer' | 'guest' = 'viewer'
      if (userData.roles.includes('admin')) {
        role = 'admin'
      } else if (userData.roles.includes('operator')) {
        role = 'operator'
      }
      
      const mappedUser: User = {
        id: userData.username,
        username: userData.username,
        email: userData.email,
        role
      }
      
      setUser(mappedUser)
      return mappedUser
    } catch (error) {
      console.error('Fetch user error:', error)
      logout()
      return null
    }
  }

  function loginAsGuest() {
    const sessionId = crypto.randomUUID()
    const expiry = Date.now() + GUEST_SESSION_DURATION
    
    const guestUser: User = {
      id: `guest-${sessionId}`,
      username: 'Guest',
      email: 'guest@encabulator.local',
      role: 'guest'
    }
    
    setUser(guestUser)
    setToken(`guest-${sessionId}`)
    guestSessionExpiry.value = expiry
    localStorage.setItem('guest_session_expiry', expiry.toString())
    
    return guestUser
  }

  function isGuestSessionValid(): boolean {
    if (!isGuest.value || !guestSessionExpiry.value) return false
    return Date.now() < guestSessionExpiry.value
  }

  function getGuestSessionTimeRemaining(): number {
    if (!guestSessionExpiry.value) return 0
    const remaining = guestSessionExpiry.value - Date.now()
    return Math.max(0, remaining)
  }

  function initializeSession() {
    // Check if we have a valid token
    if (token.value && !token.value.startsWith('guest-')) {
      // Try to fetch user for authenticated session
      fetchCurrentUser()
      return
    }
    
    // Check if we have a valid guest session
    if (token.value?.startsWith('guest-') && guestSessionExpiry.value) {
      if (isGuestSessionValid()) {
        // Restore guest session
        const sessionId = token.value.replace('guest-', '')
        setUser({
          id: `guest-${sessionId}`,
          username: 'Guest',
          email: 'guest@encabulator.local',
          role: 'guest'
        })
        return
      } else {
        // Guest session expired, clear it
        logout()
      }
    }
    
    // No valid session, create guest session
    loginAsGuest()
  }

  return {
    user,
    token,
    guestSessionExpiry,
    isAuthenticated,
    isGuest,
    isAdmin,
    isOperator,
    setUser,
    setToken,
    logout,
    login,
    fetchCurrentUser,
    loginAsGuest,
    isGuestSessionValid,
    getGuestSessionTimeRemaining,
    initializeSession
  }
})
