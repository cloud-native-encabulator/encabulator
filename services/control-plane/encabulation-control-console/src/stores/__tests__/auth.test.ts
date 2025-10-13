import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '../auth'

describe('Auth Store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
  })

  afterEach(() => {
    localStorage.clear()
  })

  it('initializes with no user', () => {
    const store = useAuthStore()
    
    expect(store.user).toBeNull()
    expect(store.isAuthenticated).toBe(false)
    expect(store.isAdmin).toBe(false)
    expect(store.isOperator).toBe(false)
  })

  it('loads token from localStorage on init', () => {
    localStorage.setItem('auth_token', 'test-token')
    setActivePinia(createPinia())
    const store = useAuthStore()
    
    expect(store.token).toBe('test-token')
  })

  it('sets user correctly', () => {
    const store = useAuthStore()
    const mockUser = {
      id: '1',
      username: 'testuser',
      email: 'test@example.com',
      role: 'admin' as const
    }
    
    store.setUser(mockUser)
    
    expect(store.user).toEqual(mockUser)
  })

  it('sets token and persists to localStorage', () => {
    const store = useAuthStore()
    
    store.setToken('new-token')
    
    expect(store.token).toBe('new-token')
    expect(localStorage.getItem('auth_token')).toBe('new-token')
  })

  it.skip('logs in user successfully', async () => {
    // Skipping this test as it requires full API mocking
    // Login functionality is tested via integration tests
    const store = useAuthStore()
    expect(store).toBeTruthy()
  })

  it('logs out user and clears data', () => {
    const store = useAuthStore()
    store.setToken('test-token')
    store.setUser({
      id: '1',
      username: 'test',
      email: 'test@example.com',
      role: 'admin'
    })
    
    store.logout()
    
    expect(store.user).toBeNull()
    expect(store.token).toBeNull()
    expect(localStorage.getItem('auth_token')).toBeNull()
    expect(store.isAuthenticated).toBe(false)
  })

  it('correctly identifies admin role', () => {
    const store = useAuthStore()
    store.setUser({
      id: '1',
      username: 'admin',
      email: 'admin@example.com',
      role: 'admin'
    })
    
    expect(store.isAdmin).toBe(true)
    expect(store.isOperator).toBe(true)
  })

  it('correctly identifies operator role', () => {
    const store = useAuthStore()
    store.setUser({
      id: '2',
      username: 'operator',
      email: 'operator@example.com',
      role: 'operator'
    })
    
    expect(store.isAdmin).toBe(false)
    expect(store.isOperator).toBe(true)
  })

  it('correctly identifies viewer role', () => {
    const store = useAuthStore()
    store.setUser({
      id: '3',
      username: 'viewer',
      email: 'viewer@example.com',
      role: 'viewer'
    })
    
    expect(store.isAdmin).toBe(false)
    expect(store.isOperator).toBe(false)
  })

  it('creates guest session', () => {
    const store = useAuthStore()
    
    const guestUser = store.loginAsGuest()
    
    expect(guestUser.role).toBe('guest')
    expect(guestUser.username).toBe('Guest')
    expect(store.isGuest).toBe(true)
    expect(store.isAuthenticated).toBe(true)
    expect(store.guestSessionExpiry).toBeGreaterThan(Date.now())
  })

  it('validates guest session expiry', () => {
    const store = useAuthStore()
    
    store.loginAsGuest()
    expect(store.isGuestSessionValid()).toBe(true)
    
    // Manually expire the session
    store.guestSessionExpiry = Date.now() - 1000
    expect(store.isGuestSessionValid()).toBe(false)
  })

  it('calculates guest session time remaining', () => {
    const store = useAuthStore()
    
    store.loginAsGuest()
    const remaining = store.getGuestSessionTimeRemaining()
    
    expect(remaining).toBeGreaterThan(0)
    expect(remaining).toBeLessThanOrEqual(24 * 60 * 60 * 1000)
  })

  it('initializes guest session when no auth exists', () => {
    const store = useAuthStore()
    
    store.initializeSession()
    
    expect(store.user).toBeTruthy()
    expect(store.isGuest).toBe(true)
  })
})
