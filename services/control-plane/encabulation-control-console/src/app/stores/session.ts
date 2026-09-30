/**
 * The session store.
 *
 * `canOperate` and `canAdminister` come from the server, not from inspecting a
 * token in the browser. The store uses them to grey out controls, which is a
 * courtesy: the BFF refuses the request regardless.
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { ApiFailure, api } from '@/api/client'

export interface PublicSession {
  authenticated: boolean
  username: string
  displayName: string
  roles: string[]
  expiresAt: string | null
  canOperate: boolean
  canAdminister: boolean
}

export interface UpstreamInfo {
  service: string
  configured: boolean
  env: string
}

export interface SessionPayload {
  session: PublicSession
  links: { grafana: string | null; jaeger: string | null }
  upstreams: Record<string, UpstreamInfo>
  prometheusConfigured: boolean
}

export const GUEST_SESSION: PublicSession = {
  authenticated: false,
  username: 'guest',
  displayName: 'Guest',
  roles: ['viewer'],
  expiresAt: null,
  canOperate: false,
  canAdminister: false,
}

export const useSessionStore = defineStore('session', () => {
  const session = ref<PublicSession>({ ...GUEST_SESSION })
  const links = ref<{ grafana: string | null; jaeger: string | null }>({
    grafana: null,
    jaeger: null,
  })
  const upstreams = ref<Record<string, UpstreamInfo>>({})
  const prometheusConfigured = ref(false)
  const loading = ref(false)
  const loaded = ref(false)
  const error = ref<string | null>(null)

  function apply(payload: SessionPayload): void {
    session.value = payload.session
    links.value = payload.links
    upstreams.value = payload.upstreams
    prometheusConfigured.value = payload.prometheusConfigured
  }

  async function load(): Promise<void> {
    loading.value = true
    error.value = null
    try {
      apply(await api.get<SessionPayload>('/session'))
      loaded.value = true
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
    } finally {
      loading.value = false
    }
  }

  async function signIn(username: string, password: string): Promise<void> {
    error.value = null
    try {
      const result = await api.post<{ session: PublicSession }>('/login', { username, password })
      session.value = result.session
      // Re-read so links and upstream availability are refreshed too.
      await load()
    } catch (err) {
      error.value =
        err instanceof ApiFailure
          ? err.message
          : 'The console could not reach the identity service.'
      throw err
    }
  }

  async function signOut(): Promise<void> {
    try {
      await api.post<{ session: PublicSession }>('/logout')
    } finally {
      session.value = { ...GUEST_SESSION }
      await load()
    }
  }

  const isGuest = computed(() => !session.value.authenticated)
  const canOperate = computed(() => session.value.canOperate)
  const canAdminister = computed(() => session.value.canAdminister)
  const roleLabel = computed(() =>
    session.value.roles.length === 0 ? 'no role' : session.value.roles.join(', '),
  )

  /** Which upstreams are unset, so a page can explain an empty plate. */
  const unconfigured = computed(() =>
    Object.values(upstreams.value).filter((u) => !u.configured),
  )

  function upstreamFor(key: string): UpstreamInfo | null {
    return upstreams.value[key] ?? null
  }

  return {
    session,
    links,
    upstreams,
    prometheusConfigured,
    loading,
    loaded,
    error,
    isGuest,
    canOperate,
    canAdminister,
    roleLabel,
    unconfigured,
    upstreamFor,
    apply,
    load,
    signIn,
    signOut,
  }
})
