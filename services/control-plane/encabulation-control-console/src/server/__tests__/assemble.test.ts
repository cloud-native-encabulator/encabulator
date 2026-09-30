/**
 * Topology assembly, from recorded Prometheus and Kubernetes responses.
 *
 * These tests are the guard on the honesty rules. Each one states a rule the
 * design imposes and then breaks it on purpose to prove the code refuses.
 */

import { describe, expect, it } from 'vitest'
import { assembleEdges, assembleTopology, weightedMean } from '../topology/assemble.js'
import { buildClusterSnapshot, mapPod, unavailableCluster } from '../topology/kubernetes.js'
import { parseVector, type PromQueryResult } from '../topology/prometheus.js'
import { REGISTRY } from '../../shared/registry.js'
import { classifyHealth } from '../../shared/topology.js'

function vector(rows: [Record<string, string>, string][]): PromQueryResult {
  return parseVector({
    status: 'success',
    data: {
      resultType: 'vector',
      result: rows.map(([metric, value]) => ({ metric, value: [1_700_000_000, value] })),
    },
  })
}

const NO_ROWS = vector([])

const POD_LIST = [
  {
    metadata: {
      name: 'panametric-fam-processor-7d4-abcde',
      labels: { 'app.kubernetes.io/name': 'panametric-fam-processor' },
    },
    status: {
      phase: 'Running',
      conditions: [{ type: 'Ready', status: 'True' }],
      containerStatuses: [{ ready: true, restartCount: 2 }],
    },
  },
  {
    metadata: {
      name: 'panametric-fam-processor-7d4-fghij',
      labels: { 'app.kubernetes.io/name': 'panametric-fam-processor' },
    },
    status: {
      phase: 'Running',
      conditions: [{ type: 'Ready', status: 'False' }],
      containerStatuses: [{ ready: false, restartCount: 7 }],
    },
  },
  {
    metadata: {
      name: 'valkey-0',
      labels: { 'app.kubernetes.io/name': 'valkey' },
    },
    status: { phase: 'Running', conditions: [{ type: 'Ready', status: 'True' }] },
  },
]

const DEPLOYMENT_LIST = [
  {
    metadata: {
      name: 'panametric-fam-processor',
      labels: { 'app.kubernetes.io/name': 'panametric-fam-processor' },
    },
    spec: { replicas: 3 },
    status: { readyReplicas: 2 },
  },
]

describe('the node registry', () => {
  it('holds the fifteen services and four backing stores from the design', () => {
    expect(REGISTRY.filter((n) => n.kind === 'service')).toHaveLength(15)
    expect(REGISTRY.filter((n) => n.kind === 'store')).toHaveLength(4)
  })

  it('gives every node an epigraph and a plain-English explanation', () => {
    for (const node of REGISTRY) {
      expect(node.epigraph.length, node.name).toBeGreaterThan(10)
      expect(node.plainly.length, node.name).toBeGreaterThan(10)
    }
  })
})

describe('merging Kubernetes pods onto registry nodes', () => {
  it('reads phase, readiness and restarts verbatim', () => {
    const pod = mapPod(POD_LIST[1]!)
    expect(pod).toEqual({
      name: 'panametric-fam-processor-7d4-fghij',
      app: 'panametric-fam-processor',
      phase: 'Running',
      ready: false,
      restarts: 7,
    })
  })

  it('keeps every registry node even when the cluster reports no pods for it', () => {
    const snapshot = assembleTopology({
      cluster: buildClusterSnapshot({ pods: POD_LIST, deployments: DEPLOYMENT_LIST }),
      pods: { rps: NO_ROWS, errors: NO_ROWS, p50: NO_ROWS, p99: NO_ROWS },
      edges: { rps: NO_ROWS, failed: NO_ROWS, p50: NO_ROWS, p99: NO_ROWS },
    })
    expect(snapshot.nodes).toHaveLength(19)
    const quoter = snapshot.nodes.find((n) => n.name === 'spurving-bearing-quoter')
    expect(quoter?.pods).toEqual([])
    expect(quoter?.health).toBe('nodata')
  })

  it('reports deployment replicas, and null for a StatefulSet-backed store', () => {
    const snapshot = assembleTopology({
      cluster: buildClusterSnapshot({ pods: POD_LIST, deployments: DEPLOYMENT_LIST }),
      pods: { rps: NO_ROWS, errors: NO_ROWS, p50: NO_ROWS, p99: NO_ROWS },
      edges: { rps: NO_ROWS, failed: NO_ROWS, p50: NO_ROWS, p99: NO_ROWS },
    })
    const processor = snapshot.nodes.find((n) => n.name === 'panametric-fam-processor')
    expect(processor?.desiredReplicas).toBe(3)
    expect(processor?.readyReplicas).toBe(2)
    expect(processor?.pods).toHaveLength(2)

    const valkey = snapshot.nodes.find((n) => n.name === 'valkey')
    expect(valkey?.desiredReplicas).toBeNull()
    expect(valkey?.pods.map((p) => p.name)).toEqual(['valkey-0'])
  })

  it('says so when the Kubernetes API is unreachable, rather than showing zero pods as fact', () => {
    const snapshot = assembleTopology({
      cluster: unavailableCluster('connect ECONNREFUSED 10.96.0.1:443'),
      pods: null,
      edges: null,
    })
    expect(snapshot.sources.kubernetes.ok).toBe(false)
    expect(snapshot.sources.kubernetes.detail).toContain('ECONNREFUSED')
    expect(snapshot.nodes.every((n) => n.pods.length === 0)).toBe(true)
  })
})

