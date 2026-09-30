/**
 * The server-sent-event hub.
 *
 * Two design requirements meet here:
 *
 *  - "Polling of upstreams runs only while at least one client is connected."
 *    The hub counts clients and calls `onFirstClient` / `onLastClient`, so an
 *    idle console puts no load on the factory at all.
 *  - "cache and fan out to all clients". One poll produces one payload, which
 *    every attached client receives. A client attaching mid-cycle is sent the
 *    cached snapshot immediately so the panel is never blank.
 */

export interface SseSink {
  write(chunk: string): boolean
  /** Called when the hub gives up on this client. */
  end(): void
  /** Node's `destroyed`, or equivalent. */
  readonly closed: boolean
}

export interface HubEvent {
  event: string
  data: unknown
  /** Optional SSE id. */
  id?: string
}

export function formatSse(event: HubEvent): string {
  const payload = JSON.stringify(event.data)
  const lines: string[] = []
  if (event.id !== undefined) lines.push(`id: ${event.id}`)
  lines.push(`event: ${event.event}`)
  // A JSON payload never contains a raw newline, so one data line is enough.
  lines.push(`data: ${payload}`)
  return `${lines.join('\n')}\n\n`
}

export class SseHub {
  private clients = new Set<SseSink>()
  /** Last payload per event name, replayed to new clients. */
  private cache = new Map<string, HubEvent>()
  private seq = 0

  constructor(
    private readonly opts: {
      onFirstClient?: () => void
      onLastClient?: () => void
      onCount?: (n: number) => void
      /** Events replayed to a newly attached client, in order. */
      replayEvents?: string[]
    } = {},
  ) {}

  get size(): number {
    return this.clients.size
  }

  add(sink: SseSink): void {
    const wasEmpty = this.clients.size === 0
    this.clients.add(sink)
    this.opts.onCount?.(this.clients.size)

    // Tell the browser how long to wait before reconnecting, and open the
    // stream with a comment so proxies flush their headers immediately.
    sink.write('retry: 3000\n\n')

    for (const name of this.opts.replayEvents ?? [...this.cache.keys()]) {
      const cached = this.cache.get(name)
      if (cached !== undefined) sink.write(formatSse(cached))
    }

    if (wasEmpty) this.opts.onFirstClient?.()
  }

  remove(sink: SseSink): void {
    if (!this.clients.delete(sink)) return
    this.opts.onCount?.(this.clients.size)
    if (this.clients.size === 0) this.opts.onLastClient?.()
  }

  /** Broadcast and cache. Returns the number of clients written to. */
  broadcast(event: string, data: unknown): number {
    this.seq += 1
    const payload: HubEvent = { event, data, id: String(this.seq) }
    this.cache.set(event, payload)
    const frame = formatSse(payload)
    let written = 0
    for (const sink of [...this.clients]) {
      if (sink.closed) {
        this.remove(sink)
        continue
      }
      try {
        sink.write(frame)
        written += 1
      } catch {
        this.remove(sink)
        sink.end()
      }
    }
    return written
  }

  /** SSE comment, to keep idle proxies and load balancers from closing us. */
  heartbeat(): void {
    const frame = `: ping ${new Date().toISOString()}\n\n`
    for (const sink of [...this.clients]) {
      if (sink.closed) {
        this.remove(sink)
        continue
      }
      try {
        sink.write(frame)
      } catch {
        this.remove(sink)
        sink.end()
      }
    }
  }

  cached(event: string): unknown {
    return this.cache.get(event)?.data ?? null
  }

  closeAll(): void {
    for (const sink of [...this.clients]) {
      this.clients.delete(sink)
      try {
        sink.end()
      } catch {
        // The client is already gone; nothing to do.
      }
    }
    this.opts.onCount?.(0)
  }
}
