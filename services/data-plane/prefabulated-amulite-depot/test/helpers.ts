import { createApp, createLogger } from '@encabulator/kit';
import type { FastifyInstance } from 'fastify';
import type { Depot } from '../src/depot.js';
import { recordStock } from '../src/metrics.js';
import { registerRoutes } from '../src/routes.js';

/** The real app, minus telemetry, malaise polling and a listening socket. */
export async function buildTestApp(depot: Depot): Promise<FastifyInstance> {
  const app = await createApp({
    serviceName: 'prefabulated-amulite-depot',
    logger: createLogger({ service: 'prefabulated-amulite-depot', level: 'silent' }),
    readiness: [
      {
        name: 'valkey',
        check: async () => {
          await depot.parts();
        },
      },
    ],
    register: async (instance) => {
      registerRoutes(instance, depot);
      instance.addHook('onResponse', async () => {
        recordStock(await depot.parts());
      });
    },
  });
  await app.ready();
  return app;
}
