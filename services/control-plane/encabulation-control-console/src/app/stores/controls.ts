/**
 * The control stores: grammeters, malaise, scenarios, girdlespring, works,
 * administration.
 *
 * Each one is a thin shell over `/bff` calls. None of them decides who may do
 * what; they report what the BFF said. A 401 or 403 becomes a message on the
 * plate, which is how a viewer discovers a control is locked even if the SPA's
 * greying-out were bypassed.
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { api } from '@/api/client'
import { attempt, createResource } from './resource'

// ── Grammeters ───────────────────────────────────────────────────────────────

export type TrafficPattern = 'steady' | 'burst' | 'ramp' | 'sine'

export interface JourneyStats {
  count: number | null
  errors: number | null
  p50Ms: number | null
  p99Ms: number | null
}

export interface TrafficState {
  running?: boolean
  rps?: number
  pattern?: TrafficPattern
  periodSeconds?: number
  customers?: number
  mix?: Record<string, number>
  targetRps?: number
  actualRps?: number
  inFlight?: number
  dropped?: number
  journeys?: Record<string, JourneyStats>
  statusCodes?: Record<string, number>
}

export interface TrafficChange {
  running?: boolean
  rps?: number
  pattern?: TrafficPattern
  periodSeconds?: number
  customers?: number
  mix?: Record<string, number>
}

export const useTrafficStore = defineStore('traffic', () => {
  const traffic = createResource<TrafficState | null>(null, () =>
    api.get<TrafficState>('/traffic'),
  )
  const saving = ref(false)
  const notice = ref<string | null>(null)

  async function change(patch: TrafficChange) {
    saving.value = true
    notice.value = null
    const result = await attempt(async () => {
      traffic.set(await api.put<TrafficState>('/traffic', patch))
    })
    saving.value = false
    if (!result.ok) notice.value = result.message
    return result
  }

  const journeyRows = computed(() => {
    const journeys = traffic.data.value?.journeys ?? {}
    return Object.entries(journeys).map(([name, stats]) => ({ name, ...stats }))
  })

  return { ...traffic, saving, notice, change, journeyRows }
})

// ── Malaise ──────────────────────────────────────────────────────────────────

export type MalaiseKind =
  | 'latency'
  | 'error'
  | 'congestion'
  | 'cpu-burn'
  | 'pod-kill'
  | 'partition'
  | 'scale'

export interface Malaise {
  id: string
  target: string
  kind: MalaiseKind
  params: Record<string, unknown>
  expiresAt: string | null
  createdAt?: string | null
  state?: string
  detail?: string | null
}

/** The seven kinds from design section 8.13, with what each really does. */
export const MALAISE_KINDS: {
  kind: MalaiseKind
  label: string
  real: string
  level: 'app' | 'kubernetes'
  fields: { name: string; label: string; default: number; min: number; max: number }[]
}[] = [
  {
    kind: 'latency',
    label: 'Sluggish repleneration',
    real: 'Sleep before the handler, for a percentage of requests',
    level: 'app',
    fields: [
      { name: 'ms', label: 'Delay (ms)', default: 400, min: 1, max: 30_000 },
      { name: 'jitterMs', label: 'Jitter (ms)', default: 100, min: 0, max: 10_000 },
      { name: 'percent', label: 'Of requests (%)', default: 100, min: 1, max: 100 },
    ],
  },
  {
    kind: 'error',
    label: 'Prefamulated refusal',
    real: 'Respond with an error status instead of calling the handler',
    level: 'app',
    fields: [
      { name: 'status', label: 'HTTP status', default: 503, min: 400, max: 599 },
      { name: 'percent', label: 'Of requests (%)', default: 25, min: 1, max: 100 },
    ],
  },
  {
    kind: 'congestion',
    label: 'Dingle-arm congestion',
    real: 'Cap concurrent handlers; the rest queue then get 503',
    level: 'app',
    fields: [
      { name: 'maxConcurrent', label: 'Max concurrent', default: 2, min: 1, max: 512 },
      { name: 'queueTimeoutMs', label: 'Queue timeout (ms)', default: 2000, min: 1, max: 60_000 },
    ],
  },
  {
    kind: 'cpu-burn',
    label: 'Excessive sinusoidal load',
    real: 'Burn CPU in a background worker until the malaise expires',
    level: 'app',
    fields: [{ name: 'millicores', label: 'Millicores', default: 500, min: 10, max: 4000 }],
  },
  {
    kind: 'pod-kill',
    label: 'Marzlevane shear',
    real: 'Delete ready pods of the target',
    level: 'kubernetes',
    fields: [{ name: 'count', label: 'Pods', default: 1, min: 1, max: 20 }],
  },
  {
    kind: 'partition',
    label: 'Severed tremie pipe',
    real: 'A NetworkPolicy that blocks ingress from the named workloads',
    level: 'kubernetes',
    fields: [],
  },
  {
    kind: 'scale',
    label: 'Girdlespring adjustment',
    real: 'Set the target Deployment’s replica count; restored on expiry',
    level: 'kubernetes',
    fields: [{ name: 'replicas', label: 'Replicas', default: 0, min: 0, max: 20 }],
  },
]

