/**
 * Serving the SPA.
 *
 * This suite exists because of a bug that unit tests with `serveStatic: false`
 * could never have caught: `@fastify/static`'s `setHeaders` hook is handed a
 * FastifyReply, not a raw `ServerResponse`, and calling `setHeader` on it throws
 * inside the send stream where Fastify's error handler cannot reach it · taking
 * the process down on the first page load. So these tests serve real files from a
 * temporary directory laid out exactly like `dist/web`, and assert the headers.
 */

import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import { buildApp } from '../app.js'
import { consoleMetrics } from '../platform/index.js'
import {
  EMPTY_CLUSTER,
  SILENT_LOGGER,
  fakeCluster,
  fakeGateway,
  fakePrometheus,
  testConfig,
} from './helpers.js'
import type { FastifyInstance } from 'fastify'

let root: string
let app: FastifyInstance
let collectorClose: () => void

beforeAll(async () => {
  root = mkdtempSync(join(tmpdir(), 'encabulator-web-'))
  mkdirSync(join(root, 'assets'))
  writeFileSync(
    join(root, 'index.html'),
    '<!doctype html><html><body><div id="panel"></div></body></html>',
  )
  writeFileSync(join(root, 'assets', 'index-a1b2c3d4.js'), 'console.log(1)\n')
  writeFileSync(join(root, 'assets', 'index-a1b2c3d4.css'), '.plate{}\n')
  writeFileSync(join(root, 'favicon.svg'), '<svg xmlns="http://www.w3.org/2000/svg"/>')

  const config = testConfig()
  const built = await buildApp({
    config: { ...config, webRoot: root },
    logger: SILENT_LOGGER,
    metrics: consoleMetrics,
    gateway: fakeGateway(config),
    prometheus: fakePrometheus(),
    cluster: fakeCluster(EMPTY_CLUSTER),
    serveStatic: true,
  })
  app = built.app
  collectorClose = () => built.collector.close()
  await app.ready()
})

afterAll(async () => {
  collectorClose()
  await app.close()
  rmSync(root, { recursive: true, force: true })
})

describe('the built SPA', () => {
  it('serves index.html at the root', async () => {
    const res = await app.inject({ method: 'GET', url: '/' })
    expect(res.statusCode).toBe(200)
    expect(res.headers['content-type']).toContain('text/html')
    expect(res.body).toContain('id="panel"')
  })

  it('tells caches never to keep index.html, because it names the asset hashes', async () => {
    const res = await app.inject({ method: 'GET', url: '/index.html' })
    expect(res.statusCode).toBe(200)
    expect(res.headers['cache-control']).toBe('no-cache')
  })

  it('lets a fingerprinted asset be cached forever', async () => {
    const res = await app.inject({ method: 'GET', url: '/assets/index-a1b2c3d4.js' })
    expect(res.statusCode).toBe(200)
    expect(res.headers['cache-control']).toBe('public, max-age=31536000, immutable')
  })

  it('serves a file at the root of the web directory', async () => {
    const res = await app.inject({ method: 'GET', url: '/favicon.svg' })
    expect(res.statusCode).toBe(200)
  })

  it('returns index.html for a deep link, so a pasted URL works', async () => {
    for (const path of ['/works', '/malaise', '/girdlespring', '/administration', '/sign-in']) {
      const res = await app.inject({ method: 'GET', url: path })
      expect(res.statusCode, path).toBe(200)
      expect(res.body, path).toContain('id="panel"')
    }
  })

  it('returns index.html for a deep link with a query string', async () => {
    const res = await app.inject({ method: 'GET', url: '/scenarios?run=run_1' })
    expect(res.statusCode).toBe(200)
    expect(res.body).toContain('id="panel"')
  })

  it('does not hand HTML to a fetch() for an unknown API path', async () => {
    const res = await app.inject({ method: 'GET', url: '/bff/not-a-route' })
    expect(res.statusCode).toBe(404)
    expect(res.headers['content-type']).toContain('application/json')
    expect(res.json().error.code).toBe('not_found')
  })

  it('does not hand HTML to a request for a missing file', async () => {
    const res = await app.inject({ method: 'GET', url: '/assets/gone-deadbeef.js' })
    expect(res.statusCode).toBe(404)
    expect(res.body).not.toContain('id="panel"')
  })

  it('does not answer a POST to a client route with HTML', async () => {
    const res = await app.inject({ method: 'POST', url: '/works' })
    expect(res.statusCode).toBe(404)
    expect(res.headers['content-type']).toContain('application/json')
  })

  it('still serves the probes and the BFF alongside the SPA', async () => {
    expect((await app.inject({ url: '/healthz' })).statusCode).toBe(200)
    expect((await app.inject({ url: '/readyz' })).statusCode).toBe(200)
    expect((await app.inject({ url: '/metrics' })).statusCode).toBe(200)
    expect((await app.inject({ url: '/bff/session' })).statusCode).toBe(200)
  })
})
