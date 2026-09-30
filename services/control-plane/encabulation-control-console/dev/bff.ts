/**
 * DEV ONLY. Never copied into the production image.
 *
 * The production `buildApp`, wired to the fixture cluster reader instead of the
 * real Kubernetes API. Everything else · the BFF routes, the role checks, the
 * SSE collector, the topology assembler · is exactly the shipped code, so what
 * you develop against is the real thing with one seam replaced.
 *
 *   npm run dev:bff
 */

import { buildApp } from '../src/server/app.ts'
import { loadConfig } from '../src/server/config.ts'
import { createLogger, serveApp } from '@encabulator/kit'
import { fixtureClusterReader } from './cluster-fixture.ts'

const config = loadConfig()
const logger = createLogger({ service: config.serviceName, level: config.logLevel })

const { app, collector } = await buildApp({
  config,
  logger,
  cluster: fixtureClusterReader(),
  // Serve dist/web if it has been built; otherwise Vite serves the SPA and
  // proxies /bff here.
  serveStatic: true,
})

await serveApp(app, {
  port: config.port,
  host: '127.0.0.1',
  logger,
  shutdownHooks: [() => collector.close()],
})

logger.info(
  { port: config.port },
  'development BFF up, with fixture pods instead of a Kubernetes cluster',
)