describe('per-pod statistics from http_server_request_duration_seconds', () => {
  const pods = {
    rps: vector([
      [{ pod: 'panametric-fam-processor-7d4-abcde' }, '8'],
      [{ pod: 'panametric-fam-processor-7d4-fghij' }, '2'],
    ]),
    errors: vector([[{ pod: 'panametric-fam-processor-7d4-fghij' }, '1']]),
    p50: vector([
      [{ pod: 'panametric-fam-processor-7d4-abcde' }, '0.012'],
      [{ pod: 'panametric-fam-processor-7d4-fghij' }, '0.030'],
    ]),
    p99: vector([
      [{ pod: 'panametric-fam-processor-7d4-abcde' }, '0.040'],
      [{ pod: 'panametric-fam-processor-7d4-fghij' }, '0.620'],
    ]),
  }

  const snapshot = assembleTopology({
    cluster: buildClusterSnapshot({ pods: POD_LIST, deployments: DEPLOYMENT_LIST }),
    pods,
    edges: { rps: NO_ROWS, failed: NO_ROWS, p50: NO_ROWS, p99: NO_ROWS },
  })
  const processor = snapshot.nodes.find((n) => n.name === 'panametric-fam-processor')!

  it('converts seconds to milliseconds and computes an error ratio per pod', () => {
    const bad = processor.pods.find((p) => p.name.endsWith('fghij'))!
    expect(bad.rps).toBe(2)
    expect(bad.p50Ms).toBeCloseTo(30)
    expect(bad.p99Ms).toBeCloseTo(620)
    expect(bad.errorRatio).toBeCloseTo(0.5)
  })

  it('leaves a pod Prometheus has not scraped as null, not zero', () => {
    const valkey = snapshot.nodes.find((n) => n.name === 'valkey')!
    expect(valkey.pods[0]?.rps).toBeNull()
    expect(valkey.pods[0]?.p99Ms).toBeNull()
    expect(valkey.pods[0]?.errorRatio).toBeNull()
  })

  it('rolls a node up as the sum of rates and the worst pod p99', () => {
    expect(processor.rps).toBe(10)
    expect(processor.p99Ms).toBeCloseTo(620)
    expect(processor.errorRatio).toBeCloseTo(0.1)
    expect(processor.health).toBe('faulted')
  })

  it('weights the node p50 by request rate', () => {
    // (0.012*8 + 0.030*2) / 10 = 0.0156s
    expect(processor.p50Ms).toBeCloseTo(15.6, 1)
  })
})

