/**
 * The live store: one `EventSource` on `/bff/stream`, shared by every page.
 *
 * The BFF only polls upstreams while a client is attached, so this store closes
 * the stream when the last consumer releases it and when the tab goes to sleep.
 * That is not an optimisation; it is the mechanism that keeps an unattended
 * console from generating load on the factory.
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { STREAM_URL, api } from '@/api/client'
import type { FeedBatch, FeedItem, HeadlineStats, TopologySnapshot } from '@shared/topology'

export interface ScenarioProgress {
  runId: string | null
  scenarioId: string | null
  status: string | null
  currentStep: string | null
  stepIndex: number | null
  stepCount: number | null
  items: FeedItem[]
}

/** How many rows each feed keeps. Enough to scroll, not enough to leak. */
export const FEED_LIMIT = 120

export type StreamState = 'closed' | 'connecting' | 'open' | 'retrying'

export const useLiveStore = defineStore('live', () => {
  const topology = ref<TopologySnapshot | null>(null)
  const headline = ref<HeadlineStats | null>(null)
  const scenario = ref<ScenarioProgress | null>(null)
  const orders = ref<FeedItem[]>([])
  const incidents = ref<FeedItem[]>([])
  const shipments = ref<FeedItem[]>([])
  const narration = ref<FeedItem[]>([])
  const feedSources = ref<FeedBatch['sources']>({})
  const state = ref<StreamState>('closed')
  const lastError = ref<string | null>(null)
  const lastEventAt = ref<string | null>(null)

  let source: EventSource | null = null
  let holders = 0

  /** The unnamed bucket for anything that is not one of the four feeds. */
  function bucketFor(kind: FeedItem['kind']) {
    switch (kind) {
      case 'order':
        return orders
      case 'incident':
        return incidents
      case 'shipment':
        return shipments
      case 'narration':
        return narration
    }
  }

  function applyFeed(batch: FeedBatch): void {
    feedSources.value = batch.sources
    for (const item of batch.items) {
      const bucket = bucketFor(item.kind)
      // Newest first; the BFF already suppresses repeats, this guards a replay.
      if (bucket.value.some((existing) => existing.id === item.id)) continue
      bucket.value.unshift(item)
      if (bucket.value.length > FEED_LIMIT) bucket.value.length = FEED_LIMIT
    }
  }

  function handle(event: MessageEvent<string>, name: string): void {
    lastEventAt.value = new Date().toISOString()
    let payload: unknown
    try {
      payload = JSON.parse(event.data)
    } catch {
      lastError.value = 'The stream sent a frame this console could not read.'
      return
    }
    switch (name) {
      case 'topology':
        topology.value = payload as TopologySnapshot
        break
      case 'headline':
        headline.value = payload as HeadlineStats
        break
      case 'feed':
        applyFeed(payload as FeedBatch)
        break
      case 'scenario':
        scenario.value = payload as ScenarioProgress | null
        break
    }
  }

  function open(): void {
    if (source !== null) return
    state.value = 'connecting'
    const stream = new EventSource(STREAM_URL, { withCredentials: true })
    source = stream

    stream.onopen = () => {
      state.value = 'open'
      lastError.value = null
    }
    stream.onerror = () => {
      // EventSource reconnects on its own using the server's `retry:` hint.
      state.value = stream.readyState === 2 ? 'closed' : 'retrying'
      lastError.value = 'The live stream dropped. Reconnecting.'
    }
    for (const name of ['topology', 'headline', 'feed', 'scenario']) {
      stream.addEventListener(name, (event) => handle(event as MessageEvent<string>, name))
    }
  }

  function close(): void {
    source?.close()
    source = null
    state.value = 'closed'
  }

  /** Take a reference to the stream. Returns the release function. */
  function attach(): () => void {
    holders += 1
    open()
    let released = false
    return () => {
      if (released) return
      released = true
      holders = Math.max(0, holders - 1)
      if (holders === 0) close()
    }
  }

  /** Pause while the tab is hidden, so the BFF stops polling too. */
  function setVisible(visible: boolean): void {
    if (visible) {
      if (holders > 0) open()
    } else {
      close()
    }
  }

  /** One-shot fetch, for a first paint before the stream warms up. */
  async function primeTopology(): Promise<void> {
    if (topology.value !== null) return
    try {
      topology.value = await api.get<TopologySnapshot>('/topology')
    } catch {
      // The stream will supply it; nothing to report here.
    }
  }

  const nodes = computed(() => topology.value?.nodes ?? [])
  const edges = computed(() => topology.value?.edges ?? [])
  const unmapped = computed(() => topology.value?.unmapped ?? [])
  const sources = computed(
    () =>
      topology.value?.sources ?? {
        kubernetes: { ok: false, detail: 'waiting for the first snapshot' },
        prometheus: { ok: false, detail: 'waiting for the first snapshot' },
      },
  )
  const podCount = computed(() =>
    nodes.value.reduce((total, node) => total + node.pods.length, 0),
  )
  const activeEdges = computed(() => edges.value.filter((e) => e.health !== 'nodata').length)

  return {
    topology,
    headline,
    scenario,
    orders,
    incidents,
    shipments,
    narration,
    feedSources,
    state,
    lastError,
    lastEventAt,
    nodes,
    edges,
    unmapped,
    sources,
    podCount,
    activeEdges,
    attach,
    setVisible,
    primeTopology,
    applyFeed,
    // Exposed for tests: drive the store without a real EventSource.
    handle,
  }
})
