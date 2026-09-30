/**
 * Start-up pieces of the kit, re-exported here so `run.ts` imports from one
 * place and the kit's surface stays visible in a single file.
 */

export { createLogger, serveApp, startDefaultMetrics } from '@encabulator/kit'
export type { Logger, RunningService, Telemetry } from '@encabulator/kit'

import { startDefaultMetrics } from '@encabulator/kit'

let started = false

/** prom-client's default collectors may only be registered once per process. */
export function startDefaultMetricsOnce(): void {
  if (started) return
  started = true
  startDefaultMetrics()
}
