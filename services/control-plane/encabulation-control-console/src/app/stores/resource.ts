/**
 * A tiny async-resource helper the control stores share.
 *
 * It exists to make one behaviour uniform: when a load fails, the store keeps
 * the last good value and records the reason, and the page shows the reason.
 * It does not substitute an empty object for a failed read, because an empty
 * parts cage and an unreachable depot look very different on a factory floor.
 */

import { ref, type Ref } from 'vue'
import { ApiFailure } from '@/api/client'

export interface Resource<T> {
  data: Ref<T>
  loading: Ref<boolean>
  /** Human readable reason the last load failed, or null. */
  error: Ref<string | null>
  /** True once a load has succeeded at least once. */
  loaded: Ref<boolean>
  /** RFC 3339 of the last successful load. */
  at: Ref<string | null>
  reload(): Promise<void>
  /** Replace the value locally, e.g. after a successful mutation. */
  set(value: T): void
}

export function messageOf(err: unknown): string {
  if (err instanceof ApiFailure) return err.message
  if (err instanceof Error) return err.message
  return String(err)
}

export function createResource<T>(initial: T, load: () => Promise<T>): Resource<T> {
  const data = ref(initial) as Ref<T>
  const loading = ref(false)
  const error = ref<string | null>(null)
  const loaded = ref(false)
  const at = ref<string | null>(null)

  async function reload(): Promise<void> {
    loading.value = true
    try {
      data.value = await load()
      loaded.value = true
      error.value = null
      at.value = new Date().toISOString()
    } catch (err) {
      error.value = messageOf(err)
    } finally {
      loading.value = false
    }
  }

  return {
    data,
    loading,
    error,
    loaded,
    at,
    reload,
    set(value: T) {
      data.value = value
      at.value = new Date().toISOString()
    },
  }
}

/** Wrap a mutation so the caller gets a message rather than an exception. */
export async function attempt(
  action: () => Promise<unknown>,
): Promise<{ ok: true } | { ok: false; message: string; code: string }> {
  try {
    await action()
    return { ok: true }
  } catch (err) {
    return {
      ok: false,
      message: messageOf(err),
      code: err instanceof ApiFailure ? err.code : 'unknown',
    }
  }
}
