/**
 * DEV ONLY. Never copied into the production image.
 *
 * A `ClusterReader` built from a fixed list of pods, so the SPA can be developed
 * with pod dots on the plates without a Kubernetes cluster. The pod names match
 * the ones `dev/fixture-server.ts` reports metrics for, which is what lets the
 * two halves of the topology merge in dev exactly as they do in a cluster.
 *
 * One pod is deliberately not ready and one is deliberately CrashLoopBackOff, so
 * the amber and red pod dots and the "no data" paths all get exercised while
 * developing.
 */

import type { ClusterReader, ClusterSnapshot, ObservedPod } from '../src/server/topology/kubernetes.ts'

const REPLICAS: Record<string, number> = {
  'logarithmic-casing-line': 3,
  'spurving-bearing-quoter': 2,
}

const SERVICES = [
  'dingle-arm-gateway',
  'panametric-fam-processor',
  'spurving-bearing-quoter',
  'logarithmic-casing-line',
  'nofer-trunnion-dispatch',
  'hydrocoptic-datastore',
  'prefabulated-amulite-depot',
  'sinusoidal-recorder',
  'unilateral-phase-detractor',
  'differential-girdlespring',
  'side-fumbling-detector',
  'cardinal-grammeter',
  'malaise-injection-engine',
  'demo-scenario-orchestrator',
  'encabulation-control-console',
]

const STORES = ['postgres', 'valkey', 'nats', 'influxdb']

function pods(): ObservedPod[] {
  const out: ObservedPod[] = []
  for (const app of SERVICES) {
    const replicas = REPLICAS[app] ?? 1
    for (let i = 0; i < replicas; i += 1) {
      out.push({
        name: `${app}-7d4f9-${'abcde'.slice(i, i + 1)}${i}x`,
        app,
        phase: 'Running',
        ready: true,
        restarts: 0,
      })
    }
  }
  for (const app of STORES) {
    out.push({ name: `${app}-0`, app, phase: 'Running', ready: true, restarts: 0 })
  }

  // One unready pod and one crashing pod, so amber and red dots are visible.
  const unready = out.find((p) => p.app === 'logarithmic-casing-line' && p.name.endsWith('c2x'))
  if (unready !== undefined) unready.ready = false
  const crashing = out.find((p) => p.app === 'side-fumbling-detector')
  if (crashing !== undefined) {
    crashing.phase = 'CrashLoopBackOff'
    crashing.ready = false
    crashing.restarts = 7
  }
  return out
}

export function fixtureClusterReader(): ClusterReader {
  const snapshot: ClusterSnapshot = {
    ok: true,
    detail: null,
    pods: pods(),
    workloads: [...SERVICES].map((app) => ({
      app,
      desiredReplicas: REPLICAS[app] ?? 1,
      readyReplicas: app === 'side-fumbling-detector' ? 0 : (REPLICAS[app] ?? 1),
    })),
  }
  return { configured: true, read: async () => snapshot }
}
