/**
 * Paths that are neither traced nor subject to injected malaise (design section 7:
 * "Do not trace /healthz, /readyz, /metrics, or the malaise and flag polling loops").
 */

export const HEALTH_PATH = '/healthz';
export const READY_PATH = '/readyz';
export const METRICS_PATH = '/metrics';

export const INFRA_PATHS: readonly string[] = [HEALTH_PATH, READY_PATH, METRICS_PATH];

/** True for `/healthz`, `/readyz` and `/metrics`, with or without a query string. */
export function isInfraPath(url: string | undefined): boolean {
  if (!url) return false;
  const path = url.split('?', 1)[0] ?? url;
  return INFRA_PATHS.includes(path);
}
