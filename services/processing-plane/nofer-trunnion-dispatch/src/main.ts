/** Wiring for nofer-trunnion-dispatch. */

import {
  connectNats,
  consume,
  createApp,
  createLogger,
  DEFAULT_ACK_WAIT_MS,
  Durables,
  EventPublisher,
  ensureStream,
  logLevel,
  MalaiseController,
  natsReadiness,
  Subjects,
  serveApp,
  servicePort,
  type Telemetry,
} from '@encabulator/kit';
import { DELIVERY_TICK_MS, DispatchOffice, type InspectionPassed } from './dispatch.js';
import { recordDelivered, recordDispatched, recordInTransit } from './metrics.js';
import { registerRoutes } from './routes.js';
import { createValkey, valkeyReadiness } from './valkey.js';

export const SERVICE_NAME = 'nofer-trunnion-dispatch';

export async function main(telemetry: Telemetry): Promise<void> {
  const logger = createLogger({ service: telemetry.serviceName, level: logLevel() });
  logger.info('opening the dispatch office');

  const redis = createValkey({ connectionName: telemetry.serviceName });
  redis.on('error', (err: Error) => logger.warn({ err: err.message }, 'valkey error'));

  const nc = await connectNats({ name: telemetry.serviceName, logger });
  const jsm = await ensureStream(nc, logger);
  const publisher = new EventPublisher(nc, {
    source: telemetry.serviceName,
    tracer: telemetry.tracer(),
    logger,
  });

  const malaise = new MalaiseController({ serviceName: telemetry.serviceName, logger });
  await malaise.start();

  const dispatch = new DispatchOffice({
    redis,
    publisher,
    logger,
    onShipmentDispatched: recordDispatched,
    onShipmentDelivered: recordDelivered,
    onInTransit: recordInTransit,
  });

  // Consume `inspection.passed`. Malaise slows or naks individual messages.
  const handleInspectionPassed = malaise.wrapMessageHandler(async (event: InspectionPassed) => {
    await dispatch.dispatch(event);
  });

  const subscription = await consume(nc, jsm, {
    spec: {
      durable: Durables.Dispatch,
      filterSubject: Subjects.InspectionPassed,
      ackWaitMs: DEFAULT_ACK_WAIT_MS,
    },
    logger,
    tracer: telemetry.tracer(),
    concurrency: 4,
    handler: async ({ envelope }) => {
      await handleInspectionPassed(envelope.data as InspectionPassed);
    },
  });

  // The delivery loop: a 1s tick publishes `shipment.delivered` for anything due.
  const deliveries = setInterval(() => {
    void dispatch.deliverDue().catch((err: unknown) => {
      logger.warn(
        { err: err instanceof Error ? err.message : String(err) },
        'delivery round failed; will retry',
      );
    });
  }, DELIVERY_TICK_MS);
  deliveries.unref();

  const app = await createApp({
    serviceName: telemetry.serviceName,
    logger,
    telemetry,
    malaise,
    readiness: [valkeyReadiness(redis), natsReadiness(nc)],
    register: async (instance) => {
      registerRoutes(instance, dispatch);
    },
  });

  await serveApp(app, {
    port: servicePort(),
    logger,
    shutdownHooks: [
      () => clearInterval(deliveries),
      () => subscription.stop(),
      () => malaise.stop(),
      async () => {
        await nc.drain().catch(() => nc.close());
      },
      async () => {
        await redis.quit().catch(() => redis.disconnect());
      },
      () => telemetry.shutdown(),
    ],
  });
}
