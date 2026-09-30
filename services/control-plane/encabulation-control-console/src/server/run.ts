/**
 * Start-up and shutdown, separated from `main.ts` so that the telemetry import
 * in `main.ts` is the very first thing the process evaluates.
 *
 * `serveApp` from the kit owns the SIGTERM sequence: fail readiness, stop
 * accepting, drain for up to twenty seconds, run the shutdown hooks (design
 * section 7). The console's hook closes the SSE hub so attached browsers see the
 * stream end and reconnect to the next pod, rather than hanging on a dead socket
 * until their own timeout.
 */

import { buildApp } from './app.js'
import { MissingSessionSecretError, loadConfig } from './config.js'
import { createLogger, serveApp, startDefaultMetricsOnce, type Telemetry } from './startup.js'

export async function run(telemetry: Telemetry): Promise<void> {
  let config
  try {
    config = loadConfig()
  } catch (err) {
    if (err instanceof MissingSessionSecretError) {
      process.stderr.write(`${err.message}\n`)
      process.exit(1)
    }
    throw err
  }

  const logger = createLogger({ service: config.serviceName, level: config.logLevel })
  startDefaultMetricsOnce()

  const { app, collector } = await buildApp({ config, logger, telemetry })

  await serveApp(app, {
    port: config.port,
    host: '0.0.0.0',
    logger,
    shutdownHooks: [
      () => {
        collector.close()
      },
      () => telemetry.shutdown(),
    ],
  })

  logger.info(
    {
      port: config.port,
      namespace: config.podNamespace,
      tracing: telemetry.enabled,
      prometheus: config.prometheusUrl !== null,
      upstreams: Object.fromEntries(
        Object.entries(config.upstreams).map(([key, up]) => [key, up.url !== null]),
      ),
    },
    'the encabulation control console is on the air',
  )
}
