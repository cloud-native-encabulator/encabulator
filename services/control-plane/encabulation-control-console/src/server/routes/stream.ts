/**
 * `GET /bff/stream`: the live feed, as server-sent events.
 *
 * Fastify's reply is hijacked so the response stays open and unbuffered. The
 * headers matter for deployment: `Cache-Control: no-cache, no-transform` and
 * `X-Accel-Buffering: no` tell nginx-family ingress controllers not to buffer
 * the stream, which would otherwise hold every frame until the buffer filled and
 * make the panel look frozen.
 */

import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify'
import type { Collector } from '../collector.js'
import { EVENT_TOPOLOGY } from '../collector.js'
import type { SseSink } from '../hub.js'
import type { Logger } from '../platform/index.js'

export interface StreamDeps {
  collector: Collector
  logger: Logger
}

export function sinkFromReply(reply: FastifyReply): SseSink {
  const raw = reply.raw
  return {
    write(chunk: string): boolean {
      return raw.write(chunk)
    },
    end(): void {
      raw.end()
    },
    get closed(): boolean {
      return raw.destroyed === true || raw.writableEnded === true
    },
  }
}

export function registerStreamRoutes(app: FastifyInstance, deps: StreamDeps): void {
  app.get('/stream', async (request: FastifyRequest, reply: FastifyReply) => {
    reply.raw.writeHead(200, {
      'content-type': 'text/event-stream; charset=utf-8',
      'cache-control': 'no-cache, no-transform',
      connection: 'keep-alive',
      // Ask nginx-style proxies not to buffer; harmless everywhere else.
      'x-accel-buffering': 'no',
    })
    reply.hijack()

    const sink = sinkFromReply(reply)
    deps.collector.hub.add(sink)

    const detach = (): void => {
      deps.collector.hub.remove(sink)
    }
    request.raw.on('close', detach)
    request.raw.on('error', detach)
    reply.raw.on('error', detach)
  })

  // A non-streaming fallback so a deep link, a curl, or a test can read the
  // current snapshot without opening a stream. It answers from the cache when
  // there is one, and assembles on demand when there is not.
  app.get('/topology', async (_request: FastifyRequest, reply: FastifyReply) => {
    const cached = deps.collector.hub.cached(EVENT_TOPOLOGY)
    if (cached !== null) {
      await reply.send(cached)
      return
    }
    await reply.send(await deps.collector.buildTopology())
  })

  app.get('/headline', async (_request: FastifyRequest, reply: FastifyReply) => {
    await reply.send(await deps.collector.buildHeadlineStats())
  })
}
