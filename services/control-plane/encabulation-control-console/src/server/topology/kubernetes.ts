/**
 * The Kubernetes reader: read only, and nothing else.
 *
 * The design allows the console exactly two verbs' worth of access: list pods
 * and list deployments labelled `app.kubernetes.io/part-of=encabulator` in its
 * own namespace. It never patches, scales, deletes, or creates anything. When
 * the demo needs the cluster changed, the console asks the malaise injection
 * engine, which holds the write permissions.
 *
 * The reader is an interface so tests can supply recorded API responses, and so
 * a console running outside a cluster degrades to "no data" instead of crashing.
 */

import { AppsV1Api, CoreV1Api, KubeConfig } from '@kubernetes/client-node'
import type { Logger } from '../platform/index.js'

export const PART_OF_SELECTOR = 'app.kubernetes.io/part-of=encabulator'
export const NAME_LABEL = 'app.kubernetes.io/name'

export interface ObservedPod {
  name: string
  /** `app.kubernetes.io/name`, i.e. the registry node this pod belongs to. */
  app: string
  phase: string
  ready: boolean
  restarts: number
}

export interface ObservedWorkload {
  app: string
  desiredReplicas: number | null
  readyReplicas: number | null
}

export interface ClusterSnapshot {
  ok: boolean
  detail: string | null
  pods: ObservedPod[]
  workloads: ObservedWorkload[]
}

export interface ClusterReader {
  readonly configured: boolean
  read(): Promise<ClusterSnapshot>
}

/** Minimal shapes of the API objects we read, so tests need no client library. */
export interface RawPod {
  metadata?: { name?: string; labels?: Record<string, string> }
  status?: {
    phase?: string
    containerStatuses?: { ready?: boolean; restartCount?: number }[]
    conditions?: { type?: string; status?: string }[]
  }
}

export interface RawDeployment {
  metadata?: { name?: string; labels?: Record<string, string> }
  spec?: { replicas?: number }
  status?: { readyReplicas?: number }
}

export function mapPod(raw: RawPod): ObservedPod | null {
  const name = raw.metadata?.name
  if (name === undefined || name === '') return null
  const app = raw.metadata?.labels?.[NAME_LABEL] ?? ''
  const statuses = raw.status?.containerStatuses ?? []
  const conditions = raw.status?.conditions ?? []
  const readyCondition = conditions.find((c) => c.type === 'Ready')
  const ready =
    readyCondition !== undefined
      ? readyCondition.status === 'True'
      : statuses.length > 0 && statuses.every((c) => c.ready === true)
  const restarts = statuses.reduce((sum, c) => sum + (c.restartCount ?? 0), 0)
  return {
    name,
    app,
    phase: raw.status?.phase ?? 'Unknown',
    ready,
    restarts,
  }
}

export function mapDeployment(raw: RawDeployment): ObservedWorkload | null {
  const app = raw.metadata?.labels?.[NAME_LABEL] ?? raw.metadata?.name
  if (app === undefined || app === '') return null
  return {
    app,
    desiredReplicas: raw.spec?.replicas ?? null,
    readyReplicas: raw.status?.readyReplicas ?? 0,
  }
}

/** Assemble a snapshot from already-fetched API lists. Pure, hence testable. */
export function buildClusterSnapshot(opts: {
  pods: RawPod[]
  deployments: RawDeployment[]
}): ClusterSnapshot {
  return {
    ok: true,
    detail: null,
    pods: opts.pods.map(mapPod).filter((p): p is ObservedPod => p !== null),
    workloads: opts.deployments
      .map(mapDeployment)
      .filter((w): w is ObservedWorkload => w !== null),
  }
}

export function unavailableCluster(detail: string): ClusterSnapshot {
  return { ok: false, detail, pods: [], workloads: [] }
}

export function createClusterReader(opts: {
  namespace: string
  logger: Logger
  /** Set false to skip the cluster entirely (used by `dev` mock mode). */
  enabled?: boolean
}): ClusterReader {
  if (opts.enabled === false) {
    return {
      configured: false,
      read: async () => unavailableCluster('kubernetes reader disabled'),
    }
  }

  let core: CoreV1Api | null = null
  let apps: AppsV1Api | null = null
  let initError: string | null = null

  try {
    const kc = new KubeConfig()
    // In a pod this reads the service account token; on a laptop, ~/.kube/config.
    kc.loadFromDefault()
    core = kc.makeApiClient(CoreV1Api)
    apps = kc.makeApiClient(AppsV1Api)
  } catch (err) {
    initError = err instanceof Error ? err.message : String(err)
    opts.logger.warn({ detail: initError }, 'no kubernetes credentials; pods will show as no data')
  }

  return {
    configured: initError === null,
    async read(): Promise<ClusterSnapshot> {
      if (core === null || apps === null) {
        return unavailableCluster(initError ?? 'kubernetes client not initialised')
      }
      try {
        // @kubernetes/client-node 1.x: object-style parameters, body returned
        // directly. Read only: `list`, nothing more.
        const [podList, deploymentList] = await Promise.all([
          core.listNamespacedPod({
            namespace: opts.namespace,
            labelSelector: PART_OF_SELECTOR,
          }),
          apps.listNamespacedDeployment({
            namespace: opts.namespace,
            labelSelector: PART_OF_SELECTOR,
          }),
        ])
        return buildClusterSnapshot({
          pods: (podList.items ?? []) as RawPod[],
          deployments: (deploymentList.items ?? []) as RawDeployment[],
        })
      } catch (err) {
        const detail = err instanceof Error ? err.message : String(err)
        return unavailableCluster(detail)
      }
    },
  }
}