describe('edges come only from the OTel service graph', () => {
  it('draws nothing when the service graph has no series, however sure we are the call exists', () => {
    const { edges } = assembleEdges({ rps: NO_ROWS, failed: NO_ROWS, p50: NO_ROWS, p99: NO_ROWS })
    expect(edges).toEqual([])
  })

  it('builds an edge with rate, error ratio and latency', () => {
    const { edges } = assembleEdges({
      rps: vector([
        [{ client: 'dingle-arm-gateway', server: 'panametric-fam-processor' }, '20'],
      ]),
      failed: vector([
        [{ client: 'dingle-arm-gateway', server: 'panametric-fam-processor' }, '0.1'],
      ]),
      p50: vector([
        [{ client: 'dingle-arm-gateway', server: 'panametric-fam-processor' }, '0.020'],
      ]),
      p99: vector([
        [{ client: 'dingle-arm-gateway', server: 'panametric-fam-processor' }, '0.085'],
      ]),
    })
    expect(edges).toHaveLength(1)
    expect(edges[0]).toMatchObject({
      source: 'dingle-arm-gateway',
      target: 'panametric-fam-processor',
      via: 'http',
      rps: 20,
      health: 'healthy',
    })
    expect(edges[0]!.errorRatio).toBeCloseTo(0.005)
    expect(edges[0]!.p99Ms).toBeCloseTo(85)
  })

  it('classifies a database edge and a messaging edge from the target', () => {
    const { edges } = assembleEdges({
      rps: vector([
        [{ client: 'hydrocoptic-datastore', server: 'postgresql' }, '5'],
        [{ client: 'panametric-fam-processor', server: 'nats' }, '9'],
      ]),
      failed: NO_ROWS,
      p50: NO_ROWS,
      p99: NO_ROWS,
    })
    expect(edges.find((e) => e.target === 'postgres')?.via).toBe('database')
    expect(edges.find((e) => e.target === 'nats')?.via).toBe('messaging')
  })

  it('reports a peer outside the registry as unmapped rather than inventing a node', () => {
    const { edges, unmapped } = assembleEdges({
      rps: vector([
        [{ client: 'some-other-workload', server: 'panametric-fam-processor' }, '3'],
        [{ client: 'dingle-arm-gateway', server: 'panametric-fam-processor' }, '4'],
      ]),
      failed: NO_ROWS,
      p50: NO_ROWS,
      p99: NO_ROWS,
    })
    expect(edges).toHaveLength(1)
    expect(unmapped).toEqual([
      { client: 'some-other-workload', server: 'panametric-fam-processor', rps: 3 },
    ])
  })

  it('drops the observability platform rather than drawing it', () => {
    const { edges, unmapped } = assembleEdges({
      rps: vector([[{ client: 'prometheus', server: 'panametric-fam-processor' }, '1']]),
      failed: NO_ROWS,
      p50: NO_ROWS,
      p99: NO_ROWS,
    })
    expect(edges).toEqual([])
    expect(unmapped).toEqual([])
  })

  it('merges aliases of the same store onto one edge, adding rates', () => {
    const { edges } = assembleEdges({
      rps: vector([
        [{ client: 'prefabulated-amulite-depot', server: 'valkey' }, '4'],
        [{ client: 'prefabulated-amulite-depot', server: 'redis' }, '6'],
      ]),
      failed: NO_ROWS,
      p50: NO_ROWS,
      p99: vector([
        [{ client: 'prefabulated-amulite-depot', server: 'valkey' }, '0.004' ],
        [{ client: 'prefabulated-amulite-depot', server: 'redis' }, '0.009' ],
      ]),
    })
    expect(edges).toHaveLength(1)
    expect(edges[0]?.rps).toBe(10)
    expect(edges[0]?.p99Ms).toBeCloseTo(9)
  })

  it('keeps an edge that has latency but no rate series', () => {
    const { edges } = assembleEdges({
      rps: NO_ROWS,
      failed: NO_ROWS,
      p50: NO_ROWS,
      p99: vector([
        [{ client: 'dingle-arm-gateway', server: 'spurving-bearing-quoter' }, '0.300'],
      ]),
    })
    expect(edges).toHaveLength(1)
    expect(edges[0]?.rps).toBeNull()
    expect(edges[0]?.health).toBe('degraded')
  })
})

describe('health colouring', () => {
  it('follows the design: green under 100ms and 1%, yellow under 500ms, red beyond', () => {
    expect(classifyHealth(40, 0.001, 10)).toBe('healthy')
    expect(classifyHealth(40, 0.02, 10)).toBe('degraded')
    expect(classifyHealth(300, 0, 10)).toBe('degraded')
    expect(classifyHealth(900, 0, 10)).toBe('faulted')
    expect(classifyHealth(10, 0.4, 10)).toBe('faulted')
  })

  it('is gray, not green, when nothing was measured', () => {
    expect(classifyHealth(null, null, null)).toBe('nodata')
    expect(classifyHealth(null, null, 0)).toBe('nodata')
  })
})

describe('prometheus responses', () => {
  it('turns NaN into null', () => {
    const result = vector([[{ pod: 'a' }, 'NaN']])
    expect(result.samples[0]?.value).toBeNull()
  })

  it('reports a Prometheus error as not-ok rather than as an empty factory', () => {
    const result = parseVector({ status: 'error', error: 'parse error at char 12' })
    expect(result.ok).toBe(false)
    expect(result.detail).toContain('parse error')
    expect(result.samples).toEqual([])
  })

  it('marks the prometheus source down when both query sets failed', () => {
    const failedQueries: PromQueryResult = {
      ok: false,
      detail: 'prometheus returned HTTP 503',
      samples: [],
    }
    const snapshot = assembleTopology({
      cluster: buildClusterSnapshot({ pods: POD_LIST, deployments: [] }),
      pods: { rps: failedQueries, errors: failedQueries, p50: failedQueries, p99: failedQueries },
      edges: { rps: failedQueries, failed: failedQueries, p50: failedQueries, p99: failedQueries },
    })
    expect(snapshot.sources.prometheus.ok).toBe(false)
    expect(snapshot.sources.prometheus.detail).toContain('503')
    expect(snapshot.edges).toEqual([])
  })
})

describe('weightedMean', () => {
  it('ignores nulls and falls back to a plain mean when no weights exist', () => {
    expect(weightedMean([{ value: null, weight: 5 }])).toBeNull()
    expect(weightedMean([{ value: 10, weight: null }, { value: 20, weight: null }])).toBe(15)
    expect(weightedMean([{ value: 10, weight: 3 }, { value: 20, weight: 1 }])).toBe(12.5)
  })
})
