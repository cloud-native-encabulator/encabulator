/** Wiring for panametric-fam-processor. */

import {
  connectNats,
  consume,
  createApp,
  createLogger,
  Durables,
  EventPublisher,
  ensureStream,
  FlagClient,
  HttpClient,
  type Logger,
  logLevel,
  MalaiseController,
  natsReadiness,
  requireEnv,
  Subjects,
  serveApp,
  servicePort,
  type Telemetry,
} from '@encabulator/kit';
import { CircuitBreaker } from './circuit-breaker.js';
import type { InspectionFailed } from './domain.js';
import { registerRoutes } from './routes.js';
import { type Dependency, OrderSaga } from './saga.js';

export const SERVICE_NAME = 'panametric-fam-processor';

function dependency(name: string, urlVar: string, logger: Logger): Dependency {
  return {
    client: new HttpClient({ name, baseUrl: requireEnv(urlVar), logger }),
    breaker: new CircuitBreaker({ name, logger }),
  };
}

export async function main(telemetry: Telemetry): Promise<void> {
  const logger = createLogger({ service: telemetry.serviceName, level: logLevel() });
  logger.info('spinning up the panametric fam');

  const nc = await connectNats({ name: telemetry.serviceName, logger });
  const jsm = await ensureStream(nc, logger);
  const publisher = new EventPublisher(nc, {
    source: telemetry.serviceName,
    tracer: telemetry.tracer(),
    logger,
  });

  const flags = new FlagClient({ logger });
  const malaise = new MalaiseController({ serviceName: telemetry.serviceName, logger });
  await Promise.all([flags.start(), malaise.start()]);

  const saga = new OrderSaga({
    quoter: dependency('spurving-bearing-quoter', 'SPURVING_BEARING_QUOTER_URL', logger),
    depot: dependency('prefabulated-amulite-depot', 'PREFABULATED_AMULITE_DEPOT_URL', logger),
    datastore: dependency('hydrocoptic-datastore', 'HYDROCOPTIC_DATASTORE_URL', logger),
    publisher,
    flags,
    logger,
  });

  // Consume `inspection.failed`: refabricate, or scrap on the third failure.
  const handleInspectionFailed = malaise.wrapMessageHandler(async (event: InspectionFailed) => {
    await saga.handleInspectionFailed(event);
  });

  const subscription = await consume(nc, jsm, {
    spec: { durable: Durables.Processor, filterSubject: Subjects.InspectionFailed },
    logger,
    tracer: telemetry.tracer(),
    concurrency: 4,
    handler: async ({ envelope }) => {
      await handleInspectionFailed(envelope.data as InspectionFailed);
    },
  });

  const app = await createApp({
    serviceName: telemetry.serviceName,
    logger,
    telemetry,
    malaise,
    // Readiness covers only the processor's own hard dependency: NATS. Downstream services
    // are guarded by circuit breakers, not by readiness (design section 7).
    readiness: [natsReadiness(nc)],
    register: async (instance) => {
      registerRoutes(instance, saga);
    },
  });

  await serveApp(app, {
    port: servicePort(),
    logger,
    shutdownHooks: [
      () => subscription.stop(),
      () => flags.stop(),
      () => malaise.stop(),
      async () => {
        await nc.drain().catch(() => nc.close());
      },
      () => telemetry.shutdown(),
    ],
  });
}