export const useMalaiseStore = defineStore('malaise', () => {
  const malaises = createResource<Malaise[]>([], async () => {
    const body = await api.get<{ malaises?: Malaise[] } | Malaise[]>('/malaises', {
      query: { state: 'all' },
    })
    return Array.isArray(body) ? body : (body.malaises ?? [])
  })
  const notice = ref<string | null>(null)
  const busy = ref(false)

  async function induce(input: {
    target: string
    kind: MalaiseKind
    params: Record<string, unknown>
    durationSeconds: number
  }) {
    busy.value = true
    notice.value = null
    const result = await attempt(() => api.post('/malaises', input))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await malaises.reload()
    return result
  }

  async function clear(id: string) {
    busy.value = true
    const result = await attempt(() => api.delete(`/malaises/${encodeURIComponent(id)}`))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await malaises.reload()
    return result
  }

  async function clearAll() {
    busy.value = true
    const result = await attempt(() => api.delete('/malaises'))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await malaises.reload()
    return result
  }

  const active = computed(() =>
    malaises.data.value.filter((m) => {
      if (m.state === 'active') return true
      if (m.expiresAt === null) return false
      return Date.parse(m.expiresAt) > Date.now()
    }),
  )

  return { ...malaises, notice, busy, active, induce, clear, clearAll }
})

// ── Scenarios ────────────────────────────────────────────────────────────────

export interface Scenario {
  id: string
  title: string
  description?: string
  steps?: unknown[]
}

export interface Run {
  id: string
  scenarioId?: string
  status?: string
  startedAt?: string
  finishedAt?: string | null
  currentStep?: string | null
}

export const useScenarioStore = defineStore('scenarios', () => {
  const scenarios = createResource<Scenario[]>([], async () => {
    const body = await api.get<{ scenarios?: Scenario[] } | Scenario[]>('/scenarios')
    return Array.isArray(body) ? body : (body.scenarios ?? [])
  })
  const runs = createResource<Run[]>([], async () => {
    const body = await api.get<{ runs?: Run[] } | Run[]>('/runs', { query: { limit: 20 } })
    return Array.isArray(body) ? body : (body.runs ?? [])
  })
  const notice = ref<string | null>(null)
  const busy = ref(false)

  async function run(id: string) {
    busy.value = true
    notice.value = null
    const result = await attempt(() => api.post(`/scenarios/${encodeURIComponent(id)}/runs`))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await runs.reload()
    return result
  }

  async function cancel(runId: string) {
    busy.value = true
    const result = await attempt(() => api.post(`/runs/${encodeURIComponent(runId)}/cancel`))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await runs.reload()
    return result
  }

  return {
    scenarios: scenarios.data,
    scenariosError: scenarios.error,
    scenariosLoading: scenarios.loading,
    loadScenarios: scenarios.reload,
    runs: runs.data,
    runsError: runs.error,
    loadRuns: runs.reload,
    notice,
    busy,
    run,
    cancel,
  }
})

// ── Girdlespring ─────────────────────────────────────────────────────────────

export interface Flag {
  key: string
  type: 'boolean' | 'number' | 'string'
  value: boolean | number | string
  rolloutPercent?: number | null
  description?: string | null
  updatedAt?: string | null
  updatedBy?: string | null
}

export const useFlagStore = defineStore('flags', () => {
  const flags = createResource<Flag[]>([], async () => {
    const body = await api.get<{ flags?: Flag[] } | Flag[]>('/flags')
    return Array.isArray(body) ? body : (body.flags ?? [])
  })
  const notice = ref<string | null>(null)
  const saving = ref<string | null>(null)

  async function set(key: string, patch: { value: unknown; rolloutPercent?: number | null }) {
    saving.value = key
    notice.value = null
    const result = await attempt(() => api.put(`/flags/${encodeURIComponent(key)}`, patch))
    saving.value = null
    if (!result.ok) notice.value = result.message
    else await flags.reload()
    return result
  }

  return { ...flags, notice, saving, set }
})

// ── Works: parts, orders, shipments, incidents, storefront ───────────────────

export interface Part {
  sku: string
  name: string
  stock: number | null
  reserved: number | null
  perUnit: number | null
}

export interface Order {
  id: string
  customer?: string
  model?: string
  quantity?: number
  status?: string
  stage?: string | null
  attempt?: number
  serials?: string[]
  carrier?: string | null
  trackingNumber?: string | null
  createdAt?: string
  updatedAt?: string
  quote?: { total?: number; currency?: string }
  timeline?: { time: string; type: string; detail?: string }[]
}

