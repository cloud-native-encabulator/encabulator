/**
 * Console-specific metrics.
 *
 * The two HTTP duration histograms, the default process metrics and the
 * `/metrics` endpoint all come from the kit; these are the four series that only
 * the console has. They land in the kit's registry (prom-client's default), so
 * they appear on `/metrics` with no extra wiring.
 */

import { counter, gauge, registry, type Registry } from '@encabulator/kit'

export interface ConsoleMetrics {
  registry: Registry
  /** SSE clients currently attached to /bff/stream. */
  setStreamClients(n: number): void
  /** Upstream polls the live collector attempted, by upstream and result. */
  countUpstreamPoll(upstream: string, result: 'ok' | 'error'): void
  /** Mutating BFF requests refused by the server-side role check. */
  countForbidden(route: string, role: string): void
  /** Topology snapshots assembled, by whether Prometheus and Kubernetes answered. */
  countSnapshot(prometheus: 'ok' | 'error', kubernetes: 'ok' | 'error'): void
  /** Read a metric back in tests without parsing the text format. */
  read(name: string): Promise<{ values: { value: number; labels: Record<string, string> }[] } | null>
}

function build(): ConsoleMetrics {
  const streamClients = gauge({
    name: 'encabulator_console_stream_clients',
    help: 'Server-sent-event clients currently connected to /bff/stream.',
  })
  const upstreamPolls = counter({
    name: 'encabulator_console_upstream_polls_total',
    help: 'Upstream polls performed by the live collector.',
    labelNames: ['upstream', 'result'],
  })
  const forbidden = counter({
    name: 'encabulator_console_forbidden_total',
    help: 'Requests refused by the BFF role check.',
    labelNames: ['http_route', 'role'],
  })
  const snapshots = counter({
    name: 'encabulator_console_topology_snapshots_total',
    help: 'Topology snapshots assembled, by whether each source answered.',
    labelNames: ['prometheus', 'kubernetes'],
  })

  return {
    registry,
    setStreamClients(n) {
      streamClients.set(n)
    },
    countUpstreamPoll(upstream, result) {
      upstreamPolls.inc({ upstream, result })
    },
    countForbidden(route, role) {
      forbidden.inc({ http_route: route, role })
    },
    countSnapshot(prometheus, kubernetes) {
      snapshots.inc({ prometheus, kubernetes })
    },
    async read(name) {
      const metric = registry.getSingleMetric(name)
      if (metric === undefined) return null
      const snapshot = (await metric.get()) as {
        values: { value: number; labels: Record<string, string> }[]
      }
      return snapshot
    },
  }
}

/**
 * A singleton, because prom-client's default registry is one per process and
 * re-registering the same metric name throws. The kit's `counter`/`gauge`
 * factories are idempotent, so building twice is harmless, but sharing one
 * instance keeps the intent obvious.
 */
export const consoleMetrics: ConsoleMetrics = build()
