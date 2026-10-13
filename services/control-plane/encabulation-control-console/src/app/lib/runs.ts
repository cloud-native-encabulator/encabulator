/**
 * Which scenario runs still hold the orchestrator.
 *
 * The orchestrator allows one run at a time. A run reports `running` until it
 * completes, fails or is cancelled (`RunStatus` in the demo-scenario-orchestrator's
 * `models.py`: running, completed, failed, cancelled). `pending` is treated as
 * active as well, so a queued state could never unlock a second run. Anything
 * else, including a status the orchestrator did not report, is a run that is
 * over: the page shows it as the last run, offers no Cancel, and lets the next
 * run start.
 */
export const ACTIVE_RUN_STATUSES: ReadonlySet<string> = new Set(['running', 'pending'])

export function isRunActive(status: string | null | undefined): boolean {
  return typeof status === 'string' && ACTIVE_RUN_STATUSES.has(status.trim().toLowerCase())
}
