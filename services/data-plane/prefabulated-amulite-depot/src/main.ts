/** Wiring for prefabulated-amulite-depot. */

import {
  createApp,
  createLogger,
  FlagClient,
  logLevel,
  MalaiseController,
  serveApp,
  servicePort,
  type Telemetry,
} from '@encabulator/kit';
import { Depot, FOUNDRY_INTERVAL_MS, SWEEP_INTERVAL_MS } from './depot.js';
import { recordStock } from './metrics.js';
import { registerRoutes } from './routes.js';
import { createValkey, valkeyReadiness } from './valkey.js';

export const SERVICE_NAME = 'prefabulated-amulite-depot';

export async function main(telemetry: Telemetry): Promise<void> {
  const logger = createLogger({ service: telemetry.serviceName, level: logLevel() });
  logger.info('opening the parts cage');

  const redis = createValkey({ connectionName: telemetry.serviceName });
  redis.on('error', (err: Error) => logger.warn({ err: err.message }, 'valkey error'));

  const flags = new FlagClient({ logger });
  const malaise = new MalaiseController({ serviceName: telemetry.serviceName, logger });
  const depot = new Depot({ redis, logger, onStockChange: recordStock });

  await Promise.all([flags.start(), malaise.start()]);

  // Seeding retries in the background: the depot starts, reports itself unready until Valkey
  // answers, and stocks the cage as soon as it does. Blocking startup on Valkey would turn a
  // slow backing store into a crash loop.
  const seeding = { stopped: false };
  void depot.seedUntilSuccessful(2_000, seeding);

  // The amulite foundry: every 10s, add `amulite-foundry-rate` units of each SKU.
  const foundry = setInterval(() => {
    const rate = flags.number('amulite-foundry-rate');
    void depot.runFoundry(rate).catch((err: unknown) => {
      logger.warn(
        { err: err instanceof Error ? err.message : String(err) },
        'the foundry skipped a pour',
      );
    });
  }, FOUNDRY_INTERVAL_MS);
  foundry.unref();

  // The sweeper: uncommitted reservations go back on the shelf after fifteen minutes.
  const sweeper = setInterval(() => {
    void depot.sweepExpired().catch((err: unknown) => {
      logger.warn(
        { err: err instanceof Error ? err.message : String(err) },
        'sweep failed; will retry',
      );
    });
  }, SWEEP_INTERVAL_MS);
  sweeper.unref();

  const app = await createApp({
    serviceName: telemetry.serviceName,
    logger,
    telemetry,
    malaise,
    readiness: [valkeyReadiness(redis)],
    register: async (instance) => {
      registerRoutes(instance, depot);
    },
  });

  await serveApp(app, {
    port: servicePort(),
    logger,
    shutdownHooks: [
      () => {
        seeding.stopped = true;
      },
      () => clearInterval(foundry),
      () => clearInterval(sweeper),
      () => flags.stop(),
      () => malaise.stop(),
      async () => {
        await redis.quit().catch(() => redis.disconnect());
      },
      () => telemetry.shutdown(),
    ],
  });
}