export interface Quote {
  id: string
  model: string
  quantity: number
  unitPrice?: number
  subtotal?: number
  surcharges?: { code: string; amount: number }[]
  total?: number
  currency?: string
  cached?: boolean
  computedAt?: string
}

export const useWorksStore = defineStore('works', () => {
  const parts = createResource<Part[]>([], async () => {
    const body = await api.get<{ parts?: Part[] } | Part[]>('/parts')
    return Array.isArray(body) ? body : (body.parts ?? [])
  })
  const orders = createResource<Order[]>([], async () => {
    const body = await api.get<{ orders?: Order[] } | Order[]>('/orders', { query: { limit: 25 } })
    return Array.isArray(body) ? body : (body.orders ?? [])
  })

  const quote = ref<Quote | null>(null)
  const placed = ref<Order | null>(null)
  const notice = ref<string | null>(null)
  const busy = ref(false)

  async function restock(sku: string, quantity: number) {
    busy.value = true
    notice.value = null
    const result = await attempt(() =>
      api.post(`/parts/${encodeURIComponent(sku)}/restock`, { quantity }),
    )
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await parts.reload()
    return result
  }

  async function getQuote(input: { model: string; quantity: number; options: { dingleArm: boolean } }) {
    busy.value = true
    notice.value = null
    const result = await attempt(async () => {
      quote.value = await api.post<Quote>('/storefront/quotes', input)
    })
    busy.value = false
    if (!result.ok) {
      quote.value = null
      notice.value = result.message
    }
    return result
  }

  async function placeOrder(input: {
    model: string
    quantity: number
    options: { dingleArm: boolean }
  }) {
    busy.value = true
    notice.value = null
    const result = await attempt(async () => {
      placed.value = await api.post<Order>('/storefront/orders', input)
    })
    busy.value = false
    if (!result.ok) {
      placed.value = null
      notice.value = result.message
    } else {
      await orders.reload()
    }
    return result
  }

  async function trackOrder(id: string): Promise<Order | null> {
    const result = await attempt(async () => {
      placed.value = await api.get<Order>(`/orders/${encodeURIComponent(id)}`)
    })
    if (!result.ok) {
      notice.value = result.message
      return null
    }
    return placed.value
  }

  const lowestStock = computed(() => {
    const rows = parts.data.value.filter((p): p is Part & { stock: number } => p.stock !== null)
    if (rows.length === 0) return null
    return rows.reduce((min, row) => (row.stock < min.stock ? row : min))
  })

  return {
    parts: parts.data,
    partsError: parts.error,
    partsLoading: parts.loading,
    loadParts: parts.reload,
    orders: orders.data,
    ordersError: orders.error,
    loadOrders: orders.reload,
    quote,
    placed,
    notice,
    busy,
    lowestStock,
    restock,
    getQuote,
    placeOrder,
    trackOrder,
  }
})

// ── Administration ───────────────────────────────────────────────────────────

export interface User {
  username: string
  displayName?: string
  roles: string[]
  createdAt?: string
  updatedAt?: string
}

export interface AuditEntry {
  time?: string
  action?: string
  subject?: string
  actor?: string
  detail?: string
  result?: string
}

export const useAdminStore = defineStore('admin', () => {
  const users = createResource<User[]>([], async () => {
    const body = await api.get<{ users?: User[] } | User[]>('/users')
    return Array.isArray(body) ? body : (body.users ?? [])
  })
  const audit = createResource<AuditEntry[]>([], async () => {
    const body = await api.get<{ entries?: AuditEntry[]; audit?: AuditEntry[] } | AuditEntry[]>(
      '/audit',
      { query: { limit: 100 } },
    )
    if (Array.isArray(body)) return body
    return body.entries ?? body.audit ?? []
  })
  const notice = ref<string | null>(null)
  const busy = ref(false)

  async function create(input: { username: string; password: string; displayName: string; roles: string[] }) {
    busy.value = true
    notice.value = null
    const result = await attempt(() => api.post('/users', input))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await users.reload()
    return result
  }

  async function setRoles(username: string, roles: string[]) {
    busy.value = true
    const result = await attempt(() =>
      api.patch(`/users/${encodeURIComponent(username)}`, { roles }),
    )
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await users.reload()
    return result
  }

  async function remove(username: string) {
    busy.value = true
    const result = await attempt(() => api.delete(`/users/${encodeURIComponent(username)}`))
    busy.value = false
    if (!result.ok) notice.value = result.message
    else await users.reload()
    return result
  }

  return {
    users: users.data,
    usersError: users.error,
    usersLoading: users.loading,
    loadUsers: users.reload,
    audit: audit.data,
    auditError: audit.error,
    loadAudit: audit.reload,
    notice,
    busy,
    create,
    setRoles,
    remove,
  }
})
